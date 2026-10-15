package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

// AuthConfig defines credentials, session signing and session persistence.
type AuthConfig struct {
	AdminUser     string
	AdminPass     string
	SessionSecret string
	SessionTTL    time.Duration
	CookieName    string
	DB            *sql.DB
}

type sessionPayload struct {
	Sub   string `json:"sub"`
	Exp   int64  `json:"exp"`
	Admin bool   `json:"admin,omitempty"`
	ID    string `json:"id,omitempty"`
}
type principalKey struct{}

func principal(r *http.Request) sessionPayload {
	p, _ := r.Context().Value(principalKey{}).(sessionPayload)
	return p
}
func (a AuthConfig) cookieName() string {
	if a.CookieName == "" {
		return "sfd_session"
	}
	return a.CookieName
}
func (a AuthConfig) ttl() time.Duration {
	if a.SessionTTL <= 0 {
		return 12 * time.Hour
	}
	return a.SessionTTL
}
func (a AuthConfig) secretBytes() []byte { return []byte(a.SessionSecret) }
func signPayload(secret []byte, msg string) string {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}
func encodeSession(p sessionPayload) (string, error) {
	b, err := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(b), err
}
func decodeSession(token string) (sessionPayload, error) {
	var p sessionPayload
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(b, &p)
	return p, err
}
func (a AuthConfig) makeToken(sub string) (string, time.Time, error) {
	return a.makeSessionToken(sub, false)
}
func (a AuthConfig) makeSessionToken(sub string, admin bool) (string, time.Time, error) {
	if a.SessionSecret == "" || sub == "" {
		return "", time.Time{}, errors.New("invalid session configuration")
	}
	exp := time.Now().Add(a.ttl())
	payload, err := encodeSession(sessionPayload{Sub: sub, Exp: exp.Unix(), Admin: admin, ID: generateRequestID()})
	if err != nil {
		return "", time.Time{}, err
	}
	return payload + "." + signPayload(a.secretBytes(), payload), exp, nil
}
func (a AuthConfig) verifyToken(tok string) (sessionPayload, error) {
	var p sessionPayload
	if a.SessionSecret == "" {
		return p, errors.New("session secret missing")
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return p, errors.New("invalid token format")
	}
	if !hmac.Equal([]byte(parts[1]), []byte(signPayload(a.secretBytes(), parts[0]))) {
		return p, errors.New("invalid signature")
	}
	p, err := decodeSession(parts[0])
	if err != nil {
		return p, err
	}
	if p.Sub == "" || p.Exp <= time.Now().Unix() {
		return p, errors.New("invalid or expired session")
	}
	return p, nil
}
func sessionHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func (a AuthConfig) cookie(r *http.Request, value string, expiry time.Time) *http.Cookie {
	// #nosec G124 -- Secure follows trusted HTTPS config/TLS; HTTP is supported for loopback development.
	return &http.Cookie{Name: a.cookieName(), Value: value, Path: "/", Expires: expiry, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || strings.HasPrefix(os.Getenv("SFD_PUBLIC_BASE_URL"), "https://")}
}
func (a AuthConfig) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var authenticated, admin bool
		var userID string
		if a.DB != nil {
			userID, authenticated = authenticateUser(a.DB, body.Username, body.Password)
		}
		if !authenticated && a.AdminUser != "" && a.AdminPass != "" {
			pw := sha256.Sum256([]byte(body.Password))
			want := sha256.Sum256([]byte(a.AdminPass))
			if body.Username == a.AdminUser && hmac.Equal(pw[:], want[:]) {
				authenticated = true
				admin = true
				userID = a.AdminUser
			}
		}
		if !authenticated {
			GetMetrics().RecordLoginAttempt(false)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tok, exp, err := a.makeSessionToken(userID, admin)
		if err == nil && a.DB != nil {
			_, err = a.DB.ExecContext(r.Context(), "INSERT INTO sessions (token_hash, expires_at) VALUES ($1,$2)", sessionHash(tok), exp)
		}
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		GetMetrics().RecordLoginAttempt(true)
		http.SetCookie(w, a.cookie(r, tok, exp))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "is_admin": admin})
	}
}
func (a AuthConfig) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(a.cookieName())
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		p, err := a.verifyToken(c.Value)
		if err != nil || (p.Admin && p.Sub != a.AdminUser) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if a.DB != nil {
			var valid bool
			err = a.DB.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND expires_at > CURRENT_TIMESTAMP)", sessionHash(c.Value)).Scan(&valid)
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if !valid {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !p.Admin {
				err = a.DB.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE id::text=$1 AND is_active=TRUE)", p.Sub).Scan(&valid)
				if err != nil {
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}
				if !valid {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}
func (a AuthConfig) requireAdmin(next http.Handler) http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !principal(r).Admin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
func (a AuthConfig) logoutHandler() http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c, _ := r.Cookie(a.cookieName())
		if a.DB != nil {
			if _, err := a.DB.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", sessionHash(c.Value)); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
		}
		// #nosec G124 -- Same secure attributes as login, with empty value and immediate expiry.
		cookie := a.cookie(r, "", time.Unix(1, 0))
		cookie.MaxAge = -1
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	}))
}
