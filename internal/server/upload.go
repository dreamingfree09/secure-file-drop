package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// uploadResp is the JSON response returned after a successful file upload.
// It contains the file ID, the MinIO object key, and the updated status.
type uploadResp struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Status    string `json:"status"`
}

// maxUploadBytes reads the SFD_MAX_UPLOAD_BYTES environment variable and
// returns the maximum allowed upload size in bytes. Returns 0 if not set
// (meaning no limit). Returns an error if the value cannot be parsed.
func maxUploadBytes() (int64, error) {
	raw := os.Getenv("SFD_MAX_UPLOAD_BYTES")
	if raw == "" {
		return 0, nil // no limit configured
	}
	return strconv.ParseInt(raw, 10, 64)
}

// uploadHandler handles POST /upload?id={uuid} requests for streaming file uploads to MinIO.
// It validates the file ID exists in the database with status "pending", reads the multipart
// form data, streams it directly to MinIO, then updates the database status to "stored".
// After storage, it triggers asynchronous hashing of the file via the native C utility.
//
// Required query parameter: id (UUID of file record created via /files)
// Required form field: file (the binary file data)
// Authentication: Required (checked by requireAuth middleware)
func (cfg Config) uploadHandler(db *sql.DB, mc *minio.Client, bucket string) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		limit, err := maxUploadBytes()
		if err != nil {
			http.Error(w, "server misconfigured", http.StatusInternalServerError)
			return
		}
		if limit > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}

		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		id, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}

		var objectKey string
		var status string
		err = db.QueryRow(
			`SELECT object_key, status FROM files WHERE id = $1`,
			id,
		).Scan(&objectKey, &status)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		if status != "pending" {
			http.Error(w, "invalid status", http.StatusConflict)
			return
		}

		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}

		var filePart io.Reader
		var contentType string

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			defer func() { _ = part.Close() }()

			if part.FormName() != "file" {
				continue
			}

			filePart = part
			contentType = part.Header.Get("Content-Type")
			break
		}

		if filePart == nil {
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		_, err = mc.PutObject(
			ctx,
			bucket,
			objectKey,
			filePart,
			-1,
			minio.PutObjectOptions{ContentType: contentType},
		)
		if err != nil {
			// Mark the file as failed in case of storage errors.
			_, _ = db.Exec(
				`UPDATE files SET status = 'failed' WHERE id = $1 AND status = 'pending'`,
				id,
			)

			rid := RequestIDFromContext(r.Context())
			log.Printf("rid=%s msg=putobject err=%v", rid, err)

			// If MaxBytesReader tripped, surface 413.
			if r.Body != nil {
				if _, ok := err.(*http.MaxBytesError); ok {
					http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
					return
				}
			}

			http.Error(w, "upload failed", http.StatusBadGateway)
			return
		}

		_, err = db.Exec(
			`UPDATE files SET status = 'stored' WHERE id = $1 AND status = 'pending'`,
			id,
		)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		shaHex, _, hashBytes, herr := sha256FromMinioObject(ctx, mc, bucket, objectKey)
		if herr != nil {
			_, _ = db.Exec(
				`UPDATE files SET status = 'failed' WHERE id = $1 AND status = 'stored'`,
				id,
			)
			rid := RequestIDFromContext(r.Context())
			log.Printf("rid=%s msg=hashing_failed err=%v", rid, herr)
			http.Error(w, "hashing failed", http.StatusBadGateway)
			return
		}

		_, err = db.Exec(
			`UPDATE files SET sha256_hex = $2, sha256_bytes = $3, status = 'hashed' WHERE id = $1 AND status = 'stored'`,
			id,
			shaHex,
			hashBytes,
		)
		if err != nil {
			rid := RequestIDFromContext(r.Context())
			log.Printf("rid=%s msg=db_update_hash err=%v", rid, err)
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(uploadResp{
			ID:        id.String(),
			ObjectKey: objectKey,
			Status:    "hashed",
		})
	}))
}
