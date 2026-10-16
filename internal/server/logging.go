package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
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
		if rid == "" || len(rid) > 128 || strings.IndexFunc(rid, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
			rid = generateRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		w.Header().Set("X-Request-Id", rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs one line per request in a simple structured format.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := RequestIDFromContext(r.Context())

		// Wrap ResponseWriter to capture status code.
		lrw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lrw, r)

		ms := time.Since(start).Milliseconds()
		// #nosec G706 -- All request-controlled string fields are quoted with %q.
		log.Printf("rid=%q method=%q path=%q status=%d ms=%d remote=%q ua=%q",
			rid, r.Method, r.URL.Path, lrw.status, ms, r.RemoteAddr, r.UserAgent())

		// Record metrics
		GetMetrics().RecordRequest(lrw.status)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
