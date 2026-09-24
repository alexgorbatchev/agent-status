package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
	"github.com/mattn/go-runewidth"
)

func TestIsAgentMode(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() { _ = os.Setenv("AGENT", orig) }()

	tests := []struct {
		val  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"yes", true},
		{"0", false},
		{"false", false},
		{"", false},
		{"no", false},
	}

	for _, tt := range tests {
		_ = os.Setenv("AGENT", tt.val)
		if got := IsAgentMode(); got != tt.want {
			t.Errorf("IsAgentMode() with AGENT=%q = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestGetTerminalWidth(t *testing.T) {
	w := GetTerminalWidth()
	if w <= 0 {
		t.Errorf("GetTerminalWidth() = %d, want > 0", w)
	}
}

func TestRenderDivider(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() { _ = os.Setenv("AGENT", orig) }()

	t.Run("human mode", func(t *testing.T) {
		_ = os.Setenv("AGENT", "0")
		var buf bytes.Buffer
		RenderDivider(&buf, '-')
		if buf.Len() == 0 {
			t.Errorf("expected divider output in human mode")
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		_ = os.Setenv("AGENT", "1")
		var buf bytes.Buffer
		RenderDivider(&buf, '-')
		if buf.Len() != 0 {
			t.Errorf("expected no divider in agent mode, got %q", buf.String())
		}
	})
}

func TestRenderSessionList(t *testing.T) {
	sessions := []SessionSummary{
		{
			ID:        "ses_1234567890abcdef",
			Title:     "Refactor Authentication Middleware",
			Directory: "/home/user/work/my-project",
			CreatedAt: time.Date(2026, 9, 21, 14, 30, 0, 0, time.UTC),
			CostUSD:   0.0452,
			Tokens:    15200,
		},
	}

	t.Run("human mode with sessions", func(t *testing.T) {
		var buf bytes.Buffer
		RenderSessionList(&buf, sessions, false)
		out := buf.String()
		if !strings.Contains(out, "ses_1234567890abcdef") {
			t.Errorf("expected session ID in output, got: %s", out)
		}
		if !strings.Contains(out, "Refactor Authentication") {
			t.Errorf("expected title in output, got: %s", out)
		}
	})

	t.Run("human mode empty sessions", func(t *testing.T) {
		var buf bytes.Buffer
		RenderSessionList(&buf, nil, false)
		out := buf.String()
		if !strings.Contains(out, "No sessions found.") {
			t.Errorf("expected 'No sessions found.', got: %s", out)
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderSessionList(&buf, sessions, true)
		out := buf.String()
		if !strings.Contains(out, "session_id:ses_1234567890abcdef") {
			t.Errorf("expected compact agent output, got: %s", out)
		}
		if strings.Contains(out, "+---") {
			t.Errorf("agent mode should not contain table borders")
		}
	})
}

func TestRenderDiff(t *testing.T) {
	subID := "sub-1"
	diff := StateDiff{
		StepIndex:  2,
		LineNumber: 15,
		EventType:  "tool_call",
		SubagentID: &subID,
		Mutations: []FieldMutation{
			{Field: "Activity", From: "IDLE", To: "BUSY"},
			{Field: "ActiveTool", From: "nil", To: "Read"},
		},
		InputDelta:  1200,
		OutputDelta: 150,
		CostDelta:   0.0035,
	}

	t.Run("human mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderDiff(&buf, diff, false)
		out := buf.String()
		if !strings.Contains(out, "Step 2 [tool_call]") {
			t.Errorf("expected step header, got: %s", out)
		}
		if !strings.Contains(out, "Activity:      IDLE -> BUSY") {
			t.Errorf("expected Activity mutation, got: %s", out)
		}
		if !strings.Contains(out, "Tokens:        In: +1200 | Out: +150") {
			t.Errorf("expected token delta, got: %s", out)
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderDiff(&buf, diff, true)
		out := buf.String()
		if !strings.Contains(out, "diff step:2 event:tool_call line:15 subagent:sub-1") {
			t.Errorf("expected compact diff line, got: %s", out)
		}
		if !strings.Contains(out, "Activity:IDLE->BUSY") {
			t.Errorf("expected mutation, got: %s", out)
		}
	})

	t.Run("synthetic diff annotation in human and agent mode", func(t *testing.T) {
		synthDiff := diff
		synthDiff.IsSynthetic = true
		var buf bytes.Buffer
		RenderDiff(&buf, synthDiff, false)
		out := buf.String()
		if !strings.Contains(out, "(synthetic)") {
			t.Errorf("expected '(synthetic)' in diff header, got: %s", out)
		}

		buf.Reset()
		RenderDiff(&buf, synthDiff, true)
		agentOut := buf.String()
		if !strings.Contains(agentOut, "synthetic:true") {
			t.Errorf("expected 'synthetic:true' in agent diff, got: %s", agentOut)
		}
	})
}

func TestRenderReplaySummary(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() { _ = os.Setenv("AGENT", orig) }()
	_ = os.Setenv("AGENT", "0")

	summary := ReplaySummary{
		Harness:          types.HarnessClaudeCode,
		SessionID:        "ses-abc",
		AgentID:          "env-1:claude-code:ses-abc",
		ProjectName:      "test-project",
		Branch:           "main",
		CWD:              "/path/to/project",
		Activity:         types.ActivityIdle,
		Lifecycle:        types.LifecycleExited,
		Title:            "Fix database connection pool",
		Action:           "Reviewing code",
		TokensInput:      5000,
		TokensOutput:     800,
		TokensTotal:      5800,
		CostUSD:          0.0150,
		SubagentCount:    1,
		FramesProcessed:  10,
		VerificationOK:   true,
		VerificationLogs: 10,
		DurationMs:       120,
	}

	t.Run("human mode passed", func(t *testing.T) {
		var buf bytes.Buffer
		RenderReplaySummary(&buf, summary, false)
		out := buf.String()
		if !strings.Contains(out, "Session Replay Summary (claude-code)") {
			t.Errorf("expected header, got: %s", out)
		}
		if !strings.Contains(out, "[OK] Read-side query verification passed") {
			t.Errorf("expected OK verification, got: %s", out)
		}
	})

	t.Run("human mode failed verification", func(t *testing.T) {
		failSummary := summary
		failSummary.VerificationOK = false
		var buf bytes.Buffer
		RenderReplaySummary(&buf, failSummary, false)
		out := buf.String()
		if !strings.Contains(out, "[ERROR] Read-side query verification failed!") {
			t.Errorf("expected failure message, got: %s", out)
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderReplaySummary(&buf, summary, true)
		out := buf.String()
		if !strings.Contains(out, "harness:claude-code session_id:ses-abc") {
			t.Errorf("expected agent line, got: %s", out)
		}
		if !strings.Contains(out, "verification:OK") {
			t.Errorf("expected verification status, got: %s", out)
		}
	})

	t.Run("synthetic lifecycle annotation in human and agent mode", func(t *testing.T) {
		synthSummary := summary
		synthSummary.IsLifecycleSynthetic = true
		var buf bytes.Buffer
		RenderReplaySummary(&buf, synthSummary, false)
		out := buf.String()
		if !strings.Contains(out, "Lifecycle:       EXITED (synthetic)") {
			t.Errorf("expected 'Lifecycle:       EXITED (synthetic)', got: %s", out)
		}

		buf.Reset()
		RenderReplaySummary(&buf, synthSummary, true)
		agentOut := buf.String()
		if !strings.Contains(agentOut, "lifecycle:EXITED(synthetic)") {
			t.Errorf("expected 'lifecycle:EXITED(synthetic)', got: %s", agentOut)
		}
	})

	t.Run("native lifecycle annotation in human and agent mode", func(t *testing.T) {
		nativeSummary := summary
		nativeSummary.IsLifecycleSynthetic = false
		var buf bytes.Buffer
		RenderReplaySummary(&buf, nativeSummary, false)
		out := buf.String()
		if !strings.Contains(out, "Lifecycle:       EXITED (native)") {
			t.Errorf("expected 'Lifecycle:       EXITED (native)', got: %s", out)
		}

		buf.Reset()
		RenderReplaySummary(&buf, nativeSummary, true)
		agentOut := buf.String()
		if !strings.Contains(agentOut, "lifecycle:EXITED(native)") {
			t.Errorf("expected 'lifecycle:EXITED(native)', got: %s", agentOut)
		}
	})
}

func TestRenderValidationReport(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() { _ = os.Setenv("AGENT", orig) }()
	_ = os.Setenv("AGENT", "0")

	report := ValidationReport{
		Harness: types.HarnessClaudeCode,
		Target:  "session-abc.jsonl",
		Passed:  true,
		Checks: []ValidationCheck{
			{Name: "Harness Detection", Passed: true, Details: "Detected Claude Code"},
			{Name: "Event Framing", Passed: true, Details: "10 frames processed without errors"},
		},
	}

	t.Run("human mode passed", func(t *testing.T) {
		var buf bytes.Buffer
		RenderValidationReport(&buf, report, false)
		out := buf.String()
		if !strings.Contains(out, "Validation Report: session-abc.jsonl") {
			t.Errorf("expected report header, got: %s", out)
		}
		if !strings.Contains(out, "[OK] Session transcript is VALID") {
			t.Errorf("expected valid status, got: %s", out)
		}
	})

	t.Run("human mode failed", func(t *testing.T) {
		failReport := report
		failReport.Passed = false
		failReport.Checks[0].Passed = false
		var buf bytes.Buffer
		RenderValidationReport(&buf, failReport, false)
		out := buf.String()
		if !strings.Contains(out, "[FAIL]") {
			t.Errorf("expected fail tag, got: %s", out)
		}
		if !strings.Contains(out, "failed one or more validation checks") {
			t.Errorf("expected failure footer, got: %s", out)
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderValidationReport(&buf, report, true)
		out := buf.String()
		if !strings.Contains(out, "validation target:session-abc.jsonl harness:claude-code status:PASS") {
			t.Errorf("expected compact agent output, got: %s", out)
		}
	})
}

func TestRenderComparison(t *testing.T) {
	comp := &ComparisonResult{
		DBPath:    "/tmp/agent-status.db",
		SessionID: "ses-1",
		AgentID:   "env-1:claude-code:ses-1",
		Fields: []DiscrepancyField{
			{Name: "Lifecycle", ReplayedValue: "EXITED", PersistedValue: "RUNNING", Status: "DRIFT", IsDrift: true},
			{Name: "Tokens Total", ReplayedValue: "1,000", PersistedValue: "1,000", Status: "MATCH", IsDrift: false},
		},
		Warnings: []string{"Database updated_at is 2h newer than transcript last event time!"},
		HasDrift: true,
	}

	t.Run("nil comparison", func(t *testing.T) {
		var buf bytes.Buffer
		RenderComparison(&buf, nil, false)
		if buf.Len() != 0 {
			t.Errorf("expected empty output for nil comparison")
		}
	})

	t.Run("human mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderComparison(&buf, comp, false)
		out := buf.String()
		if !strings.Contains(out, "Live Database Comparison (agent-status.db)") {
			t.Errorf("expected header, got %s", out)
		}
		if !strings.Contains(out, "Lifecycle") || !strings.Contains(out, "DRIFT") {
			t.Errorf("expected Lifecycle row with DRIFT, got %s", out)
		}
		if !strings.Contains(out, "[WARN] Database updated_at is 2h newer") {
			t.Errorf("expected warning, got %s", out)
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderComparison(&buf, comp, true)
		out := buf.String()
		if !strings.Contains(out, "db_comparison status:DRIFT db_path:/tmp/agent-status.db") {
			t.Errorf("expected db_comparison line, got %s", out)
		}
		if !strings.Contains(out, "db_field name:Lifecycle") {
			t.Errorf("expected db_field line, got %s", out)
		}
		if !strings.Contains(out, "db_warning") {
			t.Errorf("expected db_warning line, got %s", out)
		}
	})
}

func TestRenderSessionInspect(t *testing.T) {
	now := time.Now().UnixMilli()
	rec := &db.SessionRecord{
		DBPath: "/tmp/agent-status.db",
		Agent: &types.IAgentState{
			ID:        "env-1:claude-code:ses-1",
			SessionID: "ses-1",
			Harness:   types.HarnessClaudeCode,
			Environment: types.IEnvironmentInfo{
				Label:    "local-dev",
				Hostname: "macbook",
			},
			Project: types.IProjectInfo{
				Name: "agent-status",
			},
			Workspace: types.IWorkspaceInfo{
				Branch: "main",
				CWD:    "/repo",
			},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityBusy,
				Title:     "Debugging test",
				Action:    "Editing file",
				Usage: types.ITokenUsage{
					Input:  1000,
					Output: 200,
					Total:  1200,
				},
				Cost: &types.ICostInfo{
					Total: 0.045,
				},
				CurrentRun: &types.IRunInfo{
					ID:  "run-1",
					PID: 9999,
				},
				UpdatedAt: now,
			},
		},
		Runs: []db.RunRecord{
			{ID: "run-1", PID: 9999, HarnessVersion: "1.0", StartedAt: now - 10000, EndedAt: nil},
		},
		EventCount:          15,
		FirstEventTimestamp: &now,
		LastEventTimestamp:  &now,
	}

	t.Run("nil record", func(t *testing.T) {
		var buf bytes.Buffer
		RenderSessionInspect(&buf, nil, false)
		if buf.Len() != 0 {
			t.Errorf("expected empty output for nil record")
		}
	})

	t.Run("human mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderSessionInspect(&buf, rec, false)
		out := buf.String()
		if !strings.Contains(out, "Persisted Agent Record (ses-1)") {
			t.Errorf("expected header, got %s", out)
		}
		if !strings.Contains(out, "env-1:claude-code:ses-1") {
			t.Errorf("expected Agent ID, got %s", out)
		}
		if !strings.Contains(out, "Debugging test") {
			t.Errorf("expected Title, got %s", out)
		}
		if !strings.Contains(out, "Telemetry Events: 15 recorded") {
			t.Errorf("expected event count, got %s", out)
		}
	})

	t.Run("agent mode", func(t *testing.T) {
		var buf bytes.Buffer
		RenderSessionInspect(&buf, rec, true)
		out := buf.String()
		if !strings.Contains(out, "db_agent id:env-1:claude-code:ses-1 session_id:ses-1") {
			t.Errorf("expected db_agent line, got %s", out)
		}
		if !strings.Contains(out, "db_run id:run-1 pid:9999") {
			t.Errorf("expected db_run line, got %s", out)
		}
	})
}

func TestRenderSessionList_EastAsianTruncation(t *testing.T) {
	sessions := []SessionSummary{
		{
			ID:        "ses_eaw_12345",
			Title:     "日本語タイトルでテーブルのアライメントを確認するためのテストセッション",
			Directory: "/home/ユーザー/プロジェクト/サブディレクトリ/agent-status",
			CreatedAt: time.Date(2026, 9, 21, 14, 30, 0, 0, time.UTC),
			CostUSD:   0.05,
			Tokens:    20000,
		},
	}

	var buf bytes.Buffer
	RenderSessionList(&buf, sessions, false)
	out := buf.String()

	// 1. Output must be valid UTF-8 (no broken rune bytes from naive byte slicing)
	if !utf8.ValidString(out) {
		t.Errorf("RenderSessionList produced invalid UTF-8 string due to mid-rune slicing:\n%s", out)
	}

	// 2. All table rows must have matching visual width
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected table with at least 3 lines, got %d", len(lines))
	}

	expectedWidth := runewidth.StringWidth(lines[0])
	for i, line := range lines {
		w := runewidth.StringWidth(line)
		if w != expectedWidth {
			t.Errorf("table line %d has visual width %d, want %d: %q", i, w, expectedWidth, line)
		}
	}
}

func TestRenderDivider_EastAsianWideRune(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() { _ = os.Setenv("AGENT", orig) }()
	_ = os.Setenv("AGENT", "0")

	var buf bytes.Buffer
	// Fullwidth hyphen-minus (East Asian wide character, width 2)
	RenderDivider(&buf, '－')
	line := strings.TrimRight(buf.String(), "\n")
	w := runewidth.StringWidth(line)
	expected := GetTerminalWidth()
	if w != expected {
		t.Errorf("RenderDivider with wide rune visual width = %d, want %d", w, expected)
	}
}

func TestStripANSI_And_VisualWidth(t *testing.T) {
	plain := "Hello World"
	colored := "\x1b[31;1mHello \x1b[32mWorld\x1b[0m"
	osc := "\x1b]0;Title\x07Hello World"

	if got := StripANSI(plain); got != plain {
		t.Errorf("StripANSI(%q) = %q, want %q", plain, got, plain)
	}
	if got := StripANSI(colored); got != plain {
		t.Errorf("StripANSI(%q) = %q, want %q", colored, got, plain)
	}
	if got := StripANSI(osc); got != plain {
		t.Errorf("StripANSI(%q) = %q, want %q", osc, got, plain)
	}

	if got := VisualWidth(plain); got != 11 {
		t.Errorf("VisualWidth(%q) = %d, want 11", plain, got)
	}
	if got := VisualWidth(colored); got != 11 {
		t.Errorf("VisualWidth(%q) = %d, want 11", colored, got)
	}
	if got := VisualWidth("日本語"); got != 6 {
		t.Errorf("VisualWidth('日本語') = %d, want 6", got)
	}
}

func TestTruncateLeftVisual_EdgeCases(t *testing.T) {
	// 1. String already fits
	short := "/short/path"
	if got := TruncateLeftVisual(short, 20, "..."); got != short {
		t.Errorf("TruncateLeftVisual(%q) = %q, want %q", short, got, short)
	}

	// 2. maxWidth smaller than prefix
	if got := TruncateLeftVisual("longstring", 2, "..."); got != ".." {
		t.Errorf("TruncateLeftVisual with small maxWidth = %q, want '..'", got)
	}

	// 3. ASCII long string
	longASCII := "1234567890abcdefghijklmnopqrstuvwxyz"
	gotASCII := TruncateLeftVisual(longASCII, 10, "...")
	if VisualWidth(gotASCII) > 10 {
		t.Errorf("VisualWidth(%q) = %d > 10", gotASCII, VisualWidth(gotASCII))
	}
	if !strings.HasPrefix(gotASCII, "...") {
		t.Errorf("expected prefix '...', got %q", gotASCII)
	}

	// 4. ANSI string that fits in maxWidth: returned intact
	ansiShort := "\x1b[32m/short/path\x1b[0m"
	if got := TruncateLeftVisual(ansiShort, 20, "..."); got != ansiShort {
		t.Errorf("TruncateLeftVisual(%q) = %q, want %q", ansiShort, got, ansiShort)
	}

	// 5. ANSI string that exceeds maxWidth: strip-safe, visual width <= maxWidth, no broken escape codes
	ansiLong := "\x1b[31;1m/home/user/long/path/to/project\x1b[0m"
	gotANSI := TruncateLeftVisual(ansiLong, 15, "...")
	if VisualWidth(gotANSI) > 15 {
		t.Errorf("VisualWidth(%q) = %d > 15", gotANSI, VisualWidth(gotANSI))
	}
	if !strings.HasPrefix(gotANSI, "...") {
		t.Errorf("expected prefix '...', got %q", gotANSI)
	}
	// Verify it does not contain dangling escape fragments like "\x1b[" without terminator
	if strings.Contains(gotANSI, "\x1b[") && !strings.Contains(gotANSI, "m") {
		t.Errorf("gotANSI contains broken escape sequence: %q", gotANSI)
	}

	// 6. Wide runes with ANSI codes
	wideANSI := "\x1b[34m/日本語/ディレクトリ/プロジェクト\x1b[0m"
	gotWide := TruncateLeftVisual(wideANSI, 14, "...")
	if VisualWidth(gotWide) > 14 {
		t.Errorf("VisualWidth(%q) = %d > 14", gotWide, VisualWidth(gotWide))
	}
	if !strings.HasPrefix(gotWide, "...") {
		t.Errorf("expected prefix '...', got %q", gotWide)
	}
}

func TestTruncateVisual_Comprehensive(t *testing.T) {
	// 1. Fits within maxWidth
	s := "hello world"
	if got := TruncateVisual(s, 20, "..."); got != s {
		t.Errorf("TruncateVisual(%q) = %q, want %q", s, got, s)
	}

	// 2. maxWidth <= 0
	if got := TruncateVisual(s, 0, "..."); got != "" {
		t.Errorf("TruncateVisual with 0 maxWidth = %q, want empty", got)
	}
	if got := TruncateVisual(s, -5, "..."); got != "" {
		t.Errorf("TruncateVisual with negative maxWidth = %q, want empty", got)
	}

	// 3. tailWidth > maxWidth
	if got := TruncateVisual("somelongstring", 2, "..."); got != ".." {
		t.Errorf("TruncateVisual with tail > maxWidth = %q, want '..'", got)
	}

	// 4. Plain CJK text
	cjk := "日本語タイトルでテーブルのアライメントを確認"
	gotCJK := TruncateVisual(cjk, 15, "...")
	if VisualWidth(gotCJK) > 15 {
		t.Errorf("VisualWidth(%q) = %d > 15", gotCJK, VisualWidth(gotCJK))
	}
	if !strings.HasSuffix(gotCJK, "...") {
		t.Errorf("expected suffix '...', got %q", gotCJK)
	}
	if !utf8.ValidString(gotCJK) {
		t.Errorf("gotCJK is not valid UTF-8: %s", gotCJK)
	}

	// 5. ANSI text within maxWidth
	ansiShort := "\x1b[32mhello\x1b[0m"
	if got := TruncateVisual(ansiShort, 10, "..."); got != ansiShort {
		t.Errorf("TruncateVisual(%q) = %q, want %q", ansiShort, got, ansiShort)
	}

	// 6. ANSI text exceeding maxWidth
	ansiLong := "\x1b[31;1mvery long colored text exceeding maxWidth\x1b[0m"
	gotANSI := TruncateVisual(ansiLong, 12, "...")
	if VisualWidth(gotANSI) > 12 {
		t.Errorf("VisualWidth(%q) = %d > 12", gotANSI, VisualWidth(gotANSI))
	}
	if !strings.HasSuffix(gotANSI, AnsiReset) {
		t.Errorf("expected AnsiReset terminator on truncated ANSI string, got %q", gotANSI)
	}

	// 7. CJK with emoji
	emojiCJK := "🚀 日本語テキスト 🌟"
	gotEmoji := TruncateVisual(emojiCJK, 10, "...")
	if VisualWidth(gotEmoji) > 10 {
		t.Errorf("VisualWidth(%q) = %d > 10", gotEmoji, VisualWidth(gotEmoji))
	}
	if !utf8.ValidString(gotEmoji) {
		t.Errorf("gotEmoji is not valid UTF-8: %s", gotEmoji)
	}
}

func TestPinnedCondition_EastAsianDeterminism(t *testing.T) {
	if runewidth.DefaultCondition.EastAsianWidth {
		t.Errorf("expected DefaultCondition.EastAsianWidth to be false, got true")
	}

	// Structural ASCII glyphs must always be 1 cell
	asciiGlyphs := []rune{'-', '|', '+', 'v', '*', '>'}
	for _, r := range asciiGlyphs {
		if w := runewidth.RuneWidth(r); w != 1 {
			t.Errorf("ASCII glyph %c visual width = %d, want 1", r, w)
		}
	}

	// Ambiguous box-drawing runes should evaluate as 1 cell under pinned condition
	ambiguousBoxRunes := []rune{'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼'}
	for _, r := range ambiguousBoxRunes {
		if w := runewidth.RuneWidth(r); w != 1 {
			t.Errorf("Ambiguous rune %c width = %d under pinned condition, want 1", r, w)
		}
	}

	// Wide CJK runes must evaluate as 2 cells
	if w := runewidth.RuneWidth('日'); w != 2 {
		t.Errorf("CJK rune '日' width = %d, want 2", w)
	}
}

func TestTrimProjectPrefix(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		projectName string
		want        string
	}{
		{
			name:        "colon separator",
			title:       "agent-status: add test coverage",
			projectName: "agent-status",
			want:        "add test coverage",
		},
		{
			name:        "colon separator without space",
			title:       "agent-status:add test coverage",
			projectName: "agent-status",
			want:        "add test coverage",
		},
		{
			name:        "colon separator with leading space",
			title:       "agent-status : add test coverage",
			projectName: "agent-status",
			want:        "add test coverage",
		},
		{
			name:        "middle dot separator",
			title:       "agent-status · fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "middle dot separator without spaces",
			title:       "agent-status·fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "dash separator",
			title:       "agent-status - fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "double-dash separator",
			title:       "agent-status -- fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "en-dash separator",
			title:       "agent-status – fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "em-dash separator",
			title:       "agent-status — fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "pipe separator",
			title:       "agent-status | fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "pipe separator without space",
			title:       "agent-status|fix issue 101",
			projectName: "agent-status",
			want:        "fix issue 101",
		},
		{
			name:        "bracketed prefix",
			title:       "[agent-status] implement oauth flow",
			projectName: "agent-status",
			want:        "implement oauth flow",
		},
		{
			name:        "bracketed prefix with colon",
			title:       "[agent-status]: implement oauth flow",
			projectName: "agent-status",
			want:        "implement oauth flow",
		},
		{
			name:        "bracketed prefix with dash",
			title:       "[agent-status] - implement oauth flow",
			projectName: "agent-status",
			want:        "implement oauth flow",
		},
		{
			name:        "case-insensitive match title upper",
			title:       "AGENT-STATUS: Fix bug",
			projectName: "agent-status",
			want:        "Fix bug",
		},
		{
			name:        "case-insensitive match title mixed",
			title:       "Agent-Status: Fix bug",
			projectName: "agent-status",
			want:        "Fix bug",
		},
		{
			name:        "case-insensitive match project upper",
			title:       "agent-status · Fix bug",
			projectName: "AGENT-STATUS",
			want:        "Fix bug",
		},
		{
			name:        "case-insensitive bracketed",
			title:       "[AGENT-STATUS] Fix bug",
			projectName: "agent-status",
			want:        "Fix bug",
		},
		{
			name:        "repo basename from github url",
			title:       "agent-status: add test coverage",
			projectName: "github.com/alexgorbatchev/agent-status",
			want:        "add test coverage",
		},
		{
			name:        "repo basename from git clone url",
			title:       "agent-status: add test coverage",
			projectName: "https://github.com/alexgorbatchev/agent-status.git",
			want:        "add test coverage",
		},
		{
			name:        "repo basename from local directory path",
			title:       "agent-status · fix bug",
			projectName: "/home/alex/repos/agent-status",
			want:        "fix bug",
		},
		{
			name:        "preserves tail when tail equals project name",
			title:       "agent-status: agent-status",
			projectName: "agent-status",
			want:        "agent-status",
		},
		{
			name:        "preserves bracketed tail when tail equals project name",
			title:       "[agent-status] [agent-status]",
			projectName: "agent-status",
			want:        "[agent-status]",
		},
		{
			name:        "preserves title when title equals project name without separator",
			title:       "agent-status",
			projectName: "agent-status",
			want:        "agent-status",
		},
		{
			name:        "preserves title when only separator remains",
			title:       "agent-status: ",
			projectName: "agent-status",
			want:        "agent-status: ",
		},
		{
			name:        "preserves title when only bracketed name",
			title:       "[agent-status]",
			projectName: "agent-status",
			want:        "[agent-status]",
		},
		{
			name:        "preserves title when only punctuation follows separator",
			title:       "agent-status: - ",
			projectName: "agent-status",
			want:        "agent-status: - ",
		},
		{
			name:        "unmatched project untouched",
			title:       "billing · trace duplicate charges",
			projectName: "agent-status",
			want:        "billing · trace duplicate charges",
		},
		{
			name:        "unmatched bracketed project untouched",
			title:       "[billing] trace duplicate charges",
			projectName: "agent-status",
			want:        "[billing] trace duplicate charges",
		},
		{
			name:        "word prefix not stripped",
			title:       "agent-status-service: add tests",
			projectName: "agent-status",
			want:        "agent-status-service: add tests",
		},
		{
			name:        "partial word not stripped",
			title:       "agent: add tests",
			projectName: "agent-status",
			want:        "agent: add tests",
		},
		{
			name:        "empty title",
			title:       "",
			projectName: "agent-status",
			want:        "",
		},
		{
			name:        "empty project name",
			title:       "agent-status: add tests",
			projectName: "",
			want:        "agent-status: add tests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrimProjectPrefix(tt.title, tt.projectName)
			if got != tt.want {
				t.Fatalf("TrimProjectPrefix(%q, %q) = %q, want %q", tt.title, tt.projectName, got, tt.want)
			}
		})
	}
}

func TestFormatActivityStatus(t *testing.T) {
	tTrue := true
	tFalse := false

	tests := []struct {
		name                string
		lifecycle           types.LifecycleState
		activity            types.ActivityState
		hasUnseenCompletion *bool
		want                string
	}{
		{"running completed", types.LifecycleRunning, types.ActivityIdle, &tTrue, "Completed"},
		{"running idle unseen false", types.LifecycleRunning, types.ActivityIdle, &tFalse, "Idle"},
		{"running idle unseen nil", types.LifecycleRunning, types.ActivityIdle, nil, "Idle"},
		{"running busy", types.LifecycleRunning, types.ActivityBusy, nil, "Working"},
		{"running question", types.LifecycleRunning, types.ActivityWaitingQuestion, nil, "Waiting on Question"},
		{"running error", types.LifecycleRunning, types.ActivityError, nil, "Error"},
		{"running rate limited", types.LifecycleRunning, types.ActivityRateLimited, nil, "Rate Limited"},
		{"exited", types.LifecycleExited, types.ActivityIdle, nil, "Finished"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatActivityStatus(tt.lifecycle, tt.activity, tt.hasUnseenCompletion)
			if got != tt.want {
				t.Errorf("FormatActivityStatus(%s, %s, %v) = %q, want %q", tt.lifecycle, tt.activity, tt.hasUnseenCompletion, got, tt.want)
			}
		})
	}
}

