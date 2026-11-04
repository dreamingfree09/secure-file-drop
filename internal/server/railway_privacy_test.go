package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const privacyTestBucket = "owned-fixture"
const privacyTestHost = "owned-fixture.t3.storageapi.dev"

type privacyReadError struct{}

func (privacyReadError) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type privacyBody struct {
	io.Reader
	close func() error
}

func (b privacyBody) Close() error { return b.close() }

type privacyFixture struct {
	mu                                        sync.Mutex
	mode                                      string
	objects                                   map[string][]byte
	seen                                      map[string]bool
	requests, puts, deletes, open, signedGets int
	closedBodies                              chan struct{}
}

func newPrivacyFixture(mode string) *privacyFixture {
	return &privacyFixture{mode: mode, objects: map[string][]byte{}, seen: map[string]bool{}, closedBodies: make(chan struct{}, 1)}
}

func (f *privacyFixture) response(req *http.Request, status int, value string, headers http.Header) (*http.Response, error) {
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Length", strconv.Itoa(len(value)))
	headers.Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	headers.Set("ETag", `"fixture-etag"`)
	f.open++
	closed := false
	var reader io.Reader = strings.NewReader(value)
	if f.mode == "denial_read_error" && req.Header.Get("Authorization") == "" || f.mode == "authenticated_read_error" && req.Method == http.MethodGet {
		reader = io.MultiReader(reader, privacyReadError{})
	}
	body := privacyBody{Reader: reader, close: func() error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !closed {
			f.open--
			closed = true
			select {
			case f.closedBodies <- struct{}{}:
			default:
			}
		}
		if f.mode == "denial_close_error" && req.Header.Get("Authorization") == "" {
			return errors.New("private-provider-sentinel")
		}
		return nil
	}}
	return &http.Response{StatusCode: status, Header: headers, Body: body, ContentLength: int64(len(value)), Request: req}, nil
}

// The SDK closes response readers asynchronously after Object.Close returns.
// Wait for that cancellation, with a failing bound and no polling sleep.
func (f *privacyFixture) requireClosed(t *testing.T) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		f.mu.Lock()
		open := f.open
		f.mu.Unlock()
		if open == 0 {
			return
		}
		select {
		case <-f.closedBodies:
		case <-deadline.C:
			t.Fatal("SDK response reader did not close within the fixture deadline")
		}
	}
}

func privacyXML(code, bucket, key string) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(key))
	resource := "/" + bucket
	if key != "" {
		resource += "/" + key
	}
	return "<Error><Code>" + code + "</Code><BucketName>" + bucket + "</BucketName><Key>" + escaped.String() + "</Key><Resource>" + resource + "</Resource></Error>"
}

