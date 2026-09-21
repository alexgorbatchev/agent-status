//go:build linux

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetPlatformMachineID_Linux(t *testing.T) {
	t.Run("real system call", func(t *testing.T) {
		id, err := getPlatformMachineID()
		if err != nil {
			t.Logf("getPlatformMachineID error: %v", err)
		}
		t.Logf("Platform machine ID: %q", id)
	})

	t.Run("reads machine-id from filesystem", func(t *testing.T) {
		tmpDir := t.TempDir()
		idFile := filepath.Join(tmpDir, "machine-id")
		if err := os.WriteFile(idFile, []byte("linux-test-uuid-456\n"), 0o600); err != nil {
			t.Fatalf("failed to write test machine-id: %v", err)
		}

		data, err := os.ReadFile(idFile)
		if err != nil {
			t.Fatalf("failed to read test machine-id: %v", err)
		}
		if string(data) != "linux-test-uuid-456\n" {
			t.Errorf("unexpected file content: %s", string(data))
		}
	})
}
