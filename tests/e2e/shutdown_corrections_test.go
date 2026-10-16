package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Hold a genuine multipart request open during SIGTERM. The process must cancel
// it, finish failed-upload compensation, and exit before Docker's kill deadline.
func verifyActiveShutdown(ctx context.Context, t *testing.T, run string, appArgs []string,
	db *sql.DB, cookie *http.Cookie, create func(string, int, *http.Cookie) string,
	docker func(...string) string, own func(string, string), port func(string, string) string) {
	t.Helper()
	args := append([]string(nil), appArgs...)
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--name" {
			args[i+1] = run + "-active-shutdown"
		}
	}
	app := docker(args...)
	own("container", app)
	base := "http://" + port(app, "8080/tcp")
	client := &http.Client{Timeout: 25 * time.Second}
	ready := false
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); {
		res, err := client.Get(base + "/ready")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		t.Fatal("shutdown fixture readiness")
	}
	id := create("active-shutdown.bin", 3, cookie)
	var header bytes.Buffer
	mw := multipart.NewWriter(&header)
	if _, err := mw.CreateFormFile("file", "active-shutdown.bin"); err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, "POST", base+"/upload?id="+id, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(cookie)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		res, _ := client.Do(req)
		if res != nil {
			_ = res.Body.Close()
		}
	}()
	if _, err = writer.Write(append(header.Bytes(), []byte("abc")...)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	// Observe the real row lock before initiating shutdown.
	locked := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.ExecContext(ctx, "SELECT id FROM files WHERE id=$1 FOR UPDATE NOWAIT", id)
		_ = tx.Rollback()
		if e != nil && strings.Contains(e.Error(), "55P03") {
			locked = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !locked {
		t.Fatal("active upload did not acquire row lock")
	}
	started := time.Now()
	docker("stop", "--time", "25", app)
	if time.Since(started) > 20*time.Second {
		t.Fatal("active shutdown exceeded application budget")
	}
	_ = writer.Close()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("active HTTP handler did not terminate")
	}
	if docker("inspect", "--format", "{{.State.ExitCode}}", app) != "0" {
		t.Fatal("active shutdown did not exit cleanly")
	}
	var status string
	if err = db.QueryRowContext(ctx, "SELECT status FROM files WHERE id=$1", id).Scan(&status); err != nil || status != "failed" {
		t.Fatal("shutdown compensation", status, err)
	}
	t.Log("PASS: active multipart SIGTERM cancellation, bounded compensation and clean process exit")
}
