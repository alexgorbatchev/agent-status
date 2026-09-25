package output

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

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
	if !strings.Contains(StripANSI(out), "(Claude Code) [Focus] [Hide]") {
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

	if !strings.Contains(StripANSI(out), "(Pi) [Focus] [Hide]") {
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

	if !strings.Contains(StripANSI(out), "(Codex) [Unhide]") {
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
	if !strings.Contains(StripANSI(out), "(OpenCode) [Focus] [Hide]") {
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
	if !strings.Contains(StripANSI(buf.String()), "(Claude Code) [Focus] [Hide]") {
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

	dividerWidth := runewidth.StringWidth(StripANSI(lines[0]))
	headerLine := lines[1]
	headerWidth := runewidth.StringWidth(StripANSI(headerLine))

	if headerWidth != dividerWidth {
		t.Errorf("headerLine visual width = %d, want %d (equal to divider width)", headerWidth, dividerWidth)
	}

	metricsLine := lines[len(lines)-2]
	metricsWidth := runewidth.StringWidth(StripANSI(metricsLine))
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
	dividerWidth := runewidth.StringWidth(StripANSI(lines[0]))
	headerWidth := runewidth.StringWidth(StripANSI(lines[1]))

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
	dividerWidth := runewidth.StringWidth(StripANSI(lines[0]))
	headerWidth := runewidth.StringWidth(StripANSI(lines[1]))

	if headerWidth != dividerWidth {
		t.Errorf("headerLine visual width = %d, want %d (equal to divider width)", headerWidth, dividerWidth)
	}
}

func TestRenderAgentView_HumanMode_TrimsProjectPrefix(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-prefix-1",
		SessionID: "ses-prefix-1",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:    "env-1",
			Label: "local-dev",
		},
		Project: types.IProjectInfo{
			ID:   "github.com/alexgorbatchev/agent-status",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "agent-status: Refactor database layer",
			Action:    "Editing store.go",
			Cost: &types.ICostInfo{
				Currency: "USD",
				Total:    0.01,
			},
			Usage: types.ITokenUsage{
				Total: 100,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	if !strings.Contains(out, "Refactor database layer") {
		t.Errorf("expected trimmed title %q in output, got:\n%s", "Refactor database layer", out)
	}
	if strings.Contains(out, "agent-status: Refactor database layer") {
		t.Errorf("expected redundant project prefix to be stripped from output, got:\n%s", out)
	}
}

func TestRenderAgentView_AgentMode_TrimsProjectPrefix(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-prefix-2",
		SessionID: "ses-prefix-2",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:    "env-1",
			Label: "local-dev",
		},
		Project: types.IProjectInfo{
			ID:   "github.com/alexgorbatchev/agent-status",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "[agent-status] Run scanner audit",
			Action:    "Running audit",
			Cost: &types.ICostInfo{
				Currency: "USD",
				Total:    0.01,
			},
			Usage: types.ITokenUsage{
				Total: 100,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentView(&buf, agent, true)
	out := buf.String()

	if !strings.Contains(out, `title:"Run scanner audit"`) {
		t.Errorf("expected trimmed title %q in agent mode output, got:\n%s", `title:"Run scanner audit"`, out)
	}
	if strings.Contains(out, `title:"[agent-status] Run scanner audit"`) {
		t.Errorf("expected redundant project prefix to be stripped from agent mode output, got:\n%s", out)
	}
}

func TestRenderAgentView_HumanMode_SemanticPrecedence_Question(t *testing.T) {
	q := "Should we apply migration?"
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-q",
		SessionID: "ses-q",
		Harness:   types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityWaitingQuestion,
			Title:     "Ask user confirmation",
			Action:    "Waiting for answer",
			Question:  &q,
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// 1. In WAITING_QUESTION, prompt style (ANSI Yellow) must override vendor badge coloring in header
	expectedBadge := AnsiYellow + "(Claude Code)" + AnsiReset
	if !strings.Contains(out, expectedBadge) {
		t.Errorf("expected harness badge to have prompt yellow style %q in WAITING_QUESTION, got:\n%s", expectedBadge, out)
	}
	// It must NOT contain vendor brand color (AnsiMagenta)
	if strings.Contains(out, AnsiMagenta+"(Claude Code)") {
		t.Errorf("harness badge must not use vendor brand color AnsiMagenta during WAITING_QUESTION state")
	}

	// 2. Callout must use prompt style (ANSI Yellow)
	expectedCallout := AnsiYellow + GlyphQuestion + " Should we apply migration?" + AnsiReset
	if !strings.Contains(out, expectedCallout) {
		t.Errorf("expected callout to have prompt yellow style %q, got:\n%s", expectedCallout, out)
	}
}

func TestRenderAgentView_HumanMode_SemanticPrecedence_Error(t *testing.T) {
	errMsg := "connection refused on port 5432"
	agent := types.IAgentState{
		ID:        "env-1:pi:ses-err",
		SessionID: "ses-err",
		Harness:   types.HarnessPi,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityError,
			Title:     "Database connect failure",
			Action:    "Retrying",
			LastError: &errMsg,
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// 1. In ERROR, error style (ANSI Red) must override vendor badge coloring in header
	expectedBadge := AnsiRed + "(Pi)" + AnsiReset
	if !strings.Contains(out, expectedBadge) {
		t.Errorf("expected harness badge to have error red style %q in ERROR state, got:\n%s", expectedBadge, out)
	}
	// It must NOT contain vendor brand color (AnsiBlue or AnsiMagenta)
	if strings.Contains(out, AnsiBlue+"(Pi)") || strings.Contains(out, AnsiMagenta+"(Pi)") {
		t.Errorf("harness badge must not use vendor brand color during ERROR state")
	}

	// 2. Callout must use error style (ANSI Red)
	expectedCallout := AnsiRed + GlyphAlert + " connection refused on port 5432" + AnsiReset
	if !strings.Contains(out, expectedCallout) {
		t.Errorf("expected callout to have error red style %q, got:\n%s", expectedCallout, out)
	}
}

func TestRenderAgentView_HumanMode_VendorBadge_WorkingState(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-busy",
		SessionID: "ses-busy",
		Harness:   types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Active working session",
			Action:    "Compiling",
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// In working state, harness badge uses vendor brand color (AnsiMagenta)
	expectedBadge := AnsiMagenta + "(Claude Code)" + AnsiReset
	if !strings.Contains(out, expectedBadge) {
		t.Errorf("expected harness badge to have vendor brand color %q in working state, got:\n%s", expectedBadge, out)
	}

	// Must strictly NOT contain Red or Green
	if strings.Contains(out, AnsiRed+"(Claude Code)") || strings.Contains(out, AnsiGreen+"(Claude Code)") {
		t.Errorf("vendor badge must never use Red or Green hues in working state")
	}
}

func TestRenderAgentView_HumanMode_VendorBadge_Pi_WorkingState(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:pi:ses-pi-work",
		SessionID: "ses-pi-work",
		Harness:   types.HarnessPi,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Pi working session",
			Action:    "Executing tool",
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// In working state, Pi badge uses distinct vendor brand styling (AnsiBold + AnsiMagenta) (#106)
	expectedBadge := AnsiBold + AnsiMagenta + "(Pi)" + AnsiReset
	if !strings.Contains(out, expectedBadge) {
		t.Errorf("expected harness badge to have Pi vendor brand styling %q in working state, got:\n%s", expectedBadge, out)
	}

	// Must strictly NOT contain Red, Green, or Blue
	if strings.Contains(out, AnsiRed+"(Pi)") || strings.Contains(out, AnsiGreen+"(Pi)") || strings.Contains(out, AnsiBlue+"(Pi)") {
		t.Errorf("vendor badge must never use Red, Green, or Blue hues in working state")
	}
}

func TestRenderAgentView_HumanMode_VendorBadge_Codex_WorkingState(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:codex:ses-codex-work",
		SessionID: "ses-codex-work",
		Harness:   types.HarnessCodex,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Codex working session",
			Action:    "Executing tool",
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// In working state, Codex badge uses AnsiCyan (#106)
	expectedBadge := AnsiCyan + "(Codex)" + AnsiReset
	if !strings.Contains(out, expectedBadge) {
		t.Errorf("expected harness badge to have Codex vendor brand styling %q in working state, got:\n%s", expectedBadge, out)
	}

	// Must strictly NOT contain Red, Green, Yellow, or Blue
	if strings.Contains(out, AnsiRed+"(Codex)") || strings.Contains(out, AnsiGreen+"(Codex)") || strings.Contains(out, AnsiYellow+"(Codex)") || strings.Contains(out, AnsiBlue+"(Codex)") {
		t.Errorf("vendor badge must never use semantic Red, Green, Yellow, or Blue hues in working state")
	}
}

func TestRenderAgentView_HumanMode_VendorBadge_OpenCode_WorkingState(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:opencode:ses-opencode-work",
		SessionID: "ses-opencode-work",
		Harness:   types.HarnessOpencode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "OpenCode working session",
			Action:    "Executing tool",
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// In working state, OpenCode badge uses distinct vendor brand styling (AnsiBold + AnsiCyan) (#106)
	expectedBadge := AnsiBold + AnsiCyan + "(OpenCode)" + AnsiReset
	if !strings.Contains(out, expectedBadge) {
		t.Errorf("expected harness badge to have OpenCode vendor brand styling %q in working state, got:\n%s", expectedBadge, out)
	}

	// Must strictly NOT contain Red, Green, Yellow, or Blue
	if strings.Contains(out, AnsiRed+"(OpenCode)") || strings.Contains(out, AnsiGreen+"(OpenCode)") || strings.Contains(out, AnsiYellow+"(OpenCode)") || strings.Contains(out, AnsiBlue+"(OpenCode)") {
		t.Errorf("vendor badge must never use semantic Red, Green, Yellow, or Blue hues in working state")
	}
}

func TestFormatHarnessBadge_RateLimited(t *testing.T) {
	badge := FormatHarnessBadge(types.HarnessClaudeCode, types.ActivityRateLimited)
	expected := AnsiYellow + "(Claude Code)" + AnsiReset
	if badge != expected {
		t.Errorf("FormatHarnessBadge(ClaudeCode, ActivityRateLimited) = %q, want %q", badge, expected)
	}
}

func TestRenderAgentView_RateLimited(t *testing.T) {
	rlMsg := "HTTP 429: Rate limit exceeded"
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-rl",
		SessionID: "ses-rl",
		Harness:   types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityRateLimited,
			Title:     "Rate limited session",
			Action:    "Rate limit reached — backoff/retry in progress",
			LastError: &rlMsg,
			UpdatedAt: 1000,
		},
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderAgentViewAt(&bufHuman, agent, false, 1000)
	humanOut := bufHuman.String()

	expectedCallout := AnsiYellow + GlyphWarn + " " + rlMsg + AnsiReset
	if !strings.Contains(humanOut, expectedCallout) {
		t.Errorf("expected human output to contain rate limit callout %q, got:\n%s", expectedCallout, humanOut)
	}
	expectedStatus := AnsiYellow + "Rate Limited" + AnsiReset
	if !strings.Contains(humanOut, expectedStatus) {
		t.Errorf("expected human output to contain status label %q, got:\n%s", expectedStatus, humanOut)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderAgentViewAt(&bufAgent, agent, true, 1000)
	agentOut := bufAgent.String()

	if !strings.Contains(agentOut, "activity:RATE_LIMITED") {
		t.Errorf("expected agent mode output to contain activity:RATE_LIMITED, got:\n%s", agentOut)
	}
	if !strings.Contains(agentOut, `rate_limit:"HTTP 429: Rate limit exceeded"`) {
		t.Errorf("expected agent mode output to contain rate_limit payload, got:\n%s", agentOut)
	}
}

func TestRenderAgentView_HumanMode_SemanticPrecedence_RateLimited(t *testing.T) {
	rlMsg := "Too Many Requests: Retry in 30s"
	agent := types.IAgentState{
		ID:        "env-1:pi:ses-rl-pi",
		SessionID: "ses-rl-pi",
		Harness:   types.HarnessPi,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityRateLimited,
			Title:     "Pi Rate Limited Session",
			Action:    "Waiting for backoff",
			LastError: &rlMsg,
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// 1. In RATE_LIMITED, semantic warning style (ANSI Yellow) must override Pi's vendor badge coloring
	expectedBadge := AnsiYellow + "(Pi)" + AnsiReset
	if !strings.Contains(out, expectedBadge) {
		t.Errorf("expected harness badge to have yellow warning style %q in RATE_LIMITED state, got:\n%s", expectedBadge, out)
	}
	// Vendor brand color (AnsiBold+AnsiMagenta, AnsiMagenta, or AnsiBlue) must be suppressed
	if strings.Contains(out, AnsiBold+AnsiMagenta+"(Pi)") || strings.Contains(out, AnsiMagenta+"(Pi)") || strings.Contains(out, AnsiBlue+"(Pi)") {
		t.Errorf("harness badge must not use vendor brand color during RATE_LIMITED state")
	}

	// 2. Callout must use prompt/warning style (ANSI Yellow)
	expectedCallout := AnsiYellow + GlyphWarn + " " + rlMsg + AnsiReset
	if !strings.Contains(out, expectedCallout) {
		t.Errorf("expected callout to have yellow warning style %q, got:\n%s", expectedCallout, out)
	}

	// 3. Status label must use ANSI Yellow
	expectedStatus := AnsiYellow + "Rate Limited" + AnsiReset
	if !strings.Contains(out, expectedStatus) {
		t.Errorf("expected status label %q in RATE_LIMITED state, got:\n%s", expectedStatus, out)
	}
}

func TestRenderAgentView_RateLimitedWithRemainingWait(t *testing.T) {
	nowMs := int64(1000000)
	resetAt := nowMs + 45000 // 45 seconds remaining
	retrySec := int64(45)

	agent := types.IAgentState{
		ID:        "env-1:pi:ses-rl-wait",
		SessionID: "ses-rl-wait",
		Harness:   types.HarnessPi,
		Activity: types.IAgentActivity{
			Lifecycle:         types.LifecycleRunning,
			Activity:          types.ActivityRateLimited,
			Title:             "Pi Throttled Session",
			Action:            "Rate limit wait",
			RateLimitResetAt:  &resetAt,
			RetryAfterSeconds: &retrySec,
			UpdatedAt:         nowMs,
		},
	}

	var bufHuman bytes.Buffer
	RenderAgentViewAt(&bufHuman, agent, false, nowMs)
	humanOut := bufHuman.String()

	if !strings.Contains(humanOut, "retry in 45s") && !strings.Contains(humanOut, "45s") {
		t.Errorf("expected human output to contain remaining wait time (45s), got:\n%s", humanOut)
	}

	var bufAgent bytes.Buffer
	RenderAgentViewAt(&bufAgent, agent, true, nowMs)
	agentOut := bufAgent.String()

	if !strings.Contains(agentOut, "activity:RATE_LIMITED") {
		t.Errorf("expected agent mode output to contain activity:RATE_LIMITED, got:\n%s", agentOut)
	}
	if !strings.Contains(agentOut, "45s") {
		t.Errorf("expected agent mode output to contain remaining wait time (45s), got:\n%s", agentOut)
	}
}

func TestFormatRateLimitWait(t *testing.T) {
	nowMs := int64(1000000)

	// 1. ResetAt in future
	futureReset := nowMs + 45000
	act1 := types.IAgentActivity{
		RateLimitResetAt: &futureReset,
	}
	if got := formatRateLimitWait(act1, nowMs); got != "retry in 45s" {
		t.Errorf("expected %q, got %q", "retry in 45s", got)
	}

	// 2. ResetAt in past with RetryAfterSeconds present -> must return "" (no fallthrough)
	pastReset := nowMs - 5000
	retrySec := int64(60)
	act2 := types.IAgentActivity{
		RateLimitResetAt:  &pastReset,
		RetryAfterSeconds: &retrySec,
		UpdatedAt:         nowMs,
	}
	if got := formatRateLimitWait(act2, nowMs); got != "" {
		t.Errorf("expected empty string when resetAt has passed, got %q", got)
	}

	// 3. Only RetryAfterSeconds present, partial elapsed
	act3 := types.IAgentActivity{
		RetryAfterSeconds: &retrySec,
		UpdatedAt:         nowMs - 15000, // 15s elapsed
	}
	if got := formatRateLimitWait(act3, nowMs); got != "retry in 45s" {
		t.Errorf("expected %q, got %q", "retry in 45s", got)
	}

	// 4. Only RetryAfterSeconds present, fully elapsed
	shortSec := int64(30)
	act4 := types.IAgentActivity{
		RetryAfterSeconds: &shortSec,
		UpdatedAt:         nowMs - 45000, // 45s elapsed (> 30s)
	}
	if got := formatRateLimitWait(act4, nowMs); got != "" {
		t.Errorf("expected empty string when retryAfter has passed, got %q", got)
	}

	// 5. Boundary: exactly 120s remaining -> "retry in 120s"
	reset120 := nowMs + 120000
	act5 := types.IAgentActivity{
		RateLimitResetAt: &reset120,
	}
	if got := formatRateLimitWait(act5, nowMs); got != "retry in 120s" {
		t.Errorf("expected %q, got %q", "retry in 120s", got)
	}

	// 6. ResetAt > 120s: 125s -> "retry in 2m 5s"
	reset125 := nowMs + 125000
	act6 := types.IAgentActivity{
		RateLimitResetAt: &reset125,
	}
	if got := formatRateLimitWait(act6, nowMs); got != "retry in 2m 5s" {
		t.Errorf("expected %q, got %q", "retry in 2m 5s", got)
	}

	// 7. ResetAt > 120s: 300s (5m) -> "retry in 5m"
	reset300 := nowMs + 300000
	act7 := types.IAgentActivity{
		RateLimitResetAt: &reset300,
	}
	if got := formatRateLimitWait(act7, nowMs); got != "retry in 5m" {
		t.Errorf("expected %q, got %q", "retry in 5m", got)
	}

	// 8. ResetAt > 120s: 3600s (1h) -> "retry in 1h"
	reset3600 := nowMs + 3600000
	act8 := types.IAgentActivity{
		RateLimitResetAt: &reset3600,
	}
	if got := formatRateLimitWait(act8, nowMs); got != "retry in 1h" {
		t.Errorf("expected %q, got %q", "retry in 1h", got)
	}

	// 9. ResetAt > 120s: 3660s (1h 1m) -> "retry in 1h 1m"
	reset3660 := nowMs + 3660000
	act9 := types.IAgentActivity{
		RateLimitResetAt: &reset3660,
	}
	if got := formatRateLimitWait(act9, nowMs); got != "retry in 1h 1m" {
		t.Errorf("expected %q, got %q", "retry in 1h 1m", got)
	}

	// 10. RetryAfterSeconds > 120s: 300s (5m) -> "retry in 5m"
	retrySec300 := int64(300)
	act10 := types.IAgentActivity{
		RetryAfterSeconds: &retrySec300,
		UpdatedAt:         nowMs,
	}
	if got := formatRateLimitWait(act10, nowMs); got != "retry in 5m" {
		t.Errorf("expected %q, got %q", "retry in 5m", got)
	}

	// 11. RetryAfterSeconds > 120s: 3600s (1h) -> "retry in 1h"
	retrySec3600 := int64(3600)
	act11 := types.IAgentActivity{
		RetryAfterSeconds: &retrySec3600,
		UpdatedAt:         nowMs,
	}
	if got := formatRateLimitWait(act11, nowMs); got != "retry in 1h" {
		t.Errorf("expected %q, got %q", "retry in 1h", got)
	}
}

func TestRenderAgentView_RateLimitedFallbackMessage(t *testing.T) {
	nowMs := int64(1000000)
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-rl-fallback",
		SessionID: "ses-rl-fallback",
		Harness:   types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityRateLimited,
			Title:     "Rate Limited Agent",
			Action:    "Rate limit reached — backoff/retry in progress",
			UpdatedAt: nowMs,
		},
	}

	// Human mode without wait
	var bufHuman bytes.Buffer
	RenderAgentViewAt(&bufHuman, agent, false, nowMs)
	humanOut := bufHuman.String()
	if !strings.Contains(humanOut, "Rate limit reached — backoff/retry in progress") {
		t.Errorf("expected human output to contain fallback message with em-dash, got:\n%s", humanOut)
	}

	// Agent mode without wait
	var bufAgent bytes.Buffer
	RenderAgentViewAt(&bufAgent, agent, true, nowMs)
	agentOut := bufAgent.String()
	if !strings.Contains(agentOut, `rate_limit:"Rate limit reached — backoff/retry in progress"`) {
		t.Errorf("expected agent mode to contain aligned fallback rate limit, got:\n%s", agentOut)
	}

	// With future resetAt
	futureReset := nowMs + 30000
	agent.Activity.RateLimitResetAt = &futureReset

	var bufHumanWait bytes.Buffer
	RenderAgentViewAt(&bufHumanWait, agent, false, nowMs)
	humanWaitOut := bufHumanWait.String()
	if !strings.Contains(humanWaitOut, "Rate limit reached — retry in 30s") {
		t.Errorf("expected human wait output to contain em-dash retry, got:\n%s", humanWaitOut)
	}

	var bufAgentWait bytes.Buffer
	RenderAgentViewAt(&bufAgentWait, agent, true, nowMs)
	agentWaitOut := bufAgentWait.String()
	if !strings.Contains(agentWaitOut, `rate_limit:"Rate limit reached — retry in 30s"`) {
		t.Errorf("expected agent wait output to contain em-dash retry, got:\n%s", agentWaitOut)
	}

	// With future resetAt > 120s (e.g., 5m)
	futureReset5m := nowMs + 300000
	agent.Activity.RateLimitResetAt = &futureReset5m

	var bufHumanWait5m bytes.Buffer
	RenderAgentViewAt(&bufHumanWait5m, agent, false, nowMs)
	humanWait5mOut := bufHumanWait5m.String()
	if !strings.Contains(humanWait5mOut, "Rate limit reached — retry in 5m") {
		t.Errorf("expected human wait output to contain 'retry in 5m', got:\n%s", humanWait5mOut)
	}

	var bufAgentWait5m bytes.Buffer
	RenderAgentViewAt(&bufAgentWait5m, agent, true, nowMs)
	agentWait5mOut := bufAgentWait5m.String()
	if !strings.Contains(agentWait5mOut, `rate_limit:"Rate limit reached — retry in 5m"`) {
		t.Errorf("expected agent wait output to contain 'retry in 5m', got:\n%s", agentWait5mOut)
	}
}

func TestFormatHumanDuration(t *testing.T) {
	tests := []struct {
		name string
		ms   int64
		want string
	}{
		{"seconds", 45000, "45s"},
		{"single second", 5000, "5s"},
		{"exact minute", 60000, "1m"},
		{"minutes and seconds", 75000, "1m 15s"},
		{"minutes and seconds 2m 15s", 135000, "2m 15s"},
		{"minutes and seconds 3m 22s", 202000, "3m 22s"},
		{"exact 5 minutes", 300000, "5m"},
		{"exact 10 minutes", 600000, "10m"},
		{"exact hour", 3600000, "1h"},
		{"hours and minutes 1h 1m", 3660000, "1h 1m"},
		{"exact 2 hours", 7200000, "2h"},
		{"hours and minutes 2h 15m", 8100000, "2h 15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatHumanDuration(tt.ms); got != tt.want {
				t.Errorf("formatHumanDuration(%d) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}

func TestRenderAgentView_ColorGating_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	errMsg := "something failed"
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-nc",
		SessionID: "ses-nc",
		Harness:   types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityError,
			Title:     "Failing Agent",
			Action:    "Retrying",
			LastError: &errMsg,
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000)
	out := buf.String()

	// When NO_COLOR is set, output must not contain any ANSI escape sequences
	if StripANSI(out) != out {
		t.Errorf("expected no ANSI escape sequences when NO_COLOR=1, but output has ANSI:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output must not contain escape code \\x1b[ when NO_COLOR=1")
	}
}

func TestRenderAgentView_ColorGating_NonTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	errMsg := "something failed"
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-nontty",
		SessionID: "ses-nontty",
		Harness:   types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityError,
			Title:     "Failing Agent",
			Action:    "Retrying",
			LastError: &errMsg,
			UpdatedAt: 1000,
		},
	}

	// Render into non-TTY pipe writer
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	RenderAgentViewAt(w, agent, false, 1000)
	_ = w.Close()

	out := <-done

	// When writing to a non-TTY pipe, output must not contain any ANSI escape sequences
	if StripANSI(out) != out {
		t.Errorf("expected no ANSI escape sequences when writing to non-TTY pipe, but output has ANSI:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output must not contain escape code \\x1b[ when writing to non-TTY pipe")
	}
}

func TestFormatHarnessBadge_ProhibitsReservedSemanticColors(t *testing.T) {
	// Custom or unknown harness should never be assigned Red, Green, Yellow, or Blue
	badge := FormatHarnessBadge(types.HarnessType("custom-harness"), types.ActivityBusy)
	if strings.Contains(badge, AnsiRed) || strings.Contains(badge, AnsiGreen) || strings.Contains(badge, AnsiYellow) || strings.Contains(badge, AnsiBlue) {
		t.Errorf("FormatHarnessBadge must never assign reserved semantic color, got: %q", badge)
	}

	// Prohibited semantic inputs must be sanitized in working state
	for _, semanticColor := range []string{AnsiRed, AnsiGreen, AnsiYellow, AnsiBlue, AnsiBold + AnsiRed, AnsiBold + AnsiYellow} {
		prohibitedHarness := types.HarnessType(semanticColor + "EvilHarness" + AnsiReset)
		sanitized := FormatHarnessBadge(prohibitedHarness, types.ActivityBusy)
		if strings.Contains(sanitized, semanticColor) {
			t.Errorf("FormatHarnessBadge must sanitize prohibited color %q from vendor input in working state, got: %q", semanticColor, sanitized)
		}
	}
}

func TestGetIdleFreshness(t *testing.T) {
	nowMs := int64(10000000)
	tests := []struct {
		name      string
		updatedAt int64
		want      IdleFreshness
	}{
		{"fresh recent", nowMs - 5*60*1000, FreshnessFresh},
		{"fresh boundary", nowMs - (15*60*1000 - 1), FreshnessFresh},
		{"standard at 15m", nowMs - 15*60*1000, FreshnessStandard},
		{"standard mid", nowMs - 60*60*1000, FreshnessStandard},
		{"standard boundary", nowMs - (2*60*60*1000 - 1), FreshnessStandard},
		{"stale at 2h", nowMs - 2*60*60*1000, FreshnessStale},
		{"stale older", nowMs - 5*60*60*1000, FreshnessStale},
		{"stale zero", 0, FreshnessStale},
		{"stale negative", -100, FreshnessStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetIdleFreshness(tt.updatedAt, nowMs)
			if got != tt.want {
				t.Errorf("GetIdleFreshness(%d, %d) = %v, want %v", tt.updatedAt, nowMs, got, tt.want)
			}
		})
	}
}

func TestRenderAgentView_HumanMode_IdleStaleDimmed(t *testing.T) {
	agent := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			Title:     "Dormant session",
			Action:    "Idle",
			UpdatedAt: 1000000,
		},
	}

	var buf bytes.Buffer
	// 3 hours later
	nowMs := int64(1000000 + 3*60*60*1000)
	RenderAgentViewAt(&buf, agent, false, nowMs)
	out := buf.String()

	if !strings.Contains(out, AnsiDim) {
		t.Errorf("expected AnsiDim in human mode view for stale idle session (>2h), got:\n%s", out)
	}
}

func TestRenderAgentView_HumanMode_IdleFreshNotDimmed(t *testing.T) {
	agent := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			Title:     "Recent idle session",
			Action:    "Idle",
			UpdatedAt: 1000000,
		},
	}

	var buf bytes.Buffer
	// 5 minutes later
	nowMs := int64(1000000 + 5*60*1000)
	RenderAgentViewAt(&buf, agent, false, nowMs)
	out := buf.String()

	if strings.Contains(out, AnsiDim) {
		t.Errorf("expected no AnsiDim in human mode view for fresh idle session (<15m), got:\n%s", out)
	}
}

