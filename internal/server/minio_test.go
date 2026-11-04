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
	// This is a real TLS HTTP server, but no real provider bucket is used.
	for _, tc := range []struct {
		status int
		denied bool
	}{
		{http.StatusForbidden, true},
		{http.StatusNotFound, false},
		{http.StatusUnauthorized, false},
		{http.StatusOK, false},
		{http.StatusInternalServerError, false},
		{http.StatusTemporaryRedirect, false},
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if tc.status == http.StatusTemporaryRedirect {
				w.Header().Set("Location", "https://example.invalid/not-the-owned-canary")
			}
			w.WriteHeader(tc.status)
		}))
		got := anonymousObjectReadDenied(context.Background(), server.URL, server.Client().Transport)
		server.Close()
		if got != tc.denied {
			t.Fatalf("anonymous object GET HTTP %d: denial=%v wanted %v", tc.status, got, tc.denied)
		}
	}
	if anonymousObjectReadDenied(context.Background(), "http://example.invalid/probe", nil) ||
		anonymousObjectReadDenied(context.Background(), "https://user:pass@example.invalid/probe", nil) ||
		anonymousObjectReadDenied(context.Background(), "https://example.invalid/probe?X-Amz-Signature=bad", nil) {
		t.Fatal("insecure or signed anonymous probe URL accepted")
	}
	for _, tc := range []struct {
		endpoint string
		secure bool
		lookup minio.BucketLookupType
		bucket string
		ok bool
	}{
		{"t3.storageapi.dev", true, minio.BucketLookupDNS, "private-staging-bucket", true},
		{"t3.storageapi.dev", false, minio.BucketLookupDNS, "private-staging-bucket", false},
		{"t3.storageapi.dev", true, minio.BucketLookupPath, "private-staging-bucket", false},
		{"evil.storageapi.dev", true, minio.BucketLookupDNS, "private-staging-bucket", false},
		{"t3.storageapi.dev:443", true, minio.BucketLookupDNS, "private-staging-bucket", false},
		{"t3.storageapi.dev", true, minio.BucketLookupDNS, "bucket/name", false},
	}{
		if (validateRailwayProbeTarget(tc.endpoint, tc.secure, tc.lookup, tc.bucket) == nil) != tc.ok {
			t.Fatalf("Railway provider target policy incorrect for endpoint %q", tc.endpoint)
		}
	}
}
