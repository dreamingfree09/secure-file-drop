package server

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestMakeAndVerifyToken(t *testing.T) {
	cfg := AuthConfig{SessionSecret: "test-secret", SessionTTL: 1 * time.Hour}
	tok, exp, err := cfg.makeToken("admin")
	if err != nil {
		t.Fatalf("makeToken error: %v", err)
	}
	if exp.Before(time.Now()) {
		t.Fatalf("expected exp in the future")
	}

	p, err := cfg.verifyToken(tok)
	if err != nil {
		t.Fatalf("verifyToken error: %v", err)
	}
	if p.Sub != "admin" {
		t.Fatalf("unexpected sub: %s", p.Sub)
	}
}

func TestVerifyTokenExpired(t *testing.T) {
	// craft an expired token manually
	secret := []byte("s")
	exp := time.Now().Add(-1 * time.Hour).Unix()
	sp := sessionPayload{Sub: "admin", Exp: exp}
	b, _ := json.Marshal(sp)
	payload := base64.RawURLEncoding.EncodeToString(b)
	sig := signPayload(secret, payload)
	tok := payload + "." + sig

	cfg := AuthConfig{SessionSecret: string(secret)}
	if _, err := cfg.verifyToken(tok); err == nil {
		t.Fatalf("expected error for expired token")
	}
}
