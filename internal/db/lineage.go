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
func checkSchemaLineage(ctx context.Context, db *sql.DB) error {
	var versioned, legacySessions, files bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL,
 to_regclass('public.auth_sessions') IS NOT NULL, to_regclass('public.files') IS NOT NULL`).Scan(&versioned, &legacySessions, &files); err != nil {
		return fmt.Errorf("schema lineage inspection: %w", err)
	}
	incompatible := errors.New("unsupported schema lineage; preserve database and use a separately reviewed reconciliation")
	if legacySessions {
		return incompatible
	}
	if files {
		var compatible bool
		if err := db.QueryRowContext(ctx, `SELECT count(*)=2 FROM information_schema.columns
 WHERE table_schema='public' AND table_name='files'
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
	err := db.QueryRowContext(ctx, "SELECT version,dirty FROM public.schema_migrations").Scan(&version, &dirty)
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
		if err := db.QueryRowContext(ctx, `SELECT count(*)=2 FROM information_schema.columns
 WHERE table_schema='public' AND table_name='sessions'
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
