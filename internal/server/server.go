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

type BuildInfo struct {
	Version string
	Commit  string
}

type Config struct {
	Addr  string // e.g. ":8080"
	Build BuildInfo
	Auth  AuthConfig
	DB    *sql.DB
}

type Server struct {
	httpServer *http.Server
	db         *sql.DB
	minio      *minio.Client
	bucket     string
}

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
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})

	// Ready endpoint: dependencies are reachable (initially only Postgres).
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if cfg.DB == nil {
			http.Error(w, "db not configured", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		if err := cfg.DB.PingContext(ctx); err != nil {
			http.Error(w, "db not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})

	// Version endpoint (no secrets)
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
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
	mux.Handle("/me", cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	return s.httpServer.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
