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

	t.Run("db command help", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"db"})
		if err := cmd.Execute(); err != nil {
			t.Errorf("db command failed: %v", err)
		}
	})

	t.Run("db inspect human and json", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		statusDB := createTestStatusDB(t, t.TempDir())

		// 1. Human mode
		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"db", "inspect", "test-session", "--db", statusDB})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("db inspect human mode failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Persisted Agent Record (test-session)") {
			t.Errorf("expected header in human mode, got: %s", out)
		}
		if !strings.Contains(out, "Main Test Agent") {
			t.Errorf("expected title in human mode, got: %s", out)
		}

		// 2. JSON mode
		cmdJSON := newRootCmd()
		var bufJSON bytes.Buffer
		cmdJSON.SetOut(&bufJSON)
		cmdJSON.SetArgs([]string{"db", "inspect", "test-session", "--db", statusDB, "--json"})
		if err := cmdJSON.Execute(); err != nil {
			t.Fatalf("db inspect json mode failed: %v", err)
		}
		outJSON := bufJSON.String()
		if !strings.Contains(outJSON, `"sessionId": "test-session"`) {
			t.Errorf("expected sessionId in json mode, got: %s", outJSON)
		}
	})

	t.Run("session replay with db comparison", func(t *testing.T) {
		t.Setenv("AGENT", "0")
		transcriptPath := createTestClaudeTranscript(t, t.TempDir())
		statusDB := createTestStatusDB(t, t.TempDir())

		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "replay", "--db", statusDB, transcriptPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("session replay with db failed: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Live Database Comparison") {
			t.Errorf("expected Live Database Comparison section, got: %s", out)
		}
		if !strings.Contains(out, "Lifecycle") {
			t.Errorf("expected comparison fields in output, got: %s", out)
		}

		// Also verify AGENT=1 mode
		t.Setenv("AGENT", "1")
		var bufAgent bytes.Buffer
		cmdAgent := newRootCmd()
		cmdAgent.SetOut(&bufAgent)
		cmdAgent.SetArgs([]string{"session", "replay", "--db", statusDB, transcriptPath})
		if err := cmdAgent.Execute(); err != nil {
			t.Fatalf("session replay in agent mode failed: %v", err)
		}
		outAgent := bufAgent.String()
		if !strings.Contains(outAgent, "db_comparison") {
			t.Errorf("expected db_comparison in agent mode, got: %s", outAgent)
		}
	})

	t.Run("session validate with db comparison", func(t *testing.T) {
		transcriptPath := createTestClaudeTranscript(t, t.TempDir())
		statusDB := createTestStatusDB(t, t.TempDir())

		cmd := newRootCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"session", "validate", "--db", statusDB, transcriptPath})
		// validate might fail due to drift (e.g. tokens/cost or lifecycle), but the check should execute!
		_ = cmd.Execute()
		out := buf.String()
		if !strings.Contains(out, "Database Discrepancy Check") {
			t.Errorf("expected Database Discrepancy Check in validation output, got: %s", out)
		}
	})

	t.Run("db commands error paths", func(t *testing.T) {
		transcriptPath := createTestClaudeTranscript(t, t.TempDir())
		statusDB := createTestStatusDB(t, t.TempDir())

		// 1. db inspect with missing db
		cmd1 := newRootCmd()
		cmd1.SetArgs([]string{"db", "inspect", "ses-1", "--db", "/non/existent/db.sqlite"})
		if err := cmd1.Execute(); err == nil {
			t.Error("expected error for non-existent db in db inspect, got nil")
		}

		// 2. db inspect with missing session
		cmd2 := newRootCmd()
		cmd2.SetArgs([]string{"db", "inspect", "ses-missing", "--db", statusDB})
		if err := cmd2.Execute(); err == nil {
			t.Error("expected error for missing session in db inspect, got nil")
		}

		// 3. session replay with invalid db path
		cmd3 := newRootCmd()
		cmd3.SetArgs([]string{"session", "replay", "--db", "/non/existent/db.sqlite", transcriptPath})
		if err := cmd3.Execute(); err == nil {
			t.Error("expected error for non-existent db in replay, got nil")
		}

		// 4. session replay with valid db but session missing
		emptyDBDir := t.TempDir()
		emptyDB := filepath.Join(emptyDBDir, "empty.db")
		edb, err := sql.Open("sqlite3", "file:"+emptyDB)
		if err == nil {
			_, _ = edb.Exec(`CREATE TABLE agents (id TEXT, session_id TEXT); CREATE TABLE runs (id TEXT); CREATE TABLE telemetry_events (id INTEGER); CREATE TABLE environments (id TEXT);`)
			_ = edb.Close()
		}
		cmd4 := newRootCmd()
		cmd4.SetArgs([]string{"session", "replay", "--db", emptyDB, transcriptPath})
		if err := cmd4.Execute(); err == nil {
			t.Error("expected error for missing session in replay --db, got nil")
		}

		// 5. session validate with invalid db path
		cmd5 := newRootCmd()
		var buf5 bytes.Buffer
		cmd5.SetOut(&buf5)
		cmd5.SetArgs([]string{"session", "validate", "--db", "/non/existent/db.sqlite", transcriptPath})
		_ = cmd5.Execute()
		if !strings.Contains(buf5.String(), "resolving database failed") {
			t.Errorf("expected resolving database failed, got: %s", buf5.String())
		}

		// 6. session validate with valid db but session missing
		cmd6 := newRootCmd()
		var buf6 bytes.Buffer
		cmd6.SetOut(&buf6)
		cmd6.SetArgs([]string{"session", "validate", "--db", emptyDB, transcriptPath})
		_ = cmd6.Execute()
		if !strings.Contains(buf6.String(), "session not found in database") {
			t.Errorf("expected session not found in database, got: %s", buf6.String())
		}
	})
}

