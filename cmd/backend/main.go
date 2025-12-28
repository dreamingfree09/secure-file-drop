package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"secure-file-drop/internal/db"
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
	dbConn, err := server.OpenDB(dsn)
	if err != nil {
		log.Printf("service=backend msg=%q err=%v", "db_connect_failed", err)
		os.Exit(1)
	}
	defer func() { _ = dbConn.Close() }()

	// Run migrations
	log.Printf("service=backend msg=%q", "running_migrations")
	if err := db.RunMigrations(dbConn); err != nil {
		log.Printf("service=backend msg=%q err=%v", "migration_failed", err)
		os.Exit(1)
	}
	log.Printf("service=backend msg=%q", "migrations_complete")

	// Add database to auth config for user authentication
	auth.DB = dbConn

	// Load email configuration
	emailCfg := server.LoadEmailConfig()
	emailSvc := server.NewEmailService(emailCfg)

	// Get base URL for email links
	baseURL := getenvDefault("SFD_BASE_URL", "http://localhost:8080")

	srv := server.New(server.Config{
		Addr:     addr,
		Build:    build,
		Auth:     auth,
		DB:       dbConn,
		EmailSvc: emailSvc,
		BaseURL:  baseURL,
	})

	// Start the HTTP server in a background goroutine.
	// This allows us to listen for OS signals while the server runs.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("service=backend msg=%q addr=%s version=%s commit=%s",
			"starting", addr, build.Version, build.Commit)
		errCh <- srv.Start()
	}()

	// Set up signal handling for graceful shutdown on SIGINT (Ctrl+C) or SIGTERM (container stop).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Block until either a shutdown signal is received or the server encounters an error.
	select {
	case sig := <-sigCh:
		// Signal received: initiate graceful shutdown.
		log.Printf("service=backend msg=%q signal=%s", "shutting_down", sig.String())
		// Give the server 5 seconds to finish in-flight requests and cleanup.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("service=backend msg=%q err=%v", "shutdown_error", err)
			os.Exit(1)
		}
		log.Printf("service=backend msg=%q", "shutdown_complete")
	case err := <-errCh:
		// Server error: exit immediately.
		if err != nil {
			log.Printf("service=backend msg=%q err=%v", "server_error", err)
			os.Exit(1)
		}
	}
}

// getenvDefault reads an environment variable and returns a default value if not set.
// This helper avoids importing extra packages and keeps main.go self-contained.
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
