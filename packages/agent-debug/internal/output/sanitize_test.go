package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/scanner"
)

func TestSanitizeTranscriptText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean text unchanged",
			input:    "Hello world 123",
			expected: "Hello world 123",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "preserves tab and newline",
			input:    "Line 1\tCol 1\nLine 2\tCol 2\n",
			expected: "Line 1\tCol 1\nLine 2\tCol 2\n",
		},
		{
			name:     "preserves international unicode characters",
			input:    "Привет, 世界! 🚀 Καλημέρα",
			expected: "Привет, 世界! 🚀 Καλημέρα",
		},
		// ESC & CSI sequences
		{
			name:     "strips basic CSI color sequence",
			input:    "\x1b[31mRed text\x1b[0m",
			expected: "Red text",
		},
		{
			name:     "strips complex CSI style sequence",
			input:    "\x1b[1;32;40mBold Green on Black\x1b[m",
			expected: "Bold Green on Black",
		},
		{
			name:     "strips CSI cursor move and screen clear",
			input:    "\x1b[2JScreen Cleared\x1b[1;1HAt Top",
			expected: "Screen ClearedAt Top",
		},
		{
			name:     "strips CSI private mode sequences",
			input:    "\x1b[?25lHidden Cursor\x1b[?25hVisible Cursor",
			expected: "Hidden CursorVisible Cursor",
		},
		{
			name:     "strips CSI line clear",
			input:    "\x1b[2KLine erased",
			expected: "Line erased",
		},
		// OSC sequences
		{
			name:     "strips OSC sequence with BEL terminator",
			input:    "\x1b]0;Injected Window Title\x07Clean Text",
			expected: "Clean Text",
		},
		{
			name:     "strips OSC sequence with ST terminator",
			input:    "\x1b]2;Injected Window Title\x1b\\Clean Text",
			expected: "Clean Text",
		},
		{
			name:     "strips OSC sequence with C1 ST terminator",
			input:    "\x1b]0;Title\u009cClean Text",
			expected: "Clean Text",
		},
		{
			name:     "strips unterminated OSC sequence at end of string",
			input:    "\x1b]0;Unterminated Title",
			expected: "",
		},
		{
			name:     "unterminated OSC on line 1 does not delete line 2",
			input:    "\x1b]0;Unterminated Title\nLine 2 content",
			expected: "0;Unterminated Title\nLine 2 content",
		},
		{
			name:     "strips unterminated C1 OSC sequence at end of string",
			input:    "\u009d0;Unterminated C1 Title",
			expected: "",
		},
		{
			name:     "strips unterminated DCS sequence at end of string",
			input:    "\x1bP123;device;payload",
			expected: "",
		},
		{
			name:     "strips unterminated C1 DCS sequence at end of string",
			input:    "\u0090payload without terminator",
			expected: "",
		},
		{
			name:     "strips unterminated C1 APC sequence at end of string",
			input:    "\u009fapc payload",
			expected: "",
		},
		// DCS, APC, PM, SOS sequences
		{
			name:     "strips DCS sequence with ST",
			input:    "\x1bP123;device\x1b\\Device Output",
			expected: "Device Output",
		},
		// C1 control characters & escape sequences
		{
			name:     "strips C1 CSI sequence",
			input:    "\u009b31mC1 Red Text\u009b0m",
			expected: "C1 Red Text",
		},
		{
			name:     "strips C1 OSC sequence with BEL",
			input:    "\u009d0;C1 Title\x07Clean Text",
			expected: "Clean Text",
		},
		{
			name:     "strips C1 OSC sequence with C1 ST",
			input:    "\u009d0;C1 Title\u009cClean Text",
			expected: "Clean Text",
		},
		{
			name:     "strips C1 DCS sequence",
			input:    "\u0090payload\u009cPayload Clean",
			expected: "Payload Clean",
		},
		{
			name:     "strips standalone C1 NEL (U+0085)",
			input:    "Line 1\u0085Line 2",
			expected: "Line 1Line 2",
		},
		{
			name:     "strips standalone C1 control characters (IND, HTS, RI, SS2, SS3)",
			input:    "A\u0084B\u0088C\u008dD\u008eE\u008fF",
			expected: "ABCDEF",
		},
		{
			name:     "strips terminated C1 string control characters (SOS, PM, APC)",
			input:    "A\u0098\u009cB\u009e\u009cC\u009f\u009cD",
			expected: "ABCD",
		},
		{
			name:     "strips unterminated C1 SOS sequence at end of string",
			input:    "A\u0098B\u009eC\u009fD",
			expected: "A",
		},
		{
			name: "strips all C1 code points U+0080 to U+009F",
			input: func() string {
				var b strings.Builder
				b.WriteString("prefix_")
				for r := rune(0x80); r <= 0x9f; r++ {
					b.WriteRune(r)
					b.WriteRune(0x9c)
				}
				b.WriteString("_suffix")
				return b.String()
			}(),
			expected: "prefix__suffix",
		},
		// C0 control characters
		{
			name:     "strips NUL, BEL, BS, VT, FF, DEL",
			input:    "A\x00B\x07C\x08D\x0bE\x0cF\x7fG",
			expected: "ABCDEFG",
		},
		{
			name:     "strips carriage return without newline",
			input:    "Overwrite\rLine",
			expected: "OverwriteLine",
		},
		{
			name:     "normalizes CRLF to LF",
			input:    "Line 1\r\nLine 2\r\n",
			expected: "Line 1\nLine 2\n",
		},
		// Other ESC sequences
		{
			name:     "strips charset designate ESC sequence",
			input:    "\x1b(BStandard ASCII\x1b(0Alt Charset",
			expected: "Standard ASCIIAlt Charset",
		},
		{
			name:     "strips 2-character ESC sequences (reset, keypad)",
			input:    "\x1bcFull Reset\x1b=Keypad\x1b>Normal",
			expected: "Full ResetKeypadNormal",
		},
		{
			name:     "strips lone ESC character",
			input:    "Lone \x1b ESC",
			expected: "Lone  ESC",
		},
		// Combined adversarial sequence
		{
			name:     "strips combined hostile payload",
			input:    "\x1b]0;PWNED\x07\x1b[31;1mAlert:\x1b[0m \u009b2K\u0085\r\n\x00Critical error\toccurred\x1b[?25h",
			expected: "Alert: \nCritical error\toccurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeTranscriptText(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeTranscriptText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeTranscriptText_UnterminatedOSCDoesNotDeleteLine2(t *testing.T) {
	input := "\x1b]0;Unterminated Title\nLine 2 content"
	got := SanitizeTranscriptText(input)
	if !strings.Contains(got, "Line 2 content") {
		t.Errorf("SanitizeTranscriptText deleted line 2: got %q, want it to contain %q", got, "Line 2 content")
	}
}

