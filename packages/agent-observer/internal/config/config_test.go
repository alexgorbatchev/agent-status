package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetDefaultConfigPath(t *testing.T) {
	t.Run("with XDG_CONFIG_HOME set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/xdg/config")
		got := getDefaultConfigPath()
		want := filepath.Join("/custom/xdg/config", "ai-registry", "telemetry.json")
		if got != want {
			t.Errorf("getDefaultConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("with XDG_CONFIG_HOME unset", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot get user home dir")
		}
		got := getDefaultConfigPath()
		want := filepath.Join(home, ".config", "ai-registry", "telemetry.json")
		if got != want {
			t.Errorf("getDefaultConfigPath() = %q, want %q", got, want)
		}
	})
}

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
			fileJSON:   `{"environment": "from-file-env"}`,
			wantPrefix: "from-file-env",
		},
		{
			name:       "config file third priority with env field",
			flag:       "",
			envVar:     "",
			fileJSON:   `{"env": "from-file-short-env"}`,
			wantPrefix: "from-file-short-env",
		},
		{
			name:       "config file third priority with label field",
			flag:       "",
			envVar:     "",
			fileJSON:   `{"label": "from-file-label"}`,
			wantPrefix: "from-file-label",
		},
		{
			name:       "invalid JSON in config file falls back to hostname",
			flag:       "",
			envVar:     "",
			fileJSON:   `{invalid-json}`,
			wantPrefix: "",
		},
		{
			name:       "empty config file falls back to hostname",
			flag:       "",
			envVar:     "",
			fileJSON:   "",
			wantPrefix: "",
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
				if hostname == "" {
					hostname = "unknown-host"
				}
				if label != hostname {
					t.Errorf("resolveLabel() = %q, want hostname %q", label, hostname)
				}
			}
		})
	}

	t.Run("resolveLabel with default config path", func(t *testing.T) {
		t.Setenv("AI_ENV", "")
		defaultConfigDir := filepath.Join(tmpDir, "ai-registry")
		_ = os.MkdirAll(defaultConfigDir, 0755)
		defaultCfgFile := filepath.Join(defaultConfigDir, "telemetry.json")
		_ = os.WriteFile(defaultCfgFile, []byte(`{"label": "xdg-label"}`), 0600)
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		label := resolveLabel("", "")
		if label != "xdg-label" {
			t.Errorf("resolveLabel(\"\", \"\") with XDG_CONFIG_HOME = %q, want 'xdg-label'", label)
		}
	})

	t.Run("resolveLabel hostname error fallback", func(t *testing.T) {
		origHostname := hostnameFunc
		defer func() { hostnameFunc = origHostname }()
		hostnameFunc = func() (string, error) {
			return "", errors.New("hostname error")
		}
		t.Setenv("AI_ENV", "")
		label := resolveLabel("", "/nonexistent-path")
		if label != "unknown-host" {
			t.Errorf("resolveLabel fallback = %q, want 'unknown-host'", label)
		}
	})
}

func TestParseIOPlatformUUID(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name: "valid ioreg output",
			input: []byte(`+-o Root  <class IORegistryEntry, id 0x100000100, retain 10>
  +-o IOPlatformExpertDevice  <class IOPlatformExpertDevice, id 0x100000110, registered, matched, active, busy 0 (0 ms), retain 33>
      {
        "IOPlatformUUID" = "12345678-ABCD-EF01-2345-6789ABCDEF01"
        "model" = <"MacBookPro18,1">
      }`),
			want: "12345678-ABCD-EF01-2345-6789ABCDEF01",
		},
		{
			name:  "no IOPlatformUUID",
			input: []byte(`"model" = <"MacBookPro18,1">`),
			want:  "",
		},
		{
			name:  "malformed line",
			input: []byte(`IOPlatformUUID`),
			want:  "",
		},
		{
			name:  "empty",
			input: []byte(""),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIOPlatformUUID(tt.input)
			if got != tt.want {
				t.Errorf("parseIOPlatformUUID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetPlatformMachineID(t *testing.T) {
	t.Run("real system call", func(t *testing.T) {
		id, err := getPlatformMachineID()
		if err != nil {
			t.Logf("getPlatformMachineID error: %v", err)
		}
		t.Logf("Platform machine ID: %q", id)
	})

	t.Run("sysctl empty fallback to ioreg success", func(t *testing.T) {
		origExec := execCommand
		defer func() { execCommand = origExec }()

		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "sysctl" {
				return exec.Command("echo", "")
			}
			if name == "ioreg" {
				return exec.Command("echo", `"IOPlatformUUID" = "MOCKED-IOREG-UUID-123"`)
			}
			return exec.Command(name, args...)
		}

		id, err := getPlatformMachineID()
		if err != nil {
			t.Fatalf("getPlatformMachineID error: %v", err)
		}
		if id != "MOCKED-IOREG-UUID-123" {
			t.Errorf("getPlatformMachineID() = %q, want 'MOCKED-IOREG-UUID-123'", id)
		}
	})

	t.Run("sysctl error and ioreg error", func(t *testing.T) {
		origExec := execCommand
		defer func() { execCommand = origExec }()

		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("nonexistent-command-for-test-error")
		}

		_, err := getPlatformMachineID()
		if err == nil {
			t.Errorf("expected error when both sysctl and ioreg fail")
		}
	})
}

