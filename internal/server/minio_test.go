package server

import "testing"

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
