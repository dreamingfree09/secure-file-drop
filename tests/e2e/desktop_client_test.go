package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Runs the actual portable client source through the existing real Traefik,
// PostgreSQL, MinIO and native-hash fixture. Only this child process trusts its
// fresh fixture CA; no machine trust store or certificate callback is changed.
func verifyDesktopClient(ctx context.Context, t *testing.T, base, run, certificate string) {
	t.Helper()
	if os.Getenv("SFD_DESKTOP_CLIENT") != "1" {
		return
	}
	if !strings.HasPrefix(run, "sfd-quality-") || !strings.HasPrefix(base, "https://localhost:") {
		t.Fatal("unexpected desktop fixture ownership")
	}
	var untrustedCalls atomic.Int32
	untrusted := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		untrustedCalls.Add(1)
		_, _ = io.WriteString(w, "{}")
	}))
	defer untrusted.Close()
	// Keep the unavailable target port owned while returning no HTTP response.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	deadline, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dll := filepath.Join(root, "desktop", "windows", "tests", "bin", "Release", "net10.0", "Client.Contract.dll")
	if _, err := os.Stat(dll); err != nil {
		t.Fatal("desktop harness was not built", err)
	}
	// #nosec G204 G702 -- Fixed test executable and validated fresh loopback fixture arguments, no shell.
	cmd := exec.CommandContext(deadline, "dotnet", dll, base, run, untrusted.URL, "http://"+listener.Addr().String())
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+certificate, "SFD_DESKTOP_RUN="+run)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal("desktop real-backend acceptance", err)
	}
	if !strings.Contains(string(output), "DESKTOP REAL BACKEND CHECKS PASSED:") {
		t.Fatal("desktop client did not actually execute")
	}
	if untrustedCalls.Load() != 0 {
		t.Fatal("desktop client accepted the untrusted TLS fixture")
	}
	raw, err := os.ReadFile("../../desktop/windows/Client.cs")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	t.Logf("PASS: portable desktop client source sha256=%s; real authenticated TLS/native flow, progress, Unicode/empty files, account isolation, 401/retry and untrusted TLS rejection", hex.EncodeToString(digest[:]))
}