func (f *privacyFixture) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if req.URL.Host != privacyTestHost || req.URL.Scheme != "https" {
		return nil, errors.New("unexpected fixture destination")
	}
	signed := req.Header.Get("Authorization") != ""
	key := strings.TrimPrefix(req.URL.Path, "/")
	listing := key == ""
	if !signed {
		if req.Header.Get("Cookie") != "" || req.Header.Get("X-Amz-Security-Token") != "" || req.URL.Query().Has("X-Amz-Signature") {
			return nil, errors.New("credentials reached unsigned fixture")
		}
		if !listing && !f.seen[key] {
			return nil, errors.New("unsigned request changed object key")
		}
		if f.mode == "unsigned_network" {
			return nil, errors.New("private-provider-sentinel")
		}
		if f.mode == "unsigned_timeout" {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		if f.mode == "object_disappears" {
			delete(f.objects, key)
		}
		if req.Method == http.MethodPut && (f.mode == "public_write" || f.mode == "denied_write_mutates") {
			value, err := io.ReadAll(req.Body)
			if err != nil { return nil, err }
			f.objects[key] = value
			if f.mode == "public_write" { return f.response(req, 200, "", nil) }
		}
		if req.Method == http.MethodDelete && (f.mode == "public_delete" || f.mode == "denied_delete_mutates") {
			delete(f.objects, key)
			if f.mode == "public_delete" { return f.response(req, 204, "", nil) }
		}
		if f.mode == "public_object" && !listing {
			return f.response(req, 200, string(f.objects[key]), nil)
		}
		if f.mode == "public_list_v1" && listing && !req.URL.Query().Has("list-type") {
			return f.response(req, 200, "<ListBucketResult/>", nil)
		}
		if f.mode == "public_list_v2" && listing && req.URL.Query().Has("list-type") {
			return f.response(req, 200, "<ListBucketResult/>", nil)
		}
		if f.mode == "unsigned_redirect" {
			return f.response(req, 307, "", http.Header{"Location": []string{"https://redirect.invalid/never"}})
		}
		if strings.HasPrefix(f.mode, "unsigned_status_") {
			n, _ := strconv.Atoi(strings.TrimPrefix(f.mode, "unsigned_status_"))
			return f.response(req, n, privacyXML("AccessDenied", privacyTestBucket, key), nil)
		}
		body := privacyXML("AccessDenied", privacyTestBucket, key)
		switch f.mode {
		case "denial_wrong_code":
			body = privacyXML("InvalidAccessKeyId", privacyTestBucket, key)
		case "denial_wrong_key":
			body = privacyXML("AccessDenied", privacyTestBucket, "another-key")
		case "denial_wrong_bucket":
			body = privacyXML("AccessDenied", "another-bucket", key)
		case "denial_wrong_resource":
			body = strings.Replace(body, "<Resource>", "<Resource>/another", 1)
		case "denial_html":
			body = "<html>Forbidden</html>"
		case "denial_truncated":
			body = "<Error><Code>AccessDenied</Code>"
		case "denial_trailing":
			body += "<Error><Code>AccessDenied</Code></Error>"
		case "denial_duplicate_code":
			body = strings.Replace(body, "<Code>AccessDenied</Code>", "<Code>InternalError</Code><Code>AccessDenied</Code>", 1)
		case "denial_duplicate_key":
			body = strings.Replace(body, "<Key>", "<Key>wrong</Key><Key>", 1)
		case "denial_oversize":
			body += strings.Repeat(" ", 8193)
		}
		return f.response(req, 403, body, nil)
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		return nil, errors.New("SDK did not sign fixture request")
	}
	switch req.Method {
	case http.MethodPut:
		f.puts++
		if !strings.HasPrefix(key, "_sfd_privacy_attestation/") || len(strings.TrimPrefix(key, "_sfd_privacy_attestation/")) != 64 || req.Header.Get("If-None-Match") != "*" || req.Header.Get("Cache-Control") != "no-store" || req.URL.RawQuery != "" {
			return nil, errors.New("unsafe probe PUT")
		}
		for h, v := range req.Header {
			if strings.Contains(strings.ToLower(h), "grant") || strings.EqualFold(h, "x-amz-acl") && strings.Join(v, "") != "private" {
				return nil, errors.New("public ACL request")
			}
		}
		if f.seen[key] {
			return nil, errors.New("probe key reused")
		}
		f.seen[key] = true
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if f.mode == "put_before_error" {
			return f.response(req, 500, privacyXML("InternalError", privacyTestBucket, key), nil)
		}
		if f.mode == "put_collision" || f.mode == "unknown_put_foreign" {
			f.objects[key] = []byte("preserved foreign bytes")
			status := 412
			code := "PreconditionFailed"
			if f.mode == "unknown_put_foreign" {
				status = 500
				code = "InternalError"
			}
			return f.response(req, status, privacyXML(code, privacyTestBucket, key), nil)
		}
		f.objects[key] = body
		if f.mode == "put_after_error" {
			return f.response(req, 500, privacyXML("InternalError", privacyTestBucket, key), nil)
		}
		if f.mode == "signed_redirect" {
			return f.response(req, 307, "", http.Header{"Location": []string{"https://redirect.invalid/never"}})
		}
		return f.response(req, 200, "", nil)
	case http.MethodGet:
		if listing {
			if f.mode == "authenticated_list_error" {
				return f.response(req, 403, privacyXML("AccessDenied", privacyTestBucket, ""), nil)
			}
			prefix := req.URL.Query().Get("prefix")
			body := "<ListBucketResult><Name>" + privacyTestBucket + "</Name><IsTruncated>false</IsTruncated>"
			for k, v := range f.objects {
				if strings.HasPrefix(k, prefix) {
					listed := k
					if f.mode == "authenticated_list_key" {
						listed = "foreign-key"
					}
					body += fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size><ETag>fixture-etag</ETag></Contents>", listed, len(v))
				}
			}
			return f.response(req, 200, body+"</ListBucketResult>", nil)
		}
		f.signedGets++
		value, ok := f.objects[key]
		if !ok {
			return f.response(req, 404, privacyXML("NoSuchKey", privacyTestBucket, key), nil)
		}
		if f.mode == "authenticated_get_error" {
			return f.response(req, 503, privacyXML("SlowDown", privacyTestBucket, key), nil)
		}
		if f.mode == "authenticated_get_corrupt" {
			value = []byte("different bytes")
		}
		if f.mode == "authenticated_get_long" {
			value = append(append([]byte{}, value...), 1)
		}
		if f.mode == "authenticated_recheck_corrupt" && f.signedGets > 1 {
			value = []byte("different final bytes")
		}
		return f.response(req, 200, string(value), nil)
	case http.MethodDelete:
		f.deletes++
		if f.mode == "cleanup_error" {
			return f.response(req, 403, privacyXML("AccessDenied", privacyTestBucket, key), nil)
		}
		if f.mode != "cleanup_ignored" {
			delete(f.objects, key)
		}
		return f.response(req, 204, "", nil)
	case http.MethodHead:
		if f.mode == "cleanup_head_error" {
			return f.response(req, 403, "", nil)
		}
		if v, ok := f.objects[key]; ok {
			return f.response(req, 200, string(v), nil)
		}
		return f.response(req, 404, "", nil)
	default:
		return nil, errors.New("unexpected SDK method")
	}
}

