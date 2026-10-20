package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const preparationRustImage = "rustfs/rustfs@sha256:c917317a76af3df35b596612a32e297b6a31a5c815e023fdf62ca78c2cf84201"

// Every mutable target is freshly created here; no existing endpoint, database,
// environment file or volume can be selected by a caller.
type rehearsal struct {
	ownerT                                                                           *testing.T
	t                                                                                *testing.T
	ctx                                                                              context.Context
	cli, run, network, label, appImage, minioImage, evidence                         string
	root, rootSecret, app, appSecret, reader, readerSecret, outsider, outsiderSecret string
	pgSecret, adminSecret, sessionSecret, downloadSecret                             string
	resources                                                                        [][2]string
	rustEnv                                                                          map[string]string
}
type rehearsalStore struct {
	id, alias, volume, image, addr string
	client, read                   *minio.Client
}
type rehearsalDB struct {
	id, alias, name, volume string
	conn                    *sql.DB
}
type rehearsalApp struct {
	id, base string
	db       rehearsalDB
}
type rehearsalFile struct {
	ID, Key, Owner, Username, Status, SHA256 string
	Bytes, HashBytes                         int64
}

func TestRustFSMigrationRehearsal(t *testing.T) {
	if os.Getenv("SFD_RUSTFS_REHEARSAL") != "1" {
		if os.Getenv("SFD_REHEARSAL_REQUIRED") == "1" {
			t.Fatal("required rehearsal opt-in missing")
		}
		t.Skip("requires SFD_RUSTFS_REHEARSAL=1; not migration acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 28*time.Minute)
	defer cancel()
	f := &rehearsal{ownerT: t, t: t, ctx: ctx, cli: "docker", run: "sfd-quality-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]}
	f.network, f.label = f.run+"-net", "sfd.quality-run="+f.run
	f.appImage, f.minioImage = f.run+":app", f.run+":source"
	f.root, f.rootSecret = "root_"+rehearsalSecret()[:16], rehearsalSecret()
	f.app, f.appSecret = "app_"+rehearsalSecret()[:16], rehearsalSecret()
	f.reader, f.readerSecret = "reader_"+rehearsalSecret()[:16], rehearsalSecret()
	f.outsider, f.outsiderSecret = "outsider_"+rehearsalSecret()[:16], rehearsalSecret()
	f.pgSecret, f.adminSecret = rehearsalSecret(), rehearsalSecret()
	f.sessionSecret, f.downloadSecret = rehearsalSecret(), rehearsalSecret()
	f.evidence = t.TempDir()
	if base := os.Getenv("SFD_REHEARSAL_EVIDENCE"); base != "" {
		f.evidence = filepath.Join(base, f.run)
		if err := os.Mkdir(f.evidence, 0700); err != nil {
			t.Fatal(err)
		}
		marker, err := os.OpenFile(filepath.Join(f.evidence, "owner"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = marker.WriteString(f.run)
		if err != nil {
			t.Fatal(err)
		}
		if err = marker.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { f.t = t; f.cleanup() })
	endpoint := f.docker("context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	if os.Getenv("DOCKER_HOST") != "" || (!strings.HasPrefix(endpoint, "unix://") && !strings.HasPrefix(endpoint, "npipe://")) {
		t.Fatal("only local Docker engine permitted")
	}
	if f.docker("info", "--format", "{{.OSType}}") != "linux" {
		t.Fatal("Linux container engine required")
	}
	phase := func(name string, fn func()) {
		if !t.Run(name, func(st *testing.T) { f.t = st; fn() }) {
			f.t = t
			t.Fatal("rehearsal hard stop; source evidence retained; no further phases")
		}
		f.t = t
	}
	var source, destination, restored rehearsalStore
	var sourceDB, restoredDB rehearsalDB
	var sourceApp, restoredApp rehearsalApp
	var baseline []rehearsalFile
	var baselineBytes map[string][]byte
	var checkpoint, checkpointSHA string
	phase("A-configuration-and-identity", func() {
		f.docker("network", "create", "--label", f.label, f.network)
		f.own("network", f.network)
		f.docker("build", "--label", f.label, "-t", f.minioImage, "-f", "Dockerfile.storage", ".")
		f.own("image", f.minioImage)
		f.docker("build", "--label", f.label, "-t", f.appImage, "-f", "Dockerfile.backend", ".")
		f.own("image", f.appImage)
		f.configuration()
		f.docker("pull", "--platform", "linux/amd64", preparationRustImage)
		if !strings.Contains(f.docker("image", "inspect", "--format", "{{json .RepoDigests}}", preparationRustImage), strings.Split(preparationRustImage, "@")[1]) {
			f.t.Fatal("official immutable manifest identity mismatch")
		}
		source = f.store("source", false, "")
		destination = f.store("destination", true, "")
		version := f.docker("exec", destination.id, "/usr/bin/rustfs", "--version")
		if !strings.Contains(version, "1.0.2") {
			f.t.Fatal("executed RustFS version mismatch")
		}
		f.t.Log("official manifest", preparationRustImage, "source 2b243d8659497778c2406cc844a490565ec13287; executed", version)
		f.t.Log("executed binary", f.docker("exec", destination.id, "sha256sum", "/usr/bin/rustfs"))
		f.restrictions(destination)
		f.writeEvidence("runtime-images.json", map[string]string{
			"rustfs_manifest":       preparationRustImage,
			"rustfs_image_id":       f.docker("inspect", "--format", "{{.Image}}", destination.id),
			"source_minio_image_id": f.docker("image", "inspect", "--format", "{{.Id}}", f.minioImage),
			"backend_image_id":      f.docker("image", "inspect", "--format", "{{.Id}}", f.appImage),
		})
		f.t.Log("source MinIO image ID", f.docker("image", "inspect", "--format", "{{.Id}}", f.minioImage),
			"backend image ID", f.docker("image", "inspect", "--format", "{{.Id}}", f.appImage))
	})
	phase("B-authenticated-source-and-checkpoint", func() {
		sourceDB = f.database("source", "")
		sourceApp = f.application("source", sourceDB, source)
		for _, user := range []string{"alice", "bob"} {
			f.expect(f.request(sourceApp, "POST", "/register", f.json(map[string]string{"username": user, "email": user + "@example.test", "password": f.adminSecret}), nil, "").code, 201)
		}
		alice, bob := f.login(sourceApp, "alice"), f.login(sourceApp, "bob")
		for i, data := range [][]byte{{}, []byte("ordinary native-hashed synthetic file\n"), append(bytes.Repeat([]byte{0x53}, 5<<20), bytes.Repeat([]byte{0x17}, 17)...), append(bytes.Repeat([]byte{0x4d}, 5<<20), bytes.Repeat([]byte{0x21}, 17)...)} {
			cookie := alice
			if i == 3 {
				cookie = bob
			}
			f.upload(sourceApp, fmt.Sprintf("seed-%d.bin", i), data, cookie)
		}
		baseline, baselineBytes = f.manifest(sourceDB, source)
		if len(baseline) != 4 {
			f.t.Fatal("source seed inventory mismatch")
		}
		var multipartFile rehearsalFile
		for _, row := range baseline {
			if row.Username == "bob" {
				multipartFile = row
			}
		}
		f.copyObject(source.client, source.client, multipartFile.Key, baselineBytes[multipartFile.Key], true)
		again, _ := f.manifest(sourceDB, source)
		f.equalManifest(baseline, again)
		f.accept(sourceApp, baseline, baselineBytes)
		f.stop(sourceApp.id, 25)
		checkpoint, checkpointSHA = f.checkpoint(sourceDB, "baseline")
		f.stop(sourceDB.id, 60)
		f.writeEvidence("baseline.json", baseline)
		f.writeEvidence("checkpoint.json", map[string]string{"sha256": checkpointSHA, "run": f.run, "database": sourceDB.name, "storage": source.id})
		f.t.Logf("quiesced checkpoint SHA256=%s; files=%d; native hash/bytes and explicit completed multipart verified", checkpointSHA, len(baseline))
	})
	phase("C-logical-scoped-copy", func() {
		f.copyManifest(source.read, destination.client, baseline, baselineBytes)
		f.permissions(destination, baseline[0].Key)
	})
	phase("D-before-writes-rollback", func() {
		rollback := f.store("prewrite-rollback", false, "")
		f.copyManifest(source.read, rollback.client, baseline, baselineBytes)
		database := f.database("prewrite-rollback", checkpoint)
		app := f.application("prewrite-rollback", database, rollback)
		got, data := f.manifest(database, rollback)
		f.equalManifest(baseline, got)
		f.equalBytes(baselineBytes, data)
		f.accept(app, got, data)
		f.stop(app.id, 25)
		f.stop(database.id, 60)
		f.stop(rollback.id, 60)
	})
	phase("E-cold-restored-pair-acceptance", func() {
		f.stop(destination.id, 60)
		volume := f.volume("cold-restored")
		f.verify("volume", destination.volume)
		f.verify("volume", volume)
		copier := f.docker("create", "--name", f.run+"-cold-copy", "--label", f.label, "--network", "none", "--user", "10001:10001", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh", "-v", destination.volume+":/source:ro", "-v", volume+":/data", preparationRustImage, "-c", "cp -R /source/. /data/")
		f.own("container", copier)
		f.docker("start", "-a", copier)
		if f.docker("inspect", "--format", "{{.State.ExitCode}}", copier) != "0" {
			f.t.Fatal("cold copy failed")
		}
		restored = f.store("restored", true, volume)
		restoredDB = f.database("restored", checkpoint)
		restoredApp = f.application("restored", restoredDB, restored)
		got, data := f.manifest(restoredDB, restored)
		f.equalManifest(baseline, got)
		f.equalBytes(baselineBytes, data)
		f.accept(restoredApp, baseline, baselineBytes)
		f.permissions(restored, baseline[0].Key)
		if os.Getenv("SFD_BROWSER") != "1" {
			f.t.Fatal("restored-pair browser acceptance required")
		}
		// #nosec G204 G702 -- Fixed browser script; argument is the verified owned loopback port, no shell.
		cmd := exec.CommandContext(ctx, "node", "tests/browser/journey.cjs", restoredApp.base)
		fixture, err := json.Marshal(map[string]string{"username": "admin", "password": f.adminSecret})
		if err != nil {
			f.t.Fatal(err)
		}
		cmd.Stdin = bytes.NewReader(fixture)
		cmd.Env = append(os.Environ(), "SFD_BROWSER_RUN="+f.run)
		cmd.Dir = "../.."
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			f.t.Fatalf("restored-pair browser: %v %s", err, out)
		}
		f.t.Log(string(out))
		f.upload(restoredApp, "post-restore.bin", []byte("new native-hashed bytes after pair restoration"), f.login(restoredApp, "alice"))
		f.t.Log("unchanged app + fresh restored PG + cold-restored RustFS: ownership/admin/anonymous/cross-owner/native/signed links/browser/new upload verified")
	})
	phase("F-postwrite-delete-coherent-rollback", func() {
		f.upload(restoredApp, "post-cutover.bin", []byte("rollback must include this addition"), f.login(restoredApp, "bob"))
		deleted := baseline[0]
		f.expect(f.request(restoredApp, "DELETE", "/admin/files/"+deleted.ID, nil, f.login(restoredApp, "admin"), "").code, 204)
		changed, data := f.manifest(restoredDB, restored)
		var added bool
		for _, row := range changed {
			if row.ID == deleted.ID {
				f.t.Fatal("deleted metadata persisted")
			}
			if _, ok := baselineBytes[row.Key]; !ok {
				added = true
			}
		}
		if !added {
			f.t.Fatal("post-cutover addition missing")
		}
		if _, err := source.read.StatObject(ctx, f.run+"-private", deleted.Key, minio.StatObjectOptions{}); err != nil {
			f.t.Fatal("original baseline was changed")
		}
		missing := 0
		for _, row := range changed {
			if _, ok := baselineBytes[row.Key]; !ok {
				_, err := source.read.StatObject(ctx, f.run+"-private", row.Key, minio.StatObjectOptions{})
				if minio.ToErrorResponse(err).Code != "NoSuchKey" {
					f.t.Fatal("expected added key absent from source baseline", err)
				}
				missing++
			}
		}
		if missing == 0 {
			f.t.Fatal("endpoint-only reversal did not demonstrate divergence")
		}
		f.stop(restoredApp.id, 25)
		changedCheckpoint, _ := f.checkpoint(restoredDB, "postwrite")
		f.stop(restoredDB.id, 60)
		rollback := f.store("postwrite-rollback", false, "")
		f.copyManifest(restored.read, rollback.client, changed, data)
		database := f.database("postwrite-rollback", changedCheckpoint)
		app := f.application("postwrite-rollback", database, rollback)
		got, recovered := f.manifest(database, rollback)
		f.equalManifest(changed, got)
		f.equalBytes(data, recovered)
		f.accept(app, got, recovered)
		if _, err := rollback.client.StatObject(ctx, f.run+"-private", deleted.Key, minio.StatObjectOptions{}); minio.ToErrorResponse(err).Code != "NoSuchKey" {
			f.t.Fatal("rollback resurrected deleted bytes", err)
		}
		f.equalBytes(baselineBytes, f.storageBytes(source.read, baseline))
		// #nosec G304 -- Immutable checkpoint generated in this fixture-owned unique evidence directory.
		raw, err := os.ReadFile(checkpoint)
		if err != nil {
			f.t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != checkpointSHA {
			f.t.Fatal("original checkpoint mutated")
		}
		f.writeEvidence("rollback.json", got)
		f.t.Logf("endpoint-only reversal rejected: %d added keys absent, deleted key still present; coherent rollback PAIR files=%d; original baseline bytes/checkpoint unchanged", missing, len(got))
	})
	f.t = t
	t.Log("REHEARSAL COMPLETE: 6 mandatory phases, zero skipped; no production adoption or cross-host DR claim")
}

func rehearsalSecret() string { return strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "") }
func (f *rehearsal) command(ctx context.Context, args ...string) (string, error) {
	// #nosec G204 G702 -- Fixed Docker executable and argument lists; no shell.
	cmd := exec.CommandContext(ctx, f.cli, args...)
	cmd.Dir = "../.."
	raw, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			raw = append(raw, exit.Stderr...)
		}
	}
	return strings.TrimSpace(string(raw)), err
}
func (f *rehearsal) docker(args ...string) string {
	f.t.Helper()
	out, err := f.command(f.ctx, args...)
	if err != nil {
		f.t.Fatalf("owned Docker operation %s: %v %s", args[0], err, out)
	}
	return out
}
func (f *rehearsal) own(kind, id string) {
	f.resources = append(f.resources, [2]string{kind, id})
	f.verify(kind, id)
}
func (f *rehearsal) verify(kind, id string) {
	f.t.Helper()
	format := "{{index .Labels \"sfd.quality-run\"}}"
	if kind == "container" || kind == "image" {
		format = "{{index .Config.Labels \"sfd.quality-run\"}}"
	}
	if f.docker(kind, "inspect", "--format", format, id) != f.run {
		f.t.Fatal("resource ownership mismatch; hard stop")
	}
}
func (f *rehearsal) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for i := len(f.resources) - 1; i >= 0; i-- {
		r := f.resources[i]
		format := "{{index .Labels \"sfd.quality-run\"}}"
		if r[0] == "container" || r[0] == "image" {
			format = "{{index .Config.Labels \"sfd.quality-run\"}}"
		}
		out, err := f.command(ctx, r[0], "inspect", "--format", format, r[1])
		if err != nil || out != f.run {
			f.t.Errorf("cleanup refused: ownership not verified %s %s", r[0], r[1])
			continue
		}
		if f.t.Failed() {
			if r[0] == "container" {
				if _, err = f.command(ctx, "stop", "--time", "25", r[1]); err != nil {
					f.t.Error("owned stop failed", err)
				}
			}
			continue
		}
		args := []string{r[0], "rm"}
		if r[0] == "container" || r[0] == "image" {
			args = append(args, "--force")
		}
		args = append(args, r[1])
		if _, err = f.command(ctx, args...); err != nil {
			f.t.Error("owned cleanup failed", err)
		}
	}
	if f.t.Failed() {
		f.t.Log("FAILURE: owned containers stopped; source volumes/checkpoints retained on runner, not deleted; sanitized artifacts survive retention, ephemeral runner data does not")
	}
}
func (f *rehearsal) volume(suffix string) string {
	v := f.run + "-" + suffix
	f.docker("volume", "create", "--label", f.label, v)
	f.own("volume", v)
	return v
}
func (f *rehearsal) port(id, p string) string {
	a := f.docker("port", id, p)
	if !strings.HasPrefix(a, "127.0.0.1:") || strings.Contains(a, "\n") {
		f.t.Fatal("unexpected port exposure")
	}
	return a
}
func (f *rehearsal) wait(name string, fn func() bool) {
	f.t.Helper()
	until := time.Now().Add(90 * time.Second)
	for time.Now().Before(until) {
		if fn() {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	f.t.Fatal(name, "unavailable")
}
func (f *rehearsal) stop(id string, seconds int) {
	f.verify("container", id)
	f.docker("stop", "--time", strconv.Itoa(seconds), id)
	if f.docker("inspect", "--format", "{{.State.ExitCode}}", id) != "0" {
		f.t.Fatal("owned service did not drain cleanly")
	}
}
func (f *rehearsal) s3(addr, key, secret string) *minio.Client {
	c, err := minio.New(addr, &minio.Options{Creds: credentials.NewStaticV4(key, secret, ""), Secure: false, Region: "us-east-1", BucketLookup: minio.BucketLookupPath})
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}
func (f *rehearsal) store(suffix string, rust bool, volume string) rehearsalStore {
	if volume == "" {
		volume = f.volume(suffix + "-objects")
	}
	f.verify("volume", volume)
	s := rehearsalStore{alias: f.run + "-" + suffix + "-s3", volume: volume, image: f.minioImage}
	args := []string{"run", "-d", "--name", s.alias, "--label", f.label, "--network", f.network, "--network-alias", s.alias, "-p", "127.0.0.1::9000", "-v", volume + ":/data", "--cap-drop=ALL", "--security-opt", "no-new-privileges"}
	if rust {
		s.image = preparationRustImage
		args = append(args, "--user", "10001:10001", "--entrypoint", "/usr/bin/rustfs")
		for k, v := range f.rustEnv {
			args = append(args, "-e", k+"="+v)
		}
	} else {
		args = append(args, "-e", "MINIO_ROOT_USER="+f.root, "-e", "MINIO_ROOT_PASSWORD="+f.rootSecret)
	}
	args = append(args, s.image)
	if rust {
		args = append(args, "/data")
	} else {
		args = append(args, "server", "/data")
	}
	s.id = f.docker(args...)
	f.own("container", s.id)
	s.addr = f.port(s.id, "9000/tcp")
	admin := f.s3(s.addr, f.root, f.rootSecret)
	f.wait("fresh storage", func() bool { _, err := admin.ListBuckets(f.ctx); return err == nil })
	if volume != f.run+"-cold-restored" {
		if err := admin.MakeBucket(f.ctx, f.run+"-private", minio.MakeBucketOptions{}); err != nil {
			f.t.Fatal(err)
		}
		if err := admin.MakeBucket(f.ctx, f.run+"-other", minio.MakeBucketOptions{}); err != nil {
			f.t.Fatal(err)
		}
		f.verify("container", s.id)
		f.verify("volume", s.volume)
		// #nosec G204 G702 -- Fixed synthetic helper; verified owned container/volume and loopback endpoint, no shell.
		cmd := exec.CommandContext(f.ctx, "go", "run", ".", s.addr, f.run+"-private", f.run, "generated")
		cmd.Dir = "../storage/iam"
		cmd.Env = append(os.Environ(), "SFD_IAM_RUN="+f.run, "SFD_IAM_ROOT_KEY="+f.root, "SFD_IAM_ROOT_SECRET="+f.rootSecret, "SFD_IAM_APP_KEY="+f.app, "SFD_IAM_APP_SECRET="+f.appSecret, "SFD_IAM_READER_KEY="+f.reader, "SFD_IAM_READER_SECRET="+f.readerSecret, "SFD_IAM_OUTSIDER_KEY="+f.outsider, "SFD_IAM_OUTSIDER_SECRET="+f.outsiderSecret)
		if out, err := cmd.CombinedOutput(); err != nil {
			f.t.Fatalf("signed synthetic IAM provisioning: %v %s", err, out)
		}
	}
	s.client = f.s3(s.addr, f.app, f.appSecret)
	s.read = f.s3(s.addr, f.reader, f.readerSecret)
	f.wait("scoped storage", func() bool { ok, err := s.client.BucketExists(f.ctx, f.run+"-private"); return ok && err == nil })
	return s
}
func (f *rehearsal) database(suffix, checkpoint string) rehearsalDB {
	d := rehearsalDB{alias: f.run + "-" + suffix + "-pg", name: "sfd_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16], volume: f.volume(suffix + "-pgdata")}
	d.id = f.docker("run", "-d", "--name", d.alias, "--label", f.label, "--network", f.network, "--network-alias", d.alias, "-p", "127.0.0.1::5432", "-v", d.volume+":/var/lib/postgresql/data", "-e", "POSTGRES_DB="+d.name, "-e", "POSTGRES_USER=sfd", "-e", "POSTGRES_PASSWORD="+f.pgSecret, "postgres:16")
	f.own("container", d.id)
	var err error
	d.conn, err = sql.Open("pgx", "postgres://sfd:"+f.pgSecret+"@"+f.port(d.id, "5432/tcp")+"/"+d.name+"?sslmode=disable")
	if err != nil {
		f.t.Fatal(err)
	}
	f.ownerT.Cleanup(func() { _ = d.conn.Close() })
	f.wait("fresh PostgreSQL", func() bool { return d.conn.PingContext(f.ctx) == nil })
	var name, user string
	var tables int
	if err = d.conn.QueryRowContext(f.ctx, "SELECT current_database(),current_user").Scan(&name, &user); err != nil || name != d.name || user != "sfd" {
		f.t.Fatal("unexpected PostgreSQL target")
	}
	if err = d.conn.QueryRowContext(f.ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&tables); err != nil || tables != 0 {
		f.t.Fatal("database not empty")
	}
	if checkpoint != "" {
		f.verify("container", d.id)
		f.verify("volume", d.volume)
		f.docker("cp", checkpoint, d.id+":/tmp/restore.dump")
		f.docker("exec", d.id, "pg_restore", "--exit-on-error", "--no-owner", "-U", "sfd", "-d", d.name, "/tmp/restore.dump")
	}
	f.t.Log("PostgreSQL actual image", f.docker("inspect", "--format", "{{.Image}}", d.id))
	return d
}
func (f *rehearsal) application(suffix string, d rehearsalDB, s rehearsalStore) rehearsalApp {
	args := make([]string, 0, 64)
	args = append(args, "run", "-d", "--name", f.run+"-"+suffix+"-app", "--label", f.label, "--network", f.network, "-p", "127.0.0.1::8080", "--cap-drop=ALL", "--security-opt", "no-new-privileges")
	env := map[string]string{"DATABASE_URL": "postgres://sfd:" + f.pgSecret + "@" + d.alias + ":5432/" + d.name + "?sslmode=disable", "SFD_S3_ENDPOINT": "http://" + s.alias + ":9000", "SFD_S3_ACCESS_KEY": f.app, "SFD_S3_SECRET_KEY": f.appSecret, "SFD_BUCKET": f.run + "-private", "SFD_ADMIN_USER": "admin", "SFD_ADMIN_PASS": f.adminSecret, "SFD_SESSION_SECRET": f.sessionSecret, "SFD_DOWNLOAD_SECRET": f.downloadSecret, "SFD_MAX_UPLOAD_BYTES": "6291456", "SFD_MAX_REQUEST_BYTES": "7340032", "SFD_CLEANUP_ENABLED": "false"}
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, f.appImage)
	a := rehearsalApp{id: f.docker(args...), db: d}
	f.own("container", a.id)
	a.base = "http://" + f.port(a.id, "8080/tcp")
	f.wait("unchanged backend", func() bool {
		req, err := http.NewRequestWithContext(f.ctx, "GET", a.base+"/ready", nil)
		if err != nil {
			return false
		}
		res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			return false
		}
		_ = res.Body.Close()
		return res.StatusCode == 200
	})
	var version int
	var dirty bool
	if err := d.conn.QueryRowContext(f.ctx, "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty); err != nil || version != 4 || dirty {
		f.t.Fatal("restored schema mismatch")
	}
	return a
}

type rehearsalResponse struct {
	code    int
	body    []byte
	cookies []*http.Cookie
}

func (f *rehearsal) request(a rehearsalApp, method, path string, body io.Reader, cookie *http.Cookie, contentType string) rehearsalResponse {
	f.t.Helper()
	target := a.base + path
	if strings.HasPrefix(path, "http://") {
		target = path
	}
	req, err := http.NewRequestWithContext(f.ctx, method, target, body)
	if err != nil {
		f.t.Fatal(err)
	}
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		f.t.Fatal(err)
	}
	return rehearsalResponse{res.StatusCode, raw, res.Cookies()}
}
func (f *rehearsal) expect(got, want int) {
	f.t.Helper()
	if got != want {
		f.t.Fatalf("HTTP %d want %d", got, want)
	}
}
func (f *rehearsal) json(v any) io.Reader {
	raw, err := json.Marshal(v)
	if err != nil {
		f.t.Fatal(err)
	}
	return bytes.NewReader(raw)
}
func (f *rehearsal) login(a rehearsalApp, user string) *http.Cookie {
	r := f.request(a, "POST", "/login", f.json(map[string]string{"username": user, "password": f.adminSecret}), nil, "")
	f.expect(r.code, 200)
	if len(r.cookies) != 1 || !r.cookies[0].HttpOnly {
		f.t.Fatal("invalid authentication cookie")
	}
	return r.cookies[0]
}
func (f *rehearsal) upload(a rehearsalApp, name string, data []byte, cookie *http.Cookie) {
	r := f.request(a, "POST", "/files", f.json(map[string]any{"orig_name": name, "content_type": "application/octet-stream", "size_bytes": len(data)}), cookie, "")
	f.expect(r.code, 201)
	var meta struct{ ID string }
	if json.Unmarshal(r.body, &meta) != nil || meta.ID == "" {
		f.t.Fatal("invalid file metadata")
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = part.Write(data); err != nil {
		f.t.Fatal(err)
	}
	if err = mw.Close(); err != nil {
		f.t.Fatal(err)
	}
	f.expect(f.request(a, "POST", "/upload?id="+meta.ID, &body, cookie, mw.FormDataContentType()).code, 200)
	var hash string
	var size, n int64
	sum := sha256.Sum256(data)
	if err = a.db.conn.QueryRowContext(f.ctx, "SELECT sha256_hex,size_bytes,sha256_bytes FROM files WHERE id=$1", meta.ID).Scan(&hash, &size, &n); err != nil || hash != hex.EncodeToString(sum[:]) || size != int64(len(data)) || n != size {
		f.t.Fatal("native upload integrity mismatch")
	}
}

func (f *rehearsal) manifest(d rehearsalDB, s rehearsalStore) ([]rehearsalFile, map[string][]byte) {
	rows, err := d.conn.QueryContext(f.ctx, "SELECT f.id::text,f.object_key,coalesce(f.user_id::text,''),coalesce(u.username,''),f.status,coalesce(f.sha256_hex,''),f.size_bytes,coalesce(f.sha256_bytes,-1) FROM files f LEFT JOIN users u ON u.id=f.user_id ORDER BY f.id")
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	manifest := []rehearsalFile{}
	for rows.Next() {
		var r rehearsalFile
		if err = rows.Scan(&r.ID, &r.Key, &r.Owner, &r.Username, &r.Status, &r.SHA256, &r.Bytes, &r.HashBytes); err != nil {
			f.t.Fatal(err)
		}
		if _, err = uuid.Parse(r.ID); err != nil {
			f.t.Fatal("invalid file UUID")
		}
		if _, err = uuid.Parse(r.Owner); err != nil || r.Username == "" || r.Key != "uploads/"+r.ID || (r.Status != "hashed" && r.Status != "ready") || len(r.SHA256) != 64 || r.Bytes != r.HashBytes {
			f.t.Fatal("unresolved lifecycle, key, ownership or byte metadata; no repair permitted")
		}
		manifest = append(manifest, r)
	}
	if err = rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	data := f.storageBytes(s.read, manifest)
	keys := []string{}
	for obj := range s.client.ListObjects(f.ctx, f.run+"-private", minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			f.t.Fatal(obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	if len(keys) != len(manifest) {
		f.t.Fatal("object/metadata inventory count mismatch")
	}
	known := map[string]bool{}
	for _, r := range manifest {
		known[r.Key] = true
	}
	for _, k := range keys {
		if !known[k] {
			f.t.Fatal("orphan object")
		}
	}
	return manifest, data
}
func (f *rehearsal) storageBytes(client *minio.Client, manifest []rehearsalFile) map[string][]byte {
	data := map[string][]byte{}
	for _, r := range manifest {
		obj, err := client.GetObject(f.ctx, f.run+"-private", r.Key, minio.GetObjectOptions{})
		if err != nil {
			f.t.Fatal(err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(obj, 7<<20))
		closeErr := obj.Close()
		sum := sha256.Sum256(raw)
		if readErr != nil || closeErr != nil || int64(len(raw)) != r.Bytes || hex.EncodeToString(sum[:]) != r.SHA256 {
			f.t.Fatal("independent object SHA256/bytes mismatch", readErr, closeErr)
		}
		data[r.Key] = raw
	}
	return data
}
func (f *rehearsal) copyObject(source, dest *minio.Client, key string, want []byte, multipartCopy bool) {
	obj, err := source.GetObject(f.ctx, f.run+"-private", key, minio.GetObjectOptions{})
	if err != nil {
		f.t.Fatal(err)
	}
	raw, err := io.ReadAll(io.LimitReader(obj, 7<<20))
	_ = obj.Close()
	if err != nil || !bytes.Equal(raw, want) {
		f.t.Fatal("source logical copy mismatch")
	}
	if multipartCopy {
		if len(raw) != (5<<20)+17 {
			f.t.Fatal("explicit multipart fixture size mismatch")
		}
		core := minio.Core{Client: dest}
		upload, err := core.NewMultipartUpload(f.ctx, f.run+"-private", key, minio.PutObjectOptions{})
		if err != nil {
			f.t.Fatal(err)
		}
		parts := make([]minio.CompletePart, 0, 2)
		for i, chunk := range [][]byte{raw[:5<<20], raw[5<<20:]} {
			p, err := core.PutObjectPart(f.ctx, f.run+"-private", key, upload, i+1, bytes.NewReader(chunk), int64(len(chunk)), minio.PutObjectPartOptions{})
			if err != nil {
				f.t.Fatal(err)
			}
			parts = append(parts, minio.CompletePart{PartNumber: i + 1, ETag: p.ETag})
		}
		if _, err = core.CompleteMultipartUpload(f.ctx, f.run+"-private", key, upload, parts, minio.PutObjectOptions{}); err != nil {
			f.t.Fatal(err)
		}
	} else {
		if _, err = dest.PutObject(f.ctx, f.run+"-private", key, bytes.NewReader(raw), int64(len(raw)), minio.PutObjectOptions{}); err != nil {
			f.t.Fatal(err)
		}
	}
	verify, err := dest.GetObject(f.ctx, f.run+"-private", key, minio.GetObjectOptions{})
	if err != nil {
		f.t.Fatal(err)
	}
	copied, err := io.ReadAll(io.LimitReader(verify, 7<<20))
	_ = verify.Close()
	a, b := sha256.Sum256(raw), sha256.Sum256(copied)
	if err != nil || len(copied) != len(raw) || !bytes.Equal(copied, raw) || a != b {
		f.t.Fatal("logical migration exact byte/count/SHA256 mismatch")
	}
}
func (f *rehearsal) copyManifest(source, dest *minio.Client, manifest []rehearsalFile, data map[string][]byte) {
	for _, r := range manifest {
		f.copyObject(source, dest, r.Key, data[r.Key], r.Bytes == (5<<20)+17)
	}
	got := f.storageBytes(dest, manifest)
	f.equalBytes(data, got)
	count := 0
	for o := range dest.ListObjects(f.ctx, f.run+"-private", minio.ListObjectsOptions{Recursive: true}) {
		if o.Err != nil {
			f.t.Fatal(o.Err)
		}
		count++
	}
	if count != len(manifest) {
		f.t.Fatal("logical copy count mismatch")
	}
	f.t.Logf("logical scoped S3 copy verified objects=%d; keys/bytes/count/SHA256 exact; no ETag integrity inference", count)
}
func (f *rehearsal) equalManifest(a, b []rehearsalFile) {
	if !reflect.DeepEqual(a, b) {
		f.t.Fatal("database/owner/key/lifecycle manifest differs")
	}
}
func (f *rehearsal) equalBytes(a, b map[string][]byte) {
	if !reflect.DeepEqual(a, b) {
		f.t.Fatal("object byte sets differ")
	}
}
func (f *rehearsal) checkpoint(d rehearsalDB, suffix string) (string, string) {
	f.verify("container", d.id)
	f.verify("volume", d.volume)
	remote := "/tmp/" + f.run + "-" + suffix + ".dump"
	f.docker("exec", d.id, "pg_dump", "--format=custom", "-U", "sfd", "-d", d.name, "--file", remote)
	file := filepath.Join(f.evidence, suffix+".dump")
	f.docker("cp", d.id+":"+remote, file)
	if err := os.Chmod(file, 0600); err != nil {
		f.t.Fatal(err)
	}
	// #nosec G304 -- Fixed checkpoint basename in the fixture-owned unique evidence directory.
	raw, err := os.ReadFile(file)
	if err != nil {
		f.t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return file, hex.EncodeToString(sum[:])
}
func (f *rehearsal) accept(a rehearsalApp, manifest []rehearsalFile, data map[string][]byte) {
	alice, bob, admin := f.login(a, "alice"), f.login(a, "bob"), f.login(a, "admin")
	f.expect(f.request(a, "GET", "/files", nil, nil, "").code, 401)
	f.expect(f.request(a, "GET", "/admin/files", nil, bob, "").code, 403)
	all := f.request(a, "GET", "/admin/files", nil, admin, "")
	f.expect(all.code, 200)
	var adminFiles []map[string]any
	if json.Unmarshal(all.body, &adminFiles) != nil || len(adminFiles) != len(manifest) {
		f.t.Fatal("administrator inventory mismatch")
	}
	for _, r := range manifest {
		owner, other := alice, bob
		if r.Username == "bob" {
			owner, other = bob, alice
		} else if r.Username != "alice" {
			if !strings.HasPrefix(r.Username, "browser_") {
				f.t.Fatal("unexpected synthetic owner")
			}
			browserLogin := f.request(a, "POST", "/login", f.json(map[string]string{"username": r.Username, "password": "SyntheticBrowser123"}), nil, "")
			f.expect(browserLogin.code, 200)
			if len(browserLogin.cookies) != 1 {
				f.t.Fatal("browser owner missing session")
			}
			owner, other = browserLogin.cookies[0], alice
		}
		f.expect(f.request(a, "POST", "/links", f.json(map[string]any{"id": r.ID, "ttl_seconds": 300}), other, "").code, 404)
		link := f.request(a, "POST", "/links", f.json(map[string]any{"id": r.ID, "ttl_seconds": 300}), owner, "")
		f.expect(link.code, 200)
		var v struct{ URL string }
		if json.Unmarshal(link.body, &v) != nil || v.URL == "" {
			f.t.Fatal("signed link missing")
		}
		downloaded := f.request(a, "GET", v.URL, nil, nil, "")
		f.expect(downloaded.code, 200)
		if !bytes.Equal(downloaded.body, data[r.Key]) {
			f.t.Fatal("signed download bytes mismatch")
		}
		f.expect(f.request(a, "GET", v.URL+"x", nil, nil, "").code, 401)
	}
}
func (f *rehearsal) permissions(s rehearsalStore, key string) {
	bucket := f.run + "-private"
	policy, err := s.client.GetBucketPolicy(f.ctx, bucket)
	if err != nil || strings.Contains(policy, "Allow") {
		f.t.Fatal("destination private policy check failed", err)
	}
	req, err := http.NewRequestWithContext(f.ctx, "GET", "http://"+s.addr+"/"+bucket+"/"+key, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	_ = res.Body.Close()
	f.expect(res.StatusCode, 403)
	// Ordinary correctly signed denial checks only; no signature bypass fixture.
	outsider := candidateClient(f.t, s.addr, f.outsider, f.outsiderSecret)
	_, err = outsider.StatObject(f.ctx, bucket, key, minio.StatObjectOptions{})
	denied(f.t, outsider, err)
	app := candidateClient(f.t, s.addr, f.app, f.appSecret)
	_, err = app.StatObject(f.ctx, f.run+"-other", "anything", minio.StatObjectOptions{})
	denied(f.t, app, err)
	err = app.SetBucketPolicy(f.ctx, bucket, privateDenyPolicy(bucket))
	denied(f.t, app, err)
	reader := candidateClient(f.t, s.addr, f.reader, f.readerSecret)
	_, err = reader.PutObject(f.ctx, bucket, "denied", bytes.NewReader([]byte{1}), 1, minio.PutObjectOptions{})
	denied(f.t, reader, err)
	f.t.Log("distinct generated root/app/reader/outsider; anonymous, cross-bucket, cross-identity and administrative policy change denied")
}
func (f *rehearsal) writeEvidence(name string, value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		f.t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(f.evidence, name), append(raw, '\n'), 0600); err != nil {
		f.t.Fatal(err)
	}
}
func (f *rehearsal) configuration() {
	env := map[string]string{"SFD_PREPARATION_RUN": f.run, "SFD_PREPARATION_DB": "sfd_configuration", "SFD_ROOT_KEY": f.root, "SFD_ROOT_SECRET": f.rootSecret, "SFD_APP_KEY": f.app, "SFD_APP_SECRET": f.appSecret, "SFD_PG_SECRET": f.pgSecret, "SFD_ADMIN_SECRET": f.adminSecret, "SFD_SESSION_SECRET": f.sessionSecret, "SFD_DOWNLOAD_SECRET": f.downloadSecret}
	file := filepath.Join(f.evidence, "configuration.env")
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var body strings.Builder
	for _, k := range keys {
		body.WriteString(k + "=" + env[k] + "\n")
	}
	if err := os.WriteFile(file, []byte(body.String()), 0600); err != nil {
		f.t.Fatal(err)
	}
	raw := f.docker("compose", "--env-file", file, "-f", "compose.rustfs-preparation.yml", "--profile", "preparation", "config", "--format", "json")
	var config struct {
		Services map[string]struct {
			Image, User                               string
			Environment                               map[string]string
			CapDrop, SecurityOpt, Entrypoint, Command []string
			Privileged                                bool
			Ports                                     []map[string]any
			Networks                                  map[string]any
		}
	}
	// Compose JSON uses underscores for these two runtime properties.
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\"cap_drop\"", "\"CapDrop\""), "\"security_opt\"", "\"SecurityOpt\"")
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		f.t.Fatal(err)
	}
	rust := config.Services["rustfs"]
	app := config.Services["backend"]
	if rust.Image != preparationRustImage || rust.User != "10001:10001" || rust.Privileged || !reflect.DeepEqual(rust.CapDrop, []string{"ALL"}) || !reflect.DeepEqual(rust.SecurityOpt, []string{"no-new-privileges:true"}) || !reflect.DeepEqual(rust.Entrypoint, []string{"/usr/bin/rustfs"}) || !reflect.DeepEqual(rust.Command, []string{"/data"}) {
		f.t.Fatal("preparation runtime constraints differ")
	}
	if len(rust.Ports) != 1 || rust.Ports[0]["host_ip"] != "127.0.0.1" || len(rust.Networks) != 1 || app.Environment["SFD_S3_ACCESS_KEY"] != f.app || app.Environment["SFD_S3_SECRET_KEY"] != f.appSecret {
		f.t.Fatal("preparation network/credential boundary differs")
	}
	for _, v := range app.Environment {
		if v == f.root || strings.Contains(v, f.rootSecret) {
			f.t.Fatal("backend received root identity")
		}
	}
	for _, key := range []string{"RUSTFS_CONSOLE_ENABLE", "RUSTFS_FTP_ENABLE", "RUSTFS_FTPS_ENABLE", "RUSTFS_SFTP_ENABLE", "RUSTFS_WEBDAV_ENABLE"} {
		if rust.Environment[key] != "false" {
			f.t.Fatal("optional service not disabled")
		}
	}
	f.rustEnv = rust.Environment
	f.t.Log("actual preparation Compose rendered; private dedicated network, fresh volumes, test-loopback S3, separate backend identity; rendered RustFS runtime settings drive containers")
}
func (f *rehearsal) restrictions(s rehearsalStore) {
	var inspect []struct {
		Config     struct{ User string }
		HostConfig struct {
			Privileged           bool
			CapDrop, SecurityOpt []string
		}
	}
	if err := json.Unmarshal([]byte(f.docker("inspect", s.id)), &inspect); err != nil || len(inspect) != 1 {
		f.t.Fatal("runtime inspection failed")
	}
	got := inspect[0]
	if got.Config.User != "10001:10001" || got.HostConfig.Privileged || !reflect.DeepEqual(got.HostConfig.CapDrop, []string{"ALL"}) || !strings.Contains(strings.Join(got.HostConfig.SecurityOpt, ","), "no-new-privileges") {
		f.t.Fatal("executed runtime privilege boundary mismatch")
	}
	if f.docker("port", s.id) != "9000/tcp -> "+s.addr {
		f.t.Fatal("unexpected published storage service")
	}
	for _, p := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		for _, line := range strings.Split(f.docker("exec", s.id, "cat", p), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			parts := strings.Split(fields[1], ":")
			port, err := strconv.ParseInt(parts[len(parts)-1], 16, 32)
			if err != nil {
				f.t.Fatal(err)
			}
			if port != 9000 && parts[0] != "0B00007F" { // Docker embedded DNS is container loopback only.
				f.t.Fatalf("unexpected RustFS TCP listener %d", port)
			}
		}
	}
}
