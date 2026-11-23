package replay

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/testenv"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

const agyTranscript = `{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-21T00:00:00Z","content":"Inspect the repository"}
{"step_index":1,"type":"PLANNER_RESPONSE","created_at":"2026-11-21T00:00:01Z","tool_calls":[{"id":"call-1","name":"run_command","args":{"CommandLine":"pwd"}}],"model":"gemini","input_tokens":100}
{"step_index":2,"type":"GENERIC","created_at":"2026-11-21T00:00:02Z","content":"/repo"}
{"step_index":3,"type":"PLANNER_RESPONSE","created_at":"2026-11-21T00:00:03Z","content":"Done","output_tokens":12}
`

func writeAgyTranscript(t *testing.T, brain, id string) string {
	t.Helper()
	path := filepath.Join(brain, id, ".system_generated", "logs", "transcript_full.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(agyTranscript), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAgyReplayRecoversWorkspaceFromCustomRoot(t *testing.T) {
	root := t.TempDir()
	path := writeAgyTranscript(t, filepath.Join(root, "custom-sessions"), "conversation-1")
	conversations := filepath.Join(root, "conversations")
	if err := os.MkdirAll(conversations, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conversations, "conversation-1.meta"), []byte(`{"cwd":"/repo/custom"}`), 0600); err != nil {
		t.Fatal(err)
	}
	session, err := LoadSession(context.Background(), path, types.HarnessAgy, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if session.CWD != "/repo/custom" || session.SessionID != "conversation-1" {
		t.Fatalf("session=%+v", session)
	}
}

func TestAgyReplayPreservesNativeTitleWithCWDOverride(t *testing.T) {
	root := t.TempDir()
	path := writeAgyTranscript(t, filepath.Join(root, "brain"), "conversation-1")
	db, err := sql.Open("sqlite", filepath.Join(root, "conversation_summaries.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE conversation_summaries (conversation_id TEXT, title TEXT, workspace_uris TEXT, status TEXT, parent_conversation_id TEXT, last_modified_time TEXT, not_fully_idle INTEGER, killed INTEGER);
INSERT INTO conversation_summaries VALUES ('conversation-1', 'Native title', '["file:///original"]', '', '', '', 0, 0);`); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{"", "/override"} {
		session, err := LoadSession(context.Background(), path, types.HarnessAgy, "", cwd)
		if err != nil {
			t.Fatal(err)
		}
		wantCWD := cwd
		if wantCWD == "" {
			wantCWD = "/original"
		}
		if session.CWD != wantCWD || session.Events[0].Event.Data.Title == nil || *session.Events[0].Event.Data.Title != "Native title" {
			t.Fatalf("session=%+v", session)
		}
	}
}

func TestAgyReplayDetectionResolutionAndToolCorrelation(t *testing.T) {
	testenv.IsolateHarnessHome(t)
	brain := filepath.Join(t.TempDir(), "brain")
	t.Setenv("AGY_SESSIONS_DIR", brain)
	path := writeAgyTranscript(t, brain, "conversation-123")
	for _, hint := range []types.HarnessType{"", types.HarnessAgy, "antigravity"} {
		target, err := ResolveSessionTarget(context.Background(), "conversation-123", hint)
		if err != nil {
			t.Fatal(err)
		}
		if target.FilePath != path || target.Harness != types.HarnessAgy || target.SessionID != "conversation-123" {
			t.Fatalf("target=%+v", target)
		}
		session, err := LoadSession(context.Background(), target.FilePath, hint, "", "/repo", true)
		if err != nil {
			t.Fatal(err)
		}
		if session.Harness != types.HarnessAgy || session.SessionID != "conversation-123" || session.CWD != "/repo" {
			t.Fatalf("session=%+v", session)
		}
		var startID, endID string
		var turns int
		for _, event := range session.Events {
			if event.Event.CWD != "/repo" {
				t.Fatalf("missing cwd: %+v", event)
			}
			switch event.Event.EventType {
			case "tool_call":
				if event.Event.Data.ToolCallID != nil {
					startID = *event.Event.Data.ToolCallID
				}
			case "tool_result":
				if event.Event.Data.ToolCallID != nil {
					endID = *event.Event.Data.ToolCallID
				}
			case "turn_end":
				turns++
			}
		}
		if startID != "call-1" || endID != startID || turns != 1 {
			t.Fatalf("tool ids=%q/%q turns=%d", startID, endID, turns)
		}
	}
	if _, err := ResolveSessionTarget(context.Background(), "conversation-", types.HarnessAgy); err != nil {
		t.Fatal(err)
	}
	writeAgyTranscript(t, brain, "conversation-456")
	if _, err := ResolveSessionTarget(context.Background(), "conversation-", types.HarnessAgy); err == nil {
		t.Fatal("ambiguous prefix accepted")
	}
	if _, err := LoadSession(context.Background(), "/missing", types.HarnessAgy, "", ""); err == nil {
		t.Fatal("missing transcript accepted")
	}
}
