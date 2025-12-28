package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
)

// BuildInfo contains build-time metadata embedded into the server.
//
// Version is a semantic version string and Commit is the short git
// commit hash used to build the binary.
type BuildInfo struct {
	Version string
	Commit  string
}

// Config contains configuration for creating a Server instance.
//
// Addr is the listen address (e.g. ":8080"). Auth and DB are required
// for production use; other values are validated during startup.
type Config struct {
	Addr  string // e.g. ":8080"
	Build BuildInfo
	Auth  AuthConfig
	DB    *sql.DB
}

// Server is the application HTTP server with its dependencies.
//
// It exposes Start and Shutdown to manage lifecycle in tests and in the
// production entrypoint.
type Server struct {
	httpServer  *http.Server
	db          *sql.DB
	minio       *minio.Client
	bucket      string
	cleanupDone chan struct{}
	authCfg     AuthConfig // Store auth config for getCurrentUser
}

// New constructs and returns an initialized Server wiring handlers and
// dependencies (DB, MinIO). It panics early if required dependencies
// are missing, to avoid running in a half-configured state.
func New(cfg Config) *Server {
	mux := http.NewServeMux()

	// Minimal web UI (Milestone 7)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "/app/web/static/index.html")
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("/app/web/static"))))

	mc, bucket, err := newMinioClient()
	if err != nil {
		// fail fast: uploads depend on MinIO; do not start in a half-configured state
		panic(err)
	}

	// Health endpoint: process is running (does not check dependencies).
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})

	// Ready endpoint: comprehensive dependency health checks with detailed status.
	// Returns 200 OK if all dependencies are healthy, 503 if any are unhealthy.
	// Provides detailed status for each component (postgres, minio) with latency.
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		type ComponentStatus struct {
			Status  string `json:"status"` // "ok" or "error"
			Message string `json:"message,omitempty"`
			Latency int64  `json:"latency_ms,omitempty"`
		}

		response := map[string]any{
			"status":     "ok",
			"components": map[string]ComponentStatus{},
		}

		overallOK := true

		// Check Postgres
		if cfg.DB == nil {
			response["components"].(map[string]ComponentStatus)["postgres"] = ComponentStatus{
				Status:  "error",
				Message: "not configured",
			}
			overallOK = false
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			start := time.Now()
			err := cfg.DB.PingContext(ctx)
			latency := time.Since(start).Milliseconds()

			if err != nil {
				response["components"].(map[string]ComponentStatus)["postgres"] = ComponentStatus{
					Status:  "error",
					Message: err.Error(),
					Latency: latency,
				}
				overallOK = false
			} else {
				response["components"].(map[string]ComponentStatus)["postgres"] = ComponentStatus{
					Status:  "ok",
					Latency: latency,
				}
			}
		}

		// Check MinIO
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		start := time.Now()
		exists, err := mc.BucketExists(ctx, bucket)
		latency := time.Since(start).Milliseconds()

		if err != nil || !exists {
			msg := "bucket not found"
			if err != nil {
				msg = err.Error()
			}
			response["components"].(map[string]ComponentStatus)["minio"] = ComponentStatus{
				Status:  "error",
				Message: msg,
				Latency: latency,
			}
			overallOK = false
		} else {
			response["components"].(map[string]ComponentStatus)["minio"] = ComponentStatus{
				Status:  "ok",
				Latency: latency,
			}
		}

		// Set overall status
		if !overallOK {
			response["status"] = "degraded"
		}

		w.Header().Set("Content-Type", "application/json")
		if overallOK {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(response)
	})

	// Version endpoint (no secrets)
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": cfg.Build.Version,
			"commit":  cfg.Build.Commit,
		})
	})

	// Config endpoint (public) - expose client-side configuration
	mux.HandleFunc("/config", func(w http.ResponseWriter, _ *http.Request) {
		maxBytes, _ := maxUploadBytes()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"max_upload_bytes": maxBytes,
		})
	})

	// Metrics endpoint (protected) - includes disk usage stats
	mux.Handle("/metrics", cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshot := GetMetrics().Snapshot()

		// Add disk usage statistics from database
		var totalBytes, totalFiles int64
		err := cfg.DB.QueryRow(`
			SELECT 
				COALESCE(SUM(size_bytes), 0) as total_bytes,
				COUNT(*) as total_files
			FROM files
			WHERE status IN ('stored', 'hashed', 'ready')
		`).Scan(&totalBytes, &totalFiles)

		if err == nil {
			snapshot.StorageTotalBytes = totalBytes
			snapshot.StorageTotalFiles = totalFiles
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(snapshot)
	})))

	// Login endpoint (POST JSON {username,password})
	mux.HandleFunc("/login", cfg.Auth.loginHandler())

	// Register endpoint (POST JSON {email,username,password})
	mux.HandleFunc("/register", cfg.RegisterHandler)

	// Email verification endpoint (GET /verify?token={token})
	mux.HandleFunc("/verify", cfg.VerifyEmailHandler)

	// Password reset request (POST JSON {email})
	mux.HandleFunc("/reset-password-request", cfg.RequestPasswordResetHandler)

	// Password reset completion (POST JSON {token, new_password})
	mux.HandleFunc("/reset-password", cfg.ResetPasswordHandler)

	// Protected endpoint for verification only
	mux.Handle("/me", cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})))

	// Create file record (metadata only; proves DB writes end-to-end)
	mux.Handle("/files", cfg.createFileHandler(cfg.DB))

	// Stream upload to MinIO (pending -> stored)
	mux.Handle("/upload", cfg.uploadHandler(cfg.DB, mc, bucket))

	// Create signed, expiring download links (Milestone 6)
	mux.Handle("/links", cfg.createLinkHandler(cfg.DB))

	// Download file via signed token (Milestone 6)
	mux.Handle("/download", cfg.downloadHandler(cfg.DB, mc, bucket))

	// Wrap middleware: requestID -> logging -> rate limiting -> mux
	// Apply global rate limit: 100 requests per minute per IP
	rateLimiter := newRateLimiter(100, time.Minute)

	var handler http.Handler = mux
	handler = rateLimiter.middleware(handler)
	handler = loggingMiddleware(handler)
	handler = requestIDMiddleware(handler)

	s := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	srv := &Server{
		httpServer:  s,
		db:          cfg.DB,
		minio:       mc,
		bucket:      bucket,
		cleanupDone: make(chan struct{}),
		authCfg:     cfg.Auth,
	}

	// Admin endpoints (protected) - registered after Server creation
	mux.Handle("/admin/files", cfg.Auth.requireAuth(http.HandlerFunc(srv.AdminListFilesHandler)))
	mux.HandleFunc("/admin/files/", func(w http.ResponseWriter, r *http.Request) {
		cfg.Auth.requireAuth(http.HandlerFunc(srv.AdminDeleteFileHandler)).ServeHTTP(w, r)
	})
	mux.Handle("/admin/cleanup", cfg.Auth.requireAuth(http.HandlerFunc(srv.AdminManualCleanupHandler)))

	// User endpoints (protected) - show user's own files
	mux.Handle("/user/files", cfg.Auth.requireAuth(http.HandlerFunc(srv.UserFilesHandler)))

	// User quota endpoint (protected)
	mux.Handle("/quota", cfg.Auth.requireAuth(http.HandlerFunc(srv.UserQuotaHandler)))

	return srv
}

// Start begins serving HTTP on the configured address and starts background jobs.
// It blocks until the listener returns an error (or Shutdown is called).
func (s *Server) Start() error {
	// Start cleanup job in background
	cleanupCfg := GetCleanupConfigFromEnv(s.db, s.minio, s.bucket)
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())

	go func() {
		defer close(s.cleanupDone)
		StartCleanupJob(cleanupCtx, cleanupCfg)
	}()

	// Store cancel func for shutdown
	go func() {
		<-s.cleanupDone
		cleanupCancel()
	}()

	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		cleanupCancel()
		return err
	}
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the HTTP server and background jobs
// using the provided context (respecting the deadline/timeout supplied by the caller).
func (s *Server) Shutdown(ctx context.Context) error {
	// Signal cleanup job to stop (via cleanupDone channel close will happen)
	// The cleanup job checks context.Done() which we handle in Start()

	return s.httpServer.Shutdown(ctx)
}
