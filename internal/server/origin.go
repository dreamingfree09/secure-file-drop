package server

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func publicOrigin() (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(os.Getenv("SFD_PUBLIC_BASE_URL")), "/")
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("invalid public origin")
	}
	return u.Scheme + "://" + u.Host, nil
}

// Forwarded headers remain untrusted. Browsers must match the configured origin.
func sameOriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			expected, err := publicOrigin()
			if err != nil {
				http.Error(w, "server misconfigured", http.StatusInternalServerError)
				return
			}
			if expected == "" {
				expected = requestOrigin(r)
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != expected {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
