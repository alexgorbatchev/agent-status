package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDBPath(t *testing.T) {
	t.Run("XDG_DATA_HOME unset", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("os.UserHomeDir() failed: %v", err)
		}
		expected := filepath.Join(home, ".local", "share", "agent-status", "agent-status.db")
		if path := DefaultDBPath(); path != expected {
			t.Errorf("DefaultDBPath() = %q, want %q", path, expected)
		}
	})

	t.Run("XDG_DATA_HOME set", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/custom/data")
		expected := filepath.Join("/custom/data", "agent-status", "agent-status.db")
		if path := DefaultDBPath(); path != expected {
			t.Errorf("DefaultDBPath() with XDG_DATA_HOME = %q, want %q", path, expected)
		}
	})
}

func TestDefaultSocketPath(t *testing.T) {
	t.Run("XDG_RUNTIME_DIR unset", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "")
		expected := filepath.Join("/tmp", "agent-status", "notify.sock")
		if path := DefaultSocketPath(); path != expected {
			t.Errorf("DefaultSocketPath() = %q, want %q", path, expected)
		}
	})

	t.Run("XDG_RUNTIME_DIR set", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/custom/runtime")
		expected := filepath.Join("/custom/runtime", "agent-status", "notify.sock")
		if path := DefaultSocketPath(); path != expected {
			t.Errorf("DefaultSocketPath() with XDG_RUNTIME_DIR = %q, want %q", path, expected)
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	t.Run("with token set", func(t *testing.T) {
		t.Setenv("AGENT_STATUS_TOKEN", "test-token")
		t.Setenv("XDG_DATA_HOME", "/custom/data")
		t.Setenv("XDG_RUNTIME_DIR", "/custom/runtime")

		cfg := DefaultConfig()
		if cfg.Addr != ":4502" {
			t.Errorf("cfg.Addr = %q, want ':4502'", cfg.Addr)
		}
		if cfg.AuthToken != "test-token" {
			t.Errorf("cfg.AuthToken = %q, want 'test-token'", cfg.AuthToken)
		}
		if cfg.DBPath != filepath.Join("/custom/data", "agent-status", "agent-status.db") {
			t.Errorf("cfg.DBPath = %q, want '/custom/data/agent-status/agent-status.db'", cfg.DBPath)
		}
		if cfg.SocketPath != filepath.Join("/custom/runtime", "agent-status", "notify.sock") {
			t.Errorf("cfg.SocketPath = %q, want '/custom/runtime/agent-status/notify.sock'", cfg.SocketPath)
		}
		if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "http://localhost:3000" {
			t.Errorf("cfg.CORSOrigins = %v, want ['http://localhost:3000']", cfg.CORSOrigins)
		}
		if cfg.LogLevel != "info" {
			t.Errorf("cfg.LogLevel = %q, want 'info'", cfg.LogLevel)
		}
	})

	t.Run("with token unset", func(t *testing.T) {
		t.Setenv("AGENT_STATUS_TOKEN", "")

		cfg := DefaultConfig()
		if cfg.AuthToken != "" {
			t.Errorf("cfg.AuthToken = %q, want empty string", cfg.AuthToken)
		}
	})
}
