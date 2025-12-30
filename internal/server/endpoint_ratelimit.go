// endpoint_ratelimit.go - Per-endpoint rate limiting for Secure File Drop.
//
// Provides specialized rate limiting for different endpoint types with
// configurable limits based on endpoint sensitivity and resource usage.
package server

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// EndpointRateLimiter manages rate limits for different endpoint types.
type EndpointRateLimiter struct {
	// Different rate limiters for different endpoint categories
	authLimiter     *rateLimiter // Stricter limits for auth endpoints
	uploadLimiter   *rateLimiter // Upload-specific limits
	downloadLimiter *rateLimiter // Download-specific limits
	apiLimiter      *rateLimiter // General API limits
	adminLimiter    *rateLimiter // Admin endpoint limits
}

// NewEndpointRateLimiter creates a new endpoint-specific rate limiter.
func NewEndpointRateLimiter() *EndpointRateLimiter {
	return &EndpointRateLimiter{
		// Auth endpoints: 10 attempts per minute (prevent brute force)
		authLimiter: newRateLimiter(10, time.Minute),

		// Upload endpoints: 20 uploads per hour per IP
		uploadLimiter: newRateLimiter(20, time.Hour),

		// Download endpoints: 100 downloads per hour per IP
		downloadLimiter: newRateLimiter(100, time.Hour),

		// General API: 300 requests per minute
		apiLimiter: newRateLimiter(300, time.Minute),

		// Admin endpoints: 50 requests per minute
		adminLimiter: newRateLimiter(50, time.Minute),
	}
}

// Middleware returns an HTTP middleware that applies endpoint-specific rate limits.
func (erl *EndpointRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		ip := getClientIP(r)

		// Determine which rate limiter to use based on endpoint
		var limiter *rateLimiter
		var limitType string

		switch {
		case strings.HasPrefix(path, "/login") || strings.HasPrefix(path, "/register"):
			limiter = erl.authLimiter
			limitType = "authentication"

		case strings.HasPrefix(path, "/upload"):
			limiter = erl.uploadLimiter
			limitType = "upload"

		case strings.HasPrefix(path, "/download") || strings.HasPrefix(path, "/links"):
			limiter = erl.downloadLimiter
			limitType = "download"

		case strings.HasPrefix(path, "/admin/"):
			limiter = erl.adminLimiter
			limitType = "admin"

		default:
			limiter = erl.apiLimiter
			limitType = "api"
		}

		// Check rate limit
		if !limiter.allow(ip) {
			// Log rate limit violation
			Warn("rate_limit_exceeded", map[string]any{
				"ip":         ip,
				"path":       path,
				"method":     r.Method,
				"limit_type": limitType,
			})

			// Return 429 Too Many Requests with Retry-After header
			w.Header().Set("Retry-After", "60") // Suggest retry after 60 seconds
			w.Header().Set("X-RateLimit-Limit-Type", limitType)
			http.Error(w, "Rate limit exceeded for "+limitType+" endpoints. Please try again later.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// EndpointRateLimitConfig holds configuration for endpoint rate limits.
type EndpointRateLimitConfig struct {
	AuthRate       int           // Requests per window for auth endpoints
	AuthWindow     time.Duration // Window for auth rate limiting
	UploadRate     int           // Uploads per window
	UploadWindow   time.Duration // Window for upload rate limiting
	DownloadRate   int           // Downloads per window
	DownloadWindow time.Duration // Window for download rate limiting
	APIRate        int           // General API requests per window
	APIWindow      time.Duration // Window for API rate limiting
	AdminRate      int           // Admin requests per window
	AdminWindow    time.Duration // Window for admin rate limiting
}

// DefaultEndpointRateLimitConfig returns sensible default rate limit configuration.
func DefaultEndpointRateLimitConfig() EndpointRateLimitConfig {
	return EndpointRateLimitConfig{
		AuthRate:       10,
		AuthWindow:     time.Minute,
		UploadRate:     20,
		UploadWindow:   time.Hour,
		DownloadRate:   100,
		DownloadWindow: time.Hour,
		APIRate:        300,
		APIWindow:      time.Minute,
		AdminRate:      50,
		AdminWindow:    time.Minute,
	}
}

// NewEndpointRateLimiterWithConfig creates a rate limiter with custom configuration.
func NewEndpointRateLimiterWithConfig(cfg EndpointRateLimitConfig) *EndpointRateLimiter {
	return &EndpointRateLimiter{
		authLimiter:     newRateLimiter(cfg.AuthRate, cfg.AuthWindow),
		uploadLimiter:   newRateLimiter(cfg.UploadRate, cfg.UploadWindow),
		downloadLimiter: newRateLimiter(cfg.DownloadRate, cfg.DownloadWindow),
		apiLimiter:      newRateLimiter(cfg.APIRate, cfg.APIWindow),
		adminLimiter:    newRateLimiter(cfg.AdminRate, cfg.AdminWindow),
	}
}

// UserRateLimiter implements per-user rate limiting (in addition to IP-based).
type UserRateLimiter struct {
	mu     sync.RWMutex
	users  map[string]*visitor
	rate   int
	window time.Duration
}

// NewUserRateLimiter creates a rate limiter that tracks per-user requests.
func NewUserRateLimiter(rate int, window time.Duration) *UserRateLimiter {
	url := &UserRateLimiter{
		users:  make(map[string]*visitor),
		rate:   rate,
		window: window,
	}

	go url.cleanup()

	return url
}

// Allow checks if a request from the given user should be allowed.
func (url *UserRateLimiter) Allow(userID string) bool {
	url.mu.Lock()
	v, exists := url.users[userID]
	if !exists {
		v = &visitor{
			requests: make([]time.Time, 0, url.rate),
		}
		url.users[userID] = v
	}
	url.mu.Unlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-url.window)

	// Remove old requests
	validRequests := make([]time.Time, 0, len(v.requests))
	for _, t := range v.requests {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}
	v.requests = validRequests

	// Check limit
	if len(v.requests) >= url.rate {
		return false
	}

	// Add current request
	v.requests = append(v.requests, now)
	return true
}

// cleanup periodically removes inactive users.
func (url *UserRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		url.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-url.window * 2)

		for userID, v := range url.users {
			v.mu.Lock()
			if len(v.requests) == 0 || v.requests[len(v.requests)-1].Before(cutoff) {
				delete(url.users, userID)
			}
			v.mu.Unlock()
		}
		url.mu.Unlock()
	}
}
