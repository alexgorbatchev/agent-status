package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/mattn/go-runewidth"
)

func TestRenderAgentView_HumanMode_Question(t *testing.T) {
	q := "Do you want to proceed with editing jwt.go?"
	relPath := "feature-auth"
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-1",
		SessionID: "ses-1",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:    "env-1",
			Label: "local-dev",
		},
		Project: types.IProjectInfo{
			ID:   "proj-1",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     "main",
			IsWorktree: true,
			RelPath:    &relPath,
		},
		Activity: types.IAgentActivity{
			Lifecycle:     types.LifecycleRunning,
			Activity:      types.ActivityWaitingQuestion,
			Title:         "Refactor authentication middleware",
			Action:        "Executing Edit: internal/auth/jwt.go",
			Question:      &q,
			SubagentCount: 2,
			UpdatedAt:     1000000,
			Cost: &types.ICostInfo{
				Currency: "USD",
				Total:    0.0142,
			},
			Usage: types.ITokenUsage{
				Input:  10000,
				Output: 4520,
				Total:  14520,
			},
		},
	}

	var buf bytes.Buffer
	// 1000000 + 120000 ms = 2 minutes later
	RenderAgentViewAt(&buf, agent, false, 1000000+120000)
	out := buf.String()

	// Assert semantic components
	if !strings.Contains(out, "(Claude Code) [Focus] [Hide]") {
		t.Errorf("expected header with harness, [Focus], and [Hide], got:\n%s", out)
	}
	if !strings.Contains(out, "(RUNNING)") {
		t.Errorf("expected header with lifecycle, got:\n%s", out)
	}
	if !strings.Contains(out, "Refactor authentication middleware") {
		t.Errorf("expected title in body, got:\n%s", out)
	}
	if !strings.Contains(out, "Executing Edit: internal/auth/jwt.go") {
		t.Errorf("expected action in body, got:\n%s", out)
	}
	if !strings.Contains(out, "[?] Do you want to proceed with editing jwt.go?") {
		t.Errorf("expected question callout, got:\n%s", out)
	}
	if !strings.Contains(out, "(local-dev)  (branch: main)") {
		t.Errorf("expected env badge before branch badge, got:\n%s", out)
	}
	if !strings.Contains(out, "(worktree: feature-auth)") {
		t.Errorf("expected worktree badge, got:\n%s", out)
	}
	if !strings.Contains(out, "(2 subagents)") {
		t.Errorf("expected subagents badge, got:\n%s", out)
	}
	if !strings.Contains(out, "(active: 2m ago)") {
		t.Errorf("expected relative active time badge, got:\n%s", out)
	}
	if !strings.Contains(out, "$0.0142 USD") {
		t.Errorf("expected cost in metrics row, got:\n%s", out)
	}
	if !strings.Contains(out, "14,520 tokens") {
		t.Errorf("expected formatted tokens in metrics row, got:\n%s", out)
	}
}

