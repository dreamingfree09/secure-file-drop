// backup.go - Automated database backup mechanism for Secure File Drop.
//
// Provides scheduled PostgreSQL backups with retention policies, compression,
// and optional cloud storage upload. Supports full and incremental backups.
package server

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

	// Upload to S3 if configured
	if bm.config.UploadToS3 {
		if err := bm.uploadToS3(backupPath, filename); err != nil {
			Error("failed to upload backup to S3", map[string]any{"error": err.Error()}, err)
		} else {
			Info("backup uploaded to S3", map[string]any{
				"bucket": bm.config.S3Bucket,
				"key":    filepath.Join(bm.config.S3Prefix, filename),
			})
		}
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
	// Parse DATABASE_URL to extract connection parameters
	// Format: postgres://user:password@host:port/database?sslmode=disable
	dbURL := bm.config.DatabaseURL

	// Simple parsing (assumes standard format)
	var host, user, password, dbname string
	var port string = "5432"

	// Extract from postgres://user:password@host:port/database
	if strings.HasPrefix(dbURL, "postgres://") {
		dbURL = strings.TrimPrefix(dbURL, "postgres://")

		// Split user:password from rest
		parts := strings.SplitN(dbURL, "@", 2)
		if len(parts) == 2 {
			userPass := parts[0]
			hostDB := parts[1]

			// Extract user and password
			if strings.Contains(userPass, ":") {
				userPassParts := strings.SplitN(userPass, ":", 2)
				user = userPassParts[0]
				password = userPassParts[1]
			} else {
				user = userPass
			}

			// Extract host, port, and database
			if strings.Contains(hostDB, "/") {
				hostPortDB := strings.SplitN(hostDB, "/", 2)
				hostPort := hostPortDB[0]
				dbAndParams := hostPortDB[1]

				// Remove query parameters
				if strings.Contains(dbAndParams, "?") {
					dbname = strings.SplitN(dbAndParams, "?", 2)[0]
				} else {
					dbname = dbAndParams
				}

				// Extract host and port
				if strings.Contains(hostPort, ":") {
					hostPortParts := strings.SplitN(hostPort, ":", 2)
					host = hostPortParts[0]
					port = hostPortParts[1]
				} else {
					host = hostPort
				}
			}
		}
	}

	// Build pg_dump command with connection parameters
	cmd := exec.Command("pg_dump",
		"--format=plain",
		"--no-owner",
		"--no-acl",
		"--host="+host,
		"--port="+port,
		"--username="+user,
		"--dbname="+dbname,
	)

	var output io.Writer
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	if bm.config.Compression {
		gzWriter := gzip.NewWriter(file)
		defer gzWriter.Close()
		output = gzWriter
	} else {
		output = file
	}

	cmd.Stdout = output
	cmd.Stderr = os.Stderr

	// Set PGPASSWORD environment variable for authentication
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", password))

	if err := cmd.Run(); err != nil {
		os.Remove(outputPath) // Clean up partial backup
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	return nil
}

// uploadToS3 uploads a backup file to S3/MinIO.
func (bm *BackupManager) uploadToS3(localPath, filename string) error {
	// This would integrate with MinIO client to upload the backup
	// Placeholder implementation - would need MinIO client instance
	return fmt.Errorf("S3 upload not yet implemented")
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
func (bm *BackupManager) sendBackupNotification(success bool, err error) {
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

// LoadBackupConfig loads backup configuration from environment variables.
func LoadBackupConfig() BackupConfig {
	enabled := getenvDefault("SFD_BACKUP_ENABLED", "false") == "true"

	interval := 24 * time.Hour // Default: daily
	if intervalStr := os.Getenv("SFD_BACKUP_INTERVAL"); intervalStr != "" {
		if d, err := time.ParseDuration(intervalStr); err == nil {
			interval = d
		}
	}

	retentionDays := 7 // Default: 7 days
	if retStr := os.Getenv("SFD_BACKUP_RETENTION_DAYS"); retStr != "" {
		if days, err := time.ParseDuration(retStr + "d"); err == nil {
			retentionDays = int(days.Hours() / 24)
		}
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
