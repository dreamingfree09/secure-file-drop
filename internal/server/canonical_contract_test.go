package server

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func canonicalOwnedAuthDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SFD_AUTH_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicitly owned PostgreSQL acceptance; not an authentication pass")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(u.Path, "/sfd_auth_") || os.Getenv("SFD_AUTH_TEST_OWNER") == "" {
		t.Fatal("refusing unowned database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("open owned database")
	}
	t.Cleanup(func() { _ = db.Close() })
	var marker string
	if db.QueryRow("SELECT owner FROM sfd_test_owner").Scan(&marker) != nil || marker != os.Getenv("SFD_AUTH_TEST_OWNER") {
		t.Fatal("fixture owner mismatch")
	}
	return db
}
func canonicalPasswordHash(t *testing.T, password string) string {
	t.Helper()
	b, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func canonicalRegisteredUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.NewString()
	_, e := db.Exec("INSERT INTO users(id,email,username,password_hash,email_verified,is_active,is_admin) VALUES($1,$2,$3,$4,TRUE,TRUE,FALSE)", id, id+"@example.invalid", id, canonicalPasswordHash(t, "SyntheticPass123"))
	if e != nil {
		t.Fatal("insert synthetic user")
	}
	return id
}

type canonicalDeadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *canonicalDeadlineWriter) SetReadDeadline(v time.Time) error { w.deadline = v; return nil }
func TestCanonicalMiddlewarePreservesReadDeadline(t *testing.T) {
	original := &canonicalDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	logging := &loggingResponseWriter{ResponseWriter: original}
	tracing := &responseWriter{ResponseWriter: logging}
	compressed := &compressionResponseWriter{ResponseWriter: tracing, writer: &bytes.Buffer{}}
	want := time.Now().Add(time.Second)
	if err := http.NewResponseController(compressed).SetReadDeadline(want); err != nil || !original.deadline.Equal(want) {
		t.Fatal("middleware lost bounded request deadline")
	}
}
