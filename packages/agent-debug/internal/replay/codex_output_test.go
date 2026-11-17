package replay

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	serverstore "github.com/alexgorbatchev/agent-status/packages/agent-server/store"
)

func TestReplayCodexStructuredOutputAndTaskTimes(t *testing.T) {
	const body = `[{"type":"input_text","text":"first result"},{"type":"input_text","text":"second result"}]`
	transcript := `{"type":"session_meta","timestamp":"2026-10-11T19:38:22Z","payload":{"id":"structured-output","cwd":"/repo"}}
{"type":"event_msg","timestamp":"2026-10-11T19:38:23.118Z","payload":{"type":"task_started","started_at":1791747503}}
{"type":"response_item","timestamp":"2026-10-11T19:38:24Z","payload":{"type":"custom_tool_call","name":"mcp__example__query","call_id":"call-1","input":"{}"}}
{"type":"response_item","timestamp":"2026-10-11T19:38:25Z","payload":{"type":"custom_tool_call_output","call_id":"call-1","output":` + body + `}}
{"type":"event_msg","timestamp":"2026-10-11T19:40:22.354Z","payload":{"type":"task_complete","started_at":1791747503,"completed_at":1791747622}}`
	dir := t.TempDir()
	file, db := filepath.Join(dir, "codex.jsonl"), filepath.Join(dir, "replayed.db")
	if err := os.WriteFile(file, []byte(transcript), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := RunReplay(context.Background(), ReplayOptions{FilePath: file, Harness: types.HarnessCodex, ExportDB: db})
	if err != nil {
		t.Fatal(err)
	}
	if res.EventCounts["tool_result"] != 1 || res.Agent.Activity.ActiveTool != nil {
		t.Fatalf("result dropped or tool still active: counts=%v tool=%v", res.EventCounts, res.Agent.Activity.ActiveTool)
	}
	if res.EventCounts["turn_start"] != 1 || res.EventCounts["turn_end"] != 1 {
		t.Fatalf("task lifecycle dropped: counts=%v", res.EventCounts)
	}
	s, err := serverstore.OpenReadOnly(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	logs, err := s.GetAgentLogs(context.Background(), res.AgentID, 100, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	var foundOutput, foundStart, foundEnd bool
	for _, frame := range logs {
		switch frame.EventType {
		case "turn_start":
			foundStart = true
			if frame.Timestamp != 1791747503000 {
				t.Fatalf("start timestamp = %d, want milliseconds", frame.Timestamp)
			}
		case "turn_end":
			foundEnd = true
			if frame.Timestamp != 1791747622000 {
				t.Fatalf("end timestamp = %d, want milliseconds", frame.Timestamp)
			}
		case "tool_result":
			foundOutput = true
			if frame.Data.ToolOutput == nil || *frame.Data.ToolOutput != body || frame.Data.ToolCallID == nil || *frame.Data.ToolCallID != "call-1" {
				t.Fatalf("output or correlation lost: %+v", frame.Data)
			}
		}
	}
	if !foundOutput || !foundStart || !foundEnd {
		t.Fatal("tool output or task lifecycle missing from server logs")
	}
}
