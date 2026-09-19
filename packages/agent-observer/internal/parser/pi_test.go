package parser

import (
	"testing"
)

func TestParsePiContentBlocks(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		wantBlocks int
	}{
		{"nil", nil, 0},
		{"string", "just text", 1},
		{
			"array of maps",
			[]interface{}{
				map[string]interface{}{"type": "text", "text": "block 1"},
				map[string]interface{}{"type": "thinking", "thinking": "block 2"},
			},
			2,
		},
		{
			"single map",
			map[string]interface{}{"type": "toolCall", "toolName": "read_file"},
			1,
		},
		{"unsupported int", 12345, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePiContentBlocks(tt.input)
			if len(got) != tt.wantBlocks {
				t.Fatalf("parsePiContentBlocks() returned %d blocks, want %d", len(got), tt.wantBlocks)
			}
		})
	}
}

func TestParsePiLine(t *testing.T) {
	t.Run("empty line", func(t *testing.T) {
		evs, err := ParsePiLine([]byte(""), "/fallback")
		if err != nil || len(evs) != 0 {
			t.Errorf("expected nil, nil for empty line, got %v, %v", evs, err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParsePiLine([]byte("{invalid"), "/fallback")
		if err == nil {
			t.Errorf("expected error for invalid json")
		}
	})

	t.Run("session header with fallback CWD", func(t *testing.T) {
		line := []byte(`{"type":"session","version":3,"id":"01a0b64b-test","timestamp":"2026-09-18T20:53:30.007Z"}`)
		evs, err := ParsePiLine(line, "/fallback/dir")
		if err != nil {
			t.Fatalf("ParsePiLine error: %v", err)
		}
		if len(evs) != 1 || evs[0].EventType != "run_started" || evs[0].CWD != "/fallback/dir" {
			t.Errorf("unexpected session event: %+v", evs)
		}
	})

	t.Run("label and session_info", func(t *testing.T) {
		line1 := []byte(`{"type":"label","title":"My Session Title","timestamp":"2026-09-18T20:53:30.000Z","cwd":"/cwd"}`)
		evs1, err := ParsePiLine(line1, "")
		if err != nil || len(evs1) != 1 || *evs1[0].Data.Title != "My Session Title" {
			t.Errorf("label event mismatch: %+v", evs1)
		}

		line2 := []byte(`{"type":"session_info","name":"Named Session","timestamp":"2026-09-18T20:53:30.000Z"}`)
		evs2, err := ParsePiLine(line2, "/cwd")
		if err != nil || len(evs2) != 1 || *evs2[0].Data.Title != "Named Session" {
			t.Errorf("session_info event mismatch: %+v", evs2)
		}
	})

	t.Run("model_change", func(t *testing.T) {
		line := []byte(`{"type":"model_change","model":"gemini-3.7-flash","timestamp":"2026-09-18T20:53:30.000Z"}`)
		evs, err := ParsePiLine(line, "/cwd")
		if err != nil || len(evs) != 1 || evs[0].EventType != "model_change" || *evs[0].Data.Model != "gemini-3.7-flash" {
			t.Errorf("model_change event mismatch: %+v", evs)
		}
	})

	t.Run("message with nil message field", func(t *testing.T) {
		line := []byte(`{"type":"message","timestamp":"2026-09-18T20:53:30.000Z"}`)
		evs, err := ParsePiLine(line, "/cwd")
		if err != nil || len(evs) != 0 {
			t.Errorf("expected empty events for nil message, got %v", evs)
		}
	})

	t.Run("user message with string content", func(t *testing.T) {
		line := []byte(`{"type":"message","message":{"role":"user","content":"Simple text prompt"}}`)
		evs, err := ParsePiLine(line, "/cwd")
		if err != nil || len(evs) != 1 || *evs[0].Data.LastPrompt != "Simple text prompt" {
			t.Errorf("user string content mismatch: %+v", evs)
		}
	})

	t.Run("toolResult with isError false and true", func(t *testing.T) {
		lineFalse := []byte(`{"type":"message","message":{"role":"toolResult","toolCallId":"call-1","toolName":"read_file","content":"ok","isError":false}}`)
		evsFalse, err := ParsePiLine(lineFalse, "/cwd")
		if err != nil || len(evsFalse) != 1 || *evsFalse[0].Data.IsError != false || *evsFalse[0].Data.ToolCallID != "call-1" {
			t.Errorf("toolResult false mismatch: %+v", evsFalse)
		}

		lineTrue := []byte(`{"type":"message","message":{"role":"toolResult","toolCallId":"call-2","toolName":"read_file","content":"err","isError":true}}`)
		evsTrue, err := ParsePiLine(lineTrue, "/cwd")
		if err != nil || len(evsTrue) != 1 || *evsTrue[0].Data.IsError != true {
			t.Errorf("toolResult true mismatch: %+v", evsTrue)
		}
	})

	t.Run("assistant thinking via text fallback, toolCall with arguments, usage total computed", func(t *testing.T) {
		line := []byte(`{
			"type": "message",
			"message": {
				"role": "assistant",
				"model": "claude-3-7-sonnet",
				"stopReason": "stop",
				"content": [
					{"type": "thinking", "text": "Thought via text field"},
					{"type": "toolCall", "name": "custom_tool", "id": "t-1", "arguments": {"foo": "bar"}}
				],
				"usage": {
					"input": 50,
					"output": 25,
					"cacheRead": 10,
					"cacheWrite": 5,
					"reasoning": 15,
					"total": 0,
					"cost": 0.005
				}
			}
		}`)
		evs, err := ParsePiLine(line, "/cwd")
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 3 {
			t.Fatalf("expected 3 events, got %d", len(evs))
		}

		// Thinking
		if evs[0].EventType != "thinking" || *evs[0].Data.Content != "Thought via text field" {
			t.Errorf("thinking mismatch: %+v", evs[0])
		}

		// Tool call
		if evs[1].EventType != "tool_call" || *evs[1].Data.ToolName != "custom_tool" || *evs[1].Data.ToolCallID != "t-1" {
			t.Errorf("toolCall mismatch: %+v", evs[1])
		}

		// Turn end with computed total
		u := evs[2].Data.Usage
		if u == nil {
			t.Fatalf("usage is nil")
		}
		// total = 50 + 25 + 10 + 5 + 15 = 105
		if u.Total != 105 {
			t.Errorf("u.Total = %d, want 105", u.Total)
		}
		if evs[2].Data.RawHarnessCost == nil || *evs[2].Data.RawHarnessCost != 0.005 {
			t.Errorf("cost mismatch: %v", evs[2].Data.RawHarnessCost)
		}
	})

	t.Run("assistant AskUserQuestion tool names and prompt parameter", func(t *testing.T) {
		toolNames := []string{"AskUserQuestion", "ask_user", "ask_question"}
		for _, name := range toolNames {
			line := []byte(`{
				"type": "message",
				"message": {
					"role": "assistant",
					"content": [
						{"type": "toolCall", "toolName": "` + name + `", "id": "q-1", "parameters": {"prompt": "Confirm action?"}}
					]
				}
			}`)
			evs, err := ParsePiLine(line, "/cwd")
			if err != nil {
				t.Fatalf("name %s: error %v", name, err)
			}
			if len(evs) != 2 || evs[0].EventType != "question_prompt" || *evs[0].Data.Question != "Confirm action?" {
				t.Errorf("name %s: question_prompt mismatch: %+v", name, evs)
			}
		}
	})
}
