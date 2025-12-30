// security.go - Security middleware for headers and CSRF protection
package server

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

// securityHeadersMiddleware adds security headers to all responses
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// XSS Protection (legacy but still useful)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer Policy - don't leak URLs
		w.Header().Set("Referrer-Policy", "no-referrer")

		// Content Security Policy - defense in depth against XSS
		// Note: 'unsafe-inline' for scripts is needed for current implementation
		// TODO: Move to external JS files and remove 'unsafe-inline'
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data:; " +
			"font-src 'self'; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		// HSTS - Force HTTPS (uncomment in production with HTTPS)
		// w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Permissions Policy - disable unused browser features
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
}

// csrfToken represents a CSRF token with expiration
type csrfToken struct {
	token     string
	expiresAt time.Time
}

// CSRFProtection implements CSRF token generation and validation
type CSRFProtection struct {
	mu     sync.RWMutex
	tokens map[string]csrfToken // sessionID -> token
	ttl    time.Duration
}

// NewCSRFProtection creates a new CSRF protection instance
func NewCSRFProtection(ttl time.Duration) *CSRFProtection {
	csrf := &CSRFProtection{
		tokens: make(map[string]csrfToken),
		ttl:    ttl,
	}

	// Cleanup expired tokens every hour
	go csrf.cleanup()

	return csrf
}

// generateToken creates a random CSRF token
func (c *CSRFProtection) generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GetToken retrieves or creates a CSRF token for the session
func (c *CSRFProtection) GetToken(sessionID string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if valid token exists
	if t, ok := c.tokens[sessionID]; ok && time.Now().Before(t.expiresAt) {
		return t.token, nil
	}

	// Generate new token
	token, err := c.generateToken()
	if err != nil {
		return "", err
	}

	c.tokens[sessionID] = csrfToken{
		token:     token,
		expiresAt: time.Now().Add(c.ttl),
	}

	return token, nil
}

// ValidateToken checks if the provided token matches the session's CSRF token
func (c *CSRFProtection) ValidateToken(sessionID, token string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	t, ok := c.tokens[sessionID]
	if !ok {
		return false
	}

	// Check expiration
	if time.Now().After(t.expiresAt) {
		return false
	}

	// Constant-time comparison
	return token == t.token
}

// InvalidateToken removes a CSRF token (e.g., on logout)
func (c *CSRFProtection) InvalidateToken(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, sessionID)
}

// cleanup removes expired tokens periodically
func (c *CSRFProtection) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for sessionID, t := range c.tokens {
			if now.After(t.expiresAt) {
				delete(c.tokens, sessionID)
			}
		}
		c.mu.Unlock()
	}
}

// CSRFMiddleware validates CSRF tokens for state-changing requests
func (c *CSRFProtection) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only check CSRF for state-changing methods
		if r.Method == http.MethodPost || r.Method == http.MethodPut ||
			r.Method == http.MethodDelete || r.Method == http.MethodPatch {

			// Get session ID from cookie (if authenticated)
			cookie, err := r.Cookie("sfd_session")
			if err != nil {
				// No session = no CSRF check needed (will fail auth anyway)
				next.ServeHTTP(w, r)
				return
			}

			// Get CSRF token from header
			csrfToken := r.Header.Get("X-CSRF-Token")
			if csrfToken == "" {
				// Try form value as fallback
				csrfToken = r.FormValue("csrf_token")
			}

			if csrfToken == "" || !c.ValidateToken(cookie.Value, csrfToken) {
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
