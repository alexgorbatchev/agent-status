package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
)

func strPtr(s string) *string {
	return &s
}

func TestFormatEventSummary(t *testing.T) {
	t.Run("run_started with pid and harness", func(t *testing.T) {
		rec := db.EventRecord{
			EventType: "run_started",
			Harness:   "pi",
			PID:       27680,
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "pid=27680") || !strings.Contains(summary, "harness=pi") {
			t.Errorf("expected summary with pid and harness, got %q", summary)
		}
	})

	t.Run("run_exited with pid", func(t *testing.T) {
		rec := db.EventRecord{
			EventType: "run_exited",
			PID:       27680,
		}
		summary := FormatEventSummary(rec)
		if summary != "pid=27680" {
			t.Errorf("got %q, want 'pid=27680'", summary)
		}
	})

	t.Run("turn_end with usage", func(t *testing.T) {
		rec := db.EventRecord{
			EventType: "turn_end",
			Data: types.TelemetryFrameData{
				Usage: &types.ITokenUsage{Total: 242260},
			},
		}
		summary := FormatEventSummary(rec)
		if summary != "tokens=242260" {
			t.Errorf("got %q, want 'tokens=242260'", summary)
		}
	})

	t.Run("turn_end intermediate tool-use with usage", func(t *testing.T) {
		toolReasons := []string{"tool_use", "toolUse", "tool_calls", "tool_call"}
		for _, reason := range toolReasons {
			t.Run(reason, func(t *testing.T) {
				rec := db.EventRecord{
					EventType: "turn_end",
					Data: types.TelemetryFrameData{
						StopReason: strPtr(reason),
						Usage:      &types.ITokenUsage{Total: 1500},
					},
				}
				if !IsIntermediateToolTurnEnd(rec) {
					t.Errorf("expected IsIntermediateToolTurnEnd=true for stopReason=%s", reason)
				}
				summary := FormatEventSummary(rec)
				if summary != "(api_response) tokens=1500" {
					t.Errorf("got %q, want '(api_response) tokens=1500'", summary)
				}
				if label := FormatEventLabel(rec); label != "API RESPONSE" {
					t.Errorf("got %q, want 'API RESPONSE'", label)
				}
			})
		}
	})

	t.Run("turn_end intermediate tool-use without usage", func(t *testing.T) {
		rec := db.EventRecord{
			EventType: "turn_end",
			Data: types.TelemetryFrameData{
				StopReason: strPtr("tool_use"),
			},
		}
		if !IsIntermediateToolTurnEnd(rec) {
			t.Errorf("expected IsIntermediateToolTurnEnd=true")
		}
		summary := FormatEventSummary(rec)
		if summary != "(api_response)" {
			t.Errorf("got %q, want '(api_response)'", summary)
		}
		if label := FormatEventLabel(rec); label != "API RESPONSE" {
			t.Errorf("got %q, want 'API RESPONSE'", label)
		}
	})

	t.Run("turn_end intermediate tool-use from DataJSON", func(t *testing.T) {
		rec := db.EventRecord{
			EventType: "turn_end",
			DataJSON:  `{"stopReason": "tool_calls", "usage": {"total": 3500}}`,
		}
		if !IsIntermediateToolTurnEnd(rec) {
			t.Errorf("expected IsIntermediateToolTurnEnd=true for DataJSON")
		}
		summary := FormatEventSummary(rec)
		if summary != "(api_response) tokens=3500" {
			t.Errorf("got %q, want '(api_response) tokens=3500'", summary)
		}
	})

	t.Run("turn_end terminal stop reasons", func(t *testing.T) {
		terminalReasons := []*string{
			strPtr("end_turn"),
			strPtr("max_tokens"),
			strPtr("stop_sequence"),
			strPtr("refusal"),
			nil,
		}
		for _, reason := range terminalReasons {
			name := "nil"
			if reason != nil {
				name = *reason
			}
			t.Run(name, func(t *testing.T) {
				rec := db.EventRecord{
					EventType: "turn_end",
					Data: types.TelemetryFrameData{
						StopReason: reason,
						Usage:      &types.ITokenUsage{Total: 2000},
					},
				}
				if IsIntermediateToolTurnEnd(rec) {
					t.Errorf("expected IsIntermediateToolTurnEnd=false for %s", name)
				}
				summary := FormatEventSummary(rec)
				if summary != "tokens=2000" {
					t.Errorf("got %q, want 'tokens=2000'", summary)
				}
				if label := FormatEventLabel(rec); label != "turn_end" {
					t.Errorf("got %q, want 'turn_end'", label)
				}
			})
		}
	})

	t.Run("tool_result success", func(t *testing.T) {
		isErr := false
		exitCode := 0
		rec := db.EventRecord{
			EventType: "tool_result",
			Data: types.TelemetryFrameData{
				ToolName: strPtr("bash"),
				IsError:  &isErr,
				ExitCode: &exitCode,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "tool=bash") || !strings.Contains(summary, "status=success") {
			t.Errorf("got %q, want 'tool=bash status=success'", summary)
		}
	})

	t.Run("tool_result error with exit code", func(t *testing.T) {
		isErr := true
		exitCode := 127
		rec := db.EventRecord{
			EventType: "tool_result",
			Data: types.TelemetryFrameData{
				ToolName: strPtr("bash"),
				IsError:  &isErr,
				ExitCode: &exitCode,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "status=error") || !strings.Contains(summary, "exit=127") {
			t.Errorf("got %q, want error status with exit=127", summary)
		}
	})

	t.Run("tool_call command", func(t *testing.T) {
		rec := db.EventRecord{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolName:  strPtr("bash"),
				ToolInput: map[string]interface{}{"command": "npm test"},
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "tool=bash") || !strings.Contains(summary, "cmd=") {
			t.Errorf("got %q, want tool=bash cmd=...", summary)
		}
	})

	t.Run("thinking length", func(t *testing.T) {
		content := "Thinking about solving the problem..."
		rec := db.EventRecord{
			EventType: "thinking",
			Data: types.TelemetryFrameData{
				Content: &content,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "len=") {
			t.Errorf("got %q, want len=... chars", summary)
		}
	})

	t.Run("error message", func(t *testing.T) {
		msg := "connection refused"
		rec := db.EventRecord{
			EventType: "error",
			Data: types.TelemetryFrameData{
				Content: &msg,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "connection refused") {
			t.Errorf("got %q, want message in summary", summary)
		}
	})

	t.Run("user_message with lastPrompt", func(t *testing.T) {
		prompt := "Please write a test function for this module"
		rec := db.EventRecord{
			EventType: "user_message",
			Data: types.TelemetryFrameData{
				LastPrompt: &prompt,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "Please write") {
			t.Errorf("got %q, want user prompt", summary)
		}
	})

	t.Run("user_message with content fallback", func(t *testing.T) {
		content := "User question here"
		rec := db.EventRecord{
			EventType: "user_message",
			Data: types.TelemetryFrameData{
				Content: &content,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "User question") {
			t.Errorf("got %q, want user content", summary)
		}
	})

	t.Run("assistant_message with content", func(t *testing.T) {
		content := "I will create the necessary files for you."
		rec := db.EventRecord{
			EventType: "assistant_message",
			Data: types.TelemetryFrameData{
				Content: &content,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "I will create") {
			t.Errorf("got %q, want assistant content", summary)
		}
	})

	t.Run("waiting_question with question", func(t *testing.T) {
		q := "Do you want to apply this diff?"
		rec := db.EventRecord{
			EventType: "waiting_question",
			Data: types.TelemetryFrameData{
				Question: &q,
			},
		}
		summary := FormatEventSummary(rec)
		if !strings.Contains(summary, "question=") {
			t.Errorf("got %q, want question", summary)
		}
	})

	t.Run("tool_call with path, file_path, and pattern", func(t *testing.T) {
		recPath := db.EventRecord{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolName:  strPtr("read"),
				ToolInput: map[string]interface{}{"path": "src/main.go"},
			},
		}
		if s := FormatEventSummary(recPath); !strings.Contains(s, "path=src/main.go") {
			t.Errorf("got %q, want path=src/main.go", s)
		}

		recFile := db.EventRecord{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolName:  strPtr("write"),
				ToolInput: map[string]interface{}{"file_path": "src/out.go"},
			},
		}
		if s := FormatEventSummary(recFile); !strings.Contains(s, "file=src/out.go") {
			t.Errorf("got %q, want file=src/out.go", s)
		}

		recPat := db.EventRecord{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolName:  strPtr("grep"),
				ToolInput: map[string]interface{}{"pattern": "foo.*bar"},
			},
		}
		if s := FormatEventSummary(recPat); !strings.Contains(s, "pattern=") {
			t.Errorf("got %q, want pattern=", s)
		}

		recFileMod := db.EventRecord{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolName: strPtr("Edit"),
				FileModification: &types.FileModification{
					Action:   "EDIT",
					FilePath: "src/normalized.go",
				},
			},
		}
		if s := FormatEventSummary(recFileMod); !strings.Contains(s, "file=src/normalized.go") {
			t.Errorf("got %q, want file=src/normalized.go", s)
		}
	})

	t.Run("run_started with title and model", func(t *testing.T) {
		rec := db.EventRecord{
			EventType: "run_started",
			Data: types.TelemetryFrameData{
				Title: strPtr("My Task"),
				Model: strPtr("claude-3-7-sonnet"),
			},
		}
		s := FormatEventSummary(rec)
		if !strings.Contains(s, "title=\"My Task\"") || !strings.Contains(s, "model=claude-3-7-sonnet") {
			t.Errorf("got %q, want title and model", s)
		}
	})

	t.Run("fallback branches for all types", func(t *testing.T) {
		if s := FormatEventSummary(db.EventRecord{EventType: "run_started"}); s != "run started" {
			t.Errorf("got %q, want 'run started'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "run_exited"}); s != "run exited" {
			t.Errorf("got %q, want 'run exited'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "turn_end"}); s != "turn ended" {
			t.Errorf("got %q, want 'turn ended'", s)
		}
		turnInputOutput := db.EventRecord{
			EventType: "turn_end",
			Data: types.TelemetryFrameData{
				Usage: &types.ITokenUsage{Input: 100, Output: 200},
			},
		}
		if s := FormatEventSummary(turnInputOutput); s != "tokens=300" {
			t.Errorf("got %q, want 'tokens=300'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "tool_call"}); s != "tool call" {
			t.Errorf("got %q, want 'tool call'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "thinking"}); s != "thinking" {
			t.Errorf("got %q, want 'thinking'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "user_message"}); s != "user message" {
			t.Errorf("got %q, want 'user message'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "assistant_message"}); s != "assistant message" {
			t.Errorf("got %q, want 'assistant message'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "error"}); s != "error" {
			t.Errorf("got %q, want 'error'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "waiting_question"}); s != "waiting question" {
			t.Errorf("got %q, want 'waiting question'", s)
		}
		if s := FormatEventSummary(db.EventRecord{EventType: "custom_type"}); s != "custom_type" {
			t.Errorf("got %q, want 'custom_type'", s)
		}
	})
}

func TestRenderEventsTable(t *testing.T) {
	events := []db.EventRecord{
		{
			ID:        18588,
			SessionID: "ses-1",
			AgentID:   "env:pi:ses-1",
			EventType: "run_exited",
			Timestamp: 1758490291138,
			PID:       27680,
		},
		{
			ID:         18366,
			SessionID:  "ses-1",
			AgentID:    "env:pi:ses-1",
			SubagentID: strPtr("sub-1"),
			EventType:  "run_started",
			Timestamp:  1758347252623,
			Harness:    "pi",
			PID:        27680,
		},
	}

	t.Run("human mode renders table", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderEventsTable(&buf, events, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "18588") || !strings.Contains(out, "run_exited") {
			t.Errorf("missing expected event content: %s", out)
		}
		if !strings.Contains(out, "sub-1") {
			t.Errorf("missing subagent in table: %s", out)
		}
	})

	t.Run("agent mode renders key-value lines without tables", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderEventsTable(&buf, events, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d: %s", len(lines), out)
		}
		if !strings.HasPrefix(lines[0], "id:18588") || !strings.Contains(lines[0], "type:run_exited") {
			t.Errorf("unexpected agent line: %s", lines[0])
		}
		if strings.Contains(out, "|") || strings.Contains(out, "+-") {
			t.Errorf("agent mode should not contain table borders: %s", out)
		}
	})

	t.Run("intermediate tool turn_end renders API RESPONSE badge", func(t *testing.T) {
		turnEndEvents := []db.EventRecord{
			{
				ID:        18590,
				SessionID: "ses-1",
				AgentID:   "env:pi:ses-1",
				EventType: "turn_end",
				Timestamp: 1758490295000,
				Data: types.TelemetryFrameData{
					StopReason: strPtr("tool_use"),
					Usage:      &types.ITokenUsage{Total: 4200},
				},
			},
		}

		var bufHuman bytes.Buffer
		if err := RenderEventsTable(&bufHuman, turnEndEvents, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		humanOut := bufHuman.String()
		if !strings.Contains(humanOut, "turn_end (API RESPONSE)") {
			t.Errorf("expected 'turn_end (API RESPONSE)' in human table, got: %s", humanOut)
		}
		if !strings.Contains(humanOut, "(api_response) tokens=4200") {
			t.Errorf("expected '(api_response) tokens=4200' in human table summary, got: %s", humanOut)
		}

		var bufAgent bytes.Buffer
		if err := RenderEventsTable(&bufAgent, turnEndEvents, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		agentOut := bufAgent.String()
		if !strings.Contains(agentOut, "type:turn_end") {
			t.Errorf("expected 'type:turn_end' in agent mode, got: %s", agentOut)
		}
		if !strings.Contains(agentOut, `summary:"(api_response) tokens=4200"`) {
			t.Errorf("expected summary with (api_response) in agent mode, got: %s", agentOut)
		}
	})

	t.Run("empty events human mode", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderEventsTable(&buf, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "No telemetry events found.") {
			t.Errorf("expected empty message, got %s", buf.String())
		}
	})
}

func TestRenderEventDetail(t *testing.T) {
	rec := db.EventRecord{
		ID:        18588,
		SessionID: "ses-1",
		AgentID:   "env:pi:ses-1",
		EventType: "run_exited",
		Timestamp: 1758490291138,
		PID:       27680,
		DataJSON:  `{"exitCode": 0}`,
	}

	t.Run("human mode detail card", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderEventDetail(&buf, rec, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Event ID:") || !strings.Contains(out, "18588") {
			t.Errorf("missing event ID in detail: %s", out)
		}
		if !strings.Contains(out, "run_exited") {
			t.Errorf("missing event type in detail: %s", out)
		}
	})

	t.Run("intermediate tool turn_end detail card", func(t *testing.T) {
		recTurn := db.EventRecord{
			ID:        18591,
			SessionID: "ses-1",
			AgentID:   "env:pi:ses-1",
			EventType: "turn_end",
			Timestamp: 1758490296000,
			Data: types.TelemetryFrameData{
				StopReason: strPtr("tool_use"),
				Usage:      &types.ITokenUsage{Total: 3000},
			},
		}

		var buf bytes.Buffer
		if err := RenderEventDetail(&buf, recTurn, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "turn_end (API RESPONSE)") {
			t.Errorf("expected 'turn_end (API RESPONSE)' in detail card event type, got: %s", out)
		}
		if !strings.Contains(out, "(api_response) tokens=3000") {
			t.Errorf("expected '(api_response) tokens=3000' in detail card summary, got: %s", out)
		}
	})

	t.Run("human mode detail card with empty dataJSON", func(t *testing.T) {
		recEmpty := db.EventRecord{
			ID:        1,
			SessionID: "s1",
			AgentID:   "env:h:s1",
			EventType: "run_started",
			Timestamp: 1000,
			Harness:   "pi",
			PID:       1234,
		}
		var buf bytes.Buffer
		err := RenderEventDetail(&buf, recEmpty, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Harness:         pi") || !strings.Contains(out, "PID:             1234") {
			t.Errorf("missing harness or pid in detail: %s", out)
		}
	})

	t.Run("agent mode detail", func(t *testing.T) {
		var buf bytes.Buffer
		err := RenderEventDetail(&buf, rec, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "id:18588") || !strings.Contains(out, "type:run_exited") {
			t.Errorf("unexpected agent mode detail: %s", out)
		}
	})
}

func TestRenderEventsJSON(t *testing.T) {
	events := []db.EventRecord{
		{
			ID:        18588,
			SessionID: "ses-1",
			AgentID:   "env:pi:ses-1",
			EventType: "run_exited",
			Timestamp: 1758490291138,
			PID:       27680,
			DataJSON:  `{"exitCode": 0}`,
		},
	}

	var buf bytes.Buffer
	err := RenderEventsJSON(&buf, events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []EventJSONItem
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, buf.String())
	}
	if len(parsed) != 1 || parsed[0].ID != 18588 {
		t.Errorf("unexpected parsed JSON: %+v", parsed)
	}
}

func TestRenderEventJSON(t *testing.T) {
	rec := db.EventRecord{
		ID:        18588,
		SessionID: "ses-1",
		AgentID:   "env:pi:ses-1",
		EventType: "run_exited",
		Timestamp: 1758490291138,
		PID:       27680,
		DataJSON:  `{"exitCode": 0}`,
	}

	var buf bytes.Buffer
	err := RenderEventJSON(&buf, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed EventJSONItem
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, buf.String())
	}
	if parsed.ID != 18588 || parsed.EventType != "run_exited" {
		t.Errorf("unexpected parsed JSON item: %+v", parsed)
	}
}

func TestFormatEventSummary_RuneSafeTruncation_CJKAndEmoji(t *testing.T) {
	t.Run("tool_call command does not split runes or emit hex escapes", func(t *testing.T) {
		cmd := "a日本語日本語日本語日本語日本語日本語日本語"
		rec := db.EventRecord{
			EventType: "tool_call",
			Data: types.TelemetryFrameData{
				ToolName:  strPtr("bash"),
				ToolInput: map[string]interface{}{"command": cmd},
			},
		}
		summary := FormatEventSummary(rec)
		if strings.Contains(summary, `\x`) {
			t.Errorf("summary contains escaped hex bytes indicating sliced rune: %s", summary)
		}
		if !utf8.ValidString(summary) {
			t.Errorf("summary is not valid UTF-8: %s", summary)
		}
	})

	t.Run("thinking len uses rune count not byte length", func(t *testing.T) {
		content := "日本語"
		rec := db.EventRecord{
			EventType: "thinking",
			Data: types.TelemetryFrameData{
				Content: &content,
			},
		}
		summary := FormatEventSummary(rec)
		if summary != "len=3 chars" {
			t.Errorf("got %q, want 'len=3 chars' (rune count, not byte count)", summary)
		}
	})

	t.Run("user_message does not split runes or emit hex escapes", func(t *testing.T) {
		prompt := "ab日本語日本語日本語日本語日本語日本語日本語日本語日本語日本語"
		rec := db.EventRecord{
			EventType: "user_message",
			Data: types.TelemetryFrameData{
				LastPrompt: &prompt,
			},
		}
		summary := FormatEventSummary(rec)
		if strings.Contains(summary, `\x`) {
			t.Errorf("summary contains escaped hex bytes indicating sliced rune: %s", summary)
		}
		if !utf8.ValidString(summary) {
			t.Errorf("summary is not valid UTF-8: %s", summary)
		}
	})

	t.Run("assistant_message does not split runes or emit hex escapes", func(t *testing.T) {
		content := "ab日本語日本語日本語日本語日本語日本語日本語日本語日本語日本語"
		rec := db.EventRecord{
			EventType: "assistant_message",
			Data: types.TelemetryFrameData{
				Content: &content,
			},
		}
		summary := FormatEventSummary(rec)
		if strings.Contains(summary, `\x`) {
			t.Errorf("summary contains escaped hex bytes indicating sliced rune: %s", summary)
		}
		if !utf8.ValidString(summary) {
			t.Errorf("summary is not valid UTF-8: %s", summary)
		}
	})
}
