//go:build darwin

package config

import (
	"os/exec"
	"testing"
)

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

func TestGetPlatformMachineID_Darwin(t *testing.T) {
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
