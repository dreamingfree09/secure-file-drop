package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Fault-injection observation only: no external database, storage or credentials.
type assessmentConnector struct{ observed *context.Context }

func (c assessmentConnector) Connect(context.Context) (driver.Conn, error) {
	return assessmentConn(c), nil
}
func (c assessmentConnector) Driver() driver.Driver { return assessmentDriver{} }

type assessmentDriver struct{}

func (assessmentDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("connector required")
}

type assessmentConn struct{ observed *context.Context }

func (assessmentConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not used") }
func (assessmentConn) Close() error                        { return nil }
func (assessmentConn) Begin() (driver.Tx, error)           { return nil, errors.New("not used") }
func (c assessmentConn) QueryContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	*c.observed = ctx
	return nil, errors.New("synthetic database failure")
}

// Canceled requests must not issue SQL with a live background context.
func TestCanceledRequestDoesNotIssueUncancelableSQL(t *testing.T) {
	for _, route := range []string{"registration", "login", "download"} {
		t.Run(route, func(t *testing.T) {
			var observed context.Context
			db := sql.OpenDB(assessmentConnector{&observed})
			defer func() { _ = db.Close() }()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			a := AuthConfig{DB: db, AdminUser: "admin", SessionSecret: "synthetic-assessment-secret"}
			cfg := Config{DB: db, Auth: a}
			r := httptest.NewRequest("POST", "http://fixture.test/register", strings.NewReader("{\"email\":\"fixture@example.test\",\"username\":\"fixture\",\"password\":\"SyntheticPass123\"}")).WithContext(ctx)
			w := httptest.NewRecorder()
			switch route {
			case "registration":
				cfg.RegisterHandler(w, r)
			case "login":
				r = httptest.NewRequest("POST", "http://fixture.test/login", strings.NewReader("{\"username\":\"fixture\",\"password\":\"SyntheticPass123\"}")).WithContext(ctx)
				a.loginHandler().ServeHTTP(w, r)
			case "download":
				t.Setenv("SFD_DOWNLOAD_SECRET", "synthetic-assessment-download-secret")
				tok, err := signDownloadToken("7f9c4604-cc9c-4b17-8c80-87c1a1fa01a0", time.Now().Add(time.Minute))
				if err != nil {
					t.Fatal(err)
				}
				r = httptest.NewRequest("GET", "http://fixture.test/download?token="+tok, nil).WithContext(ctx)
				cfg.downloadHandler(db, nil, "").ServeHTTP(w, r)
			}
			if observed != nil && observed.Err() == nil {
				t.Fatal("SQL lost request cancellation")
			}
			if w.Code != 500 {
				t.Fatalf("unexpected HTTP %d", w.Code)
			}

		})
	}
}

func TestAssessmentJSONEnvelopeRejectsOversizeAndTrailingData(t *testing.T) {
	a := AuthConfig{AdminUser: "admin", AdminPass: "SyntheticPass123", SessionSecret: "synthetic-assessment-session-secret"}
	valid := "{\"username\":\"admin\",\"password\":\"SyntheticPass123\"}"
	for _, body := range []string{valid + strings.Repeat(" ", 17<<10), valid + "{}"} {
		r := httptest.NewRequest("POST", "http://fixture.test/login", strings.NewReader(body))
		w := httptest.NewRecorder()
		a.loginHandler().ServeHTTP(w, r)
		if w.Code < 400 {
			t.Errorf("accepted invalid JSON envelope with HTTP %d: bytes=%d; expected 400/413", w.Code, len(body))
		}
	}
}
