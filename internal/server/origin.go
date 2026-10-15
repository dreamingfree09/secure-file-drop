package server

import (
	"net/http"
	"os"
	"strings"
)

// Browsers send Origin on cross-origin state-changing requests. Non-browser
// clients may omit it; session cookies also use SameSite=Lax.
func sameOriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origin := r.Header.Get("Origin")
			expected := strings.TrimRight(os.Getenv("SFD_PUBLIC_BASE_URL"), "/")
			if expected == "" {
				expected = requestOrigin(r)
			}
			if origin != "" && origin != expected {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
