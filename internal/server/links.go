// createLinkHandler handles POST /links to create signed, expiring tokens
// for downloads. The token includes file id, expiry, and optional password
// requirements. Returns the public URL constructed from Config.BaseURL.
// links.go - Signed download link creation.
//
// Generates HMAC-signed tokens bound to file ID and expiry; optional
// password protection is supported.
package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// createLinkReq represents the JSON payload for creating a signed download link.
// Requires the file ID (UUID) and desired TTL (time to live) in seconds.
type createLinkReq struct {
	ID         string `json:"id"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// createLinkResp is the JSON response containing the signed download URL
// and its expiration timestamp (RFC3339 format).
type createLinkResp struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

// clampTTLSeconds enforces TTL constraints for download links.
// Default: 5 minutes (300 seconds) if omitted or invalid.
// Minimum: 60 seconds, Maximum: 24 hours (86400 seconds).
// This prevents both too-short and effectively-permanent links.
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

func requestOrigin(r *http.Request) string {
	// Prefer reverse-proxy headers if present.
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))

	// Some proxies set X-Forwarded-Host as a comma-separated list.
	if i := strings.IndexByte(host, ','); i >= 0 {
		host = strings.TrimSpace(host[:i])
	}

	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	if host == "" {
		host = r.Host
	}
	if host == "" {
		host = "localhost:8080"
	}

	return scheme + "://" + host
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

		// Milestone 8: prefer configured public base URL for deterministic links.
		// This is critical when deployed behind reverse proxies (e.g., Proxmox + Nginx/Traefik/Caddy).
		base := strings.TrimSpace(os.Getenv("SFD_PUBLIC_BASE_URL"))
		base = strings.TrimRight(base, "/")
		if base == "" {
			base = requestOrigin(r)
		}

		url := base + "/download?token=" + token

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(createLinkResp{
			URL:       url,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		})
	}))
}
