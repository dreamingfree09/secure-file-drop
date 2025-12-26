package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type createFileReq struct {
	OrigName    string `json:"orig_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type createFileResp struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Status    string `json:"status"`
}

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

		req.OrigName = strings.TrimSpace(req.OrigName)
		req.ContentType = strings.TrimSpace(req.ContentType)

		if req.OrigName == "" || req.ContentType == "" || req.SizeBytes < 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		id := uuid.New()
		// Stable, non-guessable object key format (no user-supplied path components).
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
