package server

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type gatedUploadBody struct {
	io.Reader
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedUploadBody) Read(p []byte) (int, error) {
	g.once.Do(func() { close(g.entered); <-g.release })
	return g.Reader.Read(p)
}
func TestLifecycleOrdering(t *testing.T) {
	dsn := os.Getenv("SFD_AUTH_TEST_DSN")
	if dsn == "" || os.Getenv("SFD_LIFECYCLE_STORAGE") == "" {
		t.Skip("requires parent-owned disposable full-stack fixture; not a real database pass")
	}
	expected := os.Getenv("SFD_LIFECYCLE_DATABASE")
	run := os.Getenv("SFD_AUTH_TEST_OWNER")
	bucket := os.Getenv("SFD_LIFECYCLE_BUCKET")
	endpoint := os.Getenv("SFD_LIFECYCLE_STORAGE")
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(expected, "sfd_auth_") || u.Path != "/"+expected || !strings.HasPrefix(run, "sfd-auth-") || bucket != run || !strings.HasPrefix(endpoint, "127.0.0.1:") {
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
	var marker string
	if check.QueryRowContext(ctx, "SELECT owner FROM public.sfd_test_owner").Scan(&marker) != nil || marker != run {
		t.Fatal("fixture ownership marker mismatch")
	}
	access, secret := os.Getenv("SFD_LIFECYCLE_ACCESS"), os.Getenv("SFD_LIFECYCLE_SECRET")
	if access == "" || secret == "" {
		t.Fatal("owned storage credentials absent")
	}
	mc, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.NewString()
	if _, err = check.ExecContext(ctx, "INSERT INTO users(id,email,username,password_hash,email_verified,is_active) VALUES($1,$2,$3,'not-a-credential',TRUE,TRUE)", user, user+"@example.invalid", "lifecycle_"+user); err != nil {
		t.Fatal("owned account creation")
	}
	auth := AuthConfig{AdminUser: "fixture-admin", SessionSecret: "synthetic-lifecycle-session", DB: check}
	token, expiry, err := auth.makeSessionToken(user, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = check.ExecContext(ctx, "INSERT INTO auth_sessions(token_hash,expires_at) VALUES($1,$2)", sessionHash(token), expiry); err != nil {
		t.Fatal("owned session creation")
	}
	file := func(status string, age bool) (uuid.UUID, string) {
		id := uuid.New()
		key := "lifecycle-exact-key/" + id.String()
		if _, e := check.ExecContext(ctx, "INSERT INTO files(id,object_key,orig_name,content_type,size_bytes,user_id,status,created_at) VALUES($1,$2,'owned.txt','text/plain',3,$3,$4,CASE WHEN $5 THEN NOW()-INTERVAL '2 days' ELSE NOW() END)", id, key, user, status, age); e != nil {
			t.Fatal("owned file creation")
		}
		return id, key
	}
	request := func(id uuid.UUID) *http.Request {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		p, e := mw.CreateFormFile("file", "owned.txt")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.Write([]byte("abc")); e != nil {
			t.Fatal(e)
		}
		if e = mw.Close(); e != nil {
			t.Fatal(e)
		}
		r := httptest.NewRequest("POST", "/upload?id="+id.String(), &body).WithContext(ctx)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		r.AddCookie(&http.Cookie{Name: auth.cookieName(), Value: token, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		return r
	}
	handler := (Config{Auth: auth}).uploadHandler(check, mc, bucket)
	for _, scenario := range []string{"competing-upload", "upload-then-delete"} {
		t.Run(scenario, func(t *testing.T) {
			id, key := file("pending", false)
			r := request(id)
			gate := &gatedUploadBody{Reader: r.Body, entered: make(chan struct{}), release: make(chan struct{})}
			r.Body = io.NopCloser(gate)
			var release sync.Once
			defer release.Do(func() { close(gate.release) })
			first := make(chan int, 1)
			go func() { w := httptest.NewRecorder(); handler.ServeHTTP(w, r); first <- w.Code }()
			select {
			case <-gate.entered:
			case <-ctx.Done():
				t.Fatal("first upload did not acquire file lock")
			}
			second := make(chan int, 1)
			go func() {
				w := httptest.NewRecorder()
				if scenario == "competing-upload" {
					handler.ServeHTTP(w, request(id))
				} else {
					s := Server{db: check, minio: mc, bucket: bucket}
					s.deleteFile(w, httptest.NewRequest("DELETE", "/user/files/"+id.String(), nil).WithContext(ctx), id.String(), user, false)
				}
				second <- w.Code
			}()
			select {
			case <-second:
				t.Fatal("competing operation escaped upload lock")
			case <-time.After(100 * time.Millisecond):
			}
			release.Do(func() { close(gate.release) })
			select {
			case code := <-first:
				if code != 200 {
					t.Fatalf("first upload HTTP %d", code)
				}
			case <-ctx.Done():
				t.Fatal("upload timeout")
			}
			want := 409
			if scenario == "upload-then-delete" {
				want = 204
			}
			select {
			case code := <-second:
				if code != want {
					t.Fatalf("competing operation HTTP %d want %d", code, want)
				}
			case <-ctx.Done():
				t.Fatal("competing timeout")
			}
			obj, e := mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
			if e != nil {
				t.Fatal(e)
			}
			got, readErr := io.ReadAll(obj)
			_ = obj.Close()
			if scenario == "competing-upload" {
				if readErr != nil || string(got) != "abc" {
					t.Fatal("winning bytes corrupted")
				}
			} else {
				var exists bool
				if check.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM files WHERE id=$1)", id).Scan(&exists) != nil || exists || readErr == nil {
					t.Fatal("delete left metadata or object")
				}
			}
		})
	}
	t.Run("cleanup-lock-and-exact-key", func(t *testing.T) {
		id, key := file("failed", true)
		if _, e := mc.PutObject(ctx, bucket, key, strings.NewReader("abc"), 3, minio.PutObjectOptions{}); e != nil {
			t.Fatal(e)
		}
		tx, e := check.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = tx.Rollback() }()
		var locked string
		if tx.QueryRowContext(ctx, "SELECT id::text FROM files WHERE id=$1 FOR UPDATE", id).Scan(&locked) != nil {
			t.Fatal("owned lock")
		}
		cfg := CleanupConfig{DB: check, MinioClient: mc, Bucket: bucket, MaxAge: 24 * time.Hour}
		n, e := runCleanup(ctx, cfg)
		if e != nil || n != 0 {
			t.Fatal("cleanup did not skip locked file")
		}
		if e = tx.Rollback(); e != nil {
			t.Fatal(e)
		}
		n, e = runCleanup(ctx, cfg)
		if e != nil || n != 1 {
			t.Fatal("cleanup exact key failed")
		}
		if _, e = mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); e == nil {
			t.Fatal("cleanup left actual object")
		}
	})
	t.Run("cleanup-storage-failure-retains-state", func(t *testing.T) {
		id, key := file("failed", true)
		if _, e := mc.PutObject(ctx, bucket, key, strings.NewReader("abc"), 3, minio.PutObjectOptions{}); e != nil {
			t.Fatal(e)
		}
		unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer unavailable.Close()
		bad, e := minio.New(strings.TrimPrefix(unavailable.URL, "http://"), &minio.Options{Creds: credentials.NewStaticV4("fixture", "synthetic", "")})
		if e != nil {
			t.Fatal(e)
		}
		short, done := context.WithTimeout(ctx, time.Second)
		defer done()
		if _, e = runCleanup(short, CleanupConfig{DB: check, MinioClient: bad, Bucket: bucket, MaxAge: 24 * time.Hour}); e == nil {
			t.Fatal("storage failure accepted")
		}
		var status string
		if check.QueryRowContext(ctx, "SELECT status FROM files WHERE id=$1", id).Scan(&status) != nil || status != "failed" {
			t.Fatal("metadata forgotten")
		}
		if _, e = mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); e != nil {
			t.Fatal("bytes lost")
		}
		if _, e = runCleanup(ctx, CleanupConfig{DB: check, MinioClient: mc, Bucket: bucket, MaxAge: 24 * time.Hour}); e != nil {
			t.Fatal("owned retry failed")
		}
	})
	t.Run("manual-versus-explicit-expiry", func(t *testing.T) {
		id, key := file("hashed", false)
		if _, e := mc.PutObject(ctx, bucket, key, strings.NewReader("abc"), 3, minio.PutObjectOptions{}); e != nil {
			t.Fatal(e)
		}
		if _, e := check.ExecContext(ctx, "UPDATE files SET auto_delete=TRUE,expires_at=NOW()-INTERVAL '1 minute' WHERE id=$1", id); e != nil {
			t.Fatal(e)
		}
		cfg := CleanupConfig{DB: check, MinioClient: mc, Bucket: bucket, MaxAge: 24 * time.Hour}
		if n, e := cleanupFiles(ctx, cfg, false); e != nil || n != 0 {
			t.Fatal("manual cleanup changed expiry policy")
		}
		if n, e := runCleanup(ctx, cfg); e != nil || n != 1 {
			t.Fatal("explicit expiration not cleaned")
		}
		if _, e := mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); e == nil {
			t.Fatal("expired object retained")
		}
	})
}
