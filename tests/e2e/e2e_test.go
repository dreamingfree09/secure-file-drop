//
// Secure File Drop - End-to-End Test
//
// Purpose:
//   Validates the core upload → hash → link → download flow against real
//   Postgres and MinIO instances using dockertest. It builds a compatible
//   hash tool (native C if available, else a tiny Go fallback), starts the
//   backend with ephemeral configuration, performs an authenticated session,
//   creates file metadata, uploads content, creates a download link, and
//   verifies the downloaded payload.
//
// Usage:
//   Requires Docker available to the test runner. Run:
//     go test -v ./tests/e2e -run TestUploadHashDownloadFlow
//   Optional env:
//     SFD_MINIO_TEST_TAG  override MinIO image tag for compatibility.
//
// Notes:
//   - Network ports are dynamically mapped by dockertest; the test queries
//     assigned host ports and injects them into backend env vars.
//   - The test applies schema migrations by executing internal/db/schema.sql
//     directly via database/sql to minimize external tooling dependencies.
//   - This suite is self-contained and does not require the local docker-compose
//     stack to be running.

package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

func TestUploadHashDownloadFlow(t *testing.T) {
	// Start Postgres and MinIO using dockertest
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("could not connect to docker: %v", err)
	}

	// Postgres
	pgResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "15",
		Env: []string{
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_DB=sfd",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
	})
	if err != nil {
		t.Fatalf("could not start postgres: %v", err)
	}
	pgPort := pgResource.GetPort("5432/tcp")

	// MinIO (tag can be overridden by SFD_MINIO_TEST_TAG env var)
	tag := os.Getenv("SFD_MINIO_TEST_TAG")
	if tag == "" {
		tag = "RELEASE.2024-01-31T20-20-33Z"
	}
	minioResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "minio/minio",
		Tag:        tag,
		Cmd:        []string{"server", "/data"},
		Env: []string{
			"MINIO_ROOT_USER=minio",
			"MINIO_ROOT_PASSWORD=minio123",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
	})
	if err != nil {
		t.Fatalf("could not start minio: %v", err)
	}
	minioPort := minioResource.GetPort("9000/tcp")

	// Wait for minio to be fully ready
	if err := pool.Retry(func() error {
		resp, err := http.Get("http://localhost:" + minioPort + "/minio/health/live")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("minio not ready: %d", resp.StatusCode)
		}
		return nil
	}); err != nil {
		t.Fatalf("minio not ready: %v", err)
	}

	// Create bucket using minio-go client (avoids relying on external `mc` binary)
	mc, err := minio.New("localhost:"+minioPort, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("failed to create minio client: %v", err)
	}
	bucket := "testbucket"
	if err := mc.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{}); err != nil {
		// If bucket already exists, that's okay
		exists, err2 := mc.BucketExists(context.Background(), bucket)
		if err2 != nil || !exists {
			t.Fatalf("could not create or verify bucket: %v / %v", err, err2)
		}
	}

	// Wait for Postgres
	if err := pool.Retry(func() error {
		db, err := sql.Open("postgres", fmt.Sprintf("postgres://postgres:secret@localhost:%s/sfd?sslmode=disable", pgPort))
		if err != nil {
			return err
		}
		return db.Ping()
	}); err != nil {
		t.Fatalf("could not connect to postgres: %v", err)
	}

	// Apply migrations by executing the SQL file over the DB connection (avoid relying on `psql` binary)
	// schema.sql is located in the repo root at internal/db/schema.sql. When
	// running from the test package directory, use a relative path that climbs
	// to the repo root.
	schemaPath := "../../internal/db/schema.sql"
	if b, err := os.ReadFile(schemaPath); err != nil {
		t.Fatalf("failed to read %s: %v", schemaPath, err)
	} else {
		db, err := sql.Open("postgres", fmt.Sprintf("postgres://postgres:secret@localhost:%s/sfd?sslmode=disable", pgPort))
		if err != nil {
			t.Fatalf("failed to open db for migrations: %v", err)
		}
		defer db.Close()
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("failed to apply migrations: %v", err)
		}
	}

	// Prepare env for server
	env := os.Environ()
	env = append(env, "SFD_DB_DSN=postgres://postgres:secret@localhost:"+pgPort+"/sfd?sslmode=disable")
	// The backend currently also reads DATABASE_URL in the main entrypoint.
	env = append(env, "DATABASE_URL=postgres://postgres:secret@localhost:"+pgPort+"/sfd?sslmode=disable")
	env = append(env, "SFD_MINIO_ENDPOINT=localhost:"+minioPort)
	env = append(env, "SFD_MINIO_ACCESS_KEY=minio")
	env = append(env, "SFD_MINIO_SECRET_KEY=minio123")
	env = append(env, "SFD_MINIO_BUCKET=testbucket")
	// Also set the SFD_S3_* and SFD_BUCKET variants used by the production
	// entrypoint to avoid configuration gaps between environments.
	env = append(env, "SFD_S3_ENDPOINT=localhost:"+minioPort)
	env = append(env, "SFD_S3_ACCESS_KEY=minio")
	env = append(env, "SFD_S3_SECRET_KEY=minio123")
	env = append(env, "SFD_BUCKET=testbucket")
	env = append(env, "SFD_ADMIN_USER=admin")
	env = append(env, "SFD_ADMIN_PASS=pass")
	env = append(env, "SFD_SESSION_SECRET=secret")
	env = append(env, "SFD_DOWNLOAD_SECRET=secret2")

	// Create bucket
	// Use mc client or minio-go; for speed use mc if available in runner
	// Wait for minio to be ready
	if err := pool.Retry(func() error {
		resp, err := http.Get("http://localhost:" + minioPort + "/minio/health/live")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("minio not ready: %d", resp.StatusCode)
		}
		return nil
	}); err != nil {
		t.Fatalf("minio not ready: %v", err)
	}

	// Build a small compatible hash tool (C or Go fallback) and set SFD_HASH_TOOL
	toolPath := ""
	// Try to build the native C tool first (if gcc is available)
	cPath := "/tmp/sfd-hash-c"
	if err := exec.Command("gcc", "-o", cPath, "./native/sfd_hash.c", "./native/sfd_hash_cli.c", "-lcrypto").Run(); err == nil {
		toolPath = cPath
	} else {
		// Fallback: compile a tiny Go program that computes sha256 and emits the expected JSON.
		gPath := "/tmp/sfd-hash-go"
		src := `package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing path")
		os.Exit(2)
	}
	p := os.Args[1]
	f, err := os.Open(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer f.Close()
	h := sha256.New()
	n, _ := io.Copy(h, f)
	s := hex.EncodeToString(h.Sum(nil))
	out := map[string]interface{}{"algorithm": "sha256", "hash": s, "bytes": n}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
}
`
		if err := os.WriteFile("/tmp/sfd-hash-go.go", []byte(src), 0o644); err == nil {
			if err := exec.Command("go", "build", "-o", gPath, "/tmp/sfd-hash-go.go").Run(); err == nil {
				toolPath = gPath
			}
		}
	}
	if toolPath == "" {
		t.Fatalf("could not build a hash tool for tests")
	}
	// Export it for the server to use
	env = append(env, "SFD_HASH_TOOL="+toolPath)

	// Run server (go run) in background from the repo root
	cmd := exec.CommandContext(context.Background(), "go", "run", "./cmd/backend")
	cmd.Env = env
	cmd.Dir = "../../"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer cmd.Process.Kill()

	// Wait for readiness (longer timeout for CI environments)
	if err := retryHTTPGet("http://localhost:8080/ready", 90*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	// Login
	client := &http.Client{}
	loginReq := map[string]string{"username": "admin", "password": "pass"}
	b, _ := json.Marshal(loginReq)
	req, _ := http.NewRequest(http.MethodPost, "http://localhost:8080/login", bytesReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	resp.Body.Close()
	// Extract cookie
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatalf("no cookies from login")
	}

	// Create file metadata
	metaReq := map[string]interface{}{"orig_name": "e2e.txt", "content_type": "text/plain", "size_bytes": 4}
	mb, _ := json.Marshal(metaReq)
	req, _ = http.NewRequest(http.MethodPost, "http://localhost:8080/files", bytesReader(mb))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("create file returned %d", resp.StatusCode)
	}
	var metaResp struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&metaResp)
	resp.Body.Close()

	// Upload file
	body := bytesFromString("test")
	bReq := newMultipartUploadRequest("http://localhost:8080/upload?id="+metaResp.ID, "file", "e2e.txt", body, cookies)
	resp, err = client.Do(bReq)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("upload returned %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create link
	linkReq := map[string]interface{}{"id": metaResp.ID, "ttl_seconds": 60}
	lb, _ := json.Marshal(linkReq)
	req, _ = http.NewRequest(http.MethodPost, "http://localhost:8080/links", bytesReader(lb))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("create link failed: %v", err)
	}
	var linkResp struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&linkResp)
	resp.Body.Close()

	// Download the file via link
	dRes, err := http.Get(linkResp.URL)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if dRes.StatusCode != 200 {
		t.Fatalf("download status %d", dRes.StatusCode)
	}
	defer dRes.Body.Close()
	data, _ := io.ReadAll(dRes.Body)
	if string(data) != "test" {
		t.Fatalf("downloaded content mismatch: %s", string(data))
	}
}

