package parser

import (
	"testing"
)

func TestParsePiLine(t *testing.T) {
	// 1. Session header
	sessionLine := []byte(`{"type":"session","version":3,"id":"01a0b64b-test","timestamp":"2026-09-18T20:53:30.007Z","cwd":"/Users/test/workspace"}`)
	events, err := ParsePiLine(sessionLine, "")
	if err != nil {
		t.Fatalf("ParsePiLine(session) error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "run_started" {
		t.Errorf("EventType = %q, want 'run_started'", events[0].EventType)
	}
	if events[0].CWD != "/Users/test/workspace" {
		t.Errorf("CWD = %q, want '/Users/test/workspace'", events[0].CWD)
	}

	// 2. User Message
	userLine := []byte(`{"type":"message","id":"msg1","timestamp":"2026-09-18T20:53:31.000Z","message":{"role":"user","content":[{"type":"text","text":"Implement phase 2"}]}}`)
	events, err = ParsePiLine(userLine, "/fallback")
	if err != nil {
		t.Fatalf("ParsePiLine(user) error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "turn_start" {
		t.Errorf("EventType = %q, want 'turn_start'", events[0].EventType)
	}
	if events[0].Data.LastPrompt == nil || *events[0].Data.LastPrompt != "Implement phase 2" {
		t.Errorf("LastPrompt = %v, want 'Implement phase 2'", events[0].Data.LastPrompt)
	}

	// 3. Assistant message with thinking, toolCall, and usage
	assistantLine := []byte(`{"type":"message","id":"msg2","timestamp":"2026-09-18T20:53:32.000Z","message":{"role":"assistant","model":"gemini-3.7-flash","provider":"google","content":[{"type":"thinking","thinking":"Analyzing files"},{"type":"toolCall","id":"call-1","toolName":"read_file","parameters":{"path":"main.go"}}],"usage":{"input":100,"output":50,"cacheRead":20,"cacheWrite":10,"reasoning":30,"total":210,"cost":0.0012}}}`)
	events, err = ParsePiLine(assistantLine, "/fallback")
	if err != nil {
		t.Fatalf("ParsePiLine(assistant) error: %v", err)
	}
	if len(events) != 3 { // thinking + tool_call + turn_end
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if events[0].EventType != "thinking" {
		t.Errorf("events[0].EventType = %q, want 'thinking'", events[0].EventType)
	}
	if events[0].Data.Content == nil || *events[0].Data.Content != "Analyzing files" {
		t.Errorf("thinking content = %v, want 'Analyzing files'", events[0].Data.Content)
	}

	if events[1].EventType != "tool_call" {
		t.Errorf("events[1].EventType = %q, want 'tool_call'", events[1].EventType)
	}
	if events[1].Data.ToolName == nil || *events[1].Data.ToolName != "read_file" {
		t.Errorf("toolName = %v, want 'read_file'", events[1].Data.ToolName)
	}
	if events[1].Data.ToolCallID == nil || *events[1].Data.ToolCallID != "call-1" {
		t.Errorf("toolCallID = %v, want 'call-1'", events[1].Data.ToolCallID)
	}

	if events[2].EventType != "turn_end" {
		t.Errorf("events[2].EventType = %q, want 'turn_end'", events[2].EventType)
	}
	if events[2].Data.Usage == nil {
		t.Fatalf("events[2].Data.Usage is nil")
	}
	if events[2].Data.Usage.Input != 100 || events[2].Data.Usage.Output != 50 || events[2].Data.Usage.Total != 210 {
		t.Errorf("unexpected usage: %+v", events[2].Data.Usage)
	}
	if events[2].Data.RawHarnessCost == nil || *events[2].Data.RawHarnessCost != 0.0012 {
		t.Errorf("rawHarnessCost = %v, want 0.0012", events[2].Data.RawHarnessCost)
	}

	// 4. Tool result with error
	toolResultLine := []byte(`{"type":"message","id":"msg3","timestamp":"2026-09-18T20:53:33.000Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"read_file","content":"file not found","isError":true}}`)
	events, err = ParsePiLine(toolResultLine, "/fallback")
	if err != nil {
		t.Fatalf("ParsePiLine(toolResult) error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "tool_result" {
		t.Errorf("EventType = %q, want 'tool_result'", events[0].EventType)
	}
	if events[0].Data.IsError == nil || !*events[0].Data.IsError {
		t.Errorf("IsError = %v, want true", events[0].Data.IsError)
	}
	if events[0].Data.ToolOutput == nil || *events[0].Data.ToolOutput != "file not found" {
		t.Errorf("ToolOutput = %v, want 'file not found'", events[0].Data.ToolOutput)
	}

	// 5. AskUserQuestion in Pi
	askLine := []byte(`{"type":"message","id":"msg4","timestamp":"2026-09-18T20:53:34.000Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-q","toolName":"AskUserQuestion","parameters":{"question":"Proceed with deletion?"}}]}}`)
	events, err = ParsePiLine(askLine, "/fallback")
	if err != nil {
		t.Fatalf("ParsePiLine(askLine) error: %v", err)
	}
	if len(events) < 1 {
		t.Fatalf("expected events, got 0")
	}
	if events[0].EventType != "question_prompt" {
		t.Errorf("events[0].EventType = %q, want 'question_prompt'", events[0].EventType)
	}
	if events[0].Data.Question == nil || *events[0].Data.Question != "Proceed with deletion?" {
		t.Errorf("Question = %v, want 'Proceed with deletion?'", events[0].Data.Question)
	}
}
