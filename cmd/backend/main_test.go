package main

import (
	"os"
	"testing"
)

func TestGetenvDefault(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		def      string
		envValue string
		want     string
	}{
		{
			name:     "env var set",
			key:      "TEST_VAR_SET",
			def:      "default",
			envValue: "custom",
			want:     "custom",
		},
		{
			name:     "env var empty",
			key:      "TEST_VAR_EMPTY",
			def:      "default",
			envValue: "",
			want:     "default",
		},
		{
			name:     "env var not set",
			key:      "TEST_VAR_NOTSET",
			def:      "default",
			envValue: "",
			want:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: clear env var first
			os.Unsetenv(tt.key)

			// Set env var if test requires it
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getenvDefault(tt.key, tt.def)
			if got != tt.want {
				t.Errorf("getenvDefault(%q, %q) = %q, want %q", tt.key, tt.def, got, tt.want)
			}
		})
	}
}
