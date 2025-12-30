// validation.go - Input validation and sanitization helpers
package server

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// allowedMimeTypes defines file types permitted for upload
// Customize this list based on your application's requirements
var allowedMimeTypes = map[string]bool{
	// Documents
	"application/pdf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"text/plain":       true,
	"text/csv":         true,
	"text/html":        true,
	"text/css":         true,
	"text/javascript":  true,
	"application/json": true,
	"application/xml":  true,

	// Images
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
	"image/bmp":     true,
	"image/tiff":    true,

	// Audio
	"audio/mpeg": true,
	"audio/ogg":  true,
	"audio/wav":  true,
	"audio/webm": true,
	"audio/flac": true,

	// Video
	"video/mp4":       true,
	"video/mpeg":      true,
	"video/ogg":       true,
	"video/webm":      true,
	"video/x-msvideo": true,

	// Archives
	"application/zip":              true,
	"application/x-tar":            true,
	"application/gzip":             true,
	"application/x-7z-compressed":  true,
	"application/x-rar-compressed": true,

	// Code
	"application/x-python-code": true,
	"application/x-sh":          true,

	// Generic binary (use with caution)
	"application/octet-stream": true,
}

// dangerousExtensions lists file extensions that should never be executed
var dangerousExtensions = map[string]bool{
	".exe":   true,
	".bat":   true,
	".cmd":   true,
	".com":   true,
	".pif":   true,
	".scr":   true,
	".vbs":   true,
	".js":    false, // JavaScript can be legitimate
	".jar":   true,
	".app":   true,
	".deb":   true,
	".rpm":   true,
	".dmg":   true,
	".pkg":   true,
	".msi":   true,
	".dll":   true,
	".so":    true,
	".dylib": true,
}

// ValidateUploadMimeType checks if the uploaded file's MIME type is allowed
// It validates against both the client-provided Content-Type and file extension
func ValidateUploadMimeType(filename, clientContentType string) error {
	// Normalize client content type
	clientContentType = strings.TrimSpace(strings.ToLower(clientContentType))

	// Check file extension
	ext := strings.ToLower(filepath.Ext(filename))

	// Reject dangerous executables
	if dangerous, exists := dangerousExtensions[ext]; exists && dangerous {
		return fmt.Errorf("file type not allowed: %s", ext)
	}

	// If no extension, be more strict with MIME type
	if ext == "" && clientContentType == "" {
		return fmt.Errorf("file must have an extension or content type")
	}

	// Validate MIME type from client
	if clientContentType != "" {
		// Remove charset and other parameters
		mimeType := clientContentType
		if idx := strings.Index(clientContentType, ";"); idx > 0 {
			mimeType = clientContentType[:idx]
		}
		mimeType = strings.TrimSpace(mimeType)

		// Check if allowed
		if !allowedMimeTypes[mimeType] {
			return fmt.Errorf("MIME type not allowed: %s", mimeType)
		}
	}

	// Cross-check extension with expected MIME type
	if ext != "" {
		expectedMime := mime.TypeByExtension(ext)
		if expectedMime != "" {
			// Remove charset parameters
			if idx := strings.Index(expectedMime, ";"); idx > 0 {
				expectedMime = expectedMime[:idx]
			}
			expectedMime = strings.TrimSpace(expectedMime)

			// If we have a client content type, it should match or be compatible
			if clientContentType != "" {
				clientMime := clientContentType
				if idx := strings.Index(clientContentType, ";"); idx > 0 {
					clientMime = clientContentType[:idx]
				}
				clientMime = strings.TrimSpace(clientMime)

				// Allow application/octet-stream as generic fallback
				if clientMime != expectedMime && clientMime != "application/octet-stream" {
					// Be lenient for common mismatches
					if !isMimeTypeCompatible(expectedMime, clientMime) {
						return fmt.Errorf("MIME type mismatch: extension suggests %s but got %s", expectedMime, clientMime)
					}
				}
			}
		}
	}

	return nil
}

// isMimeTypeCompatible checks if two MIME types are compatible (e.g., text/plain vs text/*)
func isMimeTypeCompatible(expected, actual string) bool {
	// Split into type/subtype
	expParts := strings.Split(expected, "/")
	actParts := strings.Split(actual, "/")

	if len(expParts) != 2 || len(actParts) != 2 {
		return false
	}

	// Same major type is often acceptable
	return expParts[0] == actParts[0]
}

// DetectContentType uses http.DetectContentType to verify file content
// This reads the first 512 bytes to detect the actual MIME type
func DetectContentType(data []byte) string {
	return http.DetectContentType(data)
}

// SanitizeFilename removes potentially dangerous characters from filenames
func SanitizeFilename(filename string) string {
	// Remove path separators
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")

	// Remove null bytes
	filename = strings.ReplaceAll(filename, "\x00", "")

	// Trim spaces and dots from start/end
	filename = strings.Trim(filename, " .")

	// Limit length
	if len(filename) > 255 {
		ext := filepath.Ext(filename)
		nameWithoutExt := filename[:len(filename)-len(ext)]
		filename = nameWithoutExt[:255-len(ext)] + ext
	}

	if filename == "" {
		filename = "unnamed"
	}

	return filename
}