// helpers

func retryHTTPGet(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

// small helpers avoid importing extra packages in test header
func bytesReader(b []byte) *bytesReaderType { return &bytesReaderType{b: b} }

type bytesReaderType struct{ b []byte }

func (r *bytesReaderType) Read(p []byte) (int, error) { return copy(p, r.b), io.EOF }

func bytesFromString(s string) []byte { return []byte(s) }

func newMultipartUploadRequest(url, fieldname, filename string, content []byte, cookies []*http.Cookie) *http.Request {
	// For brevity, create a simple request using standard library multipart in-memory
	pr, pw := io.Pipe()
	writer := multipartNewWriter(pw)
	go func() {
		_ = writer.WriteField("dummy", "1")
		part, _ := writer.CreateFormFile(fieldname, filename)
		part.Write(content)
		writer.Close()
		pw.Close()
	}()
	req, _ := http.NewRequest(http.MethodPost, url, pr)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

// minimal multipart writer wrapper to avoid big imports
func multipartNewWriter(w io.Writer) *multipartWriter { return &multipartWriter{w: w} }

// very small writer implementation (not robust, but sufficient for this test)
type multipartWriter struct{ w io.Writer }

func (m *multipartWriter) WriteField(key, val string) error {
	_, err := fmt.Fprintf(m.w, "--boundary\r\nContent-Disposition: form-data; name=\"%s\"\r\n\r\n%s\r\n", key, val)
	return err
}
func (m *multipartWriter) CreateFormFile(fieldname, filename string) (io.Writer, error) {
	_, err := fmt.Fprintf(m.w, "--boundary\r\nContent-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\nContent-Type: application/octet-stream\r\n\r\n", fieldname, filename)
	if err != nil {
		return nil, err
	}
	return m.w, nil
}
func (m *multipartWriter) FormDataContentType() string {
	return "multipart/form-data; boundary=boundary"
}
func (m *multipartWriter) Close() error {
	_, err := fmt.Fprint(m.w, "\r\n--boundary--\r\n")
	return err
}
