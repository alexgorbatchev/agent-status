package session

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
)

func TestComputeHandoffContext_EmptyEvents(t *testing.T) {
	h := ComputeHandoffContext("/test/session.jsonl", nil, 5)
	if h.FilePath != "/test/session.jsonl" {
		t.Errorf("FilePath = %s", h.FilePath)
	}
	if h.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d", h.TotalEvents)
	}
}

func TestComputeHandoffContext_FullFlow(t *testing.T) {
	evs := []parser.ParsedEvent{
		{
			EventType: "run_started",
			Timestamp: 1000,
			CWD:       "/workspace/repo",
			GitBranch: "feat/resume-work",
			Data: types.TelemetryFrameData{
				Title: func() *string { v := "Refactor Auth"; return &v }(),
			},
		},
		{
			EventType: "turn_start",
			Timestamp: 2000,
			Data: types.TelemetryFrameData{
				Content: func() *string { v := "<local-command-caveat>Ignore</local-command-caveat>Initial goal: migrate to JWT"; return &v }(),
			},
		},
		{
			EventType: "tool_call",
			Timestamp: 3000,
			Data: types.TelemetryFrameData{
				ToolName:   func() *string { v := "Edit"; return &v }(),
				ToolCallID: func() *string { v := "c1"; return &v }(),
				ToolInput: map[string]interface{}{
					"file_path": "auth/jwt.go",
				},
				FileModification: &types.FileModification{
					Action:   "EDIT",
					FilePath: "auth/jwt.go",
					Hunks:    make([]types.DiffHunk, 1),
				},
			},
		},
		{
			EventType: "tool_result",
			Timestamp: 3500,
			Data: types.TelemetryFrameData{
				ToolCallID: func() *string { v := "c1"; return &v }(),
				ToolOutput: func() *string { v := "Updated auth/jwt.go successfully"; return &v }(),
			},
		},
		{
			EventType: "turn_start",
			Timestamp: 4000,
			Data: types.TelemetryFrameData{
				Content: func() *string { v := "Now run the tests please"; return &v }(),
			},
		},
		{
			EventType: "tool_call",
			Timestamp: 4500,
			Data: types.TelemetryFrameData{
				ToolName:   func() *string { v := "Bash"; return &v }(),
				ToolCallID: func() *string { v := "c2"; return &v }(),
				ToolInput: map[string]interface{}{
					"command": "go test ./auth",
				},
			},
		},
		{
			EventType: "tool_result",
			Timestamp: 5000,
			Data: types.TelemetryFrameData{
				ToolCallID: func() *string { v := "c2"; return &v }(),
				IsError:    func() *bool { v := true; return &v }(),
				ToolOutput: func() *string { v := "FAIL: TestTokenExpiry"; return &v }(),
			},
		},
		{
			EventType: "turn_end",
			Timestamp: 5500,
			Data: types.TelemetryFrameData{
				Content: func() *string { v := "I have migrated the JWT handler, but TestTokenExpiry is failing due to clock skew."; return &v }(),
			},
		},
		{
			EventType: "run_exited",
			Timestamp: 6000,
			Data: types.TelemetryFrameData{
				ExitCode: func() *int { v := 1; return &v }(),
			},
		},
	}

	h := ComputeHandoffContext("/test/session.jsonl", evs, 0) // test recentLimit <= 0 fallback

	if h.Title != "Refactor Auth" {
		t.Errorf("Title = %q", h.Title)
	}
	if h.CWD != "/workspace/repo" {
		t.Errorf("CWD = %q", h.CWD)
	}
	if h.GitBranch != "feat/resume-work" {
		t.Errorf("GitBranch = %q", h.GitBranch)
	}
	if h.InitialObjective != "Initial goal: migrate to JWT" {
		t.Errorf("InitialObjective = %q", h.InitialObjective)
	}
	if h.LatestUserPrompt != "Now run the tests please" {
		t.Errorf("LatestUserPrompt = %q", h.LatestUserPrompt)
	}
	if !strings.Contains(h.FinalAssistantMessage, "TestTokenExpiry is failing") {
		t.Errorf("FinalAssistantMessage = %q", h.FinalAssistantMessage)
	}
	if len(h.FilesModified) != 1 || h.FilesModified[0].FilePath != "auth/jwt.go" {
		t.Errorf("FilesModified = %+v", h.FilesModified)
	}
	if len(h.RecentActivity) != 2 {
		t.Errorf("RecentActivity = %+v, want 2", h.RecentActivity)
	}
	if len(h.RecentErrors) != 1 || !strings.Contains(h.RecentErrors[0].Message, "TestTokenExpiry") {
		t.Errorf("RecentErrors = %+v", h.RecentErrors)
	}
	if h.ExitCode == nil || *h.ExitCode != 1 {
		t.Errorf("ExitCode = %v, want 1", h.ExitCode)
	}
}

