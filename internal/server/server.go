package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
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
	httpServer    *http.Server
	db            *sql.DB
	minio         *minio.Client
	bucket        string
	cleanupDone   chan struct{}
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	requestCancel context.CancelFunc
	requests      sync.WaitGroup
	requestMu     sync.Mutex
	closing       bool
}

// New constructs and returns an initialized Server wiring handlers and
// dependencies (DB, MinIO). It panics early if required dependencies
// are missing, to avoid running in a half-configured state.
func New(cfg Config) *Server {
	if _, err := maxRequestBytes(); err != nil {
		panic("invalid upload envelope configuration")
	}
	if _, err := publicOrigin(); err != nil {
		panic("invalid SFD_PUBLIC_BASE_URL")
	}
	if cfg.Auth.DB == nil {
		cfg.Auth.DB = cfg.DB
	}
	mux := http.NewServeMux()
	webDir := os.Getenv("SFD_WEB_DIR")
	if webDir == "" {
		webDir = "web/static"
	}

	// Minimal web UI (Milestone 7)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// #nosec G703 -- Path is operator configuration plus a constant basename; request path is required to equal /.
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(webDir))))

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

	// Ready endpoint: dependencies are reachable (Postgres and MinIO).
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if cfg.DB == nil {
			http.Error(w, "db not configured", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()

		// Check Postgres
		if err := cfg.DB.PingContext(ctx); err != nil {
			http.Error(w, "db not ready", http.StatusServiceUnavailable)
			return
		}

		// Check MinIO
		exists, err := mc.BucketExists(ctx, bucket)
		if err != nil || !exists {
			http.Error(w, "minio not ready", http.StatusServiceUnavailable)
			return
		}

		if hashToolReady(ctx) != nil {
			http.Error(w, "native hash not ready", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
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

	// Metrics endpoint (protected)
	mux.Handle("/metrics", cfg.Auth.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshot := GetMetrics().Snapshot()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(snapshot)
	})))

	// Login endpoint (POST JSON {username,password})
	mux.HandleFunc("/login", cfg.Auth.loginHandler())

	// Register endpoint (POST JSON {email,username,password})
	mux.HandleFunc("/register", cfg.RegisterHandler)
	mux.Handle("/logout", cfg.Auth.logoutHandler())

	// Protected endpoint for verification only
	mux.Handle("/me", cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "is_admin": principal(r).Admin})
	})))

	// Create file record (metadata only; proves DB writes end-to-end)
	mux.Handle("/files", cfg.createFileHandler(cfg.DB))

	// Stream upload to MinIO (pending -> stored)
	mux.Handle("/upload", cfg.uploadHandler(cfg.DB, mc, bucket))

	// Create signed, expiring download links (Milestone 6)
	mux.Handle("/links", cfg.createLinkHandler(cfg.DB))

	// Download file via signed token (Milestone 6)
	mux.Handle("/download", cfg.downloadHandler(cfg.DB, mc, bucket))

	// Wrap middleware: requestID -> logging -> mux
	var handler http.Handler = mux
	handler = sameOriginMiddleware(handler)
	handler = loggingMiddleware(handler)
	handler = requestIDMiddleware(handler)

	s := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      31 * time.Minute,
		IdleTimeout:       time.Minute,
	}

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	srv := &Server{
		cleanupCtx: cleanupCtx, cleanupCancel: cleanupCancel,
		httpServer:  s,
		db:          cfg.DB,
		minio:       mc,
		bucket:      bucket,
		cleanupDone: make(chan struct{}),
	}

	requestCtx, requestCancel := context.WithCancel(context.Background())
	srv.requestCancel = requestCancel
	s.BaseContext = func(net.Listener) context.Context { return requestCtx }
	s.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.requestMu.Lock()
		if srv.closing {
			srv.requestMu.Unlock()
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		srv.requests.Add(1)
		srv.requestMu.Unlock()
		defer srv.requests.Done()
		handler.ServeHTTP(w, r)
	})

	// Admin endpoints (protected) - registered after Server creation
	mux.Handle("/admin/files", cfg.Auth.requireAdmin(http.HandlerFunc(srv.AdminListFilesHandler)))
	mux.HandleFunc("/admin/files/", func(w http.ResponseWriter, r *http.Request) {
		cfg.Auth.requireAdmin(http.HandlerFunc(srv.AdminDeleteFileHandler)).ServeHTTP(w, r)
	})
	mux.Handle("/admin/cleanup", cfg.Auth.requireAdmin(http.HandlerFunc(srv.AdminManualCleanupHandler)))

	return srv
}

// Start begins serving HTTP on the configured address and starts background jobs.
// It blocks until the listener returns an error (or Shutdown is called).
func (s *Server) Start() error {
	// Start cleanup job in background
	cleanupCfg := GetCleanupConfigFromEnv(s.db, s.minio, s.bucket)
	defer s.cleanupCancel()
	go func() { defer close(s.cleanupDone); StartCleanupJob(s.cleanupCtx, cleanupCfg) }()
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		s.cleanupCancel()
		return err
	}
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the HTTP server and background jobs
// using the provided context (respecting the deadline/timeout supplied by the caller).
func (s *Server) Shutdown(ctx context.Context) error {
	s.cleanupCancel()
	// Allow short transfers to drain, then cancel active handlers and leave the
	// caller's remaining budget for upload compensation and background cleanup.
	grace, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := s.httpServer.Shutdown(grace)
	cancel()
	s.requestMu.Lock()
	s.closing = true
	s.requestMu.Unlock()
	s.requestCancel()
	if err != nil {
		_ = s.httpServer.Close()
	}
	done := make(chan struct{})
	go func() { s.requests.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-s.cleanupDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
