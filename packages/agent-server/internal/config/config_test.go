package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDBPath(t *testing.T) {
	os.Unsetenv("XDG_DATA_HOME")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "share", "agent-status", "agent-status.db")
	if path := DefaultDBPath(); path != expected {
		t.Errorf("DefaultDBPath() = %q, want %q", path, expected)
	}

	os.Setenv("XDG_DATA_HOME", "/custom/data")
	defer os.Unsetenv("XDG_DATA_HOME")
	if path := DefaultDBPath(); path != "/custom/data/agent-status/agent-status.db" {
		t.Errorf("DefaultDBPath() with XDG_DATA_HOME = %q, want /custom/data/agent-status/agent-status.db", path)
	}
}

func TestDefaultConfig(t *testing.T) {
	os.Setenv("AGENT_STATUS_TOKEN", "test-token")
	defer os.Unsetenv("AGENT_STATUS_TOKEN")

	cfg := DefaultConfig()
	if cfg.Addr != ":4502" {
		t.Errorf("cfg.Addr = %q, want ':4502'", cfg.Addr)
	}
	if cfg.AuthToken != "test-token" {
		t.Errorf("cfg.AuthToken = %q, want 'test-token'", cfg.AuthToken)
	}
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "http://localhost:3000" {
		t.Errorf("cfg.CORSOrigins = %v, want ['http://localhost:3000']", cfg.CORSOrigins)
	}
}
