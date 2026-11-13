// Package db applies the embedded, versioned Secure File Drop schema.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type boundedMigrationSource struct {
	embed.FS
	maxVersion uint
}

func (source boundedMigrationSource) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := source.FS.ReadDir(name)
	visible := entries[:0]
	for _, entry := range entries {
		if (source.maxVersion < 14 && strings.HasPrefix(entry.Name(), "000014_")) ||
			(source.maxVersion < 15 && strings.HasPrefix(entry.Name(), "000015_")) ||
			(source.maxVersion < 16 && strings.HasPrefix(entry.Name(), "000016_")) {
			continue
		}
		visible = append(visible, entry)
	}
	return visible, err
}

// RunMigrations borrows one connection and leaves the shared pool usable.
func RunMigrations(db *sql.DB) error {
	return runMigrations(db, 13)
}

// RunResumableMigrations enables the separately opted-in self-hosted schema.
func RunResumableMigrations(db *sql.DB) error {
	return runMigrations(db, 14)
}

// RunManagedShareMigrations installs the separately gated durable link ledger.
// The schema is never activated by default Railway/staging configurations.
func RunManagedShareMigrations(db *sql.DB) error {
	return runMigrations(db, 15)
}

// RunRegistrationInvitationMigrations enables separately reviewed schema16.
func RunRegistrationInvitationMigrations(db *sql.DB) error {
	return runMigrations(db, 16)
}

func runMigrations(db *sql.DB, target uint) (result error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var files fs.FS = migrationsFS
	if target < 16 {
		files = boundedMigrationSource{FS: migrationsFS, maxVersion: target}
	}
	source, err := iofs.New(files, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer func() { result = errors.Join(result, source.Close()) }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration connection: %w", err)
	}
	defer func() { result = errors.Join(result, conn.Close()) }()
	if err := checkSchemaLineage(ctx, conn); err != nil {
		return err
	}
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
	version, _, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("read migration version: %w", err)
	}
	if version > target {
		return errors.New("existing resumable schema requires its original enabled configuration")
	}
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration failed: %w", err)
	}
	if version, dirty, err := m.Version(); err != nil || dirty || version != target {
		return errors.New("migration version does not match enabled runtime")
	}
	return nil
}
