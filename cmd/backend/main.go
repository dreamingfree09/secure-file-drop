// cmd/backend/main.go - Production entrypoint for Secure File Drop.
//
// Wires configuration, runs migrations, starts the HTTP server, and
// performs graceful shutdown on signals. Prefers SFD_PUBLIC_BASE_URL
// for generating absolute links.
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"secure-file-drop/internal/db"
	"secure-file-drop/internal/server"
)

func main() {
	// Validate all configuration before proceeding
	log.Printf("service=backend msg=%q", "validating_configuration")
	if err := server.ValidateAllConfiguration(); err != nil {
		log.Printf("service=backend msg=%q err=%v", "configuration_validation_failed", err)
		os.Exit(1)
	}
	log.Printf("service=backend msg=%q", "configuration_valid")

	// Log warnings for optional missing configuration
	server.WarnOnOptionalMissingConfig()

	addr := getenvDefault("SFD_ADDR", ":8080")

	build := server.BuildInfo{
		Version: getenvDefault("SFD_VERSION", "dev"),
		Commit:  getenvDefault("SFD_COMMIT", "unknown"),
	}

	auth := server.AuthConfig{
		AdminUser:     getenvDefault("SFD_ADMIN_USER", "admin"),
		AdminPass:     getenvDefault("SFD_ADMIN_PASS", ""), // Should be bcrypt hash
		SessionSecret: getenvDefault("SFD_SESSION_SECRET", ""),
		SessionTTL:    12 * time.Hour,
		CookieName:    "sfd_session",
	}

	// Safety: refuse to start if secrets are missing.
	if auth.AdminPass == "" || auth.SessionSecret == "" {
		log.Printf("service=backend msg=%q", "missing SFD_ADMIN_PASS or SFD_SESSION_SECRET")
		os.Exit(1)
	}

	// Security: refuse to start with default/insecure secrets
	insecureSecrets := []string{
		"change-me",
		"changeme",
		"admin",
		"password",
		"secret",
		"default",
		"123456",
	}

	sessionSecretLower := strings.ToLower(auth.SessionSecret)
	for _, insecure := range insecureSecrets {
		if strings.Contains(sessionSecretLower, insecure) {
			log.Printf("service=backend msg=%q value=%q", "SECURITY ERROR: SFD_SESSION_SECRET contains insecure default value", insecure)
			os.Exit(1)
		}
	}

	if len(auth.SessionSecret) < 32 {
		log.Printf("service=backend msg=%q len=%d", "SECURITY ERROR: SFD_SESSION_SECRET too short (minimum 32 characters)", len(auth.SessionSecret))
		os.Exit(1)
	}

	// Validate admin password is a bcrypt hash (starts with $2a$, $2b$, or $2y$)
	if auth.AdminPass != "" && !strings.HasPrefix(auth.AdminPass, "$2a$") &&
		!strings.HasPrefix(auth.AdminPass, "$2b$") && !strings.HasPrefix(auth.AdminPass, "$2y$") {
		log.Printf("service=backend msg=%q", "SECURITY ERROR: SFD_ADMIN_PASS must be a bcrypt hash (use 'htpasswd -bnBC 12 \"\" password | tr -d ':'\" to generate)")
		log.Printf("service=backend msg=%q", "Example: htpasswd -bnBC 12 '' yourpassword | tr -d ':'")
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

	// Optimize database connection pooling for production
	dbConn.SetMaxOpenConns(25)                 // Maximum open connections
	dbConn.SetMaxIdleConns(5)                  // Maximum idle connections
	dbConn.SetConnMaxLifetime(5 * time.Minute) // Maximum connection lifetime
	dbConn.SetConnMaxIdleTime(2 * time.Minute) // Maximum idle time before closing

	log.Printf("service=backend msg=%q max_open=%d max_idle=%d max_lifetime=%s max_idle_time=%s",
		"db_pool_configured", 25, 5, "5m", "2m")

	// Run migrations
	log.Printf("service=backend msg=%q", "running_migrations")
	if err := db.RunMigrations(dbConn); err != nil {
		log.Printf("service=backend msg=%q err=%v", "migration_failed", err)
		os.Exit(1)
	}
	log.Printf("service=backend msg=%q", "migrations_complete")

	// Initialize email service
	emailCfg := server.LoadEmailConfig()
	emailSvc := server.NewEmailService(emailCfg)

	// Initialize account lockout (5 attempts, 15min lockout, 10min window)
	accountLockout := server.NewAccountLockout(5, 15*time.Minute, 10*time.Minute)

	// Add database and services to auth config
	auth.DB = dbConn
	auth.AccountLockout = accountLockout
	auth.EmailService = emailSvc

	// Get public base URL for links (prefer SFD_PUBLIC_BASE_URL, fallback to SFD_BASE_URL)
	baseURL := getenvDefault("SFD_PUBLIC_BASE_URL", getenvDefault("SFD_BASE_URL", "http://localhost:8080"))

	// Initialize automated database backup system
	backupCfg := server.LoadBackupConfig()
	backupMgr := server.NewBackupManager(backupCfg, dbConn, emailSvc)
	backupMgr.Start()
	defer backupMgr.Stop()

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