func privacyTestClients(signed, unsigned http.RoundTripper) (*minio.Client, *http.Client, error) {
	client, err := minio.New("t3.storageapi.dev", &minio.Options{
		Creds: credentials.NewStaticV4("synthetic-access", "synthetic-secret", ""), Secure: true, Region: "auto", BucketLookup: minio.BucketLookupDNS, MaxRetries: 1,
		Transport: privacyTransport{base: signed, host: privacyTestHost},
	})
	anon := &http.Client{Transport: privacyTransport{base: unsigned, host: privacyTestHost, unsigned: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client, anon, err
}

func TestRailwayPrivacyConfiguration(t *testing.T) {
	t.Setenv("SFD_ENV", "staging")
	t.Setenv("RAILWAY_PROJECT_ID", railwayStagingProject)
	t.Setenv("RAILWAY_ENVIRONMENT_ID", railwayStagingEnvironment)
	for _, mode := range []string{"", "bucket-policy", railwayPrivacyMode, "skip", "railway", railwayPrivacyMode + " "} {
		t.Setenv("SFD_S3_PRIVACY_MODE", mode)
		selected, err := railwayPrivacySelected()
		wantValid := mode == "" || mode == "bucket-policy" || mode == railwayPrivacyMode
		if (err == nil) != wantValid || selected != (mode == railwayPrivacyMode) {
			t.Errorf("mode selection failed for fixture mode %q", mode)
		}
	}
	if err := validateRailwayPrivacyConfig(railwayStorageEndpoint, privacyTestBucket, minio.BucketLookupDNS); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://t3.storageapi.dev", "https://t3.storageapi.dev.evil.invalid", "https://t3.storageapi.dev/", "https://t3.storageapi.dev:443", "https://t3.storageapi.dev?x=1", "https://secret@t3.storageapi.dev", "https://127.0.0.1", "https://T3.storageapi.dev"} {
		if validateRailwayPrivacyConfig(endpoint, privacyTestBucket, minio.BucketLookupDNS) == nil {
			t.Errorf("nonallowlisted endpoint accepted")
		}
	}
	for _, bucket := range []string{"x", "-bucket", "bucket-", "bucket.name", "bucket/name", "bucket:443", "UPPER", strings.Repeat("a", 64)} {
		if validateRailwayPrivacyConfig(railwayStorageEndpoint, bucket, minio.BucketLookupDNS) == nil {
			t.Errorf("invalid bucket identity accepted")
		}
	}
	for _, lookup := range []minio.BucketLookupType{minio.BucketLookupAuto, minio.BucketLookupPath} {
		if validateRailwayPrivacyConfig(railwayStorageEndpoint, privacyTestBucket, lookup) == nil {
			t.Error("nonvirtual lookup accepted")
		}
	}
	for _, name := range []string{"SFD_ENV", "RAILWAY_PROJECT_ID", "RAILWAY_ENVIRONMENT_ID"} {
		old := map[string]string{"SFD_ENV": "staging", "RAILWAY_PROJECT_ID": railwayStagingProject, "RAILWAY_ENVIRONMENT_ID": railwayStagingEnvironment}[name]
		t.Setenv(name, "other-context")
		if validateRailwayPrivacyConfig(railwayStorageEndpoint, privacyTestBucket, minio.BucketLookupDNS) == nil {
			t.Error("nonstaging scope accepted")
		}
		t.Setenv(name, old)
	}
	t.Log("configuration: default and explicit strict policy preserved; 24 mode/scope/endpoint/bucket negatives exercised")
}

func TestRailwayPrivacyAttestation(t *testing.T) {
	cases := []string{"private", "public_write", "public_delete", "denied_write_mutates", "denied_delete_mutates", "public_object", "public_list_v1", "public_list_v2", "unsigned_network", "unsigned_timeout", "unsigned_redirect", "unsigned_status_200", "unsigned_status_204", "unsigned_status_401", "unsigned_status_404", "unsigned_status_429", "unsigned_status_500", "denial_wrong_code", "denial_wrong_key", "denial_wrong_bucket", "denial_wrong_resource", "denial_html", "denial_truncated", "denial_trailing", "denial_oversize", "denial_close_error", "denial_read_error", "denial_duplicate_code", "denial_duplicate_key", "object_disappears", "put_before_error", "put_after_error", "put_collision", "unknown_put_foreign", "signed_redirect", "authenticated_get_error", "authenticated_read_error", "authenticated_get_corrupt", "authenticated_get_long", "authenticated_recheck_corrupt", "authenticated_list_error", "authenticated_list_key", "cleanup_error", "cleanup_ignored", "cleanup_head_error", "entropy_failure"}
	for _, mode := range cases {
		fixture := newPrivacyFixture(mode)
		client, anon, err := privacyTestClients(fixture, fixture)
		if err != nil {
			t.Fatal(err)
		}
		budget := 2 * time.Second
		if mode == "unsigned_timeout" {
			budget = 20 * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		entropy := rand.Reader
		if mode == "entropy_failure" {
			entropy = strings.NewReader("")
		}
		err = attestRailwayStorage(ctx, client, privacyTestBucket, anon, entropy)
		cancel()
		if (err == nil) != (mode == "private") {
			t.Errorf("case %s acceptance mismatch: %v", mode, err)
		}
		if err != nil && (strings.Contains(err.Error(), "private-provider-sentinel") || strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), privacyTestHost)) {
			t.Errorf("case %s leaked provider values", mode)
		}
		fixture.requireClosed(t)
		fixture.mu.Lock()
		if fixture.open != 0 {
			t.Errorf("case %s leaked %d bodies", mode, fixture.open)
		}
		preserved := mode == "put_collision" || mode == "unknown_put_foreign" || mode == "cleanup_error" || mode == "cleanup_ignored"
		if !preserved && len(fixture.objects) != 0 {
			t.Errorf("case %s did not clean owned probe", mode)
		}
		if (mode == "put_collision" || mode == "unknown_put_foreign") && (fixture.deletes != 0 || len(fixture.objects) != 1) {
			t.Errorf("case %s changed foreign data", mode)
		}
		if mode == "entropy_failure" && fixture.requests != 0 {
			t.Error("entropy failure performed IO")
		}
		if mode == "private" && (fixture.puts != 1 || fixture.deletes != 1 || fixture.signedGets != 2) {
			t.Error("positive path omitted signed controls/cleanup")
		}
		fixture.mu.Unlock()
	}
	t.Logf("SDK attestation cases executed: %d; includes uncertainty, public access, listing, body and cleanup faults", len(cases))
}

