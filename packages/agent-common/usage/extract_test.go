package usage

import (
	"reflect"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func str(s string) *string { return &s }

func TestExtract(t *testing.T) {
	for _, tc := range []struct {
		name, harness, event, tool, content string
		input                               map[string]any
		want                                []string
	}{
		{"Claude skill invocation", "claude-code", "tool_call", "Skill", "", map[string]any{"skill": "plugin:review", "args": "private arguments"}, []string{"skill/invoke//plugin:review"}},
		{"OpenCode skill invocation", "opencode", "tool_use", "skill", "", map[string]any{"name": "review"}, []string{"skill/invoke//review"}},
		{"Pi skill expansion", "pi", "turn_start", "", "<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nInstructions\n</skill>\n\nprivate arguments", nil, []string{"skill/load//review", "command/invoke///skill:review"}},
		{"Codex skill injection", "codex", "turn_start", "", "<skill>\n<name>review</name>\n<path>/skills/review/SKILL.md</path>\nInstructions\n</skill>", nil, []string{"skill/load//review"}},
		{"Claude command", "claude-code", "turn_start", "", "<command-name>/review</command-name>\n<command-message>review</command-message>\n<command-args>private arguments</command-args>", nil, []string{"command/invoke///review"}},
		{"Pi command request", "pi", "turn_start", "", "/review private arguments", nil, []string{"command/request///review"}},
		{"Codex command request", "codex", "turn_start", "", "/review", nil, []string{"command/request///review"}},
		{"OpenCode command request", "opencode", "user_prompt", "", "/review", nil, []string{"command/request///review"}},
		{"Claude skill read", "claude-code", "tool_call", "Read", "", map[string]any{"file_path": "/skills/review/SKILL.md"}, []string{"skill/load///skills/review/SKILL.md"}},
		{"Pi skill read", "pi", "tool_call", "read", "", map[string]any{"path": "skills/review/SKILL.md"}, []string{"skill/load//skills/review/SKILL.md"}},
		{"OpenCode skill read", "opencode", "tool_use", "read", "", map[string]any{"filePath": "skills/review/SKILL.md"}, []string{"skill/load//skills/review/SKILL.md"}},
		{"Codex skill read", "codex", "tool_call", "read_file", "", map[string]any{"path": "skills/review/SKILL.md"}, []string{"skill/load//skills/review/SKILL.md"}},
		{"malformed read path", "pi", "tool_call", "read", "", map[string]any{"path": "skills/\nreview/SKILL.md"}, nil},
		{"missing tool name", "pi", "tool_call", "", "", nil, nil},
		{"MCP invocation", "codex", "tool_call", "mcp__github__list_issues", "", map[string]any{"token": "private arguments"}, []string{"mcp/invoke/github/list_issues"}},
		{"Pi MCP invocation", "pi", "tool_call", "mcp__github__list_issues", "", nil, []string{"mcp/invoke/github/list_issues"}},
		{"plugin MCP invocation", "claude-code", "tool_call", "mcp__plugin_git_github__list_issues", "", nil, []string{"mcp/invoke/plugin_git_github/list_issues"}},
		{"shell excluded", "codex", "tool_call", "exec_command", "", map[string]any{"cmd": "cat /skills/review/SKILL.md"}, nil},
		{"result excluded", "claude-code", "tool_result", "Skill", "", map[string]any{"skill": "review"}, nil},
		{"catalog excluded", "pi", "turn_start", "", "<available_skills>\n<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nInstructions\n</skill>\n</available_skills>", nil, nil},
		{"mention excluded", "pi", "turn_start", "", "Please use /review and $review", nil, nil},
		{"path excluded", "pi", "turn_start", "", "/etc/passwd", nil, nil},
		{"incomplete MCP", "codex", "tool_call", "mcp__github__", "", nil, nil},
		{"non MCP", "opencode", "tool_use", "github_list_issues", "", nil, nil},
		{"missing skill name", "claude-code", "tool_call", "Skill", "", map[string]any{"skill": 7}, nil},
		{"wrong harness", "pi", "tool_call", "Skill", "", map[string]any{"skill": "review"}, nil},
		{"regular file", "pi", "tool_call", "read", "", map[string]any{"path": "README.md"}, nil},
		{"quoted marker", "claude-code", "turn_start", "", "Example: <command-name>/review</command-name>", nil, nil},
		{"assistant text excluded", "codex", "agent_message", "", "/review", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := types.ITelemetryFrame{AgentID: "agent", Harness: types.HarnessType(tc.harness), EventType: tc.event, Timestamp: 1000}
			f.Data.ToolName = str(tc.tool)
			f.Data.ToolCallID = str("call-1")
			f.Data.ToolInput = tc.input
			f.Data.Content = str(tc.content)
			var got []string
			for _, o := range Extract(f) {
				got = append(got, o.Kind+"/"+o.Operation+"/"+o.Server+"/"+o.Name)
				if o.ID == "" || o.Evidence.Timestamp != 1000 || o.Evidence.EventType != tc.event {
					t.Fatalf("missing evidence: %+v", o)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestObservationIdentity(t *testing.T) {
	f := types.ITelemetryFrame{AgentID: "agent", Harness: types.HarnessClaudeCode, EventType: "tool_call", Timestamp: 1000}
	f.Data.ToolName = str("Skill")
	f.Data.ToolCallID = str("call-1")
	f.Data.ToolInput = map[string]any{"skill": "review"}
	first := Extract(f)[0]
	f.Seq = 500
	f.Timestamp = 2000
	if got := Extract(f)[0]; got.ID != first.ID {
		t.Fatal("native call ID must survive replay and timestamp updates")
	}
	f.SubagentID = str("child")
	if got := Extract(f)[0]; got.ID == first.ID {
		t.Fatal("subagents must have independent invocation identities")
	}
	f.Data.ToolCallID = nil
	a := Extract(f)[0]
	f.Seq++
	if got := Extract(f)[0]; got.ID != a.ID {
		t.Fatal("fallback identity must ignore transport sequence")
	}
	f.Timestamp++
	if got := Extract(f)[0]; got.ID == a.ID {
		t.Fatal("distinct timestamped calls without native IDs must remain distinct")
	}
	f.Timestamp = 0
	if got := Extract(f); len(got) != 0 {
		t.Fatal("cannot count an observation without a stable identity")
	}
}
