package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/crypto/bcrypt"
)

// This SMTP peer binds only loopback, accepts synthetic recipients, and never
// logs message contents. Delay the final DATA acknowledgement, not a mock call.
func correctionSMTP(t *testing.T, delay time.Duration, reject bool, entered chan<- struct{}) (*EmailService, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("owned SMTP listener")
	}
	t.Cleanup(func() { _ = ln.Close() })
	count := &atomic.Int32{}
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(12 * time.Second))
		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		reply := func(s string) bool {
			_, e = fmt.Fprint(rw, s+"\r\n")
			if e != nil {
				return false
			}
			return rw.Flush() == nil
		}
		if !reply("220 localhost fixture") {
			return
		}
		for {
			line, e := rw.ReadString('\n')
			if e != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				if !reply("250-localhost\r\n250 AUTH PLAIN") {
					return
				}
			case strings.HasPrefix(line, "AUTH"):
				if !reply("235 fixture accepted") {
					return
				}
			case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
				if !reply("250 fixture accepted") {
					return
				}
			case strings.HasPrefix(line, "DATA"):
				if !reply("354 fixture data") {
					return
				}
				for {
					line, e = rw.ReadString('\n')
					if e != nil {
						return
					}
					if line == ".\r\n" {
						break
					}
				}
				if entered != nil {
					entered <- struct{}{}
				}
				time.Sleep(delay)
				if reject {
					_ = reply("451 fixture unavailable")
					return
				}
				if !reply("250 fixture stored") {
					return
				}
				count.Add(1)
			case strings.HasPrefix(line, "QUIT"):
				_ = reply("221 fixture closing")
				return
			default:
				return
			}
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return NewEmailService(EmailConfig{Enabled: true, SMTPHost: "localhost", SMTPPort: port, SMTPUser: "fixture", SMTPPassword: "synthetic-only", FromEmail: "fixture@example.invalid"}), count
}

func TestCorrectionRegistrationTiming(t *testing.T) {
	db := canonicalOwnedAuthDB(t)
	for _, test := range []struct {
		name                   string
		delay                  time.Duration
		reject, cancel, dbFail bool
		status                 int
	}{
		{name: "fast", status: 201},
		{name: "delayed beyond original SQL budget", delay: 6 * time.Second, status: 201},
		{name: "SMTP failure", reject: true, status: 503},
		{name: "database insertion failure", dbFail: true, status: 500},
		{name: "request cancellation during delivery", delay: time.Second, cancel: true, status: 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			svc, count := correctionSMTP(t, test.delay, test.reject, entered)
			name := "reg" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if test.dbFail {
				// Only this freshly marked acceptance database is allowed above.
				_, e := db.Exec("ALTER TABLE users ADD CONSTRAINT sfd_registration_fixture CHECK(username <> '" + name + "') NOT VALID")
				if e != nil {
					t.Fatal("install owned insertion fault")
				}
				defer func() {
					if _, e := db.Exec("ALTER TABLE users DROP CONSTRAINT sfd_registration_fixture"); e != nil {
						t.Error("remove owned insertion fault")
					}
				}()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest("POST", "/register", strings.NewReader(fmt.Sprintf(`{"email":"%s@example.invalid","username":"%s","password":"SyntheticPass123"}`, name, name))).WithContext(ctx)
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				(Config{DB: db, EmailSvc: svc, BaseURL: "https://fixture.invalid"}).RegisterHandler(w, r)
				close(done)
			}()
			if test.cancel {
				select {
				case <-entered:
					cancel()
				case <-time.After(5 * time.Second):
					t.Fatal("delivery not entered")
				}
			}
			select {
			case <-done:
			case <-time.After(12 * time.Second):
				t.Fatal("registration exceeded finite delivery budget")
			}
			if w.Code != test.status {
				t.Fatalf("registration status got%d want%d", w.Code, test.status)
			}
			var present int
			if db.QueryRow("SELECT count(*) FROM users WHERE username=$1", name).Scan(&present) != nil {
				t.Fatal("read owned registration state")
			}
			if test.status != 201 {
				if present != 0 {
					t.Fatal("failed registration retained an account")
				}
				if test.dbFail && count.Load() != 0 {
					t.Fatal("insertion failure delivered mail")
				}
				return
			}
			var hash, token string
			var admin, verified bool
			if present != 1 || db.QueryRow("SELECT password_hash,verification_token,is_admin,email_verified FROM users WHERE username=$1", name).Scan(&hash, &token, &admin, &verified) != nil {
				t.Fatal("successful delivery lost registration")
			}
			cost, e := bcrypt.Cost([]byte(hash))
			if e != nil || cost != 12 || bcrypt.CompareHashAndPassword([]byte(hash), []byte("SyntheticPass123")) != nil || admin || verified || len(token) != 64 || count.Load() != 1 {
				t.Fatal("password, role, or verification invariant")
			}
			verify := httptest.NewRecorder()
			(Config{DB: db}).VerifyEmailHandler(verify, httptest.NewRequest("GET", "/verify?token="+token, nil))
			if verify.Code != 200 {
				t.Fatal("delivered registration cannot be verified")
			}
		})
	}
}

