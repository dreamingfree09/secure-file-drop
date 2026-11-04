package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestNormaliseEndpoint(t *testing.T) {
	tests := []struct {
		in           string
		wantEndpoint string
		wantSecure   bool
		wantErr      bool
	}{
		{"minio:9000", "minio:9000", false, false},
		{"http://minio:9000", "minio:9000", false, false},
		{"https://minio:9000", "minio:9000", true, false},
		{"http://minio:9000/", "minio:9000", false, false},
		{"http://minio:9000/foo", "", false, true},
		{"", "", false, true},
		{"ftp://minio:9000", "", false, true},
		{"https://user:secret@minio:9000", "", false, true},
		{"https://minio:9000?query=value", "", false, true},
		{"https://minio:9000#fragment", "", false, true},
	}

	for _, tt := range tests {
		ep, secure, err := normaliseEndpoint(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("expected error for input %q", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tt.in, err)
		}
		if ep != tt.wantEndpoint || secure != tt.wantSecure {
			t.Fatalf("normaliseEndpoint(%q) = (%q,%v), want (%q,%v)", tt.in, ep, secure, tt.wantEndpoint, tt.wantSecure)
		}
	}
}

func TestPrivateBucketPolicy(t *testing.T) {
	for _, tt := range []struct {
		policy  string
		private bool
	}{
		{"", true},
		{"invalid", false},
		{`{"Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject"}]}`, false},
		{`{"Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]}}]}`, false},
		{`{"Statement":[{"Effect":"Allow","Principal":{"AWS":["arn:aws:iam::123:user/backend"]}}]}`, true},
		{`{"Statement":{"Effect":"Deny","Principal":"*"}}`, true},
		{`{"Statement":{"Effect":"Allow","NotPrincipal":{"AWS":"arn:aws:iam::123:user/backend"}}}`, false},
	} {
		if got := privateBucketPolicy(tt.policy); got != tt.private {
			t.Fatalf("policy privacy got %v want %v", got, tt.private)
		}
	}
}

