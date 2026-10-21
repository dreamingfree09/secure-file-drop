package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdministratorRequiresSignedRole(t *testing.T) {
	db := canonicalOwnedAuthDB(t)
	a := AuthConfig{DB: db, AdminUser: "canonical-admin", SessionSecret: "synthetic-secret"}
	for _, admin := range []bool{false, true} {
		sub := "canonical-admin"
		if !admin {
			sub = canonicalRegisteredUser(t, db)
		}
		tok, exp, err := a.makeSessionToken(sub, admin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO auth_sessions(token_hash,expires_at) VALUES($1,$2)", sessionHash(tok), exp); err != nil {
			t.Fatal("persist synthetic session")
		}
		r := httptest.NewRequest("GET", "/admin/files", nil)
		r.AddCookie(a.cookie(r, tok, exp))
		w := httptest.NewRecorder()
		a.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
		want := 403
		if admin {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("role HTTP %d want %d", w.Code, want)
		}
	}
}

func TestActualMetadataValidation(t *testing.T) {
	a := AuthConfig{AdminUser: "admin", SessionSecret: "synthetic-secret"}
	token, _, err := a.makeSessionToken("admin", true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Auth: a}
	t.Setenv("SFD_MAX_UPLOAD_BYTES", "10")
	for _, body := range []string{
		`{"orig_name":"","content_type":"text/plain","size_bytes":0}`,
		`{"orig_name":"file\r\nInjected","content_type":"text/plain","size_bytes":0}`,
		`{"orig_name":"file","content_type":"bad/type; invalid","size_bytes":0}`,
		`{"orig_name":"file","content_type":"text/plain","size_bytes":-1}`,
		`{"orig_name":"file","content_type":"text/plain","size_bytes":11}`,
	} {
		r := httptest.NewRequest("POST", "/files", strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: a.cookieName(), Value: token, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w := httptest.NewRecorder()
		r = r.WithContext(context.WithValue(r.Context(), identityKey{}, identity{Subject: "00000000-0000-0000-0000-000000000001", Username: "fixture"}))
		cfg.createFileHandler(nil).ServeHTTP(w, r)
		if w.Code != 400 && w.Code != 413 {
			t.Fatalf("invalid metadata reached database: %d", w.Code)
		}
	}
}
func TestFilenameSafety(t *testing.T) {
	for _, tt := range []struct {
		in, want string
		ok       bool
	}{
		{"../../fixture.txt", "fixture.txt", true}, {`C:\fakepath\fixture.txt`, "fixture.txt", true},
		{"quote\"name.txt", "quote\"name.txt", true}, {"..", "", false}, {"bad\nname", "", false}, {strings.Repeat("a", 256), "", false},
	} {
		got, ok := safeFilename(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("%q -> %q %v", tt.in, got, ok)
		}
	}
}
func TestRegistrationBcryptBoundsAndPasswordBytes(t *testing.T) {
	if ok, _ := validatePassword(strings.Repeat("a", 72) + "1"); ok {
		t.Fatal("accepted bcrypt-truncated password")
	}
	pass := " SpacesPreserved123 "
	hash, err := hashPassword(pass)
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(pass, hash) || verifyPassword(strings.TrimSpace(pass), hash) {
		t.Fatal("password bytes changed")
	}
}
func TestLoginIssuesDistinctRoleBoundCookies(t *testing.T) {
	a := AuthConfig{DB: canonicalOwnedAuthDB(t), AdminUser: "canonical-cookie-admin", AdminPass: canonicalPasswordHash(t, "SyntheticPass123"), SessionSecret: "synthetic-secret"}
	t.Setenv("SFD_PUBLIC_BASE_URL", "https://files.test")
	var previous string
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "https://files.test/login", bytes.NewBufferString(`{"username":"canonical-cookie-admin","password":"SyntheticPass123"}`))
		w := httptest.NewRecorder()
		a.loginHandler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
			t.Fatal("unsafe cookie")
		}
		p, err := a.verifyToken(cookies[0].Value)
		if err != nil || !p.Admin || p.Sub != "canonical-cookie-admin" || p.ID == "" {
			t.Fatal("missing signed identity")
		}
		if previous == cookies[0].Value {
			t.Fatal("sessions are not unique")
		}
		previous = cookies[0].Value
	}
}
func TestCrossOriginMutationRejected(t *testing.T) {
	t.Setenv("SFD_PUBLIC_BASE_URL", "https://files.test")
	handler := sameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for _, origin := range []string{"https://attacker.test", "null", "https://files.test", ""} {
		r := httptest.NewRequest("POST", "http://internal/files", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := 403
		if origin == "" || origin == "https://files.test" {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("%q got %d", origin, w.Code)
		}
	}
}
func TestCleanupRejectsNonPositiveDurations(t *testing.T) {
	t.Setenv("SFD_CLEANUP_INTERVAL", "0s")
	t.Setenv("SFD_CLEANUP_MAX_AGE", "-1h")
	t.Setenv("SFD_CLEANUP_MAX_AGE_HOURS", "-1")
	cfg := GetCleanupConfigFromEnv(nil, nil, "")
	if cfg.Interval <= 0 || cfg.MaxAge <= 0 {
		t.Fatal("unsafe cleanup duration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg.Enabled = false
	done := make(chan struct{})
	go func() { StartCleanupJob(ctx, cfg); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disabled cleanup did not return")
	}
}
