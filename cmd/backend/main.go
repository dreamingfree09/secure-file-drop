package main

import (
	"context"
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

	// Basic safety: refuse to start if secrets are missing (prevents accidental public exposure).
	if auth.AdminPass == "" || auth.SessionSecret == "" {
		log.Printf("service=backend msg=%q", "missing SFD_ADMIN_PASS or SFD_SESSION_SECRET")
		os.Exit(1)
	}

	srv := server.New(server.Config{
		Addr:  addr,
		Build: build,
		Auth:  auth,
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

func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
