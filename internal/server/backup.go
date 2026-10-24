// backup.go - Automated database backup mechanism for Secure File Drop.
//
// Provides scheduled PostgreSQL backups with retention policies, compression,
// and optional cloud storage upload. Supports full and incremental backups.

package server

import (
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BackupConfig contains configuration for database backup operations.
type BackupConfig struct {
	Enabled         bool          // Enable automated backups
	Interval        time.Duration // Backup interval (e.g., 24h for daily)
	RetentionDays   int           // Number of days to retain backups
	BackupDir       string        // Directory to store backup files
	Compression     bool          // Enable gzip compression
	DatabaseURL     string        // PostgreSQL connection string
	UploadToS3      bool          // Upload backups to S3/MinIO
	S3Bucket        string        // S3 bucket for backup storage
	S3Prefix        string        // S3 prefix/folder for backups
	NotifyOnFailure bool          // Send notification on backup failure
	NotifyOnSuccess bool          // Send notification on successful backup
}

// BackupManager handles scheduled database backups.
type BackupManager struct {
	config       BackupConfig
	db           *sql.DB
	emailService *EmailService
	stopChan     chan struct{}
}

// NewBackupManager creates a new backup manager instance.
func NewBackupManager(config BackupConfig, db *sql.DB, emailSvc *EmailService) *BackupManager {
	return &BackupManager{
		config:       config,
		db:           db,
		emailService: emailSvc,
		stopChan:     make(chan struct{}),
	}
}

// Start begins the automated backup scheduler.
func (bm *BackupManager) Start() {
	if !bm.config.Enabled {
		Info("database backups disabled", nil)
		return
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(bm.config.BackupDir, 0750); err != nil {
		Error("failed to create backup directory", map[string]any{
			"error": err.Error(),
			"dir":   bm.config.BackupDir,
		}, err)
		return
	}

	Info("database backup scheduler started", map[string]any{
		"interval":       bm.config.Interval.String(),
		"retention_days": bm.config.RetentionDays,
		"backup_dir":     bm.config.BackupDir,
		"compression":    bm.config.Compression,
	})

	// Run initial backup
	go func() {
		if err := bm.performBackup(); err != nil {
			Error("initial backup failed", map[string]any{"error": err.Error()}, err)
		}
	}()

	// Schedule periodic backups
	ticker := time.NewTicker(bm.config.Interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				if err := bm.performBackup(); err != nil {
					Error("scheduled backup failed", map[string]any{"error": err.Error()}, err)
				}
			case <-bm.stopChan:
				ticker.Stop()
				Info("backup scheduler stopped", nil)
				return
			}
		}
	}()
}

// Stop halts the backup scheduler.
func (bm *BackupManager) Stop() {
	close(bm.stopChan)
}

// performBackup executes a database backup operation.
func (bm *BackupManager) performBackup() error {
	startTime := time.Now()

	Info("starting database backup", nil)

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("sfd-backup-%s.sql", timestamp)
	if bm.config.Compression {
		filename += ".gz"
	}
	backupPath := filepath.Join(bm.config.BackupDir, filename)

	// Execute pg_dump
	if err := bm.dumpDatabase(backupPath); err != nil {
		if bm.config.NotifyOnFailure {
			bm.sendBackupNotification(false, err)
		}
		return fmt.Errorf("backup failed: %w", err)
	}

	// Get backup file size
	fileInfo, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("failed to stat backup file: %w", err)
	}

	duration := time.Since(startTime)

	Info("database backup completed", map[string]any{
		"filename":    filename,
		"size_bytes":  fileInfo.Size(),
		"duration_ms": duration.Milliseconds(),
	})

	// Cloud upload is still unsupported; never report a successful remote backup.
	if bm.config.UploadToS3 {
		Warn("backup S3 upload not implemented; local backup only", nil)
	}

	// Clean up old backups
	if err := bm.cleanupOldBackups(); err != nil {
		Warn("failed to cleanup old backups", map[string]any{"error": err.Error()})
	}

	if bm.config.NotifyOnSuccess {
		bm.sendBackupNotification(true, nil)
	}

	return nil
}

