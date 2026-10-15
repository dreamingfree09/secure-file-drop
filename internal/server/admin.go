package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// FileInfo is the public metadata returned by file listings.
type FileInfo struct {
	ID          string    `json:"id"`
	OrigName    string    `json:"orig_name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	Status      string    `json:"status"`
	SHA256Hex   string    `json:"sha256_hex,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func listFiles(w http.ResponseWriter, r *http.Request, db *sql.DB, all bool) {
	p := principal(r)
	rows, err := db.QueryContext(r.Context(), `SELECT id,orig_name,content_type,size_bytes,status,COALESCE(sha256_hex,''),created_at
 FROM files WHERE $1 OR user_id::text=$2 OR ($3 AND user_id IS NULL AND created_by=$2)
 ORDER BY created_at DESC LIMIT 100`, all, p.Sub, p.Admin)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()
	files := make([]FileInfo, 0)
	for rows.Next() {
		var f FileInfo
		if rows.Scan(&f.ID, &f.OrigName, &f.ContentType, &f.SizeBytes, &f.Status, &f.SHA256Hex, &f.CreatedAt) != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		files = append(files, f)
	}
	if rows.Err() != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(files)
}

// AdminListFilesHandler lists all files after route-level administrator checks.
func (s *Server) AdminListFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	listFiles(w, r, s.db, true)
}

// AdminDeleteFileHandler removes the exact stored object before its metadata.
func (s *Server) AdminDeleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/admin/files/")
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "bad file id", http.StatusBadRequest)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var key string
	err = tx.QueryRowContext(r.Context(), "SELECT object_key FROM files WHERE id=$1 FOR UPDATE", id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err = s.minio.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		http.Error(w, "storage error", http.StatusBadGateway)
		return
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM files WHERE id=$1", id); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	// #nosec G706 -- Log fields are quoted with %q; the ID is also validated as UUID.
	log.Printf("admin_delete id=%q", id)
	w.WriteHeader(http.StatusNoContent)
}

// CleanupResult reports committed cleanup deletions.
type CleanupResult struct {
	DeletedCount int `json:"deleted_count"`
}

// AdminManualCleanupHandler runs the same cleanup operation as the background job.
func (s *Server) AdminManualCleanupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := GetCleanupConfigFromEnv(s.db, s.minio, s.bucket)
	if !cfg.Enabled {
		http.Error(w, "cleanup disabled", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	n, err := runCleanup(ctx, cfg)
	if err != nil {
		http.Error(w, "cleanup failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CleanupResult{DeletedCount: n})
}