func TestRailwayPrivacyTransportBoundaries(t *testing.T) {
	for _, mutate := range []string{"host", "scheme", "userinfo", "authorization", "cookie", "token", "signed_query", "method"} {
		calls := 0
		base := policyFixtureTransport(func(_ *http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not run") })
		req, err := http.NewRequest(http.MethodGet, "https://"+privacyTestHost+"/known-key", nil)
		if err != nil {
			t.Fatal(err)
		}
		switch mutate {
		case "host":
			req.URL.Host = "other.invalid"
		case "scheme":
			req.URL.Scheme = "http"
		case "userinfo":
			req.URL.User = url.UserPassword("synthetic", "private")
		case "authorization":
			req.Header.Set("Authorization", "synthetic-private")
		case "cookie":
			req.Header.Set("Cookie", "synthetic-private")
		case "token":
			req.Header.Set("X-Amz-Security-Token", "synthetic-private")
		case "signed_query":
			req.URL.RawQuery = "X-Amz-Signature=synthetic-private"
		case "method":
			req.Method = http.MethodHead
		}
		guard := privacyTransport{base: base, host: privacyTestHost, unsigned: true}
		if _, err := guard.RoundTrip(req); err == nil || calls != 0 {
			t.Errorf("unsigned %s boundary was not enforced before IO", mutate)
		}
	}
	t.Log("8 credential/origin/request negatives rejected before transport IO")
}

func TestRailwayPrivacyConcurrent(t *testing.T) {
	fixture := newPrivacyFixture("private")
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, anon, err := privacyTestClients(fixture, fixture)
			if err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				err = attestRailwayStorage(ctx, client, privacyTestBucket, anon, rand.Reader)
			}
			failures <- err
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Error(err)
		}
	}
	fixture.requireClosed(t)
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.seen) != 12 || fixture.deletes != 12 || len(fixture.objects) != 0 || fixture.open != 0 {
		t.Error("concurrent starts collided or leaked resources")
	}
	t.Log("12 simultaneous starts: unique keys, distinct controls and verified cleanup")
}

