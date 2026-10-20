package server

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAuditFiltersOwnedFixture(t *testing.T) {
	dsn := os.Getenv("SFD_AUTH_TEST_DSN")
	if dsn == "" {
		t.Skip("requires guarded owned PostgreSQL fixture; not acceptance")
	}
	u, err := url.Parse(dsn)
	owner := os.Getenv("SFD_AUTH_TEST_OWNER")
	if err != nil || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(u.Path, "/sfd_auth_") || owner == "" {
		t.Fatal("unowned audit target")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("open audit fixture")
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error("close audit fixture")
		}
	}()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	var marker string
	if db.QueryRow("SELECT owner FROM sfd_test_owner").Scan(&marker) != nil || marker != owner {
		t.Fatal("audit fixture owner")
	}
	// Use the applied public migration, not an invented temporary API schema.
	// This database is newly created and verified by the fixture owner marker.
	var existing int
	if db.QueryRow("SELECT count(*) FROM public.audit_logs").Scan(&existing) != nil || existing != 0 {
		t.Fatal("fresh migrated audit table required")
	}
	server := &Server{db: db}
	ctx := context.Background()
	start := time.Now().Add(-time.Minute)
	for _, entry := range []AuditLog{{Action: AuditActionLogin, UserID: "first", IPAddress: "127.0.0.1", Success: true, Details: map[string]interface{}{"synthetic": true}}, {Action: AuditActionLogout, UserID: "second", IPAddress: "127.0.0.1", Success: true}} {
		if err := server.LogAudit(ctx, entry); err != nil {
			t.Fatal("insert synthetic audit")
		}
	}
	end := time.Now().Add(time.Minute)
	cases := []struct {
		name    string
		filters AuditFilters
		want    int
	}{
		{"all", AuditFilters{Limit: 10}, 2}, {"action", AuditFilters{Action: AuditActionLogin, Limit: 10}, 1},
		{"owner", AuditFilters{UserID: "first", Limit: 10}, 1}, {"bounds", AuditFilters{StartTime: start, EndTime: end, Limit: 10}, 2},
		{"after", AuditFilters{StartTime: end, Limit: 10}, 0}, {"before", AuditFilters{EndTime: start, Limit: 10}, 0},
		{"limit", AuditFilters{Limit: 1}, 1}, {"no limit", AuditFilters{Limit: 0}, 0}, {"unknown", AuditFilters{UserID: "unknown", Limit: 10}, 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			entries, err := server.GetAuditLogs(ctx, test.filters)
			if err != nil || len(entries) != test.want {
				t.Fatal("audit filter result")
			}
		})
	}
}
