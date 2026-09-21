package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func createTestClaudeTranscript(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "test-session.jsonl")
	content := `{"type":"ai-title","aiTitle":"CLI Test Title","timestamp":"2026-09-21T10:00:00.000Z","cwd":"/repo"}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Run build"}]},"timestamp":"2026-09-21T10:00:01.000Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Build finished"}]},"usage":{"input_tokens":100,"output_tokens":20},"timestamp":"2026-09-21T10:00:02.000Z"}
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("writing test transcript: %v", err)
	}
	return p
}

func createTestOpencodeDB(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "test-opencode.db")
	db, err := sql.Open("sqlite3", "file:"+p)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
		CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE session (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			parent_id TEXT,
			slug TEXT,
			directory TEXT NOT NULL,
			title TEXT,
			version TEXT,
			agent TEXT,
			model TEXT,
			cost REAL,
			tokens_input INTEGER NOT NULL DEFAULT 0,
			tokens_output INTEGER NOT NULL DEFAULT 0,
			tokens_reasoning INTEGER NOT NULL DEFAULT 0,
			tokens_cache_read INTEGER NOT NULL DEFAULT 0,
			tokens_cache_write INTEGER NOT NULL DEFAULT 0,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			time_archived INTEGER,
			metadata TEXT
		);
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
		CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
	`
	_, _ = db.Exec(schema)
	now := time.Now().UnixMilli()
	_, _ = db.Exec(`
		INSERT INTO session (id, directory, title, version, cost, tokens_input, tokens_output, time_created, time_updated)
		VALUES ('ses_cli_1', '/test/repo', 'CLI DB Session', '1.0.0', 0.05, 500, 100, ?, ?)
	`, now-5000, now)
	return p
}

func TestRun(t *testing.T) {
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

func TestCLI_Version(t *testing.T) {
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "dev" {
		t.Errorf("expected 'dev', got %q", buf.String())
	}
}

func TestMainFunc(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"air-debug", "--version"}
	main()
}

func TestMain_ErrorExit(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
	}()
	exitCalled := false
	exitFunc = func(code int) {
		exitCalled = true
	}
	os.Args = []string{"air-debug", "unknown-subcommand"}
	main()
	if !exitCalled {
		t.Errorf("expected exitFunc to be called on error")
	}
}

func TestCLI_SessionList(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := createTestOpencodeDB(t, tmpDir)

	cmd := newRootCmd()
	cmd.SetContext(context.Background())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"session", "list", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("session list failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ses_cli_1") {
		t.Errorf("expected session ID in output, got: %s", out)
	}

	// Test invalid db file error path
	badDB := filepath.Join(tmpDir, "bad.db")
	_ = os.WriteFile(badDB, []byte("not a sqlite db"), 0644)
	cmdBad := newRootCmd()
	cmdBad.SetArgs([]string{"session", "list", badDB})
	if err := cmdBad.Execute(); err == nil {
		t.Errorf("expected error opening invalid db file")
	}
}

func TestCLI_SessionReplay(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := createTestClaudeTranscript(t, tmpDir)

	t.Run("human output", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetContext(context.Background())
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "replay", transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session replay failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Session Replay Summary") && !strings.Contains(out, "harness:claude-code") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("json output", func(t *testing.T) {
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "replay", "--json", transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session replay --json failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, `"harness": "claude-code"`) {
			t.Errorf("expected JSON output, got: %s", out)
		}
	})

	t.Run("export-db and until options", func(t *testing.T) {
		exportPath := filepath.Join(tmpDir, "exported.db")
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "replay", "--export-db", exportPath, "--until-line", "1", "--stop-on-error", "--harness", "claude-code", "--env", "custom-env", transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session replay with export-db failed: %v", err)
		}
	})

	t.Run("diff flag", func(t *testing.T) {
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "replay", "--diff", transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session replay --diff failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Step 1") && !strings.Contains(out, "diff step:1") {
			t.Errorf("expected diff steps in output, got: %s", out)
		}
	})
}

