package e2e

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"secure-file-drop/internal/db"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// This test owns a fresh network, database, buckets, volumes, image and containers.
// It never uses application environment files or existing service endpoints.
func TestUploadHashDownloadFlow(t *testing.T) {
	if os.Getenv("SFD_E2E") != "1" {
		t.Skip("full-stack Docker test requires SFD_E2E=1; not a full-stack pass")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Minute)
	defer cancel()
	cli := os.Getenv("SFD_TEST_DOCKER")
	if cli == "" {
		cli = "docker"
	}
	call := func(args ...string) (string, error) {
		// #nosec G204 G702 -- Test operator selects the Docker CLI; fixed arguments, no shell.
		c := exec.CommandContext(ctx, cli, args...)
		c.Dir = "../.."
		b, e := c.Output()
		if e != nil {
			var exit *exec.ExitError
			if errors.As(e, &exit) {
				b = append(b, exit.Stderr...)
			}
		}
		return strings.TrimSpace(string(b)), e
	}
	docker := func(args ...string) string {
		t.Helper()
		out, e := call(args...)
		if e != nil {
			t.Fatalf("Docker %s failed: %v\n%s", args[0], e, out)
		}
		return out
	}
	endpoint := docker("context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	if os.Getenv("DOCKER_HOST") != "" || (!strings.HasPrefix(endpoint, "unix://") && !strings.HasPrefix(endpoint, "npipe://")) {
		t.Fatal("only the local Docker engine is permitted")
	}
	docker("info", "--format", "{{.OSType}}")
	run := "sfd-quality-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	label := "sfd.quality-run=" + run
	own := func(kind, name string) {
		t.Helper()
		t.Cleanup(func() {
			// Cleanup has its own deadline because the test context may have expired.
			cleanCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
			defer done()
			format := "{{index .Labels \"sfd.quality-run\"}}"
			if kind == "container" || kind == "image" {
				format = "{{index .Config.Labels \"sfd.quality-run\"}}"
			}
			// #nosec G204 G702 -- Test operator selects the Docker CLI; fixed arguments, no shell.
			c := exec.CommandContext(cleanCtx, cli, kind, "inspect", "--format", format, name)
			out, e := c.CombinedOutput()
			if e != nil || strings.TrimSpace(string(out)) != run {
				t.Errorf("refusing cleanup: ownership not verified for %s %s", kind, name)
				return
			}
			args := []string{kind, "rm"}
			if kind == "container" || kind == "image" {
				args = append(args, "--force")
			}
			args = append(args, name)
			// #nosec G204 G702 -- Test operator selects the CLI; cleanup uses fixed subcommands after label verification.
			if b, e := exec.CommandContext(cleanCtx, cli, args...).CombinedOutput(); e != nil {
				t.Errorf("owned resource cleanup failed: %v %s", e, b)
			}
		})
	}
	network := run + "-net"
	docker("network", "create", "--label", label, network)
	own("network", network)
	pgVol := run + "-pgdata"
	miVol := run + "-objects"
	for _, v := range []string{pgVol, miVol} {
		docker("volume", "create", "--label", label, v)
		own("volume", v)
	}
	pgName := run + "-pg"
	miName := run + "-minio"
	appName := run + "-app"
	image := run + ":test"
	storageImage := run + ":storage"
	database := "sfd_quality_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "SyntheticPass123456"
	secret := "synthetic-download-secret-for-isolated-tests"
	docker("build", "--label", label, "-t", storageImage, "-f", "Dockerfile.storage", ".")
	own("image", storageImage)
	pg := docker("run", "-d", "--name", pgName, "--label", label, "--network", network, "--network-alias", "pg",
		"-p", "127.0.0.1::5432", "-v", pgVol+":/var/lib/postgresql/data",
		"-e", "POSTGRES_DB="+database, "-e", "POSTGRES_USER=sfd", "-e", "POSTGRES_PASSWORD="+password, "postgres:16")
	own("container", pg)
	mi := docker("run", "-d", "--name", miName, "--label", label, "--network", network, "--network-alias", "minio",
		"-p", "127.0.0.1::9000", "-v", miVol+":/data", "-e", "MINIO_ROOT_USER=sfdtest", "-e", "MINIO_ROOT_PASSWORD="+password,
		storageImage, "server", "/data")
	own("container", mi)
	port := func(id, p string) string {
		out := docker("port", id, p)
		if !strings.HasPrefix(out, "127.0.0.1:") {
			t.Fatalf("non-loopback port: %s", out)
		}
		return out
	}
	pgAddr := port(pg, "5432/tcp")
	miAddr := port(mi, "9000/tcp")
	dsn := "postgres://sfd:" + password + "@" + pgAddr + "/" + database + "?sslmode=disable"
	conn, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	wait := func(name string, f func() bool) {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			if f() {
				return
			}
			time.Sleep(300 * time.Millisecond)
		}
		t.Fatalf("%s did not become ready", name)
	}
	wait("fresh postgres", func() bool { return conn.PingContext(ctx) == nil })
	var gotDB, gotUser string
	if e = conn.QueryRow("SELECT current_database(),current_user").Scan(&gotDB, &gotUser); e != nil || gotDB != database || gotUser != "sfd" {
		t.Fatal("unexpected database target")
	}
	var tables int
	if e = conn.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&tables); e != nil || tables != 0 {
		t.Fatal("database not fresh")
	}
	mc, e := minio.New(miAddr, &minio.Options{Creds: credentials.NewStaticV4("sfdtest", password, ""), Secure: false})
	if e != nil {
		t.Fatal(e)
	}
	wait("fresh minio", func() bool { _, e := mc.ListBuckets(ctx); return e == nil })
	bucket := run + "-private"
	if e = mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); e != nil {
		t.Fatal(e)
	}

	// Validate the real legacy schema upgrade and idempotent forward migration on
	// another newly created database in this assignment-owned PostgreSQL volume.
	legacy := database + "_legacy"
	if _, e = conn.Exec("CREATE DATABASE " + legacy); e != nil {
		t.Fatal(e)
	}
	legacyDB, e := sql.Open("pgx", strings.Replace(dsn, "/"+database+"?", "/"+legacy+"?", 1))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range []string{"schema.sql", "alter_001.sql"} {
		// #nosec G304 -- f comes only from the two constant legacy fixture names above.
		b, e := os.ReadFile(filepath.Join("../../internal/db", f))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = legacyDB.Exec(string(b)); e != nil {
			t.Fatal(e)
		}
	}
	legacyDB.SetMaxOpenConns(1)
	if e = db.RunMigrations(legacyDB); e != nil {
		t.Fatal(e)
	}
	if e = db.RunMigrations(legacyDB); e != nil {
		t.Fatal(e)
	}

	for i := 0; i < 5; i++ {
		if err := db.RunMigrations(legacyDB); err != nil {
			t.Fatal(err)
		}
		if legacyDB.Stats().InUse != 0 {
			t.Fatal("migration retained connection")
		}
	}
	if _, err := legacyDB.Exec("UPDATE schema_migrations SET dirty=true"); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(legacyDB); err == nil {
		t.Fatal("expected dirty-schema error")
	}
	if legacyDB.Stats().InUse != 0 {
		t.Fatal("migration error retained connection")
	}
	if err := legacyDB.PingContext(ctx); err != nil {
		t.Fatal("migration closed shared pool", err)
	}
	if _, err := legacyDB.Exec("UPDATE schema_migrations SET dirty=false"); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(legacyDB); err != nil {
		t.Fatal(err)
	}
	t.Log("PASS: repeated legacy migrations, MaxOpenConns=1, dirty-error cleanup and usable shared pool")
	if e = legacyDB.Close(); e != nil {
		t.Fatal(e)
	}

	docker("build", "--label", label, "-t", image, "-f", "Dockerfile.backend", ".")
	own("image", image)
	appArgs := []string{"run", "-d", "--name", appName, "--label", label, "--network", network, "-p", "127.0.0.1::8080",
		"-e", "DATABASE_URL=postgres://sfd:" + password + "@pg:5432/" + database + "?sslmode=disable",
		"-e", "SFD_S3_ENDPOINT=http://minio:9000", "-e", "SFD_S3_ACCESS_KEY=sfdtest", "-e", "SFD_S3_SECRET_KEY=" + password,
		"-e", "SFD_BUCKET=" + bucket, "-e", "SFD_ADMIN_USER=admin", "-e", "SFD_ADMIN_PASS=" + password,
		"-e", "SFD_SESSION_SECRET=synthetic-session-secret-for-isolated-tests", "-e", "SFD_DOWNLOAD_SECRET=" + secret,
		"-e", "SFD_MAX_UPLOAD_BYTES=1024", "-e", "SFD_CLEANUP_ENABLED=true", "-e", "SFD_CLEANUP_INTERVAL=24h", image}
	app := docker(appArgs...)
	own("container", app)
	base := "http://" + port(app, "8080/tcp")
	client := &http.Client{Timeout: 20 * time.Second}
	request := func(method, url string, body io.Reader, cookie *http.Cookie, headers map[string]string) (int, []byte, []*http.Cookie) {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, url, body)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = res.Body.Close() }()
		b, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, b, res.Cookies()
	}
	wait("container backend", func() bool {
		res, e := client.Get(base + "/ready")
		if e != nil {
			return false
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode == 200
	})
	var version int
	var dirty bool
	if e = conn.QueryRow("SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty); e != nil || version != 4 || dirty {
		t.Fatalf("migration state: %d dirty %v err %v", version, dirty, e)
	}
	expect := func(got, want int) {
		t.Helper()
		if got != want {
			t.Fatalf("HTTP %d want %d", got, want)
		}
	}
	jsonBody := func(v any) io.Reader {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return bytes.NewReader(b)
	}
	login := func(user, pass string) *http.Cookie {
		code, _, cookies := request("POST", base+"/login", jsonBody(map[string]string{"username": user, "password": pass}), nil, nil)
		expect(code, 200)
		if len(cookies) != 1 || !cookies[0].HttpOnly {
			t.Fatal("missing secure session attributes")
		}
		return cookies[0]
	}
	admin := login("admin", password)
	for _, user := range []string{"alice", "bob"} {
		code, _, _ := request("POST", base+"/register", jsonBody(map[string]string{"email": user + "@example.test", "username": user, "password": " SpacePass123 "}), nil, nil)
		expect(code, 201)
	}
	alice := login("alice", " SpacePass123 ")
	bob := login("bob", " SpacePass123 ")
	code, _, _ := request("POST", base+"/login", jsonBody(map[string]string{"username": "alice", "password": "SpacePass123"}), nil, nil)
	expect(code, 401)
	for _, p := range []string{"/admin/files", "/metrics"} {
		code, _, _ = request("GET", base+p, nil, bob, nil)
		expect(code, 403)
	}
	code, _, _ = request("POST", base+"/admin/cleanup", nil, bob, nil)
	expect(code, 403)
	code, _, _ = request("POST", base+"/files", jsonBody(map[string]any{}), nil, nil)
	expect(code, 401)
	code, _, _ = request("POST", base+"/files", jsonBody(map[string]any{}), alice, map[string]string{"Origin": "https://attacker.test"})
	expect(code, 403)
	create := func(name string, size int, cookie *http.Cookie) string {
		code, b, _ := request("POST", base+"/files", jsonBody(map[string]any{"orig_name": name, "content_type": "text/plain", "size_bytes": size}), cookie, nil)
		expect(code, 201)
		var meta struct{ ID string }
		if json.Unmarshal(b, &meta) != nil || meta.ID == "" {
			t.Fatal("bad metadata response")
		}
		return meta.ID
	}
	upload := func(id string, content []byte, cookie *http.Cookie) int {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		p, e := mw.CreateFormFile("file", "fixture.txt")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.Write(content); e != nil {
			t.Fatal(e)
		}
		if e = mw.Close(); e != nil {
			t.Fatal(e)
		}
		code, _, _ := request("POST", base+"/upload?id="+id, &body, cookie, map[string]string{"Content-Type": mw.FormDataContentType()})
		return code
	}
	fixture := []byte("native C hash and signed download\n")
	id := create("../../quote\"fixture.txt", 999, alice)
	expect(upload(id, fixture, bob), 404)
	code, _, _ = request("POST", base+"/links", jsonBody(map[string]any{"id": id}), bob, nil)
	expect(code, 404)
	code, _, _ = request("DELETE", base+"/admin/files/"+id, nil, bob, nil)
	expect(code, 403)
	code, b, _ := request("GET", base+"/files", nil, bob, nil)
	expect(code, 200)
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatal("cross-user file listing")
	}
	expect(upload(id, fixture, alice), 200)
	expect(upload(id, fixture, alice), 409)
	var sha, key, name, owner string
	var size, hashBytes int
	if e = conn.QueryRow("SELECT sha256_hex,object_key,orig_name,user_id::text,size_bytes,sha256_bytes FROM files WHERE id=$1", id).Scan(&sha, &key, &name, &owner, &size, &hashBytes); e != nil {
		t.Fatal(e)
	}
	wantHash := sha256.Sum256(fixture)
	if sha != hex.EncodeToString(wantHash[:]) || size != len(fixture) || hashBytes != len(fixture) || name != "quote\"fixture.txt" || owner == "" {
		t.Fatal("integrity, filename or ownership mismatch")
	}
	res, e := http.Get("http://" + miAddr + "/" + bucket + "/" + key)
	if e != nil {
		t.Fatal(e)
	}
	if e = res.Body.Close(); e != nil {
		t.Fatal(e)
	}
	expect(res.StatusCode, 403)
	code, _, _ = request("GET", base+"/admin/files", nil, admin, nil)
	expect(code, 200)
	code, b, _ = request("POST", base+"/links", jsonBody(map[string]any{"id": id, "ttl_seconds": 30}), alice, nil)
	expect(code, 200)
	var link struct{ URL string }
	if json.Unmarshal(b, &link) != nil {
		t.Fatal("invalid link response")
	}

	download, downloadErr := client.Get(link.URL)
	if downloadErr != nil {
		t.Fatal(downloadErr)
	}
	code = download.StatusCode
	disposition, params, parseErr := mime.ParseMediaType(download.Header.Get("Content-Disposition"))
	if parseErr != nil || disposition != "attachment" || params["filename"] != name ||
		download.Header.Get("Content-Length") != strconv.Itoa(len(fixture)) || download.Header.Get("X-Content-Type-Options") != "nosniff" {
		_ = download.Body.Close()
		t.Fatal("unsafe download headers or incorrect byte length")
	}
	b, downloadErr = io.ReadAll(download.Body)
	if closeErr := download.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if downloadErr != nil {
		t.Fatal(downloadErr)
	}
	expect(code, 200)
	if !bytes.Equal(b, fixture) {
		t.Fatal("download bytes differ")
	}
	code, _, _ = request("GET", link.URL+"x", nil, nil, nil)
	expect(code, 401)
	token := func(exp int64) string {
		payload, _ := json.Marshal(map[string]any{"file_id": id, "exp": exp})
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	code, _, _ = request("GET", base+"/download?token="+token(time.Now().Add(-time.Minute).Unix()), nil, nil, nil)
	expect(code, 410)
	// Exercise concurrent real uploads: one transaction owns the row and wins;
	// the other must observe the completed lifecycle and return conflict.
	concurrentID := create("concurrent.txt", len(fixture), alice)
	var multipartBody bytes.Buffer
	mw := multipart.NewWriter(&multipartBody)
	part, err := mw.CreateFormFile("file", "concurrent.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(fixture); err != nil {
		t.Fatal(err)
	}
	if err = mw.Close(); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-gate
			req, err := http.NewRequestWithContext(ctx, "POST", base+"/upload?id="+concurrentID, bytes.NewReader(multipartBody.Bytes()))
			if err != nil {
				codes <- 0
				return
			}
			req.AddCookie(alice)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			res, err := client.Do(req)
			if err != nil {
				codes <- 0
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			codes <- res.StatusCode
		}()
	}
	close(gate)
	statuses := map[int]int{}
	statuses[<-codes]++
	statuses[<-codes]++
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("concurrent upload results: %v", statuses)
	}

	t.Run("sql-cancellation", func(t *testing.T) { verifySQLCancellation(ctx, t, conn, base, link.URL) })
	t.Run("ambiguous-commit", func(t *testing.T) {
		if docker("inspect", "--format", "{{index .Config.Labels \"sfd.quality-run\"}}", pg) != run ||
			docker("inspect", "--format", "{{index .Config.Labels \"sfd.quality-run\"}}", mi) != run {
			t.Fatal("fault fixture ownership mismatch")
		}
		tool, err := filepath.Abs("../../native/sfd-hash")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, "go", "test", "./internal/server", "-run", "^TestDisposableCommitFaults$", "-count=1", "-v")
		cmd.Dir = "../.."
		cmd.Env = append(os.Environ(), "SFD_CORRECTION_FIXTURE_DSN="+dsn, "SFD_CORRECTION_EXPECTED_DB="+database, "SFD_CORRECTION_RUN="+run, "SFD_CORRECTION_BUCKET="+bucket, "SFD_CORRECTION_STORAGE="+miAddr, "SFD_HASH_TOOL="+tool, "SFD_MAX_UPLOAD_BYTES=1024")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("COMMIT fault fixtures: %v %s", err, out)
		} else {
			t.Log(string(out))
		}
	})
	t.Run("active-transfer-shutdown", func(t *testing.T) {
		verifyActiveShutdown(ctx, t, run, appArgs, conn, alice, create, docker, own, port)
	})
	t.Run("authenticated-browser", func(t *testing.T) {
		if os.Getenv("SFD_BROWSER") != "1" {
			t.Skip("real browser requires SFD_BROWSER=1; not a browser pass")
		}
		// #nosec G204 G702 -- Constant script, verified synthetic loopback target; no shell.
		cmd := exec.CommandContext(ctx, "node", "../browser/journey.cjs", base)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("real browser: %v %s", err, out)
		} else {
			t.Log(string(out))
		}
	})
	t.Run("actual-proxy", func(t *testing.T) { verifyProxy(ctx, t, run, network, image, appArgs, docker, own, port) })
	assessBoundaries(ctx, t, conn, mc, bucket, base, admin, alice, request, create, upload)

	t.Run("assessment-cancellation-and-locks", func(t *testing.T) {
		assessCancellation(ctx, t, conn, base, alice, admin, create("canceled.bin", 3, alice))
	})

	// Run the same real image with an intentionally missing native executable.
	// A hashing error must retain a failed record and never create a usable link.
	badArgs := append([]string(nil), appArgs...)
	for i := 0; i < len(badArgs)-1; i++ {
		if badArgs[i] == "--name" {
			badArgs[i+1] = run + "-bad-hash"
		}
	}
	badArgs = append(badArgs[:len(badArgs)-1], "-e", "SFD_HASH_TOOL=/missing-native-hash", image)
	badApp := docker(badArgs...)
	own("container", badApp)
	badBase := "http://" + port(badApp, "8080/tcp")
	wait("negative hash backend", func() bool {
		res, e := client.Get(badBase + "/health")
		if e != nil {
			return false
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode == 200
	})
	code, _, _ = request("GET", badBase+"/ready", nil, nil, nil)
	expect(code, 503)
	failedID := create("native-failure.txt", len(fixture), alice)
	code, _, _ = request("POST", badBase+"/upload?id="+failedID, bytes.NewReader(multipartBody.Bytes()), alice, map[string]string{"Content-Type": mw.FormDataContentType()})
	expect(code, 502)
	var failedStatus string
	if e = conn.QueryRow("SELECT status FROM files WHERE id=$1", failedID).Scan(&failedStatus); e != nil || failedStatus != "failed" {
		t.Fatal("native failure did not persist failed state")
	}
	code, _, _ = request("POST", base+"/links", jsonBody(map[string]any{"id": failedID}), alice, nil)
	expect(code, 409)

	// Oversized bytes must be rejected even when metadata lies, and failed records
	// must remain available to cleanup.
	big := create("oversize.txt", 1, alice)
	expect(upload(big, bytes.Repeat([]byte("x"), 1025), alice), 413)
	code, _, _ = request("POST", base+"/files", jsonBody(map[string]any{"orig_name": "big", "content_type": "text/plain", "size_bytes": 1025}), alice, nil)
	expect(code, 413)
	// A real native hash failure must not yield a downloadable file.
	// The runtime image runs the compiled C binary; success above has no fallback.
	// Verify logout invalidates replay, not only the browser's cookie.
	expiredSessionHash := sha256.Sum256([]byte(bob.Value))
	if _, e = conn.Exec("UPDATE sessions SET expires_at=now()-interval '1 minute' WHERE token_hash=$1", hex.EncodeToString(expiredSessionHash[:])); e != nil {
		t.Fatal(e)
	}
	code, _, _ = request("GET", base+"/me", nil, bob, nil)
	expect(code, 401)
	bob = login("bob", " SpacePass123 ")

	old := *bob
	code, _, cookies := request("POST", base+"/logout", nil, bob, nil)
	expect(code, 204)
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatal("logout did not clear cookie")
	}
	code, _, _ = request("GET", base+"/me", nil, &old, nil)
	expect(code, 401)
	// #nosec G124 -- Copy of the HTTP loopback test cookie, deliberately corrupted for rejection.
	tampered := *alice
	tampered.Value += "x"
	code, _, _ = request("GET", base+"/me", nil, &tampered, nil)
	expect(code, 401)
	if _, e = conn.Exec("UPDATE users SET is_active=FALSE WHERE username='alice'"); e != nil {
		t.Fatal(e)
	}
	code, _, _ = request("GET", base+"/me", nil, alice, nil)
	expect(code, 401)
	if _, e = conn.Exec("UPDATE users SET is_active=TRUE WHERE username='alice'"); e != nil {
		t.Fatal(e)
	}
	// Simulate a storage outage using only this test's container; metadata must survive.
	docker("stop", mi)
	code, _, _ = request("GET", base+"/ready", nil, nil, nil)
	expect(code, 503)
	code, _, _ = request("GET", base+"/health", nil, nil, nil)
	expect(code, 200)
	code, _, _ = request("DELETE", base+"/admin/files/"+id, nil, admin, nil)
	expect(code, 502)
	var remains int
	if e = conn.QueryRow("SELECT count(*) FROM files WHERE id=$1", id).Scan(&remains); e != nil || remains != 1 {
		t.Fatal("metadata lost on storage failure")
	}
	docker("start", mi)
	oldMiAddr := miAddr
	miAddr = port(mi, "9000/tcp")
	t.Logf("owned MinIO restart endpoint: %s -> %s", oldMiAddr, miAddr)
	mc, e = minio.New(miAddr, &minio.Options{Creds: credentials.NewStaticV4("sfdtest", password, ""), Secure: false})
	if e != nil {
		t.Fatal(e)
	}
	wait("restarted minio", func() bool { _, e := mc.ListBuckets(ctx); return e == nil })
	code, _, _ = request("DELETE", base+"/admin/files/"+id, nil, admin, nil)
	expect(code, 204)
	if _, e = mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); e == nil {
		t.Fatal("deleted object remains")
	}
	code, _, _ = request("GET", link.URL, nil, nil, nil)
	expect(code, 404)
	if _, e = conn.Exec("UPDATE files SET created_at=now()-interval '48 hours' WHERE id=$1", big); e != nil {
		t.Fatal(e)
	}
	code, b, _ = request("POST", base+"/admin/cleanup", nil, admin, nil)
	expect(code, 200)
	var cleanup struct {
		DeletedCount int `json:"deleted_count"`
	}
	if json.Unmarshal(b, &cleanup) != nil || cleanup.DeletedCount < 1 {
		t.Fatal("cleanup did not delete failed upload")
	}
	code, _, _ = request("GET", base+"/", nil, nil, nil)
	expect(code, 200)

	t.Run("assessment-runtime-dependencies", func(t *testing.T) { assessRuntimeDependencies(ctx, t, app, mi, run, docker) })
	t.Run("assessment-fixture-recovery", func(t *testing.T) {
		assessFixtureRecovery(ctx, t, conn, mc, database, bucket, "uploads/"+concurrentID, pg, run, docker)
	})
	t.Run("assessment-database-outage", func(t *testing.T) {
		docker("stop", pg)
		code, _, _ = request("GET", base+"/health", nil, nil, nil)
		expect(code, 200)
		code, _, _ = request("GET", base+"/ready", nil, nil, nil)
		expect(code, 503)
		code, _, _ = request("GET", base+"/me", nil, alice, nil)
		expect(code, 500)
		docker("start", pg)
		wait("database restart readiness", func() bool {
			res, err := client.Get(base + "/ready")
			if err != nil {
				return false
			}
			defer func() { _ = res.Body.Close() }()
			return res.StatusCode == 200
		})
		t.Log("PASS: process liveness distinct from readiness; DB outage fails closed; restart recovers readiness")
	})
	t.Run("assessment-idle-graceful-restart", func(t *testing.T) {
		docker("stop", "--time", "10", app)
		if exit := docker("inspect", "--format", "{{.State.ExitCode}}", app); exit != "0" {
			t.Fatalf("shutdown exit %s", exit)
		}
		// Docker forwards application stderr on its own stderr; IDs still use
		// stdout-only capture elsewhere. Never print raw application logs here.
		// #nosec G204 G702 -- Parent-verified local Docker CLI, owned container ID, fixed logs subcommand.
		logsCmd := exec.CommandContext(ctx, cli, "logs", app)
		logs, logsErr := logsCmd.CombinedOutput()
		if logsErr != nil {
			t.Fatal("owned shutdown log read failed")
		}
		if !strings.Contains(string(logs), "shutdown_complete") {
			t.Fatal("graceful shutdown not acknowledged")
		}
		docker("start", app)
		base = "http://" + port(app, "8080/tcp")
		wait("application restart readiness", func() bool {
			res, err := client.Get(base + "/ready")
			if err != nil {
				return false
			}
			defer func() { _ = res.Body.Close() }()
			return res.StatusCode == 200
		})
		code, _, _ = request("GET", base+"/me", nil, alice, nil)
		expect(code, 200)
		t.Log("PASS: idle SIGTERM exit 0, readiness and persisted session after restart; in-flight shutdown unverified")
	})
	t.Logf("real container/native flow completed; individual subtest results determine acceptance; disposable run %s", run)
}
