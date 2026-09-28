package replay

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-parser"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestLoadSession_EdgeCases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("unsupported harness", func(t *testing.T) {
		f := filepath.Join(tmpDir, "dummy.txt")
		_ = os.WriteFile(f, []byte("test"), 0644)
		_, err := LoadSession(ctx, f, types.HarnessType("unsupported_harness"), "", "")
		if err == nil {
			t.Errorf("expected error for unsupported harness")
		}
	})

	t.Run("auto-detect failure", func(t *testing.T) {
		f := filepath.Join(tmpDir, "unknown.xyz")
		_ = os.WriteFile(f, []byte("completely random text"), 0644)
		_, err := LoadSession(ctx, f, "", "", "")
		if err == nil {
			t.Errorf("expected error when auto-detection fails")
		}
	})

	t.Run("opencode empty database", func(t *testing.T) {
		dbFile := filepath.Join(tmpDir, "empty_opencode.db")
		db, err := sql.Open("sqlite3", "file:"+dbFile)
		if err != nil {
			t.Fatalf("opening sqlite: %v", err)
		}
		schema := `
			CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL);
			CREATE TABLE session (id TEXT PRIMARY KEY, project_id TEXT, directory TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL);
			CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
			CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
		`
		_, _ = db.Exec(schema)
		_ = db.Close()

		_, err = LoadSession(ctx, dbFile, types.HarnessOpencode, "", "")
		if err == nil {
			t.Errorf("expected error when opencode database has 0 sessions")
		}
	})

	t.Run("opencode non-existent session ID", func(t *testing.T) {
		dbFile := filepath.Join(tmpDir, "single_sess.db")
		db, err := sql.Open("sqlite3", "file:"+dbFile)
		if err != nil {
			t.Fatalf("opening sqlite: %v", err)
		}
		schema := `
			CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL);
			CREATE TABLE session (id TEXT PRIMARY KEY, project_id TEXT, directory TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL);
			CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
			CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
		`
		_, _ = db.Exec(schema)
		_, _ = db.Exec(`INSERT INTO session (id, directory, time_created, time_updated) VALUES ('s1', '/repo', 1000, 2000)`)
		_ = db.Close()

		_, err = LoadSession(ctx, dbFile, types.HarnessOpencode, "missing_session_id", "")
		if err == nil {
			t.Errorf("expected error for non-existent session ID")
		}
	})

	t.Run("ensureSyntheticLifecycle with empty events", func(t *testing.T) {
		res := ensureSyntheticLifecycle(nil, 1000, 2000, "/repo")
		if len(res) != 0 {
			t.Errorf("expected 0 events, got %d", len(res))
		}
	})

	t.Run("readJSONLLines with missing file", func(t *testing.T) {
		_, _, err := readJSONLLines(filepath.Join(tmpDir, "does-not-exist.jsonl"), nil)
		if err == nil {
			t.Errorf("expected error for missing file")
		}
	})

	t.Run("ensureSyntheticLifecycle tags synthetic frames and respects strictLifecycle", func(t *testing.T) {
		events := []ReplayEvent{
			{
				Event: parser.ParsedEvent{
					EventType: "tool_call",
					Timestamp: 1500,
					CWD:       "/repo",
				},
				SourceLine: 1,
			},
		}

		// Non-strict: should inject synthetic run_started and run_exited
		res := ensureSyntheticLifecycle(events, 1000, 2000, "/repo", false)
		if len(res) != 3 {
			t.Fatalf("expected 3 events, got %d", len(res))
		}
		if !res[0].IsSynthetic || res[0].Event.EventType != "run_started" {
			t.Errorf("expected synthetic run_started at index 0, got synthetic=%v type=%s", res[0].IsSynthetic, res[0].Event.EventType)
		}
		if res[1].IsSynthetic {
			t.Errorf("expected native event at index 1 to not be synthetic")
		}
		if !res[2].IsSynthetic || res[2].Event.EventType != "run_exited" {
			t.Errorf("expected synthetic run_exited at index 2, got synthetic=%v type=%s", res[2].IsSynthetic, res[2].Event.EventType)
		}

		// Strict: should NOT inject synthetic run_started or run_exited
		strictRes := ensureSyntheticLifecycle(events, 1000, 2000, "/repo", true)
		if len(strictRes) != 1 {
			t.Fatalf("expected 1 event in strict mode, got %d", len(strictRes))
		}
		if strictRes[0].Event.EventType != "tool_call" {
			t.Errorf("expected tool_call, got %s", strictRes[0].Event.EventType)
		}
	})

	t.Run("loadCodexSession preserves parser state across lines within session", func(t *testing.T) {
		codexFile := filepath.Join(tmpDir, "rollout-2026-06-12T16-08-43-test-session.jsonl")
		content := `{"type":"session_meta","timestamp":"2026-06-12T16:08:43.000Z","payload":{"id":"test-session","cwd":"/repo"}}
{"type":"event_msg","timestamp":"2026-06-12T16:08:44.000Z","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":100,"prompt_tokens":80,"completion_tokens":20},"last_token_usage":{"total_tokens":100,"prompt_tokens":80,"completion_tokens":20}},"rate_limits":{"remaining_tokens":0,"reset_tokens":"60s"}}}
{"type":"event_msg","timestamp":"2026-06-12T16:08:45.000Z","payload":{"type":"error","message":"Rate limit exceeded","codex_error_info":"RateLimitExceeded"}}
`
		if err := os.WriteFile(codexFile, []byte(content), 0644); err != nil {
			t.Fatalf("writing codex file: %v", err)
		}
		loaded, err := LoadSession(ctx, codexFile, types.HarnessCodex, "", "")
		if err != nil {
			t.Fatalf("LoadSession failed: %v", err)
		}
		var foundErrorEvent bool
		for _, re := range loaded.Events {
			if re.Event.EventType == "error" {
				foundErrorEvent = true
				if re.Event.Data.RetryAfterSeconds == nil || *re.Event.Data.RetryAfterSeconds != 59 {
					t.Errorf("expected error event to have RetryAfterSeconds 59 (60s discounted by 1s) from session parser state, got %v", re.Event.Data.RetryAfterSeconds)
				}
			}
		}
		if !foundErrorEvent {
			t.Errorf("expected error event in loaded session")
		}
	})
}
