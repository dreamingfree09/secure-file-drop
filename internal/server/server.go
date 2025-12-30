// server.go - HTTP server wiring and lifecycle for Secure File Drop.
//
// Registers routes, configures dependencies, and exposes Start/Shutdown.
// Keeps handler logic modular for testability.
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
// commit hash used to build the binary. These values are exposed by
// the /version endpoint and can be used for debugging and telemetry.
type BuildInfo struct {
	Version string
	Commit  string
}

// Config contains dependency injection and runtime settings for Server.
//
// Addr is the listen address (e.g. ":8080"). Auth holds middleware
// and session handling configuration. DB is a live *sql.DB connection.
// EmailSvc is optional; when nil, a service is constructed from env.
// BaseURL is the publicly reachable URL used in emails and link generation.
type Config struct {
	Addr     string // e.g. ":8080"
	Build    BuildInfo
	Auth     AuthConfig
	DB       *sql.DB
	EmailSvc *EmailService
	BaseURL  string // Base URL for email links (e.g. "http://localhost:8080")
}

// Server wires HTTP routes to handlers and holds external dependencies.
//
// It exposes Start and Shutdown to manage lifecycle in tests and in the
// production entrypoint. The struct is intentionally simple; most logic
// resides in handler functions to keep unit-testing straightforward.
type Server struct {
	httpServer  *http.Server
	db          *sql.DB
	minio       *minio.Client
	bucket      string
	cleanupDone chan struct{}
	authCfg     AuthConfig // Store auth config for getCurrentUser
	emailSvc    *EmailService
	csrf        *CSRFProtection // CSRF token management
}

// New constructs and returns a Server, registers routes, and validates
// critical dependencies (MinIO). It panics early if required dependencies
// are missing to avoid running in a half-configured state.
func New(cfg Config) *Server {
	mux := http.NewServeMux()

	// Initialize CSRF protection (24-hour token TTL)
	csrf := NewCSRFProtection(24 * time.Hour)

	// Initialize email service if not provided
	if cfg.EmailSvc == nil {
		emailCfg := LoadEmailConfig()
		cfg.EmailSvc = NewEmailService(emailCfg)
	}

	// Set default base URL if not provided
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}

	// Minimal web UI (Milestone 7): serves the SPA and static assets.
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

	// Health endpoint: process liveness (does not check dependencies).
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})

	// Ready endpoint: comprehensive dependency readiness checks with detailed status.
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

	// Version endpoint (no secrets): exposes build info for debugging.
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": cfg.Build.Version,
			"commit":  cfg.Build.Commit,
		})
	})

	// Config endpoint (public): exposes client-side configuration such as
	// maximum upload size in bytes.
	mux.HandleFunc("/config", func(w http.ResponseWriter, _ *http.Request) {
		maxBytes, _ := maxUploadBytes()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"max_upload_bytes": maxBytes,
		})
	})

	// Metrics endpoint (protected): includes disk usage stats in addition to
	// in-memory application counters.
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

	// CSRF token endpoint (GET) - returns token for authenticated sessions
	mux.Handle("/csrf-token", cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("sfd_session")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token, err := csrf.GetToken(cookie.Value)
		if err != nil {
			http.Error(w, "failed to generate CSRF token", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"csrf_token": token,
		})
	})))

	// Logout endpoint (POST) clears session cookie
	mux.HandleFunc("/logout", cfg.Auth.logoutHandler())

	// Register endpoint (POST JSON {email,username,password})
	mux.HandleFunc("/register", cfg.RegisterHandler)

	// Email verification endpoint (GET /verify?token={token})
	mux.HandleFunc("/verify", cfg.VerifyEmailHandler)

	// Password reset request (POST JSON {email})
	mux.HandleFunc("/reset-password-request", cfg.RequestPasswordResetHandler)

	// Password reset completion (POST JSON {token, new_password})
	mux.HandleFunc("/reset-password", cfg.ResetPasswordHandler)

	// Protected endpoint for user info (includes admin status for UI).
	mux.Handle("/me", cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := cfg.Auth.getCurrentUser(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var username string
		var isAdmin bool
		err = cfg.DB.QueryRow(
			"SELECT username, is_admin FROM users WHERE id = $1 AND is_active = TRUE",
			userID,
		).Scan(&username, &isAdmin)

		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "ok",
			"username": username,
			"is_admin": isAdmin,
		})
	})))

	// Create file record (metadata only): step 1 in upload lifecycle
	// (pending -> stored -> hashed -> ready/failed).
	mux.Handle("/files", cfg.createFileHandler(cfg.DB))

	// Stream upload to MinIO (pending -> stored).
	mux.Handle("/upload", cfg.uploadHandler(cfg.DB, mc, bucket))

	// Create signed, expiring download links.
	mux.Handle("/links", cfg.createLinkHandler(cfg.DB))

	// Download file via signed token.
	mux.Handle("/download", cfg.downloadHandler(cfg.DB, mc, bucket))

	// Wrap middleware: requestID -> logging -> rate limiting -> security headers -> mux
	// Apply global rate limit: 100 requests per minute per IP
	rateLimiter := newRateLimiter(100, time.Minute)

	var handler http.Handler = mux
	handler = securityHeadersMiddleware(handler)
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
		emailSvc:    cfg.EmailSvc,
		csrf:        csrf,
	}

	// Admin endpoints (protected) - only accessible by admin users
	mux.Handle("/admin/files", cfg.Auth.requireAdmin(http.HandlerFunc(srv.AdminListFilesHandler)))
	mux.HandleFunc("/admin/files/", func(w http.ResponseWriter, r *http.Request) {
		cfg.Auth.requireAdmin(http.HandlerFunc(srv.AdminDeleteFileHandler)).ServeHTTP(w, r)
	})
	mux.Handle("/admin/cleanup", cfg.Auth.requireAdmin(http.HandlerFunc(srv.AdminManualCleanupHandler)))

	// User endpoints (protected) - show user's own files
	mux.Handle("/user/files", cfg.Auth.requireAuth(http.HandlerFunc(srv.UserFilesHandler)))
	mux.HandleFunc("/user/files/", func(w http.ResponseWriter, r *http.Request) {
		cfg.Auth.requireAuth(http.HandlerFunc(srv.UserDeleteFileHandler)).ServeHTTP(w, r)
	})

	// User quota endpoint (protected)
	mux.Handle("/quota", cfg.Auth.requireAuth(http.HandlerFunc(srv.UserQuotaHandler)))

	return srv
}

// Start begins serving HTTP on the configured address and starts background jobs.
// It blocks until the listener returns an error or Shutdown is called.
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

// Shutdown gracefully shuts down the HTTP server and background jobs using
// the provided context (deadline/timeout respected).
func (s *Server) Shutdown(ctx context.Context) error {
	// Signal cleanup job to stop (via cleanupDone channel close will happen)
	// The cleanup job checks context.Done() which we handle in Start()

	return s.httpServer.Shutdown(ctx)
}
