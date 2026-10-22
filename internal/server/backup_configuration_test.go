package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func backupReviewEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SFD_ADDR", "SFD_BASE_URL", "SFD_PUBLIC_BASE_URL", "SFD_LOG_FORMAT", "SFD_LOG_LEVEL", "SFD_ENV", "SFD_BACKUP_INTERVAL", "SFD_BACKUP_RETENTION_DAYS", "SFD_MAX_UPLOAD_BYTES", "SFD_MAX_REQUEST_BYTES", "SMTP_PORT", "SMTP_FROM"} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", "postgres://fixture:synthetic@127.0.0.1/sfd_owned?sslmode=disable")
	t.Setenv("SFD_ADMIN_PASS", canonicalPasswordHash(t, "SyntheticBackup123"))
	t.Setenv("SFD_SESSION_SECRET", strings.Repeat("a", 32))
	t.Setenv("SFD_DOWNLOAD_SECRET", strings.Repeat("b", 32))
}

func TestBackupConfiguredRetentionPreservesWindow(t *testing.T) {
	backupReviewEnvironment(t)
	root := t.TempDir()
	t.Setenv("SFD_BACKUP_DIR", root)
	t.Setenv("SFD_BACKUP_RETENTION_DAYS", "30")
	for name, age := range map[string]time.Duration{"sfd-backup-retained.sql": 10 * 24 * time.Hour, "sfd-backup-expired.sql": 40 * 24 * time.Hour, "unrelated.txt": 40 * 24 * time.Hour} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("owned synthetic backup"), 0600); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-age)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	manager := NewBackupManager(LoadBackupConfig(), nil, nil)
	if err := manager.cleanupOldBackups(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sfd-backup-retained.sql", "unrelated.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("retention removed an in-window or unrelated file: %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "sfd-backup-expired.sql")); !os.IsNotExist(err) {
		t.Fatal("expired owned backup was not removed")
	}
}

func TestBackupIntervalConfigurationBoundary(t *testing.T) {
	backupReviewEnvironment(t)
	for _, tc := range []struct {
		value string
		valid bool
		want  time.Duration
	}{
		{"", true, 24 * time.Hour}, {"24h", true, 24 * time.Hour}, {"1440m", true, 24 * time.Hour}, {"1h30m", true, 90 * time.Minute},
		{"0h", false, 24 * time.Hour}, {"-1h", false, 24 * time.Hour}, {"24", false, 24 * time.Hour}, {"invalid", false, 24 * time.Hour},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("SFD_BACKUP_INTERVAL", tc.value)
			err := ValidateAllConfiguration()
			if (err == nil) != tc.valid {
				t.Fatal("backup interval startup validation does not match the scheduler contract")
			}
			if got := LoadBackupConfig().Interval; got != tc.want {
				t.Fatal("backup interval was not loaded safely")
			}
		})
	}
}

func TestBackupRetentionConfigurationBoundary(t *testing.T) {
	backupReviewEnvironment(t)
	for _, tc := range []struct {
		value string
		valid bool
		want  int
	}{
		{"", true, 7}, {"1", true, 1}, {"7", true, 7}, {"30", true, 30}, {"106751", true, 106751},
		{"0", false, 7}, {"-1", false, 7}, {"invalid", false, 7}, {"106752", false, 7}, {"9223372036854775808", false, 7},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("SFD_BACKUP_RETENTION_DAYS", tc.value)
			err := ValidateAllConfiguration()
			if (err == nil) != tc.valid {
				t.Fatal("backup retention startup validation does not match the supported date range")
			}
			if got := LoadBackupConfig().RetentionDays; got != tc.want {
				t.Fatal("configured backup retention was silently changed")
			}
		})
	}
}
