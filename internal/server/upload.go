package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type uploadResp struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Status    string `json:"status"`
}

func (cfg Config) uploadHandler(db *sql.DB, mc *minio.Client, bucket string) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if db == nil || mc == nil || bucket == "" {
			http.Error(w, "server not configured", http.StatusServiceUnavailable)
			return
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
		err = db.QueryRow(`SELECT object_key, status FROM files WHERE id = $1`, id).Scan(&objectKey, &status)
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
			defer part.Close()

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

		ctx, cancel := context.WithTimeout(r.Context(), 5*60*1000*1000*1000) // 5 minutes
		defer cancel()

		_, putErr := mc.PutObject(ctx, bucket, objectKey, filePart, -1, minio.PutObjectOptions{
			ContentType: contentType,
		})
		if putErr != nil {
			_, _ = db.Exec(`UPDATE files SET status = 'failed' WHERE id = $1 AND status = 'pending'`, id)
			http.Error(w, "upload failed", http.StatusBadGateway)
			return
		}

		_, err = db.Exec(`UPDATE files SET status = 'stored' WHERE id = $1 AND status = 'pending'`, id)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(uploadResp{
			ID:        id.String(),
			ObjectKey: objectKey,
			Status:    "stored",
		})
	}))
}