func TestRenderAgentView_HumanMode_BusyNotDimmed(t *testing.T) {
	agent := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Long running busy session",
			Action:    "Running build",
			UpdatedAt: 1000000,
		},
	}

	var buf bytes.Buffer
	// 3 hours later
	nowMs := int64(1000000 + 3*60*60*1000)
	RenderAgentViewAt(&buf, agent, false, nowMs)
	out := buf.String()

	if strings.Contains(out, AnsiDim) {
		t.Errorf("expected no AnsiDim in human mode view for busy session even if age > 2h, got:\n%s", out)
	}
}

func TestRenderFleetView_Empty(t *testing.T) {
	var bufHuman bytes.Buffer
	RenderFleetView(&bufHuman, nil, false)
	if !strings.Contains(bufHuman.String(), "No active or recent agent runs detected.") {
		t.Errorf("expected empty message in human mode, got: %s", bufHuman.String())
	}

	var bufAgent bytes.Buffer
	RenderFleetView(&bufAgent, []types.IAgentState{}, true)
	if bufAgent.Len() != 0 {
		t.Errorf("expected empty output in agent mode, got: %s", bufAgent.String())
	}
}

func TestRenderFleetView_HumanMode_HierarchicalWorktrees(t *testing.T) {
	relPathAuth := ".workspaces/feat-auth"
	relPathUI := ".workspaces/feat-ui"

	root1 := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:   "github.com/alexgorbatchev/agent-status",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: false,
			Branch:     "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "agent-status: Fix CI build failure",
		},
	}
	root2 := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:   "github.com/alexgorbatchev/agent-status",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: false,
			Branch:     "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Refactor subscriber reconnect",
		},
	}
	wt1 := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:   "github.com/alexgorbatchev/agent-status",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: true,
			Branch:     "feat-auth",
			RelPath:    &relPathAuth,
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Implement OAuth login flow",
		},
	}
	wt2 := types.IAgentState{
		Harness: types.HarnessPi,
		Project: types.IProjectInfo{
			ID:   "github.com/alexgorbatchev/agent-status",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: true,
			Branch:     "feat-ui",
			RelPath:    &relPathUI,
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			Title:     "Tweak dashboard styling",
		},
	}

	agents := []types.IAgentState{root1, root2, wt1, wt2}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, agents, false, 1000)
	out := buf.String()
	clean := StripANSI(out)

	// Check parent project header
	if !strings.Contains(clean, "v agent-status (github.com/alexgorbatchev/agent-status)") {
		t.Errorf("expected project header in fleet view, got:\n%s", clean)
	}

	// Root branch should be first child branch with |-
	if !strings.Contains(clean, "|- [main] (2 sessions)") {
		t.Errorf("expected root branch header with |-, got:\n%s", clean)
	}

	// Middle worktree should have |-
	if !strings.Contains(clean, "|- [worktree: feat-auth] (.workspaces/feat-auth) (1 session)") {
		t.Errorf("expected wt1 branch header with |-, got:\n%s", clean)
	}

	// Last worktree should have +-
	if !strings.Contains(clean, "+- [worktree: feat-ui] (.workspaces/feat-ui) (1 session)") {
		t.Errorf("expected wt2 branch header with +-, got:\n%s", clean)
	}

	// Child sessions should be indented and prefix trimmed
	if !strings.Contains(clean, "Fix CI build failure") {
		t.Errorf("expected trimmed session title in output, got:\n%s", clean)
	}
	if !strings.Contains(clean, "Implement OAuth login flow") {
		t.Errorf("expected worktree session title in output, got:\n%s", clean)
	}
}

