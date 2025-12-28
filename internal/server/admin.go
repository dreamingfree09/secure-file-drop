package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	minio "github.com/minio/minio-go/v7"
)

// FileInfo represents a file record for admin listing
type FileInfo struct {
	ID          string    `json:"id"`
	OrigName    string    `json:"orig_name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	Status      string    `json:"status"`
	SHA256Hex   string    `json:"sha256_hex,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AdminListFilesHandler returns all files for admin dashboard
func (s *Server) AdminListFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Query all files ordered by creation time (newest first)
	rows, err := s.db.Query(`
		SELECT id, orig_name, content_type, size_bytes, status, 
		       COALESCE(sha256_hex, '') as sha256_hex, created_at, updated_at
		FROM files 
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		log.Printf("admin list files: query failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var files []FileInfo
	for rows.Next() {
		var f FileInfo
		if err := rows.Scan(&f.ID, &f.OrigName, &f.ContentType, &f.SizeBytes,
			&f.Status, &f.SHA256Hex, &f.CreatedAt, &f.UpdatedAt); err != nil {
			log.Printf("admin list files: scan failed: %v", err)
			continue
		}
		files = append(files, f)
	}

	if err := rows.Err(); err != nil {
		log.Printf("admin list files: rows error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		log.Printf("admin list files: encode failed: %v", err)
	}
}

// AdminDeleteFileHandler deletes a specific file from both MinIO and database
func (s *Server) AdminDeleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract file ID from URL path
	// Expected: /admin/files/{id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/admin/files/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "File ID required", http.StatusBadRequest)
		return
	}
	fileID := parts[0]

	// Get file info before deletion (for MinIO cleanup)
	var status string
	err := s.db.QueryRow("SELECT status FROM files WHERE id = $1", fileID).Scan(&status)
	if err != nil {
		log.Printf("admin delete file: query failed: %v", err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Delete from MinIO if file was stored
	if status != "pending" {
		objectName := fileID
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := s.minio.RemoveObject(ctx, s.bucket, objectName, minio.RemoveObjectOptions{})
		if err != nil {
			log.Printf("admin delete file: MinIO removal failed: %v", err)
			// Continue with database deletion even if MinIO fails
		} else {
			log.Printf("admin delete file: removed from MinIO: %s", fileID)
		}
	}

	// Delete from database
	result, err := s.db.Exec("DELETE FROM files WHERE id = $1", fileID)
	if err != nil {
		log.Printf("admin delete file: db delete failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	log.Printf("admin delete file: deleted file %s", fileID)
	w.WriteHeader(http.StatusNoContent)
}

// CleanupResult represents the result of a cleanup operation
type CleanupResult struct {
	DeletedCount int `json:"deleted_count"`
}

// AdminManualCleanupHandler triggers a manual cleanup of old pending/failed files
func (s *Server) AdminManualCleanupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get cleanup config from environment
	cfg := GetCleanupConfigFromEnv(s.db, s.minio, s.bucket)

	if !cfg.Enabled {
		log.Printf("admin manual cleanup: cleanup is disabled in config")
		http.Error(w, "Cleanup is disabled", http.StatusServiceUnavailable)
		return
	}

	// Run cleanup synchronously for manual trigger
	deletedCount := 0
	cutoff := time.Now().Add(-cfg.MaxAge)

	log.Printf("admin manual cleanup: scanning for files older than %s", cutoff.Format(time.RFC3339))

	rows, err := s.db.Query(`
		SELECT id, status 
		FROM files 
		WHERE status IN ('pending', 'failed') 
		  AND created_at < $1
	`, cutoff)
	if err != nil {
		log.Printf("admin manual cleanup: query failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var toDelete []struct {
		ID     string
		Status string
	}

	for rows.Next() {
		var item struct {
			ID     string
			Status string
		}
		if err := rows.Scan(&item.ID, &item.Status); err != nil {
			log.Printf("admin manual cleanup: scan failed: %v", err)
			continue
		}
		toDelete = append(toDelete, item)
	}

	if err := rows.Err(); err != nil {
		log.Printf("admin manual cleanup: rows error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Delete each file
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, item := range toDelete {
		// Remove from MinIO if not pending (pending files were never uploaded)
		if item.Status != "pending" {
			err := s.minio.RemoveObject(ctx, s.bucket, item.ID, minio.RemoveObjectOptions{})
			if err != nil {
				log.Printf("admin manual cleanup: MinIO removal failed for %s: %v", item.ID, err)
			}
		}

		// Remove from database
		_, err := s.db.Exec("DELETE FROM files WHERE id = $1", item.ID)
		if err != nil {
			log.Printf("admin manual cleanup: db delete failed for %s: %v", item.ID, err)
			continue
		}

		deletedCount++
		log.Printf("admin manual cleanup: deleted file %s (status=%s, age > %s)",
			item.ID, item.Status, cfg.MaxAge)
	}

	log.Printf("admin manual cleanup: completed, deleted %d files", deletedCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CleanupResult{DeletedCount: deletedCount})
}
