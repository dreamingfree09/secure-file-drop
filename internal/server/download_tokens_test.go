package server

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyDownloadToken(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", "testsecret")

	now := time.Now()
	exp := now.Add(1 * time.Hour)
	tok, err := signDownloadToken("file-123", exp)
	if err != nil {
		t.Fatalf("signDownloadToken error: %v", err)
	}

	claims, err := verifyDownloadToken(tok, now)
	if err != nil {
		t.Fatalf("verifyDownloadToken error: %v", err)
	}
	if claims.FileID != "file-123" {
		t.Fatalf("unexpected FileID: got %q want %q", claims.FileID, "file-123")
	}
	if claims.Exp != exp.Unix() {
		t.Fatalf("unexpected Exp: got %d want %d", claims.Exp, exp.Unix())
	}
}

func TestVerifyExpiredDownloadToken(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", "testsecret")

	now := time.Now()
	exp := now.Add(-1 * time.Hour)
	tok, err := signDownloadToken("file-456", exp)
	if err != nil {
		t.Fatalf("signDownloadToken error: %v", err)
	}

	_, err = verifyDownloadToken(tok, now)
	if err == nil {
		t.Fatalf("expected error for expired token, got nil")
	}
	if err != errTokenExpired {
		t.Fatalf("unexpected error: got %v want %v", err, errTokenExpired)
	}
}

func TestVerifyTamperedSignature(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", "testsecret")

	now := time.Now()
	exp := now.Add(1 * time.Hour)
	tok, err := signDownloadToken("file-789", exp)
	if err != nil {
		t.Fatalf("signDownloadToken error: %v", err)
	}

	// Split token into payload and sig, decode sig, flip a bit, re-encode.
	dot := strings.IndexByte(tok, '.')
	if dot < 0 {
		t.Fatalf("token format unexpected: %q", tok)
	}
	payload := tok[:dot]
	sigEnc := tok[dot+1:]
	sig, err := base64.RawURLEncoding.DecodeString(sigEnc)
	if err != nil {
		t.Fatalf("decode sig error: %v", err)
	}
	// flip a bit to corrupt signature
	sig[0] ^= 0x01
	sigBad := base64.RawURLEncoding.EncodeToString(sig)
	badTok := payload + "." + sigBad

	_, err = verifyDownloadToken(badTok, now)
	if err == nil {
		t.Fatalf("expected error for tampered signature, got nil")
	}
	if err != errBadToken {
		t.Fatalf("unexpected error: got %v want %v", err, errBadToken)
	}
}

func TestDownloadSecretMissing(t *testing.T) {
	// Ensure env is not set
	t.Setenv("SFD_DOWNLOAD_SECRET", "")

	_, err := signDownloadToken("file-000", time.Now().Add(1*time.Hour))
	if err == nil {
		t.Fatalf("expected error when secret missing for signDownloadToken, got nil")
	}
	if err != errDownloadSecretMissing {
		t.Fatalf("unexpected error: got %v want %v", err, errDownloadSecretMissing)
	}

	// verify should also fail early due to missing secret
	_, err = verifyDownloadToken("invalid.token", time.Now())
	if err == nil {
		t.Fatalf("expected error when secret missing for verifyDownloadToken, got nil")
	}
	if err != errDownloadSecretMissing {
		t.Fatalf("unexpected error: got %v want %v", err, errDownloadSecretMissing)
	}
}

func TestVerifyMalformedToken(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", "testsecret")

	// missing dot
	_, err := verifyDownloadToken("badtoken", time.Now())
	if err == nil {
		t.Fatalf("expected error for malformed token, got nil")
	}
	if err != errBadToken {
		t.Fatalf("unexpected error for malformed token: got %v want %v", err, errBadToken)
	}

	// invalid base64 payload
	_, err = verifyDownloadToken("!!."+base64.RawURLEncoding.EncodeToString([]byte("sig")), time.Now())
	if err == nil {
		t.Fatalf("expected error for invalid base64 payload, got nil")
	}
	if err != errBadToken {
		t.Fatalf("unexpected error for invalid base64 payload: got %v want %v", err, errBadToken)
	}
}
