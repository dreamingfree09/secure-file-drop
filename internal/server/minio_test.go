package server

import (
	"errors"
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
	// Never expose arbitrary provider errors or silently authorise an unknown
	// response. Rejection is mandatory even when the error is recognisable.
	for _, tc := range []struct {
		providerCode string
		category string
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
}
