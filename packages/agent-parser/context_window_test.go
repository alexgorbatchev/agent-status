package parser_test

import (
	"fmt"
	"math"
	"testing"

	parser "github.com/alexgorbatchev/agent-status/packages/agent-parser"
)

func TestContextWindowResolutionAndFullness_ClaudeCode(t *testing.T) {
	p := parser.NewClaudeCodeParser()
	sessID := "test-claude-cw"

	// Standard Claude 3.7 Sonnet (200k window)
	lineStandard := fmt.Sprintf(`{
		"type": "assistant",
		"requestId": "req_cw_std",
		"sessionId": "%s",
		"apiBlockIndex": 0,
		"message": {
			"id": "msg_cw_std",
			"role": "assistant",
			"model": "claude-3-7-sonnet-20250219",
			"stop_reason": "end_turn",
			"content": [{"type": "text", "text": "Hello"}],
			"usage": {
				"input_tokens": 5000,
				"output_tokens": 1000,
				"cache_read_input_tokens": 40000,
				"cache_creation_input_tokens": 4000,
				"thinking_tokens": 500
			}
		}
	}`, sessID)

	evs, err := p.ParseLine([]byte(lineStandard))
	if err != nil {
		t.Fatalf("ParseLine failed: %v", err)
	}

	var turnEnd *parser.ParsedEvent
	for i := range evs {
		if evs[i].EventType == "turn_end" {
			turnEnd = &evs[i]
			break
		}
	}
	if turnEnd == nil {
		t.Fatalf("expected turn_end event")
	}

	if turnEnd.Data.Model == nil || *turnEnd.Data.Model != "claude-3-7-sonnet-20250219" {
		t.Fatalf("expected model claude-3-7-sonnet-20250219, got %v", turnEnd.Data.Model)
	}

	if turnEnd.Data.ModelContextWindow == nil || *turnEnd.Data.ModelContextWindow != 200000 {
		t.Fatalf("expected ModelContextWindow 200000, got %v", turnEnd.Data.ModelContextWindow)
	}

	expectedUsed := int64(5000 + 1000 + 40000 + 4000) // 50000
	expectedFullness := float64(expectedUsed) / 200000.0 // 0.25
	if turnEnd.Data.ContextFullness == nil || math.Abs(*turnEnd.Data.ContextFullness-expectedFullness) > 1e-6 {
		t.Fatalf("expected ContextFullness %f, got %v", expectedFullness, turnEnd.Data.ContextFullness)
	}

	u := turnEnd.Data.Usage
	if u == nil {
		t.Fatalf("expected non-nil Usage")
	}
	if u.Input != 5000 || u.Output != 1000 || u.CacheRead != 40000 || u.CacheWrite != 4000 || u.Reasoning != 500 || u.Total != 50000 {
		t.Fatalf("categorized usage mismatch: %+v", u)
	}

	// Claude 3.7 Sonnet 1M variant (1,000,000 window)
	line1M := fmt.Sprintf(`{
		"type": "assistant",
		"requestId": "req_cw_1m",
		"sessionId": "%s",
		"apiBlockIndex": 0,
		"message": {
			"id": "msg_cw_1m",
			"role": "assistant",
			"model": "claude-3-7-sonnet-1m",
			"stop_reason": "end_turn",
			"content": [{"type": "text", "text": "Extended context answer"}],
			"usage": {
				"input_tokens": 10000,
				"output_tokens": 2000,
				"cache_read_input_tokens": 150000,
				"cache_creation_input_tokens": 0
			}
		}
	}`, sessID)

	evs1M, err := p.ParseLine([]byte(line1M))
	if err != nil {
		t.Fatalf("ParseLine 1M failed: %v", err)
	}
	var turnEnd1M *parser.ParsedEvent
	for i := range evs1M {
		if evs1M[i].EventType == "turn_end" {
			turnEnd1M = &evs1M[i]
			break
		}
	}
	if turnEnd1M == nil {
		t.Fatalf("expected turn_end for 1M variant")
	}

	if turnEnd1M.Data.ModelContextWindow == nil || *turnEnd1M.Data.ModelContextWindow != 1000000 {
		t.Fatalf("expected ModelContextWindow 1000000, got %v", turnEnd1M.Data.ModelContextWindow)
	}

	expectedUsed1M := int64(10000 + 2000 + 150000) // 162000
	expectedFullness1M := float64(expectedUsed1M) / 1000000.0 // 0.162
	if turnEnd1M.Data.ContextFullness == nil || math.Abs(*turnEnd1M.Data.ContextFullness-expectedFullness1M) > 1e-6 {
		t.Fatalf("expected ContextFullness %f, got %v", expectedFullness1M, turnEnd1M.Data.ContextFullness)
	}
}