func TestRenderFleetView_HumanMode_OnlyWorktrees(t *testing.T) {
	relPathAuth := ".workspaces/feat-auth"
	wt := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:   "github.com/alexgorbatchev/agent-status",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: true,
			Branch:     "feat-auth",
			RelPath:    &relPathAuth,
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Implement OAuth login flow",
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{wt}, false, 1000)
	clean := StripANSI(buf.String())

	if strings.Contains(clean, "[main]") {
		t.Errorf("did not expect [main] branch when no root agents exist, got:\n%s", clean)
	}
	if !strings.Contains(clean, "+- [worktree: feat-auth] (.workspaces/feat-auth) (1 session)") {
		t.Errorf("expected single worktree branch to have +-, got:\n%s", clean)
	}
}

func TestRenderFleetView_AgentMode(t *testing.T) {
	relPathAuth := ".workspaces/feat-auth"
	root := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:   "proj-1",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: false,
			Branch:     "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Root session",
		},
	}
	wt := types.IAgentState{
		Harness: types.HarnessPi,
		Project: types.IProjectInfo{
			ID:   "proj-1",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: true,
			Branch:     "feat-auth",
			RelPath:    &relPathAuth,
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "WT session",
		},
	}

	var buf bytes.Buffer
	RenderFleetView(&buf, []types.IAgentState{root, wt}, true)
	out := buf.String()

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines in agent mode, got %d:\n%s", len(lines), out)
	}

	if !strings.Contains(lines[0], "project:proj-1") || !strings.Contains(lines[0], "is_worktree:false") {
		t.Errorf("expected line 0 to have project and is_worktree:false, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "project:proj-1") || !strings.Contains(lines[1], "is_worktree:true") || !strings.Contains(lines[1], "worktree:.workspaces/feat-auth") {
		t.Errorf("expected line 1 to have project, is_worktree:true, and worktree path, got: %s", lines[1])
	}
}

