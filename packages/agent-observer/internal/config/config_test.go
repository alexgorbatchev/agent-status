package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLabel(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "telemetry.json")

	tests := []struct {
		name       string
		flag       string
		envVar     string
		fileJSON   string
		wantPrefix string
	}{
		{
			name:       "flag has highest priority",
			flag:       "from-flag",
			envVar:     "from-env",
			fileJSON:   `{"environment": "from-file"}`,
			wantPrefix: "from-flag",
		},
		{
			name:       "env var second priority",
			flag:       "",
			envVar:     "from-env",
			fileJSON:   `{"environment": "from-file"}`,
			wantPrefix: "from-env",
		},
		{
			name:       "config file third priority with environment field",
			flag:       "",
			envVar:     "",
			fileJSON:   `{"environment": "from-file"}`,
			wantPrefix: "from-file",
		},
		{
			name:       "config file third priority with label field",
			flag:       "",
			envVar:     "",
			fileJSON:   `{"label": "from-file-label"}`,
			wantPrefix: "from-file-label",
		},
		{
			name:       "fallback to hostname when empty",
			flag:       "",
			envVar:     "",
			fileJSON:   "",
			wantPrefix: "", // empty means match hostname
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fileJSON != "" {
				if err := os.WriteFile(configFile, []byte(tt.fileJSON), 0600); err != nil {
					t.Fatalf("failed to write test config file: %v", err)
				}
			} else {
				_ = os.Remove(configFile)
			}

			t.Setenv("AI_ENV", tt.envVar)

			label := resolveLabel(tt.flag, configFile)
			if tt.wantPrefix != "" {
				if label != tt.wantPrefix {
					t.Errorf("resolveLabel() = %q, want %q", label, tt.wantPrefix)
				}
			} else {
				hostname, _ := os.Hostname()
				if label != hostname {
					t.Errorf("resolveLabel() = %q, want hostname %q", label, hostname)
				}
			}
		})
	}
}

func TestResolveEnvironment(t *testing.T) {
	env, err := ResolveEnvironment("test-label")
	if err != nil {
		t.Fatalf("ResolveEnvironment() unexpected error = %v", err)
	}

	if env.Label != "test-label" {
		t.Errorf("env.Label = %q, want %q", env.Label, "test-label")
	}

	if len(env.ID) != 16 {
		t.Errorf("len(env.ID) = %d, want 16 (hex sha256 prefix)", len(env.ID))
	}

	if env.Platform != "darwin" && env.Platform != "linux" {
		t.Errorf("env.Platform = %q, want darwin or linux", env.Platform)
	}

	if env.User == "" {
		t.Errorf("env.User is empty")
	}

	if env.Hostname == "" {
		t.Errorf("env.Hostname is empty")
	}
}
