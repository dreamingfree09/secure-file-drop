package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Real Traefik uses the middleware/router settings from rendered Compose labels.
// No Docker socket is exposed to this disposable proxy.
func verifyProxy(ctx context.Context, t *testing.T, run, network, image string, appArgs []string,
	docker func(...string) string, own func(string, string), port func(string, string) string) {
	t.Helper()
	dir := t.TempDir()
	env := "POSTGRES_DB=fixture\nPOSTGRES_USER=fixture\nPOSTGRES_PASSWORD=synthetic\nDATABASE_URL=postgres://postgres/fixture\nMINIO_ROOT_USER=fixture\nMINIO_ROOT_PASSWORD=syntheticfixture123\nSFD_BUCKET=fixture\nSFD_ADMIN_USER=admin\nSFD_ADMIN_PASS=synthetic\nSFD_SESSION_SECRET=synthetic-session-secret-for-isolated-tests\nSFD_DOWNLOAD_SECRET=synthetic-download-secret-for-isolated-tests\nSFD_PUBLIC_BASE_URL=https://localhost:8443\nSFD_MAX_UPLOAD_BYTES=1024\nSFD_MAX_REQUEST_BYTES=1049600\n"
	envPath := filepath.Join(dir, "synthetic.env")
	if err := os.WriteFile(envPath, []byte(env), 0600); err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]struct {
			Labels      map[string]string
			Environment map[string]string
			Ports       []any
		}
	}
	raw := docker("compose", "--env-file", envPath, "-p", run, "config", "--format", "json")
	if err := json.Unmarshal([]byte(raw), &compose); err != nil {
		t.Fatal("compose JSON", err)
	}
	labels := compose.Services["backend"].Labels
	if labels["traefik.http.middlewares.sfd-buf.buffering.maxRequestBodyBytes"] != "1049600" ||
		labels["traefik.http.middlewares.sfd-redirect-https.redirectscheme.port"] != "8443" ||
		compose.Services["backend"].Environment["SFD_MAX_REQUEST_BYTES"] != "1049600" ||
		len(compose.Services["minio"].Ports) != 0 {
		t.Fatal("Compose envelope, external port or storage isolation mismatch")
	}
	// Build middleware objects from the actual rendered labels.
	middlewares := map[string]any{}
	for _, name := range []string{"sfd-sec", "sfd-rl", "sfd-buf", "sfd-redirect-https"} {
		settings := map[string]any{}
		prefix := "traefik.http.middlewares." + name + "."
		for k, v := range labels {
			if !strings.HasPrefix(k, prefix) {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(k, prefix), ".")
			if len(parts) != 2 {
				t.Fatal("unexpected middleware label", k)
			}
			nested, ok := settings[parts[0]].(map[string]any)
			if !ok {
				nested = map[string]any{}
				settings[parts[0]] = nested
			}
			var value any = v
			if v == "true" || v == "false" {
				value = v == "true"
			} else if n, e := strconv.ParseInt(v, 10, 64); e == nil && parts[0] != "redirectscheme" {
				value = n
			}
			nested[parts[1]] = value
		}
		middlewares[name] = settings
	}
	// Traefik file-provider names use camel case for these middleware types.
	for _, pair := range [][2]string{{"sfd-rl", "rateLimit"}, {"sfd-redirect-https", "redirectScheme"}} {
		m := middlewares[pair[0]].(map[string]any)
		for k, v := range m {
			if strings.EqualFold(k, pair[1]) {
				delete(m, k)
				m[pair[1]] = v
				break
			}
		}
	}
	appName := run + "-proxy-app"
	config := map[string]any{
		"http": map[string]any{
			"routers": map[string]any{
				"sfd":     map[string]any{"rule": labels["traefik.http.routers.sfd.rule"], "entryPoints": []string{"websecure"}, "tls": map[string]any{}, "service": "sfd", "middlewares": strings.Split(labels["traefik.http.routers.sfd.middlewares"], ",")},
				"sfd-web": map[string]any{"rule": labels["traefik.http.routers.sfd-web.rule"], "entryPoints": []string{"web"}, "service": "sfd", "middlewares": []string{"sfd-redirect-https"}},
			},
			"middlewares": middlewares,
			"services":    map[string]any{"sfd": map[string]any{"loadBalancer": map[string]any{"servers": []any{map[string]string{"url": "http://" + appName + ":8080"}}}}},
		},
		"tls": map[string]any{"certificates": []any{map[string]string{"certFile": "/fixture/cert.pem", "keyFile": "/fixture/key.pem"}}},
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "SFD disposable proxy"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	for name, b := range map[string][]byte{"cert.pem": cert, "key.pem": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})} {
		if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	b, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "dynamic.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	proxy := docker("run", "-d", "--name", run+"-proxy", "--label", "sfd.quality-run="+run, "--network", network, "-p", "127.0.0.1::80", "-p", "127.0.0.1::443", "-v", dir+":/fixture:ro", "traefik:v2.11",
		"--providers.file.filename=/fixture/dynamic.json", "--entrypoints.web.address=:80", "--entrypoints.websecure.address=:443")
	own("container", proxy)
	tlsAddress := port(proxy, "443/tcp")
	_, tlsPort, err := net.SplitHostPort(tlsAddress)
	if err != nil {
		t.Fatal(err)
	}
	base := "https://localhost:" + tlsPort
	args := append([]string(nil), appArgs...)
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--name" {
			args[i+1] = appName
		}
	}
	args = append(args[:len(args)-1], "-e", "SFD_MAX_REQUEST_BYTES=1049600", "-e", "SFD_PUBLIC_BASE_URL="+base, image)
	app := docker(args...)
	own("container", app)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("fixture cert")
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	defer client.CloseIdleConnections()
	request := func(method, path string, body io.Reader, cookie *http.Cookie, contentType string) (int, []byte, []*http.Cookie) {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, method, base+path, body)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", base)
		r.Header.Set("Content-Type", contentType)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, b, res.Cookies()
	}
	deadline := time.Now().Add(90 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		res, e := client.Get(base + "/ready")
		if e == nil {
			_ = res.Body.Close()
			if res.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !ready {
		t.Fatal("real TLS proxy readiness failed")
	}
	redirectClient := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	httpAddress := port(proxy, "80/tcp")
	_, httpPort, _ := net.SplitHostPort(httpAddress)
	redirect, err := redirectClient.Get("http://localhost:" + httpPort + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = redirect.Body.Close()
	if redirect.StatusCode != 301 || redirect.Header.Get("Location") != "https://localhost:8443/" {
		t.Fatal("external HTTPS port redirect mismatch")
	}
	code, _, cookies := request("POST", "/login", strings.NewReader(`{"username":"alice","password":" SpacePass123 "}`), nil, "application/json")
	if code != 200 || len(cookies) != 1 || !cookies[0].Secure {
		t.Fatal("proxy secure login", code)
	}
	cookie := cookies[0]
	data := bytes.Repeat([]byte("z"), 1024)
	code, b, _ = request("POST", "/files", strings.NewReader(`{"orig_name":"proxy.bin","content_type":"application/octet-stream","size_bytes":1024}`), cookie, "application/json")
	var meta struct{ ID string }
	if code != 201 || json.Unmarshal(b, &meta) != nil {
		t.Fatal("proxy metadata", code)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	p, err := mw.CreateFormFile("file", "proxy.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = mw.Close(); err != nil {
		t.Fatal(err)
	}
	if body.Len() <= len(data) {
		t.Fatal("missing envelope overhead")
	}
	code, _, _ = request("POST", "/upload?id="+meta.ID, &body, cookie, mw.FormDataContentType())
	if code != 200 {
		t.Fatal("actual proxy exact-limit upload", code)
	}
	code, b, _ = request("POST", "/links", strings.NewReader(`{"id":"`+meta.ID+`"}`), cookie, "application/json")
	var link struct{ URL string }
	if code != 200 || json.Unmarshal(b, &link) != nil || !strings.HasPrefix(link.URL, base+"/download?") {
		t.Fatal("canonical proxy link", code)
	}
	code, b, _ = request("GET", strings.TrimPrefix(link.URL, base), nil, nil, "")
	if code != 200 || !bytes.Equal(b, data) {
		t.Fatal("proxy downloaded bytes mismatch")
	}
	t.Log("PASS: real Traefik TLS, actual Compose envelope/redirect settings, Secure cookie, exact-file-limit multipart upload and canonical byte-equal download")
}
