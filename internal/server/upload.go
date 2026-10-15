package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type uploadResp struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Status    string `json:"status"`
}

func maxUploadBytes() (int64, error) {
	raw := os.Getenv("SFD_MAX_UPLOAD_BYTES")
	if raw == "" {
		return 50 << 30, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 || n > math.MaxInt64-(1<<20) {
		return 0, fmt.Errorf("invalid upload limit")
	}
	return n, nil
}
func (cfg Config) uploadHandler(db *sql.DB, mc *minio.Client, bucket string) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		start := time.Now()
		limit, err := maxUploadBytes()
		if err != nil {
			http.Error(w, "server misconfigured", http.StatusInternalServerError)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
		id, err := uuid.Parse(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		transactionCtx, transactionCancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer transactionCancel()
		tx, err := db.BeginTx(transactionCtx, nil)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback() }()
		p := principal(r)
		var key, status string
		err = tx.QueryRowContext(r.Context(), `SELECT object_key,status FROM files WHERE id=$1 AND ($2 OR user_id::text=$3) FOR UPDATE`, id, p.Admin, p.Sub).Scan(&key, &status)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
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
		var file io.Reader
		for {
			part, e := mr.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			if part.FormName() == "file" {
				file = part
				break
			}
			_ = part.Close()
		}
		if file == nil {
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		// Hold the row lock until storage and hashing finish. Competing uploads,
		// admin deletion and cleanup cannot act on an in-flight file.
		failed := true
		defer func() {
			if failed {
				cleanupCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
				defer done()
				// Keep metadata even if removal fails: cleanup can retry the exact key.
				if e := mc.RemoveObject(cleanupCtx, bucket, key, minio.RemoveObjectOptions{}); e != nil {
					// #nosec G706 -- Log fields are quoted with %q; the ID is also validated as UUID.
					log.Printf("upload cleanup failed id=%q", id)
				}
				// Remove while the transaction still holds the file lock. A
				// canceled HTTP request must not release it before cleanup.
				_, stateErr := tx.ExecContext(cleanupCtx, "UPDATE files SET status='failed' WHERE id=$1", id)
				if stateErr == nil {
					stateErr = tx.Commit()
				}
				if stateErr != nil {
					_ = tx.Rollback()
					if _, e := db.ExecContext(cleanupCtx, "UPDATE files SET status='failed' WHERE id=$1 AND status='pending'", id); e != nil {
						// #nosec G706 -- Validated UUID is quoted to prevent log control characters.
						log.Printf("upload failure state failed id=%q", id)
					}
				}
				GetMetrics().RecordUploadError()
				GetMetrics().RecordFileStateTransition("failed")
			}
		}()
		limited := &io.LimitedReader{R: file, N: limit + 1}
		info, err := mc.PutObject(ctx, bucket, key, limited, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
		var maxErr *http.MaxBytesError
		if limited.N == 0 || errors.As(err, &maxErr) {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			http.Error(w, "upload failed", http.StatusBadGateway)
			return
		}
		if _, err = tx.ExecContext(ctx, "UPDATE files SET status='stored',size_bytes=$2 WHERE id=$1", id, info.Size); err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		sha, _, count, err := sha256FromMinioObject(ctx, mc, bucket, key)
		if err != nil || count > math.MaxInt64 || int64(count) != info.Size {
			http.Error(w, "hashing failed", http.StatusBadGateway)
			return
		}
		if _, err = tx.ExecContext(ctx, "UPDATE files SET sha256_hex=$2,sha256_bytes=$3,status='hashed' WHERE id=$1", id, sha, count); err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		failed = false
		GetMetrics().RecordUpload(info.Size, time.Since(start))
		GetMetrics().RecordFileStateTransition("hashed")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(uploadResp{id.String(), key, "hashed"})
	}))
}
