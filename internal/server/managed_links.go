package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// The PostgreSQL ledger stores a one-way signed-bearer digest, not the token.
func managedLinkDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Revocation is checked before S3 access on each new full/ranged request.
// Already streaming responses cannot be interrupted by revocation.
func managedLinkAllowed(ctx context.Context, db *sql.DB, claims downloadClaims, token string) (bool, error) {
	if db == nil || claims.LinkID == "" || claims.FileID == "" || token == "" {
		return false, errors.New("managed link cannot be verified")
	}
	var valid bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM managed_share_links
		WHERE id=$1 AND file_id::text=$2 AND token_sha256=$3
		  AND revoked_at IS NULL AND expires_at>now())`,
		claims.LinkID, claims.FileID, managedLinkDigest(token)).Scan(&valid)
	return valid, err
}

type managedLinkPublic struct {
	ID        string     `json:"id"`
	FileID    string     `json:"file_id"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func (cfg Config) listManagedLinksHandler(db *sql.DB) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		owner, err := cfg.Auth.getCurrentUser(r)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		fileText := r.URL.Query().Get("file_id")
		query := `SELECT id::text,file_id::text,created_at,expires_at,revoked_at
			FROM managed_share_links WHERE owner_id=$1::uuid`
		args := []any{owner}
		if fileText != "" {
			fileID, parseErr := uuid.Parse(fileText)
			if parseErr != nil || fileID == uuid.Nil || fileID.String() != fileText {
				http.Error(w, "invalid file id", http.StatusBadRequest)
				return
			}
			query += " AND file_id=$2"
			args = append(args, fileID)
		}
		query += " ORDER BY created_at DESC,id LIMIT 50"
		ctx, cancel := sqlContext(r.Context())
		defer cancel()
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			http.Error(w, "managed sharing unavailable", http.StatusServiceUnavailable)
			return
		}
		defer func() { _ = rows.Close() }()
		items := make([]managedLinkPublic, 0)
		for rows.Next() {
			var item managedLinkPublic
			var revoked sql.NullTime
			if err := rows.Scan(&item.ID, &item.FileID, &item.CreatedAt, &item.ExpiresAt, &revoked); err != nil {
				http.Error(w, "managed sharing unavailable", http.StatusServiceUnavailable)
				return
			}
			if revoked.Valid {
				item.RevokedAt = &revoked.Time
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "managed sharing unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}))
}

func (cfg Config) revokeManagedLinkHandler(db *sql.DB) http.Handler {
	return cfg.Auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method != http.MethodDelete {
			w.Header().Set("Allow", "DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		owner, err := cfg.Auth.getCurrentUser(r)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		idText := r.PathValue("id")
		linkID, err := uuid.Parse(idText)
		if err != nil || linkID == uuid.Nil || linkID.String() != idText {
			http.Error(w, "link not found", http.StatusNotFound)
			return
		}
		ctx, cancel := sqlContext(r.Context())
		defer cancel()
		var owned string
		err = db.QueryRowContext(ctx, `UPDATE managed_share_links
			SET revoked_at=COALESCE(revoked_at,now())
			WHERE id=$1 AND owner_id=$2::uuid RETURNING id::text`,
			linkID, owner).Scan(&owned)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "link not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "managed sharing unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
