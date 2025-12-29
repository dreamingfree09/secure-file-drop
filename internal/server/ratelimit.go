// rateLimiter implements a simple token bucket rate limiting middleware
// applied globally. It limits requests per IP over a fixed interval and
// responds with 429 when the bucket is empty.
// ratelimit.go - Token-bucket rate limiter middleware by client IP.
//
// Provides a simple per-IP limiter to protect endpoints; designed
// to complement proxy-side limits.
package server

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a simple token bucket rate limiter for HTTP requests.
// It tracks requests per IP address using an in-memory map with periodic cleanup.
type rateLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	rate     int           // requests allowed per window
	window   time.Duration // time window for rate limiting
}

// visitor tracks request timestamps for a single IP address
type visitor struct {
	requests []time.Time
	mu       sync.Mutex
}

// newRateLimiter creates a rate limiter that allows 'rate' requests per 'window'.
// Example: newRateLimiter(100, time.Minute) allows 100 requests per minute per IP.
func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}

	// Start cleanup goroutine to remove old visitor entries
	go rl.cleanup()

	return rl
}

// middleware returns an HTTP middleware that enforces rate limits
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		if !rl.allow(ip) {
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allow checks if a request from the given IP should be allowed
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	v, exists := rl.visitors[ip]
	if !exists {
		v = &visitor{
			requests: make([]time.Time, 0, rl.rate),
		}
		rl.visitors[ip] = v
	}
	rl.mu.Unlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove requests older than the window
	validRequests := make([]time.Time, 0, len(v.requests))
	for _, t := range v.requests {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}
	v.requests = validRequests

	// Check if we're under the limit
	if len(v.requests) >= rl.rate {
		return false
	}

	// Add current request
	v.requests = append(v.requests, now)
	return true
}

// cleanup periodically removes visitors with no recent requests
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window * 2) // Keep visitors for 2x window

		for ip, v := range rl.visitors {
			v.mu.Lock()
			if len(v.requests) == 0 || v.requests[len(v.requests)-1].Before(cutoff) {
				delete(rl.visitors, ip)
			}
			v.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

// getClientIP extracts the client's IP address from the request.
// It checks X-Forwarded-For and X-Real-IP headers first (for reverse proxies),
// then falls back to RemoteAddr.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (comma-separated list of IPs)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		for i, c := range xff {
			if c == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr (format: "ip:port")
	for i := len(r.RemoteAddr) - 1; i >= 0; i-- {
		if r.RemoteAddr[i] == ':' {
			return r.RemoteAddr[:i]
		}
	}

	return r.RemoteAddr
}
