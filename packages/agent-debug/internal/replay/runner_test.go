package replay

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestReplay_ClaudeCode(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create main Claude Code transcript
	sessID := "claude-session-001"
	mainFile := filepath.Join(tmpDir, fmt.Sprintf("%s.jsonl", sessID))
	mainContent := `{"type":"ai-title","aiTitle":"Refactor Database Layer","timestamp":"2026-09-21T10:00:00.000Z","cwd":"/repo","gitBranch":"feature/db"}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Refactor the DB queries"}]},"timestamp":"2026-09-21T10:00:01.000Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-1","name":"GlobTool","input":{"pattern":"*.go"}}]},"timestamp":"2026-09-21T10:00:03.000Z"}
{"type":"tool_result","tool_use_id":"tool-1","content":"main.go\nstore.go","timestamp":"2026-09-21T10:00:04.000Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Found database files."}]},"usage":{"input_tokens":1500,"output_tokens":300},"costUSD":0.0045,"timestamp":"2026-09-21T10:00:06.000Z"}
`
	if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
		t.Fatalf("writing main transcript: %v", err)
	}

	// 2. Create subagents directory with a sibling subagent transcript
	subagentsDir := filepath.Join(tmpDir, sessID, "subagents")
	if err := os.MkdirAll(subagentsDir, 0755); err != nil {
		t.Fatalf("creating subagents dir: %v", err)
	}
	subFile := filepath.Join(subagentsDir, "sub-indexer.jsonl")
	subContent := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Index codebase symbols"}]},"timestamp":"2026-09-21T10:00:02.000Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Indexing finished"}]},"usage":{"input_tokens":500,"output_tokens":100},"costUSD":0.0015,"timestamp":"2026-09-21T10:00:05.000Z"}
