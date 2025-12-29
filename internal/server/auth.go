// auth.go - Stateless session cookies and authentication helpers.
//
// Implements HMAC-signed cookie sessions, login/logout handlers,
// and DB-backed user authentication compatible with the MVP.
package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
)

// AuthConfig holds authentication-related configuration used by HTTP handlers
// (admin credentials, session secrets, cookie settings, and DB for user auth).
//
// It is intentionally lightweight for the MVP yet production-ready.
// Unit tests can construct this directly. Database-backed user auth
// is supported when DB is non-nil.
type AuthConfig struct {
	AdminUser     string
	AdminPass     string
	SessionSecret string
	SessionTTL    time.Duration
	CookieName    string
	DB            *sql.DB // Database connection for user authentication
}

type sessionPayload struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
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

func (a AuthConfig) secretBytes() []byte {
	return []byte(a.SessionSecret)
}

func signPayload(secret []byte, msg string) string {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

func encodeSession(p sessionPayload) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeSession(token string) (sessionPayload, error) {
	var p sessionPayload
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	return p, nil
}

// makeToken returns "payload.signature"
func (a AuthConfig) makeToken(sub string) (string, time.Time, error) {
	exp := time.Now().Add(a.ttl())
	p := sessionPayload{Sub: sub, Exp: exp.Unix()}
	payload, err := encodeSession(p)
	if err != nil {
		return "", time.Time{}, err
	}
	sig := signPayload(a.secretBytes(), payload)
	return payload + "." + sig, exp, nil
}

func (a AuthConfig) verifyToken(tok string) (sessionPayload, error) {
	var p sessionPayload
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return p, errors.New("invalid token format")
	}
	payload := parts[0]
	sig := parts[1]
	want := signPayload(a.secretBytes(), payload)
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return p, errors.New("invalid signature")
	}
	decoded, err := decodeSession(payload)
	if err != nil {
		return p, err
	}
	if decoded.Exp <= time.Now().Unix() {
		return p, errors.New("expired")
	}
	return decoded, nil
}

// loginHandler handles both database-backed user login and legacy admin login.
// On success, it issues a signed session cookie (HttpOnly, SameSite=Lax, Secure).
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var authenticated bool
		var userID string

		// First, try database authentication if DB is available
		if a.DB != nil {
			userID, authenticated = authenticateUser(a.DB, body.Username, body.Password)
		}

		// Fallback to legacy admin authentication if DB auth failed or no DB
		if !authenticated && a.AdminUser != "" && a.AdminPass != "" {
			uOK := body.Username == a.AdminUser
			pwHash := sha256.Sum256([]byte(body.Password))
			adminHash := sha256.Sum256([]byte(a.AdminPass))
			pOK := hmac.Equal(pwHash[:], adminHash[:])

			if uOK && pOK {
				authenticated = true
				userID = a.AdminUser
			}
		}

		if !authenticated {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		tok, exp, err := a.makeToken(userID)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     a.cookieName(),
			Value:    tok,
			Path:     "/",
			Expires:  exp,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			// Secure flag enabled for HTTPS (cloudflared tunnel provides TLS)
			Secure: true,
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	}
}

// logoutHandler clears the session cookie by setting an expired cookie
func (a AuthConfig) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     a.cookieName(),
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}
}

func (a AuthConfig) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(a.cookieName())
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := a.verifyToken(c.Value); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// getCurrentUser extracts the current user ID (subject) from the session cookie.
func (a AuthConfig) getCurrentUser(r *http.Request) (string, error) {
	c, err := r.Cookie(a.cookieName())
	if err != nil {
		return "", errors.New("no session cookie")
	}
	payload, err := a.verifyToken(c.Value)
	if err != nil {
		return "", err
	}
	return payload.Sub, nil
}

// requireAdmin is middleware that checks if the authenticated user has admin role.
func (a AuthConfig) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First check authentication
		c, err := r.Cookie(a.cookieName())
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		payload, err := a.verifyToken(c.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if user is admin
		if a.DB == nil {
			http.Error(w, "server misconfigured", http.StatusInternalServerError)
			return
		}

		var isAdmin bool
		err = a.DB.QueryRow("SELECT is_admin FROM users WHERE id = $1 AND is_active = TRUE", payload.Sub).Scan(&isAdmin)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			log.Printf("requireAdmin: db query failed: %v", err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if !isAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
