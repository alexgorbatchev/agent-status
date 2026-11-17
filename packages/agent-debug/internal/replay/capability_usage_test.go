package replay

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	serverstore "github.com/alexgorbatchev/agent-status/packages/agent-server/store"
)

func TestReplayCapabilityUsage(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		harness               types.HarnessType
		transcript            string
		skills, commands, mcp int
	}{
		{"Claude", types.HarnessClaudeCode,
			`{"type":"user","sessionId":"usage","timestamp":"2026-10-11T12:00:00Z","message":{"role":"user","content":"<command-name>/review</command-name>\n<command-message>review</command-message>"}}
{"type":"assistant","sessionId":"usage","timestamp":"2026-10-11T12:00:01Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"skill-1","name":"Skill","input":{"skill":"review"}},{"type":"tool_use","id":"mcp-1","name":"mcp__github__list_issues","input":{}}]}}`, 1, 1, 1},
		{"Pi", types.HarnessPi,
			`{"type":"session","id":"usage","timestamp":"2026-10-11T12:00:00Z","cwd":"/repo"}
{"type":"message","id":"msg-1","timestamp":"2026-10-11T12:00:01Z","message":{"role":"user","content":"<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nInstructions\n</skill>"}}
{"type":"message","id":"msg-2","timestamp":"2026-10-11T12:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"mcp-1","name":"mcp__github__list_issues","arguments":{}}]}}`, 1, 1, 1},
		{"Codex", types.HarnessCodex,
			`{"type":"session_meta","timestamp":"2026-10-11T12:00:00Z","payload":{"id":"usage","cwd":"/repo"}}
{"type":"response_item","timestamp":"2026-10-11T12:00:01Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<skill>\n<name>review</name>\n<path>/skills/review/SKILL.md</path>\nInstructions\n</skill>"}]}}
{"type":"response_item","timestamp":"2026-10-11T12:00:02Z","payload":{"type":"function_call","call_id":"mcp-1","name":"mcp__github__list_issues","arguments":"{}"}}`, 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "usage.jsonl")
			db := filepath.Join(dir, "usage.db")
			if err := os.WriteFile(file, []byte(tc.transcript), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := RunReplay(context.Background(), ReplayOptions{FilePath: file, Harness: tc.harness, ExportDB: db})
			if err != nil {
				t.Fatal(err)
			}
			s, err := serverstore.OpenReadOnly(db)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			r, err := s.GetAgentUsage(context.Background(), result.AgentID)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Skills) != tc.skills || len(r.Commands) != tc.commands || len(r.MCP) != tc.mcp {
				t.Fatalf("usage did not survive parser, replay, persistence, and read queries: %+v", r)
			}
			if r.Skills[0].Count != 1 || r.MCP[0].Count != 1 {
				t.Fatalf("duplicate parser emissions inflated counts: %+v", r)
			}
		})
	}
}