func TestRenderAgentView_HumanMode_Error(t *testing.T) {
	errMsg := "fatal: compilation failed at line 42"
	agent := types.IAgentState{
		ID:        "env-1:pi:ses-2",
		SessionID: "ses-2",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			Label: "prod-box",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "fix-bug",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityError,
			Title:     "Fix build error",
			Action:    "Running test",
			LastError: &errMsg,
			UpdatedAt: 50000,
			Usage: types.ITokenUsage{
				Total: 500,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 50000+10000)
	out := buf.String()

	if !strings.Contains(out, "(Pi) [Focus] [Hide]") {
		t.Errorf("expected header with Pi harness, [Focus], and [Hide], got:\n%s", out)
	}
	if !strings.Contains(out, "(prod-box)  (branch: fix-bug)") {
		t.Errorf("expected env badge before branch badge, got:\n%s", out)
	}
	if !strings.Contains(out, "[!] fatal: compilation failed at line 42") {
		t.Errorf("expected error callout, got:\n%s", out)
	}
	if !strings.Contains(out, "(active: 10s ago)") {
		t.Errorf("expected 10s ago, got:\n%s", out)
	}
	if !strings.Contains(out, "$0.0000 USD") {
		t.Errorf("expected 0.0000 cost, got:\n%s", out)
	}
	if !strings.Contains(out, "500 tokens") {
		t.Errorf("expected 500 tokens, got:\n%s", out)
	}
}

func TestRenderAgentView_HumanMode_Hidden(t *testing.T) {
	agent := types.IAgentState{
		Harness:  types.HarnessCodex,
		IsHidden: true,
		Activity: types.IAgentActivity{
			Lifecycle:     types.LifecycleExited,
			Activity:      types.ActivityIdle,
			SubagentCount: 1,
			UpdatedAt:     1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000+2000)
	out := buf.String()

	if !strings.Contains(out, "(Codex) [Unhide]") {
		t.Errorf("expected header with Codex harness and [Unhide], got:\n%s", out)
	}
	if !strings.Contains(out, "(local-dev)  (branch: main)") {
		t.Errorf("expected env badge before branch badge, got:\n%s", out)
	}
	if !strings.Contains(out, "(EXITED)") {
		t.Errorf("expected (EXITED) lifecycle, got:\n%s", out)
	}
	if !strings.Contains(out, "(1 subagent)") {
		t.Errorf("expected singular (1 subagent), got:\n%s", out)
	}
	if !strings.Contains(out, "(active: just now)") {
		t.Errorf("expected (active: just now), got:\n%s", out)
	}
}

func TestRenderAgentView_HumanMode_Fallbacks(t *testing.T) {
	tool := "grep"
	agent := types.IAgentState{
		Harness: types.HarnessOpencode,
		Activity: types.IAgentActivity{
			Lifecycle:  types.LifecycleRunning,
			Activity:   types.ActivityBusy,
			ActiveTool: &tool,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 0)
	out := buf.String()

	if !strings.Contains(out, "Untitled Agent Session") {
		t.Errorf("expected fallback title, got:\n%s", out)
	}
	if !strings.Contains(out, "Executing grep") {
		t.Errorf("expected Executing grep from ActiveTool, got:\n%s", out)
	}
	if !strings.Contains(out, "(OpenCode) [Focus] [Hide]") {
		t.Errorf("expected header with OpenCode harness, [Focus], and [Hide], got:\n%s", out)
	}
	if !strings.Contains(out, "(local-dev)  (branch: main)") {
		t.Errorf("expected fallback env badge before branch badge, got:\n%s", out)
	}
	if !strings.Contains(out, "(active: never)") {
		t.Errorf("expected active: never when updatedAt is 0, got:\n%s", out)
	}
}

func TestRenderAgentView_AgentMode(t *testing.T) {
	q := "Do you want to proceed with editing jwt.go?"
	relPath := "feature-auth"
	agent := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			Label: "local-dev",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     "main",
			IsWorktree: true,
			RelPath:    &relPath,
		},
		Activity: types.IAgentActivity{
			Lifecycle:     types.LifecycleRunning,
			Activity:      types.ActivityWaitingQuestion,
			Title:         "Refactor authentication middleware",
			Action:        "Executing Edit: internal/auth/jwt.go",
			Question:      &q,
			SubagentCount: 2,
			Cost: &types.ICostInfo{
				Total: 0.0142,
			},
			Usage: types.ITokenUsage{
				Total: 14520,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentView(&buf, agent, true)
	line := strings.TrimSpace(buf.String())

	expected := `harness:claude-code env:local-dev title:"Refactor authentication middleware" action:"Executing Edit: internal/auth/jwt.go" activity:WAITING_QUESTION question:"Do you want to proceed with editing jwt.go?" lifecycle:RUNNING branch:main worktree:feature-auth subagents:2 cost_usd:0.0142 tokens:14520`

	if line != expected {
		t.Errorf("agent mode mismatch:\nexpected: %s\ngot:      %s", expected, line)
	}
}

func TestRenderAgentView_AgentMode_Error(t *testing.T) {
	errMsg := "something blew up"
	agent := types.IAgentState{
		Harness: types.HarnessPi,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityError,
			Title:     "Crash task",
			Action:    "Crashing",
			LastError: &errMsg,
			Usage: types.ITokenUsage{
				Total: 123,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentView(&buf, agent, true)
	line := strings.TrimSpace(buf.String())

	if !strings.Contains(line, `error:"something blew up"`) {
		t.Errorf("expected error in agent mode line, got:\n%s", line)
	}
	if !strings.Contains(line, `activity:ERROR`) {
		t.Errorf("expected activity:ERROR, got:\n%s", line)
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		tokens int64
		want   string
	}{
		{0, "0 tokens"},
		{500, "500 tokens"},
		{1000, "1,000 tokens"},
		{14520, "14,520 tokens"},
		{1234567, "1,234,567 tokens"},
	}
	for _, tc := range tests {
		got := FormatTokens(tc.tokens)
		if got != tc.want {
			t.Errorf("FormatTokens(%d) = %q, want %q", tc.tokens, got, tc.want)
		}
	}
}

func TestFormatRelativeTime(t *testing.T) {
	now := int64(1000000000)
	tests := []struct {
		ts   int64
		want string
	}{
		{0, "never"},
		{-10, "never"},
		{now + 100, "just now"},
		{now - 3000, "just now"},
		{now - 15000, "15s ago"},
		{now - 180000, "3m ago"},
		{now - 7200000, "2h ago"},
		{now - 172800000, "2d ago"},
	}
	for _, tc := range tests {
		got := FormatRelativeTime(tc.ts, now)
		if got != tc.want {
			t.Errorf("FormatRelativeTime(%d) = %q, want %q", tc.ts, got, tc.want)
		}
	}
}

func TestFormatHarnessLabel(t *testing.T) {
	if got := FormatHarnessLabel(types.HarnessClaudeCode); got != "Claude Code" {
		t.Errorf("expected Claude Code, got %s", got)
	}
	if got := FormatHarnessLabel(types.HarnessPi); got != "Pi" {
		t.Errorf("expected Pi, got %s", got)
	}
	if got := FormatHarnessLabel(types.HarnessCodex); got != "Codex" {
		t.Errorf("expected Codex, got %s", got)
	}
	if got := FormatHarnessLabel(types.HarnessOpencode); got != "OpenCode" {
		t.Errorf("expected OpenCode, got %s", got)
	}
	if got := FormatHarnessLabel(types.HarnessType("custom-harness")); got != "custom-harness" {
		t.Errorf("expected custom-harness, got %s", got)
	}
	if got := FormatHarnessLabel(""); got != "Unknown" {
		t.Errorf("expected Unknown, got %s", got)
	}
}

func TestRenderAgentView_WallClock(t *testing.T) {
	agent := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
		},
	}
	var buf bytes.Buffer
	RenderAgentView(&buf, agent, false)
	if !strings.Contains(buf.String(), "(Claude Code) [Focus] [Hide]") {
		t.Errorf("expected (Claude Code) [Focus] [Hide] in wall clock rendering, got:\n%s", buf.String())
	}
}

func TestRenderAgentView_HumanMode_EastAsianWidthAlignment(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:custom:ses-eaw",
		SessionID: "ses-eaw",
		Harness:   types.HarnessType("日本語ハーネス"), // 7 CJK characters = 14 visual cells, 21 UTF-8 bytes
		Environment: types.IEnvironmentInfo{
			Label: "開発環境", // 4 CJK chars = 8 visual cells, 12 UTF-8 bytes
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "機能追加", // 4 CJK chars = 8 visual cells, 12 UTF-8 bytes
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "日本語タイトルのエージェントセッション",
			Action:    "ソースコードを編集中",
			Cost: &types.ICostInfo{
				Total: 0.1234,
			},
			Usage: types.ITokenUsage{
				Total: 9876,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d", len(lines))
	}

	dividerWidth := runewidth.StringWidth(lines[0])
	headerLine := lines[1]
	headerWidth := runewidth.StringWidth(headerLine)

	if headerWidth != dividerWidth {
		t.Errorf("headerLine visual width = %d, want %d (equal to divider width)", headerWidth, dividerWidth)
	}

	metricsLine := lines[len(lines)-2]
	metricsWidth := runewidth.StringWidth(metricsLine)
	if metricsWidth != dividerWidth {
		t.Errorf("metricsLine visual width = %d, want %d (equal to divider width)", metricsWidth, dividerWidth)
	}
}

func TestRenderAgentView_HumanMode_EmojiWidthAlignment(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:custom:ses-emoji",
		SessionID: "ses-emoji",
		Harness:   types.HarnessType("Claude 🚀"), // Emoji 🚀 is 2 visual cells, 4 UTF-8 bytes
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			Title:     "Task with emoji",
			Action:    "Waiting",
			Usage: types.ITokenUsage{
				Total: 100,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	dividerWidth := runewidth.StringWidth(lines[0])
	headerWidth := runewidth.StringWidth(lines[1])

	if headerWidth != dividerWidth {
		t.Errorf("headerLine visual width = %d, want %d (equal to divider width)", headerWidth, dividerWidth)
	}
}

func TestRenderAgentView_HumanMode_AnsiColorWidthAlignment(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:custom:ses-ansi",
		SessionID: "ses-ansi",
		Harness:   types.HarnessType("\x1b[34mClaude Code\x1b[0m"), // ANSI colored harness label
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleState("\x1b[32mRUNNING\x1b[0m"), // ANSI colored lifecycle
			Activity:  types.ActivityBusy,
			Title:     "Session with ANSI styling",
			Action:    "Testing ANSI width calculations",
			Cost: &types.ICostInfo{
				Total: 0.5,
			},
			Usage: types.ITokenUsage{
				Total: 5000,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	dividerWidth := runewidth.StringWidth(stripANSI(lines[0]))
	headerWidth := runewidth.StringWidth(stripANSI(lines[1]))

	if headerWidth != dividerWidth {
		t.Errorf("headerLine visual width = %d, want %d (equal to divider width)", headerWidth, dividerWidth)
	}
}



