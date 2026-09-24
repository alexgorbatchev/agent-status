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
	if !strings.Contains(stripANSI(out), "(Claude Code) [Focus] [Hide]") {
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

	if !strings.Contains(stripANSI(out), "(Pi) [Focus] [Hide]") {
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

	if !strings.Contains(stripANSI(out), "(Codex) [Unhide]") {
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
	if !strings.Contains(stripANSI(out), "(OpenCode) [Focus] [Hide]") {
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
	if !strings.Contains(stripANSI(buf.String()), "(Claude Code) [Focus] [Hide]") {
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

	dividerWidth := runewidth.StringWidth(stripANSI(lines[0]))
	headerLine := lines[1]
	headerWidth := runewidth.StringWidth(stripANSI(headerLine))

	if headerWidth != dividerWidth {
		t.Errorf("headerLine visual width = %d, want %d (equal to divider width)", headerWidth, dividerWidth)
	}

	metricsLine := lines[len(lines)-2]
	metricsWidth := runewidth.StringWidth(stripANSI(metricsLine))
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
	dividerWidth := runewidth.StringWidth(stripANSI(lines[0]))
	headerWidth := runewidth.StringWidth(stripANSI(lines[1]))

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

func TestFormatHarnessBadge_ProhibitsRedAndGreen(t *testing.T) {
	// Custom or unknown harness should never be assigned Red or Green
	badge := FormatHarnessBadge(types.HarnessType("custom-harness"), types.ActivityBusy)
	if strings.Contains(badge, AnsiRed) || strings.Contains(badge, AnsiGreen) {
		t.Errorf("FormatHarnessBadge must never assign Red or Green as vendor brand color, got: %q", badge)
	}

	// Prohibited red input must be sanitized/prohibited
	prohibitedHarness := types.HarnessType(AnsiRed + "EvilHarness" + AnsiReset)
	sanitized := FormatHarnessBadge(prohibitedHarness, types.ActivityBusy)
	if strings.Contains(sanitized, AnsiRed) {
		t.Errorf("FormatHarnessBadge must sanitize prohibited red hue from vendor input in working state, got: %q", sanitized)
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
	if !strings.Contains(clean, "▼ agent-status (github.com/alexgorbatchev/agent-status)") {
		t.Errorf("expected project header in fleet view, got:\n%s", clean)
	}

	// Root branch should be first child branch with ├─
	if !strings.Contains(clean, "├─ [main] (2 sessions)") {
		t.Errorf("expected root branch header with ├─, got:\n%s", clean)
	}

	// Middle worktree should have ├─
	if !strings.Contains(clean, "├─ [worktree: feat-auth] (.workspaces/feat-auth) (1 session)") {
		t.Errorf("expected wt1 branch header with ├─, got:\n%s", clean)
	}

	// Last worktree should have └─
	if !strings.Contains(clean, "└─ [worktree: feat-ui] (.workspaces/feat-ui) (1 session)") {
		t.Errorf("expected wt2 branch header with └─, got:\n%s", clean)
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
	if !strings.Contains(clean, "└─ [worktree: feat-auth] (.workspaces/feat-auth) (1 session)") {
		t.Errorf("expected single worktree branch to have └─, got:\n%s", clean)
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

	if !strings.Contains(stripANSI(out), "(Completed)") {
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

	if !strings.Contains(out, "(completed)") {
		t.Errorf("expected fleet view tree to display (completed) for unacknowledged completion, got:\n%s", out)
	}
}






