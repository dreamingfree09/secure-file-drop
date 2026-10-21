// ratelimit.go - Enhanced token-bucket rate limiter with per-IP and per-user limiting
//
// Provides comprehensive rate limiting for different endpoint types:
// - General rate limiting (IP-based)
// - Authentication endpoints (stricter IP-based)
// - Upload endpoints (user-based)
//
// Includes automatic cleanup of inactive limiters to prevent memory leaks.

package server

import (
	"net"
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

// getClientIP uses the immediate peer. Forwarded headers are not authenticated.
// Proxy-shared limits remain a deployment/topology constraint until an explicit trust policy is reviewed.
func getClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
