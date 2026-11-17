package replay

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	serverstore "github.com/alexgorbatchev/agent-status/packages/agent-server/store"
)

func TestReplayNativeRecapAndQuestions(t *testing.T) {
	for _, tt := range []struct {
		name               string
		harness            types.HarnessType
		transcript         string
		field, text, event string
	}{
		{"claude recap", types.HarnessClaudeCode,
			`{"type":"system","subtype":"away_summary","sessionId":"native","timestamp":"2026-10-09T23:47:11.636Z","content":"We're finishing the Yarn-to-Bun migration PR and its review fixes"}`, "summary", "We're finishing the Yarn-to-Bun migration PR and its review fixes", "summary"},
		{"codex question", types.HarnessCodex,
			`{"type":"session_meta","timestamp":"2026-10-09T23:47:10Z","payload":{"id":"native","cwd":"/repo"}}
{"type":"event_msg","timestamp":"2026-10-09T23:47:11.636Z","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"question-1","content":[{"type":"Text","text":"For --all, where does your dashboard run?"}],"delivery":"async","questions":[{"title":"For --all, where does your dashboard run?"}]}}}`, "question", "For --all, where does your dashboard run?", "question_prompt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			file := filepath.Join(dir, "native.jsonl")
			if err := os.WriteFile(file, []byte(tt.transcript), 0600); err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(dir, "replayed.db")
			res, err := RunReplay(ctx, ReplayOptions{FilePath: file, Harness: tt.harness, ExportDB: db})
			if err != nil {
				t.Fatal(err)
			}
			if res.EventCounts[tt.event] != 1 {
				t.Fatalf("event counts = %v", res.EventCounts)
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
			agent, err := s.GetAgentByID(ctx, res.AgentID)
			if err != nil {
				t.Fatal(err)
			}
			agents, err := s.GetAgents(ctx, serverstore.AgentFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(agents) != 1 {
				t.Fatalf("agents = %d, want 1", len(agents))
			}
			for _, candidate := range []*types.IAgentState{res.Agent, agent, &agents[0]} {
				encoded, err := json.Marshal(candidate.Activity)
				if err != nil {
					t.Fatal(err)
				}
				var activity map[string]interface{}
				if err := json.Unmarshal(encoded, &activity); err != nil {
					t.Fatal(err)
				}
				if activity[tt.field] != tt.text {
					t.Fatalf("%s = %v, want %q", tt.field, activity[tt.field], tt.text)
				}
				if tt.field == "summary" && activity["summaryUpdatedAt"] != float64(1791589631636) {
					t.Fatalf("recap timestamp = %v", activity["summaryUpdatedAt"])
				}
			}
			logs, err := s.GetAgentLogs(ctx, res.AgentID, 100, 0, "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, frame := range logs {
				if frame.EventType != tt.event {
					continue
				}
				found = true
				text := frame.Data.Content
				if tt.event == "question_prompt" {
					text = frame.Data.Question
				}
				if text == nil || *text != tt.text {
					t.Fatalf("lost event text: %+v", frame.Data)
				}
			}
			if !found {
				t.Fatalf("missing %s log", tt.event)
			}
		})
	}
}