func TestCorrectionSMTPCancellation(t *testing.T) {
	entered := make(chan struct{}, 1)
	svc, _ := correctionSMTP(t, time.Second, false, entered)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- svc.sendVerificationEmailContext(ctx, "fixture@example.invalid", "synthetic-token", "https://fixture.invalid")
	}()
	select {
	case <-entered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("SMTP not entered")
	}
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("canceled SMTP reported success")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SMTP ignored request cancellation")
	}
}

func correctionStorage(t *testing.T) (*minio.Client, string) {
	t.Helper()
	run := os.Getenv("SFD_AUTH_TEST_OWNER")
	endpoint := os.Getenv("SFD_LIFECYCLE_STORAGE")
	bucket := os.Getenv("SFD_LIFECYCLE_BUCKET")
	if endpoint == "" {
		t.Skip("requires freshly owned storage fixture; not an acceptance pass")
	}
	if !strings.HasPrefix(run, "sfd-auth-") || bucket != run || !strings.HasPrefix(endpoint, "127.0.0.1:") {
		t.Fatal("refusing unowned storage")
	}
	mc, e := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(os.Getenv("SFD_LIFECYCLE_ACCESS"), os.Getenv("SFD_LIFECYCLE_SECRET"), ""), Secure: false})
	if e != nil {
		t.Fatal("owned storage client")
	}
	return mc, bucket
}

type correctionInterruptedWriter struct{ *httptest.ResponseRecorder }

func (w correctionInterruptedWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("synthetic disconnected client")
}

func TestCorrectionDownloadOutcomes(t *testing.T) {
	db := canonicalOwnedAuthDB(t)
	mc, bucket := correctionStorage(t)
	t.Setenv("SFD_DOWNLOAD_SECRET", strings.Repeat("d", 32))
	for _, name := range []string{"empty", "completed", "interrupted", "storage missing", "stored size mismatch"} {
		t.Run(name, func(t *testing.T) {
			body := []byte("synthetic download integrity payload")
			if name == "empty" {
				body = nil
			}
			id := uuid.NewString()
			key := "correction/" + id
			if name != "storage missing" {
				if _, e := mc.PutObject(context.Background(), bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{}); e != nil {
					t.Fatal("put owned synthetic object")
				}
			}
			size := len(body)
			if name == "stored size mismatch" {
				size++
			}
			if _, e := db.Exec("INSERT INTO files(id,object_key,orig_name,content_type,size_bytes,status) VALUES($1,$2,'synthetic.bin','application/octet-stream',$3,'hashed')", id, key, size); e != nil {
				t.Fatal("insert owned file")
			}
			token, e := signDownloadToken(id, time.Now().Add(time.Minute))
			if e != nil {
				t.Fatal("sign synthetic link")
			}
			before := GetMetrics().Snapshot()
			w := httptest.NewRecorder()
			var writer http.ResponseWriter = w
			if name == "interrupted" {
				writer = correctionInterruptedWriter{w}
			}
			(Config{}).downloadHandler(db, mc, bucket).ServeHTTP(writer, httptest.NewRequest("GET", "/download?token="+token, nil))
			after := GetMetrics().Snapshot()
			success := name == "completed" || name == "empty"
			if success {
				if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), body) || after.DownloadsTotal-before.DownloadsTotal != 1 || after.DownloadBytesTotal-before.DownloadBytesTotal != int64(len(body)) || after.DownloadErrorsTotal != before.DownloadErrorsTotal {
					t.Fatal("completed transfer integrity or single success metric")
				}
			} else if after.DownloadsTotal != before.DownloadsTotal || after.DownloadBytesTotal != before.DownloadBytesTotal || after.DownloadErrorsTotal-before.DownloadErrorsTotal != 1 {
				t.Fatal("failed transfer not counted exactly once")
			}
			// Poll only the successful asynchronous DB statistics update.
			deadline := time.Now().Add(2 * time.Second)
			var count int
			for {
				if db.QueryRow("SELECT download_count FROM files WHERE id=$1", id).Scan(&count) != nil {
					t.Fatal("read owned statistics")
				}
				if !success || count == 1 || time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if success && count != 1 || !success && count != 0 {
				t.Fatal("database counter diverged from actual transfer outcome")
			}
			prom := httptest.NewRecorder()
			PrometheusMetricsHandler().ServeHTTP(prom, httptest.NewRequest("GET", "/metrics", nil))
			for key, value := range map[string]int64{"sfd_downloads_total": after.DownloadsTotal, "sfd_download_bytes_total": after.DownloadBytesTotal, "sfd_download_errors_total": after.DownloadErrorsTotal} {
				if !strings.Contains(prom.Body.String(), fmt.Sprintf("%s %d\n", key, value)) {
					t.Fatal("Prometheus counter differs from transfer outcome")
				}
			}
		})
	}
}