`
	if err := os.WriteFile(subFile, []byte(subContent), 0644); err != nil {
		t.Fatalf("writing subagent transcript: %v", err)
	}

	ctx := context.Background()

	t.Run("basic replay with subagent interleaving and verification", func(t *testing.T) {
		res, err := RunReplay(ctx, ReplayOptions{
			FilePath: mainFile,
			Harness:  types.HarnessClaudeCode,
		})
		if err != nil {
			t.Fatalf("RunReplay failed: %v", err)
		}

		if res.Harness != types.HarnessClaudeCode {
			t.Errorf("expected harness %s, got %s", types.HarnessClaudeCode, res.Harness)
		}
		if res.SessionID != sessID {
			t.Errorf("expected session ID %s, got %s", sessID, res.SessionID)
		}
		if len(res.Subagents) != 1 || res.Subagents[0] != "sub-indexer" {
			t.Errorf("expected 1 subagent 'sub-indexer', got %v", res.Subagents)
		}

		if res.Agent == nil {
			t.Fatalf("expected non-nil Agent in ReplayResult")
		}
		if res.Agent.Activity.Title != "Refactor Database Layer" {
			t.Errorf("expected title 'Refactor Database Layer', got %q", res.Agent.Activity.Title)
		}
		if res.Agent.Activity.Lifecycle != types.LifecycleExited {
			t.Errorf("expected Lifecycle EXITED (via synthetic run_exited), got %s", res.Agent.Activity.Lifecycle)
		}

		// Check server-side read verification report
		if res.Verification == nil || !res.Verification.Passed {
			t.Fatalf("expected verification to pass, report: %+v", res.Verification)
		}
		if res.Verification.LogCount == 0 {
			t.Errorf("expected > 0 verified log frames")
		}

		summary := res.Summary()
		if summary.SessionID != sessID || !summary.VerificationOK {
			t.Errorf("unexpected summary: %+v", summary)
		}
	})

	t.Run("replay with diff tracking and foreign CWD remapping", func(t *testing.T) {
		var collectedDiffs []output.StateDiff
		customCWD := "/local/dev/my-custom-repo"

		res, err := RunReplay(ctx, ReplayOptions{
			FilePath: mainFile,
			Harness:  types.HarnessClaudeCode,
			CWD:      customCWD,
			Diff:     true,
			OnDiff: func(diff output.StateDiff) {
				collectedDiffs = append(collectedDiffs, diff)
			},
		})
		if err != nil {
			t.Fatalf("RunReplay failed: %v", err)
		}

		if res.Agent.Workspace.CWD != customCWD {
			t.Errorf("expected Workspace CWD %q, got %q", customCWD, res.Agent.Workspace.CWD)
		}
		if len(collectedDiffs) == 0 {
			t.Errorf("expected diffs to be collected, got 0")
		}
	})

	t.Run("replay with export db", func(t *testing.T) {
		exportedDB := filepath.Join(tmpDir, "exported-replay.db")
		res, err := RunReplay(ctx, ReplayOptions{
			FilePath: mainFile,
			ExportDB: exportedDB,
		})
		if err != nil {
			t.Fatalf("RunReplay with ExportDB failed: %v", err)
		}

		if _, err := os.Stat(exportedDB); err != nil {
			t.Fatalf("exported db not created: %v", err)
		}

		if !res.Verification.Passed {
			t.Errorf("verification failed on exported DB")
		}
	})

	t.Run("stop on error and live summarize options", func(t *testing.T) {
		res, err := RunReplay(ctx, ReplayOptions{
			FilePath:    mainFile,
			StopOnError: true,
			Summarize:   true,
		})
		if err != nil {
			t.Fatalf("RunReplay with StopOnError & Summarize failed: %v", err)
		}
		if res.FramesProcessed == 0 {
			t.Errorf("expected frames processed")
		}
	})

	t.Run("session without ai-title extracts title from prompt and preserves it through subagent", func(t *testing.T) {
		noAiSessID := "f9c8fea8-a808-4887-9018-121c5d354d59"
		noAiFile := filepath.Join(tmpDir, fmt.Sprintf("%s.jsonl", noAiSessID))
		noAiContent := `{"type":"mode","mode":"default","sessionId":"` + noAiSessID + `"}
{"type":"permission-mode","permissionMode":"bypassPermissions","sessionId":"` + noAiSessID + `"}
{"type":"system","content":"System prompt","timestamp":"2026-09-22T19:55:16.976Z","sessionId":"` + noAiSessID + `"}
{"type":"user","message":{"role":"user","content":"<local-command-caveat>Caveat...</local-command-caveat>"},"timestamp":"2026-09-22T19:55:37.203Z","sessionId":"` + noAiSessID + `"}
{"type":"user","message":{"role":"user","content":"/code-review"},"timestamp":"2026-09-22T19:56:00.385Z","sessionId":"` + noAiSessID + `"}
{"type":"last-prompt","lastPrompt":"/code-review","sessionId":"` + noAiSessID + `"}
`
		if err := os.WriteFile(noAiFile, []byte(noAiContent), 0644); err != nil {
			t.Fatalf("writing no-ai transcript: %v", err)
		}

		// Create subagent dir
		noAiSubDir := filepath.Join(tmpDir, noAiSessID, "subagents")
		if err := os.MkdirAll(noAiSubDir, 0755); err != nil {
			t.Fatalf("creating subagents dir: %v", err)
		}
		noAiSubFile := filepath.Join(noAiSubDir, "agent-a2945b5234405c174.jsonl")
		noAiSubContent := `{"parentUuid":"p1","isSidechain":true,"agentId":"a2945b5234405c174","type":"user","message":{"role":"user","content":"start review"},"timestamp":"2026-09-22T19:56:04.755Z","sessionId":"` + noAiSessID + `"}
{"parentUuid":"p2","isSidechain":true,"agentId":"a2945b5234405c174","type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Review complete"}]},"usage":{"input_tokens":500,"output_tokens":100},"costUSD":0.001,"timestamp":"2026-09-22T19:56:10.000Z","sessionId":"` + noAiSessID + `"}
`
		if err := os.WriteFile(noAiSubFile, []byte(noAiSubContent), 0644); err != nil {
			t.Fatalf("writing subagent file: %v", err)
		}

		res, err := RunReplay(ctx, ReplayOptions{
			FilePath: noAiFile,
			Harness:  types.HarnessClaudeCode,
		})
		if err != nil {
			t.Fatalf("RunReplay failed: %v", err)
		}

		if res.Agent == nil {
			t.Fatalf("expected non-nil Agent")
		}
		if res.Agent.Activity.Title != "/code-review" {
			t.Errorf("Agent Title = %q, want '/code-review'", res.Agent.Activity.Title)
		}
	})

	t.Run("breakpoints: until-line and until-event", func(t *testing.T) {
		resLine, err := RunReplay(ctx, ReplayOptions{
			FilePath:  mainFile,
			UntilLine: 2,
		})
		if err != nil {
			t.Fatalf("RunReplay until-line failed: %v", err)
		}
		if resLine.FramesProcessed > 5 {
			t.Errorf("expected <= 5 frames with line breakpoint, got %d", resLine.FramesProcessed)
		}

		resEvent, err := RunReplay(ctx, ReplayOptions{
			FilePath:   mainFile,
			UntilEvent: "turn_start",
		})
		if err != nil {
			t.Fatalf("RunReplay until-event failed: %v", err)
		}
		if resEvent.EventCounts["turn_start"] != 1 {
			t.Errorf("expected exactly 1 turn_start before break, got %v", resEvent.EventCounts)
		}
	})
}

func TestReplay_Pi(t *testing.T) {
	tmpDir := t.TempDir()
	piFile := filepath.Join(tmpDir, "pi-session-test.jsonl")
	piContent := `{"type":"session","id":"pi-sess-101","cwd":"/projects/agent","version":1,"timestamp":1700000000000}
{"type":"model_change","modelId":"claude-3-5-sonnet-20241022","timestamp":1700000001000}
{"type":"message","message":{"role":"user","content":[{"type":"text","text":"Implement CLI command"}]},"timestamp":1700000002000}
{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","toolName":"bash","arguments":{"command":"ls"}}]},"timestamp":1700000003000}
{"type":"message","message":{"role":"toolResult","toolName":"bash","content":"file1.go\nfile2.go"},"timestamp":1700000004000}
{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Done"}]},"usage":{"input":800,"output":120},"timestamp":1700000005000}
`
	if err := os.WriteFile(piFile, []byte(piContent), 0644); err != nil {
		t.Fatalf("writing pi transcript: %v", err)
	}

	ctx := context.Background()
	res, err := RunReplay(ctx, ReplayOptions{
		FilePath: piFile,
	})
	if err != nil {
		t.Fatalf("RunReplay(Pi) failed: %v", err)
	}

	if res.Harness != types.HarnessPi {
		t.Errorf("expected harness Pi, got %s", res.Harness)
	}
	if res.Agent == nil {
		t.Fatalf("expected non-nil Agent")
	}
	if res.Agent.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected Lifecycle EXITED, got %s", res.Agent.Activity.Lifecycle)
	}
	if res.Verification == nil || !res.Verification.Passed {
		t.Errorf("expected verification to pass, report: %+v", res.Verification)
	}
	if !res.HasSyntheticEnd {
		t.Errorf("expected HasSyntheticEnd to be true in default mode")
	}
	if !res.Summary().IsLifecycleSynthetic {
		t.Errorf("expected IsLifecycleSynthetic to be true in default mode")
	}

	// In strict mode, open-ended Pi transcript must NOT inject synthetic run_exited
	resStrict, err := RunReplay(ctx, ReplayOptions{
		FilePath:        piFile,
		StrictLifecycle: true,
	})
	if err != nil {
		t.Fatalf("RunReplay(Pi, strict) failed: %v", err)
	}
	if resStrict.Agent == nil {
		t.Fatalf("expected non-nil Agent in strict mode")
	}
	if resStrict.Agent.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("expected Lifecycle RUNNING in strict mode, got %s", resStrict.Agent.Activity.Lifecycle)
	}
	if resStrict.HasSyntheticEnd {
		t.Errorf("expected HasSyntheticEnd to be false in strict mode")
	}
	if resStrict.Summary().Lifecycle != types.LifecycleRunning {
		t.Errorf("expected Summary Lifecycle RUNNING in strict mode, got %s", resStrict.Summary().Lifecycle)
	}
	if resStrict.Summary().IsLifecycleSynthetic {
		t.Errorf("expected IsLifecycleSynthetic to be false in strict mode")
	}
	if resStrict.Verification == nil || resStrict.Verification.Passed {
		t.Errorf("expected verification to fail in strict mode for open-ended transcript")
	}
}

func TestReplay_Codex(t *testing.T) {
	tmpDir := t.TempDir()
	codexFile := filepath.Join(tmpDir, "rollout-2026-09-21T10-30-00-cdx-1.jsonl")
	codexContent := `{"type":"session_meta","timestamp":"2026-09-21T10:30:00.000Z","payload":{"id":"cdx-1","cwd":"/repo"}}
{"type":"event_msg","timestamp":"2026-09-21T10:30:01.000Z","payload":{"type":"user_message","message":"Build project"}}
{"type":"event_msg","timestamp":"2026-09-21T10:30:02.000Z","payload":{"type":"agent_message","message":"Building now"}}
{"type":"event_msg","timestamp":"2026-09-21T10:30:03.000Z","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":1450,"prompt_tokens":1200,"completion_tokens":250}}}}
`
	if err := os.WriteFile(codexFile, []byte(codexContent), 0644); err != nil {
		t.Fatalf("writing codex transcript: %v", err)
	}

	ctx := context.Background()
	res, err := RunReplay(ctx, ReplayOptions{
		FilePath: codexFile,
	})
	if err != nil {
		t.Fatalf("RunReplay(Codex) failed: %v", err)
	}

	if res.Harness != types.HarnessCodex {
		t.Errorf("expected harness Codex, got %s", res.Harness)
	}
	if res.Agent.Activity.Usage.Input != 1200 || res.Agent.Activity.Usage.Output != 250 {
		t.Errorf("unexpected token usage: in=%d out=%d", res.Agent.Activity.Usage.Input, res.Agent.Activity.Usage.Output)
	}
	if res.Verification == nil || !res.Verification.Passed {
		t.Errorf("expected verification to pass, report: %+v", res.Verification)
	}
}

func TestReplay_Opencode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "opencode.db")

	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}

	schema := `
		CREATE TABLE project (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			worktree TEXT,
			vcs TEXT
		);

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

		CREATE TABLE message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			data TEXT NOT NULL
		);

		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			data TEXT NOT NULL
		);

		CREATE TABLE permission (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			type TEXT NOT NULL,
			data TEXT NOT NULL,
			time_created INTEGER NOT NULL
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating schema: %v", err)
	}

	now := time.Now().UnixMilli()
	// Insert two sessions into OpenCode DB to test multi-session selection
	_, err = db.Exec(`
		INSERT INTO session (id, project_id, directory, title, version, tokens_input, tokens_output, time_created, time_updated)
		VALUES 
			('ses_open_1', 'proj-1', '/repo/app', 'First Session', '1.0.0', 1000, 200, ?, ?),
			('ses_open_2', 'proj-1', '/repo/app', 'Second Session', '1.0.0', 2000, 400, ?, ?)
	`, now-10000, now-5000, now-4000, now)
	if err != nil {
		t.Fatalf("inserting sessions: %v", err)
	}

	// Insert messages and parts for ses_open_1
	_, err = db.Exec(`
		INSERT INTO message (id, session_id, time_created, data)
		VALUES 
			('msg_1', 'ses_open_1', ?, '{"role":"user"}'),
			('msg_2', 'ses_open_1', ?, '{"role":"assistant","model":"claude-3-5-sonnet"}')
	`, now-9000, now-8000)
	if err != nil {
		t.Fatalf("inserting messages: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO part (id, message_id, session_id, time_created, data)
		VALUES 
			('prt_1', 'msg_1', 'ses_open_1', ?, '{"type":"text","text":"Implement feature"}'),
			('prt_2', 'msg_2', 'ses_open_1', ?, '{"type":"tool_call","name":"edit","input":{"path":"main.go"}}'),
			('prt_3', 'msg_2', 'ses_open_1', ?, '{"type":"step-finish","usage":{"input_tokens":1000,"output_tokens":200}}')
	`, now-9000, now-8000, now-7000)
	if err != nil {
		t.Fatalf("inserting parts: %v", err)
	}
	_ = db.Close()

	ctx := context.Background()

	t.Run("multiple sessions requires session-id", func(t *testing.T) {
		_, err := RunReplay(ctx, ReplayOptions{
			FilePath: dbPath,
		})
		if err == nil {
			t.Errorf("expected error when multiple sessions exist and session-id is omitted")
		}
	})

	t.Run("single session auto-selection in opencode", func(t *testing.T) {
		singleDB := filepath.Join(tmpDir, "single_auto.db")
		sdb, err := sql.Open("sqlite3", "file:"+singleDB)
		if err != nil {
			t.Fatalf("opening sqlite: %v", err)
		}
		_, _ = sdb.Exec(schema)
		_, _ = sdb.Exec(`
			INSERT INTO session (id, project_id, directory, title, version, tokens_input, tokens_output, time_created, time_updated)
			VALUES ('auto_sess_1', 'proj-1', '/repo/app', 'Auto Session', '1.0.0', 500, 100, ?, ?)
		`, now-1000, now)
		_ = sdb.Close()

		res, err := RunReplay(ctx, ReplayOptions{
			FilePath: singleDB,
			OnStep: func(step int, frame types.ITelemetryFrame) {
				// progress hook
			},
		})
		if err != nil {
			t.Fatalf("RunReplay auto single session failed: %v", err)
		}
		if res.SessionID != "auto_sess_1" {
			t.Errorf("expected auto_sess_1, got %s", res.SessionID)
		}
	})

	t.Run("specific session-id replay", func(t *testing.T) {
		res, err := RunReplay(ctx, ReplayOptions{
			FilePath:  dbPath,
			SessionID: "ses_open_1",
		})
		if err != nil {
			t.Fatalf("RunReplay(OpenCode) failed: %v", err)
		}

		if res.Harness != types.HarnessOpencode {
			t.Errorf("expected harness OpenCode, got %s", res.Harness)
		}
		if res.SessionID != "ses_open_1" {
			t.Errorf("expected session ID ses_open_1, got %s", res.SessionID)
		}
		if res.Agent.Activity.Title != "First Session" {
			t.Errorf("expected title 'First Session', got %q", res.Agent.Activity.Title)
		}
		if res.Verification == nil || !res.Verification.Passed {
			t.Errorf("expected verification to pass, report: %+v", res.Verification)
		}
	})
}
