// admin.go - Admin and user file listing and deletion endpoints.
//
// Provides JSON APIs for admins to list/delete files and for users
// to view and remove their own uploads with ownership checks.

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

// FileInfo represents a file record for admin and user listings.
type FileInfo struct {
	ID               string     `json:"id"`
	OrigName         string     `json:"orig_name"`
	ContentType      string     `json:"content_type"`
	SizeBytes        int64      `json:"size_bytes"`
	Status           string     `json:"status"`
	SHA256Hex        string     `json:"sha256_hex,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	DownloadCount    int        `json:"download_count"`
	LastDownloadedAt *time.Time `json:"last_downloaded_at,omitempty"`
}

// AdminListFilesHandler returns a page of files for the admin dashboard.
// Results are ordered by creation time (newest first) and limited to 100.
func (s *Server) AdminListFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Query all files ordered by creation time (newest first)
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, orig_name, content_type, size_bytes, status,
		       COALESCE(sha256_hex, '') as sha256_hex, created_at,
		       download_count, last_downloaded_at
		FROM files
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		log.Printf("admin list files: query failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer closeRows(rows)

	var files []FileInfo
	for rows.Next() {
		var f FileInfo
		if err := rows.Scan(&f.ID, &f.OrigName, &f.ContentType, &f.SizeBytes,
			&f.Status, &f.SHA256Hex, &f.CreatedAt, &f.DownloadCount, &f.LastDownloadedAt); err != nil {
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

// UserFilesHandler returns files uploaded by the current authenticated user.
// Results are ordered by creation time (newest first) and limited to 100.
func (s *Server) UserFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current user from session (returns user UUID)
	userID, err := s.authCfg.getCurrentUser(r)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	// Query files created by this user (using user_id UUID, not created_by text)
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, orig_name, content_type, size_bytes, status,
		       COALESCE(sha256_hex, '') as sha256_hex, created_at,
		       download_count, last_downloaded_at
		FROM files
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		log.Printf("user files: query failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer closeRows(rows)

	var files []FileInfo
	for rows.Next() {
		var f FileInfo
		if err := rows.Scan(&f.ID, &f.OrigName, &f.ContentType, &f.SizeBytes,
			&f.Status, &f.SHA256Hex, &f.CreatedAt, &f.DownloadCount, &f.LastDownloadedAt); err != nil {
			log.Printf("user files: scan failed: %v", err)
			continue
		}
		files = append(files, f)
	}

	if err := rows.Err(); err != nil {
		log.Printf("user files: rows error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		log.Printf("user files: encode failed: %v", err)
	}
}

// UserDeleteFileHandler allows a user to delete their own file
// Route: DELETE /user/files/{id}
// Auth: required; verifies ownership via user_id
func (s *Server) UserDeleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, err := s.authCfg.getCurrentUser(r)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	s.deleteFile(w, r, strings.TrimPrefix(r.URL.Path, "/user/files/"), userID, false)
}

// AdminDeleteFileHandler removes the actual object before its metadata.
func (s *Server) AdminDeleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.deleteFile(w, r, strings.TrimPrefix(r.URL.Path, "/admin/files/"), "", true)
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request, id, owner string, admin bool) {
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "bad file id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var key, name, address string
	err = tx.QueryRowContext(ctx, `SELECT f.object_key,f.orig_name,COALESCE(u.email,'') FROM files f LEFT JOIN users u ON u.id=f.user_id
 WHERE f.id=$1 AND ($2 OR f.user_id::text=$3) FOR UPDATE OF f`, id, admin, owner).Scan(&key, &name, &address)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
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
	if s.emailSvc != nil && address != "" {
		reason := "manual deletion by user"
		if admin {
			reason = "manual deletion by admin"
		}
		go func() { _ = s.emailSvc.SendFileDeletedNotification(address, name, reason) }()
	}
	w.WriteHeader(http.StatusNoContent)
}

// CleanupResult represents the result of a manual cleanup operation.
type CleanupResult struct {
	DeletedCount int `json:"deleted_count"`
}

// AdminManualCleanupHandler triggers a manual cleanup of old pending/failed files.
// The operation is synchronous and returns the number of files deleted.
func (s *Server) AdminManualCleanupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := GetCleanupConfigFromEnv(s.db, s.minio, s.bucket)
	if !cfg.Enabled {
		http.Error(w, "cleanup disabled", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	n, err := cleanupFiles(ctx, cfg, false)
	if err != nil {
		http.Error(w, "cleanup failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(CleanupResult{DeletedCount: n}); err != nil {
		Warn("cleanup response write failed", nil)
	}
}
