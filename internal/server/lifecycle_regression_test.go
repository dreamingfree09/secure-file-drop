package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLifecycleCrossOriginMutation(t *testing.T) {
	t.Setenv("SFD_PUBLIC_BASE_URL", "https://files.test")
	handler := sameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for _, origin := range []string{"https://attacker.test", "null", "https://files.test", ""} {
		r := httptest.NewRequest("POST", "http://internal/files", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := 403
		if origin == "" || origin == "https://files.test" {
			want = 204
		}
		if w.Code != want {
			t.Fatal("origin boundary", w.Code)
		}
	}
}

func TestLifecycleDownloadExpiryBoundary(t *testing.T) {
	t.Setenv("SFD_DOWNLOAD_SECRET", "synthetic-expiry-boundary-secret")
	expiry := time.Unix(2000000000, 0).UTC()
	token, err := signDownloadToken("fixture-file", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyDownloadToken(token, expiry.Add(-time.Second)); err != nil {
		t.Fatal("unexpired fixture rejected")
	}
	for _, at := range []time.Time{expiry, expiry.Add(time.Second)} {
		if _, err := verifyDownloadToken(token, at); !errors.Is(err, errTokenExpired) {
			t.Fatal("expired fixture accepted")
		}
	}
}

func TestLifecycleOversizedDownloadTokenRejectedBeforeStorage(t *testing.T) {
	request := httptest.NewRequest("GET", "/download?token="+strings.Repeat("x", 4097), nil)
	response := httptest.NewRecorder()
	Config{}.downloadHandler(nil, nil, "").ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatal("unbounded token reached metadata/storage")
	}
}

func TestCanonicalOriginSpellings(t *testing.T) {
	for _, tc := range []struct {
		configured, received string
		want                 int
	}{
		{"https://DROP.example:443", "https://drop.example", 204},
		{"https://drop.example", "https://DROP.example:443", 204},
		{"http://drop.example:80", "http://drop.example", 204},
		{"https://[::1]:443", "https://[::1]", 204},
		{"https://drop.example:8443", "https://drop.example", 403},
		{"https://drop.example", "https://drop.example.attacker.invalid", 403},
		{"https://drop.example", "https://user@drop.example", 403},
		{"https://drop.example", "https://drop.example/path", 403},
	} {
		t.Run(tc.configured+"_"+tc.received, func(t *testing.T) {
			t.Setenv("SFD_PUBLIC_BASE_URL", tc.configured)
			h := sameOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
			r := httptest.NewRequest("POST", "https://internal/files", nil)
			r.Header.Set("Origin", tc.received)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("origin HTTP %d want %d", w.Code, tc.want)
			}
		})
	}
}