func createTestStatusDB(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "test-agent-status.db")
	db, err := sql.Open("sqlite3", "file:"+p)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
	CREATE TABLE environments (
		id TEXT PRIMARY KEY,
		label TEXT NOT NULL,
		hostname TEXT NOT NULL,
		platform TEXT NOT NULL,
		user TEXT NOT NULL,
		dropped_frames INTEGER NOT NULL DEFAULT 0,
		last_heartbeat_at INTEGER NOT NULL
	);
	CREATE TABLE runs (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		pid INTEGER NOT NULL,
		harness_version TEXT NOT NULL,
		started_at INTEGER NOT NULL,
		ended_at INTEGER
	);
	CREATE TABLE agents (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		environment_id TEXT NOT NULL,
		harness TEXT NOT NULL,
		project_id TEXT NOT NULL,
		project_name TEXT NOT NULL,
		canonical_root TEXT NOT NULL,
		branch TEXT NOT NULL,
		is_detached INTEGER NOT NULL DEFAULT 0,
		is_worktree INTEGER NOT NULL DEFAULT 0,
		worktree_rel_path TEXT,
		cwd TEXT NOT NULL,
		current_run_id TEXT,
		activity TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL DEFAULT '',
		action_stale INTEGER NOT NULL DEFAULT 0,
		phase TEXT,
		active_tool TEXT,
		active_target TEXT,
		question TEXT,
		last_thinking_excerpt TEXT,
		last_error TEXT,
		tokens_input INTEGER NOT NULL DEFAULT 0,
		tokens_output INTEGER NOT NULL DEFAULT 0,
		tokens_cache_read INTEGER NOT NULL DEFAULT 0,
		tokens_cache_write INTEGER NOT NULL DEFAULT 0,
		tokens_reasoning INTEGER NOT NULL DEFAULT 0,
		tokens_total INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL,
		subagent_count INTEGER NOT NULL DEFAULT 0,
		first_started_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE TABLE telemetry_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id TEXT NOT NULL,
		subagent_id TEXT,
		event_type TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		data_json TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating test status db schema: %v", err)
	}
	now := time.Now().UnixMilli()
	_, _ = db.Exec(`INSERT INTO environments VALUES ('env-1', 'local', 'host', 'darwin', 'dev', 0, ?)`, now)
	agentID := "env-1:claude-code:test-session"
	sessID := "test-session"
	_, _ = db.Exec(`
		INSERT INTO agents (
			id, session_id, environment_id, harness, project_id, project_name,
			canonical_root, branch, cwd, current_run_id, activity, title, action,
			tokens_input, tokens_output, tokens_total, cost_usd, first_started_at, updated_at
		) VALUES (
			?, ?, 'env-1', 'claude-code', 'proj-1', 'agent-status',
			'/repo', 'main', '/repo', 'run-1', 'IDLE', 'Main Test Agent', 'Done',
			100, 20, 120, 0.005, ?, ?
		)
	`, agentID, sessID, now-5000, now)
	_, _ = db.Exec(`INSERT INTO runs VALUES ('run-1', ?, 5555, '1.0', ?, NULL)`, agentID, now-5000)
	_, _ = db.Exec(`INSERT INTO telemetry_events (agent_id, event_type, timestamp, data_json) VALUES (?, 'run_started', ?, '{}')`, agentID, now-5000)
	return p
}

func TestCLI_ScannerAudit(t *testing.T) {
	// Test human mode
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"scanner", "audit", "--harness", "nonexistent"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("scanner audit failed: %v", err)
	}
	if !strings.Contains(buf.String(), "No processes or sessions found") &&
		!strings.Contains(buf.String(), "Process Discovery") &&
		!strings.Contains(buf.String(), "scanner_audit") {
		t.Errorf("unexpected output: %s", buf.String())
	}

	// Test JSON mode
	var jsonBuf bytes.Buffer
	cmdJSON := newRootCmd()
	cmdJSON.SetOut(&jsonBuf)
	cmdJSON.SetErr(&jsonBuf)
	cmdJSON.SetArgs([]string{"scanner", "audit", "--json", "--harness", "nonexistent"})
	if err := cmdJSON.Execute(); err != nil {
		t.Fatalf("scanner audit --json failed: %v", err)
	}
	if !strings.Contains(jsonBuf.String(), "[]") && !strings.Contains(jsonBuf.String(), "null") {
		t.Errorf("expected json array, got %s", jsonBuf.String())
	}
}

func TestCLI_ProcessList(t *testing.T) {
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"process", "list", "--harness", "nonexistent"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("process list failed: %v", err)
	}
}

