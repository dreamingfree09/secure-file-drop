package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Opt-in only. Production source and storage defaults remain unchanged.
// Credentials are synthetic and confined to fresh assignment-owned fixtures.
type storageFixture struct {
	candidate, dockerfile, binary, appAccess string
	args, command                            []string
}

func evaluationStorage(t *testing.T, run, bucket, password string) storageFixture {
	t.Helper()
	candidate := os.Getenv("SFD_STORAGE_EVALUATION")
	if candidate == "" {
		return storageFixture{dockerfile: "Dockerfile.storage", binary: "/usr/local/bin/minio", appAccess: "sfdtest",
			args: []string{"-e", "MINIO_ROOT_USER=sfdtest", "-e", "MINIO_ROOT_PASSWORD=" + password}, command: []string{"server", "/data"}}
	}
	if candidate == "rustfs" {
		return rustStorage(password)
	}
	if candidate != "seaweedfs" {
		t.Fatal("unsupported evaluation candidate")
	}
	names := []string{"sfdtest", "sfdapp", "sfdreader", "sfdoutsider"}
	actions := [][]string{{"Admin"}, {"Read:" + bucket, "Write:" + bucket, "List:" + bucket}, {"Read:" + bucket, "List:" + bucket}, {"Read:" + run + "-other", "Write:" + run + "-other", "List:" + run + "-other"}}
	identities := []any{}
	for i, name := range names {
		identities = append(identities, map[string]any{"name": name, "credentials": []any{map[string]string{"accessKey": name, "secretKey": password}}, "actions": actions[i]})
	}
	b, err := json.Marshal(map[string]any{"identities": identities})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "identities.json")
	// #nosec G306 -- Synthetic disposable credentials; container uid 1000 needs read access.
	if err := os.WriteFile(file, b, 0644); err != nil {
		t.Fatal(err)
	}
	masterFile := filepath.Join(t.TempDir(), "master.toml")
	// #nosec G306 -- Nonsecret test configuration for uid 1000.
	if err := os.WriteFile(masterFile, []byte("[master.volume_growth]\ncopy_1 = 1\ncopy_2 = 1\ncopy_3 = 1\ncopy_other = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return storageFixture{candidate: candidate, dockerfile: "tests/storage/Dockerfile.seaweedfs", binary: "/usr/bin/weed", appAccess: "sfdapp",
		args: []string{"--mount", "type=bind,src=" + file + ",dst=/etc/seaweedfs/sfd-identities.json,readonly", "--mount", "type=bind,src=" + masterFile + ",dst=/etc/seaweedfs/master.toml,readonly", "--cap-drop=ALL", "--security-opt", "no-new-privileges"},
		command: []string{"server", "-dir=/data", "-ip=127.0.0.1", "-ip.bind=127.0.0.1", "-master.telemetry=false", "-master.volumeSizeLimitMB=32", "-volume.max=4", "-volume.preStopSeconds=1",
			"-filer=true", "-s3=true", "-s3.ip.bind=0.0.0.0", "-s3.port=9000", "-s3.port.grpc=19000", "-s3.config=/etc/seaweedfs/sfd-identities.json",
			"-s3.port.iceberg=0", "-s3.port.lance=0", "-s3.iam=false", "-s3.autoCreateBucket=false", "-s3.allowDeleteBucketNotEmpty=false"}}
}
func candidateClient(t *testing.T, addr, access, password string) *minio.Client {
	t.Helper()
	mc, err := minio.New(addr, &minio.Options{Creds: credentials.NewStaticV4(access, password, ""), Secure: false, Region: "us-east-1", BucketLookup: minio.BucketLookupPath})
	if err != nil {
		t.Fatal(err)
	}
	return mc
}
func denied(t *testing.T, err error) {
	t.Helper()
	if minio.ToErrorResponse(err).StatusCode != http.StatusForbidden {
		t.Fatalf("expected signed permission denial; got %v", err)
	}
}
func verifyCandidateContract(ctx context.Context, t *testing.T, admin *minio.Client, addr, bucket, password, candidate string) {
	t.Helper()
	app := candidateClient(t, addr, "sfdapp", password)
	reader := candidateClient(t, addr, "sfdreader", password)
	outsider := candidateClient(t, addr, "sfdoutsider", password)
	unknown := candidateClient(t, addr, "unknown", password)
	anonymous, err := minio.New(addr, &minio.Options{Secure: false, BucketLookup: minio.BucketLookupPath})
	if err != nil {
		t.Fatal(err)
	}
	key := "candidate-contract"
	data := []byte("synthetic storage principal fixture")
	if _, err := app.PutObject(ctx, bucket, key, bytes.NewReader(data), -1, minio.PutObjectOptions{}); err != nil {
		t.Fatal("scoped unknown-length PUT", err)
	}
	if stat, err := app.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err != nil || stat.Size != int64(len(data)) {
		t.Fatal("stat mismatch", err)
	}
	object, err := reader.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(object)
	_ = object.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("reader bytes differ", err)
	}
	for _, client := range []*minio.Client{reader, outsider, unknown, anonymous} {
		_, err := client.PutObject(ctx, bucket, key, bytes.NewReader([]byte("denied")), 6, minio.PutObjectOptions{})
		denied(t, err)
		denied(t, client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}))
	}
	for _, client := range []*minio.Client{outsider, unknown, anonymous} {
		_, err := client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
		denied(t, err)
	}
	denied(t, reader.SetBucketPolicy(ctx, bucket, privateDenyPolicy(bucket)))
	if _, err := app.PutObject(ctx, bucket, "candidate-empty", bytes.NewReader(nil), -1, minio.PutObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	empty, err := app.StatObject(ctx, bucket, "candidate-empty", minio.StatObjectOptions{})
	if err != nil || empty.Size != 0 {
		t.Fatal("empty object", err)
	}
	// Two parts total only 5 MiB plus a tiny final part; no maximum-size claim.
	core := minio.Core{Client: app}
	uploadID, err := core.NewMultipartUpload(ctx, bucket, "candidate-multipart", minio.PutObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first := bytes.Repeat([]byte{0x53}, 5<<20)
	last := []byte("small-final-part!")
	parts := make([]minio.CompletePart, 0, 2)
	for i, body := range [][]byte{first, last} {
		part, err := core.PutObjectPart(ctx, bucket, "candidate-multipart", uploadID, i+1, bytes.NewReader(body), int64(len(body)), minio.PutObjectPartOptions{})
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, minio.CompletePart{PartNumber: i + 1, ETag: part.ETag})
	}
	if _, err := core.CompleteMultipartUpload(ctx, bucket, "candidate-multipart", uploadID, parts, minio.PutObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	obj, err := app.GetObject(ctx, bucket, "candidate-multipart", minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	n, err := io.Copy(h, obj)
	_ = obj.Close()
	want := sha256.Sum256(append(first, last...))
	if err != nil || n != int64(len(first)+len(last)) || !bytes.Equal(h.Sum(nil), want[:]) {
		t.Fatal("multipart integrity", err)
	}
	abort, err := core.NewMultipartUpload(ctx, bucket, "candidate-aborted", minio.PutObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.PutObjectPart(ctx, bucket, "candidate-aborted", abort, 1, bytes.NewReader(data), int64(len(data)), minio.PutObjectPartOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := core.AbortMultipartUpload(ctx, bucket, "candidate-aborted", abort); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StatObject(ctx, bucket, "candidate-aborted", minio.StatObjectOptions{}); minio.ToErrorResponse(err).StatusCode != 404 {
		t.Fatal("aborted upload became an object", err)
	}
	if _, err := admin.PutObject(ctx, bucket, "denied-only/object", bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := admin.SetBucketPolicy(ctx, bucket, privateDenyPolicy(bucket)); err != nil {
		t.Fatal("policy PUT", err)
	}
	if policy, err := app.GetBucketPolicy(ctx, bucket); err != nil || !strings.Contains(policy, "Deny") {
		t.Fatal("scoped policy GET", err)
	}
	_, err = reader.StatObject(ctx, bucket, "denied-only/object", minio.StatObjectOptions{})
	denied(t, err)
	if err := admin.SetBucketPolicy(ctx, bucket, ""); err != nil {
		t.Fatal("policy DELETE", err)
	}
	if policy, err := app.GetBucketPolicy(ctx, bucket); err != nil || strings.TrimSpace(policy) != "" {
		t.Fatal("absent policy contract", err)
	}
	if err := admin.SetBucketPolicy(ctx, bucket, privateDenyPolicy(bucket)); err != nil {
		t.Fatal(err)
	}
	if err := app.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS: %s SDK default discovery and explicit us-east-1/path-style, scoped app/reader/outsider, invalid/anonymous denials, policy GET/PUT/DELETE, empty/unknown-length, multipart completion/abort and SHA-256", candidate)
}
func privateDenyPolicy(bucket string) string {
	return fmt.Sprintf("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Deny\",\"Principal\":\"*\",\"Action\":\"s3:GetObject\",\"Resource\":\"arn:aws:s3:::%s/denied-only/*\"}]}", bucket)
}
func verifyCandidateStartup(ctx context.Context, t *testing.T, mc *minio.Client, bucket, run string, appArgs []string, docker func(...string) string, own func(string, string)) {
	t.Helper()
	// Deliberately public policy only in this owned synthetic bucket, restored
	// before the actual application starts. No real data is accessible.
	policy := fmt.Sprintf("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":\"*\",\"Action\":\"s3:GetObject\",\"Resource\":\"arn:aws:s3:::%s/*\"}]}", bucket)
	if err := mc.SetBucketPolicy(ctx, bucket, policy); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := mc.SetBucketPolicy(ctx, bucket, privateDenyPolicy(bucket)); err != nil {
			t.Error(err)
		}
	}()
	args := append([]string(nil), appArgs...)
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--name" {
			args[i+1] = run + "-public-policy-rejection"
		}
	}
	id := docker(args...)
	own("container", id)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if docker("inspect", "--format", "{{.State.Running}}", id) == "false" {
			if docker("inspect", "--format", "{{.State.ExitCode}}", id) == "0" {
				t.Fatal("public-policy startup exited successfully")
			}
			cli := os.Getenv("SFD_TEST_DOCKER")
			if cli == "" {
				cli = "docker"
			}
			// #nosec G204 G702 -- Parent-verified local CLI, owned container and fixed logs command; no raw log publication.
			logs, err := exec.CommandContext(ctx, cli, "logs", id).CombinedOutput()
			if err != nil || !bytes.Contains(logs, []byte("bucket policy must exclude anonymous access")) {
				t.Fatal("startup failed without the expected policy rejection")
			}
			t.Log("PASS: unchanged application refuses startup specifically for public Allow policy")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("application accepted public-policy startup")
}
func verifyCandidateRecovery(ctx context.Context, t *testing.T, storage, volume, run, label string, args []string, image, bucket, password string, docker func(...string) string, own func(string, string), port func(string, string) string) {
	t.Helper()
	for kind, name := range map[string]string{"container": storage, "volume": volume} {
		format := "{{index .Labels \"sfd.quality-run\"}}"
		if kind == "container" {
			format = "{{index .Config.Labels \"sfd.quality-run\"}}"
		}
		if docker(kind, "inspect", "--format", format, name) != run {
			t.Fatal("cold-copy source ownership mismatch")
		}
	}
	docker("stop", "--time", "60", storage)
	if docker("inspect", "--format", "{{.State.ExitCode}}", storage) != "0" {
		t.Fatal("storage did not quiesce cleanly")
	}
	restore := run + "-cold-restored"
	docker("volume", "create", "--label", label, restore)
	own("volume", restore)
	if docker("volume", "inspect", "--format", "{{index .Labels \"sfd.quality-run\"}}", restore) != run {
		t.Fatal("cold-copy destination ownership mismatch")
	}
	copier := docker("create", "--name", run+"-cold-copy", "--label", label, "--network", "none", "--user", docker("inspect", "--format", "{{.Config.User}}", storage), "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh", "-v", volume+":/source:ro", "-v", restore+":/data", image, "-c", "cp -R /source/. /data/")
	own("container", copier)
	docker("start", "-a", copier)
	if docker("inspect", "--format", "{{.State.ExitCode}}", copier) != "0" {
		t.Fatal("cold copy failed")
	}
	recoveredArgs := append([]string(nil), args...)
	for i := 0; i < len(recoveredArgs)-1; i++ {
		switch recoveredArgs[i] {
		case "--name":
			recoveredArgs[i+1] = run + "-cold-recovery"
		case "--network-alias":
			recoveredArgs[i+1] = "recovered-storage"
		case "-v":
			if recoveredArgs[i+1] == volume+":/data" {
				recoveredArgs[i+1] = restore + ":/data"
			}
		}
	}
	recovered := docker(recoveredArgs...)
	own("container", recovered)
	addr := port(recovered, "9000/tcp")
	client := candidateClient(t, addr, "sfdapp", password)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if stat, err := client.StatObject(ctx, bucket, "candidate-empty", minio.StatObjectOptions{}); err == nil && stat.Size == 0 {
			obj, err := client.GetObject(ctx, bucket, "candidate-multipart", minio.GetObjectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			h := sha256.New()
			n, err := io.Copy(h, obj)
			_ = obj.Close()
			want := sha256.Sum256(append(bytes.Repeat([]byte{0x53}, 5<<20), []byte("small-final-part!")...))
			if err != nil || n != (5<<20)+int64(len("small-final-part!")) || !bytes.Equal(h.Sum(nil), want[:]) {
				t.Fatal("cold recovery integrity mismatch")
			}
			if policy, err := client.GetBucketPolicy(ctx, bucket); err != nil || !strings.Contains(policy, "Deny") {
				t.Fatal("recovered private policy", err)
			}
			outsider := candidateClient(t, addr, "sfdoutsider", password)
			_, err = outsider.StatObject(ctx, bucket, "candidate-empty", minio.StatObjectOptions{})
			denied(t, err)
			t.Log("PASS: owned clean-stop cold copy and fresh candidate restart, persisted empty/multipart bytes and scoped permission denial; PostgreSQL restore tested separately")
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("cold-recovered storage unavailable")
}

// Inspect only the owned container's network namespace; never host sockets.
func verifyCandidateBindings(t *testing.T, id string, docker func(...string) string) {
	t.Helper()
	published := docker("port", id)
	if !strings.HasPrefix(published, "9000/tcp -> 127.0.0.1:") || strings.Contains(published, "\n") {
		t.Fatal("unexpected candidate host publication")
	}
	controls := map[int]bool{9333: false, 19333: false, 8888: false, 18888: false, 8080: false, 18080: false}
	for _, file := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		raw := docker("exec", id, "cat", file)
		for _, line := range strings.Split(raw, "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			address := strings.Split(fields[1], ":")
			if len(address) != 2 {
				continue
			}
			port, err := strconv.ParseInt(address[1], 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := controls[int(port)]; ok {
				if address[0] != "0100007F" && address[0] != "00000000000000000000000001000000" {
					t.Fatal("internal component control listener is not loopback")
				}
				controls[int(port)] = true
			}
			if port == 8181 || port == 9101 || port == 8733 || port == 7333 || port == 2022 {
				t.Fatal("unused optional service is listening")
			}
		}
	}
	for port, seen := range controls {
		if !seen {
			t.Fatalf("expected component listener %d missing", port)
		}
	}
	t.Log("PASS: master/filer/volume HTTP+gRPC bind container loopback; only S3 HTTP is host-published on loopback; S3 control gRPC 19000 remains on the trusted owned bridge, never host-published")
}
