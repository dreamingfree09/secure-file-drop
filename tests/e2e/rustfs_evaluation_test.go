package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func rustStorage(password string) storageFixture {
	return storageFixture{candidate: "rustfs", dockerfile: "tests/storage/Dockerfile.rustfs", binary: "/usr/bin/rustfs", appAccess: "sfdapp",
		args: []string{"--cap-drop=ALL", "--security-opt", "no-new-privileges", "-e", "RUSTFS_ACCESS_KEY=sfdtest", "-e", "RUSTFS_SECRET_KEY=" + password,
			"-e", "RUSTFS_ADDRESS=0.0.0.0:9000", "-e", "RUSTFS_CONSOLE_ENABLE=false", "-e", "RUSTFS_OBS_LOG_DIRECTORY="},
		command: []string{"/data"}}
}
func setupRustPrincipals(ctx context.Context, t *testing.T, addr, bucket, run string) {
	t.Helper()
	// Isolated tool module: signed admin operations only, to parent-owned S3,
	// without adding administration dependencies to the application module.
	// #nosec G204 G702 -- Fixed test module, no shell; parent-owned loopback target also checked inside the tool.
	cmd := exec.CommandContext(ctx, "go", "run", ".", addr, bucket, run)
	cmd.Dir = "../storage/iam"
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("synthetic RustFS IAM setup: %v %s", err, out)
	}
	t.Log("PASS: normal authenticated admin API created scoped synthetic RustFS principals")
}

func assessRustSBOM(ctx context.Context, t *testing.T, run string, docker func(...string) string, own func(string, string)) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://github.com/rustfs/rustfs/releases/download/1.0.2/rustfs-1.0.2.sbom.cdx.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	_ = res.Body.Close()
	if err != nil || res.StatusCode != 200 {
		t.Fatal("official SBOM download failed", err)
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != "a9de85288b5dd639ce19ff4432c4ee6c780cdbf1bd31c1a11d866e6693a97a49" {
		t.Fatal("official release SBOM digest mismatch")
	}
	file := filepath.Join(t.TempDir(), "rustfs-release-sbom.json")
	// #nosec G306 -- Public upstream SBOM; isolated scanner container needs read access.
	if err := os.WriteFile(file, body, 0644); err != nil {
		t.Fatal(err)
	}
	id := docker("create", "--name", run+"-sbom-scanner", "--label", "sfd.quality-run="+run, "--network", run+"-net",
		"--cap-drop=ALL", "--security-opt", "no-new-privileges", "--mount", "type=bind,src="+file+",dst=/scan/sbom.json,readonly",
		"aquasec/trivy@sha256:166061a4a15c64a94a3aab430659e2c166dc0815fbc54a222c6d56d64132f552", "sbom", "--timeout", "3m", "--format", "json", "--output", "/tmp/result.json", "/scan/sbom.json")
	own("container", id)
	docker("start", "-a", id)
	if docker("inspect", "--format", "{{.State.ExitCode}}", id) != "0" {
		t.Fatal("RustFS release SBOM scanner failed")
	}
	result := filepath.Join(t.TempDir(), "result.json")
	docker("cp", id+":/tmp/result.json", result)
	// #nosec G304 -- Constant basename within t.TempDir; copied only from the owned scanner container.
	raw, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	type vulnerability struct{ VulnerabilityID, PkgName, InstalledVersion, FixedVersion, Severity string }
	var report struct {
		Results []struct{ Vulnerabilities []vulnerability }
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	findings := []string{}
	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			ids[v.VulnerabilityID] = true
			findings = append(findings, v.VulnerabilityID+" "+v.PkgName+" installed="+v.InstalledVersion+" fixed="+v.FixedVersion+" severity="+v.Severity)
		}
	}
	sort.Strings(findings)
	for _, finding := range findings {
		t.Log("RUST RELEASE SBOM ADVISORY", finding)
	}
	t.Logf("RustFS official release SBOM comparison complete: %d advisory IDs, %d package matches; crate inventory is not binary-symbol or request reachability evidence", len(ids), len(findings))
	if len(findings) == 0 {
		t.Log("No release-SBOM package advisories observed; not proof of absence")
	}
	if !strings.Contains(string(body), "rustfs") {
		t.Fatal("unexpected release SBOM identity")
	}
}