// dumpDatabase executes pg_dump to create a backup file.
func (bm *BackupManager) dumpDatabase(outputPath string) error {
	if filepath.Dir(outputPath) != filepath.Clean(bm.config.BackupDir) || bm.config.DatabaseURL == "" {
		return errors.New("invalid backup configuration")
	}
	connection, err := backupConnectionEnvironment(bm.config.DatabaseURL)
	if err != nil {
		return err
	}
	// Keep connection credentials out of process arguments and diagnostic output.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=plain", "--no-owner", "--no-acl", "--no-password")
	cmd.Env = append(os.Environ(), connection...)
	cmd.Stderr = io.Discard
	return writeBackup(outputPath, bm.config.Compression, func(dst io.Writer) error {
		cmd.Stdout = dst
		if err := cmd.Run(); err != nil {
			return errors.New("pg_dump failed; backup not published")
		}
		return nil
	})
}

// backupConnectionEnvironment supports PostgreSQL URIs and explicit libpq
// environment options. Unknown/duplicate options fail closed rather than silently
// dropping TLS requirements. Error text never contains connection values.
func backupConnectionEnvironment(dsn string) ([]string, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.Hostname() == "" || u.Path == "" {
		return nil, errors.New("invalid backup connection URI")
	}
	password, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	values := map[string]string{"PGHOST": u.Hostname(), "PGPORT": port, "PGUSER": u.User.Username(), "PGPASSWORD": password, "PGDATABASE": strings.TrimPrefix(u.Path, "/"), "PGCONNECT_TIMEOUT": "10"}
	options := map[string]string{"sslmode": "PGSSLMODE", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY", "sslrootcert": "PGSSLROOTCERT", "sslcrl": "PGSSLCRL", "sslcrldir": "PGSSLCRLDIR", "connect_timeout": "PGCONNECT_TIMEOUT", "application_name": "PGAPPNAME", "options": "PGOPTIONS", "target_session_attrs": "PGTARGETSESSIONATTRS", "channel_binding": "PGCHANNELBINDING", "gssencmode": "PGGSSENCMODE", "ssl_min_protocol_version": "PGSSLMINPROTOCOLVERSION", "ssl_max_protocol_version": "PGSSLMAXPROTOCOLVERSION"}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, errors.New("invalid backup connection options")
	}
	for key, v := range query {
		env, ok := options[key]
		if !ok || len(v) != 1 {
			return nil, errors.New("unsupported or duplicate backup connection option")
		}
		values[env] = v[0]
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result, nil
}

// writeBackup publishes only a closed, synced successful stream. Hard-link publication
// is exclusive: existing backups and symlink targets are never overwritten.
func writeBackup(outputPath string, compressed bool, produce func(io.Writer) error) (result error) {
	file, err := os.CreateTemp(filepath.Dir(outputPath), ".sfd-backup-partial-*")
	if err != nil {
		return fmt.Errorf("create private backup: %w", err)
	}
	defer func() {
		if err := os.Remove(file.Name()); err != nil {
			result = errors.Join(result, fmt.Errorf("remove owned partial backup: %w", err))
		}
	}()
	var output io.Writer = file
	var gz *gzip.Writer
	if compressed {
		gz = gzip.NewWriter(file)
		output = gz
	}
	result = produce(output)
	if gz != nil {
		result = errors.Join(result, gz.Close())
	}
	result = errors.Join(result, file.Sync(), file.Close())
	if result != nil {
		return result
	}
	return os.Link(file.Name(), outputPath)
}

// cleanupOldBackups removes backup files older than retention period.
func (bm *BackupManager) cleanupOldBackups() error {
	cutoffTime := time.Now().AddDate(0, 0, -bm.config.RetentionDays)

	files, err := os.ReadDir(bm.config.BackupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Only process backup files
		if !strings.HasPrefix(file.Name(), "sfd-backup-") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			Warn("failed to get file info", map[string]any{
				"file":  file.Name(),
				"error": err.Error(),
			})
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(bm.config.BackupDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				Warn("failed to remove old backup", map[string]any{
					"file":  file.Name(),
					"error": err.Error(),
				})
			} else {
				Info("removed old backup", map[string]any{
					"file": file.Name(),
					"age":  time.Since(info.ModTime()).String(),
				})
			}
		}
	}

	return nil
}

