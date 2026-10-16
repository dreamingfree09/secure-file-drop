package server

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
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
		if token == "" {
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
			objectKey   string
			status      string
			contentType string
			origName    string
			sizeBytes   int64
		)

		queryCtx, queryCancel := sqlContext(r.Context())
		err = db.QueryRowContext(queryCtx,
			`SELECT object_key, status, content_type, orig_name, size_bytes
			 FROM files
			 WHERE id = $1`,
			claims.FileID,
		).Scan(&objectKey, &status, &contentType, &origName, &sizeBytes)
		queryCancel()
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

		// Set a generous timeout for large file downloads (30 minutes for up to 50GB files)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()

		// Stream the object directly from MinIO (no memory buffering)
		obj, err := mc.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, "storage error", http.StatusBadGateway)
			return
		}
		defer func() { _ = obj.Close() }()

		// Force an early error for missing object / auth issues.
		stat, statErr := obj.Stat()
		if statErr != nil {
			http.Error(w, "storage error", http.StatusBadGateway)
			return
		}

		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		sizeBytes = stat.Size
		if sizeBytes >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(sizeBytes, 10))
		}

		// Encourage safe download behavior in browsers.
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": origName}))

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)

		n, copyErr := io.Copy(w, obj)
		if copyErr != nil {
			GetMetrics().RecordDownloadError()
			log.Printf("download interrupted rid=%s", RequestIDFromContext(r.Context()))
			return
		}
		GetMetrics().RecordDownload(n, 0)
	})
}
