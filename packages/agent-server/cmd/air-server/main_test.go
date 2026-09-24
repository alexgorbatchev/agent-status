package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/config"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "air-server-test-logs-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test log dir: %v\n", err)
		os.Exit(1)
	}

	if err := os.Setenv("AIR_LOG_DIR", tmpDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set AIR_LOG_DIR: %v\n", err)
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestServerCommandHelp(t *testing.T) {
	cfg := config.DefaultConfig()
	cmd := newRootCmd(&cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("executing --help: %v", err)
	}

	out := buf.String()
	requiredHelpStrings := []string{
		"air-server",
		"--addr",
		"--db",
		"--socket",
		"--cors-origin",
		"--token",
		"--log-level",
		"--log-dir",
		"--log-file",
	}

	for _, s := range requiredHelpStrings {
		if !strings.Contains(out, s) {
			t.Errorf("expected help output to contain %q, but got:\n%s", s, out)
		}
	}
}

func TestServerCommandVersion(t *testing.T) {
	cfg := config.DefaultConfig()
	cmd := newRootCmd(&cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("executing --version: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, version) {
		t.Errorf("expected version output to contain %q, got %q", version, out)
	}
}

func TestServerCommandFlagParsing(t *testing.T) {
	cfg := config.DefaultConfig()
	cmd := newRootCmd(&cfg)

	args := []string{
		"-a", ":9999",
		"--db", "/custom/path/agent.db",
		"--socket", "/custom/path/notify.sock",
		"--token", "secret-server-token",
		"--log-level", "debug",
		"--log-dir", "/tmp/logs",
		"--log-file", "/tmp/logs/server.log",
	}

	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags failed: %v", err)
	}

	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999", cfg.Addr)
	}
	if cfg.DBPath != "/custom/path/agent.db" {
		t.Errorf("DBPath = %q, want /custom/path/agent.db", cfg.DBPath)
	}
	if cfg.SocketPath != "/custom/path/notify.sock" {
		t.Errorf("SocketPath = %q, want /custom/path/notify.sock", cfg.SocketPath)
	}
	if cfg.AuthToken != "secret-server-token" {
		t.Errorf("AuthToken = %q, want secret-server-token", cfg.AuthToken)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.LogDir != "/tmp/logs" {
		t.Errorf("LogDir = %q, want /tmp/logs", cfg.LogDir)
	}
	if cfg.LogFile != "/tmp/logs/server.log" {
		t.Errorf("LogFile = %q, want /tmp/logs/server.log", cfg.LogFile)
	}
}

func TestServerCommandCORSOriginsParsing(t *testing.T) {
	t.Run("repeated flags", func(t *testing.T) {
		cfg := config.Config{}
		cmd := newRootCmd(&cfg)

		args := []string{
			"--cors-origin", "http://localhost:3000",
			"--cors-origin", "http://localhost:4502",
			"--cors-origin", "https://dashboard.example.com",
		}

		if err := cmd.ParseFlags(args); err != nil {
			t.Fatalf("ParseFlags failed: %v", err)
		}

		if len(cfg.CORSOrigins) != 3 {
			t.Fatalf("expected 3 CORS origins, got %d: %v", len(cfg.CORSOrigins), cfg.CORSOrigins)
		}
		if cfg.CORSOrigins[0] != "http://localhost:3000" || cfg.CORSOrigins[1] != "http://localhost:4502" || cfg.CORSOrigins[2] != "https://dashboard.example.com" {
			t.Errorf("unexpected CORS origins: %v", cfg.CORSOrigins)
		}
	})

	t.Run("comma-separated flag value", func(t *testing.T) {
		cfg := config.Config{}
		cmd := newRootCmd(&cfg)

		args := []string{
			"--cors-origin", "http://localhost:3000,http://localhost:4502",
		}

		if err := cmd.ParseFlags(args); err != nil {
			t.Fatalf("ParseFlags failed: %v", err)
		}

		if len(cfg.CORSOrigins) != 2 {
			t.Fatalf("expected 2 CORS origins, got %d: %v", len(cfg.CORSOrigins), cfg.CORSOrigins)
		}
		if cfg.CORSOrigins[0] != "http://localhost:3000" || cfg.CORSOrigins[1] != "http://localhost:4502" {
			t.Errorf("unexpected CORS origins: %v", cfg.CORSOrigins)
		}
	})
}
