package e2e

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// One stalled, tiny synthetic upload; no load generation or resource exhaustion.
func assessCancellation(ctx context.Context, t *testing.T, conn *sql.DB, base string, alice, admin *http.Cookie, id string) {
	t.Helper()
	if _, err := conn.ExecContext(ctx, "UPDATE files SET created_at=now()-interval '48 hours' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	transferCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	req, err := http.NewRequestWithContext(transferCtx, "POST", base+"/upload?id="+id, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(alice)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=bounded-assessment")
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		res, e := http.DefaultClient.Do(req)
		if e == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
	}()
	wrote := make(chan struct{})
	go func() {
		defer close(wrote)
		_, _ = io.Copy(writer, strings.NewReader("--bounded-assessment\r\nContent-Disposition: form-data; name=\"file\"; filename=\"x\"\r\n\r\nabc"))
	}()
	locked := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		tx, e := conn.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		var got string
		e = tx.QueryRowContext(ctx, "SELECT id::text FROM files WHERE id=$1 FOR UPDATE NOWAIT", id).Scan(&got)
		_ = tx.Rollback()
		if e != nil && strings.Contains(e.Error(), "55P03") {
			locked = true
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !locked {
		t.Fatal("upload did not acquire fixture lock")
	}
	deleteCtx, stop := context.WithTimeout(ctx, 250*time.Millisecond)
	del, err := http.NewRequestWithContext(deleteCtx, "DELETE", base+"/admin/files/"+id, nil)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	del.AddCookie(admin)
	res, err := http.DefaultClient.Do(del)
	stop()
	if err == nil {
		_ = res.Body.Close()
		t.Fatalf("delete bypassed in-flight lock: HTTP %d", res.StatusCode)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("unexpected delete-wait outcome")
	}
	cleanup, err := http.NewRequestWithContext(ctx, "POST", base+"/admin/cleanup", nil)
	if err != nil {
		t.Fatal(err)
	}
	cleanup.AddCookie(admin)
	res, err = http.DefaultClient.Do(cleanup)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("cleanup HTTP %d", res.StatusCode)
	}
	var count int
	if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE id=$1", id).Scan(&count); err != nil || count != 1 {
		t.Fatal("cleanup removed locked upload")
	}
	cancel()
	_ = writer.CloseWithError(context.Canceled)
	select {
	case <-completed:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled transfer did not finish")
	}
	select {
	case <-wrote:
	case <-time.After(time.Second):
		t.Fatal("fixture writer did not finish")
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err = conn.QueryRowContext(ctx, "SELECT status FROM files WHERE id=$1", id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "failed" {
			t.Log("PASS: upload/delete lock, cleanup SKIP LOCKED and client-canceled upload failed-state recovery")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("canceled upload did not persist failed state")
}
