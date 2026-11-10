package server

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// GET /share serves only generic HTML; no file bytes, names or object metadata.
func sharePageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	// A no-referrer document makes browser HTML form POST send Origin: null.
	// strict-origin reveals only scheme/host/port (never path or bearer query)
	// and preserves the existing fail-closed Origin check on form submission.
	w.Header().Set("Referrer-Policy", "strict-origin")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	http.ServeFile(w, r, "/app/web/static/share.html")
}

// POST /share-download reuses the existing signed-download handler without an
// HTTP redirect. The bearer token remains in the form body on the network.
func shareDownloadFormHandler(download http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			http.Error(w, "invalid download form", http.StatusBadRequest)
			return
		}
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || kind != "application/x-www-form-urlencoded" {
			http.Error(w, "form required", http.StatusUnsupportedMediaType)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid download form", http.StatusBadRequest)
			return
		}
		if len(r.PostForm["token"]) != 1 || len(r.PostForm["password"]) > 1 || len(r.PostForm) > 2 {
			http.Error(w, "invalid download form", http.StatusBadRequest)
			return
		}
		for key := range r.PostForm {
			if key != "token" && key != "password" {
				http.Error(w, "invalid download form", http.StatusBadRequest)
				return
			}
		}
		token, password := r.PostForm.Get("token"), r.PostForm.Get("password")
		if len(token) == 0 || len(token) > 4096 || strings.ContainsAny(token, "\r\n\x00") || len(password) > 512 {
			http.Error(w, "invalid download form", http.StatusBadRequest)
			return
		}
		// Clone the request for internal dispatch only. No network-visible GET
		// request/redirect is constructed and downstream HMAC checks are intact.
		clone := r.Clone(r.Context())
		clone.Method = http.MethodGet
		uri := *r.URL
		uri.Path = "/download"
		uri.RawPath = ""
		q := url.Values{"token": {token}}
		if password != "" {
			q.Set("password", password)
		}
		uri.RawQuery = q.Encode()
		clone.URL = &uri
		clone.Body = http.NoBody
		clone.ContentLength = 0
		clone.TransferEncoding = nil
		clone.Header = r.Header.Clone()
		clone.Header.Del("Content-Type")
		clone.Header.Del("Content-Length")
		download.ServeHTTP(w, clone)
	})
}
