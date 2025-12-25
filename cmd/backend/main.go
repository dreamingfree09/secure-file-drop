package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"secure-file-drop/internal/server"
)

func main() {
	addr := getenvDefault("SFD_ADDR", ":8080")

	build := server.BuildInfo{
		Version: getenvDefault("SFD_VERSION", "dev"),
		Commit:  getenvDefault("SFD_COMMIT", "unknown"),
	}

	auth := server.AuthConfig{
		AdminUser:     getenvDefault("SFD_ADMIN_USER", "admin"),
		AdminPass:     getenvDefault("SFD_ADMIN_PASS", ""),
		SessionSecret: getenvDefault("SFD_SESSION_SECRET", ""),
		SessionTTL:    12 * time.Hour,
		CookieName:    "sfd_session",
	}

	// Safety: refuse to start if secrets are missing.
	if auth.AdminPass == "" || auth.SessionSecret == "" {
		log.Printf("service=backend msg=%q", "missing SFD_ADMIN_PASS or SFD_SESSION_SECRET")
		os.Exit(1)
	}

	// Database
	dsn := getenvDefault("DATABASE_URL", "")
	db, err := server.OpenDB(dsn)
	if err != nil {
		log.Printf("service=backend msg=%q err=%v", "db_connect_failed", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	srv := server.New(server.Config{
		Addr:  addr,
		Build: build,
		Auth:  auth,
		DB:    db,
	})

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		log.Printf("service=backend msg=%q addr=%s version=%s commit=%s",
			"starting", addr, build.Version, build.Commit)
		errCh <- srv.Start()
	}()

	// Wait for signal or server error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("service=backend msg=%q signal=%s", "shutting_down", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("service=backend msg=%q err=%v", "shutdown_error", err)
			os.Exit(1)
		}
		log.Printf("service=backend msg=%q", "shutdown_complete")
	case err := <-errCh:
		if err != nil {
			log.Printf("service=backend msg=%q err=%v", "server_error", err)
			os.Exit(1)
		}
	}
}

// NOTE: kept here for clarity and minimal dependencies.
func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

// Compile-time check to ensure we keep using *sql.DB in the server config.
var _ *sql.DB