func TestRenderReplaySummary_Completed(t *testing.T) {
	tTrue := true
	s := ReplaySummary{
		SessionID:           "ses-123",
		AgentID:             "env-1:claude-code:ses-123",
		ProjectName:         "repo",
		Branch:              "main",
		Harness:             types.HarnessClaudeCode,
		Lifecycle:           types.LifecycleRunning,
		Activity:            types.ActivityIdle,
		HasUnseenCompletion: &tTrue,
	}

	var humanBuf bytes.Buffer
	RenderReplaySummary(&humanBuf, s, false)
	humanOut := humanBuf.String()
	if !strings.Contains(humanOut, "Activity:        Completed") {
		t.Errorf("expected human output to contain Activity: Completed, got:\n%s", humanOut)
	}

	var agentBuf bytes.Buffer
	RenderReplaySummary(&agentBuf, s, true)
	agentOut := agentBuf.String()
	if !strings.Contains(agentOut, "activity:COMPLETED") {
		t.Errorf("expected agent output to contain activity:COMPLETED, got:\n%s", agentOut)
	}
}

func TestSanitizeBrandColor(t *testing.T) {
	if got := SanitizeBrandColor(AnsiRed); got != AnsiWhite {
		t.Errorf("SanitizeBrandColor(AnsiRed) = %q, want %q", got, AnsiWhite)
	}
	if got := SanitizeBrandColor(AnsiGreen); got != AnsiWhite {
		t.Errorf("SanitizeBrandColor(AnsiGreen) = %q, want AnsiWhite", got)
	}
	if got := SanitizeBrandColor(AnsiMagenta); got != AnsiMagenta {
		t.Errorf("SanitizeBrandColor(AnsiMagenta) = %q, want %q", got, AnsiMagenta)
	}
}

