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

// CleanupConfig controls retention and dependencies for background cleanup.
type CleanupConfig struct {
	Enabled     bool
	Interval    time.Duration
	MaxAge      time.Duration
	DB          *sql.DB
	MinioClient *minio.Client
	Bucket      string
}

// StartCleanupJob cleans old pending/failed files until cancellation.
func StartCleanupJob(ctx context.Context, cfg CleanupConfig) {
	if !cfg.Enabled {
		return
	}
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		if _, err := runCleanup(ctx, cfg); err != nil && ctx.Err() == nil {
			log.Printf("cleanup failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func runCleanup(ctx context.Context, cfg CleanupConfig) (int, error) {
	return cleanupFiles(ctx, cfg, true)
}

// Manual cleanup retains the legacy pending/failed-only policy; background
// cleanup also honors explicitly requested file expiration.
func cleanupFiles(ctx context.Context, cfg CleanupConfig, includeExpired bool) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	tx, err := cfg.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id,object_key FROM files WHERE (created_at<$1 AND status IN ('pending','failed')) OR ($2 AND auto_delete=TRUE AND expires_at IS NOT NULL AND expires_at<CURRENT_TIMESTAMP)
 ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED`, time.Now().Add(-cfg.MaxAge), includeExpired)
	if err != nil {
		return 0, err
	}
	type item struct{ id, key string }
	var items []item
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.key); err != nil {
			_ = rows.Close()
			return 0, err
		}
		items = append(items, v)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return 0, err
	}
	for _, v := range items {
		if err = cfg.MinioClient.RemoveObject(ctx, cfg.Bucket, v.key, minio.RemoveObjectOptions{}); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM files WHERE id=$1", v.id); err != nil {
			return 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM auth_sessions WHERE expires_at <= CURRENT_TIMESTAMP"); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(items), nil
}

// GetCleanupConfigFromEnv reads positive durations with conservative defaults.
func GetCleanupConfigFromEnv(db *sql.DB, mc *minio.Client, bucket string) CleanupConfig {
	enabled := os.Getenv("SFD_CLEANUP_ENABLED") == "true"
	interval := time.Hour
	age := 24 * time.Hour
	if d, e := time.ParseDuration(os.Getenv("SFD_CLEANUP_INTERVAL")); e == nil && d > 0 {
		interval = d
	}
	if d, e := time.ParseDuration(os.Getenv("SFD_CLEANUP_MAX_AGE")); e == nil && d > 0 {
		age = d
	}
	if h, e := strconv.Atoi(os.Getenv("SFD_CLEANUP_MAX_AGE_HOURS")); e == nil && h > 0 && h <= 876000 {
		age = time.Duration(h) * time.Hour
	}
	return CleanupConfig{enabled, interval, age, db, mc, bucket}
}
