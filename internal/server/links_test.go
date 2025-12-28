package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClampTTLSeconds(t *testing.T) {
	cases := []struct{ in, out int }{{-1, 300}, {0, 300}, {60, 60}, {90000, 86400}}
	for _, c := range cases {
		if got := clampTTLSeconds(c.in); got != c.out {
			t.Fatalf("clampTTLSeconds(%d) = %d, want %d", c.in, got, c.out)
		}
	}
}

func TestRequestOriginHeaderPreference(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.local/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "files.example.com")

	got := requestOrigin(r)
	if !strings.HasPrefix(got, "https://") || !strings.Contains(got, "files.example.com") {
		t.Fatalf("unexpected origin: %s", got)
	}
}

func TestRequestOriginFallbacks(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.local/", nil)
	// no headers
	got := requestOrigin(r)
	if !strings.HasPrefix(got, "http://") {
		t.Fatalf("unexpected origin fallback: %s", got)
	}
}