func TestTruncateVisual_ZWJEmojiSequences(t *testing.T) {
	tail := "..."
	tests := []struct {
		name     string
		input    string
		maxWidth int
	}{
		{
			name:     "technologist woman ZWJ sequence",
			input:    "Developer: 👩‍💻 working on auth",
			maxWidth: 16,
		},
		{
			name:     "family emoji multi-ZWJ sequence",
			input:    "Family 👨‍👩‍👧‍👦 vacation",
			maxWidth: 12,
		},
		{
			name:     "weightlifter with VS16 and ZWJ",
			input:    "Athlete: 🏋️‍♀️ training",
			maxWidth: 14,
		},
		{
			name:     "heart on fire ZWJ sequence",
			input:    "Status: ❤️‍🔥 hot fix",
			maxWidth: 13,
		},
		{
			name:     "technologist with ANSI color and ZWJ",
			input:    "\x1b[32mAgent: 👩‍💻 coding\x1b[0m",
			maxWidth: 13,
		},
		{
			name:     "accented char with combining mark and ZWJ",
			input:    "Accent: e\u0301\u200d test",
			maxWidth: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateVisual(tt.input, tt.maxWidth, tail)

			// 1. Output must be valid UTF-8
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateVisual produced invalid UTF-8: %q", got)
			}

			// 2. Output visual width must be <= maxWidth
			w := VisualWidth(got)
			if w > tt.maxWidth {
				t.Errorf("VisualWidth(%q) = %d, want <= %d", got, w, tt.maxWidth)
			}

			// 3. The truncated body before tail must not end with dangling ZWJ, VS, or combining mark
			plain := StripANSI(got)
			if strings.HasSuffix(plain, tail) {
				body := strings.TrimSuffix(plain, tail)
				r, _ := utf8.DecodeLastRuneInString(body)
				if r == '\u200d' || r == '\u200c' {
					t.Errorf("body before tail ends with dangling joiner %U: %q", r, body)
				}
				if (r >= 0xfe00 && r <= 0xfe0f) || (r >= 0xe0100 && r <= 0xe01ef) {
					t.Errorf("body before tail ends with variation selector %U: %q", r, body)
				}
				if unicode.Is(unicode.M, r) {
					t.Errorf("body before tail ends with combining mark %U: %q", r, body)
				}
			}
		})
	}
}

