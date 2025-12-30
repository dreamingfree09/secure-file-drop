// compression.go - HTTP compression middleware for Secure File Drop.
//
// Implements gzip and deflate compression for responses to reduce bandwidth
// and improve performance for text-based responses (JSON, HTML, etc.).
package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// compressionResponseWriter wraps http.ResponseWriter to compress responses.
type compressionResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

// Write compresses data before writing to the underlying writer.
func (crw *compressionResponseWriter) Write(b []byte) (int, error) {
	return crw.writer.Write(b)
}

// CompressionMiddleware returns middleware that compresses HTTP responses.
func CompressionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts compression
		if !acceptsCompression(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Check if response should be compressed (skip binary files, already compressed)
		if shouldSkipCompression(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Create gzip writer
		gz := gzip.NewWriter(w)
		defer gz.Close()

		// Set headers
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Length will change with compression

		// Wrap response writer
		crw := &compressionResponseWriter{
			ResponseWriter: w,
			writer:         gz,
		}

		next.ServeHTTP(crw, r)
	})
}

// acceptsCompression checks if the client accepts gzip encoding.
func acceptsCompression(r *http.Request) bool {
	acceptEncoding := r.Header.Get("Accept-Encoding")
	return strings.Contains(acceptEncoding, "gzip")
}

// shouldSkipCompression determines if compression should be skipped for this request.
func shouldSkipCompression(r *http.Request) bool {
	path := r.URL.Path

	// Skip compression for file downloads (already compressed or binary)
	if strings.HasPrefix(path, "/download") {
		return true
	}

	// Skip for uploaded files
	if strings.HasPrefix(path, "/upload") && r.Method == http.MethodPost {
		return true
	}

	// Compress JSON API responses, HTML, etc.
	return false
}