func TestRenderAgentView_SanitizesTranscriptText(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	q := "\x1b[2JClear\x1b[H\u0085Should we drop table users?"
	lastErr := "\x1b]2;Evil\x1b\\Fatal exception\x00 in worker"
	subTool := "\x1b[33mBash\x1b[0m\x07"

	agent := types.IAgentState{
		Harness: types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:    "env-1",
			Label: "local-dev",
		},
		Project: types.IProjectInfo{
			ID:   "proj-1",
			Name: "my-project",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
		},
		Activity: types.IAgentActivity{
			Title:     "\x1b]0;HACKED\x07Refactor auth\x1b[31m",
			Action:    "\u009b32mExecuting\u009b0m tests\x08",
			Activity:  types.ActivityWaitingQuestion,
			Lifecycle: types.LifecycleRunning,
			Question:  &q,
			LastError: &lastErr,
			UpdatedAt: nowMs - 1000,
			ActiveSubagents: []types.ISubagentSummary{
				{
					ID:         "sub-1",
					ActiveTool: &subTool,
				},
			},
		},
	}

	// 1. Human Mode
	var bufHuman bytes.Buffer
	RenderAgentViewAt(&bufHuman, agent, false, nowMs)
	outHuman := bufHuman.String()

	hostileFragments := []string{
		"\x1b]0;HACKED",
		"\x1b]2;Evil",
		"\u009b32m",
		"\u009b0m",
		"\u0085",
		"\x08",
		"\x00",
		"\x07",
		"\x1b[2J",
		"\x1b[1;1H",
	}
	for _, frag := range hostileFragments {
		if strings.Contains(outHuman, frag) {
			t.Errorf("RenderAgentView (Human Mode) output contains unsanitized fragment %q", frag)
		}
	}

	expectedClean := []string{
		"Refactor auth",
		"Executing tests",
		"Should we drop table users?",
		"Bash",
	}
	for _, exp := range expectedClean {
		if !strings.Contains(outHuman, exp) {
			t.Errorf("RenderAgentView (Human Mode) output missing expected clean text %q", exp)
		}
	}

	// 2. Agent Mode
	var bufAgent bytes.Buffer
	RenderAgentViewAt(&bufAgent, agent, true, nowMs)
	outAgent := bufAgent.String()

	for _, frag := range hostileFragments {
		if strings.Contains(outAgent, frag) {
			t.Errorf("RenderAgentView (Agent Mode) output contains unsanitized fragment %q", frag)
		}
	}
	expectedAgentClean := []string{
		"Refactor auth",
		"Executing tests",
		"Should we drop table users?",
	}
	for _, exp := range expectedAgentClean {
		if !strings.Contains(outAgent, exp) {
			t.Errorf("RenderAgentView (Agent Mode) output missing expected clean text %q", exp)
		}
	}

	// 3. Error state callout
	agent.Activity.Activity = types.ActivityError
	var bufError bytes.Buffer
	RenderAgentViewAt(&bufError, agent, false, nowMs)
	outError := bufError.String()
	for _, frag := range hostileFragments {
		if strings.Contains(outError, frag) {
			t.Errorf("RenderAgentView (Error state) output contains unsanitized fragment %q", frag)
		}
	}
	if !strings.Contains(outError, "Fatal exception in worker") {
		t.Errorf("RenderAgentView (Error state) missing clean error text, got: %s", outError)
	}
}