func TestRenderAgentView_HumanMode_Completed(t *testing.T) {
	tVal := true
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-completed",
		SessionID: "ses-completed",
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
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle:           types.LifecycleRunning,
			Activity:            types.ActivityIdle,
			Title:               "Completed work turn",
			Action:              "Finished processing",
			HasUnseenCompletion: &tVal,
			UpdatedAt:           1000000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 1000000+5000)
	out := buf.String()

	if !strings.Contains(StripANSI(out), "(Completed)") {
		t.Errorf("expected header to contain (Completed) for unacknowledged completion, got:\n%s", out)
	}
}

func TestRenderAgentView_AgentMode_Completed(t *testing.T) {
	tVal := true
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-completed",
		SessionID: "ses-completed",
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
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle:           types.LifecycleRunning,
			Activity:            types.ActivityIdle,
			Title:               "Completed work turn",
			Action:              "Finished processing",
			HasUnseenCompletion: &tVal,
			UpdatedAt:           1000000,
		},
	}

	var buf bytes.Buffer
	RenderAgentView(&buf, agent, true)
	line := strings.TrimSpace(buf.String())

	if !strings.Contains(line, "activity:COMPLETED") {
		t.Errorf("expected activity:COMPLETED in agent mode, got:\n%s", line)
	}
	if !strings.Contains(line, "has_unseen_completion:true") {
		t.Errorf("expected has_unseen_completion:true in agent mode, got:\n%s", line)
	}
}

