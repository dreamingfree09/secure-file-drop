package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
