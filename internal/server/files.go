// files.go - Metadata creation and user quota enforcement.
//
// Handles POST /files to register file metadata with lifecycle status
// and enforces per-user storage quotas before accepting uploads.
package server

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// createFileReq represents the JSON payload for creating a new file record.
//
// This is step 1 in the upload flow: create metadata before streaming data.
type createFileReq struct {
	OrigName     string `json:"orig_name"`
	ContentType  string `json:"content_type"`
	SizeBytes    int64  `json:"size_bytes"`
	TTLHours     int    `json:"ttl_hours,omitempty"`     // Optional: hours until file expires (0 = never)
	LinkPassword string `json:"link_password,omitempty"` // Optional: password required to download
}

// createFileResp is returned when a file record is successfully created.
// It contains the generated UUID, object storage key, and initial status ("pending").
type createFileResp struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Status    string `json:"status"`
}

// createFileHandler handles POST /files to create a new file metadata record.
//
// Lifecycle: register metadata in DB with status "pending". The client must
// subsequently stream file data via POST /upload?id={uuid}.
//
// Request: JSON {orig_name, content_type, size_bytes, ttl_hours?}
// Response: JSON {id (UUID), object_key, status="pending"}
// Auth: required.
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

		// Get current user (returns user UUID from session)
		userID, err := cfg.Auth.getCurrentUser(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check user quota
		var quota sql.NullInt64
		var currentUsage int64
		err = db.QueryRow(`
			SELECT 
				u.storage_quota_bytes,
				COALESCE(SUM(f.size_bytes), 0) as current_usage
			FROM users u
			LEFT JOIN files f ON f.user_id = u.id AND f.status IN ('stored', 'hashed', 'ready')
			WHERE u.id = $1
			GROUP BY u.id, u.storage_quota_bytes
		`, userID).Scan(&quota, &currentUsage)

		if err != nil {
			log.Printf("create file: failed to get user info for %s: %v", userID, err)
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		// Enforce quota if set
		if quota.Valid && (currentUsage+req.SizeBytes) > quota.Int64 {
			http.Error(w, "storage quota exceeded", http.StatusForbidden)
			return
		}

		// Generate a unique UUID for the file record
		id := uuid.New()
		// Create a stable, non-guessable object key in MinIO.
		// Uses "uploads/" prefix + UUID to avoid path traversal attacks.
		objectKey := "uploads/" + id.String()

		// Calculate expiration time if TTL is provided
		var expiresAt sql.NullTime
		autoDelete := false
		if req.TTLHours > 0 {
			expiresAt = sql.NullTime{
				Time:  time.Now().UTC().Add(time.Duration(req.TTLHours) * time.Hour),
				Valid: true,
			}
			autoDelete = true
		}

		_, err = db.Exec(`
			INSERT INTO files (id, object_key, orig_name, content_type, size_bytes, user_id, status, expires_at, auto_delete)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, $8)
		`, id, objectKey, req.OrigName, req.ContentType, req.SizeBytes, userID, expiresAt, autoDelete)
		if err != nil {
			log.Printf("create file: failed to insert file record: %v", err)
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
