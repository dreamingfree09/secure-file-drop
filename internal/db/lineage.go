package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// checkSchemaLineage accepts only fresh databases or the verified legacy11/12
// contracts. It is read-only, shares the migration driver's connection/schema,
// and never rewrites metadata, assigns ownership or converts digest formats.
func checkSchemaLineage(ctx context.Context, conn *sql.Conn) error {
	incompatible := errors.New("unsupported schema lineage; preserve database and use a separately reviewed reconciliation")
	var schema sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		return fmt.Errorf("active schema inspection: %w", err)
	}
	if !schema.Valid {
		return incompatible
	}
	var outside bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.oid=ANY(ARRAY[to_regclass('schema_migrations'),to_regclass('files'),to_regclass('users'),to_regclass('auth_sessions'),to_regclass('resumable_uploads'),to_regclass('audit_logs'),to_regclass('sessions')]::oid[])
 AND n.nspname<>current_schema())`).Scan(&outside); err != nil {
		return fmt.Errorf("schema resolution inspection: %w", err)
	}
	if outside {
		return incompatible
	}
	var versioned, files, users, sessions, canonical, applicationData bool
	if err := conn.QueryRowContext(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL,to_regclass('files') IS NOT NULL,to_regclass('users') IS NOT NULL,to_regclass('auth_sessions') IS NOT NULL,
 EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='sessions'),
 EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relname IN ('files','users','auth_sessions','resumable_uploads','audit_logs','sessions'))`).Scan(&versioned, &files, &users, &sessions, &canonical, &applicationData); err != nil {
		return fmt.Errorf("lineage inspection: %w", err)
	}
	if canonical {
		return incompatible
	}
	if !versioned {
		if applicationData {
			return incompatible
		}
		return nil
	}
	var version, rows int64
	var dirty bool
	err := conn.QueryRowContext(ctx, "SELECT version,dirty,count(*) OVER() FROM schema_migrations").Scan(&version, &dirty, &rows)
	if errors.Is(err, sql.ErrNoRows) {
		if applicationData {
			return incompatible
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("schema version inspection: %w", err)
	}
	if dirty || rows != 1 || (version != 11 && version != 12) {
		return incompatible
	}
	if version == 11 && sessions {
		return incompatible
	}
	var compatible bool
	if err := conn.QueryRowContext(ctx, `SELECT count(*)=12 FROM information_schema.columns WHERE table_schema=current_schema() AND (
 (table_name='files' AND ((column_name='sha256_bytes' AND data_type IN ('bigint','bytea')) OR (column_name='user_id' AND data_type='uuid')
 OR (column_name='size_bytes' AND data_type='bigint') OR (column_name='status' AND data_type='text') OR (column_name='object_key' AND data_type='text')
 OR (column_name='link_password' AND data_type='text') OR (column_name='expires_at' AND data_type='timestamp with time zone') OR (column_name='auto_delete' AND data_type='boolean')))
 OR (table_name='users' AND ((column_name='id' AND data_type='uuid') OR (column_name='email_verified' AND data_type='boolean') OR (column_name='is_active' AND data_type='boolean') OR (column_name='storage_quota_bytes' AND data_type='bigint'))))`).Scan(&compatible); err != nil {
		return fmt.Errorf("legacy column inspection: %w", err)
	}
	if !compatible {
		return incompatible
	}
	if version == 12 {
		if err := conn.QueryRowContext(ctx, `SELECT count(*)=3 FROM information_schema.columns WHERE table_schema=current_schema() AND
 ((table_name='auth_sessions' AND ((column_name='token_hash' AND data_type='text') OR (column_name='expires_at' AND data_type='timestamp with time zone')))
 OR (table_name='users' AND column_name='is_admin' AND data_type='boolean'))`).Scan(&compatible); err != nil {
			return fmt.Errorf("legacy session inspection: %w", err)
		}
		if !compatible {
			return incompatible
		}
	}
	return nil
}
