package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Use a real HTTP connection: Recorder alone does not enforce Content-Length.
// The full-stack fixture separately exercises the actual recipient handler.
func TestBinaryDownloadCompression(t *testing.T) {
	payload := []byte{0, 0xff, 7}
	mux := http.NewServeMux()
	attachment := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		if _, err := w.Write(payload); err != nil {
			t.Errorf("fixture write: %v", err)
		}
	}
	mux.HandleFunc("/download", attachment)
	mux.HandleFunc("/receive-download", attachment)
	srv := httptest.NewServer(CompressionMiddleware(mux))
	t.Cleanup(srv.Close)
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	defer client.CloseIdleConnections()
	for _, route := range []struct{ method, path string }{{http.MethodGet, "/download"}, {http.MethodPost, "/receive-download"}} {
		for _, encoding := range []string{"", "gzip"} {
			req, err := http.NewRequest(route.method, srv.URL+route.path, strings.NewReader("id=synthetic&code=synthetic"))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept-Encoding", encoding)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(res.Body)
			if err = res.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if readErr != nil || res.StatusCode != http.StatusOK || res.ContentLength != int64(len(payload)) || res.Header.Get("Content-Encoding") != "" || !bytes.Equal(body, payload) {
				t.Fatalf("%s encoding=%q: status=%d length=%d encoding=%q bytes=%x read=%v", route.path, encoding, res.StatusCode, res.ContentLength, res.Header.Get("Content-Encoding"), body, readErr)
			}
		}
	}
}
