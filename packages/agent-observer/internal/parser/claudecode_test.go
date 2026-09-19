package parser

import (
	"testing"
)

func TestParseClaudeCodeLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantEvents    int
		checkFirstEv  func(t *testing.T, frames []ParsedEvent)
	}{
		{
			name: "user prompt turn start",
			line: `{"type":"user","uuid":"u-1","timestamp":"2026-09-19T10:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":"Please fix the bug in server.go"}]}}`,
			wantEvents: 1,
			checkFirstEv: func(t *testing.T, evs []ParsedEvent) {
				if evs[0].EventType != "turn_start" {
					t.Errorf("eventType = %q, want turn_start", evs[0].EventType)
				}
				if evs[0].Data.Content == nil || *evs[0].Data.Content != "Please fix the bug in server.go" {
					t.Errorf("content = %v, want prompt text", evs[0].Data.Content)
				}
			},
		},
		{
			name: "assistant thinking and tool call with AskUserQuestion",
			line: `{"type":"assistant","uuid":"a-1","timestamp":"2026-09-19T10:00:01.000Z","message":{"role":"assistant","model":"claude-3-7-sonnet-20250219","content":[{"type":"thinking","thinking":"I should ask the user for confirmation"},{"type":"tool_use","id":"call_123","name":"AskUserQuestion","input":{"question":"Should I proceed with migration?"}}],"usage":{"input_tokens":150,"output_tokens":45,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"thinking_tokens":25}}}`,
			wantEvents: 3, // thinking + question_prompt (or tool_call) + turn_end
			checkFirstEv: func(t *testing.T, evs []ParsedEvent) {
				// thinking
				if evs[0].EventType != "thinking" {
					t.Errorf("first event = %q, want thinking", evs[0].EventType)
				}
				if evs[0].Data.Content == nil || *evs[0].Data.Content != "I should ask the user for confirmation" {
					t.Errorf("thinking content mismatch: %v", evs[0].Data.Content)
				}

				// question_prompt
				if evs[1].EventType != "question_prompt" {
					t.Errorf("second event = %q, want question_prompt", evs[1].EventType)
				}
				if evs[1].Data.Question == nil || *evs[1].Data.Question != "Should I proceed with migration?" {
					t.Errorf("question mismatch: %v", evs[1].Data.Question)
				}

				// turn_end with usage
				if evs[2].EventType != "turn_end" {
					t.Errorf("third event = %q, want turn_end", evs[2].EventType)
				}
				u := evs[2].Data.Usage
				if u == nil {
					t.Fatalf("usage is nil")
				}
				if u.Input != 150 || u.Output != 45 || u.CacheRead != 1000 || u.CacheWrite != 200 || u.Reasoning != 25 {
					t.Errorf("token usage mismatch: %+v", u)
				}
				if u.Total != 1420 { // 150 + 45 + 1000 + 200 + 25
					t.Errorf("total tokens = %d, want 1420", u.Total)
				}
			},
		},
		{
			name: "user tool_result with error",
			line: `{"type":"user","uuid":"u-2","timestamp":"2026-09-19T10:00:05.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_123","content":"Error: exit status 1: permission denied","is_error":true}]}}`,
			wantEvents: 1,
			checkFirstEv: func(t *testing.T, evs []ParsedEvent) {
				if evs[0].EventType != "tool_result" {
					t.Errorf("eventType = %q, want tool_result", evs[0].EventType)
				}
				if evs[0].Data.IsError == nil || !*evs[0].Data.IsError {
					t.Errorf("isError = %v, want true", evs[0].Data.IsError)
				}
				if evs[0].Data.ToolOutput == nil || *evs[0].Data.ToolOutput != "Error: exit status 1: permission denied" {
					t.Errorf("toolOutput = %v", evs[0].Data.ToolOutput)
				}
			},
		},
		{
			name: "ai-title event",
			line: `{"type":"ai-title","title":"Refactor agent-observer tailer","timestamp":"2026-09-19T10:00:06.000Z"}`,
			wantEvents: 1,
			checkFirstEv: func(t *testing.T, evs []ParsedEvent) {
				if evs[0].EventType != "title_change" {
					t.Errorf("eventType = %q, want title_change", evs[0].EventType)
				}
				if evs[0].Data.Title == nil || *evs[0].Data.Title != "Refactor agent-observer tailer" {
					t.Errorf("title = %v, want 'Refactor agent-observer tailer'", evs[0].Data.Title)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs, err := ParseClaudeCodeLine([]byte(tt.line))
			if err != nil {
				t.Fatalf("ParseClaudeCodeLine() unexpected error = %v", err)
			}
			if len(evs) != tt.wantEvents {
				t.Fatalf("got %d events, want %d", len(evs), tt.wantEvents)
			}
			if tt.checkFirstEv != nil {
				tt.checkFirstEv(t, evs)
			}
		})
	}
}
