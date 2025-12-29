// download_tokens.go - HMAC-signed download token helpers.
//
// Encodes file ID and expiry into URL-safe tokens and verifies them
// server-side for authorization.
package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"time"
)

var (
	errDownloadSecretMissing = errors.New("SFD_DOWNLOAD_SECRET missing")
	errBadToken              = errors.New("bad token")
	errTokenExpired          = errors.New("token expired")
)

type downloadClaims struct {
	FileID string `json:"file_id"`
	Exp    int64  `json:"exp"` // unix seconds
}

// downloadSecret returns the raw secret bytes from env.
func downloadSecret() ([]byte, error) {
	sec := os.Getenv("SFD_DOWNLOAD_SECRET")
	if sec == "" {
		return nil, errDownloadSecretMissing
	}
	return []byte(sec), nil
}

// signDownloadToken creates a compact token: base64url(payload).base64url(sig)
// where sig = HMAC-SHA256(secret, payloadBytes).
func signDownloadToken(fileID string, expiresAt time.Time) (string, error) {
	sec, err := downloadSecret()
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(downloadClaims{
		FileID: fileID,
		Exp:    expiresAt.Unix(),
	})
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, sec)
	_, _ = mac.Write(payload)
	sig := mac.Sum(nil)

	enc := base64.RawURLEncoding
	return enc.EncodeToString(payload) + "." + enc.EncodeToString(sig), nil
}

// verifyDownloadToken validates signature + expiry and returns claims.
func verifyDownloadToken(token string, now time.Time) (downloadClaims, error) {
	var c downloadClaims

	sec, err := downloadSecret()
	if err != nil {
		return c, err
	}

	// token format: payload.sig
	dot := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 || dot >= len(token)-1 {
		return c, errBadToken
	}

	enc := base64.RawURLEncoding
	payloadB, err := enc.DecodeString(token[:dot])
	if err != nil {
		return c, errBadToken
	}
	sigB, err := enc.DecodeString(token[dot+1:])
	if err != nil {
		return c, errBadToken
	}

	mac := hmac.New(sha256.New, sec)
	_, _ = mac.Write(payloadB)
	want := mac.Sum(nil)

	if !hmac.Equal(sigB, want) {
		return c, errBadToken
	}

	if err := json.Unmarshal(payloadB, &c); err != nil {
		return c, errBadToken
	}

	if c.FileID == "" || c.Exp == 0 {
		return c, errBadToken
	}

	if now.Unix() > c.Exp {
		return c, errTokenExpired
	}

	return c, nil
}
