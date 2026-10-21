package db

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestLegacyLineage uses only the controller's fresh loopback PostgreSQL and
// checks both outer and per-schema ownership markers before any cleanup.
func TestLegacyLineage(t *testing.T) {
	dsn, owner := os.Getenv("SFD_AUTH_TEST_DSN"), os.Getenv("SFD_AUTH_TEST_OWNER")
	if dsn == "" && owner == "" {
		t.Skip("requires the newly owned real PostgreSQL fixture")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "sfd_auth_") || !strings.HasPrefix(owner, "sfd-auth-") {
		t.Fatal("unexpected PostgreSQL target")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("open fixture")
	}
	defer func() { _ = admin.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	var actual string
	if admin.QueryRowContext(ctx, "SELECT owner FROM public.sfd_test_owner").Scan(&actual) != nil || actual != owner {
		t.Fatal("outer database ownership mismatch")
	}
	cases := []struct {
		name                                string
		version                             int
		layout                              string
		role, dirty, canonical, unversioned bool
		accepted                            bool
		extraTable                          string
		emptyLedger                         bool
	}{
		{name: "fresh-and-idempotent", accepted: true},
		{name: "legacy11-bigint-role", version: 11, layout: "bigint", role: true, accepted: true},
		{name: "legacy11-bytea-role", version: 11, layout: "bytea", role: true, accepted: true},
		{name: "legacy11-additive-role", version: 11, layout: "bigint", accepted: true},
		{name: "legacy12-bigint", version: 12, layout: "bigint", role: true, accepted: true},
		{name: "legacy12-bytea", version: 12, layout: "bytea", role: true, accepted: true},
		{name: "canonical4-rejected-without-mutation", version: 4, layout: "bigint", canonical: true},
		{name: "dirty11-rejected", version: 11, layout: "bigint", dirty: true},
		{name: "future13-rejected", version: 13, layout: "bigint"},
		{name: "unversioned-existing-data-rejected", version: 11, layout: "bigint", unversioned: true},
		{name: "unversioned-audit-only-rejected", extraTable: "audit_logs"},
		{name: "unversioned-resumable-only-rejected", extraTable: "resumable_uploads"},
		{name: "empty-ledger-audit-only-rejected", extraTable: "audit_logs", emptyLedger: true},
		{name: "empty-ledger-resumable-only-rejected", extraTable: "resumable_uploads", emptyLedger: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "sfd_lineage_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+name); err != nil {
				t.Fatal("create fresh owned schema")
			}
			scoped := *u
			q := scoped.Query()
			q.Set("search_path", name)
			scoped.RawQuery = q.Encode()
			target, err := sql.Open("pgx", scoped.String())
			if err != nil {
				t.Fatal("open owned schema")
			}
			target.SetMaxOpenConns(4)
			if tc.version == 0 {
				target.SetMaxOpenConns(1)
			}
			execSQL := func(query string, args ...any) {
				t.Helper()
				if _, err := target.ExecContext(ctx, query, args...); err != nil {
					t.Fatal("owned schema setup failed")
				}
			}
			execSQL("CREATE TABLE sfd_lineage_owner(owner text NOT NULL)")
			execSQL("INSERT INTO sfd_lineage_owner VALUES ($1)", owner)
			t.Cleanup(func() {
				clean, done := context.WithTimeout(context.Background(), 5*time.Second)
				defer done()
				var marker string
				if target.QueryRowContext(clean, "SELECT owner FROM sfd_lineage_owner").Scan(&marker) != nil || marker != owner {
					t.Error("refusing schema cleanup: marker mismatch")
					return
				}
				_ = target.Close()
				if _, err := admin.ExecContext(clean, "DROP SCHEMA "+name+" CASCADE"); err != nil {
					t.Error("owned schema cleanup failed")
				}
			})
			if tc.version != 0 {
				entries, err := migrationsFS.ReadDir("migrations")
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if !strings.HasSuffix(entry.Name(), ".up.sql") {
						continue
					}
					number := strings.Split(entry.Name(), "_")[0]
					if number > "000003" && tc.canonical {
						continue
					}
					if number > "000011" && tc.version == 11 {
						continue
					}
					if number > "000012" {
						continue
					}
					raw, err := migrationsFS.ReadFile("migrations/" + entry.Name())
					if err != nil {
						t.Fatal(err)
					}
					execSQL(string(raw))
				}
				if tc.canonical {
					execSQL("CREATE TABLE sessions(token_hash char(64) PRIMARY KEY,expires_at timestamptz NOT NULL)")
				}
				if tc.role && !tc.canonical {
					execSQL("ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin boolean NOT NULL DEFAULT false")
				}
				if tc.layout == "bytea" {
					execSQL("ALTER TABLE files DROP CONSTRAINT files_sha256_bytes_check;ALTER TABLE files ALTER COLUMN sha256_bytes TYPE bytea USING NULL::bytea")
				}
				if !tc.unversioned {
					execSQL("CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL)")
					execSQL("INSERT INTO schema_migrations VALUES ($1,$2)", tc.version, tc.dirty)
				}
				id := uuid.NewString()
				file := uuid.NewString()
				execSQL("INSERT INTO users(id,email,username,password_hash) VALUES ($1,'fixture@example.invalid','lineage_fixture','not-a-credential')", id)
				if tc.role && !tc.canonical {
					execSQL("UPDATE users SET is_admin=true WHERE id=$1", id)
				}
				execSQL("INSERT INTO files(id,object_key,orig_name,content_type,size_bytes,status,user_id) VALUES ($1,'fixture/owned','synthetic.txt','text/plain',1,'pending',$2)", file, id)
				if tc.version >= 12 && !tc.canonical {
					execSQL("INSERT INTO auth_sessions(token_hash,expires_at) VALUES ($1,CURRENT_TIMESTAMP+interval '1 hour')", strings.Repeat("c", 64))
				}
			}
			if tc.extraTable != "" {
				execSQL("CREATE TABLE " + tc.extraTable + "(id int PRIMARY KEY,payload text NOT NULL)")
				execSQL("INSERT INTO " + tc.extraTable + " VALUES(1,'owned-historical-row')")
			}
			if tc.emptyLedger {
				execSQL("CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL)")
			}
			snapshot := func() (string, string) {
				t.Helper()
				var cols string
				if err := target.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(c) ORDER BY table_name,ordinal_position),'[]')::text FROM (SELECT table_name,column_name,data_type,ordinal_position FROM information_schema.columns WHERE table_schema=current_schema()) c`).Scan(&cols); err != nil {
					t.Fatal("schema snapshot")
				}
				data := ""
				queries := map[string]string{"users": "SELECT COALESCE(jsonb_agg(to_jsonb(u)-'is_admin' ORDER BY id),'[]')::text FROM users u", "files": "SELECT COALESCE(jsonb_agg(to_jsonb(f) ORDER BY id),'[]')::text FROM files f", "audit_logs": "SELECT COALESCE(jsonb_agg(to_jsonb(a) ORDER BY id),'[]')::text FROM audit_logs a", "resumable_uploads": "SELECT COALESCE(jsonb_agg(to_jsonb(a) ORDER BY id),'[]')::text FROM resumable_uploads a", "auth_sessions": "SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY token_hash),'[]')::text FROM auth_sessions s"}
				for _, table := range []string{"users", "files", "auth_sessions", "audit_logs", "resumable_uploads"} {
					var exists bool
					if target.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists) != nil {
						t.Fatal("table snapshot")
					}
					part := "[]"
					if exists {
						if target.QueryRowContext(ctx, queries[table]).Scan(&part) != nil {
							t.Fatal("record snapshot")
						}
					}
					data += part
				}
				return data, cols
			}
			before, columns := snapshot()
			err = RunMigrations(target)
			after, afterColumns := snapshot()
			if tc.accepted {
				if err != nil {
					t.Fatalf("supported lineage rejected: %v", err)
				}
				var version int
				var dirty bool
				if target.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty) != nil || version != 12 || dirty {
					t.Error("expected clean additive version12")
				}
				if err := RunMigrations(target); err != nil {
					t.Error("idempotent migration failed")
				}
				if tc.version != 0 {
					var role bool
					if target.QueryRowContext(ctx, "SELECT is_admin FROM users WHERE username='lineage_fixture'").Scan(&role) != nil || role != tc.role {
						t.Error("registered role changed")
					}
				}
			} else {
				if err == nil {
					t.Error("incompatible lineage accepted")
				}
				if columns != afterColumns {
					t.Error("rejected schema metadata changed")
				}
			}
			if before != after {
				t.Error("existing owned records changed")
			}
		})
	}
}
