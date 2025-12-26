package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type createLinkReq struct {
	ID         string `json:"id"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type createLinkResp struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

func clampTTLSeconds(n int) int {
	// MVP defaults: safe and simple.
	// If omitted/invalid, default to 5 minutes.
	// Hard cap at 24h to avoid “effectively permanent” links.
	if n <= 0 {
		return 300
	}
	if n > 86400 {
		return 86400
	}
	return n
}

func (cfg Config) createLinkHandler(db *sql.DB) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req createLinkReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		req.ID = strings.TrimSpace(req.ID)
		if req.ID == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		id, err := uuid.Parse(req.ID)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}

		// Ensure file exists and is in a state we allow for downloads.
		// For now: require "hashed" (Milestone 5) so integrity is proven.
		var status string
		err = db.QueryRow(`SELECT status FROM files WHERE id = $1`, id).Scan(&status)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if status != "hashed" && status != "ready" {
			http.Error(w, "invalid status", http.StatusConflict)
			return
		}

		ttl := clampTTLSeconds(req.TTLSeconds)
		expiresAt := time.Now().UTC().Add(time.Duration(ttl) * time.Second)

		token, err := signDownloadToken(id.String(), expiresAt)
		if err != nil {
			// If secret missing/misconfigured, this is a server error.
			if err == errDownloadSecretMissing {
				http.Error(w, "server misconfigured", http.StatusInternalServerError)
				return
			}
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}

		// Build an absolute-ish URL based on request host.
		// In local dev this will be localhost:8080; behind proxy it will be the proxy host.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		host := r.Host
		if host == "" {
			host = "localhost:8080"
		}

		// Keep it simple: download endpoint will be /download?token=...
		url := scheme + "://" + host + "/download?token=" + token

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(createLinkResp{
			URL:       url,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		})

		_ = strconv.ErrRange // quiet unused import guard if future edits remove strconv usage
	}))
}