func TestRenderFleetView_HumanMode_Completed(t *testing.T) {
	tVal := true
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-completed",
		SessionID: "ses-completed",
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
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle:           types.LifecycleRunning,
			Activity:            types.ActivityIdle,
			Title:               "Completed work turn",
			Action:              "Finished processing",
			HasUnseenCompletion: &tVal,
			UpdatedAt:           1000000,
		},
	}

	var buf bytes.Buffer
	RenderFleetView(&buf, []types.IAgentState{agent}, false)
	out := buf.String()

	if !strings.Contains(out, "(Completed)") {
		t.Errorf("expected fleet view tree to display (Completed) for unacknowledged completion, got:\n%s", out)
	}
}

func TestRenderFleetView_HumanMode_IdleStaleDimmed(t *testing.T) {
	nowMs := int64(10_000_000)
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-stale",
		SessionID: "ses-stale",
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
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			Title:     "Old idle work",
			UpdatedAt: nowMs - 3*60*60*1000, // 3 hours ago (>2h)
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{agent}, false, nowMs)
	out := buf.String()

	if !strings.Contains(out, AnsiDim) {
		t.Errorf("expected AnsiDim in fleet view for stale idle session (>2h), got:\n%s", out)
	}
}

func TestRenderFleetView_HumanMode_IdleFreshNotDimmed(t *testing.T) {
	nowMs := int64(10_000_000)
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-fresh",
		SessionID: "ses-fresh",
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
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			Title:     "Recent idle work",
			UpdatedAt: nowMs - 5*60*1000, // 5 minutes ago (<15m)
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{agent}, false, nowMs)
	out := buf.String()

	if strings.Contains(out, AnsiDim) {
		t.Errorf("expected no AnsiDim in fleet view for fresh idle session (<15m), got:\n%s", out)
	}
}