func TestRailwayPrivacyTLS(t *testing.T) {
	for _, mode := range []string{"trusted", "untrusted", "wrong-hostname", "expired"} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		dns := privacyTestHost
		if mode == "wrong-hostname" {
			dns = "different.invalid"
		}
		end := time.Now().Add(time.Hour)
		if mode == "expired" {
			end = time.Now().Add(-time.Hour)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: dns}, DNSNames: []string{dns}, NotBefore: time.Now().Add(-2 * time.Hour), NotAfter: end, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newPrivacyFixture("private")
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req.URL.Scheme = "https"
			req.URL.Host = req.Host
			resp, err := fixture.RoundTrip(req)
			if err != nil {
				http.Error(w, "fixture failure", 500)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			for k, vs := range resp.Header {
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, resp.Body)
		}))
		server.Config.ErrorLog = log.New(io.Discard, "", 0)
		server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
		server.StartTLS()
		transport, err := minio.DefaultTransport(true)
		if err != nil {
			t.Fatal(err)
		}
		transport.Proxy = nil
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool()}
		if mode != "untrusted" {
			transport.TLSClientConfig.RootCAs.AddCert(cert)
		}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != privacyTestHost+":443" {
				return nil, errors.New("unexpected fixture dial target")
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}
		anonTransport := transport.Clone()
		client, anon, err := privacyTestClients(transport, anonTransport)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = attestRailwayStorage(ctx, client, privacyTestBucket, anon, rand.Reader)
		cancel()
		transport.CloseIdleConnections()
		anonTransport.CloseIdleConnections()
		server.Close()
		if (err == nil) != (mode == "trusted") {
			t.Errorf("real TLS %s result: %v", mode, err)
		}
		if len(fixture.objects) != 0 || fixture.open != 0 {
			t.Errorf("real TLS %s leaked resources", mode)
		}
		if mode != "trusted" && fixture.requests != 0 {
			t.Errorf("real TLS %s sent HTTP through rejected certificate", mode)
		}
	}
	t.Log("real HTTPS: fixture-root success; untrusted, hostname-mismatched and expired certificates rejected without HTTP")
}
