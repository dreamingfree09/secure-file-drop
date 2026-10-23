// Package e2e runs the owned PostgreSQL/storage/native-hash acceptance harness.
package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestUploadHashDownloadFlow(t *testing.T) {
	keys := []string{"SFD_E2E_BACKEND_IMAGE", "SFD_E2E_POSTGRES_IMAGE", "SFD_E2E_STORAGE_IMAGE", "SFD_E2E_OUTPUT_ROOT", "SFD_E2E_PROXY_IMAGE"}
	configured := 0
	for _, key := range keys {
		if os.Getenv(key) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("requires explicitly owned local images and output root; compilation is not acceptance")
	}
	if configured != len(keys) {
		t.Fatal("incomplete isolated E2E configuration")
	}
	for _, layout := range []string{"bigint", "bytea"} {
		t.Run(layout, func(t *testing.T) {
			t.Setenv("SFD_E2E_HASH_LAYOUT", layout)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "python3", "-B", "../auth/e2e.py")
			// The harness writes only sanitized test evidence to stdout. Never forward raw
			// subprocess stderr, backend logs, environment values or signed-link contents.
			output, err := cmd.Output()
			if err != nil {
				t.Fatal("owned E2E harness failed; inspect its sanitized result.json")
			}
			t.Log(string(output))
		})
	}
}

// Configuration rendering uses a new synthetic env file and never starts
// Compose services or reads the Owner's existing installation configuration.
func TestDeploymentTemplate(t *testing.T) {
	if os.Getenv("SFD_E2E_BACKEND_IMAGE") == "" {
		t.Skip("requires explicitly enabled owned acceptance; ordinary compilation is not configuration acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "python3", "-B", "../acceptance/compose-template.py").Output()
	if err != nil {
		t.Fatal("synthetic deployment template contract failed; no private output disclosed")
	}
	var evidence struct {
		Count int `json:"count"`
	}
	if json.Unmarshal(output, &evidence) != nil || evidence.Count != 21 {
		t.Fatal("incomplete synthetic configuration contract")
	}
	root := os.Getenv("SFD_E2E_OUTPUT_ROOT")
	if root == "" {
		t.Fatal("explicit owned output root required")
	}
	outputRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal("open explicit owned output root")
	}
	defer func() { _ = outputRoot.Close() }()
	file, err := outputRoot.OpenFile("sfd-compose-contract.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("preserve existing configuration evidence; exclusive creation failed")
	}
	if _, err = file.Write(output); err != nil {
		_ = file.Close()
		t.Fatal("write sanitized configuration evidence")
	}
	if err = file.Close(); err != nil {
		t.Fatal("close configuration evidence")
	}
	t.Log(string(output))
}
