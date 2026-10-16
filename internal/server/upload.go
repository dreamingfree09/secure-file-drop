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

// maxRequestBytes makes the existing 1 MiB multipart allowance explicit for proxies.
func maxRequestBytes() (int64, error) {
	file, err := maxUploadBytes()
	if err != nil {
		return 0, err
	}
	want := file + (1 << 20)
	if raw := os.Getenv("SFD_MAX_REQUEST_BYTES"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n != want {
			return 0, fmt.Errorf("request limit must equal file limit plus 1 MiB")
		}
	}
	return want, nil
}

// multipartRead records errors even when a storage SDK treats UnexpectedEOF as EOF.
type multipartRead struct {
	io.Reader
	err error
}

func (r *multipartRead) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

// On a lost COMMIT acknowledgement, reacquire the row before compensation.
// Hashed/ready or unknown outcomes always retain their bytes. Only a locked,
// observed pending row proves that the original hashed transition rolled back.
func reconcileUploadFailure(ctx context.Context, db *sql.DB, id uuid.UUID, remove func() error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM files WHERE id=$1 FOR UPDATE", id).Scan(&status); err != nil {
		return err
	}
	if status != "pending" {
		return nil
	}
	if err = remove(); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE files SET status='failed' WHERE id=$1 AND status='pending'", id); err != nil {
		return err
	}
	return tx.Commit()
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(5 * time.Minute))
		defer func() { _ = http.NewResponseController(w).SetReadDeadline(time.Time{}) }()
		envelope, err := maxRequestBytes()
		if err != nil {
			http.Error(w, "server misconfigured", http.StatusInternalServerError)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, envelope)
		id, err := uuid.Parse(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		transactionCtx, transactionCancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer transactionCancel()
		acquireCtx, acquireCancel := sqlContext(ctx)
		conn, err := db.Conn(acquireCtx)
		acquireCancel()
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = conn.Close() }()
		tx, err := conn.BeginTx(transactionCtx, nil)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback() }()
		p := principal(r)
		var key, status string
		queryCtx, queryCancel := sqlContext(ctx)
		err = tx.QueryRowContext(queryCtx, `SELECT object_key,status FROM files WHERE id=$1 AND ($2 OR user_id::text=$3) FOR UPDATE`, id, p.Admin, p.Sub).Scan(&key, &status)
		queryCancel()
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
		// Hold the row lock until storage and hashing finish. Competing uploads,
		// admin deletion and cleanup cannot act on an in-flight file.
		failed := true
		commitAttempted := false
		defer func() {
			if failed {
				cleanupCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
				defer done()
				if commitAttempted {
					_ = tx.Rollback()
					// Release the borrowed connection before acquiring another (small pools).
					_ = conn.Close()
					e := reconcileUploadFailure(cleanupCtx, db, id, func() error {
						return mc.RemoveObject(cleanupCtx, bucket, key, minio.RemoveObjectOptions{})
					})
					if e != nil {
						// #nosec G706 -- Validated UUID is quoted; no raw request text is logged.
						log.Printf("upload commit outcome unresolved; bytes retained id=%q", id)
					}
					GetMetrics().RecordUploadError()
					return
				}
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
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}
		part, err := mr.NextPart()
		if err != nil || part.FormName() != "file" || part.FileName() == "" {
			http.Error(w, "expected one file part", http.StatusBadRequest)
			return
		}
		file := &multipartRead{Reader: part}
		limited := &io.LimitedReader{R: file, N: limit + 1}
		info, err := mc.PutObject(ctx, bucket, key, limited, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
		if err == nil {
			// Independently finish the reader even if an SDK stops consuming early.
			_, err = io.Copy(io.Discard, limited)
		}
		var maxErr *http.MaxBytesError
		if limited.N == 0 || errors.As(err, &maxErr) {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		if file.err != nil {
			code := http.StatusBadRequest
			if errors.As(file.err, &maxErr) {
				code = http.StatusRequestEntityTooLarge
			}
			http.Error(w, "bad multipart", code)
			return
		}
		if err != nil {
			http.Error(w, "upload failed", http.StatusBadGateway)
			return
		}
		// Exactly one file part. A complete closing boundary is mandatory.
		// MIME epilogue is permitted, but counts against the envelope limit.
		_, framingErr := mr.NextPart()
		if framingErr != io.EOF {
			code := http.StatusBadRequest
			if errors.As(framingErr, &maxErr) {
				code = http.StatusRequestEntityTooLarge
			}
			http.Error(w, "bad multipart", code)
			return
		}
		if _, err = io.Copy(io.Discard, r.Body); err != nil {
			var large *http.MaxBytesError
			code := http.StatusBadRequest
			if errors.As(err, &large) {
				code = http.StatusRequestEntityTooLarge
			}
			http.Error(w, "bad multipart", code)
			return
		}
		if info.Size != limit+1-limited.N {
			http.Error(w, "upload byte count mismatch", http.StatusBadGateway)
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
		commitAttempted = true
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
