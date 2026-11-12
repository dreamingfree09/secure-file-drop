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
	FileName  string     `json:"file_name"`
	State     string     `json:"state"`
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
		query := `SELECT l.id::text,l.file_id::text,f.orig_name,
			CASE WHEN l.revoked_at IS NOT NULL THEN 'revoked'
			WHEN l.expires_at<=now() THEN 'expired' ELSE 'active' END,
			l.created_at,l.expires_at,l.revoked_at
			FROM managed_share_links l JOIN files f ON f.id=l.file_id AND f.user_id=l.owner_id
			WHERE l.owner_id=$1::uuid`
		args := []any{owner}
		if fileText != "" {
			fileID, parseErr := uuid.Parse(fileText)
			if parseErr != nil || fileID == uuid.Nil || fileID.String() != fileText {
				http.Error(w, "invalid file id", http.StatusBadRequest)
				return
			}
			query += " AND l.file_id=$2"
			args = append(args, fileID)
		}
		if before := r.URL.Query().Get("before"); before != "" {
			cursor, parseErr := uuid.Parse(before)
			if parseErr != nil || cursor == uuid.Nil || cursor.String() != before {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
			// The anchor is scoped to this owner too. Foreign/missing anchors
			// produce an empty page without disclosing whether they exist.
			args = append(args, cursor)
			parameter := "$2"
			if len(args) == 3 {
				parameter = "$3"
			}
			query += " AND (l.created_at,l.id)<(SELECT created_at,id FROM managed_share_links WHERE id=" + parameter + " AND owner_id=$1::uuid)"
		}
		query += " ORDER BY l.created_at DESC,l.id DESC LIMIT 51"
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
			if err := rows.Scan(&item.ID, &item.FileID, &item.FileName, &item.State, &item.CreatedAt, &item.ExpiresAt, &revoked); err != nil {
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
		if len(items) > 50 {
			items = items[:50]
			w.Header().Set("X-Next-Cursor", items[49].ID)
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