// sendBackupNotification sends an email notification about backup status.
func (bm *BackupManager) sendBackupNotification(success bool, _ error) {
	if bm.emailService == nil {
		return
	}

	var subject string
	if success {
		subject = "Database Backup Successful"
	} else {
		subject = "Database Backup Failed"
	}

	// Would send to admin email - placeholder implementation
	Info("backup notification", map[string]any{
		"success": success,
		"subject": subject,
	})
}

// ListBackups returns a list of available backup files sorted by date (newest first).
func (bm *BackupManager) ListBackups() ([]BackupInfo, error) {
	files, err := os.ReadDir(bm.config.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, file := range files {
		if file.IsDir() || !strings.HasPrefix(file.Name(), "sfd-backup-") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Filename:  file.Name(),
			Size:      info.Size(),
			Timestamp: info.ModTime(),
		})
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// BackupInfo contains metadata about a backup file.
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Size      int64     `json:"size_bytes"`
	Timestamp time.Time `json:"timestamp"`
}

// Bound calendar retention to the positive range representable as a Go duration.
// Startup validation and the scheduler use the same configuration parsers.
const maxBackupRetentionDays = int((1<<63 - 1) / int64(24*time.Hour))

func parseBackupInterval(value string) (time.Duration, error) {
	if value == "" {
		return 24 * time.Hour, nil
	}
	interval, err := time.ParseDuration(value)
	if err != nil || interval <= 0 {
		return 0, errors.New("must be a positive duration (e.g., 24h, 1440m)")
	}
	return interval, nil
}

func parseBackupRetentionDays(value string) (int, error) {
	if value == "" {
		return 7, nil
	}
	days, err := strconv.Atoi(value)
	if err != nil || days <= 0 || days > maxBackupRetentionDays {
		return 0, errors.New("must be a positive whole number of days no greater than 106751")
	}
	return days, nil
}

// LoadBackupConfig loads backup configuration from environment variables.
func LoadBackupConfig() BackupConfig {
	enabled := getenvDefault("SFD_BACKUP_ENABLED", "false") == "true"

	interval, err := parseBackupInterval(os.Getenv("SFD_BACKUP_INTERVAL"))
	if err != nil {
		interval = 24 * time.Hour // Startup validation rejects invalid configured values.
	}
	retentionDays, err := parseBackupRetentionDays(os.Getenv("SFD_BACKUP_RETENTION_DAYS"))
	if err != nil {
		retentionDays = 7
	}

	return BackupConfig{
		Enabled:         enabled,
		Interval:        interval,
		RetentionDays:   retentionDays,
		BackupDir:       getenvDefault("SFD_BACKUP_DIR", "/var/backups/sfd"),
		Compression:     getenvDefault("SFD_BACKUP_COMPRESSION", "true") == "true",
		DatabaseURL:     getenvDefault("DATABASE_URL", ""),
		UploadToS3:      getenvDefault("SFD_BACKUP_S3_ENABLED", "false") == "true",
		S3Bucket:        getenvDefault("SFD_BACKUP_S3_BUCKET", ""),
		S3Prefix:        getenvDefault("SFD_BACKUP_S3_PREFIX", "backups"),
		NotifyOnFailure: getenvDefault("SFD_BACKUP_NOTIFY_FAILURE", "true") == "true",
		NotifyOnSuccess: getenvDefault("SFD_BACKUP_NOTIFY_SUCCESS", "false") == "true",
	}
}

func getenvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
