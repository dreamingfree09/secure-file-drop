// downloadHandler serves GET /download requests using a signed token.
// The token encodes file id and expiry; optional password protection
// may be enforced. On success, it streams the object from MinIO.

package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
)

// downloadHandler handles GET /download?token={signed-token} requests for streaming
// file downloads from MinIO. It validates the signed download token (HMAC-SHA256),
// checks the file status (must be "hashed" or "ready"), and streams the file directly
// from MinIO to the client without buffering in memory.
//
// Required query parameter: token (HMAC-signed token with file ID and expiry)
// Response: Binary file stream with Content-Type, Content-Length, Content-Disposition headers
// Authentication: Not required (uses signed token for authorization)
func (cfg Config) downloadHandler(db *sql.DB, mc *minio.Client, bucket string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := r.URL.Query().Get("token")
		if token == "" || len(token) > 4096 {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}

		claims, err := verifyDownloadToken(token, time.Now().UTC())
		if err != nil {
			if errors.Is(err, errTokenExpired) {
				http.Error(w, "token expired", http.StatusGone)
				return
			}
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		var (
			objectKey    string
			status       string
			contentType  string
			origName     string
			sizeBytes    int64
			linkPassword sql.NullString
			expiresAt    sql.NullTime
			autoDelete   bool
		)

		err = db.QueryRow(
			`SELECT object_key, status, content_type, orig_name, size_bytes, link_password,expires_at,auto_delete
		 FROM files
		 WHERE id::text = $1`,
			claims.FileID,
		).Scan(&objectKey, &status, &contentType, &origName, &sizeBytes, &linkPassword, &expiresAt, &autoDelete)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		// Only allow downloads after file integrity has been verified via hashing.
		// Status must be "hashed" (hash complete) or "ready" (verified and approved).
		if status != "hashed" && status != "ready" {
			http.Error(w, "file not ready", http.StatusConflict)
			return
		}

		if autoDelete && expiresAt.Valid && !time.Now().Before(expiresAt.Time) {
			http.Error(w, "file expired", http.StatusGone)
			return
		}
		// Verify password if file is password-protected
		if linkPassword.Valid && linkPassword.String != "" {
			providedPassword := r.URL.Query().Get("password")
			provided := sha256.Sum256([]byte(providedPassword))
			expected := sha256.Sum256([]byte(linkPassword.String))
			if !hmac.Equal(provided[:], expected[:]) {
				http.Error(w, "incorrect password", http.StatusUnauthorized)
				return
			}
		}

		// Set a generous timeout for large file downloads (30 minutes for up to 50GB files)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()

		// Stream the object directly from MinIO (no memory buffering)
		obj, err := mc.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
		if err != nil {
			Warn("download stream interrupted", nil)
			http.Error(w, "storage unavailable", http.StatusBadGateway)
			return
		}

		defer func() { _ = obj.Close() }()
		info, statErr := obj.Stat()
		if statErr != nil {
			code := http.StatusBadGateway
			if minio.ToErrorResponse(statErr).Code == "NoSuchKey" {
				code = http.StatusNotFound
			}
			http.Error(w, "stored file unavailable", code)
			return
		}
		if info.Size != sizeBytes || sizeBytes < 0 {
			http.Error(w, "stored size mismatch", http.StatusBadGateway)
			return
		}
		if _, _, err := mime.ParseMediaType(contentType); err != nil {
			contentType = "application/octet-stream"
		}

		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		if sizeBytes >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(sizeBytes, 10))
		}

		// Encourage safe download behavior in browsers.
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": SanitizeFilename(origName)}))

		w.WriteHeader(http.StatusOK)

		n, copyErr := io.Copy(w, obj)
		if copyErr != nil || n != sizeBytes {
			Warn("download stream interrupted", nil)
			return
		}

		// Track download statistics and send notification (fire and forget)
		downloaderIP := getClientIP(r)
		go func() {
			statCtx, done := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer done()
			// Update download count
			_, _ = db.ExecContext(statCtx, `
				UPDATE files 
				SET download_count = download_count + 1,
				    last_downloaded_at = NOW()
				WHERE id = $1
			`, claims.FileID)

			// Send download notification to file owner
			var userEmail string
			err := db.QueryRowContext(statCtx, `
				SELECT u.email
				FROM files f
				JOIN users u ON u.id = f.user_id
				WHERE f.id = $1
			`, claims.FileID).Scan(&userEmail)

			if err == nil && cfg.EmailSvc != nil {
				_ = cfg.EmailSvc.SendFileDownloadNotification(userEmail, origName, downloaderIP)
			}
		}()
	})
}
