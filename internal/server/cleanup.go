package server

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
)

// CleanupConfig holds configuration for the cleanup job
type CleanupConfig struct {
	Enabled     bool
	Interval    time.Duration
	MaxAge      time.Duration
	DB          *sql.DB
	MinioClient *minio.Client
	Bucket      string
}

// StartCleanupJob starts a background goroutine that periodically cleans up expired files
func StartCleanupJob(ctx context.Context, cfg CleanupConfig) {
	if !cfg.Enabled {
		log.Printf("service=cleanup msg=%q", "disabled")
		return
	}

	log.Printf("service=cleanup msg=%q interval=%s max_age=%s",
		"starting", cfg.Interval, cfg.MaxAge)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// Run immediately on start
	runCleanup(ctx, cfg)

	for {
		select {
		case <-ctx.Done():
			log.Printf("service=cleanup msg=%q", "shutting_down")
			return
		case <-ticker.C:
			runCleanup(ctx, cfg)
		}
	}
}

func runCleanup(ctx context.Context, cfg CleanupConfig) {
	start := time.Now()
	log.Printf("service=cleanup msg=%q", "starting_cleanup_run")

	cutoff := time.Now().Add(-cfg.MaxAge)

	// Find expired files (created more than MaxAge ago, not in 'ready' state)
	rows, err := cfg.DB.QueryContext(ctx, `
		SELECT id, object_key, status, created_at
		FROM files
		WHERE created_at < $1
		  AND status IN ('pending', 'failed')
		ORDER BY created_at ASC
		LIMIT 100
	`, cutoff)
	if err != nil {
		log.Printf("service=cleanup msg=%q err=%v", "query_failed", err)
		return
	}
	defer rows.Close()

	deleted := 0
	for rows.Next() {
		var (
			id        string
			objectKey string
			status    string
			createdAt time.Time
		)

		if err := rows.Scan(&id, &objectKey, &status, &createdAt); err != nil {
			log.Printf("service=cleanup msg=%q err=%v", "scan_failed", err)
			continue
		}

		age := time.Since(createdAt)
		log.Printf("service=cleanup msg=%q id=%s status=%s age=%s",
			"deleting_expired_file", id, status, age)

		// Delete from MinIO (if exists)
		if err := cfg.MinioClient.RemoveObject(ctx, cfg.Bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
			log.Printf("service=cleanup msg=%q id=%s err=%v", "minio_delete_failed", id, err)
			// Continue anyway - record might be orphaned
		}

		// Delete from database
		if _, err := cfg.DB.ExecContext(ctx, `DELETE FROM files WHERE id = $1`, id); err != nil {
			log.Printf("service=cleanup msg=%q id=%s err=%v", "db_delete_failed", id, err)
			continue
		}

		deleted++
	}

	duration := time.Since(start)
	log.Printf("service=cleanup msg=%q deleted=%d duration_ms=%d",
		"cleanup_complete", deleted, duration.Milliseconds())
}

// GetCleanupConfigFromEnv reads cleanup configuration from environment variables
func GetCleanupConfigFromEnv(db *sql.DB, mc *minio.Client, bucket string) CleanupConfig {
	enabled := os.Getenv("SFD_CLEANUP_ENABLED") == "true"

	interval := 1 * time.Hour
	if v := os.Getenv("SFD_CLEANUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	maxAge := 24 * time.Hour
	if v := os.Getenv("SFD_CLEANUP_MAX_AGE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			maxAge = d
		}
	}

	// Also support hours as a fallback
	if v := os.Getenv("SFD_CLEANUP_MAX_AGE_HOURS"); v != "" {
		if hours, err := strconv.Atoi(v); err == nil {
			maxAge = time.Duration(hours) * time.Hour
		}
	}

	return CleanupConfig{
		Enabled:     enabled,
		Interval:    interval,
		MaxAge:      maxAge,
		DB:          db,
		MinioClient: mc,
		Bucket:      bucket,
	}
}
