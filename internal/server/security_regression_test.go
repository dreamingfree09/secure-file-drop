package server

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecuritySessionRejectsMissingSecret(t *testing.T) {
	a := AuthConfig{}
	payload, _ := encodeSession(sessionPayload{Sub: "admin", Exp: time.Now().Add(time.Hour).Unix()})
	if _, err := a.verifyToken(payload + "." + signPayload(nil, payload)); err == nil {
		t.Fatal("accepted session signed with empty secret")
	}
}
func TestSecuritySessionRejectsEmptySubject(t *testing.T) {
	a := AuthConfig{SessionSecret: "synthetic-test-secret"}
	payload, _ := encodeSession(sessionPayload{Exp: time.Now().Add(time.Hour).Unix()})
	if _, err := a.verifyToken(payload + "." + signPayload(a.secretBytes(), payload)); err == nil {
		t.Fatal("accepted empty subject")
	}
}
func TestSecurityDownloadExpiryBoundary(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", "synthetic-test-secret")
	expiry := time.Unix(2000000000, 0)
	tok, err := signDownloadToken("file-boundary", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = verifyDownloadToken(tok, expiry); err != errTokenExpired {
		t.Fatalf("at expiration: got %v", err)
	}
}
func TestSecurityUploadConfigurationBounds(t *testing.T) {
	for _, v := range []string{"-1", "0", "9223372036854775807", "invalid"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("SFD_MAX_UPLOAD_BYTES", v)
			if _, err := maxUploadBytes(); err == nil {
				t.Fatal("accepted unsafe upload bound")
			}
		})
	}
	t.Setenv("SFD_MAX_UPLOAD_BYTES", "")
	if n, err := maxUploadBytes(); err != nil || n <= 0 {
		t.Fatalf("missing finite default: %d %v", n, err)
	}
}
func TestSecurityOriginIgnoresUntrustedForwardedHeaders(t *testing.T) {
	r := httptest.NewRequest("POST", "http://files.test/links", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "attacker.test")
	if got := requestOrigin(r); got != "http://files.test" {
		t.Fatalf("untrusted origin: %s", got)
	}
}
func TestSecurityActualDownloadValidation(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", "synthetic-test-secret")
	cfg := Config{}
	for _, tt := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/download?token=x", 405}, {"GET", "/download", 400}, {"GET", "/download?token=invalid.token", 401},
	} {
		w := httptest.NewRecorder()
		cfg.downloadHandler(nil, nil, "").ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != tt.want {
			t.Fatalf("%s got %d want %d", tt.path, w.Code, tt.want)
		}
	}
}
func TestSecuritySessionTampering(t *testing.T) {
	a := AuthConfig{SessionSecret: "synthetic-test-secret"}
	tok, _, err := a.makeToken("user")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := encodeSession(sessionPayload{Sub: "admin", Exp: time.Now().Add(time.Hour).Unix()})
	// Replacing the payload must not preserve authentication.
	parts := []byte(tok)
	dot := 0
	for i, b := range parts {
		if b == '.' {
			dot = i
			break
		}
	}
	tampered := payload + string(parts[dot:])
	if _, err := a.verifyToken(tampered); err == nil {
		t.Fatal("tampered subject accepted")
	}
	if _, err := base64.RawURLEncoding.DecodeString(payload); err != nil {
		t.Fatal(err)
	}
}
