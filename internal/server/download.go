package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
)

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

		err = db.QueryRow(
			`SELECT object_key, status, content_type, orig_name, size_bytes
			 FROM files
			 WHERE id = $1`,
			claims.FileID,
		).Scan(&objectKey, &status, &contentType, &origName, &sizeBytes)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		// For Milestone 6 we only allow downloads once hashing is done.
		if status != "hashed" && status != "ready" {
			http.Error(w, "file not ready", http.StatusConflict)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		obj, err := mc.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, "storage error", http.StatusBadGateway)
			return
		}
		defer func() { _ = obj.Close() }()

		// Force an early error for missing object / auth issues.
		if _, statErr := obj.Stat(); statErr != nil {
			http.Error(w, "storage error", http.StatusBadGateway)
			return
		}

		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		if sizeBytes > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(sizeBytes, 10))
		}

		// Encourage safe download behavior in browsers.
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, origName))

		w.WriteHeader(http.StatusOK)

		_, _ = io.Copy(w, obj)
	})
}
