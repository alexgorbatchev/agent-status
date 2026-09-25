package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
)

const sampleClaudeLog = `{"type":"ai-title","title":"Refactor parser","sessionId":"test-session-123"}
{"type":"user","message":{"role":"user","content":"Please check the status"},"uuid":"u-1","sessionId":"test-session-123","timestamp":"2026-09-20T12:00:00.000Z"}
{"type":"assistant","message":{"id":"msg-1","role":"assistant","model":"claude-3-7-sonnet","content":[{"type":"tool_use","id":"call-1","name":"Bash","input":{"command":"git status"}}]},"uuid":"a-1","sessionId":"test-session-123","timestamp":"2026-09-20T12:00:01.000Z"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"On branch main\nnothing to commit"}]},"uuid":"u-2","sessionId":"test-session-123","timestamp":"2026-09-20T12:00:02.000Z"}
{"type":"assistant","message":{"id":"msg-2","role":"assistant","model":"claude-3-7-sonnet","content":[{"type":"tool_use","id":"call-2","name":"Edit","input":{"file_path":"README.md","old_string":"foo","new_string":"bar"}}]},"uuid":"a-2","sessionId":"test-session-123","timestamp":"2026-09-20T12:00:03.000Z"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-2","is_error":true,"content":"File not found"}]},"uuid":"u-3","sessionId":"test-session-123","timestamp":"2026-09-20T12:00:04.000Z"}
{"type":"assistant","error":"Rate limit exceeded","sessionId":"test-session-123","timestamp":"2026-09-20T12:00:05.000Z"}
`

func createTestLogFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test-session.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test log file: %v", err)
	}
	return path
}

