package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestSchemaLineage uses only a new labeled PostgreSQL container and fresh
// databases. It never accepts an operator DSN or reads application configuration.
func TestSchemaLineage(t *testing.T) {
	if os.Getenv("SFD_SCHEMA_GUARD_TEST") != "1" {
		t.Skip("isolated PostgreSQL lineage test requires SFD_SCHEMA_GUARD_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	docker := func(args ...string) string {
		t.Helper()
		b, err := schemaDocker(ctx, args...)
		if err != nil {
			t.Fatalf("isolated Docker %s failed", args[0])
		}
		return strings.TrimSpace(string(b))
	}
	endpoint := docker("context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	if os.Getenv("DOCKER_HOST") != "" || (!strings.HasPrefix(endpoint, "unix://") && !strings.HasPrefix(endpoint, "npipe://")) {
		t.Fatal("local Docker engine required")
	}
	run := "sfd-schema-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	network := run + "-net"
	docker("network", "create", "--label", "sfd.schema-guard-run="+run, network)
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		out, err := schemaDocker(clean, "network", "inspect", "--format", `{{index .Labels "sfd.schema-guard-run"}}`, network)
		if err != nil || strings.TrimSpace(string(out)) != run {
			t.Error("refusing network cleanup: ownership changed")
			return
		}
		if _, err := schemaDocker(clean, "network", "rm", network); err != nil {
			t.Error("owned network cleanup failed")
		}
	})
	id := docker("run", "-d", "--name", run, "--network", network, "--tmpfs", "/var/lib/postgresql/data", "--label", "sfd.schema-guard-run="+run, "-p", "127.0.0.1::5432", "-e", "POSTGRES_USER=sfd", "-e", "POSTGRES_PASSWORD=SyntheticLineage123", "-e", "POSTGRES_DB=sfd_lineage", "postgres:16")
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		out, err := schemaDocker(clean, "inspect", "--format", `{{index .Config.Labels "sfd.schema-guard-run"}}`, id)
		if err != nil || strings.TrimSpace(string(out)) != run {
			t.Error("refusing cleanup: ownership changed")
			return
		}
		if _, err := schemaDocker(clean, "rm", "-fv", id); err != nil {
			t.Error("owned container/anonymous-volume cleanup failed")
		}
	})
	address := docker("port", id, "5432/tcp")
	if !strings.HasPrefix(address, "127.0.0.1:") {
		t.Fatal("loopback required")
	}
	dsn := "postgres://sfd:SyntheticLineage123@" + address + "/sfd_lineage?sslmode=disable"
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	ready := false
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if admin.PingContext(ctx) == nil {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("fresh PostgreSQL unavailable")
	}
	fresh := func(n int) *sql.DB {
		t.Helper()
		name := fmt.Sprintf("sfd_lineage_%d", n)
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("pgx", strings.Replace(dsn, "/sfd_lineage?", "/"+name+"?", 1))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	t.Run("canonical-fresh-and-idempotent", func(t *testing.T) {
		db := fresh(0)
		if err := RunMigrations(db); err != nil {
			t.Fatal(err)
		}
		if err := RunMigrations(db); err != nil {
			t.Fatal(err)
		}
		var version int
		if err := db.QueryRow("SELECT version FROM schema_migrations").Scan(&version); err != nil || version != 4 {
			t.Fatal("canonical migration identity")
		}
	})
	// These are negative contract fixtures, not a claim to reproduce every legacy
	// migration. Full legacy BYTEA/BIGINT acceptance remains in PR29.
	cases := []struct {
		name     string
		version  int
		dirty    bool
		layout   string
		sessions bool
	}{
		{"legacy-v4-collision", 4, false, "bigint", false},
		{"legacy-v11-bytea", 11, false, "bytea", false},
		{"legacy-v11-bigint", 11, false, "bigint", false},
		{"legacy-v12-bytea", 12, false, "bytea", false},
		{"legacy-v12-bigint", 12, false, "bigint", false},
		{"hybrid-v4-bytea", 4, false, "bytea", true},
		{"dirty-v4", 4, true, "bigint", true},
	}
	for n, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := fresh(n + 1)
			// All identifiers and type names below are fixed test constants.
			if _, err := db.ExecContext(ctx, "CREATE TABLE schema_migrations(version bigint NOT NULL PRIMARY KEY,dirty boolean NOT NULL)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO schema_migrations VALUES ($1,$2)", tc.version, tc.dirty); err != nil {
				t.Fatal(err)
			}
			setup := "CREATE TABLE files(id integer PRIMARY KEY,created_by text,sha256_bytes bigint); INSERT INTO files(id,created_by) VALUES (1,'synthetic-owner');"
			if tc.layout == "bytea" {
				setup = "CREATE TABLE files(id integer PRIMARY KEY,created_by text,sha256_bytes bytea); INSERT INTO files(id,created_by) VALUES (1,'synthetic-owner');"
			}
			if tc.sessions {
				setup += "CREATE TABLE sessions(token_hash char(64) PRIMARY KEY,expires_at timestamptz NOT NULL);"
			}
			if _, err := db.ExecContext(ctx, setup); err != nil {
				t.Fatal(err)
			}
			snapshot := func() string {
				t.Helper()
				var s string
				if err := db.QueryRowContext(ctx, `SELECT json_build_object('version',(SELECT row_to_json(m) FROM schema_migrations m),'files',(SELECT json_agg(f ORDER BY id) FROM files f),'columns',(SELECT json_agg(c ORDER BY table_name,ordinal_position) FROM (SELECT table_name,column_name,data_type,ordinal_position FROM information_schema.columns WHERE table_schema='public') c))::text`).Scan(&s); err != nil {
					t.Fatal(err)
				}
				var v any
				if err := json.Unmarshal([]byte(s), &v); err != nil {
					t.Fatal(err)
				}
				return s
			}
			before := snapshot()
			err := RunMigrations(db)
			if err == nil {
				t.Error("accepted incompatible schema lineage")
			}
			if after := snapshot(); after != before {
				t.Error("rejected schema was modified")
			}
		})
	}
}

// schemaDocker only receives constant subcommands plus locally generated UUID
// names/IDs. Cleanup callers verify the exact resource label before removal.
func schemaDocker(ctx context.Context, args ...string) ([]byte, error) {
	// #nosec G204 -- Fixed Docker executable, no shell; all arguments come from this isolated test, never app/operator input.
	return exec.CommandContext(ctx, "docker", args...).Output()
}