func TestCorrectionCleanupRuntime(t *testing.T) {
	db := canonicalOwnedAuthDB(t)
	mc, bucket := correctionStorage(t)
	old := uuid.NewString()
	fresh := uuid.NewString()
	ready := uuid.NewString()
	for _, id := range []string{old, fresh, ready} {
		status := "pending"
		age := "NOW()-INTERVAL '2 days'"
		if id == ready {
			status = "hashed"
		}
		if id == fresh {
			age = "NOW()"
		}
		if _, e := db.Exec("INSERT INTO files(id,object_key,orig_name,content_type,size_bytes,status,created_at) VALUES($1,$2,'cleanup.bin','application/octet-stream',0,$3,"+age+")", id, "correction/"+id, status); e != nil {
			t.Fatal("create owned cleanup record")
		}
		if _, e := mc.PutObject(context.Background(), bucket, "correction/"+id, strings.NewReader(""), 0, minio.PutObjectOptions{}); e != nil {
			t.Fatal("create owned cleanup object")
		}
	}
	t.Setenv("SFD_CLEANUP_ENABLED", "false")
	t.Setenv("SFD_CLEANUP_INTERVAL", "10ms")
	t.Setenv("SFD_CLEANUP_MAX_AGE", "24h")
	t.Setenv("SFD_CLEANUP_MAX_AGE_HOURS", "")
	off := GetCleanupConfigFromEnv(db, mc, bucket)
	StartCleanupJob(context.Background(), off)
	var n int
	if db.QueryRow("SELECT count(*) FROM files WHERE id=$1", old).Scan(&n) != nil || n != 1 {
		t.Fatal("disabled cleanup touched synthetic record")
	}
	t.Setenv("SFD_CLEANUP_ENABLED", "true")
	on := GetCleanupConfigFromEnv(db, mc, bucket)
	if !on.Enabled || on.Interval != 10*time.Millisecond || on.MaxAge != 24*time.Hour {
		t.Fatal("cleanup parameters were not forwarded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { StartCleanupJob(ctx, on); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if db.QueryRow("SELECT count(*) FROM files WHERE id=$1", old).Scan(&n) != nil {
			t.Fatal("read owned cleanup state")
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("enabled cleanup did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup ignored cancellation")
	}
	if db.QueryRow("SELECT count(*) FROM files WHERE id=$1 OR id=$2", fresh, ready).Scan(&n) != nil || n != 2 {
		t.Fatal("cleanup removed fresh or completed synthetic file")
	}
	for _, id := range []string{fresh, ready} {
		if _, e := mc.StatObject(context.Background(), bucket, "correction/"+id, minio.StatObjectOptions{}); e != nil {
			t.Fatal("cleanup removed preserved synthetic object")
		}
	}
	_, e := mc.StatObject(context.Background(), bucket, "correction/"+old, minio.StatObjectOptions{})
	if minio.ToErrorResponse(e).Code != "NoSuchKey" {
		t.Fatal("cleanup retained expired synthetic object")
	}
}

// Keep io imported as a compile-time check on the interruption adapter.
var _ io.Writer = correctionInterruptedWriter{}
