package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

type BuildInfo struct {
	Version string
	Commit  string
}

type Config struct {
	Addr  string // e.g. ":8080"
	Build BuildInfo
	Auth  AuthConfig
}

type Server struct {
	httpServer *http.Server
}

func New(cfg Config) *Server {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
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

	// Protected endpoint for verification only (will be useful for testing middleware)
	mux.Handle("/me", cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})))

	// Wrap middleware: requestID -> logging -> mux
	var handler http.Handler = mux
	handler = loggingMiddleware(handler)
	handler = requestIDMiddleware(handler)

	s := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Server{httpServer: s}
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
