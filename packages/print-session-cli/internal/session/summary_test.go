package session

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-parser"
)

func TestComputeSessionSummary_EmptyEvents(t *testing.T) {
	s := ComputeSessionSummary("/path/to/test.jsonl", nil)
	if s.FilePath != "/path/to/test.jsonl" {
		t.Errorf("FilePath = %s", s.FilePath)
	}
	if s.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d", s.TotalEvents)
	}
}

func TestComputeSessionSummary_RichEvents(t *testing.T) {
	evs := []parser.ParsedEvent{
		{
			EventType: "run_started",
			Timestamp: 1000,
			CWD:       "/test/cwd",
			Data: types.TelemetryFrameData{
				Title: func() *string { v := "Rich Test Session"; return &v }(),
				Model: func() *string { v := "claude-3-7-sonnet"; return &v }(),
			},
		},
		{
			EventType: "turn_start",
			Timestamp: 2000,
			Data: types.TelemetryFrameData{
				Content: func() *string { v := "Do something"; return &v }(),
			},
		},
		{
			EventType: "tool_call",
			Timestamp: 3000,
			Data: types.TelemetryFrameData{
				ToolName:   func() *string { v := "Edit"; return &v }(),
				ToolCallID: func() *string { v := "call_1"; return &v }(),
				FileModification: &types.FileModification{
					Action:   "EDIT",
					FilePath: "src/main.go",
					Hunks:    make([]types.DiffHunk, 2),
				},
			},
		},
		{
			EventType: "tool_call",
			Timestamp: 3500,
			Data: types.TelemetryFrameData{
				ToolName:   func() *string { v := "Edit"; return &v }(),
				ToolCallID: func() *string { v := "call_1b"; return &v }(),
				FileModification: &types.FileModification{
					Action:   "EDIT",
					FilePath: "src/main.go",
					Hunks:    make([]types.DiffHunk, 1),
				},
			},
		},
		{
			EventType: "tool_result",
			Timestamp: 4000,
			Data: types.TelemetryFrameData{
				ToolCallID: func() *string { v := "call_1"; return &v }(),
				IsError:    func() *bool { v := true; return &v }(),
				ToolOutput: func() *string { v := "File modification failed"; return &v }(),
			},
		},
		{
			EventType: "turn_end",
			Timestamp: 5000,
			Data: types.TelemetryFrameData{
				Usage: &types.ITokenUsage{
					Input:     100,
					Output:    50,
					CacheRead: 25,
					Total:     175,
				},
				RawHarnessCost: func() *float64 { v := 0.05; return &v }(),
			},
		},
		{
			EventType: "error",
			Timestamp: 6000,
			Data: types.TelemetryFrameData{
				Content: func() *string { v := "Harness crash"; return &v }(),
			},
		},
		{
			EventType: "run_exited",
			Timestamp: 7000,
			Data: types.TelemetryFrameData{
				ExitCode: func() *int { v := 1; return &v }(),
			},
		},
	}

	s := ComputeSessionSummary("/test/session.jsonl", evs)

	if s.Title != "Rich Test Session" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.CWD != "/test/cwd" {
		t.Errorf("CWD = %q", s.CWD)
	}
	if s.TurnCount != 1 {
		t.Errorf("TurnCount = %d", s.TurnCount)
	}
	if s.DurationMs != 6000 {
		t.Errorf("DurationMs = %d, want 6000", s.DurationMs)
	}
	if len(s.Models) != 1 || s.Models[0] != "claude-3-7-sonnet" {
		t.Errorf("Models = %v", s.Models)
	}
	if s.TokensTotal != 175 {
		t.Errorf("TokensTotal = %d", s.TokensTotal)
	}
	if s.CostUSD != 0.05 {
		t.Errorf("CostUSD = %f", s.CostUSD)
	}
	if len(s.Tools) != 1 || s.Tools[0].Name != "Edit" || s.Tools[0].Calls != 2 {
		t.Errorf("Tools = %+v", s.Tools)
	}
	if len(s.FilesModified) != 1 || s.FilesModified[0].Hunks != 3 {
		t.Errorf("FilesModified = %+v", s.FilesModified)
	}
	if len(s.Errors) != 2 {
		t.Errorf("Errors = %+v, want 2", s.Errors)
	}
	if s.ExitCode == nil || *s.ExitCode != 1 {
		t.Errorf("ExitCode = %v, want 1", s.ExitCode)
	}
}

func TestRenderSummary_Formats(t *testing.T) {
	exit := 0
	s := SessionSummary{
		FilePath:    "/path/to/test.jsonl",
		Title:       "Test Session",
		CWD:         "/workspace",
		DurationMs:  125000, // 2m05s
		TotalEvents: 10,
		TurnCount:   2,
		Models:      []string{"claude-3-7-sonnet"},
		TokensIn:    100,
		TokensOut:   50,
		TokensCache: 20,
		TokensTotal: 170,
		CostUSD:     0.02,
		Tools: []ToolUsageStat{
			{Name: "Bash", Calls: 5, ErrorCalls: 1},
		},
		FilesModified: []FileModificationStat{
			{FilePath: "main.go", Action: "EDIT", Hunks: 2},
		},
		Errors: []ErrorRecord{
			{Timestamp: 1000, Source: "tool_call (call_1)", Message: "exit code 1"},
		},
		ExitCode: &exit,
	}

	// 1. JSON output
	var jsonBuf bytes.Buffer
	if err := RenderSummary(&jsonBuf, s, false, true); err != nil {
		t.Fatalf("JSON RenderSummary error: %v", err)
	}
	var parsed SessionSummary
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Title != "Test Session" {
		t.Errorf("parsed JSON Title = %s", parsed.Title)
	}

	// 2. Human output
	var humanBuf bytes.Buffer
	if err := RenderSummary(&humanBuf, s, false, false); err != nil {
		t.Fatalf("Human RenderSummary error: %v", err)
	}
	humanOut := humanBuf.String()
	if !strings.Contains(humanOut, "[SESSION SUMMARY]") {
		t.Errorf("expected [SESSION SUMMARY] in human mode, got: %s", humanOut)
	}
	if !strings.Contains(humanOut, "2m05s") {
		t.Errorf("expected duration 2m05s, got: %s", humanOut)
	}

	// 3. Agent output
	var agentBuf bytes.Buffer
	if err := RenderSummary(&agentBuf, s, true, false); err != nil {
		t.Fatalf("Agent RenderSummary error: %v", err)
	}
	agentOut := agentBuf.String()
	if !strings.Contains(agentOut, "SESSION_SUMMARY") {
		t.Errorf("expected SESSION_SUMMARY in agent mode, got: %s", agentOut)
	}
	if strings.Contains(agentOut, "--------------------------------------------------------------------------------") {
		t.Errorf("dividers forbidden in agent mode: %s", agentOut)
	}
	if !strings.Contains(agentOut, "Bash(5,err:1)") {
		t.Errorf("expected Bash(5,err:1) in tools list: %s", agentOut)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "0s"},
		{-10, "0s"},
		{5000, "5.0s"},
		{65000, "1m05s"},
		{3665000, "1h01m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.ms)
		if got != tt.want {
			t.Errorf("formatDuration(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}
