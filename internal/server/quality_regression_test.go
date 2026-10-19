package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackupPublication(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "gzip"}[compressed], func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "backup.sql")
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := root.Close(); err != nil {
					t.Error(err)
				}
			}()
			produce := func(w io.Writer) error { _, err := io.WriteString(w, "synthetic backup data\n"); return err }
			if err := writeBackup(target, compressed, produce); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(target)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("backup permissions")
			}
			data, err := root.ReadFile("backup.sql")
			if err != nil {
				t.Fatal(err)
			}
			original := append([]byte(nil), data...)
			if compressed {
				reader, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				data, err = io.ReadAll(reader)
				if err != nil {
					t.Fatal("gzip checksum/trailer")
				}
				if err := reader.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if string(data) != "synthetic backup data\n" {
				t.Fatal("backup contents")
			}
			if err := writeBackup(target, compressed, produce); err == nil {
				t.Fatal("overwrote an existing backup")
			}
			again, err := root.ReadFile("backup.sql")
			if err != nil || !bytes.Equal(again, original) {
				t.Fatal("existing backup changed")
			}
			failed := filepath.Join(dir, "failed.sql")
			if err := writeBackup(failed, compressed, func(io.Writer) error { return errors.New("synthetic write failure") }); err == nil {
				t.Fatal("failed backup accepted")
			}
			if _, err := os.Stat(failed); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("partial backup published")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatal("owned partial cleanup")
			}
		})
	}
	t.Run("symlink target preserved", func(t *testing.T) {
		dir := t.TempDir()
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := root.Close(); err != nil {
				t.Error(err)
			}
		}()
		target := filepath.Join(dir, "original")
		link := filepath.Join(dir, "backup.sql")
		if err := os.WriteFile(target, []byte("preserved"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := writeBackup(link, false, func(w io.Writer) error { _, err := io.WriteString(w, "replacement"); return err }); err == nil {
			t.Fatal("symlink replaced")
		}
		data, err := root.ReadFile("original")
		if err != nil || string(data) != "preserved" {
			t.Fatal("symlink target changed")
		}
	})
}

func TestRequestLogEncoding(t *testing.T) {
	for _, structured := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[structured], func(t *testing.T) {
			var output bytes.Buffer
			saved := DefaultLogger
			DefaultLogger = &Logger{output: &output, minLevel: LogLevelInfo, enableJSON: structured}
			t.Cleanup(func() { DefaultLogger = saved })
			request := httptest.NewRequest("GET", "https://localhost/me?credential=synthetic-do-not-log", nil)
			request.Header.Set("Referer", "https://user:synthetic-do-not-log@localhost/?signed=synthetic-do-not-log")
			request.Header.Set("User-Agent", "test\nforged-entry")
			request.Header.Set("X-Forwarded-For", "127.0.0.1\r\nforged-entry")
			loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(httptest.NewRecorder(), request)
			line := output.String()
			if strings.Count(line, "\n") != 1 || strings.Contains(line, "\r") || strings.Contains(line, "synthetic-do-not-log") {
				t.Fatal("log injection or credential disclosure")
			}
			if structured {
				var entry LogEntry
				if err := json.Unmarshal(output.Bytes(), &entry); err != nil || entry.Fields["status"] != float64(204) {
					t.Fatal("structured request outcome")
				}
			}
		})
	}
}

type failingLogWriter struct{}

func (failingLogWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestConcurrentLogger(t *testing.T) {
	var output bytes.Buffer
	logger := &Logger{output: &output, minLevel: LogLevelInfo, enableJSON: true}
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() { defer group.Done(); logger.Info("test", nil) }()
	}
	group.Wait()
	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 16 {
		t.Fatal("log entries interleaved")
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatal("invalid concurrent log entry")
		}
	}
	// Failure branches must not panic or disclose field contents through fallback logs.
	(&Logger{output: failingLogWriter{}, minLevel: LogLevelInfo}).Info("test", nil)
	logger.Info("nonserializable", map[string]interface{}{"value": make(chan int)})
}

