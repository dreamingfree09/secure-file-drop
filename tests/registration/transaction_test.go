package registration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"secure-file-drop/internal/server"
)

type commitConnector struct {
	base driver.Connector
	mode string
}

func (c commitConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return commitConnection{Conn: conn, mode: c.mode}, nil
}
func (c commitConnector) Driver() driver.Driver { return c.base.Driver() }

type commitConnection struct {
	driver.Conn
	mode string
}

func (c commitConnection) Begin() (driver.Tx, error) {
	tx, err := c.Conn.Begin()
	if err != nil {
		return nil, err
	}
	return commitTransaction{Tx: tx, mode: c.mode}, nil
}

type commitTransaction struct {
	driver.Tx
	mode string
}

func (tx commitTransaction) Commit() error {
	if tx.mode == "before" {
		_ = tx.Tx.Rollback()
		return errors.New("owned fixture reports failed commit before application")
	}
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	return errors.New("owned fixture loses acknowledgement after actual commit")
}

func TestRegistrationUnknownCommit(t *testing.T) {
	dsn := os.Getenv("SFD_R4_TEST_DSN")
	if dsn == "" {
		t.Skip("requires an independently owned hosted PostgreSQL fixture; not acceptance")
	}
	parsed, err := url.Parse(dsn)
	owner := os.Getenv("SFD_R4_TEST_OWNER")
	if err != nil || parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" || parsed.Path != "/sfd" ||
		!strings.HasPrefix(owner, "sfd-rorschach-") || os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Fatal("refusing an unqualified database target")
	}
	base, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("fixture connection failed")
	}
	defer func() { _ = base.Close() }()
	var marker string
	if base.QueryRow("SELECT owner FROM r4_fixture_ownership").Scan(&marker) != nil || marker != owner {
		t.Fatal("fixture ownership marker mismatch")
	}
	password := os.Getenv("SFD_R1_FIXTURE_USER_PASSWORD")
	if len(password) < 16 {
		t.Fatal("missing synthetic credential")
	}
	for _, mode := range []string{"before", "after"} {
		t.Run(mode, func(t *testing.T) {
			var secret [32]byte
			if _, err := rand.Read(secret[:]); err != nil {
				t.Fatal("fixture entropy unavailable")
			}
			token := hex.EncodeToString(secret[:])
			digest := sha256.Sum256([]byte(token))
			id := uuid.New()
			_, err := base.Exec("INSERT INTO registration_invitations(id,token_sha256,issuer,expires_at) VALUES($1,$2,'transaction-fixture',now()+interval '1 hour')", id, hex.EncodeToString(digest[:]))
			if err != nil {
				t.Fatal("fixture invitation setup failed")
			}
			connector, err := pq.NewConnector(dsn)
			if err != nil {
				t.Fatal("fixture connector failed")
			}
			fault := sql.OpenDB(commitConnector{base: connector, mode: mode})
			defer func() { _ = fault.Close() }()
			cfg := server.Config{DB: fault, RegistrationInvitations: true, Auth: server.AuthConfig{AdminUser: "admin"}}
			https := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cfg.RedeemRegistrationInvitation(w, r) }))
			defer https.Close()
			t.Setenv("SFD_PUBLIC_BASE_URL", https.URL)
			username := "r4_commit_" + mode
			body, err := json.Marshal(map[string]string{"token": token, "username": username, "email": username + "@example.invalid", "password": password})
			if err != nil {
				t.Fatal("fixture request failed")
			}
			post := func(expected int) {
				request, err := http.NewRequest(http.MethodPost, https.URL, bytes.NewReader(body))
				if err != nil {
					t.Fatal("fixture request setup failed")
				}
				request.Header.Set("Origin", https.URL)
				request.Header.Set("Content-Type", "application/json")
				response, err := https.Client().Do(request)
				if err != nil {
					t.Fatal("fixture HTTPS request failed")
				}
				defer func() { _ = response.Body.Close() }()
				if response.StatusCode != expected || response.Header.Get("Set-Cookie") != "" {
					t.Fatal("unexpected commit response or session disclosure")
				}
				data, _ := io.ReadAll(response.Body)
				if bytes.Contains(data, []byte(token)) || bytes.Contains(data, []byte(password)) {
					t.Fatal("credential disclosure")
				}
			}
			post(503)
			var accounts int
			var consumed bool
			if base.QueryRow("SELECT count(*) FROM users WHERE username=$1", username).Scan(&accounts) != nil ||
				base.QueryRow("SELECT redeemed_at IS NOT NULL FROM registration_invitations WHERE id=$1", id).Scan(&consumed) != nil {
				t.Fatal("commit readback failed")
			}
			if (mode == "after" && (accounts != 1 || !consumed)) || (mode == "before" && (accounts != 0 || consumed)) {
				t.Fatal("account and invitation are not atomic")
			}
			cfg.DB = base
			if mode == "after" {
				post(410)
			} else {
				post(201)
			}
			var verified, admin bool
			var quota int64
			var method string
			if base.QueryRow("SELECT email_verified,is_admin,storage_quota_bytes,registration_method FROM users WHERE username=$1", username).Scan(&verified, &admin, &quota, &method) != nil || verified || admin || quota != 10<<30 || method != "administrator_invitation" {
				t.Fatal("ordinary account provenance or quota violated")
			}
		})
	}
}
