package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// createFileReq represents the JSON payload for creating a new file record.
// This is the first step in the upload flow - creating metadata before the actual upload.
type createFileReq struct {
	OrigName    string `json:"orig_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// createFileResp is the JSON response returned when a file record is successfully created.
// Contains the generated UUID, object storage key, and initial status ("pending").
type createFileResp struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Status    string `json:"status"`
}

// createFileHandler handles POST /files requests to create a new file metadata record.
// This is step 1 of the upload flow: register file metadata in the database with status "pending".
// The client must then upload the actual file data via POST /upload?id={uuid}.
//
// Request body: JSON with orig_name, content_type, size_bytes
// Response: JSON with id (UUID), object_key, status ("pending")
// Authentication: Required (checked by requireAuth middleware)
func (cfg Config) createFileHandler(db *sql.DB) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req createFileReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Sanitize input by trimming whitespace
		req.OrigName = strings.TrimSpace(req.OrigName)
		req.ContentType = strings.TrimSpace(req.ContentType)

		// Validate required fields and ensure size is non-negative
		if req.OrigName == "" || req.ContentType == "" || req.SizeBytes < 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Generate a unique UUID for the file record
		id := uuid.New()
		// Create a stable, non-guessable object key in MinIO.
		// Uses "uploads/" prefix + UUID to avoid path traversal attacks.
		objectKey := "uploads/" + id.String()

		_, err := db.Exec(`
			INSERT INTO files (id, object_key, orig_name, content_type, size_bytes, created_by, status)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		`, id, objectKey, req.OrigName, req.ContentType, req.SizeBytes, cfg.Auth.AdminUser)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createFileResp{
			ID:        id.String(),
			ObjectKey: objectKey,
			Status:    "pending",
		})
	}))
}
