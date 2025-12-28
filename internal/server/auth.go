package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// AuthConfig holds authentication-related configuration used by the
// HTTP handlers (admin credentials, session secrets and cookie settings).
//
// It is intentionally lightweight for the MVP and used by unit tests.
// Now also supports database-backed user authentication.
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

// loginHandler handles both legacy admin login and database user login
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
			// Secure will be true once HTTPS is enforced at proxy; for local dev it can remain false.
			Secure: false,
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
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

// getCurrentUser extracts the current user ID from the session cookie
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
