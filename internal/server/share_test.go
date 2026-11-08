package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestPreviewSafeShareFormAndLegacyCompatibility(t *testing.T) {
	if shareLinkPath(false) != "/download" || shareLinkPath(true) != "/share" {
		t.Fatal("legacy raw and preview-safe paths must remain distinct")
	}
	page, err := os.ReadFile("../../web/static/share.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"og:image", "twitter:image", "<img", "/download?token=", "iframe", "file_name", "file_size"} {
		if strings.Contains(strings.ToLower(string(page)), forbidden) {
			t.Errorf("generic landing contains private/preview element %q", forbidden)
		}
	}
	if !strings.Contains(string(page), "action=\"/share-download\" method=\"post\"") {
		t.Error("explicit form POST must be present")
	}
	script, err := os.ReadFile("../../web/static/share.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(script), "fetch(") || strings.Contains(string(script), ".submit(") ||
		!strings.Contains(string(script), "form.hidden = false") {
		t.Error("share script must not fetch or automatically submit any token")
	}

	seen := 0
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		if r.Method != http.MethodGet || r.URL.Path != "/download" ||
			r.URL.Query().Get("token") != "signed.test" ||
			r.URL.Query().Get("password") != "secret pass" ||
			r.ContentLength != 0 || r.Body != http.NoBody || r.Header.Get("Content-Type") != "" {
			t.Error("internal conversion violates signed-download handler contract")
		}
		w.WriteHeader(http.StatusAccepted)
	})
	handler := shareDownloadFormHandler(downstream)
	values := url.Values{"token": {"signed.test"}, "password": {"secret pass"}}
	post := func(path, contentType, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", contentType)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, request)
		return rec
	}
	if got := post("/share-download", "application/x-www-form-urlencoded", values.Encode()); got.Code != http.StatusAccepted || seen != 1 {
		t.Fatalf("correct form not forwarded: status %d; calls %d", got.Code, seen)
	}
	invalid := []struct {
		path, contentType, body string
		code                    int
	}{
		{"/share-download?token=evil", "application/x-www-form-urlencoded", values.Encode(), 400},
		{"/share-download", "application/json", "{\"token\":\"fake\"}", 415},
		{"/share-download", "application/x-www-form-urlencoded", "token=abc&token=def", 400},
		{"/share-download", "application/x-www-form-urlencoded", "token=abc&extra=evil", 400},
		{"/share-download", "application/x-www-form-urlencoded", "token=", 400},
		{"/share-download", "application/x-www-form-urlencoded", "token=" + strings.Repeat("x", 8193), 400},
	}
	for _, tc := range invalid {
		got := post(tc.path, tc.contentType, tc.body)
		if got.Code != tc.code {
			t.Errorf("invalid form returned %d, expected %d", got.Code, tc.code)
		}
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/share-download", nil))
	if get.Code != http.StatusMethodNotAllowed || seen != 1 {
		t.Errorf("form GET must not return file bytes: status %d; calls %d", get.Code, seen)
	}
	pagePost := httptest.NewRecorder()
	sharePageHandler(pagePost, httptest.NewRequest(http.MethodPost, "/share", nil))
	if pagePost.Code != http.StatusMethodNotAllowed {
		t.Errorf("landing POST must return 405, got %d", pagePost.Code)
	}
}
