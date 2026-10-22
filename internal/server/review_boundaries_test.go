package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReviewDownloadSecretStartup(t *testing.T) {
	for _, key := range []string{"SFD_ADDR", "SFD_BASE_URL", "SFD_PUBLIC_BASE_URL", "SFD_LOG_FORMAT", "SFD_LOG_LEVEL", "SFD_ENV", "SFD_BACKUP_INTERVAL", "SFD_BACKUP_RETENTION_DAYS", "SFD_MAX_UPLOAD_BYTES", "SFD_MAX_REQUEST_BYTES"} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", "postgres://fixture:synthetic@127.0.0.1/sfd_owned?sslmode=disable")
	t.Setenv("SFD_ADMIN_PASS", canonicalPasswordHash(t, "SyntheticPass123"))
	t.Setenv("SFD_SESSION_SECRET", strings.Repeat("a", 32))
	for _, size := range []int{0, 1, 31, 32} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			t.Setenv("SFD_DOWNLOAD_SECRET", strings.Repeat("b", size))
			err := ValidateAllConfiguration()
			if (err == nil) != (size >= 32) {
				t.Fatal("download secret startup strength not enforced")
			}
		})
	}
}
func TestReviewRemotePeerKey(t *testing.T) {
	for _, peer := range []string{"192.0.2.10:4321", "[2001:db8::1]:4321"} {
		r := httptest.NewRequest("POST", "/login", nil)
		r.RemoteAddr = peer
		r.Header.Set("X-Forwarded-For", "203.0.113.1, 203.0.113.2")
		r.Header.Set("X-Real-IP", "203.0.113.3")
		want, _, _ := net.SplitHostPort(peer)
		if getClientIP(r) != want || getClientIPForLogging(r) != want {
			t.Fatal("untrusted header changed peer key")
		}
	}
}
func TestReviewRegistrationNeedsDelivery(t *testing.T) {
	defer func() {
		if recover() != nil {
			t.Fatal("disabled delivery reached database mutation")
		}
	}()
	for _, svc := range []*EmailService{nil, NewEmailService(EmailConfig{})} {
		cfg := Config{EmailSvc: svc}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/register", strings.NewReader(`{"email":"fixture@example.invalid","username":"fixture","password":"SyntheticPass123"}`))
		cfg.RegisterHandler(w, r)
		if w.Code != 503 {
			t.Fatal("disabled delivery must reject registration before data access")
		}
	}
}
func TestReviewResetEmailUnavailable(t *testing.T) {
	for method, want := range map[string]int{"POST": 501, "GET": 405} {
		w := httptest.NewRecorder()
		passwordResetUnavailable(w, httptest.NewRequest(method, "/reset-password-request", nil))
		if w.Code != want {
			t.Fatal("unaccepted reset email route did not fail closed")
		}
	}
}
func TestReviewRegistrationDeliveryFailureRollsBack(t *testing.T) {
	db := canonicalOwnedAuthDB(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("owned SMTP fault listener")
	}
	defer func() { _ = listener.Close() }()
	go func() {
		c, e := listener.Accept()
		if e == nil {
			_ = c.SetDeadline(time.Now().Add(time.Second))
			_, _ = fmt.Fprint(c, "421 fixture unavailable\r\n")
			_ = c.Close()
		}
	}()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	name := "mailfailure" + strings.ReplaceAll(uuid.NewString(), "-", "")
	svc := NewEmailService(EmailConfig{Enabled: true, SMTPHost: "localhost", SMTPPort: port, SMTPUser: "fixture", SMTPPassword: "synthetic", FromEmail: "fixture@example.invalid"})
	cfg := Config{DB: db, EmailSvc: svc, BaseURL: "https://fixture.invalid"}
	body := fmt.Sprintf(`{"email":"%s@example.invalid","username":"%s","password":"SyntheticPass123"}`, name, name)
	w := httptest.NewRecorder()
	cfg.RegisterHandler(w, httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body)))
	if w.Code != 503 {
		t.Fatal("failed verification delivery reported account creation")
	}
	var count int
	if db.QueryRow("SELECT count(*) FROM users WHERE username=$1", name).Scan(&count) != nil || count != 0 {
		t.Fatal("failed verification delivery left an unusable account")
	}
}
