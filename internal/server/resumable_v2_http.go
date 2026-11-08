package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func writeR2Error(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "upload not found", http.StatusNotFound)
	case errors.Is(err, errR2Quota):
		http.Error(w, "storage quota exceeded", http.StatusForbidden)
	case errors.Is(err, errR2Expired):
		http.Error(w, "upload session expired", http.StatusGone)
	case errors.Is(err, errR2Conflict):
		http.Error(w, "upload state, offset or checksum conflict", http.StatusConflict)
	case errors.Is(err, errR2Capacity):
		http.Error(w, "resumable storage reservation is full", http.StatusInsufficientStorage)
	case errors.Is(err, errR2Rate):
		http.Error(w, "upload creation rate exceeded", http.StatusTooManyRequests)
	default:
		http.Error(w, "upload temporarily unavailable; inspect committed progress before retrying", http.StatusServiceUnavailable)
	}
}

func writeR2JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *r2Service) collection(w http.ResponseWriter, r *http.Request, owner string) {
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	if r.Method == http.MethodGet {
		rows, err := s.db.QueryContext(ctx, "SELECT "+r2Fields+" FROM r2_uploads WHERE owner_id=$1 AND state NOT IN ('canceled','complete') ORDER BY created_at DESC LIMIT 20", owner)
		if err != nil {
			writeR2Error(w, err)
			return
		}
		defer func() { _ = rows.Close() }()
		uploads := []r2Upload{}
		for rows.Next() {
			u, err := scanR2Upload(rows)
			if err != nil {
				writeR2Error(w, err)
				return
			}
			uploads = append(uploads, u)
		}
		if err := rows.Err(); err != nil {
			writeR2Error(w, err)
			return
		}
		writeR2JSON(w, http.StatusOK, uploads)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		ID          string `json:"request_id"`
		Name        string `json:"name"`
		ContentType string `json:"content_type"`
		Total       int64  `json:"size_bytes"`
		Manifest    string `json:"manifest_sha256"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	id, err := uuid.Parse(request.ID)
	name, valid := safeFilename(request.Name)
	contentType := strings.TrimSpace(request.ContentType)
	if _, _, mediaErr := mime.ParseMediaType(contentType); err != nil || id == uuid.Nil || !valid || !r2CanonicalDigest(request.Manifest) ||
		len(contentType) > 255 || mediaErr != nil || ValidateUploadMimeType(name, contentType) != nil {
		http.Error(w, "invalid file name, type or request ID", http.StatusBadRequest)
		return
	}
	if _, err := r2PartCount(request.Total, s.config.MaxFileBytes); err != nil {
		http.Error(w, "resumable file size exceeds configured limit", http.StatusRequestEntityTooLarge)
		return
	}
	u, err := s.reserve(ctx, owner, id, name, contentType, request.Manifest, request.Total)
	if err != nil {
		writeR2Error(w, err)
		return
	}
	s.signal()
	writeR2JSON(w, http.StatusCreated, u)
}

func (s *r2Service) inspect(w http.ResponseWriter, r *http.Request, owner string) {
	ctx, cancel := sqlContext(r.Context())
	defer cancel()
	u, err := r2Find(ctx, s.db, r.PathValue("id"), owner)
	if err != nil {
		writeR2Error(w, err)
		return
	}
	if r.URL.Query().Get("parts") != "1" {
		writeR2JSON(w, http.StatusOK, u)
		return
	}
	// The offset and digest list come from one committed snapshot boundary:
	// the query ignores rows starting at or beyond the already observed offset.
	rows, err := s.db.QueryContext(ctx, "SELECT digest FROM r2_parts WHERE upload=$1 AND etag IS NOT NULL AND start_byte<$2 ORDER BY number", u.ID, u.Offset)
	if err != nil {
		writeR2Error(w, err)
		return
	}
	defer func() { _ = rows.Close() }()
	digests := []string{}
	for rows.Next() {
		var digest string
		if err := rows.Scan(&digest); err != nil {
			writeR2Error(w, err)
			return
		}
		digests = append(digests, digest)
	}
	if err := rows.Err(); err != nil {
		writeR2Error(w, err)
		return
	}
	writeR2JSON(w, http.StatusOK, struct {
		r2Upload
		Parts []string `json:"part_sha256"`
	}{u, digests})
}

func (s *r2Service) patchPart(w http.ResponseWriter, r *http.Request, owner string) {
	offsetText, digest := r.Header.Get("Upload-Offset"), r.Header.Get("Upload-SHA256")
	offset, err := strconv.ParseInt(offsetText, 10, 64)
	if r.ContentLength < 1 {
		http.Error(w, "exact Content-Length required", http.StatusLengthRequired)
		return
	}
	if r.ContentLength > r2ChunkBytes {
		http.Error(w, "part exceeds 32 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil || strconv.FormatInt(offset, 10) != offsetText || len(digest) != 64 ||
		len(r.Header.Values("Upload-Offset")) != 1 || len(r.Header.Values("Upload-SHA256")) != 1 ||
		r.Header.Get("Content-Type") != "application/octet-stream" || r.Header.Get("Content-Encoding") != "" {
		http.Error(w, "invalid part headers", http.StatusBadRequest)
		return
	}
	query, endQuery := sqlContext(r.Context())
	u, err := r2Find(query, s.db, r.PathValue("id"), owner)
	endQuery()
	if err != nil {
		writeR2Error(w, err)
		return
	}
	part, err := r2PlanPart(u.Total, offset, r.ContentLength, r2MaximumBytes)
	if err != nil || offset > u.Offset || (u.State != "open" && u.State != "finalizing" && u.State != "complete") {
		writeR2Error(w, errR2Conflict)
		return
	}
	var buffer []byte
	select {
	case buffer = <-s.slots:
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "upload queue busy", http.StatusTooManyRequests)
		return
	}
	if buffer == nil {
		buffer = make([]byte, r2ChunkBytes)
	}
	defer func() {
		clear(buffer)
		s.slots <- buffer
	}()
	r.Body = http.MaxBytesReader(w, r.Body, part.Length+1)
	data := buffer[:part.Length]
	_, err = io.ReadFull(r.Body, data)
	var extra [1]byte
	_, tailErr := io.ReadFull(r.Body, extra[:])
	if err != nil || tailErr != io.EOF || r2ValidatePartDigest(digest, sha256.Sum256(data)) != nil {
		http.Error(w, "part length or SHA-256 did not match", http.StatusUnprocessableEntity)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if _, err := s.recordPartIntent(ctx, u.ID, owner, digest, part); err != nil {
		writeR2Error(w, err)
		return
	}
	u, err = s.writePart(ctx, u.ID, owner, digest, part, data)
	if err != nil {
		writeR2Error(w, err)
		return
	}
	writeR2JSON(w, http.StatusOK, u)
}

func (s *r2Service) endpoints(mux *http.ServeMux, auth AuthConfig) {
	bind := func(pattern string, f func(http.ResponseWriter, *http.Request, string)) {
		mux.Handle(pattern, auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			owner, err := auth.getCurrentUser(r)
			if err != nil {
				writeAuthError(w, err)
				return
			}
			if id := r.PathValue("id"); id != "" {
				if _, err := uuid.Parse(id); err != nil {
					writeR2Error(w, sql.ErrNoRows)
					return
				}
			}
			f(w, r, owner)
		})))
	}
	change := func(target string) func(http.ResponseWriter, *http.Request, string) {
		return func(w http.ResponseWriter, r *http.Request, owner string) {
			ctx, cancel := sqlContext(r.Context())
			defer cancel()
			u, err := s.transition(ctx, r.PathValue("id"), owner, target)
			if err != nil {
				writeR2Error(w, err)
				return
			}
			s.signal()
			writeR2JSON(w, http.StatusAccepted, u)
		}
	}
	bind("/uploads/resumable", s.collection)
	bind("GET /uploads/resumable/{id}", s.inspect)
	bind("PATCH /uploads/resumable/{id}", s.patchPart)
	bind("POST /uploads/resumable/{id}/complete", change("finalizing"))
	bind("DELETE /uploads/resumable/{id}", change("canceling"))
}
