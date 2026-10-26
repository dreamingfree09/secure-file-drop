package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestReadBoundsAndDigest(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		n    int64
		ok   bool
	}{{"exact", []byte("abc"), 3, true}, {"empty", nil, 0, true}, {"short", []byte("a"), 3, false}, {"long", []byte("abcd"), 3, false}, {"negative", nil, -1, false}, {"oversized", nil, maxObject + 1, false}} {
		t.Run(tc.name, func(t *testing.T) {
			h, e := readHash(bytes.NewReader(tc.data), tc.n)
			if (e == nil) != tc.ok {
				t.Fatal("bound result")
			}
			if tc.name == "exact" && h != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
				t.Fatal("SHA256 vector")
			}
		})
	}
}
func TestPolicyAndEndpoint(t *testing.T) {
	for _, p := range []string{`{"Statement":{"Effect":"Allow","Principal":"*"}}`, `{"Statement":{"Effect":"Allow","NotPrincipal":{"AWS":"named"},"Principal":{"AWS":"named"}}}`, "bad"} {
		if privateBucketPolicy(p) {
			t.Fatal("public/invalid policy")
		}
	}
	if !privateBucketPolicy("") {
		t.Fatal("private default")
	}
	for _, p := range []string{"http://user:pass@localhost:9000", "https://localhost:9000/path", "https://localhost:9000?q=1"} {
		if _, _, e := normaliseEndpoint(p); e == nil {
			t.Fatal("invalid endpoint")
		}
	}
	if strings.Contains(hashName("fixture", "private-key"), "private-key") {
		t.Fatal("name disclosure")
	}
}
func TestRealOwnedStorage(t *testing.T) {
	raw := os.Getenv("SFD_CONTROLLER_STORAGE_TEST")
	if raw == "" {
		t.Skip("requires newly owned isolated loopback fixture; not a pass")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("refusing non-fixture endpoint")
	}
	owner := os.Getenv("SFD_PROBE_FIXTURE_OWNER")
	image := os.Getenv("SFD_PROBE_FIXTURE_IMAGE")
	if !regexp.MustCompile(`^sfd-controller-storage-fixture-[a-f0-9]{12}$`).MatchString(owner) || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(image) {
		t.Fatal("fixture ownership identity")
	}
	var fixture []struct {
		Image           string
		Config          struct{ Labels map[string]string }
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string
			}
		}
		Mounts []struct{ Type, Name string }
	}
	dockerInspect(t, "containers/"+url.PathEscape(owner)+"/json", &fixture)
	if len(fixture) != 1 || fixture[0].Image != image || fixture[0].Config.Labels["sfd.controller-fixture"] != owner {
		t.Fatal("refusing non-owned fixture")
	}
	port := fixture[0].NetworkSettings.Ports["9000/tcp"]
	_, wanted, err := net.SplitHostPort(u.Host)
	if err != nil || len(port) != 1 || port[0].HostIP != "127.0.0.1" || port[0].HostPort != wanted || len(fixture[0].Mounts) != 1 || fixture[0].Mounts[0].Type != "volume" || fixture[0].Mounts[0].Name != owner+"-data" {
		t.Fatal("fixture target bounds")
	}
	var volumes []struct{ Labels map[string]string }
	dockerInspect(t, "volumes/"+url.PathEscape(owner+"-data"), &volumes)
	if len(volumes) != 1 || volumes[0].Labels["sfd.controller-fixture"] != owner {
		t.Fatal("fixture volume owner")
	}
	var nets []struct {
		Labels     map[string]string
		Internal   bool
		Containers map[string]any
	}
	dockerInspect(t, "networks/"+url.PathEscape(owner+"-net"), &nets)
	if len(nets) != 1 || nets[0].Labels["sfd.controller-fixture"] != owner || nets[0].Internal || len(nets[0].Containers) != 1 {
		t.Fatal("fixture network owner")
	}
	key, secret := os.Getenv("SFD_FIXTURE_ACCESS"), os.Getenv("SFD_FIXTURE_SECRET")
	c, e := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(key, secret, ""), Secure: false})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bucket := "sfd-controller-owned-fixture"
	if e = c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); e != nil {
		t.Fatal(e)
	}
	payload := bytes.Repeat([]byte{0x5a}, 512)
	if _, e = c.PutObject(ctx, bucket, "labeled-fixture", bytes.NewReader(payload), 512, minio.PutObjectOptions{}); e != nil {
		t.Fatal(e)
	}
	cfg := config{Endpoint: raw, AccessKey: key, SecretKey: secret, Bucket: bucket}
	first, e := inspect(ctx, cfg)
	if e != nil || !first.OK || len(first.Objects) != 1 || first.Total != 512 {
		t.Fatal("first read-only snapshot")
	}
	second, e := inspect(ctx, cfg)
	if e != nil || len(second.Objects) != 1 || first.Objects[0] != second.Objects[0] {
		t.Fatal("snapshot changed")
	}
}

// dockerInspect uses only GET on the existing local Unix socket. It cannot
// launch a variable command or contact a remote Docker endpoint.
func dockerInspect(t *testing.T, path string, out any) {
	t.Helper()
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
	}}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req := &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "http", Host: "docker", Path: "/" + path}, Host: "docker", Header: make(http.Header)}
	r, err := c.Do(req)
	if err != nil {
		t.Fatal("local fixture metadata unavailable")
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode != 200 {
		t.Fatal("local fixture identity missing")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1048577))
	if err != nil || len(raw) > 1048576 {
		t.Fatal("bounded fixture metadata")
	}
	wrapped := append([]byte{'['}, raw...)
	wrapped = append(wrapped, ']')
	if json.Unmarshal(wrapped, out) != nil {
		t.Fatal("fixture metadata shape")
	}
}
