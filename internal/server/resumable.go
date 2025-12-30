// resumable.go - Resumable upload support for Secure File Drop.
//
// Implements TUS (tus.io) protocol for resumable file uploads, allowing
// large files to be uploaded in chunks with automatic resume on failure.
package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ResumableUpload represents metadata for a resumable upload session.
type ResumableUpload struct {
	ID           string    `json:"id"`
	FileID       string    `json:"file_id"`
	ObjectKey    string    `json:"object_key"`
	TotalSize    int64     `json:"total_size"`
	CurrentSize  int64     `json:"current_size"`
	UploadID     string    `json:"upload_id"` // MinIO multipart upload ID
	CreatedAt    time.Time `json:"created_at"`
	LastModified time.Time `json:"last_modified"`
	Status       string    `json:"status"` // "active", "completed", "failed"
}

// InitiateResumableUpload starts a new resumable upload session.
// Returns the upload session ID and metadata.
// NOTE: Full TUS implementation requires MinIO multipart upload support
func (s *Server) InitiateResumableUpload(fileID string, totalSize int64, objectKey string) (*ResumableUpload, error) {
	// TODO: Implement MinIO multipart upload when available
	// For now, this is a placeholder that creates session metadata

	upload := &ResumableUpload{
		ID:           uuid.New().String(),
		FileID:       fileID,
		ObjectKey:    objectKey,
		TotalSize:    totalSize,
		CurrentSize:  0,
		UploadID:     uuid.New().String(), // Placeholder upload ID
		CreatedAt:    time.Now(),
		LastModified: time.Now(),
		Status:       "active",
	}

	// Store upload session in database
	_, err := s.db.Exec(`
		INSERT INTO resumable_uploads 
		(id, file_id, object_key, total_size, current_size, upload_id, created_at, last_modified, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, upload.ID, upload.FileID, upload.ObjectKey, upload.TotalSize, upload.CurrentSize,
		upload.UploadID, upload.CreatedAt, upload.LastModified, upload.Status)

	if err != nil {
		return nil, fmt.Errorf("failed to store upload session: %w", err)
	}

	return upload, nil
}

// GetResumableUpload retrieves upload session metadata.
func (s *Server) GetResumableUpload(uploadID string) (*ResumableUpload, error) {
	upload := &ResumableUpload{}
	err := s.db.QueryRow(`
		SELECT id, file_id, object_key, total_size, current_size, upload_id, 
		       created_at, last_modified, status
		FROM resumable_uploads
		WHERE id = $1
	`, uploadID).Scan(
		&upload.ID, &upload.FileID, &upload.ObjectKey, &upload.TotalSize,
		&upload.CurrentSize, &upload.UploadID, &upload.CreatedAt,
		&upload.LastModified, &upload.Status,
	)

	if err != nil {
		return nil, err
	}

	return upload, nil
}

// UpdateUploadProgress updates the current size and last modified time.
func (s *Server) UpdateUploadProgress(uploadID string, currentSize int64) error {
	_, err := s.db.Exec(`
		UPDATE resumable_uploads
		SET current_size = $1, last_modified = $2
		WHERE id = $3
	`, currentSize, time.Now(), uploadID)

	return err
}

// CompleteResumableUpload finalizes the multipart upload.
func (s *Server) CompleteResumableUpload(uploadID string) error {
	upload, err := s.GetResumableUpload(uploadID)
	if err != nil {
		return err
	}

	// TODO: Implement MinIO multipart completion when available
	// For now, just update status in database

	// Update database status
	_, err = s.db.Exec(`
		UPDATE resumable_uploads
		SET status = 'completed', last_modified = $1
		WHERE id = $2
	`, time.Now(), uploadID)

	_ = upload // Silence unused warning
	return err
}

// AbortResumableUpload cancels an in-progress upload.
func (s *Server) AbortResumableUpload(uploadID string) error {
	upload, err := s.GetResumableUpload(uploadID)
	if err != nil {
		return err
	}

	// TODO: Implement MinIO multipart abort when available

	// Update database status
	_, err = s.db.Exec(`
		UPDATE resumable_uploads
		SET status = 'failed', last_modified = $1
		WHERE id = $2
	`, time.Now(), uploadID)

	_ = upload // Silence unused warning
	return err
}

// TUS Protocol Handlers

// tusCreateHandler handles POST requests to create a new upload session.
// Implements TUS Creation extension.
func (s *Server) tusCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse Upload-Length header (required in TUS protocol)
		uploadLengthStr := r.Header.Get("Upload-Length")
		if uploadLengthStr == "" {
			http.Error(w, "Upload-Length header required", http.StatusBadRequest)
			return
		}

		uploadLength, err := strconv.ParseInt(uploadLengthStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid Upload-Length", http.StatusBadRequest)
			return
		}

		// Get file ID from metadata
		metadata := r.Header.Get("Upload-Metadata")
		fileID := extractMetadata(metadata, "file_id")
		if fileID == "" {
			http.Error(w, "file_id metadata required", http.StatusBadRequest)
			return
		}

		// Verify file exists and is pending
		var objectKey string
		var status string
		err = s.db.QueryRow(`SELECT object_key, status FROM files WHERE id = $1`, fileID).Scan(&objectKey, &status)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "file not found", http.StatusNotFound)
				return
			}
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		if status != "pending" {
			http.Error(w, "invalid file status", http.StatusConflict)
			return
		}

		// Initiate resumable upload
		upload, err := s.InitiateResumableUpload(fileID, uploadLength, objectKey)
		if err != nil {
			Error("failed to initiate resumable upload", map[string]any{"error": err.Error()}, err)
			http.Error(w, "failed to create upload", http.StatusInternalServerError)
			return
		}

		// Return TUS-compliant response
		w.Header().Set("Tus-Resumable", "1.0.0")
		w.Header().Set("Upload-Offset", "0")
		w.Header().Set("Location", fmt.Sprintf("/upload/resumable/%s", upload.ID))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(upload)
	}
}

// tusPatchHandler handles PATCH requests to upload chunks.
// Implements TUS Core protocol.
func (s *Server) tusPatchHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract upload ID from URL path
		uploadID := strings.TrimPrefix(r.URL.Path, "/upload/resumable/")
		if uploadID == "" {
			http.Error(w, "upload ID required", http.StatusBadRequest)
			return
		}

		// Get upload session
		upload, err := s.GetResumableUpload(uploadID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "upload not found", http.StatusNotFound)
				return
			}
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		if upload.Status != "active" {
			http.Error(w, "upload not active", http.StatusConflict)
			return
		}

		// Verify Upload-Offset matches current size
		offsetStr := r.Header.Get("Upload-Offset")
		offset, err := strconv.ParseInt(offsetStr, 10, 64)
		if err != nil || offset != upload.CurrentSize {
			http.Error(w, "invalid Upload-Offset", http.StatusConflict)
			return
		}

		// Read chunk data
		chunkSize := r.ContentLength
		if chunkSize <= 0 {
			http.Error(w, "Content-Length required", http.StatusBadRequest)
			return
		}

		// Stream chunk to MinIO (simplified - actual implementation would use PutObjectPart)
		// This is a placeholder - full TUS implementation requires tracking part numbers
		_, err = io.CopyN(io.Discard, r.Body, chunkSize)
		if err != nil {
			http.Error(w, "failed to read chunk", http.StatusInternalServerError)
			return
		}

		// Update progress
		newSize := upload.CurrentSize + chunkSize
		err = s.UpdateUploadProgress(uploadID, newSize)
		if err != nil {
			http.Error(w, "failed to update progress", http.StatusInternalServerError)
			return
		}

		// Check if upload is complete
		if newSize >= upload.TotalSize {
			err = s.CompleteResumableUpload(uploadID)
			if err != nil {
				Error("failed to complete upload", map[string]any{"error": err.Error()}, err)
				http.Error(w, "failed to complete upload", http.StatusInternalServerError)
				return
			}
		}

		// Return TUS-compliant response
		w.Header().Set("Tus-Resumable", "1.0.0")
		w.Header().Set("Upload-Offset", strconv.FormatInt(newSize, 10))
		w.WriteHeader(http.StatusNoContent)
	}
}

// tusHeadHandler handles HEAD requests to check upload status.
// Implements TUS Core protocol.
func (s *Server) tusHeadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		uploadID := strings.TrimPrefix(r.URL.Path, "/upload/resumable/")
		if uploadID == "" {
			http.Error(w, "upload ID required", http.StatusBadRequest)
			return
		}

		upload, err := s.GetResumableUpload(uploadID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "upload not found", http.StatusNotFound)
				return
			}
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Tus-Resumable", "1.0.0")
		w.Header().Set("Upload-Offset", strconv.FormatInt(upload.CurrentSize, 10))
		w.Header().Set("Upload-Length", strconv.FormatInt(upload.TotalSize, 10))
		w.WriteHeader(http.StatusOK)
	}
}

// extractMetadata extracts a value from TUS Upload-Metadata header.
// Format: "key1 base64value1,key2 base64value2"
func extractMetadata(header, key string) string {
	pairs := strings.Split(header, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), " ", 2)
		if len(parts) == 2 && parts[0] == key {
			return parts[1] // In production, this should be base64-decoded
		}
	}
	return ""
}
