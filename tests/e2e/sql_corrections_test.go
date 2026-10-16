package e2e

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Table locks make the affected SQL wait on this fresh database. Closing the
// HTTP request must cancel the server query and return the pool to service.
func verifySQLCancellation(ctx context.Context, t *testing.T, db *sql.DB, base, downloadURL string) {
	t.Helper()
	for _, tc := range []struct{ path, method, table, body string }{
		{base + "/register", "POST", "users", `{"email":"cancel@example.test","username":"cancel_fixture","password":"SyntheticCancel123"}`},
		{base + "/login", "POST", "users", `{"username":"alice","password":" SpacePass123 "}`},
		{downloadURL, "GET", "files", ""},
	} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		table := "users"
		if tc.table == "files" {
			table = "files"
		}
		if _, err = tx.ExecContext(ctx, "LOCK TABLE "+table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		requestCtx, cancel := context.WithTimeout(ctx, 350*time.Millisecond)
		req, err := http.NewRequestWithContext(requestCtx, tc.method, tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, requestErr := http.DefaultClient.Do(req)
		if res != nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
		cancel()
		if requestErr == nil {
			_ = tx.Rollback()
			t.Fatal("locked SQL did not block fixture request")
		}
		deadline := time.Now().Add(2 * time.Second)
		released := false
		for time.Now().Before(deadline) {
			var blocked int
			err = db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND pid<>pg_backend_pid()").Scan(&blocked)
			if err != nil {
				t.Fatal(err)
			}
			if blocked == 0 {
				released = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		_ = tx.Rollback()
		if !released {
			t.Fatal("request cancellation left SQL waiting on lock: " + tc.path)
		}
		if err = db.PingContext(ctx); err != nil {
			t.Fatal("pool unusable after cancellation", err)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "LOCK TABLE users IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/login", strings.NewReader(`{"username":"alice","password":" SpacePass123 "}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal("server SQL deadline failed", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 500 || time.Since(started) > 7*time.Second {
		t.Fatal("unbounded SQL wait", res.StatusCode, time.Since(started))
	}
	_ = tx.Rollback()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal("pool unusable after SQL timeout", err)
	}
	t.Log("PASS: real registration/login/download metadata cancellation releases locked PostgreSQL queries")
}
