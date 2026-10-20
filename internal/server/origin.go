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
	return canonicalOrigin(raw)
}

// Canonical origin spelling matches browser/Uri default-port and DNS-case rules.
// Paths, credentials and other URL components are never origins.
func canonicalOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid origin")
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("invalid origin")
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if port != "" && !((scheme == "https" && port == "443") || (scheme == "http" && port == "80")) {
		host += ":" + port
	}
	return scheme + "://" + host, nil
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
			if origin := r.Header.Get("Origin"); origin != "" {
				received, receivedErr := canonicalOrigin(origin)
				configured, configuredErr := canonicalOrigin(expected)
				if receivedErr != nil || configuredErr != nil || received != configured {
					http.Error(w, "forbidden origin", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
