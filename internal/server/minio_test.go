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
