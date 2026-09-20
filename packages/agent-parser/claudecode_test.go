package parser

import (
	"testing"
	"time"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantVal int64
		wantNow bool
	}{
		{"nil", nil, 0, true},
		{"empty string", "", 0, true},
		{"float64", float64(1700000000000), 1700000000000, false},
		{"int64", int64(1700000000000), 1700000000000, false},
		{"int", int(1700000000), 1700000000, false},
		{"rfc3339 nano", "2026-09-19T10:00:00.123456Z", 0, false},
		{"rfc3339 standard", "2026-09-19T10:00:00Z", 0, false},
		{"invalid format", "not-a-date", 0, true},
		{"unsupported type", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestamp(tt.input)
			if tt.wantNow {
				if time.Since(time.UnixMilli(got)) > 5*time.Second {
					t.Errorf("expected recent timestamp, got %d", got)
				}
			} else if tt.wantVal > 0 {
				if got != tt.wantVal {
					t.Errorf("got %d, want %d", got, tt.wantVal)
				}
			} else {
				if got <= 0 {
					t.Errorf("expected positive timestamp, got %d", got)
				}
			}
		})
	}
}

func TestExtractContentString(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"nil", nil, ""},
		{"string", "direct string", "direct string"},
		{
			"array of text objects",
			[]interface{}{
				map[string]interface{}{"text": "part 1 "},
				map[string]interface{}{"text": "part 2"},
			},
			"part 1 part 2",
		},
		{
			"arbitrary map fallback to json",
			map[string]interface{}{"key": "value"},
			`{"key":"value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContentString(tt.input)
			if got != tt.want {
				t.Errorf("extractContentString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseClaudeContentBlocks(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		blocks := parseClaudeContentBlocks(nil)
		if blocks != nil {
			t.Errorf("expected nil blocks, got %v", blocks)
		}
	})

	t.Run("single string", func(t *testing.T) {
		blocks := parseClaudeContentBlocks("test text")
		if len(blocks) != 1 || blocks[0].Text != "test text" || blocks[0].Type != "text" {
			t.Errorf("unexpected blocks: %+v", blocks)
		}
	})

	t.Run("single map without type", func(t *testing.T) {
		blocks := parseClaudeContentBlocks(map[string]interface{}{"text": "map text"})
		if len(blocks) != 1 || blocks[0].Type != "text" || blocks[0].Text != "map text" {
			t.Errorf("unexpected blocks: %+v", blocks)
		}
	})

	t.Run("slice of maps", func(t *testing.T) {
		blocks := parseClaudeContentBlocks([]interface{}{
			map[string]interface{}{"text": "block 1"},
			map[string]interface{}{"type": "thinking", "thinking": "deep thoughts"},
		})
		if len(blocks) != 2 || blocks[0].Text != "block 1" || blocks[1].Thinking != "deep thoughts" {
			t.Errorf("unexpected blocks: %+v", blocks)
		}
	})
}

func TestParseClaudeCodeLine(t *testing.T) {
	t.Run("empty line", func(t *testing.T) {
		evs, err := ParseClaudeCodeLine([]byte(""))
		if err != nil || len(evs) != 0 {
			t.Errorf("expected nil, nil for empty line, got %v, %v", evs, err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseClaudeCodeLine([]byte("{invalid"))
		if err == nil {
			t.Errorf("expected error for invalid json")
		}
	})

	t.Run("ai-title with aiTitle field", func(t *testing.T) {
		line := `{"type":"ai-title","aiTitle":"Optimized Title","timestamp":"2026-09-19T10:00:00.000Z","cwd":"/repo","gitBranch":"main"}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "title_change" || *evs[0].Data.Title != "Optimized Title" {
			t.Errorf("unexpected event: %+v", evs[0])
		}
	})

	t.Run("title with content fallback", func(t *testing.T) {
		line := `{"type":"title","content":[{"text":"Content Title"}],"timestamp":"2026-09-19T10:00:00.000Z"}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 1 || *evs[0].Data.Title != "Content Title" {
			t.Errorf("unexpected event: %+v", evs)
		}
	})

	t.Run("last-prompt with lastPrompt and content", func(t *testing.T) {
		line1 := `{"type":"last-prompt","lastPrompt":"Fix bugs","timestamp":"2026-09-19T10:00:00.000Z"}`
		evs1, err := ParseClaudeCodeLine([]byte(line1))
		if err != nil || len(evs1) != 1 || *evs1[0].Data.LastPrompt != "Fix bugs" {
			t.Errorf("failed last-prompt 1: %+v", evs1)
		}

		line2 := `{"type":"last-prompt","content":"Prompt via content","timestamp":"2026-09-19T10:00:00.000Z"}`
		evs2, err := ParseClaudeCodeLine([]byte(line2))
		if err != nil || len(evs2) != 1 || *evs2[0].Data.LastPrompt != "Prompt via content" {
			t.Errorf("failed last-prompt 2: %+v", evs2)
		}
	})

	t.Run("user prompt text", func(t *testing.T) {
		line := `{"type":"user","uuid":"u-1","timestamp":"2026-09-19T10:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":"Please fix the bug in server.go"}]}}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "turn_start" || *evs[0].Data.Content != "Please fix the bug in server.go" {
			t.Errorf("unexpected event: %+v", evs[0])
		}
	})

	t.Run("user prompt raw string content", func(t *testing.T) {
		line := `{"type":"user","uuid":"u-raw","timestamp":"2026-09-19T10:00:00.000Z","message":{"role":"user","content":"How do I fix this build error?"}}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "turn_start" || *evs[0].Data.Content != "How do I fix this build error?" {
			t.Errorf("unexpected event: %+v", evs[0])
		}
	})

	t.Run("user tool_result with is_error false and ID fallback", func(t *testing.T) {
		isErr := false
		line := `{"type":"user","uuid":"u-2","timestamp":"2026-09-19T10:00:05.000Z","message":{"role":"user","content":[{"type":"tool_result","id":"call_999","text":"success output","is_error":false}]}}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "tool_result" {
			t.Errorf("eventType = %q, want 'tool_result'", evs[0].EventType)
		}
		if *evs[0].Data.ToolCallID != "call_999" {
			t.Errorf("ToolCallID = %q, want 'call_999'", *evs[0].Data.ToolCallID)
		}
		if *evs[0].Data.ToolOutput != "success output" {
			t.Errorf("ToolOutput = %q, want 'success output'", *evs[0].Data.ToolOutput)
		}
		if *evs[0].Data.IsError != isErr {
			t.Errorf("IsError = %v, want false", *evs[0].Data.IsError)
		}
	})

	t.Run("assistant thinking, standard tool_use, reasoning tokens roll-up", func(t *testing.T) {
		line := `{
			"type": "assistant",
			"uuid": "a-2",
			"timestamp": "2026-09-19T10:00:01.000Z",
			"message": {
				"role": "assistant",
				"model": "claude-3-7-sonnet-20250219",
				"stop_reason": "tool_use",
				"content": [
					{"type": "thinking", "thinking": "Let me run a test"},
					{"type": "tool_use", "id": "call_bash", "name": "Bash", "input": {"command": "go test ./..."}}
				],
				"usage": {
					"input_tokens": 100,
					"output_tokens": 50,
					"cache_read_input_tokens": 500,
					"cache_creation_input_tokens": 50,
					"reasoning_tokens": 20
				}
			}
		}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 3 {
			t.Fatalf("expected 3 events (thinking, tool_call, turn_end), got %d", len(evs))
		}

		// 1. Thinking
		if evs[0].EventType != "thinking" || *evs[0].Data.Content != "Let me run a test" {
			t.Errorf("thinking event mismatch: %+v", evs[0])
		}

		// 2. Tool Call
		if evs[1].EventType != "tool_call" || *evs[1].Data.ToolName != "Bash" || *evs[1].Data.ToolCallID != "call_bash" {
			t.Errorf("tool_call event mismatch: %+v", evs[1])
		}

		// 3. Turn End with Usage roll-up
		if evs[2].EventType != "turn_end" {
			t.Errorf("turn_end event mismatch: %+v", evs[2])
		}
		if *evs[2].Data.StopReason != "tool_use" {
			t.Errorf("stop_reason mismatch: %v", evs[2].Data.StopReason)
		}
		u := evs[2].Data.Usage
		if u == nil {
			t.Fatalf("usage is nil")
		}
		if u.Input != 100 || u.Output != 50 || u.CacheRead != 500 || u.CacheWrite != 50 || u.Reasoning != 20 {
			t.Errorf("token usage breakdown mismatch: %+v", u)
		}
		if u.Total != 700 {
			t.Errorf("u.Total = %d, want 700 (reasoning tokens must not be double counted)", u.Total)
		}
	})

	t.Run("assistant text block conversational response", func(t *testing.T) {
		line := `{
			"type": "assistant",
			"uuid": "a-text",
			"timestamp": "2026-09-19T10:00:02.000Z",
			"cwd": "/workspace",
			"gitBranch": "fix/issue-5",
			"message": {
				"role": "assistant",
				"model": "claude-3-7-sonnet-20250219",
				"stop_reason": "end_turn",
				"content": [
					{"type": "text", "text": "I have completed the task and updated all unit tests."}
				],
				"usage": {
					"input_tokens": 150,
					"output_tokens": 30,
					"cache_read_input_tokens": 0,
					"cache_creation_input_tokens": 0
				}
			}
		}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event (turn_end), got %d: %+v", len(evs), evs)
		}
		if evs[0].EventType != "turn_end" {
			t.Errorf("eventType = %q, want 'turn_end'", evs[0].EventType)
		}
		if evs[0].Data.Content == nil || *evs[0].Data.Content != "I have completed the task and updated all unit tests." {
			t.Errorf("expected turn_end content to contain assistant text, got: %v", evs[0].Data.Content)
		}
		if evs[0].Data.Usage == nil || evs[0].Data.Usage.Total != 180 {
			t.Errorf("expected usage total 180, got: %+v", evs[0].Data.Usage)
		}
	})

	t.Run("assistant thinking and multiple text blocks combined", func(t *testing.T) {
		line := `{
			"type": "assistant",
			"uuid": "a-multi",
			"timestamp": "2026-09-19T10:00:03.000Z",
			"message": {
				"role": "assistant",
				"model": "claude-3-7-sonnet-20250219",
				"stop_reason": "end_turn",
				"content": [
					{"type": "thinking", "thinking": "Let's summarize the changes."},
					{"type": "text", "text": "First part of answer."},
					{"type": "text", "text": "Second part of answer."}
				],
				"usage": {
					"input_tokens": 200,
					"output_tokens": 80,
					"cache_read_input_tokens": 100,
					"cache_creation_input_tokens": 50,
					"thinking_tokens": 40
				}
			}
		}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 2 {
			t.Fatalf("expected 2 events (thinking, turn_end), got %d: %+v", len(evs), evs)
		}
		if evs[0].EventType != "thinking" || *evs[0].Data.Content != "Let's summarize the changes." {
			t.Errorf("thinking event mismatch: %+v", evs[0])
		}
		if evs[1].EventType != "turn_end" {
			t.Errorf("turn_end event mismatch: %+v", evs[1])
		}
		expectedContent := "First part of answer.\nSecond part of answer."
		if evs[1].Data.Content == nil || *evs[1].Data.Content != expectedContent {
			t.Errorf("expected combined text %q, got %v", expectedContent, evs[1].Data.Content)
		}
		if evs[1].Data.Usage.Reasoning != 40 {
			t.Errorf("expected reasoning tokens 40 from thinking_tokens, got %d", evs[1].Data.Usage.Reasoning)
		}
		// 200 + 80 + 100 + 50 = 430
		if evs[1].Data.Usage.Total != 430 {
			t.Errorf("expected total 430, got %d", evs[1].Data.Usage.Total)
		}
	})

	t.Run("assistant top-level error string", func(t *testing.T) {
		line := `{
			"type": "assistant",
			"uuid": "a-err1",
			"timestamp": "2026-09-19T10:00:04.000Z",
			"error": "rate_limit_exceeded"
		}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 error event, got %d: %+v", len(evs), evs)
		}
		if evs[0].EventType != "error" {
			t.Errorf("eventType = %q, want 'error'", evs[0].EventType)
		}
		if evs[0].Data.Content == nil || *evs[0].Data.Content != "rate_limit_exceeded" {
			t.Errorf("error content = %v, want 'rate_limit_exceeded'", evs[0].Data.Content)
		}
		if evs[0].Data.IsError == nil || !*evs[0].Data.IsError {
			t.Errorf("expected isError = true")
		}
	})

	t.Run("assistant apiErrorStatus and isApiErrorMessage", func(t *testing.T) {
		line := `{
			"type": "assistant",
			"uuid": "a-err2",
			"timestamp": "2026-09-19T10:00:05.000Z",
			"isApiErrorMessage": true,
			"apiErrorStatus": 529,
			"message": {
				"role": "assistant",
				"model": "claude-3-7-sonnet-20250219",
				"content": [
					{"type": "text", "text": "Anthropic API overloaded. Please retry later."}
				]
			}
		}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		// Expect turn_end and error event
		hasErrorEvent := false
		for _, ev := range evs {
			if ev.EventType == "error" {
				hasErrorEvent = true
				if ev.Data.Content == nil || *ev.Data.Content != "Anthropic API overloaded. Please retry later." {
					t.Errorf("error content = %v, want 'Anthropic API overloaded. Please retry later.'", ev.Data.Content)
				}
				if ev.Data.Model == nil || *ev.Data.Model != "claude-3-7-sonnet-20250219" {
					t.Errorf("error model = %v, want claude-3-7-sonnet-20250219", ev.Data.Model)
				}
			}
		}
		if !hasErrorEvent {
			t.Errorf("expected error event to be emitted for api error message, got: %+v", evs)
		}
	})

	t.Run("assistant message.error without content", func(t *testing.T) {
		line := `{
			"type": "assistant",
			"uuid": "a-err3",
			"timestamp": "2026-09-19T10:00:06.000Z",
			"message": {
				"role": "assistant",
				"error": "connection_reset_by_peer"
			}
		}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		hasErrorEvent := false
		for _, ev := range evs {
			if ev.EventType == "error" {
				hasErrorEvent = true
				if ev.Data.Content == nil || *ev.Data.Content != "connection_reset_by_peer" {
					t.Errorf("error content = %v, want 'connection_reset_by_peer'", ev.Data.Content)
				}
			}
		}
		if !hasErrorEvent {
			t.Errorf("expected error event for message.error, got: %+v", evs)
		}
	})

	t.Run("assistant AskUserQuestion with prompt parameter", func(t *testing.T) {
		line := `{
			"type": "assistant",
			"message": {
				"role": "assistant",
				"content": [
					{"type": "tool_use", "id": "q1", "name": "AskUserQuestion", "input": {"prompt": "Confirm delete?"}}
				]
			}
		}`
		evs, err := ParseClaudeCodeLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 2 {
			t.Fatalf("expected 2 events (question_prompt, turn_end), got %d", len(evs))
		}
		if evs[0].EventType != "question_prompt" || *evs[0].Data.Question != "Confirm delete?" {
			t.Errorf("question_prompt event mismatch: %+v", evs[0])
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseClaudeCodeLine([]byte("{invalid"))
		if err == nil {
			t.Error("expected error for invalid json")
		}
	})
}
