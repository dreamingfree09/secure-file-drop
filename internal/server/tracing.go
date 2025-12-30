// tracing.go - Request tracing and correlation ID middleware for Secure File Drop.
//
// Provides distributed tracing support with correlation IDs that are propagated
// through logs, responses, and can be used for debugging and request tracking.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

// Context keys for storing request-scoped values
type contextKey string

const (
	// CorrelationIDKey is the context key for storing correlation IDs
	CorrelationIDKey contextKey = "correlation_id"
	// RequestStartKey is the context key for storing request start time
	RequestStartKey contextKey = "request_start"
)

// Header names for correlation ID propagation
const (
	// HeaderCorrelationID is the header name for correlation IDs
	HeaderCorrelationID = "X-Correlation-ID"
	// HeaderRequestID is an alias for X-Correlation-ID (common in some systems)
	HeaderRequestID = "X-Request-ID"
)

// generateCorrelationID creates a new unique correlation ID.
func generateCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if random fails
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// GetCorrelationID extracts the correlation ID from the request context.
// Returns empty string if not found.
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// GetRequestDuration calculates how long the request has been running.
// Returns 0 if start time not found in context.
func GetRequestDuration(ctx context.Context) time.Duration {
	if start, ok := ctx.Value(RequestStartKey).(time.Time); ok {
		return time.Since(start)
	}
	return 0
}

// TracingMiddleware adds correlation ID tracking to all requests.
// It accepts existing correlation IDs from headers or generates new ones.
// The correlation ID is added to the response headers and request context.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get correlation ID from incoming headers
		correlationID := r.Header.Get(HeaderCorrelationID)
		if correlationID == "" {
			correlationID = r.Header.Get(HeaderRequestID)
		}

		// Generate new correlation ID if not provided
		if correlationID == "" {
			correlationID = generateCorrelationID()
		}

		// Add correlation ID to response headers for client tracking
		w.Header().Set(HeaderCorrelationID, correlationID)

		// Store correlation ID and request start time in context
		ctx := context.WithValue(r.Context(), CorrelationIDKey, correlationID)
		ctx = context.WithValue(ctx, RequestStartKey, time.Now())

		// Log request start with correlation ID
		Info("request_started", map[string]any{
			"method":         r.Method,
			"path":           r.URL.Path,
			"remote_addr":    r.RemoteAddr,
			"user_agent":     r.UserAgent(),
			"correlation_id": correlationID,
		})

		// Create response writer wrapper to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Continue with request processing
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		// Log request completion with duration
		duration := GetRequestDuration(ctx)
		Info("request_completed", map[string]any{
			"method":         r.Method,
			"path":           r.URL.Path,
			"status":         wrapped.statusCode,
			"duration_ms":    duration.Milliseconds(),
			"correlation_id": correlationID,
		})
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and delegates to the underlying writer.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// LogWithCorrelation logs a message with the correlation ID from the context.
// This is a convenience function for adding correlation IDs to log entries.
func LogWithCorrelation(ctx context.Context, level string, message string, fields map[string]any) {
	if fields == nil {
		fields = make(map[string]any)
	}

	// Add correlation ID if available
	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		fields["correlation_id"] = correlationID
	}

	switch level {
	case "debug":
		Debug(message, fields)
	case "info":
		Info(message, fields)
	case "warn":
		Warn(message, fields)
	case "error":
		Error(message, fields, nil)
	default:
		Info(message, fields)
	}
}
