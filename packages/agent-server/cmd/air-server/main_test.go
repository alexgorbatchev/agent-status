package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/config"
)

func createTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agent-status-test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite3 db: %v", err)
	}
	schema := `
	CREATE TABLE IF NOT EXISTS environments (
		id                TEXT PRIMARY KEY,
		label             TEXT NOT NULL,
		hostname          TEXT NOT NULL,
		platform          TEXT NOT NULL CHECK (platform IN ('darwin','linux')),
		user              TEXT NOT NULL,
		dropped_frames    INTEGER NOT NULL DEFAULT 0,
		last_heartbeat_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS runs (
		id              TEXT PRIMARY KEY,
		agent_id        TEXT NOT NULL,
		pid             INTEGER NOT NULL,
		harness_version TEXT NOT NULL,
		started_at      INTEGER NOT NULL,
		ended_at        INTEGER
	);

	CREATE TABLE IF NOT EXISTS agents (
		id                    TEXT PRIMARY KEY,
		session_id            TEXT NOT NULL,
		environment_id        TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
		harness               TEXT NOT NULL CHECK (harness IN ('claude-code','pi','codex','opencode')),
		project_id            TEXT NOT NULL,
		project_name          TEXT NOT NULL,
		canonical_root        TEXT NOT NULL,
		branch                TEXT NOT NULL,
		is_detached           INTEGER NOT NULL DEFAULT 0,
		is_worktree           INTEGER NOT NULL DEFAULT 0,
		worktree_rel_path     TEXT,
		cwd                   TEXT NOT NULL,
		current_run_id        TEXT REFERENCES runs(id) ON DELETE SET NULL,
		activity              TEXT NOT NULL CHECK (activity IN ('BUSY','WAITING_QUESTION','ERROR','RATE_LIMITED','IDLE')),
		title                 TEXT NOT NULL DEFAULT '',
		action                TEXT NOT NULL DEFAULT '',
		action_stale          INTEGER NOT NULL DEFAULT 0,
		phase                 TEXT CHECK (phase IS NULL OR phase IN ('planning','editing','testing','debugging','investigating')),
		active_tool           TEXT,
		active_target         TEXT,
		active_tool_started_at INTEGER,
		question              TEXT,
		last_thinking_excerpt TEXT,
		last_error            TEXT,
		tokens_input          INTEGER NOT NULL DEFAULT 0,
		tokens_output         INTEGER NOT NULL DEFAULT 0,
		tokens_cache_read     INTEGER NOT NULL DEFAULT 0,
		tokens_cache_write    INTEGER NOT NULL DEFAULT 0,
		tokens_reasoning      INTEGER NOT NULL DEFAULT 0,
		tokens_total          INTEGER NOT NULL DEFAULT 0,
		cost_usd              REAL,
		subagent_count        INTEGER NOT NULL DEFAULT 0,
		active_subagents      TEXT,
		first_started_at      INTEGER NOT NULL,
		updated_at            INTEGER NOT NULL,
		is_hidden             INTEGER NOT NULL DEFAULT 0,
		hidden_at             INTEGER,
		model                 TEXT,
		context_window        INTEGER,
		context_fullness      REAL,
		rate_limit_reset_at   INTEGER,
		retry_after_seconds   INTEGER,
		has_unseen_completion INTEGER NOT NULL DEFAULT 0,
		completed_at          INTEGER,
		UNIQUE (environment_id, harness, session_id)
	);

	CREATE TABLE IF NOT EXISTS telemetry_events (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		subagent_id   TEXT,
		event_type    TEXT NOT NULL,
		timestamp     INTEGER NOT NULL,
		data_json     TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS observer_cursors (
		environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
		last_seq       INTEGER NOT NULL,
		PRIMARY KEY (environment_id)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	_ = db.Close()
	return dbPath
}

func TestMain_Version(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
	})
	os.Args = []string{"air-server", "--version"}
	main()
}

func TestRun_LifecycleAndShutdown(t *testing.T) {
	dbPath := createTestDB(t)
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "server.log")

	cfg := config.Config{
		Addr:       "127.0.0.1:0",
		DBPath:     dbPath,
		SocketPath: filepath.Join(tmpDir, "notify.sock"),
		LogLevel:   "debug",
		LogDir:     tmpDir,
		LogFile:    logFile,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run() failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not shut down within timeout")
	}
}

func TestRun_WithoutLogFileOrDir(t *testing.T) {
	dbPath := createTestDB(t)
	tmpDir := t.TempDir()

	cfg := config.Config{
		Addr:       "127.0.0.1:0",
		DBPath:     dbPath,
		SocketPath: filepath.Join(tmpDir, "notify.sock"),
		LogLevel:   "info",
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run() failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not shut down within timeout")
	}
}

func TestRun_ServerError(t *testing.T) {
	dbPath := createTestDB(t)
	cfg := config.Config{
		Addr:     "999.999.999.999:9999",
		DBPath:   dbPath,
		LogLevel: "info",
	}

	ctx := context.Background()
	err := run(ctx, cfg)
	if err == nil {
		t.Fatal("expected error with invalid address, got nil")
	}
	if !strings.Contains(err.Error(), "http server error") {
		t.Errorf("expected 'http server error', got: %v", err)
	}
}

func TestRootCmd_Execute(t *testing.T) {
	dbPath := createTestDB(t)
	tmpDir := t.TempDir()

	cfg := config.Config{}
	cmd := newRootCmd(&cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetArgs([]string{
		"--addr", "127.0.0.1:0",
		"--db", dbPath,
		"--socket", filepath.Join(tmpDir, "notify.sock"),
		"--log-level", "debug",
		"--log-dir", tmpDir,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.ExecuteContext(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("cmd.ExecuteContext failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cmd.ExecuteContext timed out")
	}
}

func TestRootCmd_ExecuteError(t *testing.T) {
	cfg := config.Config{}
	cmd := newRootCmd(&cfg)
	cmd.SetArgs([]string{"--invalid-flag-that-does-not-exist"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error with invalid flag, got nil")
	}
}

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