func TestWebhookCancellation(t *testing.T) {
	var calls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer endpoint.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Server{}).sendWebhook(ctx, WebhookConfig{URL: endpoint.URL, RetryCount: 3}, WebhookPayload{Event: WebhookEventFileUploaded})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled delivery did not stop")
	}
	if calls.Load() != 0 {
		t.Fatal("canceled webhook delivered")
	}
}

func TestHashToolConfigurationRejected(t *testing.T) {
	t.Setenv("SFD_HASH_TOOL", "relative-helper")
	if _, err := runHashTool(context.Background(), "unused"); err == nil {
		t.Fatal("relative executable accepted")
	}
	file := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(file, []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SFD_HASH_TOOL", file)
	if _, err := runHashTool(context.Background(), "unused"); err == nil {
		t.Fatal("non-executable accepted")
	}
	// #nosec G302 -- Deliberately writable owned fixture; asserts rejection before execution.
	if err := os.Chmod(file, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := runHashTool(context.Background(), "unused"); err == nil {
		t.Fatal("writable executable accepted")
	}
}

func TestBackupConnectionParameters(t *testing.T) {
	values, err := backupConnectionEnvironment("postgresql://fixture:encoded%40password@[::1]:5433/synthetic?sslmode=verify-full&sslrootcert=%2Ffixture%2Fca.crt")
	if err != nil {
		t.Fatal("valid backup URI rejected")
	}
	env := map[string]string{}
	for _, entry := range values {
		k, v, _ := strings.Cut(entry, "=")
		env[k] = v
	}
	if env["PGHOST"] != "::1" || env["PGPORT"] != "5433" || env["PGDATABASE"] != "synthetic" || env["PGPASSWORD"] != "encoded@password" || env["PGSSLMODE"] != "verify-full" || env["PGSSLROOTCERT"] != "/fixture/ca.crt" {
		t.Fatal("connection parameters changed")
	}
	for _, invalid := range []string{"invalid", "postgres://fixture:password@localhost/db?sslmode=require&sslmode=disable", "postgres://fixture:password@localhost/db?unknown=value"} {
		if _, err := backupConnectionEnvironment(invalid); err == nil {
			t.Fatal("invalid or unsupported connection accepted")
		}
	}
}

// TestMain also provides a bounded, self-contained pg_dump stand-in. It validates
// command/environment separation without executing a shell or external helper.
func TestMain(m *testing.M) {
	if os.Getenv("SFD_OWNED_BACKUP_CHILD") == "1" && filepath.Base(os.Args[0]) == "pg_dump" {
		if len(os.Args) != 5 || strings.Join(os.Args[1:], " ") != "--format=plain --no-owner --no-acl --no-password" || os.Getenv("PGUSER") != "fixture" || os.Getenv("PGPASSWORD") != "synthetic@password" || os.Getenv("PGDATABASE") != "synthetic" || os.Getenv("PGSSLMODE") != "require" {
			os.Exit(2)
		}
		if os.Getenv("SFD_OWNED_BACKUP_FAIL") == "1" {
			os.Exit(3)
		}
		if _, err := io.WriteString(os.Stdout, "-- synthetic pg_dump stream\n"); err != nil {
			os.Exit(4)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestBackupCommandBoundary(t *testing.T) {
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, filepath.Join(dir, "pg_dump")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SFD_OWNED_BACKUP_CHILD", "1")
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			if fail {
				t.Setenv("SFD_OWNED_BACKUP_FAIL", "1")
			}
			connection := url.URL{Scheme: "postgres", User: url.UserPassword("fixture", "synthetic@password"), Host: "localhost", Path: "/synthetic", RawQuery: "sslmode=require"}
			manager := &BackupManager{config: BackupConfig{BackupDir: dir, DatabaseURL: connection.String(), Compression: true}}
			target := filepath.Join(dir, "output.sql.gz")
			err := manager.dumpDatabase(target)
			if fail {
				if err == nil {
					t.Fatal("failed command accepted")
				}
				if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed command published")
				}
				return
			}
			if err != nil {
				t.Fatal("command boundary rejected")
			}
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
		})
	}
}
