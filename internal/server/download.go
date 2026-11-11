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
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

type downloadRange struct{ first, last int64 }

func parseDownloadRange(value string, size int64) (downloadRange, bool) {
	offsets, supported := strings.CutPrefix(value, "bytes=")
	start, end, separated := strings.Cut(offsets, "-")
	if !supported || !separated || size <= 0 || strings.HasPrefix(start, "+") || strings.HasPrefix(end, "+") {
		return downloadRange{}, false
	}
	if start == "" {
		count, err := strconv.ParseInt(end, 10, 64)
		if err != nil || count <= 0 {
			return downloadRange{}, false
		}
		return downloadRange{max(0, size-count), size - 1}, true
	}
	first, err := strconv.ParseInt(start, 10, 64)
	if err != nil || first < 0 || first >= size {
		return downloadRange{}, false
	}
	if end == "" {
		return downloadRange{first, size - 1}, true
	}
	last, err := strconv.ParseInt(end, 10, 64)
	if err != nil || last < first {
		return downloadRange{}, false
	}
	return downloadRange{first, min(last, size-1)}, true
}

func downloadETag(raw string) string {
	if raw == "" || len(raw) > 128 {
		return ""
	}
	for _, c := range raw {
		if c <= 0x20 || c > 0x7e || c == '"' || c == '\\' {
			return ""
		}
	}
	return "\"" + raw + "\""
}

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

		w.Header().Set("Cache-Control", "private, no-store")
		claims, err := verifyDownloadToken(token, time.Now().UTC())
		if err != nil {
			if errors.Is(err, errTokenExpired) {
				http.Error(w, "token expired", http.StatusGone)
				return
			}
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// Legacy tokens have no LinkID and retain their original expiry.
		// Managed tokens require the non-revoked durable ledger on every
		// new full or ranged download, before private object-store access.
		if claims.LinkID != "" {
			if !cfg.ManagedLinks {
				http.Error(w, "managed link unavailable", http.StatusUnauthorized)
				return
			}
			checkCtx, finishCheck := sqlContext(r.Context())
			active, checkErr := managedLinkAllowed(checkCtx, db, claims, token)
			finishCheck()
			if checkErr != nil {
				http.Error(w, "managed link verification unavailable", http.StatusServiceUnavailable)
				return
			}
			if !active {
				http.Error(w, "link revoked or expired", http.StatusGone)
				return
			}
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

		queryCtx, queryCancel := sqlContext(r.Context())
		defer queryCancel()
		err = db.QueryRowContext(queryCtx,
			`SELECT object_key, status, content_type, orig_name, size_bytes, link_password,expires_at,COALESCE(auto_delete,FALSE)
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

		// Count one outcome only after an authorized transfer is attempted.
		started := time.Now()
		completed := false
		defer func() {
			if !completed {
				GetMetrics().RecordDownloadError()
			}
		}()
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
		var source io.Reader = obj
		length, responseStatus := sizeBytes, http.StatusOK
		tag := downloadETag(info.ETag)
		if tag != "" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("ETag", tag)
		}
		rangeHeader := r.Header.Get("Range")
		ifRange := r.Header.Get("If-Range")
		// Ignore unsupported units, multiple ranges and stale validators.
		if tag != "" && (ifRange == "" || ifRange == tag) && len(r.Header.Values("Range")) == 1 &&
			strings.HasPrefix(rangeHeader, "bytes=") && !strings.Contains(rangeHeader, ",") {
			part, ok := parseDownloadRange(rangeHeader, sizeBytes)
			if !ok {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", sizeBytes))
				w.Header().Set("Cache-Control", "private, no-store")
				http.Error(w, "requested range unavailable", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			opts := minio.GetObjectOptions{}
			if opts.SetRange(part.first, part.last) != nil || opts.SetMatchETag(info.ETag) != nil {
				http.Error(w, "storage unavailable", http.StatusBadGateway)
				return
			}
			body, partInfo, headers, err := (minio.Core{Client: mc}).GetObject(ctx, bucket, objectKey, opts)
			if err != nil {
				http.Error(w, "stored range unavailable", http.StatusBadGateway)
				return
			}
			defer func() { _ = body.Close() }()
			length = part.last - part.first + 1
			contentRange := fmt.Sprintf("bytes %d-%d/%d", part.first, part.last, sizeBytes)
			if headers.Get("Content-Range") != contentRange || partInfo.Size != length || partInfo.ETag != info.ETag {
				http.Error(w, "stored range mismatch", http.StatusBadGateway)
				return
			}
			source, responseStatus = body, http.StatusPartialContent
			w.Header().Set("Content-Range", contentRange)
		}
		if _, _, err := mime.ParseMediaType(contentType); err != nil {
			contentType = "application/octet-stream"
		}

		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))

		// Encourage safe download behavior in browsers.
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": SanitizeFilename(origName)}))

		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(responseStatus)

		n, copyErr := io.Copy(w, source)
		if copyErr != nil || n != length {
			Warn("download stream interrupted", nil)
			return
		}

		completed = true
		GetMetrics().RecordDownload(n, time.Since(started))
		if responseStatus == http.StatusPartialContent {
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
