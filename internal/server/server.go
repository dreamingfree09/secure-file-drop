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
	httpServer *http.Server
	db         *sql.DB
	minio      *minio.Client
	bucket     string
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

	// Ready endpoint: dependencies are reachable (Postgres and MinIO).
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if cfg.DB == nil {
			http.Error(w, "db not configured", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
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

	// Login endpoint (POST JSON {username,password})
	mux.HandleFunc("/login", cfg.Auth.loginHandler())

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

	// Wrap middleware: requestID -> logging -> mux
	var handler http.Handler = mux
	handler = loggingMiddleware(handler)
	handler = requestIDMiddleware(handler)

	s := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Server{
		httpServer: s,
		db:         cfg.DB,
		minio:      mc,
		bucket:     bucket,
	}
}

// Start begins serving HTTP on the configured address. It blocks until
// the listener returns an error (or Shutdown is called).
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the HTTP server using the provided
// context (respecting the deadline/timeout supplied by the caller).
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
