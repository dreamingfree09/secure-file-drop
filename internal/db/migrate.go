// Package db applies the embedded, versioned Secure File Drop schema.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations borrows one connection and leaves the shared pool usable.
func RunMigrations(db *sql.DB) (result error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := checkSchemaLineage(ctx, db); err != nil {
		return err
	}

	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer func() { result = errors.Join(result, source.Close()) }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration connection: %w", err)
	}
	defer func() { result = errors.Join(result, conn.Close()) }()
	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{StatementTimeout: 30 * time.Second})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}
	// WithConnection does not own db. conn.Close above also covers driver setup failures.
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migration instance: %w", err)
	}
	m.LockTimeout = 10 * time.Second
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration failed: %w", err)
	}
	return nil
}