func TestRenderFleetView_HumanMode_IdleStaleWithUnseenCompletionNotDimmed(t *testing.T) {
	nowMs := int64(10_000_000)
	tVal := true
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-completed-stale",
		SessionID: "ses-completed-stale",
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
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle:           types.LifecycleRunning,
			Activity:            types.ActivityIdle,
			Title:               "Completed work turn",
			HasUnseenCompletion: &tVal,
			UpdatedAt:           nowMs - 3*60*60*1000, // 3 hours ago, but unacknowledged
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{agent}, false, nowMs)
	out := buf.String()

	if strings.Contains(out, AnsiDim) {
		t.Errorf("expected no AnsiDim in fleet view for completed session even if >2h old, got:\n%s", out)
	}
}

func TestRenderFleetView_HumanMode_NoWorktrees_RendersFlat(t *testing.T) {
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-root",
		SessionID: "ses-root",
		Harness:   types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:   "proj-flat",
			Name: "flat-project",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     "main",
			IsWorktree: false,
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Flat session without worktrees",
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{agent}, false, 1000)
	clean := StripANSI(buf.String())

	if strings.Contains(clean, "[main]") {
		t.Errorf("expected flat rendering without [main] header when project has no worktrees, got:\n%s", clean)
	}
	if strings.Contains(clean, "+- ") || strings.Contains(clean, "|- ") {
		t.Errorf("expected flat rendering without branch tree connectors (+- / |-), got:\n%s", clean)
	}
	if !strings.Contains(clean, "Flat session without worktrees") {
		t.Errorf("expected agent session title in output, got:\n%s", clean)
	}
}

func TestRenderFleetView_HumanMode_ActivityStatusFormatting(t *testing.T) {
	exitedAgent := types.IAgentState{
		ID:        "env-1:claude-code:ses-exited",
		SessionID: "ses-exited",
		Harness:   types.HarnessClaudeCode,
		Project:   types.IProjectInfo{ID: "proj-1", Name: "my-project"},
		Workspace: types.IWorkspaceInfo{Branch: "main", IsWorktree: false},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			Title:     "Exited agent",
		},
	}
	waitingAgent := types.IAgentState{
		ID:        "env-1:claude-code:ses-wait",
		SessionID: "ses-wait",
		Harness:   types.HarnessClaudeCode,
		Project:   types.IProjectInfo{ID: "proj-1", Name: "my-project"},
		Workspace: types.IWorkspaceInfo{Branch: "main", IsWorktree: false},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityWaitingQuestion,
			Title:     "Waiting agent",
		},
	}
	busyAgent := types.IAgentState{
		ID:        "env-1:claude-code:ses-busy",
		SessionID: "ses-busy",
		Harness:   types.HarnessClaudeCode,
		Project:   types.IProjectInfo{ID: "proj-1", Name: "my-project"},
		Workspace: types.IWorkspaceInfo{Branch: "main", IsWorktree: false},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Busy agent",
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{exitedAgent, waitingAgent, busyAgent}, false, 1000)
	clean := StripANSI(buf.String())

	if !strings.Contains(clean, "(Finished)") {
		t.Errorf("expected EXITED agent to format as (Finished), got:\n%s", clean)
	}
	if !strings.Contains(clean, "(Waiting on Question)") {
		t.Errorf("expected WAITING_QUESTION agent to format as (Waiting on Question), got:\n%s", clean)
	}
	if !strings.Contains(clean, "(Working)") {
		t.Errorf("expected BUSY agent to format as (Working), got:\n%s", clean)
	}
}

func TestRenderFleetView_HumanMode_SortingWithinBranch(t *testing.T) {
	relPath := "wt"
	agents := []types.IAgentState{
		{
			ID:        "a-exited",
			SessionID: "s-exited",
			Harness:   types.HarnessClaudeCode,
			Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
			Workspace: types.IWorkspaceInfo{IsWorktree: true, RelPath: &relPath},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleExited,
				Activity:  types.ActivityIdle,
				Title:     "Exited task",
				UpdatedAt: 5000,
			},
		},
		{
			ID:        "a-idle-older",
			SessionID: "s-idle-older",
			Harness:   types.HarnessClaudeCode,
			Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
			Workspace: types.IWorkspaceInfo{IsWorktree: true, RelPath: &relPath},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityIdle,
				Title:     "Idle older task",
				UpdatedAt: 1000,
			},
		},
		{
			ID:        "a-idle-newer",
			SessionID: "s-idle-newer",
			Harness:   types.HarnessClaudeCode,
			Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
			Workspace: types.IWorkspaceInfo{IsWorktree: true, RelPath: &relPath},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityIdle,
				Title:     "Idle newer task",
				UpdatedAt: 3000,
			},
		},
		{
			ID:        "a-busy",
			SessionID: "s-busy",
			Harness:   types.HarnessClaudeCode,
			Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
			Workspace: types.IWorkspaceInfo{IsWorktree: true, RelPath: &relPath},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityBusy,
				Title:     "Busy task",
				UpdatedAt: 2000,
			},
		},
		{
			ID:        "a-waiting",
			SessionID: "s-waiting",
			Harness:   types.HarnessClaudeCode,
			Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
			Workspace: types.IWorkspaceInfo{IsWorktree: true, RelPath: &relPath},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityWaitingQuestion,
				Title:     "Waiting task",
				UpdatedAt: 4000,
			},
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, agents, false, 10000)
	clean := StripANSI(buf.String())

	idxBusy := strings.Index(clean, "Busy task")
	idxWaiting := strings.Index(clean, "Waiting task")
	idxIdleNewer := strings.Index(clean, "Idle newer task")
	idxIdleOlder := strings.Index(clean, "Idle older task")
	idxExited := strings.Index(clean, "Exited task")

	// In the 3 canonical categories (busy: 0, idle: 1, finished: 2) sorted by UpdatedAt DESC:
	// - Category "busy": Waiting task (updatedAt 4000) < Busy task (updatedAt 2000)
	// - Category "idle": Idle newer task (updatedAt 3000) < Idle older task (updatedAt 1000)
	// - Category "finished": Exited task (updatedAt 5000)
	if idxWaiting >= idxBusy || idxBusy >= idxIdleNewer || idxIdleNewer >= idxIdleOlder || idxIdleOlder >= idxExited {
		t.Errorf("expected sorting: waiting (busy category, newer) < busy (busy category, older) < idle-newer < idle-older < exited; got indices waiting=%d busy=%d idleNewer=%d idleOlder=%d exited=%d\nOutput:\n%s",
			idxWaiting, idxBusy, idxIdleNewer, idxIdleOlder, idxExited, clean)
	}
}

