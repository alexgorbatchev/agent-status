package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
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
