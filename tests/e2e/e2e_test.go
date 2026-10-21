// Package e2e runs the owned PostgreSQL/storage/native-hash acceptance harness.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestUploadHashDownloadFlow(t *testing.T) {
	keys := []string{"SFD_E2E_BACKEND_IMAGE", "SFD_E2E_POSTGRES_IMAGE", "SFD_E2E_STORAGE_IMAGE", "SFD_E2E_OUTPUT_ROOT"}
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
	t.Log(string(output))
}