func TestTruncateLeftVisual_ZWJEmojiSequences(t *testing.T) {
	prefix := "..."
	tests := []struct {
		name     string
		input    string
		maxWidth int
	}{
		{
			name:     "path with technologist woman ZWJ sequence",
			input:    "path/to/👩‍💻/workspace/file.go",
			maxWidth: 20,
		},
		{
			name:     "path with family emoji multi-ZWJ sequence",
			input:    "/home/user/👨‍👩‍👧‍👦/project",
			maxWidth: 18,
		},
		{
			name:     "weightlifter with VS16 and ZWJ",
			input:    "task-prefix-🏋️‍♀️-details",
			maxWidth: 16,
		},
		{
			name:     "heart on fire ZWJ sequence",
			input:    "status-label-❤️‍🔥-urgent",
			maxWidth: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateLeftVisual(tt.input, tt.maxWidth, prefix)

			// 1. Output must be valid UTF-8
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateLeftVisual produced invalid UTF-8: %q", got)
			}

			// 2. Output visual width must be <= maxWidth
			w := VisualWidth(got)
			if w > tt.maxWidth {
				t.Errorf("VisualWidth(%q) = %d, want <= %d", got, w, tt.maxWidth)
			}

			// 3. The truncated body after prefix must not start with dangling ZWJ, VS, or combining mark
			plain := StripANSI(got)
			if strings.HasPrefix(plain, prefix) {
				body := strings.TrimPrefix(plain, prefix)
				r, _ := utf8.DecodeRuneInString(body)
				if r == '\u200d' || r == '\u200c' {
					t.Errorf("body after prefix starts with dangling joiner %U: %q", r, body)
				}
				if (r >= 0xfe00 && r <= 0xfe0f) || (r >= 0xe0100 && r <= 0xe01ef) {
					t.Errorf("body after prefix starts with variation selector %U: %q", r, body)
				}
				if unicode.Is(unicode.M, r) {
					t.Errorf("body after prefix starts with combining mark %U: %q", r, body)
				}
			}
		})
	}
}