func TestCLI_SessionValidate(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := createTestClaudeTranscript(t, tmpDir)

	t.Run("human output", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetContext(context.Background())
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", "--env", "test-env", "--harness", "claude-code", transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session validate failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "[OK]") && !strings.Contains(out, "status:PASS") {
			t.Errorf("unexpected validate output: %s", out)
		}
	})

	t.Run("json output", func(t *testing.T) {
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", "--json", transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session validate --json failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, `"passed": true`) {
			t.Errorf("expected json validate report, got: %s", out)
		}
	})
}

func TestCLI_ErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("session list missing file", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"session", "list", filepath.Join(tmpDir, "missing.db")})
		if err := cmd.Execute(); err == nil {
			t.Errorf("expected error for missing database")
		}
	})

	t.Run("session replay missing file", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"session", "replay", filepath.Join(tmpDir, "missing.jsonl")})
		if err := cmd.Execute(); err == nil {
			t.Errorf("expected error for missing replay file")
		}
	})

	t.Run("session validate missing file", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"session", "validate", filepath.Join(tmpDir, "missing.jsonl")})
		if err := cmd.Execute(); err == nil {
			t.Errorf("expected error for missing validate file")
		}
	})

	t.Run("session validate failure path", func(t *testing.T) {
		emptyFile := filepath.Join(tmpDir, "empty.jsonl")
		_ = os.WriteFile(emptyFile, []byte(""), 0644)
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", emptyFile})
		if err := cmd.Execute(); err == nil {
			t.Errorf("expected validation to fail for empty file")
		}
	})

	t.Run("session validate with parse errors", func(t *testing.T) {
		corruptPath := filepath.Join(tmpDir, "corrupt.jsonl")
		content := `{"type":"invalid_json"
{"type":"user","message":"hello"}
`
		_ = os.WriteFile(corruptPath, []byte(content), 0644)
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", corruptPath})
		// validate should execute and return error due to parse error
		_ = cmd.Execute()
	})

	t.Run("session validate failure path json", func(t *testing.T) {
		emptyFile := filepath.Join(tmpDir, "empty.jsonl")
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", "--json", emptyFile})
		if err := cmd.Execute(); err == nil {
			t.Errorf("expected validation to fail for empty file with --json")
		}
	})

	t.Run("session replay with strict lifecycle", func(t *testing.T) {
		transcriptPath := createTestClaudeTranscript(t, tmpDir)
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "replay", "--strict-lifecycle", "--diff", transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session replay with --strict-lifecycle failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Session Replay Summary") && !strings.Contains(out, "harness:claude-code") {
			t.Errorf("expected summary, got: %s", out)
		}
	})

	t.Run("session validate with strict lifecycle detects open-ended transcript", func(t *testing.T) {
		transcriptPath := createTestClaudeTranscript(t, tmpDir)
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", "--strict-lifecycle", transcriptPath})
		err := cmd.Execute()
		if err == nil {
			t.Errorf("expected validation failure in strict mode for open-ended transcript")
		}
		out := buf.String()
		if !strings.Contains(out, "open-ended transcript") {
			t.Errorf("expected 'open-ended transcript' in validation output, got: %s", out)
		}
	})

	t.Run("session validate with alias no-synthetic-lifecycle", func(t *testing.T) {
		transcriptPath := createTestClaudeTranscript(t, tmpDir)
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", "--no-synthetic-lifecycle", "--json", transcriptPath})
		err := cmd.Execute()
		if err == nil {
			t.Errorf("expected validation failure with --no-synthetic-lifecycle")
		}
		out := buf.String()
		if !strings.Contains(out, "open-ended transcript") {
			t.Errorf("expected 'open-ended transcript' in json output, got: %s", out)
		}
	})

	t.Run("root command help", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{})
		if err := cmd.Execute(); err != nil {
			t.Errorf("root command failed: %v", err)
		}
	})

	t.Run("session command help", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"session"})
		if err := cmd.Execute(); err != nil {
			t.Errorf("session command failed: %v", err)
		}
	})
}
