package server

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestHashStreamRequiresCompleteInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   io.Reader
		command string
		count   string
		wantErr bool
	}{
		{"complete", strings.NewReader("abc"), "cat >/dev/null", "3", false},
		{"wrong native count", strings.NewReader("abc"), "cat >/dev/null", "2", true},
		{"premature child exit", strings.NewReader(strings.Repeat("x", 2<<20)), ":", "0", true},
		{"read failure", iotest.ErrReader(errors.New("fixture read failed")), "cat >/dev/null", "0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := filepath.Join(t.TempDir(), "fixture-hasher")
			script := "#!/bin/sh\n" + tc.command + "\nprintf '%s\\n' " +
				"'{\"algorithm\":\"sha256\",\"hash\":\"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\",\"bytes\":" + tc.count + "}'\n"
			if err := os.WriteFile(helper, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			// #nosec G302 -- Synthetic helper inside t.TempDir needs owner-only execute permission.
			if err := os.Chmod(helper, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SFD_HASH_TOOL", helper)
			result, err := hashStream(context.Background(), tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error = %v", err, tc.wantErr)
			}
			if !tc.wantErr && result.Bytes != 3 {
				t.Fatalf("byte count = %d, want 3", result.Bytes)
			}
		})
	}
}