func TestContextWindowResolutionAndFullness_Pi(t *testing.T) {
	line := []byte(`{
		"type": "message",
		"message": {
			"role": "assistant",
			"api": "google",
			"provider": "google",
			"model": "gemini-2.5-pro",
			"stopReason": "stop",
			"content": [{"type": "text", "text": "Gemini response"}],
			"usage": {
				"input": 50000,
				"output": 10000,
				"cacheRead": 200000,
				"cacheWrite": 0,
				"reasoning": 2000,
				"total": 260000
			}
		}
	}`)

	evs, err := parser.ParsePiLine(line, "/cwd")
	if err != nil {
		t.Fatalf("ParsePiLine failed: %v", err)
	}

	var turnEnd *parser.ParsedEvent
	for i := range evs {
		if evs[i].EventType == "turn_end" {
			turnEnd = &evs[i]
			break
		}
	}
	if turnEnd == nil {
		t.Fatalf("expected turn_end event")
	}

	if turnEnd.Data.Model == nil || *turnEnd.Data.Model != "gemini-2.5-pro" {
		t.Fatalf("expected model 'gemini-2.5-pro', got %v", turnEnd.Data.Model)
	}

	if turnEnd.Data.ModelContextWindow == nil || *turnEnd.Data.ModelContextWindow != 2000000 {
		t.Fatalf("expected ModelContextWindow 2000000, got %v", turnEnd.Data.ModelContextWindow)
	}

	expectedFullness := 260000.0 / 2000000.0 // 0.13
	if turnEnd.Data.ContextFullness == nil || math.Abs(*turnEnd.Data.ContextFullness-expectedFullness) > 1e-6 {
		t.Fatalf("expected ContextFullness %f, got %v", expectedFullness, turnEnd.Data.ContextFullness)
	}
}

func TestContextWindowResolutionAndFullness_Codex(t *testing.T) {
	// 1. turn_context with gpt-4o
	tcLine := `{
		"type": "turn_context",
		"payload": {
			"turn_id": "turn-1",
			"cwd": "/workspace",
			"model": "gpt-4o"
		}
	}`
	evsTC, err := parser.ParseCodexLine([]byte(tcLine))
	if err != nil {
		t.Fatalf("ParseCodexLine turn_context failed: %v", err)
	}
	if len(evsTC) != 1 || evsTC[0].EventType != "model_change" {
		t.Fatalf("expected model_change event, got: %+v", evsTC)
	}
	if evsTC[0].Data.ModelContextWindow == nil || *evsTC[0].Data.ModelContextWindow != 128000 {
		t.Fatalf("expected model_change ModelContextWindow 128000, got %v", evsTC[0].Data.ModelContextWindow)
	}

	// 2. token_count event
	tokenLine := `{
		"type": "event_msg",
		"payload": {
			"type": "token_count",
			"turn_id": "turn-1",
			"info": {
				"total_token_usage": {
					"total_tokens": 5000,
					"prompt_tokens": 4000,
					"completion_tokens": 1000,
					"cached_prompt_tokens": 500,
					"reasoning_tokens": 200
				},
				"last_token_usage": {
					"total_tokens": 800,
					"prompt_tokens": 600,
					"completion_tokens": 200,
					"cached_prompt_tokens": 100,
					"reasoning_tokens": 50
				},
				"model_context_window": 128000
			}
		}
	}`

	evs, err := parser.ParseCodexLine([]byte(tokenLine))
	if err != nil {
		t.Fatalf("ParseCodexLine failed: %v", err)
	}
	if len(evs) != 1 || evs[0].EventType != "turn_end" {
		t.Fatalf("expected 1 turn_end event, got: %+v", evs)
	}

	u := evs[0].Data.Usage
	if u == nil {
		t.Fatalf("expected non-nil Usage")
	}

	// Fresh input must be PromptTokens - CachedPromptTokens = 600 - 100 = 500
	if u.Input != 500 {
		t.Errorf("u.Input (fresh input) = %d, want 500", u.Input)
	}
	if u.CacheRead != 100 {
		t.Errorf("u.CacheRead = %d, want 100", u.CacheRead)
	}
	if u.Output != 200 {
		t.Errorf("u.Output = %d, want 200", u.Output)
	}
	if u.Total != 800 {
		t.Errorf("u.Total = %d, want 800", u.Total)
	}

	if evs[0].Data.ModelContextWindow == nil || *evs[0].Data.ModelContextWindow != 128000 {
		t.Fatalf("expected ModelContextWindow 128000, got %v", evs[0].Data.ModelContextWindow)
	}

	expectedFullness := 800.0 / 128000.0
	if evs[0].Data.ContextFullness == nil || math.Abs(*evs[0].Data.ContextFullness-expectedFullness) > 1e-6 {
		t.Fatalf("expected ContextFullness %f, got %v", expectedFullness, evs[0].Data.ContextFullness)
	}
}
