package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AuditAction represents the type of action being audited
type AuditAction string

// Audit actions identify recorded operations.
const (
	AuditActionLogin        AuditAction = "login"
	AuditActionLogout       AuditAction = "logout"
	AuditActionFileUpload   AuditAction = "file_upload"
	AuditActionFileDownload AuditAction = "file_download"
	AuditActionFileDelete   AuditAction = "file_delete"
	AuditActionLinkCreate   AuditAction = "link_create"
	AuditActionUserCreate   AuditAction = "user_create"
	AuditActionUserDelete   AuditAction = "user_delete"
	AuditActionAdminAction  AuditAction = "admin_action"
	AuditActionConfigChange AuditAction = "config_change"
	AuditActionCleanup      AuditAction = "cleanup"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Action    AuditAction            `json:"action"`
	UserID    string                 `json:"user_id,omitempty"`
	Username  string                 `json:"username,omitempty"`
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent,omitempty"`
	Resource  string                 `json:"resource,omitempty"` // file_id, link_id, etc.
	Details   map[string]interface{} `json:"details,omitempty"`
	Success   bool                   `json:"success"`
	ErrorMsg  string                 `json:"error_message,omitempty"`
}

// LogAudit records an audit entry using the applied schema and database-assigned ID.
func (s *Server) LogAudit(ctx context.Context, log AuditLog) error {
	log.Timestamp = time.Now()

	// Convert details to JSON
	detailsJSON, err := json.Marshal(log.Details)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO audit_logs (
			timestamp, event_type, action, user_id, username, ip_address,
			user_agent, resource_id, metadata, success, error_message
		) VALUES ($1, $2, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err = s.db.ExecContext(ctx, query,
		log.Timestamp,
		log.Action,
		nullString(log.UserID),
		nullString(log.Username),
		log.IPAddress,
		nullString(log.UserAgent),
		nullString(log.Resource),
		detailsJSON,
		log.Success,
		nullString(log.ErrorMsg),
	)

	return err
}

// GetAuditLogs retrieves audit logs with filtering
func (s *Server) GetAuditLogs(ctx context.Context, filters AuditFilters) ([]AuditLog, error) {

	const query = `SELECT id::text, timestamp, action, user_id, username, ip_address,
 user_agent, resource_id, metadata, success, error_message FROM audit_logs
 WHERE ($1::text = '' OR action = $1)
 AND ($2::text = '' OR user_id::text = $2)
 AND ($3::timestamptz IS NULL OR timestamp >= $3)
 AND ($4::timestamptz IS NULL OR timestamp <= $4)
 ORDER BY timestamp DESC LIMIT $5`
	var start, end interface{}
	if !filters.StartTime.IsZero() {
		start = filters.StartTime
	}
	if !filters.EndTime.IsZero() {
		end = filters.EndTime
	}
	rows, err := s.db.QueryContext(ctx, query, filters.Action, filters.UserID, start, end, filters.Limit)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var detailsJSON []byte
		var userID, username, userAgent, resource, errorMsg sql.NullString

		err := rows.Scan(
			&log.ID,
			&log.Timestamp,
			&log.Action,
			&userID,
			&username,
			&log.IPAddress,
			&userAgent,
			&resource,
			&detailsJSON,
			&log.Success,
			&errorMsg,
		)
		if err != nil {
			return nil, err
		}

		if userID.Valid {
			log.UserID = userID.String
		}
		if username.Valid {
			log.Username = username.String
		}
		if userAgent.Valid {
			log.UserAgent = userAgent.String
		}
		if resource.Valid {
			log.Resource = resource.String
		}
		if errorMsg.Valid {
			log.ErrorMsg = errorMsg.String
		}

		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &log.Details); err != nil {
				return nil, fmt.Errorf("invalid audit details")
			}
		}

		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// AuditFilters for querying audit logs
type AuditFilters struct {
	Action    AuditAction
	UserID    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// nullString helper for nullable strings
func nullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}
