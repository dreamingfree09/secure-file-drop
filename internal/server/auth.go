// auth.go - Signed identities and database-backed session revocation.
//
// Implements HMAC-signed cookie sessions, login/logout handlers,
// and DB-backed user authentication compatible with the MVP.

package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AuthConfig holds authentication-related configuration used by HTTP handlers
// (admin credentials, session secrets, cookie settings, and DB for user auth).
//
// Environment administrators remain separate from registered database users.
// Unit tests can construct this directly. Database-backed user auth
// is supported when DB is non-nil.
type AuthConfig struct {
	AdminUser      string
	AdminPass      string
	SessionSecret  string
	SessionTTL     time.Duration
	CookieName     string
	DB             *sql.DB         // Database connection for user authentication
	AccountLockout *AccountLockout // Account lockout manager
	EmailService   *EmailService   // Email service for notifications
}

type sessionPayload struct {
	Sub   string `json:"sub"`
	Exp   int64  `json:"exp"`
	Admin bool   `json:"admin,omitempty"`
	ID    string `json:"sid"`
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
	return a.makeSessionToken(sub, false)
}

func (a AuthConfig) makeSessionToken(sub string, admin bool) (string, time.Time, error) {
	if sub == "" || a.SessionSecret == "" {
		return "", time.Time{}, errors.New("missing identity or secret")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().Add(a.ttl())
	p := sessionPayload{Sub: sub, Exp: exp.Unix(), Admin: admin, ID: hex.EncodeToString(nonce[:])}
	payload, err := encodeSession(p)
	if err != nil {
		return "", time.Time{}, err
	}
	sig := signPayload(a.secretBytes(), payload)
	return payload + "." + sig, exp, nil
}

func (a AuthConfig) verifyToken(tok string) (sessionPayload, error) {
	var p sessionPayload
	if len(tok) > 4096 {
		return p, errors.New("invalid token size")
	}
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
	if a.SessionSecret == "" || decoded.Sub == "" || len(decoded.ID) != 64 {
		return p, errors.New("invalid identity")
	}
	if decoded.Admin && (a.AdminUser == "" || decoded.Sub != a.AdminUser) {
		return p, errors.New("invalid administrator")
	}
	if decoded.Exp <= time.Now().Unix() {
		return p, errors.New("expired")
	}
	return decoded, nil
}

// loginHandler handles both database-backed user login and legacy admin login.
// On success, it issues a signed session cookie (HttpOnly, SameSite=Lax, Secure).
func (a AuthConfig) cookie(_ *http.Request, value string, expiry time.Time) *http.Cookie {
	return &http.Cookie{Name: a.cookieName(), Value: value, Path: "/", Expires: expiry, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: true}
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
		if !decodeJSON(w, r, &body) {
			return
		}

		// Check if account is locked
		if a.AccountLockout != nil {
			locked, lockedUntil, attemptsRemaining := a.AccountLockout.IsLocked(body.Username)
			if locked {
				Info("login_attempt_locked", map[string]interface{}{
					"username":     body.Username,
					"ip":           getClientIP(r),
					"locked_until": lockedUntil,
				})
				http.Error(w, fmt.Sprintf("Account locked until %s", lockedUntil.Format(time.RFC3339)), http.StatusTooManyRequests)
				return
			}
			// Log remaining attempts if low
			if attemptsRemaining <= 2 && attemptsRemaining > 0 {
				Info("login_attempts_warning", map[string]interface{}{
					"username":           body.Username,
					"attempts_remaining": attemptsRemaining,
				})
			}
		}

		var authenticated bool
		var environmentAdmin bool
		var userID string

		// First, try database authentication if DB is available
		if a.DB != nil {
			var authErr error
			userID, authenticated, authErr = authenticateUser(r.Context(), a.DB, body.Username, body.Password)
			if authErr != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
		}

		// Fallback to admin authentication if DB auth failed or no DB
		// AdminPass should now be a bcrypt hash (not plaintext)
		if !authenticated && a.AdminUser != "" && a.AdminPass != "" {
			if body.Username == a.AdminUser {
				// AdminPass is expected to be a bcrypt hash
				err := bcrypt.CompareHashAndPassword([]byte(a.AdminPass), []byte(body.Password))
				if err == nil {
					authenticated = true
					userID = a.AdminUser
					environmentAdmin = true
				}
			}
		}

		if !authenticated {
			if a.AccountLockout != nil {
				a.AccountLockout.RecordFailedAttempt(body.Username)
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		tok, exp, err := a.makeSessionToken(userID, environmentAdmin)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if a.DB == nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Reclaim a bounded batch without waiting on another session's locks.
		// Only expired records are removed; current sessions remain isolated.
		if _, err := a.DB.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash IN (
            SELECT token_hash FROM auth_sessions WHERE expires_at <= CURRENT_TIMESTAMP
            ORDER BY expires_at LIMIT 256 FOR UPDATE SKIP LOCKED
        )`); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if _, err := a.DB.ExecContext(ctx, "INSERT INTO auth_sessions (token_hash, expires_at) VALUES ($1, $2)", sessionHash(tok), exp); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if a.AccountLockout != nil {
			a.AccountLockout.RecordSuccessfulLogin(body.Username)
		}

		http.SetCookie(w, a.cookie(r, tok, exp))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	}
}

// sessionHash stores no bearer credential in the database.
func sessionHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type identity struct {
	Subject, Username       string
	Admin, EnvironmentAdmin bool
}
type identityKey struct{}

var errUnauthenticated = errors.New("unauthorized")
var errRegisteredAccountRequired = errors.New("registered account required")

// resolveIdentity validates both the signed claims and the persisted session.
// Text comparison prevents an unknown subject from becoming a UUID SQL error.
func (a AuthConfig) resolveIdentity(r *http.Request) (identity, error) {
	if principal, ok := r.Context().Value(identityKey{}).(identity); ok {
		return principal, nil
	}
	var principal identity
	c, err := r.Cookie(a.cookieName())
	if err != nil {
		return principal, errUnauthenticated
	}
	payload, err := a.verifyToken(c.Value)
	if err != nil {
		return principal, errUnauthenticated
	}
	if a.DB == nil {
		return principal, errors.New("missing session database")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var present bool
	err = a.DB.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM auth_sessions WHERE token_hash = $1 AND expires_at > CURRENT_TIMESTAMP)", sessionHash(c.Value)).Scan(&present)
	if err != nil {
		return principal, err
	}
	if !present {
		return principal, errUnauthenticated
	}
	principal.Subject = payload.Sub
	if payload.Admin {
		principal.Username, principal.Admin, principal.EnvironmentAdmin = a.AdminUser, true, true
		return principal, nil
	}
	err = a.DB.QueryRowContext(ctx, "SELECT username, is_admin FROM users WHERE id::text = $1 AND is_active = TRUE AND email_verified = TRUE", payload.Sub).Scan(&principal.Username, &principal.Admin)
	if errors.Is(err, sql.ErrNoRows) {
		return identity{}, errUnauthenticated
	}
	return principal, err
}

func writeAuthError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "server error"
	if errors.Is(err, errUnauthenticated) {
		status, message = http.StatusUnauthorized, "unauthorized"
	}
	if errors.Is(err, errRegisteredAccountRequired) {
		status, message = http.StatusForbidden, "registered account required"
	}
	http.Error(w, message, status)
}

// logoutHandler revokes the current session before reporting success.
func (a AuthConfig) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, err := a.resolveIdentity(r); err != nil {
			writeAuthError(w, err)
			return
		}
		c, _ := r.Cookie(a.cookieName())
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, err := a.DB.ExecContext(ctx, "DELETE FROM auth_sessions WHERE token_hash = $1", sessionHash(c.Value)); err != nil {
			writeAuthError(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: a.cookieName(), Path: "/", Expires: time.Unix(0, 0), MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: true})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}
}

func (a AuthConfig) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := a.resolveIdentity(r)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, principal)))
	})
}

// getCurrentUser is for UUID-backed ownership and quota paths only.
func (a AuthConfig) getCurrentUser(r *http.Request) (string, error) {
	principal, err := a.resolveIdentity(r)
	if err != nil {
		return "", err
	}
	if principal.EnvironmentAdmin {
		return "", errRegisteredAccountRequired
	}
	return principal.Subject, nil
}

func (a AuthConfig) requireAdmin(next http.Handler) http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := a.resolveIdentity(r)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		if !principal.Admin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (a AuthConfig) meHandler() http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		principal, err := a.resolveIdentity(r)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "username": principal.Username, "is_admin": principal.Admin})
	}))
}