func TestRenderFleetView_WorktreeKeyFallback_CWD(t *testing.T) {
	agent := types.IAgentState{
		ID:        "a-wt",
		SessionID: "s-wt",
		Harness:   types.HarnessClaudeCode,
		Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: true,
			Branch:     "",
			RelPath:    nil,
			CWD:        "/path/to/worktree-dir",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "CWD fallback task",
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{agent}, false, 1000)
	clean := StripANSI(buf.String())

	if !strings.Contains(clean, "[worktree: HEAD]") {
		t.Errorf("expected worktree branch label to fall back to HEAD, got:\n%s", clean)
	}
}

func TestRenderFleetView_RootBranchLabelFallback_HEAD(t *testing.T) {
	relPath := "wt"
	root := types.IAgentState{
		ID:        "root",
		SessionID: "s-root",
		Harness:   types.HarnessClaudeCode,
		Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: false,
			Branch:     "", // empty branch
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Root task",
		},
	}
	wt := types.IAgentState{
		ID:        "wt",
		SessionID: "s-wt",
		Harness:   types.HarnessClaudeCode,
		Project:   types.IProjectInfo{ID: "p1", Name: "proj"},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: true,
			Branch:     "feat",
			RelPath:    &relPath,
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "WT task",
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{root, wt}, false, 1000)
	clean := StripANSI(buf.String())

	if !strings.Contains(clean, "[HEAD]") {
		t.Errorf("expected root branch label to fall back to [HEAD], got:\n%s", clean)
	}
	if strings.Contains(clean, "[main]") {
		t.Errorf("did not expect [main] fallback when branch is empty, got:\n%s", clean)
	}
}

func TestRenderFleetView_AgentMode_NoDuplicateWorktree(t *testing.T) {
	relPath := ".workspaces/feat-auth"
	wt := types.IAgentState{
		Harness: types.HarnessPi,
		Project: types.IProjectInfo{
			ID:   "proj-1",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: true,
			Branch:     "feat-auth",
			RelPath:    &relPath,
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "WT session",
		},
	}

	var buf bytes.Buffer
	RenderFleetView(&buf, []types.IAgentState{wt}, true)
	line := strings.TrimSpace(buf.String())

	count := strings.Count(line, " worktree:")
	if count != 1 {
		t.Errorf("expected ' worktree:' to appear exactly once per line, appeared %d times in: %s", count, line)
	}
}


func TestRenderAgentView_HumanMode_CardLinesBoundedToTerminalWidth(t *testing.T) {
	longTitle := strings.Repeat("Very long title with lots of words explaining the task in full detail ", 6)
	longAction := strings.Repeat("Executing very long tool with multiple arguments and paths that exceed width ", 4)
	longQuestion := strings.Repeat("Would you like to apply this massive changeset to the codebase right now? ", 4)
	longRelPath := strings.Repeat("very/long/worktree/relative/path/segment/", 5)

	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-overflow",
		SessionID: "ses-overflow",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			Label: strings.Repeat("custom-long-environment-label-", 3),
		},
		Project: types.IProjectInfo{
			ID:   "proj-overflow",
			Name: "my-project",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     strings.Repeat("feature/very-long-branch-name-with-details-", 3),
			IsWorktree: true,
			RelPath:    &longRelPath,
		},
		Activity: types.IAgentActivity{
			Lifecycle:     types.LifecycleRunning,
			Activity:      types.ActivityWaitingQuestion,
			Title:         longTitle,
			Action:        longAction,
			Question:      &longQuestion,
			SubagentCount: 5,
			ActiveSubagents: []types.ISubagentSummary{
				{
					ID:     "sub-1",
					Action: "Subagent running a very long nested task description",
				},
			},
			UpdatedAt: 1000,
			Cost: &types.ICostInfo{
				Total: 12345.6789,
			},
			Usage: types.ITokenUsage{
				Total: 9999999999,
			},
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 2000)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	termWidth := GetTerminalWidth()
	if termWidth < 40 {
		termWidth = 80
	}

	for i, line := range lines {
		if !utf8.ValidString(line) {
			t.Errorf("line %d is not valid UTF-8: %s", i, line)
		}
		w := VisualWidth(line)
		if w > termWidth {
			t.Errorf("line %d has visual width %d, exceeds terminal width %d: %q", i, w, termWidth, line)
		}
	}
}

func TestRenderFleetViewAt_BoundsToTerminalWidth(t *testing.T) {
	longRelPath := strings.Repeat("worktree/nested/path/to/feature-branch-details/", 4)
	agents := []types.IAgentState{
		{
			ID:        "env-1:claude-code:ses-long-1",
			SessionID: "ses-long-1",
			Harness:   types.HarnessClaudeCode,
			Environment: types.IEnvironmentInfo{
				Label: "production-workstation-environment",
			},
			Project: types.IProjectInfo{
				ID:   strings.Repeat("organization-project-repository-identifier-", 4),
				Name: strings.Repeat("Very Long Project Name For Enterprise Testing ", 4),
			},
			Workspace: types.IWorkspaceInfo{
				Branch:     strings.Repeat("feature/very-long-branch-name-with-multiple-segments-", 3),
				IsWorktree: false,
			},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityBusy,
				Title:     strings.Repeat("Investigating complex memory leak across distributed nodes ", 4),
				Action:    "Profiling heap allocations and tracing goroutine leaks",
				UpdatedAt: 1000,
			},
		},
		{
			ID:        "env-1:pi:ses-long-2",
			SessionID: "ses-long-2",
			Harness:   types.HarnessPi,
			Environment: types.IEnvironmentInfo{
				Label: "production-workstation-environment",
			},
			Project: types.IProjectInfo{
				ID:   strings.Repeat("organization-project-repository-identifier-", 4),
				Name: strings.Repeat("Very Long Project Name For Enterprise Testing ", 4),
			},
			Workspace: types.IWorkspaceInfo{
				Branch:     strings.Repeat("feature/worktree-branch-with-lengthy-description-", 3),
				IsWorktree: true,
				RelPath:    &longRelPath,
			},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityIdle,
				Title:     strings.Repeat("Long worktree task title with lots of words ", 5),
				UpdatedAt: 1000,
			},
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, agents, false, 2000)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	termWidth := GetTerminalWidth()
	if termWidth <= 0 {
		termWidth = 80
	}

	for i, line := range lines {
		if line == "" {
			continue
		}
		if !utf8.ValidString(line) {
			t.Errorf("line %d is not valid UTF-8: %s", i, line)
		}
		w := VisualWidth(line)
		if w > termWidth {
			t.Errorf("line %d has visual width %d, exceeds terminal width %d: %q", i, w, termWidth, line)
		}
	}
}