func TestRenderHandoff_Formats(t *testing.T) {
	exit := 0
	h := HandoffContext{
		FilePath:              "/path/to/claude.jsonl",
		Title:                 "Feature Implementation",
		CWD:                   "/work/repo",
		GitBranch:             "main",
		TotalEvents:           25,
		TurnCount:             3,
		DurationMs:            180000, // 3m
		InitialObjective:      "Add new endpoint",
		LatestUserPrompt:      "Check linting",
		FinalAssistantMessage: "Everything completed and clean.",
		FilesModified: []FileModificationStat{
			{FilePath: "api/routes.go", Action: "EDIT", Hunks: 1},
		},
		RecentActivity: []RecentToolActivity{
			{ToolName: "Bash", Summary: "git status", Status: "OK", OutputLen: 50},
		},
		RecentErrors: nil,
		ExitCode:     &exit,
	}

	// 1. JSON
	var jsonBuf bytes.Buffer
	if err := RenderHandoff(&jsonBuf, h, false, true); err != nil {
		t.Fatalf("JSON RenderHandoff error: %v", err)
	}
	var parsed HandoffContext
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.InitialObjective != "Add new endpoint" {
		t.Errorf("parsed JSON InitialObjective = %s", parsed.InitialObjective)
	}

	// 2. Human
	var humanBuf bytes.Buffer
	if err := RenderHandoff(&humanBuf, h, false, false); err != nil {
		t.Fatalf("Human RenderHandoff error: %v", err)
	}
	humanOut := humanBuf.String()
	if !strings.Contains(humanOut, "[SESSION HANDOFF & CONTINUATION CONTEXT]") {
		t.Errorf("missing handoff header: %s", humanOut)
	}
	if !strings.Contains(humanOut, "Add new endpoint") {
		t.Errorf("missing initial objective: %s", humanOut)
	}
	if !strings.Contains(humanOut, "Everything completed and clean.") {
		t.Errorf("missing final message: %s", humanOut)
	}

	// 3. Agent
	var agentBuf bytes.Buffer
	if err := RenderHandoff(&agentBuf, h, true, false); err != nil {
		t.Fatalf("Agent RenderHandoff error: %v", err)
	}
	agentOut := agentBuf.String()
	if !strings.Contains(agentOut, "CONTINUATION_CONTEXT") {
		t.Errorf("missing CONTINUATION_CONTEXT: %s", agentOut)
	}
	if strings.Contains(agentOut, "--------------------------------------------------------------------------------") {
		t.Errorf("dividers forbidden in agent mode: %s", agentOut)
	}
	if !strings.Contains(agentOut, "INITIAL_OBJECTIVE:\nAdd new endpoint") {
		t.Errorf("missing objective in agent mode: %s", agentOut)
	}
	if !strings.Contains(agentOut, "FINAL_ASSISTANT_NOTE:\nEverything completed and clean.") {
		t.Errorf("missing final note in agent mode: %s", agentOut)
	}
	if !strings.Contains(agentOut, "RECENT_ERRORS: none") {
		t.Errorf("expected RECENT_ERRORS: none, got: %s", agentOut)
	}
}
