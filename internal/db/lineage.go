package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// checkSchemaLineage is read-only and runs before constructing the migration
// driver. Canonical and legacy migrations reuse version numbers with different
// meanings; version equality alone cannot establish compatibility.
func checkSchemaLineage(ctx context.Context, conn *sql.Conn) error {
	// Use the same borrowed connection as postgres.WithConnection. Its driver
	// selects CURRENT_SCHEMA(); pool-wide queries could inspect a different session.
	var schema sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		return fmt.Errorf("active schema inspection: %w", err)
	}
	if !schema.Valid {
		return errors.New("no active schema; preserve database configuration")
	}
	// Unqualified application SQL must not silently resolve data/ledger tables in
	// a later search_path schema while migrations target the first active schema.
	var outside bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.oid=ANY(ARRAY[to_regclass('schema_migrations'),to_regclass('files'),to_regclass('users'),to_regclass('sessions')]::oid[])
 AND n.nspname<>current_schema())`).Scan(&outside); err != nil {
		return fmt.Errorf("schema resolution inspection: %w", err)
	}
	if outside {
		return errors.New("application tables resolve outside the active migration schema; preserve database and reconcile search_path")
	}
	var versioned, legacySessions, files bool
	if err := conn.QueryRowContext(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL,
 EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='auth_sessions'), to_regclass('files') IS NOT NULL`).Scan(&versioned, &legacySessions, &files); err != nil {
		return fmt.Errorf("schema lineage inspection: %w", err)
	}
	incompatible := errors.New("unsupported schema lineage; preserve database and use a separately reviewed reconciliation")
	if legacySessions {
		return incompatible
	}
	if files {
		var compatible bool
		if err := conn.QueryRowContext(ctx, `SELECT count(*)=2 FROM information_schema.columns
 WHERE table_schema=current_schema() AND table_name='files'
 AND ((column_name='sha256_bytes' AND data_type='bigint') OR (column_name='created_by' AND data_type='text'))`).Scan(&compatible); err != nil {
			return fmt.Errorf("file schema inspection: %w", err)
		}
		if !compatible {
			return incompatible
		}
	}
	if !versioned {
		return nil
	} // Existing supported unversioned MVP upgrade remains tested.
	var version int64
	var dirty bool
	err := conn.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("schema version inspection: %w", err)
	}
	if dirty || version < 0 || version > 4 {
		return incompatible
	}
	if version == 4 {
		var compatible bool
		if err := conn.QueryRowContext(ctx, `SELECT count(*)=2 FROM information_schema.columns
 WHERE table_schema=current_schema() AND table_name='sessions'
 AND ((column_name='token_hash' AND data_type='character' AND character_maximum_length=64)
 OR (column_name='expires_at' AND data_type='timestamp with time zone'))`).Scan(&compatible); err != nil {
			return fmt.Errorf("session schema inspection: %w", err)
		}
		if !compatible {
			return incompatible
		}
	}
	return nil
}
