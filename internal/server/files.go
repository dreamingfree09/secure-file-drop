package server

import (
	"database/sql"
	"encoding/json"
	"mime"
	"net/http"
	"path"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

type createFileReq struct {
	OrigName    string `json:"orig_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}
type createFileResp struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Status    string `json:"status"`
}

func safeFilename(name string) (string, bool) {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "" || name == "." || name == ".." || len(name) > 255 {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return name, true
}
func (cfg Config) createFileHandler(db *sql.DB) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			listFiles(w, r, db, false)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var req createFileReq
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var ok bool
		req.OrigName, ok = safeFilename(req.OrigName)
		req.ContentType = strings.TrimSpace(req.ContentType)
		if _, _, err := mime.ParseMediaType(req.ContentType); !ok || err != nil || req.SizeBytes < 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		limit, err := maxUploadBytes()
		if err != nil {
			http.Error(w, "server misconfigured", http.StatusInternalServerError)
			return
		}
		if limit > 0 && req.SizeBytes > limit {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		id := uuid.New()
		key := "uploads/" + id.String()
		p := principal(r)
		var owner any
		if !p.Admin {
			owner = p.Sub
		}
		_, err = db.ExecContext(r.Context(), `INSERT INTO files (id,object_key,orig_name,content_type,size_bytes,created_by,user_id,status)
   VALUES ($1,$2,$3,$4,$5,$6,$7,'pending')`, id, key, req.OrigName, req.ContentType, req.SizeBytes, p.Sub, owner)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		GetMetrics().RecordFileStateTransition("pending")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createFileResp{ID: id.String(), ObjectKey: key, Status: "pending"})
	}))
}
