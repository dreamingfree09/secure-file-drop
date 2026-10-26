package main

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestAdminPasswordCheck(t *testing.T) {
	h, e := bcrypt.GenerateFromPassword([]byte("synthetic-fixture-only-7!"), 12)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, password string
		ok             bool
	}{{"correct", "synthetic-fixture-only-7!", true}, {"wrong", "different-fixture-only-1!", false}, {"empty", "", false}, {"long", strings.Repeat("x", 73), false}} {
		t.Run(tc.name, func(t *testing.T) {
			r, e := run(context.Background(), input{Mode: "check-admin-password", AdminHash: string(h), AdminPassword: tc.password})
			if r.OK != tc.ok || (e == nil) != tc.ok {
				t.Fatal("credential gate result")
			}
		})
	}
}
func TestIdentityTargetGuard(t *testing.T) {
	for _, origin := range []string{"", "https://unrelated.invalid", "http://localhost:8080", "http://backend:8080/extra"} {
		if authenticate(context.Background(), input{BackendOrigin: origin, AdminName: "admin", Session: "fixture"}) == nil {
			t.Fatal("unapproved target accepted")
		}
	}
}
func TestAccountCapPostgres(t *testing.T) {
	dsn := os.Getenv("SFD_ACCEPTANCE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires label-verified newly created PostgreSQL fixture; not a pass")
	}
	u, e := url.Parse(dsn)
	if e != nil || u.Hostname() != "127.0.0.1" || !regexp.MustCompile(`^/sfd_acceptance_fixture_[a-f0-9]{12}$`).MatchString(u.Path) {
		t.Fatal("refusing non-fixture database")
	}
	db, e := sql.Open("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	// This database was just created by the ownership-checking wrapper. None of
	// these statements can address the running application database.
	exec(`CREATE TABLE users(id uuid PRIMARY KEY,email text UNIQUE NOT NULL,username text UNIQUE NOT NULL,password_hash text NOT NULL,created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,last_login timestamp,is_active boolean NOT NULL DEFAULT true,email_verified boolean NOT NULL DEFAULT false,is_admin boolean NOT NULL DEFAULT false,verification_token text,storage_quota_bytes bigint);
 CREATE TABLE files(id uuid PRIMARY KEY,user_id uuid REFERENCES users(id));
 CREATE TABLE schema_migrations(version bigint NOT NULL,dirty boolean NOT NULL);
 INSERT INTO schema_migrations VALUES(12,false);
 CREATE TABLE audit_logs(id bigserial PRIMARY KEY,timestamp timestamp NOT NULL DEFAULT NOW(),event_type text NOT NULL,action text NOT NULL,username text,ip_address text NOT NULL,resource_id text,metadata jsonb,success boolean NOT NULL DEFAULT true);`)
	goodHash, e := bcrypt.GenerateFromPassword([]byte("synthetic-fixture-only-7!"), 12)
	if e != nil {
		t.Fatal(e)
	}
	weakHash, e := bcrypt.GenerateFromPassword([]byte("synthetic-fixture-only-7!"), 11)
	if e != nil {
		t.Fatal(e)
	}
	existing := "00000000-0000-4000-8000-000000000001"
	id := "00000000-0000-4000-8000-000000000002"
	cases := []struct {
		name, change string
		mutate       func(*input)
		auth         int
		ok           bool
	}{
		{"success", "", nil, 0, true},
		{"unverified", "UPDATE users SET email_verified=false WHERE id='" + id + "'", nil, 0, false},
		{"inactive", "UPDATE users SET is_active=false WHERE id='" + id + "'", nil, 0, false},
		{"administrator", "UPDATE users SET is_admin=true WHERE id='" + id + "'", nil, 0, false},
		{"verification_token_present", "UPDATE users SET verification_token='synthetic' WHERE id='" + id + "'", nil, 0, false},
		{"already_capped", "UPDATE users SET storage_quota_bytes=1048576 WHERE id='" + id + "'", nil, 0, false},
		{"stale_account", "UPDATE users SET created_at=CURRENT_TIMESTAMP-INTERVAL '1 hour' WHERE id='" + id + "'", nil, 0, false},
		{"used_account", "INSERT INTO files VALUES('00000000-0000-4000-8000-000000000003','" + id + "')", nil, 0, false},
		{"schema11", "UPDATE schema_migrations SET version=11", nil, 0, false},
		{"dirty_schema", "UPDATE schema_migrations SET dirty=true", nil, 0, false},
		{"changed_existing", "UPDATE users SET is_admin=true WHERE id='" + existing + "'", nil, 0, false},
		{"prior_work_receipt", "INSERT INTO audit_logs(event_type,action,ip_address,metadata) VALUES('admin_action','admin_action','fixture',jsonb_build_object('work_id','" + workID + "'))", nil, 0, false},
		{"mismatched_username", "", func(in *input) { in.Username = "sfd_acceptance_20261026_0123abcd" }, 0, false},
		{"forbidden_username", "", func(in *input) { in.Username = "admin" }, 0, false},
		{"mismatched_email", "", func(in *input) { in.Email = "other@example.invalid" }, 0, false},
		{"unknown_uuid", "", func(in *input) { in.ID = "00000000-0000-4000-8000-000000000009" }, 0, false},
		{"missing_admin", "", nil, 1, false},
		{"revoked_before_commit", "", nil, 2, false},
		{"audit_failure", "ALTER TABLE audit_logs ADD CONSTRAINT fixture_fail CHECK(false) NOT VALID", nil, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec("ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS fixture_fail; TRUNCATE files,users,audit_logs; UPDATE schema_migrations SET version=12,dirty=false")
			exec(`INSERT INTO users(id,email,username,password_hash,email_verified) VALUES($1,'original@example.invalid','existing-fixture',$2,true)`, existing, string(goodHash))
			fp, _, e := usersFingerprint(context.Background(), db, "")
			if e != nil {
				t.Fatal(e)
			}
			exec(`INSERT INTO users(id,email,username,password_hash,email_verified) VALUES($1,'controlled+fixture@example.invalid','sfd_acceptance_20261026',$2,true)`, id, string(goodHash))
			in := input{Mode: "cap-new-account", DSN: dsn, AdminName: "fixture-admin", Session: "synthetic-fixture-session", BackendOrigin: "http://backend:8080", ID: id, Username: "sfd_acceptance_20261026", Email: "controlled+fixture@example.invalid", ExistingUsersSHA256: fp}
			if tc.change != "" {
				exec(tc.change)
			}
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			before, _, e := usersFingerprint(context.Background(), db, "")
			if e != nil {
				t.Fatal(e)
			}
			var auditsBefore int
			if e = db.QueryRow("SELECT count(*) FROM audit_logs").Scan(&auditsBefore); e != nil {
				t.Fatal(e)
			}
			calls := 0
			verify := func(context.Context, input) error {
				calls++
				if tc.auth == 1 || tc.auth == 2 && calls == 2 {
					return errors.New("synthetic rejected identity")
				}
				return nil
			}
			r, e := runWithAuth(context.Background(), in, verify)
			if r.OK != tc.ok || (e == nil) != tc.ok {
				t.Fatalf("unexpected guarded result for %s", tc.name)
			}
			after, _, e := usersFingerprint(context.Background(), db, "")
			if e != nil {
				t.Fatal(e)
			}
			var auditsAfter int
			if e = db.QueryRow("SELECT count(*) FROM audit_logs").Scan(&auditsAfter); e != nil {
				t.Fatal(e)
			}
			if !tc.ok && (before != after || auditsBefore != auditsAfter) {
				t.Fatal("failure did not preserve transaction state")
			}
			if tc.ok {
				var q int64
				if e = db.QueryRow("SELECT storage_quota_bytes FROM users WHERE id=$1", id).Scan(&q); e != nil || q != quotaBytes {
					t.Fatal("quota mismatch")
				}
				untouched, _, e := usersFingerprint(context.Background(), db, id)
				if e != nil || untouched != fp || auditsAfter != 1 {
					t.Fatal("existing-account or audit mismatch")
				}
				if calls != 2 {
					t.Fatal("administrator not revalidated before commit")
				}
			}
		})
	}
	t.Run("bcrypt_cost_not12", func(t *testing.T) {
		exec("ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS fixture_fail; TRUNCATE files,users,audit_logs; UPDATE schema_migrations SET version=12,dirty=false")
		exec(`INSERT INTO users(id,email,username,password_hash,email_verified) VALUES($1,'controlled+fixture@example.invalid','sfd_acceptance_20261026',$2,true)`, id, string(weakHash))
		h, _, e := usersFingerprint(context.Background(), db, id)
		if e != nil {
			t.Fatal(e)
		}
		r, e := runWithAuth(context.Background(), input{Mode: "cap-new-account", DSN: dsn, ID: id, Username: "sfd_acceptance_20261026", Email: "controlled+fixture@example.invalid", ExistingUsersSHA256: h}, func(context.Context, input) error { return nil })
		if e == nil || r.OK {
			t.Fatal("wrong bcrypt cost accepted")
		}
	})
}
func TestBoundedContext(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if authenticate(ctx, input{BackendOrigin: "http://backend:8080", AdminName: "admin", Session: "synthetic"}) == nil {
		t.Fatal("expired context accepted")
	}
}
