package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestIDFromContext returns the request id if present.
func RequestIDFromContext(ctx context.Context) string {
	v := ctx.Value(requestIDKey)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// generateRequestID creates a 16-byte random ID encoded as hex (32 chars).
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: time-based (rare)
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// requestIDMiddleware ensures every request has a request id.
// If the client supplies X-Request-Id, we keep it; otherwise we generate one.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = generateRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		w.Header().Set("X-Request-Id", rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs one line per request with detailed structured information.
// Includes request ID, method, path, status, timing, client IP, user agent, and size.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := RequestIDFromContext(r.Context())

		// Wrap ResponseWriter to capture status code and response size
		lrw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		ms := duration.Milliseconds()

		// Extract client IP (same logic as rate limiter)
		clientIP := getClientIPForLogging(r)

		// Log with enhanced details
		log.Printf("rid=%s method=%s path=%s status=%d ms=%d bytes=%d ip=%s ua=%q referer=%q",
			rid,
			r.Method,
			r.URL.Path,
			lrw.status,
			ms,
			lrw.size,
			clientIP,
			r.UserAgent(),
			r.Referer(),
		)

		// Record metrics
		GetMetrics().RecordRequest(lrw.status)
	})
}

// getClientIPForLogging extracts client IP for logging purposes
func getClientIPForLogging(r *http.Request) string {
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

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	return n, err
}
