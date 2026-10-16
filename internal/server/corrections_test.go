package server

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJSONBoundariesAcrossRoutes(t *testing.T) {
	a := AuthConfig{AdminUser: "admin", AdminPass: "SyntheticPass123", SessionSecret: "synthetic-session-secret"}
	cfg := Config{Auth: a}
	token, _, err := a.makeToken("admin")
	if err != nil {
		t.Fatal(err)
	}
	for name, handler := range map[string]http.Handler{
		"login": a.loginHandler(), "register": http.HandlerFunc(cfg.RegisterHandler),
		"files": cfg.createFileHandler(nil), "links": cfg.createLinkHandler(nil),
	} {
		for _, suffix := range []string{"{}", "!", strings.Repeat(" ", 17<<10)} {
			t.Run(name+"/"+string(suffix[0]), func(t *testing.T) {
				r := httptest.NewRequest("POST", "http://fixture.test/"+name, strings.NewReader("{}"+suffix))
				r.AddCookie(a.cookie(r, token, time.Now().Add(time.Hour)))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				want := 400
				if len(suffix) > 16<<10 {
					want = 413
				}
				if w.Code != want {
					t.Fatalf("HTTP %d want %d", w.Code, want)
				}
			})
		}
	}
}
func TestJSONTotalLimitIncludesWhitespace(t *testing.T) {
	a := AuthConfig{AdminUser: "admin", AdminPass: "SyntheticPass123", SessionSecret: "synthetic-session-secret"}
	valid := `{"username":"admin","password":"SyntheticPass123"}`
	for _, n := range []int{(16 << 10) - len(valid), (16 << 10) - len(valid) + 1} {
		w := httptest.NewRecorder()
		a.loginHandler().ServeHTTP(w, httptest.NewRequest("POST", "/login", strings.NewReader(valid+strings.Repeat(" ", n))))
		want := 200
		if len(valid)+n > 16<<10 {
			want = 413
		}
		if w.Code != want {
			t.Fatalf("size %d HTTP %d", len(valid)+n, w.Code)
		}
	}
}
func TestMultipartReadLatchesUnexpectedEOF(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	p, err := mw.CreateFormFile("file", "empty.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	// Deliberately omit the closing boundary: ordinary malformed upload regression.
	mr := multipart.NewReader(bytes.NewReader(body.Bytes()), mw.Boundary())
	part, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	r := &multipartRead{Reader: part}
	_, _ = io.ReadAll(r)
	if !errors.Is(r.err, io.ErrUnexpectedEOF) {
		t.Fatalf("lost framing error: %v", r.err)
	}
}
func TestPublicOriginCanonicalValidation(t *testing.T) {
	for _, raw := range []string{"https://fixture.test/path", "https://user:pass@fixture.test", "https://fixture.test?query=1", "https://fixture.test#fragment", "ftp://fixture.test"} {
		t.Setenv("SFD_PUBLIC_BASE_URL", raw)
		if _, err := publicOrigin(); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	t.Setenv("SFD_PUBLIC_BASE_URL", "  https://localhost:8443/  ")
	origin, err := publicOrigin()
	if err != nil || origin != "https://localhost:8443" {
		t.Fatal(origin, err)
	}
	cookie := (AuthConfig{}).cookie(httptest.NewRequest("GET", "http://localhost", nil), "", time.Now())
	if !cookie.Secure {
		t.Fatal("canonical HTTPS origin lost Secure cookie")
	}
}
