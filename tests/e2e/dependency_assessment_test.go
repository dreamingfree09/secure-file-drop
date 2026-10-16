package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// This reports vulnerability observations; findings are not converted into a
// safety pass. It scans only binaries/packages in the parent-owned images.
func assessRuntimeDependencies(ctx context.Context, t *testing.T, app, storage, run, storageBinary string, docker func(...string) string, own func(string, string)) {
	t.Helper()
	type secPackage struct {
		Pkg struct {
			Name     string
			Secfixes map[string][]string
		}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	for _, id := range []string{app, storage} {
		if docker("inspect", "--format", "{{index .Config.Labels \"sfd.quality-run\"}}", id) != run {
			t.Fatal("scanner resource ownership mismatch")
		}
		release := strings.TrimSpace(docker("exec", id, "cat", "/etc/alpine-release"))
		parts := strings.Split(release, ".")
		if len(parts) < 2 || len(parts[0]) != 1 || len(parts[1]) != 2 {
			t.Fatal("unsupported runtime distribution", release)
		}
		branch := parts[0] + "." + parts[1]
		all := make([]secPackage, 0, 1024)
		for _, repo := range []string{"main", "community"} {
			req, err := http.NewRequestWithContext(ctx, "GET", "https://secdb.alpinelinux.org/v"+branch+"/"+repo+".json", nil)
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
		t.Logf("Alpine %s main/community comparison: image=%s packages=%d advisory-matches=%d; version applicability only, not exploit reachability", release, docker("inspect", "--format", "{{.Image}}", id), packages, found)
	}

	for _, target := range []struct{ name, container, path string }{
		{"Storage", storage, storageBinary}, {"Backend", app, "/app/backend"},
	} {
		if target.path == "/usr/bin/rustfs" {
			t.Logf("RustFS actual binary SHA-256: %s", docker("exec", storage, "sha256sum", "/usr/bin/rustfs"))
			assessRustSBOM(ctx, t, run, docker, own)
			t.Log("UNSUPPORTED: govulncheck does not analyse Rust binary symbols; upstream release SBOM is package-level evidence only")
			continue
		}
		binary := filepath.Join(t.TempDir(), "scan-binary")
		docker("cp", target.container+":"+target.path, binary)
		scanCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(scanCtx, "go", "run", "golang.org/x/vuln/cmd/govulncheck@v1.9.0", "-json", "-mode=binary", "scan-binary")
		cmd.Dir = filepath.Dir(binary)
		output, scanErr := cmd.Output()
		if scanCtx.Err() != nil {
			t.Fatal("storage binary scan timed out")
		}
		if scanErr != nil {
			var exit *exec.ExitError
			if !errors.As(scanErr, &exit) || !strings.Contains(string(exit.Stderr), "exit status 3") {
				t.Fatalf("storage binary scanner failed: %v", scanErr)
			}
		}
		type frame struct{ Module, Version, Package, Function string }
		type finding struct {
			OSV          string
			FixedVersion string `json:"fixed_version"`
			Trace        []frame
		}
		decoder := json.NewDecoder(bytes.NewReader(output))
		observed := map[string]string{}
		levels := map[string]int{}
		for {
			var msg struct{ Finding *finding }
			err := decoder.Decode(&msg)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal("invalid scanner JSON")
			}
			if msg.Finding == nil {
				continue
			}
			f := msg.Finding
			if len(f.Trace) > 0 {
				v := f.Trace[0]
				level := 1
				if v.Package != "" {
					level = 2
				}
				if v.Function != "" {
					level = 3
				}
				if level >= levels[f.OSV] {
					levels[f.OSV] = level
					observed[f.OSV] = fmt.Sprintf("level=%d module=%s installed=%s package=%s symbol=%s fixed=%s", level, v.Module, v.Version, v.Package, v.Function, f.FixedVersion)
				}
			}
		}
		keys := make([]string, 0, len(observed))
		for k := range observed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			t.Logf("%s BINARY ADVISORY %s %s https://pkg.go.dev/vuln/%s", target.name, k, observed[k], k)
		}
		counts := [4]int{}
		for _, level := range levels {
			counts[level]++
		}
		t.Logf("%s binary scan complete: %d advisory IDs; symbol=%d package-only=%d module-only=%d; symbol presence is not application exploit reachability", target.name, len(keys), counts[3], counts[2], counts[1])
	}
}