func TestTruncateVisual_C1Introducers(t *testing.T) {
	tail := "..."
	c1Introducers := []struct {
		name string
		seq  string
	}{
		{"DCS 0x90", "\u0090P123\u009c"},
		{"SOS 0x98", "\u0098S123\u009c"},
		{"CSI 0x9b", "\u009b31m"},
		{"OSC 0x9d", "\u009d0;Title\u0007"},
		{"PM 0x9e", "\u009eM123\u009c"},
		{"APC 0x9f", "\u009fA123\u009c"},
	}

	for _, tt := range c1Introducers {
		t.Run(tt.name, func(t *testing.T) {
			input := "Header: " + tt.seq + "Important content that exceeds max width"
			got := TruncateVisual(input, 20, tail)
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateVisual produced invalid UTF-8 with %s: %q", tt.name, got)
			}
			w := VisualWidth(got)
			if w > 20 {
				t.Errorf("VisualWidth(%q) = %d, want <= 20 with %s", got, w, tt.name)
			}
		})
	}
}

func TestTableRendering_EastAsianWidthConditions(t *testing.T) {
	defer func() {
		runewidth.DefaultCondition.EastAsianWidth = false
	}()

	sessions := []SessionSummary{
		{
			ID:        "ses_eaw_test",
			Title:     "テストタイトル（日本語）と英語 mixed",
			Directory: "/home/ユーザー/プロジェクト/worktree",
			CreatedAt: time.Date(2026, 9, 21, 14, 30, 0, 0, time.UTC),
			CostUSD:   0.05,
			Tokens:    12345,
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
			RenderSessionList(&buf, sessions, false)
			out := buf.String()

			if !utf8.ValidString(out) {
				t.Fatalf("RenderSessionList produced invalid UTF-8 under %s:\n%s", conditionName, out)
			}

			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) < 3 {
				t.Fatalf("expected table with at least 3 lines, got %d", len(lines))
			}

			// Table width must be uniform across all lines
			firstWidth := runewidth.StringWidth(lines[0])
			for i, line := range lines {
				lw := runewidth.StringWidth(line)
				if lw != firstWidth {
					t.Errorf("line %d has width %d, want %d under %s: %q", i, lw, firstWidth, conditionName, line)
				}
			}
		})
	}
}
