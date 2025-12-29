package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// AuditAction represents the type of action being audited
type AuditAction string

const (
	AuditActionLogin          AuditAction = "login"
	AuditActionLogout         AuditAction = "logout"
	AuditActionFileUpload     AuditAction = "file_upload"
	AuditActionFileDownload   AuditAction = "file_download"
	AuditActionFileDelete     AuditAction = "file_delete"
	AuditActionLinkCreate     AuditAction = "link_create"
	AuditActionUserCreate     AuditAction = "user_create"
	AuditActionUserDelete     AuditAction = "user_delete"
	AuditActionAdminAction    AuditAction = "admin_action"
	AuditActionConfigChange   AuditAction = "config_change"
	AuditActionCleanup        AuditAction = "cleanup"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Action      AuditAction            `json:"action"`
	UserID      string                 `json:"user_id,omitempty"`
	Username    string                 `json:"username,omitempty"`
	IPAddress   string                 `json:"ip_address"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Resource    string                 `json:"resource,omitempty"` // file_id, link_id, etc.
	Details     map[string]interface{} `json:"details,omitempty"`
	Success     bool                   `json:"success"`
	ErrorMsg    string                 `json:"error_message,omitempty"`
}

// LogAudit records an audit log entry
func (s *Server) LogAudit(ctx context.Context, log AuditLog) error {
	log.Timestamp = time.Now()
	
	// Convert details to JSON
	detailsJSON, err := json.Marshal(log.Details)
	if err != nil {
		return err
	}
	
	query := `
		INSERT INTO audit_logs (
			id, timestamp, action, user_id, username, ip_address,
			user_agent, resource, details, success, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	
	_, err = s.db.ExecContext(ctx, query,
		log.ID,
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
	query := `
		SELECT id, timestamp, action, user_id, username, ip_address,
		       user_agent, resource, details, success, error_message
		FROM audit_logs
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1
	
	if filters.Action != "" {
		query += ` AND action = $` + string(rune('0'+argCount))
		args = append(args, filters.Action)
		argCount++
	}
	
	if filters.UserID != "" {
		query += ` AND user_id = $` + string(rune('0'+argCount))
		args = append(args, filters.UserID)
		argCount++
	}
	
	if !filters.StartTime.IsZero() {
		query += ` AND timestamp >= $` + string(rune('0'+argCount))
		args = append(args, filters.StartTime)
		argCount++
	}
	
	if !filters.EndTime.IsZero() {
		query += ` AND timestamp <= $` + string(rune('0'+argCount))
		args = append(args, filters.EndTime)
		argCount++
	}
	
	query += ` ORDER BY timestamp DESC LIMIT $` + string(rune('0'+argCount))
	args = append(args, filters.Limit)
	
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
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
			json.Unmarshal(detailsJSON, &log.Details)
		}
		
		logs = append(logs, log)
	}
	
	return logs, nil
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