func TestRenderFleetView_SanitizesTranscriptText(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	wtRelPath := "\x1b[?25lworktree/path\x07"
	agents := []types.IAgentState{
		{
			Harness: types.HarnessClaudeCode,
			Project: types.IProjectInfo{
				ID:   "\x1b[2Jproj-1\x08",
				Name: "\x1b]0;BAD_NAME\x07my-project\x1b[31m",
			},
			Workspace: types.IWorkspaceInfo{
				Branch: "\x1b]2;BAD_BRANCH\x07main-branch\u009b0m",
			},
			Activity: types.IAgentActivity{
				Title:     "\x1b]0;INJECTED\x07Task 1\x1b[31m",
				Activity:  types.ActivityIdle,
				Lifecycle: types.LifecycleRunning,
				UpdatedAt: nowMs - 5000,
			},
		},
		{
			Harness: types.HarnessPi,
			Project: types.IProjectInfo{
				ID:   "\x1b[2Jproj-1\x08",
				Name: "\x1b]0;BAD_NAME\x07my-project\x1b[31m",
			},
			Workspace: types.IWorkspaceInfo{
				Branch:     "\x1b[33mfeat/bad-wt\x1b[0m",
				IsWorktree: true,
				RelPath:    &wtRelPath,
			},
			Activity: types.IAgentActivity{
				Title:     "Task 2",
				Activity:  types.ActivityBusy,
				Lifecycle: types.LifecycleRunning,
				UpdatedAt: nowMs - 2000,
			},
		},
	}

	hostileFragments := []string{
		"\x1b]0;INJECTED",
		"\x1b]0;BAD_NAME",
		"\x1b]2;BAD_BRANCH",
		"\x1b[2J",
		"\x1b[?25l",
		"\x08",
		"\x07",
		"\u009b0m",
	}

	// 1. Human mode
	var bufHuman bytes.Buffer
	RenderFleetViewAt(&bufHuman, agents, false, nowMs)
	outHuman := bufHuman.String()

	for _, frag := range hostileFragments {
		if strings.Contains(outHuman, frag) {
			t.Errorf("RenderFleetView (Human) contains unsanitized control sequence %q, got: %s", frag, outHuman)
		}
	}
	if !strings.Contains(outHuman, "my-project") {
		t.Errorf("RenderFleetView (Human) missing clean project name, got: %s", outHuman)
	}
	if !strings.Contains(outHuman, "proj-1") {
		t.Errorf("RenderFleetView (Human) missing clean project ID, got: %s", outHuman)
	}
	if !strings.Contains(outHuman, "main-branch") {
		t.Errorf("RenderFleetView (Human) missing clean root branch, got: %s", outHuman)
	}
	if !strings.Contains(outHuman, "feat/bad-wt") {
		t.Errorf("RenderFleetView (Human) missing clean worktree branch, got: %s", outHuman)
	}
	if !strings.Contains(outHuman, "worktree/path") {
		t.Errorf("RenderFleetView (Human) missing clean worktree relPath, got: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Task 1") {
		t.Errorf("RenderFleetView (Human) missing clean task title, got: %s", outHuman)
	}

	// 2. Agent mode
	var bufAgent bytes.Buffer
	RenderFleetViewAt(&bufAgent, agents, true, nowMs)
	outAgent := bufAgent.String()

	for _, frag := range hostileFragments {
		if strings.Contains(outAgent, frag) {
			t.Errorf("RenderFleetView (Agent) contains unsanitized control sequence %q, got: %s", frag, outAgent)
		}
	}
	if !strings.Contains(outAgent, "project:proj-1") {
		t.Errorf("RenderFleetView (Agent) missing clean project:proj-1, got: %s", outAgent)
	}
	if !strings.Contains(outAgent, "worktree:worktree/path") {
		t.Errorf("RenderFleetView (Agent) missing clean worktree:worktree/path, got: %s", outAgent)
	}
}

func TestFormatHarnessBadge_SanitizesTranscriptText(t *testing.T) {
	// Adversarial harness type containing OSC sequences and non-red/green escape codes
	badHarness := types.HarnessType("\x1b]0;PWNED\x07custom\x1b[34m")
	badge := FormatHarnessBadge(badHarness, types.ActivityBusy)

	if strings.Contains(badge, "\x1b]0;PWNED") || strings.Contains(badge, "\x07") || strings.Contains(badge, "\x1b[34m") {
		t.Errorf("FormatHarnessBadge returned unsanitized sequences: %q", badge)
	}
	if !strings.Contains(badge, "(custom)") {
		t.Errorf("FormatHarnessBadge missing clean label (custom), got: %q", badge)
	}
}

func TestRenderEventDetail_SanitizesHarness(t *testing.T) {
	rec := db.EventRecord{
		ID:        1,
		SessionID: "ses-1",
		AgentID:   "agent-1",
		Harness:   "\x1b]0;PWNED\x07evil-harness\x1b[31m",
		EventType: "tool_call",
		Timestamp: 1000,
	}

	var buf bytes.Buffer
	if err := RenderEventDetail(&buf, rec, false); err != nil {
		t.Fatalf("RenderEventDetail error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "\x1b]0;PWNED") || strings.Contains(out, "\x1b[31m") || strings.Contains(out, "\x07") {
		t.Errorf("RenderEventDetail Harness output contains unsanitized sequence, got: %s", out)
	}
	if !strings.Contains(out, "Harness:         evil-harness") {
		t.Errorf("RenderEventDetail Harness output missing clean harness label, got: %s", out)
	}
}

func TestFormatEventSummary_SanitizesTranscriptText(t *testing.T) {
	cmd := "\x1b]0;EVIL\x07rm -rf / --no-preserve-root\x1b[31m"
	title := "\u009b31mMalicious Run\u009b0m"
	prompt := "\x1b[2JUser prompt\x08 with backspace"
	errContent := "\x1b]2;Crash\x1b\\Fatal crash\x00 occurred"
	question := "\x1b[?25lDelete all files?\u0085"

	events := []db.EventRecord{
		{
			EventType: "run_started",
			Data: types.TelemetryFrameData{
				Title: &title,
			},
		},
		{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolInput: map[string]interface{}{
					"command": cmd,
				},
			},
		},
		{
			EventType: "user_message",
			Data: types.TelemetryFrameData{
				LastPrompt: &prompt,
			},
		},
		{
			EventType: "error",
			Data: types.TelemetryFrameData{
				Content: &errContent,
			},
		},
		{
			EventType: "waiting_question",
			Data: types.TelemetryFrameData{
				Question: &question,
			},
		},
	}

	hostile := []string{"\x1b]0;", "\x1b]2;", "\u009b", "\u0085", "\x08", "\x00", "\x1b[2J", "\x1b[?25l", "\x1b[31m"}

	for _, rec := range events {
		summary := FormatEventSummary(rec)
		for _, h := range hostile {
			if strings.Contains(summary, h) {
				t.Errorf("FormatEventSummary for %s contains unsanitized sequence %q: got %q", rec.EventType, h, summary)
			}
		}
	}
}

func TestRenderSessionList_SanitizesTranscriptText(t *testing.T) {
	sessions := []SessionSummary{
		{
			ID:        "ses-1",
			Title:     "\x1b[31mEvil Title\x1b[0m\x07",
			Directory: "\x1b]0;Title\x07/path/to/repo\x08",
			CreatedAt: time.Now(),
		},
	}

	var bufHuman bytes.Buffer
	RenderSessionList(&bufHuman, sessions, false)
	outHuman := bufHuman.String()

	if strings.Contains(outHuman, "\x1b[31m") || strings.Contains(outHuman, "\x1b]0;") || strings.Contains(outHuman, "\x07") || strings.Contains(outHuman, "\x08") {
		t.Errorf("RenderSessionList (Human) contains unsanitized sequence: %s", outHuman)
	}

	var bufAgent bytes.Buffer
	RenderSessionList(&bufAgent, sessions, true)
	outAgent := bufAgent.String()

	if strings.Contains(outAgent, "\x1b[31m") || strings.Contains(outAgent, "\x1b]0;") || strings.Contains(outAgent, "\x07") || strings.Contains(outAgent, "\x08") {
		t.Errorf("RenderSessionList (Agent) contains unsanitized sequence: %s", outAgent)
	}
}

func TestRenderDiff_SanitizesTranscriptText(t *testing.T) {
	diff := StateDiff{
		StepIndex:  1,
		EventType:  "tool_call",
		LineNumber: 10,
		Mutations: []FieldMutation{
			{
				Field: "Activity",
				From:  "\x1b[31mIDLE\x1b[0m",
				To:    "\u009b32mBUSY\u009b0m\x07",
			},
		},
	}

	var buf bytes.Buffer
	RenderDiff(&buf, diff, false)
	out := buf.String()

	if strings.Contains(out, "\x1b[31m") || strings.Contains(out, "\u009b32m") || strings.Contains(out, "\x07") {
		t.Errorf("RenderDiff contains unsanitized sequence: %s", out)
	}
}

func TestRenderFleetView_NewlinesInTitleReplacedWithSpaces(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	agents := []types.IAgentState{
		{
			Harness: types.HarnessClaudeCode,
			Project: types.IProjectInfo{
				ID:   "proj-newlines",
				Name: "test-project",
			},
			Workspace: types.IWorkspaceInfo{
				Branch: "main",
			},
			Activity: types.IAgentActivity{
				Title:     "Feature A\nPart 2\nPart 3",
				Activity:  types.ActivityBusy,
				Lifecycle: types.LifecycleRunning,
				UpdatedAt: nowMs - 1000,
			},
		},
	}

	var buf bytes.Buffer
	RenderFleetViewAt(&buf, agents, false, nowMs)
	out := buf.String()

	if strings.Contains(out, "Feature A\n") || strings.Contains(out, "Part 2\n") {
		t.Errorf("RenderFleetView contains embedded newlines in title: %s", out)
	}
	if !strings.Contains(out, "Feature A Part 2 Part 3") {
		t.Errorf("RenderFleetView missing space-replaced title: %s", out)
	}
}

func TestRenderAgentViewAgentMode_SanitizesHarness(t *testing.T) {
	agent := types.IAgentState{
		Harness: types.HarnessType("\x1b]0;INJECTED\x07custom-harness\x1b[31m"),
		Activity: types.IAgentActivity{
			Title:     "Test",
			Activity:  types.ActivityIdle,
			Lifecycle: types.LifecycleRunning,
		},
	}

	var buf bytes.Buffer
	RenderAgentViewAt(&buf, agent, true, time.Now().UnixMilli())
	out := buf.String()

	if strings.Contains(out, "\x1b]0;") || strings.Contains(out, "\x1b[31m") || strings.Contains(out, "\x07") {
		t.Errorf("RenderAgentView (Agent) contains unsanitized harness sequence: %s", out)
	}
	if !strings.Contains(out, "harness:custom-harness") {
		t.Errorf("RenderAgentView (Agent) missing clean harness label: %s", out)
	}
}

func TestRenderReplaySummary_SanitizesHarness(t *testing.T) {
	s := ReplaySummary{
		Harness:   types.HarnessType("\x1b]0;EVIL\x07evil-harness\x1b[31m"),
		SessionID: "ses-1",
		AgentID:   "agent-1",
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderReplaySummary(&bufHuman, s, false)
	outHuman := bufHuman.String()

	if strings.Contains(outHuman, "\x1b]0;") || strings.Contains(outHuman, "\x1b[31m") || strings.Contains(outHuman, "\x07") {
		t.Errorf("RenderReplaySummary (Human) contains unsanitized harness sequence: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Session Replay Summary (evil-harness)") {
		t.Errorf("RenderReplaySummary (Human) missing clean harness: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderReplaySummary(&bufAgent, s, true)
	outAgent := bufAgent.String()

	if strings.Contains(outAgent, "\x1b]0;") || strings.Contains(outAgent, "\x1b[31m") || strings.Contains(outAgent, "\x07") {
		t.Errorf("RenderReplaySummary (Agent) contains unsanitized harness sequence: %s", outAgent)
	}
	if !strings.Contains(outAgent, "harness:evil-harness") {
		t.Errorf("RenderReplaySummary (Agent) missing clean harness: %s", outAgent)
	}
}

func TestRenderValidationReport_SanitizesHarness(t *testing.T) {
	r := ValidationReport{
		Harness: types.HarnessType("\x1b]0;EVIL\x07evil-harness\x1b[31m"),
		Target:  "test.jsonl",
		Passed:  true,
		Checks: []ValidationCheck{
			{Name: "check-1", Passed: true},
		},
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderValidationReport(&bufHuman, r, false)
	outHuman := bufHuman.String()

	if strings.Contains(outHuman, "\x1b]0;") || strings.Contains(outHuman, "\x1b[31m") || strings.Contains(outHuman, "\x07") {
		t.Errorf("RenderValidationReport (Human) contains unsanitized harness sequence: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Validation Report: test.jsonl (evil-harness)") {
		t.Errorf("RenderValidationReport (Human) missing clean harness: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderValidationReport(&bufAgent, r, true)
	outAgent := bufAgent.String()

	if strings.Contains(outAgent, "\x1b]0;") || strings.Contains(outAgent, "\x1b[31m") || strings.Contains(outAgent, "\x07") {
		t.Errorf("RenderValidationReport (Agent) contains unsanitized harness sequence: %s", outAgent)
	}
	if !strings.Contains(outAgent, "harness:evil-harness") {
		t.Errorf("RenderValidationReport (Agent) missing clean harness: %s", outAgent)
	}
}

func TestRenderSessionInspect_SanitizesHarness(t *testing.T) {
	rec := &db.SessionRecord{
		Agent: &types.IAgentState{
			ID:        "agent-1",
			SessionID: "ses-1",
			Harness:   types.HarnessType("\x1b]0;EVIL\x07evil-harness\x1b[31m"),
			Activity: types.IAgentActivity{
				Title:     "Test",
				Activity:  types.ActivityIdle,
				Lifecycle: types.LifecycleRunning,
			},
		},
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderSessionInspect(&bufHuman, rec, false)
	outHuman := bufHuman.String()

	if strings.Contains(outHuman, "\x1b]0;") || strings.Contains(outHuman, "\x1b[31m") || strings.Contains(outHuman, "\x07") {
		t.Errorf("RenderSessionInspect (Human) contains unsanitized harness sequence: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Harness:         evil-harness") {
		t.Errorf("RenderSessionInspect (Human) missing clean harness: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderSessionInspect(&bufAgent, rec, true)
	outAgent := bufAgent.String()

	if strings.Contains(outAgent, "\x1b]0;") || strings.Contains(outAgent, "\x1b[31m") || strings.Contains(outAgent, "\x07") {
		t.Errorf("RenderSessionInspect (Agent) contains unsanitized harness sequence: %s", outAgent)
	}
	if !strings.Contains(outAgent, "harness:evil-harness") {
		t.Errorf("RenderSessionInspect (Agent) missing clean harness: %s", outAgent)
	}
}

func TestRenderEventDetail_SanitizesPayloadData(t *testing.T) {
	toolOutput := "\u009b31mhazardous-output\u009b0m"
	rec := db.EventRecord{
		ID:        1,
		SessionID: "ses-1",
		AgentID:   "agent-1",
		Harness:   "claude-code",
		EventType: "tool_result",
		Timestamp: 1000,
		Data: types.TelemetryFrameData{
			ToolOutput: &toolOutput,
		},
	}

	var buf bytes.Buffer
	if err := RenderEventDetail(&buf, rec, false); err != nil {
		t.Fatalf("RenderEventDetail failed: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "\u009b31m") || strings.Contains(out, "\u009b0m") {
		t.Errorf("RenderEventDetail Payload Data contains unsanitized sequence: %s", out)
	}
	if !strings.Contains(out, "hazardous-output") {
		t.Errorf("RenderEventDetail Payload Data missing sanitized text 'hazardous-output': %s", out)
	}
}

func TestRenderEventDetail_SanitizesFieldsAndAgentData(t *testing.T) {
	dataJSON := "{\"output\":\"\x1b[31minjected\x1b[0m\"}"
	rec := db.EventRecord{
		ID:        42,
		SessionID: "\x1b[31mses-hostile\x1b[0m",
		AgentID:   "\x1b[32magent-hostile\x1b[0m",
		EventType: "\x1b[33mtool_call\x1b[0m",
		Timestamp: 1700000000000,
		DataJSON:  dataJSON,
	}

	// Human mode
	var bufHuman bytes.Buffer
	if err := RenderEventDetail(&bufHuman, rec, false); err != nil {
		t.Fatalf("RenderEventDetail failed: %v", err)
	}
	outHuman := bufHuman.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[0m"} {
		if strings.Contains(outHuman, hostile) {
			t.Errorf("RenderEventDetail (Human) contains unsanitized sequence %q: %s", hostile, outHuman)
		}
	}
	if !strings.Contains(outHuman, "Session ID:      ses-hostile") {
		t.Errorf("RenderEventDetail (Human) missing clean session ID: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Agent ID:        agent-hostile") {
		t.Errorf("RenderEventDetail (Human) missing clean agent ID: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Event Type:      tool_call") {
		t.Errorf("RenderEventDetail (Human) missing clean event type: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	if err := RenderEventDetail(&bufAgent, rec, true); err != nil {
		t.Fatalf("RenderEventDetail agent mode failed: %v", err)
	}
	outAgent := bufAgent.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[0m"} {
		if strings.Contains(outAgent, hostile) {
			t.Errorf("RenderEventDetail (Agent) contains unsanitized sequence %q: %s", hostile, outAgent)
		}
	}
	if !strings.Contains(outAgent, "session_id:ses-hostile") {
		t.Errorf("RenderEventDetail (Agent) missing clean session_id: %s", outAgent)
	}
	if !strings.Contains(outAgent, "agent_id:agent-hostile") {
		t.Errorf("RenderEventDetail (Agent) missing clean agent_id: %s", outAgent)
	}
	if !strings.Contains(outAgent, "type:tool_call") {
		t.Errorf("RenderEventDetail (Agent) missing clean type: %s", outAgent)
	}
	if !strings.Contains(outAgent, `data:{"output":"injected"}`) {
		t.Errorf("RenderEventDetail (Agent) missing sanitized data JSON: %s", outAgent)
	}
}

func TestRenderSessionInspect_SanitizesIdentifiersAndRuns(t *testing.T) {
	now := time.Now().UnixMilli()
	rec := &db.SessionRecord{
		Agent: &types.IAgentState{
			ID:        "\x1b[31magent-hostile\x1b[0m",
			SessionID: "\x1b[32mses-hostile\x1b[0m",
			Harness:   types.HarnessClaudeCode,
			Activity: types.IAgentActivity{
				Title:     "Test",
				Activity:  types.ActivityIdle,
				Lifecycle: types.LifecycleRunning,
				UpdatedAt: now,
			},
		},
		Runs: []db.RunRecord{
			{
				ID:             "\x1b[33mrun-hostile\x1b[0m",
				PID:            1234,
				HarnessVersion: "\x1b[34m1.2.3\x1b[0m",
				StartedAt:      now - 5000,
			},
		},
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderSessionInspect(&bufHuman, rec, false)
	outHuman := bufHuman.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m", "\x1b[0m"} {
		if strings.Contains(outHuman, hostile) {
			t.Errorf("RenderSessionInspect (Human) contains unsanitized sequence %q: %s", hostile, outHuman)
		}
	}
	if !strings.Contains(outHuman, "Agent ID:        agent-hostile") {
		t.Errorf("RenderSessionInspect (Human) missing clean Agent ID: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Session ID:      ses-hostile") {
		t.Errorf("RenderSessionInspect (Human) missing clean Session ID: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Run run-hostile: PID 1234") {
		t.Errorf("RenderSessionInspect (Human) missing clean Run ID: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderSessionInspect(&bufAgent, rec, true)
	outAgent := bufAgent.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m", "\x1b[0m"} {
		if strings.Contains(outAgent, hostile) {
			t.Errorf("RenderSessionInspect (Agent) contains unsanitized sequence %q: %s", hostile, outAgent)
		}
	}
	if !strings.Contains(outAgent, "id:agent-hostile") {
		t.Errorf("RenderSessionInspect (Agent) missing clean agent id: %s", outAgent)
	}
	if !strings.Contains(outAgent, "session_id:ses-hostile") {
		t.Errorf("RenderSessionInspect (Agent) missing clean session_id: %s", outAgent)
	}
	if !strings.Contains(outAgent, "db_run id:run-hostile") {
		t.Errorf("RenderSessionInspect (Agent) missing clean db_run id: %s", outAgent)
	}
	if !strings.Contains(outAgent, "harness_version:1.2.3") {
		t.Errorf("RenderSessionInspect (Agent) missing clean harness_version: %s", outAgent)
	}
}

func TestRenderSessionList_SanitizesSessionID(t *testing.T) {
	sessions := []SessionSummary{
		{
			ID:        "\x1b[31mses-hostile\x1b[0m",
			Title:     "Clean Title",
			Directory: "/path/to/repo",
			CreatedAt: time.Now(),
		},
	}

	var bufHuman bytes.Buffer
	RenderSessionList(&bufHuman, sessions, false)
	outHuman := bufHuman.String()
	if strings.Contains(outHuman, "\x1b[31m") || strings.Contains(outHuman, "\x1b[0m") {
		t.Errorf("RenderSessionList (Human) contains unsanitized session ID: %s", outHuman)
	}
	if !strings.Contains(outHuman, "ses-hostile") {
		t.Errorf("RenderSessionList (Human) missing clean session ID: %s", outHuman)
	}

	var bufAgent bytes.Buffer
	RenderSessionList(&bufAgent, sessions, true)
	outAgent := bufAgent.String()
	if strings.Contains(outAgent, "\x1b[31m") || strings.Contains(outAgent, "\x1b[0m") {
		t.Errorf("RenderSessionList (Agent) contains unsanitized session ID: %s", outAgent)
	}
	if !strings.Contains(outAgent, "session_id:ses-hostile") {
		t.Errorf("RenderSessionList (Agent) missing clean session ID: %s", outAgent)
	}
}

func TestRenderReplaySummary_SanitizesSessionAndAgentID(t *testing.T) {
	s := ReplaySummary{
		Harness:   types.HarnessClaudeCode,
		SessionID: "\x1b[31mses-hostile\x1b[0m",
		AgentID:   "\x1b[32magent-hostile\x1b[0m",
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderReplaySummary(&bufHuman, s, false)
	outHuman := bufHuman.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[0m"} {
		if strings.Contains(outHuman, hostile) {
			t.Errorf("RenderReplaySummary (Human) contains unsanitized sequence %q: %s", hostile, outHuman)
		}
	}
	if !strings.Contains(outHuman, "Session ID:      ses-hostile") {
		t.Errorf("RenderReplaySummary (Human) missing clean Session ID: %s", outHuman)
	}
	if !strings.Contains(outHuman, "Agent ID:        agent-hostile") {
		t.Errorf("RenderReplaySummary (Human) missing clean Agent ID: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderReplaySummary(&bufAgent, s, true)
	outAgent := bufAgent.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[0m"} {
		if strings.Contains(outAgent, hostile) {
			t.Errorf("RenderReplaySummary (Agent) contains unsanitized sequence %q: %s", hostile, outAgent)
		}
	}
	if !strings.Contains(outAgent, "session_id:ses-hostile") {
		t.Errorf("RenderReplaySummary (Agent) missing clean session_id: %s", outAgent)
	}
	if !strings.Contains(outAgent, "agent_id:agent-hostile") {
		t.Errorf("RenderReplaySummary (Agent) missing clean agent_id: %s", outAgent)
	}
}

func TestRenderComparison_SanitizesSessionAndAgentID(t *testing.T) {
	comp := &ComparisonResult{
		DBPath:    "/tmp/test.db",
		SessionID: "\x1b[31mses-hostile\x1b[0m",
		AgentID:   "\x1b[32magent-hostile\x1b[0m",
	}

	var bufAgent bytes.Buffer
	RenderComparison(&bufAgent, comp, true)
	outAgent := bufAgent.String()

	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[0m"} {
		if strings.Contains(outAgent, hostile) {
			t.Errorf("RenderComparison (Agent) contains unsanitized sequence %q: %s", hostile, outAgent)
		}
	}
	if !strings.Contains(outAgent, "session_id:ses-hostile") {
		t.Errorf("RenderComparison (Agent) missing clean session_id: %s", outAgent)
	}
	if !strings.Contains(outAgent, "agent_id:agent-hostile") {
		t.Errorf("RenderComparison (Agent) missing clean agent_id: %s", outAgent)
	}
}

func TestRenderSessionInspect_SanitizesHostname(t *testing.T) {
	rec := &db.SessionRecord{
		Agent: &types.IAgentState{
			ID:        "agent-1",
			SessionID: "ses-1",
			Harness:   types.HarnessClaudeCode,
			Environment: types.IEnvironmentInfo{
				Label:    "local",
				Hostname: "\x1b[35mhostile-host\x1b[0m",
			},
			Workspace: types.IWorkspaceInfo{
				CWD:    "/path/to/repo",
				Branch: "main",
			},
			Project: types.IProjectInfo{
				Name: "test-project",
			},
			Activity: types.IAgentActivity{
				Title:     "Test",
				Activity:  types.ActivityIdle,
				Lifecycle: types.LifecycleRunning,
			},
		},
	}

	var bufHuman bytes.Buffer
	RenderSessionInspect(&bufHuman, rec, false)
	outHuman := bufHuman.String()

	for _, hostile := range []string{"\x1b[35m", "\x1b[0m"} {
		if strings.Contains(outHuman, hostile) {
			t.Errorf("RenderSessionInspect (Human) contains unsanitized sequence %q: %s", hostile, outHuman)
		}
	}
	if !strings.Contains(outHuman, "local (hostile-host)") {
		t.Errorf("RenderSessionInspect (Human) missing clean environment hostname: %s", outHuman)
	}
}

func TestRenderEventsTable_SanitizesEventType(t *testing.T) {
	events := []db.EventRecord{
		{
			ID:        1,
			SessionID: "ses-1",
			AgentID:   "agent-1",
			EventType: "\x1b[31mcustom_event\x1b[0m",
			Timestamp: 1700000000000,
		},
	}

	// Human mode
	var bufHuman bytes.Buffer
	if err := RenderEventsTable(&bufHuman, events, false); err != nil {
		t.Fatalf("RenderEventsTable (Human) failed: %v", err)
	}
	outHuman := bufHuman.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[0m"} {
		if strings.Contains(outHuman, hostile) {
			t.Errorf("RenderEventsTable (Human) contains unsanitized sequence %q: %s", hostile, outHuman)
		}
	}
	if !strings.Contains(outHuman, "custom_event") {
		t.Errorf("RenderEventsTable (Human) missing clean event type: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	if err := RenderEventsTable(&bufAgent, events, true); err != nil {
		t.Fatalf("RenderEventsTable (Agent) failed: %v", err)
	}
	outAgent := bufAgent.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[0m"} {
		if strings.Contains(outAgent, hostile) {
			t.Errorf("RenderEventsTable (Agent) contains unsanitized sequence %q: %s", hostile, outAgent)
		}
	}
	if !strings.Contains(outAgent, "type:custom_event") {
		t.Errorf("RenderEventsTable (Agent) missing clean event type: %s", outAgent)
	}
}

func TestRenderDiff_SanitizesEventType(t *testing.T) {
	diff := StateDiff{
		StepIndex:  1,
		EventType:  "\x1b[31mhostile_event\x1b[0m",
		LineNumber: 10,
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderDiff(&bufHuman, diff, false)
	outHuman := bufHuman.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[0m"} {
		if strings.Contains(outHuman, hostile) {
			t.Errorf("RenderDiff (Human) contains unsanitized sequence %q: %s", hostile, outHuman)
		}
	}
	if !strings.Contains(outHuman, "[hostile_event]") {
		t.Errorf("RenderDiff (Human) missing clean event type: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderDiff(&bufAgent, diff, true)
	outAgent := bufAgent.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[0m"} {
		if strings.Contains(outAgent, hostile) {
			t.Errorf("RenderDiff (Agent) contains unsanitized sequence %q: %s", hostile, outAgent)
		}
	}
	if !strings.Contains(outAgent, "event:hostile_event") {
		t.Errorf("RenderDiff (Agent) missing clean event type: %s", outAgent)
	}
}

func TestRenderScannerAudit_SanitizesTranscriptText(t *testing.T) {
	delta := int64(50)
	records := []scanner.AuditRecord{
		{
			Harness:       "\x1b[31mpi\x1b[0m",
			PID:           1234,
			CreatedAt:     1758400000000,
			CWD:           "\x1b[32m/path/to/repo\x1b[0m",
			SessionID:     "\x1b[33mses-hostile\x1b[0m",
			DeltaMs:       &delta,
			Status:        scanner.StatusPaired,
			StatusDetails: "\x1b[34mACTIVE\x1b[0m",
		},
	}

	// Human mode
	var bufHuman bytes.Buffer
	RenderScannerAudit(&bufHuman, records, false)
	outHuman := bufHuman.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m", "\x1b[0m"} {
		if strings.Contains(outHuman, hostile) {
			t.Errorf("RenderScannerAudit (Human) contains unsanitized sequence %q: %s", hostile, outHuman)
		}
	}
	if !strings.Contains(outHuman, "PAIRED (ACTIVE)") {
		t.Errorf("RenderScannerAudit (Human) missing clean status details: %s", outHuman)
	}
	if !strings.Contains(outHuman, "ses-hostile") {
		t.Errorf("RenderScannerAudit (Human) missing clean session id: %s", outHuman)
	}
	if !strings.Contains(outHuman, "/path/to/repo") {
		t.Errorf("RenderScannerAudit (Human) missing clean cwd: %s", outHuman)
	}

	// Agent mode
	var bufAgent bytes.Buffer
	RenderScannerAudit(&bufAgent, records, true)
	outAgent := bufAgent.String()
	for _, hostile := range []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m", "\x1b[0m"} {
		if strings.Contains(outAgent, hostile) {
			t.Errorf("RenderScannerAudit (Agent) contains unsanitized sequence %q: %s", hostile, outAgent)
		}
	}
	if !strings.Contains(outAgent, "harness:pi") {
		t.Errorf("RenderScannerAudit (Agent) missing clean harness: %s", outAgent)
	}
	if !strings.Contains(outAgent, `cwd:"/path/to/repo"`) {
		t.Errorf("RenderScannerAudit (Agent) missing clean cwd: %s", outAgent)
	}
	if !strings.Contains(outAgent, `session_id:"ses-hostile"`) {
		t.Errorf("RenderScannerAudit (Agent) missing clean session_id: %s", outAgent)
	}
	if !strings.Contains(outAgent, `status_details:"ACTIVE"`) {
		t.Errorf("RenderScannerAudit (Agent) missing clean status_details: %s", outAgent)
	}
}