func TestStorageBucketLookup(t *testing.T) {
	// Never expose arbitrary provider errors or silently authorize an unknown
	// response. Rejection is mandatory even when the error is recognizable.
	for _, tc := range []struct {
		providerCode string
		category     string
	}{
		{"AccessDenied", "access_denied"},
		{"NotImplemented", "operation_unsupported"},
		{"SignatureDoesNotMatch", "credential_or_signature"},
		{"NoSuchBucket", "bucket_missing"},
		{"UnexpectedSecretHostAndToken", "unclassified"},
	} {
		err := classifyBucketPolicyError(minio.ErrorResponse{Code: tc.providerCode, Message: "sensitive-staging-token.example"})
		if err == nil || !strings.Contains(err.Error(), tc.category) || strings.Contains(err.Error(), "sensitive-staging-token") ||
			!strings.Contains(err.Error(), "fail-closed") {
			t.Fatalf("policy category %s was not safely rejected: %v", tc.providerCode, err)
		}
	}
	if err := classifyBucketPolicyError(errors.New("private-key-and-address")); err == nil ||
		strings.Contains(err.Error(), "private-key-and-address") {
		t.Fatal("unknown provider error leaked untrusted text")
	}
	for _, tc := range []struct {
		name   string
		want   minio.BucketLookupType
		reject bool
	}{
		{"", minio.BucketLookupAuto, false},
		{"auto", minio.BucketLookupAuto, false},
		{"virtual", minio.BucketLookupDNS, false},
		{"path", minio.BucketLookupPath, false},
		{"VIRTUAL", minio.BucketLookupDNS, false},
		{"invalid", minio.BucketLookupAuto, true},
		{"virtual-host://unsafe", minio.BucketLookupAuto, true},
	} {
		got, err := storageBucketLookup(tc.name)
		if tc.reject {
			if err == nil {
				t.Fatalf("invalid addressing mode %q accepted", tc.name)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("bucket lookup %q = %d, %v; want %d", tc.name, got, err, tc.want)
		}
	}
	// Real TLS HTTP requests prove that signed and anonymous reads share
	// the exact path, transport and headers. A generic frontend 403 fails.
	for _, tc := range []struct {
		status  int
		body    string
		allowed bool
	}{
		{http.StatusForbidden, `<Error><Code>AccessDenied</Code></Error>`, true},
		{http.StatusForbidden, `blocked by CDN`, false},
		{http.StatusForbidden, `<Error><Code>InvalidToken</Code></Error>`, false},
		{http.StatusNotFound, `<Error><Code>NoSuchKey</Code></Error>`, false},
		{http.StatusOK, `public-object`, false},
		{http.StatusTemporaryRedirect, `redirect`, false},
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") != "sfd-private-probe/1" ||
				r.Header.Get("Cache-Control") != "no-store" || r.URL.Path != "/canary" {
				t.Error("probe requests must have matched paths and headers")
			}
			if r.URL.RawQuery != "" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("known-bytes"))
				return
			}
			if tc.status == http.StatusTemporaryRedirect {
				w.Header().Set("Location", "https://example.invalid/not-the-owned-canary")
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		got := matchedSignedReadAndAnonymousDenied(context.Background(), server.URL+"/canary?X-Amz-Signature=synthetic",
			server.URL+"/canary", []byte("known-bytes"), server.Client().Transport)
		server.Close()
		if got != tc.allowed {
			t.Fatalf("matched S3 controls status %d: accepted=%v want=%v", tc.status, got, tc.allowed)
		}
	}
	if matchedSignedReadAndAnonymousDenied(context.Background(), "http://example.invalid/probe?X-Amz-Signature=bad", "http://example.invalid/probe", []byte("x"), nil) ||
		matchedSignedReadAndAnonymousDenied(context.Background(), "https://one.example.invalid/probe?X-Amz-Signature=bad", "https://two.example.invalid/probe", []byte("x"), nil) ||
		matchedSignedReadAndAnonymousDenied(context.Background(), "https://example.invalid/probe?X-Amz-Signature=bad", "https://example.invalid/other", []byte("x"), nil) {
		t.Fatal("insecure or mismatched signed/anonymous control accepted")
	}
	t.Setenv("SFD_ENV", "staging")
	for _, tc := range []struct {
		endpoint string
		secure   bool
		lookup   minio.BucketLookupType
		bucket   string
		ok       bool
	}{
		{"t3.storageapi.dev", true, minio.BucketLookupDNS, "private-staging-bucket", true},
		{"t3.storageapi.dev", false, minio.BucketLookupDNS, "private-staging-bucket", false},
		{"t3.storageapi.dev", true, minio.BucketLookupPath, "private-staging-bucket", false},
		{"evil.storageapi.dev", true, minio.BucketLookupDNS, "private-staging-bucket", false},
		{"t3.storageapi.dev:443", true, minio.BucketLookupDNS, "private-staging-bucket", false},
		{"t3.storageapi.dev", true, minio.BucketLookupDNS, "bucket/name", false},
	} {
		if (validateRailwayProbeTarget(tc.endpoint, tc.secure, tc.lookup, tc.bucket) == nil) != tc.ok {
			t.Fatalf("Railway provider target policy incorrect for endpoint %q", tc.endpoint)
		}
	}
	t.Setenv("SFD_ENV", "production")
	if validateRailwayProbeTarget("t3.storageapi.dev", true, minio.BucketLookupDNS, "private-staging-bucket") == nil {
		t.Fatal("Railway-only probe must never enable in production")
	}
	t.Setenv("SFD_ENV", "")
	if validateRailwayProbeTarget("t3.storageapi.dev", true, minio.BucketLookupDNS, "private-staging-bucket") == nil {
		t.Fatal("Railway-only probe requires an explicit staging environment")
	}
}
