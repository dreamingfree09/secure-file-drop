package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This reports vulnerability observations; findings are not converted into a
// safety pass. It scans only binaries/packages in the parent-owned images.
func assessRuntimeDependencies(ctx context.Context, t *testing.T, app, storage, run string, docker func(...string) string) {
	t.Helper()
	type secPackage struct {
		Pkg struct {
			Name     string
			Secfixes map[string][]string
		}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	all := make([]secPackage, 0, 1024)
	for _, repo := range []string{"main", "community"} {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://secdb.alpinelinux.org/v3.24/"+repo+".json", nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			_ = res.Body.Close()
			t.Fatalf("Alpine secdb HTTP %d", res.StatusCode)
		}
		var db struct{ Packages []secPackage }
		err = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&db)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, db.Packages...)
	}
	for _, id := range []string{app, storage} {
		if docker("inspect", "--format", "{{index .Config.Labels \"sfd.quality-run\"}}", id) != run {
			t.Fatal("scanner resource ownership mismatch")
		}
		raw := docker("exec", id, "cat", "/lib/apk/db/installed")
		found := 0
		packages := 0
		for _, record := range strings.Split(raw, "\n\n") {
			var name, version, origin string
			for _, line := range strings.Split(record, "\n") {
				if strings.HasPrefix(line, "P:") {
					name = line[2:]
				}
				if strings.HasPrefix(line, "V:") {
					version = line[2:]
				}
				if strings.HasPrefix(line, "o:") {
					origin = line[2:]
				}
			}
			if name == "" {
				continue
			}
			packages++
			if origin == "" {
				origin = name
			}
			for _, p := range all {
				if p.Pkg.Name != origin {
					continue
				}
				for fixed, advisories := range p.Pkg.Secfixes {
					if fixed == "0" {
						continue
					}
					if docker("exec", id, "apk", "version", "-t", version, fixed) == "<" {
						found += len(advisories)
						t.Logf("ALPINE ADVISORY package=%s origin=%s installed=%s fixed=%s ids=%s", name, origin, version, fixed, strings.Join(advisories, ","))
					}
				}
			}
		}
		t.Logf("Alpine v3.24 main/community comparison: image=%s packages=%d advisory-matches=%d; version applicability only, not exploit reachability", docker("inspect", "--format", "{{.Image}}", id), packages, found)
	}
	binary := filepath.Join(t.TempDir(), "storage-server")
	docker("cp", storage+":/usr/local/bin/minio", binary)
	scanCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(scanCtx, "go", "run", "golang.org/x/vuln/cmd/govulncheck@v1.9.0", "-mode=binary", "storage-server")
	cmd.Dir = filepath.Dir(binary)
	output, err := cmd.CombinedOutput()
	text := strings.ReplaceAll(string(output), binary, "[owned temporary storage binary]")
	if len(text) > 24000 {
		text = text[:24000] + "\n[output truncated; rerun binary scan for complete traces]"
	}
	if err != nil {
		// govulncheck returns a nonzero vulnerability status; tool/setup failures
		// remain failures instead of being mistaken for an advisory observation.
		if !strings.Contains(text, "Vulnerability #") {
			t.Fatalf("storage binary scan unavailable: %v\n%s", err, text)
		}
		t.Logf("STORAGE BINARY VULNERABILITIES OBSERVED (not a security pass):\n%s", text)
	} else {
		t.Logf("Storage binary govulncheck observation:\n%s", text)
	}
}