func TestRenderAgentView_HumanMode_SubagentActionNewlines(t *testing.T) {
	agent := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle:     types.LifecycleRunning,
			Activity:      types.ActivityBusy,
			Title:         "Session with subagent action containing newlines",
			SubagentCount: 1,
			ActiveSubagents: []types.ISubagentSummary{
				{
					ID:     "sub-multiline",
					Action: "First line of action\nSecond line of action\r\nThird line",
				},
			},
			UpdatedAt: 1000,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, false, 2000)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// Badge line containing subagent should not be split across multiple broken lines
	foundSubagentBadge := false
	for _, line := range lines {
		if strings.Contains(line, "sub-multiline") {
			foundSubagentBadge = true
			if !strings.Contains(line, "(subagent sub-multiline: First line of action Second line of action Third line)") {
				t.Errorf("expected newlines normalized to spaces in subagent badge, got line: %q", line)
			}
		}
	}
	if !foundSubagentBadge {
		t.Errorf("expected to find subagent badge in rendered output:\n%s", out)
	}
}

func TestCardRendering_EastAsianWidthConditions(t *testing.T) {
	defer func() {
		runewidth.DefaultCondition.EastAsianWidth = false
	}()

	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-cjk",
		SessionID: "ses-cjk",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			Label: "開発・検証用マシン",
		},
		Project: types.IProjectInfo{
			ID:   "cjk-project",
			Name: "日本語プロジェクト",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "フィーチャー/新機能",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "分散キャッシュ機構の再実装およびベンチマークテスト",
			Action:    "Redisクラスタ接続の最適化を実行中",
			UpdatedAt: 1000,
			Cost: &types.ICostInfo{
				Total: 0.0543,
			},
			Usage: types.ITokenUsage{
				Total: 54321,
			},
		},
	}

	for _, eastAsian := range []bool{false, true} {
		conditionName := "default (EastAsianWidth=false)"
		if eastAsian {
			conditionName = "east asian (EastAsianWidth=true)"
		}

		t.Run(conditionName, func(t *testing.T) {
			runewidth.DefaultCondition.EastAsianWidth = eastAsian

			var buf bytes.Buffer
			RenderAgentViewAt(&buf, agent, false, 2000)
			out := buf.String()

			if !utf8.ValidString(out) {
				t.Fatalf("RenderAgentViewAt produced invalid UTF-8 under %s:\n%s", conditionName, out)
			}

			lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
			termWidth := GetTerminalWidth()
			if termWidth < 40 {
				termWidth = 80
			}

			for i, line := range lines {
				w := VisualWidth(line)
				if w > termWidth {
					t.Errorf("line %d visual width = %d, exceeds termWidth %d under %s: %q", i, w, termWidth, conditionName, line)
				}
			}

			// Also test fleet view
			var fleetBuf bytes.Buffer
			RenderFleetViewAt(&fleetBuf, []types.IAgentState{agent}, false, 2000)
			fleetOut := fleetBuf.String()

			if !utf8.ValidString(fleetOut) {
				t.Fatalf("RenderFleetViewAt produced invalid UTF-8 under %s:\n%s", conditionName, fleetOut)
			}

			fleetLines := strings.Split(strings.TrimRight(fleetOut, "\n"), "\n")
			for i, line := range fleetLines {
				if line == "" {
					continue
				}
				w := VisualWidth(line)
				if w > termWidth {
					t.Errorf("fleet line %d visual width = %d, exceeds termWidth %d under %s: %q", i, w, termWidth, conditionName, line)
				}
			}
		})
	}
}

func TestRenderFleetView_HumanMode_ExitedStaleNotDimmed(t *testing.T) {
	nowMs := int64(10_000_000)
	agent := types.IAgentState{
		ID:        "env-1:claude-code:ses-exited-stale",
		SessionID: "ses-exited-stale",
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
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			Title:     "Old completed work",
			UpdatedAt: nowMs - 3*60*60*1000, // 3 hours ago (>2h)
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, []types.IAgentState{agent}, false, nowMs)
	out := buf.String()

	if strings.Contains(out, AnsiDim) {
		t.Errorf("expected no AnsiDim in fleet view for exited session, got:\n%s", out)
	}
}

func TestRenderFleetView_SharedHierarchyFixture(t *testing.T) {
	fixtureData, err := os.ReadFile("../../../../packages/agent-common/testdata/fleet-hierarchy-fixtures.json")
	if err != nil {
		t.Fatalf("failed to read shared fixture: %v", err)
	}

	var agents []types.IAgentState
	if err := json.Unmarshal(fixtureData, &agents); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	if len(agents) == 0 {
		t.Fatalf("expected non-empty agents from shared fixture")
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, agents, false, 20_000_000)
	out := StripANSI(buf.String())

	// other-project has max UpdatedAt 15000000, while agent-status has max UpdatedAt 12000000.
	// other-project should sort before agent-status.
	idxOther := strings.Index(out, "v other-project")
	idxAgentStatus := strings.Index(out, "v agent-status")

	if idxOther == -1 || idxAgentStatus == -1 {
		t.Fatalf("expected both projects in output, got:\n%s", out)
	}

	if idxOther > idxAgentStatus {
		t.Errorf("expected other-project (more recent max UpdatedAt) to appear before agent-status, got other=%d agentStatus=%d\nOutput:\n%s", idxOther, idxAgentStatus, out)
	}

	// Verify agent-status ordering:
	// In root checkout: Fix race condition (busy, 10000000) before Refactor parser (idle, 9000000)
	idxFixRace := strings.Index(out, "Fix race condition")
	idxRefactor := strings.Index(out, "Refactor parser")
	if idxFixRace == -1 || idxRefactor == -1 || idxFixRace > idxRefactor {
		t.Errorf("expected Fix race condition before Refactor parser, got fixRace=%d refactor=%d", idxFixRace, idxRefactor)
	}

	// In feat-ui worktree: Implement UI components (busy, 12000000) before Setup storybook (finished, 8000000)
	idxImplementUI := strings.Index(out, "Implement UI components")
	idxSetupStorybook := strings.Index(out, "Setup storybook")
	if idxImplementUI == -1 || idxSetupStorybook == -1 || idxImplementUI > idxSetupStorybook {
		t.Errorf("expected Implement UI components before Setup storybook, got implementUI=%d setupStorybook=%d", idxImplementUI, idxSetupStorybook)
	}
}
