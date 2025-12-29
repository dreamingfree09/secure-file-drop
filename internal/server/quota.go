// UserQuotaHandler returns per-user storage quota usage and limits.
// It is used by the UI to display percentage used and color-coded state.
// quota.go - Per-user storage quota reporting endpoint.
//
// Exposes current usage and configured quota to the UI.
package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// UserQuotaHandler returns the current user's storage quota and usage information.
// GET /quota
//
// Returns JSON:
//
//	{
//	  "storage_used_bytes": 123456789,
//	  "storage_quota_bytes": 10737418240
//	}
func (s *Server) UserQuotaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current user
	userID, err := s.authCfg.getCurrentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Query user's storage quota and current usage
	// Note: use user UUID (`users.id`) and `files.user_id` for ownership.
	var quota sql.NullInt64
	var usage int64

	err = s.db.QueryRow(`
		SELECT 
			u.storage_quota_bytes,
			COALESCE(SUM(f.size_bytes), 0) as current_usage
		FROM users u
		LEFT JOIN files f ON f.user_id = u.id AND f.status IN ('stored', 'hashed', 'ready')
		WHERE u.id = $1
		GROUP BY u.storage_quota_bytes
	`, userID).Scan(&quota, &usage)

	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Build response
	response := map[string]interface{}{
		"storage_used_bytes": usage,
	}

	// Include quota if set (null means unlimited)
	if quota.Valid {
		response["storage_quota_bytes"] = quota.Int64
	} else {
		response["storage_quota_bytes"] = 0 // Frontend will show "Unlimited"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