func TestGetEnvironmentID(t *testing.T) {
	origMachineID := platformMachineIDFunc
	origUser := currentUserFunc
	origHostname := hostnameFunc
	defer func() {
		platformMachineIDFunc = origMachineID
		currentUserFunc = origUser
		hostnameFunc = origHostname
	}()

	t.Run("standard resolution", func(t *testing.T) {
		id, err := GetEnvironmentID()
		if err != nil {
			t.Fatalf("GetEnvironmentID() unexpected error = %v", err)
		}
		if len(id) != 16 {
			t.Errorf("len(id) = %d, want 16 (hex sha256 prefix)", len(id))
		}
	})

	t.Run("machine ID error falls back to hostname", func(t *testing.T) {
		platformMachineIDFunc = func() (string, error) {
			return "", errors.New("platform error")
		}
		hostnameFunc = func() (string, error) {
			return "my-test-host", nil
		}
		currentUserFunc = func() (*user.User, error) {
			return &user.User{Uid: "1001"}, nil
		}

		id, err := GetEnvironmentID()
		if err != nil {
			t.Fatalf("GetEnvironmentID() error: %v", err)
		}

		raw := "my-test-host:1001"
		h := sha256.Sum256([]byte(raw))
		want := hex.EncodeToString(h[:])[:16]
		if id != want {
			t.Errorf("GetEnvironmentID() = %q, want %q", id, want)
		}
	})

	t.Run("machine ID and hostname both error", func(t *testing.T) {
		platformMachineIDFunc = func() (string, error) {
			return "", errors.New("platform error")
		}
		hostnameFunc = func() (string, error) {
			return "", errors.New("hostname error")
		}
		currentUserFunc = func() (*user.User, error) {
			return nil, errors.New("user error")
		}
		t.Setenv("UID", "501")

		id, err := GetEnvironmentID()
		if err != nil {
			t.Fatalf("GetEnvironmentID() error: %v", err)
		}

		raw := "fallback-machine:501"
		h := sha256.Sum256([]byte(raw))
		want := hex.EncodeToString(h[:])[:16]
		if id != want {
			t.Errorf("GetEnvironmentID() = %q, want %q", id, want)
		}
	})

	t.Run("user error with no UID env fallback to 0", func(t *testing.T) {
		platformMachineIDFunc = func() (string, error) {
			return "test-mach", nil
		}
		currentUserFunc = func() (*user.User, error) {
			return nil, errors.New("user error")
		}
		t.Setenv("UID", "")

		id, err := GetEnvironmentID()
		if err != nil {
			t.Fatalf("GetEnvironmentID() error: %v", err)
		}

		raw := "test-mach:0"
		h := sha256.Sum256([]byte(raw))
		want := hex.EncodeToString(h[:])[:16]
		if id != want {
			t.Errorf("GetEnvironmentID() = %q, want %q", id, want)
		}
	})
}

func TestResolveEnvironment(t *testing.T) {
	origMachineID := platformMachineIDFunc
	origUser := currentUserFunc
	origHostname := hostnameFunc
	defer func() {
		platformMachineIDFunc = origMachineID
		currentUserFunc = origUser
		hostnameFunc = origHostname
	}()

	t.Run("with explicit envFlag", func(t *testing.T) {
		env, err := ResolveEnvironment("prod-cluster")
		if err != nil {
			t.Fatalf("ResolveEnvironment() unexpected error = %v", err)
		}

		if env.Label != "prod-cluster" {
			t.Errorf("env.Label = %q, want %q", env.Label, "prod-cluster")
		}
		if len(env.ID) != 16 {
			t.Errorf("len(env.ID) = %d, want 16", len(env.ID))
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
		if env.IsStale {
			t.Errorf("env.IsStale = true, want false")
		}
		if env.LastHeartbeatAt <= 0 {
			t.Errorf("env.LastHeartbeatAt = %d, want > 0", env.LastHeartbeatAt)
		}
	})

	t.Run("with AI_ENV env var fallback", func(t *testing.T) {
		t.Setenv("AI_ENV", "staging-env")
		env, err := ResolveEnvironment("")
		if err != nil {
			t.Fatalf("ResolveEnvironment(\"\") error = %v", err)
		}
		if env.Label != "staging-env" {
			t.Errorf("env.Label = %q, want 'staging-env'", env.Label)
		}
	})

	t.Run("user error with USER env var fallback", func(t *testing.T) {
		currentUserFunc = func() (*user.User, error) {
			return nil, errors.New("user error")
		}
		t.Setenv("USER", "test-user-override")
		env, err := ResolveEnvironment("label")
		if err != nil {
			t.Fatalf("ResolveEnvironment error: %v", err)
		}
		if env.User != "test-user-override" {
			t.Errorf("env.User = %q, want 'test-user-override'", env.User)
		}
	})

	t.Run("user error with no USER env var", func(t *testing.T) {
		currentUserFunc = func() (*user.User, error) {
			return nil, errors.New("user error")
		}
		t.Setenv("USER", "")
		env, err := ResolveEnvironment("label")
		if err != nil {
			t.Fatalf("ResolveEnvironment error: %v", err)
		}
		if env.User != "unknown" {
			t.Errorf("env.User = %q, want 'unknown'", env.User)
		}
	})

	t.Run("hostname error fallback to unknown", func(t *testing.T) {
		hostnameFunc = func() (string, error) {
			return "", errors.New("hostname error")
		}
		env, err := ResolveEnvironment("label")
		if err != nil {
			t.Fatalf("ResolveEnvironment error: %v", err)
		}
		if env.Hostname != "unknown" {
			t.Errorf("env.Hostname = %q, want 'unknown'", env.Hostname)
		}
	})

	t.Run("platform fallback validation", func(t *testing.T) {
		expectedPlatform := runtime.GOOS
		if expectedPlatform != "darwin" && expectedPlatform != "linux" {
			expectedPlatform = "linux"
		}
		env, err := ResolveEnvironment("test")
		if err != nil {
			t.Fatal(err)
		}
		if env.Platform != expectedPlatform {
			t.Errorf("env.Platform = %q, want %q", env.Platform, expectedPlatform)
		}
	})
}