func TestPrintClaudeSession_FileNotFound(t *testing.T) {
	var buf bytes.Buffer
	err := PrintClaudeSession(&buf, "/non/existent/file.jsonl", PrintOptions{})
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestPrintClaudeSession_HumanMode(t *testing.T) {
	logPath := createTestLogFile(t, sampleClaudeLog)
	var buf bytes.Buffer

	err := PrintClaudeSession(&buf, logPath, PrintOptions{AgentMode: false, JSON: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[SESSION LOG]") {
		t.Errorf("expected [SESSION LOG] header, got: %s", out)
	}
	if !strings.Contains(out, "[USER]") {
		t.Errorf("expected [USER], got: %s", out)
	}
	if !strings.Contains(out, "[TOOL: Bash]\n    $ git status") {
		t.Errorf("expected [TOOL: Bash] with newline and indent, got: %s", out)
	}
	if !strings.Contains(out, "[RESULT]") {
		t.Errorf("expected [RESULT], got: %s", out)
	}
	if !strings.Contains(out, "[TOOL: Edit]\n    EDIT README.md") {
		t.Errorf("expected [TOOL: Edit] with newline and indent, got: %s", out)
	}
	if !strings.Contains(out, "[TOOL ERROR]") {
		t.Errorf("expected [TOOL ERROR], got: %s", out)
	}
	if !strings.Contains(out, "[ERROR]") {
		t.Errorf("expected [ERROR], got: %s", out)
	}
}

func TestPrintClaudeSession_AgentMode(t *testing.T) {
	logPath := createTestLogFile(t, sampleClaudeLog)
	var buf bytes.Buffer

	err := PrintClaudeSession(&buf, logPath, PrintOptions{AgentMode: true, JSON: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "SESSION:") {
		t.Errorf("expected SESSION: header in agent mode, got: %s", out)
	}
	if strings.Contains(out, "-----------------") {
		t.Errorf("dividers are forbidden in agent mode: %s", out)
	}
	if !strings.Contains(out, "USER:\nPlease check the status") {
		t.Errorf("expected USER: prompt in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "TOOL Bash:\n$ git status") {
		t.Errorf("expected TOOL Bash: in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "RESULT (OK):\nOn branch main\nnothing to commit") {
		t.Errorf("expected RESULT (OK): in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "TOOL Edit:") || !strings.Contains(out, "README.md") {
		t.Errorf("expected TOOL Edit: in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "RESULT (ERROR):\nFile not found") {
		t.Errorf("expected RESULT (ERROR): in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "ERROR:\nRate limit exceeded") {
		t.Errorf("expected ERROR: in agent mode, got: %s", out)
	}
}

func TestPrintClaudeSession_JSONMode(t *testing.T) {
	logPath := createTestLogFile(t, sampleClaudeLog)
	var buf bytes.Buffer

	err := PrintClaudeSession(&buf, logPath, PrintOptions{JSON: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []parser.ParsedEvent
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v, raw:\n%s", err, buf.String())
	}
	if len(parsed) == 0 {
		t.Fatal("expected non-empty parsed events in JSON mode")
	}
}

func TestPrintClaudeSession_EdgeEventsAndFormatting(t *testing.T) {
	var buf bytes.Buffer
	evs := []parser.ParsedEvent{
		{
			EventType: "turn_start",
			Data: types.TelemetryFrameData{
				Content: func() *string { v := "Hello assistant"; return &v }(),
			},
		},
		{
			EventType: "turn_end",
			Data: types.TelemetryFrameData{
				Content: func() *string { v := "Hello user, I can help you."; return &v }(),
			},
		},
		{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolName: func() *string { v := "CustomTool"; return &v }(),
				ToolInput: map[string]interface{}{
					"foo": "bar",
				},
			},
		},
		{
			EventType: "tool_result",
			Data:     types.TelemetryFrameData{},
		},
		{
			EventType: "file_modification",
			Data: types.TelemetryFrameData{
				FileModification: &types.FileModification{
					Action:   "WRITE",
					FilePath: "new_file.txt",
					Hunks: []types.DiffHunk{
						{
							Header: "@@ -0,0 +1,1 @@",
							Lines: []types.DiffLine{
								{Type: "add", Content: "Hello world"},
							},
						},
					},
				},
			},
		},
		{
			EventType: "run_exited",
			Data: types.TelemetryFrameData{
				ExitCode: func() *int { v := 0; return &v }(),
			},
		},
	}

	err := renderHumanMode(&buf, "dummy.jsonl", evs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[USER]") || !strings.Contains(out, "Hello assistant") {
		t.Errorf("missing user prompt, got: %s", out)
	}
	if !strings.Contains(out, "[ASSISTANT]") || !strings.Contains(out, "Hello user, I can help you.") {
		t.Errorf("missing assistant response, got: %s", out)
	}
	if !strings.Contains(out, "[EXIT] Session exited with code 0") {
		t.Errorf("missing exit, got: %s", out)
	}

	var agentBuf bytes.Buffer
	err = renderAgentMode(&agentBuf, "dummy.jsonl", evs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agentOut := agentBuf.String()
	if !strings.Contains(agentOut, "USER:\nHello assistant") {
		t.Errorf("missing USER: in agent mode: %s", agentOut)
	}
	if !strings.Contains(agentOut, "ASSISTANT:\nHello user, I can help you.") {
		t.Errorf("missing ASSISTANT: in agent mode: %s", agentOut)
	}
	if !strings.Contains(agentOut, "EXIT: code 0") {
		t.Errorf("missing EXIT: in agent mode: %s", agentOut)
	}
	if !strings.Contains(agentOut, "new_file.txt") {
		t.Errorf("missing file modification in agent mode: %s", agentOut)
	}
}

func TestFormatDiffHunks(t *testing.T) {
	if got := formatDiffHunks(nil); got != "" {
		t.Errorf("expected empty string for nil hunks, got %q", got)
	}

	hunks := []types.DiffHunk{
		{
			Header: "@@ -1,2 +1,2 @@",
			Lines: []types.DiffLine{
				{Type: "delete", Content: "old code"},
				{Type: "add", Content: "new code"},
				{Type: "context", Content: "same code"},
			},
		},
		{
			Lines: []types.DiffLine{
				{Type: "add", Content: "auto header code"},
			},
		},
	}

	diff := formatDiffHunks(hunks)
	if !strings.Contains(diff, "@@ -1,2 +1,2 @@") {
		t.Errorf("missing hunk header: %s", diff)
	}
	if !strings.Contains(diff, "-old code") || !strings.Contains(diff, "+new code") {
		t.Errorf("missing diff lines: %s", diff)
	}
	if !strings.Contains(diff, "@@ diff @@") {
		t.Errorf("missing fallback @@ diff @@: %s", diff)
	}
}

func TestPrintClaudeSession_Filters(t *testing.T) {
	logPath := createTestLogFile(t, sampleClaudeLog)

	// 1. ErrorsOnly
	var errBuf bytes.Buffer
	err := PrintClaudeSession(&errBuf, logPath, PrintOptions{ErrorsOnly: true, AgentMode: true})
	if err != nil {
		t.Fatalf("ErrorsOnly error: %v", err)
	}
	errOut := errBuf.String()
	if !strings.Contains(errOut, "ERROR") {
		t.Errorf("expected error events only, got: %s", errOut)
	}

	// 2. FilesOnly
	var filesBuf bytes.Buffer
	err = PrintClaudeSession(&filesBuf, logPath, PrintOptions{FilesOnly: true, AgentMode: true})
	if err != nil {
		t.Fatalf("FilesOnly error: %v", err)
	}
	filesOut := filesBuf.String()
	if !strings.Contains(filesOut, "README.md") {
		t.Errorf("expected file modification events, got: %s", filesOut)
	}

	// 3. ToolsOnly
	var toolsBuf bytes.Buffer
	err = PrintClaudeSession(&toolsBuf, logPath, PrintOptions{ToolsOnly: true, AgentMode: true})
	if err != nil {
		t.Fatalf("ToolsOnly error: %v", err)
	}
	toolsOut := toolsBuf.String()
	if !strings.Contains(toolsOut, "TOOL Bash:") {
		t.Errorf("expected tool_call events, got: %s", toolsOut)
	}
	if strings.Contains(toolsOut, "USER:") {
		t.Errorf("unexpected USER: in tools only, got: %s", toolsOut)
	}

	// 4. PromptsOnly
	var promptsBuf bytes.Buffer
	err = PrintClaudeSession(&promptsBuf, logPath, PrintOptions{PromptsOnly: true, AgentMode: true})
	if err != nil {
		t.Fatalf("PromptsOnly error: %v", err)
	}
	promptsOut := promptsBuf.String()
	if !strings.Contains(promptsOut, "USER:") {
		t.Errorf("expected USER: events, got: %s", promptsOut)
	}
	if strings.Contains(promptsOut, "TOOL Bash:") {
		t.Errorf("unexpected TOOL in prompts only, got: %s", promptsOut)
	}

	// 5. Tail
	var tailBuf bytes.Buffer
	err = PrintClaudeSession(&tailBuf, logPath, PrintOptions{Tail: 2, AgentMode: true})
	if err != nil {
		t.Fatalf("Tail error: %v", err)
	}
	if len(tailBuf.String()) == 0 {
		t.Fatal("expected non-empty output on Tail")
	}

	// 6. Limit
	var limitBuf bytes.Buffer
	err = PrintClaudeSession(&limitBuf, logPath, PrintOptions{Limit: 1, AgentMode: true})
	if err != nil {
		t.Fatalf("Limit error: %v", err)
	}
	if len(limitBuf.String()) == 0 {
		t.Fatal("expected non-empty output on Limit")
	}

	// 7. SummaryOnly
	var sumBuf bytes.Buffer
	err = PrintClaudeSession(&sumBuf, logPath, PrintOptions{SummaryOnly: true, AgentMode: true})
	if err != nil {
		t.Fatalf("SummaryOnly error: %v", err)
	}
	if !strings.Contains(sumBuf.String(), "SESSION_SUMMARY") {
		t.Errorf("expected SESSION_SUMMARY, got: %s", sumBuf.String())
	}

	// 8. Handoff
	var handoffBuf bytes.Buffer
	err = PrintClaudeSession(&handoffBuf, logPath, PrintOptions{Handoff: true, AgentMode: true})
	if err != nil {
		t.Fatalf("Handoff error: %v", err)
	}
	if !strings.Contains(handoffBuf.String(), "CONTINUATION_CONTEXT") {
		t.Errorf("expected CONTINUATION_CONTEXT, got: %s", handoffBuf.String())
	}

	// 9. LastTurn
	var lastTurnBuf bytes.Buffer
	err = PrintClaudeSession(&lastTurnBuf, logPath, PrintOptions{LastTurn: true, AgentMode: true})
	if err != nil {
		t.Fatalf("LastTurn error: %v", err)
	}
	if !strings.Contains(lastTurnBuf.String(), "USER:") {
		t.Errorf("expected USER: on LastTurn, got: %s", lastTurnBuf.String())
	}
}

func TestWrapTextWithIndent_UserExample(t *testing.T) {
	input := "Both halves flashed and verified, and the keyboard now reports **Imprint (Patched) 0.2.3**. Both used the same build (`120ad68d`, SHA-256 `d08cb7c40a0c`), so the halves match. I committed the two new log rows (`44729be`).\n\n**Remaining steps:**\n- **Step 6, the part only you can check:** make sure your keymap and drag-scroll settings survived and the DPI keys step as expected. Pointing DPI should now range from 100 to 1,000.\n- **Step 7, the flash-key test.** Unplug USB, hold `=`, and plug back in. The LEDs should turn solid blue and `RPI-RP2` should appear in Finder"

	wrapped := WrapTextWithIndent(input, "    ", 80)
	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "    ") {
			t.Errorf("line %d does not start with 4 spaces: %q", i, line)
		}
		if len(line) > 80 {
			t.Errorf("line %d exceeds 80 cols (%d): %q", i, len(line), line)
		}
	}
}

func TestWrapTextWithIndent_EdgeCases(t *testing.T) {
	// 1. termWidth <= 0 defaults to 80
	out := WrapTextWithIndent("Short text", "    ", 0)
	if out != "    Short text" {
		t.Errorf("expected '    Short text', got %q", out)
	}

	// 2. Numbered list hanging indent
	numList := "1. First long item that should wrap onto the second line with hanging indentation aligned"
	wrappedNum := WrapTextWithIndent(numList, "    ", 40)
	numLines := strings.Split(wrappedNum, "\n")
	if len(numLines) < 2 {
		t.Errorf("expected at least 2 lines for wrapped numbered list, got %d", len(numLines))
	}
	if !strings.HasPrefix(numLines[0], "    1. ") {
		t.Errorf("expected line 0 to start with '    1. ', got %q", numLines[0])
	}
	if !strings.HasPrefix(numLines[1], "       ") {
		t.Errorf("expected line 1 to have hanging indent, got %q", numLines[1])
	}

	// 3. GetTerminalWidth fallback
	width := GetTerminalWidth()
	if width <= 0 {
		t.Errorf("GetTerminalWidth = %d, want > 0", width)
	}
}
