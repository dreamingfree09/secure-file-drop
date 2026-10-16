package server

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// This fixture replaces only the driver COMMIT acknowledgement. PostgreSQL and
// object storage are real; it is not a wire/network fault reproduction.
type commitFaultConnector struct {
	driver.Connector
	mode  string
	fired atomic.Bool
}

func (c *commitFaultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &commitFaultConn{Conn: conn, owner: c}, nil
}

type commitFaultConn struct {
	driver.Conn
	owner *commitFaultConnector
}

func (c *commitFaultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if strings.HasPrefix(c.owner.mode, "unknown") && c.owner.fired.Load() {
		return nil, errors.New("synthetic reconciliation unavailable")
	}
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &commitFaultTx{Tx: tx, owner: c.owner}, nil
}
func (c *commitFaultConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}
func (c *commitFaultConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}
func (c *commitFaultConn) CheckNamedValue(v *driver.NamedValue) error {
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(v)
	}
	return driver.ErrSkip
}

type commitFaultTx struct {
	driver.Tx
	owner *commitFaultConnector
}

func (tx *commitFaultTx) Commit() error {
	if !tx.owner.fired.CompareAndSwap(false, true) {
		return tx.Tx.Commit()
	}
	var err error
	if strings.Contains(tx.owner.mode, "rollback") {
		err = tx.Rollback()
	} else {
		err = tx.Tx.Commit()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

func TestDisposableCommitFaults(t *testing.T) {
	dsn := os.Getenv("SFD_CORRECTION_FIXTURE_DSN")
	if dsn == "" {
		t.Skip("requires parent-owned disposable full-stack fixture; not a real database pass")
	}
	expected := os.Getenv("SFD_CORRECTION_EXPECTED_DB")
	run := os.Getenv("SFD_CORRECTION_RUN")
	bucket := os.Getenv("SFD_CORRECTION_BUCKET")
	endpoint := os.Getenv("SFD_CORRECTION_STORAGE")
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(expected, "sfd_quality_") || u.Path != "/"+expected || !strings.HasPrefix(run, "sfd-quality-") || bucket != run+"-private" || !strings.HasPrefix(endpoint, "127.0.0.1:") {
		t.Fatal("refusing unverified fixture target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pgcfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	check := stdlib.OpenDB(*pgcfg)
	defer func() { _ = check.Close() }()
	var actual string
	if err = check.QueryRowContext(ctx, "SELECT current_database()").Scan(&actual); err != nil || actual != expected {
		t.Fatal("unexpected fixture database")
	}
	mc, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4("sfdtest", "SyntheticPass123456", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"accepted", "rollback", "unknown-accepted", "unknown-rollback"} {
		t.Run(mode, func(t *testing.T) {
			connector := &commitFaultConnector{Connector: stdlib.GetConnector(*pgcfg), mode: mode}
			pool := sql.OpenDB(connector)
			pool.SetMaxOpenConns(1)
			defer func() { _ = pool.Close() }()
			id := uuid.New()
			key := "uploads/" + id.String()
			if _, err := check.ExecContext(ctx, "INSERT INTO files(id,object_key,orig_name,content_type,size_bytes,created_by,status) VALUES($1,$2,'commit.bin','text/plain',3,'fixture-admin','pending')", id, key); err != nil {
				t.Fatal(err)
			}
			a := AuthConfig{AdminUser: "fixture-admin", SessionSecret: "synthetic-commit-fault-session"}
			token, _, err := a.makeSessionToken("fixture-admin", true)
			if err != nil {
				t.Fatal(err)
			}
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			p, err := mw.CreateFormFile("file", "commit.bin")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Write([]byte("abc")); err != nil {
				t.Fatal(err)
			}
			if err = mw.Close(); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/upload?id="+id.String(), &body).WithContext(ctx)
			r.Header.Set("Content-Type", mw.FormDataContentType())
			r.AddCookie(a.cookie(r, token, time.Now().Add(time.Hour)))
			w := httptest.NewRecorder()
			(Config{Auth: a}).uploadHandler(pool, mc, bucket).ServeHTTP(w, r)
			if w.Code != 500 || !connector.fired.Load() {
				t.Fatalf("fault not exercised HTTP %d", w.Code)
			}
			var status string
			if err := check.QueryRowContext(ctx, "SELECT status FROM files WHERE id=$1", id).Scan(&status); err != nil {
				t.Fatal(err)
			}
			want := "hashed"
			switch mode {
			case "rollback":
				want = "failed"
			case "unknown-rollback":
				want = "pending"
			}
			if status != want {
				t.Fatalf("state %q want %q", status, want)
			}
			_, statErr := mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
			if mode == "rollback" {
				if statErr == nil {
					t.Fatal("known rollback retained object")
				}
			} else if statErr != nil {
				t.Fatal("possibly committed bytes deleted", statErr)
			}
			if pool.Stats().InUse != 0 {
				t.Fatal("fault cleanup leaked pool connection")
			}
			t.Log("PASS: real PostgreSQL/storage with synthetic driver acknowledgement fault; state=" + status)
		})
	}
}
