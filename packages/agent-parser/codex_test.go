package parser

import (
	"testing"
)

func TestParseCodexRolloutPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantYear    string
		wantMonth   string
		wantDay     string
		wantTS      string
		wantSessID  string
		wantErr     bool
	}{
		{
			name:       "standard date-partitioned unix path",
			path:       "/Users/dev/.codex/sessions/2026/06/12/rollout-2026-06-12T16-08-36-018f4a7c-1234-7000-8000-abcdef123456.jsonl",
			wantYear:   "2026",
			wantMonth:  "06",
			wantDay:    "12",
			wantTS:     "2026-06-12T16-08-36",
			wantSessID: "018f4a7c-1234-7000-8000-abcdef123456",
			wantErr:    false,
		},
		{
			name:       "windows date-partitioned path",
			path:       `C:\Users\dev\.codex\sessions\2026\09\20\rollout-2026-09-20T10-15-00-uuid-1234.jsonl`,
			wantYear:   "2026",
			wantMonth:  "09",
			wantDay:    "20",
			wantTS:     "2026-09-20T10-15-00",
			wantSessID: "uuid-1234",
			wantErr:    false,
		},
		{
			name:       "flat rollout filename",
			path:       "rollout-2026-06-12T16-08-36-my-session-uuid.jsonl",
			wantYear:   "",
			wantMonth:  "",
			wantDay:    "",
			wantTS:     "2026-06-12T16-08-36",
			wantSessID: "my-session-uuid",
			wantErr:    false,
		},
		{
			name:       "codex partitioned fallback rollout filename",
			path:       "/Users/dev/.codex/sessions/2026/06/12/rollout-customtimestamp-01900e70-659f-728b-b8dc-4c4f9bcde27a.jsonl",
			wantYear:   "2026",
			wantMonth:  "06",
			wantDay:    "12",
			wantTS:     "customtimestamp",
			wantSessID: "01900e70-659f-728b-b8dc-4c4f9bcde27a",
			wantErr:    false,
		},
		{
			name:       "codex flat fallback rollout filename",
			path:       "/Users/dev/rollout-customtimestamp-01900e70-659f-728b-b8dc-4c4f9bcde27a.jsonl",
			wantYear:   "",
			wantMonth:  "",
			wantDay:    "",
			wantTS:     "customtimestamp",
			wantSessID: "01900e70-659f-728b-b8dc-4c4f9bcde27a",
			wantErr:    false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "unrelated file path",
			path:    "/var/log/syslog",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseCodexRolloutPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseCodexRolloutPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if info.Year != tt.wantYear {
				t.Errorf("Year = %q, want %q", info.Year, tt.wantYear)
			}
			if info.Month != tt.wantMonth {
				t.Errorf("Month = %q, want %q", info.Month, tt.wantMonth)
			}
			if info.Day != tt.wantDay {
				t.Errorf("Day = %q, want %q", info.Day, tt.wantDay)
			}
			if info.TimestampString != tt.wantTS {
				t.Errorf("TimestampString = %q, want %q", info.TimestampString, tt.wantTS)
			}
			if info.SessionID != tt.wantSessID {
				t.Errorf("SessionID = %q, want %q", info.SessionID, tt.wantSessID)
			}
		})
	}
}

func TestCleanCodexPromptAndTitle(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantClean   string
		wantTitle   string
		wantHasTile bool
	}{
		{
			name:        "pure environment context is filtered out",
			input:       "<environment_context>\nOS: Darwin\nCWD: /repo\n</environment_context>",
			wantClean:   "",
			wantTitle:   "",
			wantHasTile: false,
		},
		{
			name:        "pure unclosed environment context is filtered out",
			input:       "<environment_context>\nOS: Linux",
			wantClean:   "",
			wantTitle:   "",
			wantHasTile: false,
		},
		{
			name:        "pure session context is filtered out",
			input:       "<session_context>\nPrior messages: ...\n</session_context>",
			wantClean:   "",
			wantTitle:   "",
			wantHasTile: false,
		},
		{
			name:        "environment context followed by user prompt",
			input:       "<environment_context>\nCWD: /repo\n</environment_context>\n\nRefactor the auth handler in auth.go",
			wantClean:   "Refactor the auth handler in auth.go",
			wantTitle:   "Refactor the auth handler in auth.go",
			wantHasTile: true,
		},
		{
			name:        "user prompt exceeding 100 characters is truncated",
			input:       "This is an extremely long user task prompt that goes on and on beyond one hundred characters in total length to verify truncation",
			wantClean:   "This is an extremely long user task prompt that goes on and on beyond one hundred characters in total length to verify truncation",
			wantTitle:   "This is an extremely long user task prompt that goes on and on beyond one hundred characters in tota",
			wantHasTile: true,
		},
		{
			name:        "multi-line user prompt extracts first line as title",
			input:       "Fix the database query deadlock\n\nMake sure to run with WAL mode and verify under load.",
			wantClean:   "Fix the database query deadlock\n\nMake sure to run with WAL mode and verify under load.",
			wantTitle:   "Fix the database query deadlock",
			wantHasTile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClean := CleanCodexPrompt(tt.input)
			if gotClean != tt.wantClean {
				t.Errorf("CleanCodexPrompt() = %q, want %q", gotClean, tt.wantClean)
			}

			title, hasTitle := ExtractCodexTitleFromPrompt(tt.input)
			if hasTitle != tt.wantHasTile {
				t.Errorf("ExtractCodexTitleFromPrompt() hasTitle = %v, want %v", hasTitle, tt.wantHasTile)
			}
			if title != tt.wantTitle {
				t.Errorf("ExtractCodexTitleFromPrompt() title = %q, want %q", title, tt.wantTitle)
			}
		})
	}
}

func TestParseCodexLine(t *testing.T) {
	t.Run("empty or whitespace line", func(t *testing.T) {
		evs, err := ParseCodexLine([]byte("   \n"))
		if err != nil || len(evs) != 0 {
			t.Errorf("expected empty events for whitespace line, got evs=%v err=%v", evs, err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseCodexLine([]byte("{not valid json"))
		if err == nil {
			t.Errorf("expected error for invalid json")
		}
	})

	t.Run("session_meta root CLI session", func(t *testing.T) {
		line := `{
			"type": "session_meta",
			"timestamp": "2026-06-12T16:08:36.123Z",
			"payload": {
				"id": "sess-root-123",
				"timestamp": "2026-06-12T16:08:36.123Z",
				"cwd": "/Users/dev/project",
				"originator": "codex-tui",
				"cli_version": "0.121.0",
				"source": "cli",
				"model_provider": "openai",
				"git": {
					"commit_hash": "abcdef123",
					"branch": "feature/rollout",
					"repository_url": "git@github.com:org/repo.git"
				}
			}
		}`
		evs, err := ParseCodexLine([]byte(line))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		e := evs[0]
		if e.EventType != "run_started" {
			t.Errorf("EventType = %q, want 'run_started'", e.EventType)
		}
		if e.CWD != "/Users/dev/project" {
			t.Errorf("CWD = %q, want '/Users/dev/project'", e.CWD)
		}
		if e.GitBranch != "feature/rollout" {
			t.Errorf("GitBranch = %q, want 'feature/rollout'", e.GitBranch)
		}
		if e.Data.Title == nil || *e.Data.Title != "Codex Session sess-root-123" {
			t.Errorf("Title = %v, want 'Codex Session sess-root-123'", e.Data.Title)
		}
		if e.Data.Model == nil || *e.Data.Model != "openai" {
			t.Errorf("Model = %v, want 'openai'", e.Data.Model)
		}
	})

	t.Run("session_meta subagent with thread_spawn source", func(t *testing.T) {
		line := `{
			"type": "session_meta",
			"timestamp": "2026-06-12T16:08:36.123Z",
			"payload": {
				"id": "sess-child-456",
				"cwd": "/Users/dev/project",
				"source": {
					"subagent": {
						"thread_spawn": {
							"parent_thread_id": "sess-root-123",
							"depth": 1,
							"agent_nickname": "Hubble",
							"agent_role": "explorer"
						}
					}
				}
			}
		}`
		evs, err := ParseCodexLine([]byte(line))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "run_started" {
			t.Errorf("EventType = %q, want 'run_started'", evs[0].EventType)
		}
		if evs[0].Data.Title == nil || *evs[0].Data.Title != "Codex Subagent: Hubble (explorer)" {
			t.Errorf("Title = %v, want 'Codex Subagent: Hubble (explorer)'", evs[0].Data.Title)
		}
	})

	t.Run("turn_context model switch", func(t *testing.T) {
		line := `{
			"type": "turn_context",
			"timestamp": "2026-06-12T16:08:37.000Z",
			"payload": {
				"turn_id": "turn-1",
				"cwd": "/Users/dev/project",
				"model": "gpt-5.4",
				"approval_policy": "on-request"
			}
		}`
		evs, err := ParseCodexLine([]byte(line))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "model_change" {
			t.Errorf("EventType = %q, want 'model_change'", evs[0].EventType)
		}
		if evs[0].Data.Model == nil || *evs[0].Data.Model != "gpt-5.4" {
			t.Errorf("Model = %v, want 'gpt-5.4'", evs[0].Data.Model)
		}
	})

	t.Run("event_msg user_message with synthetic context filtering", func(t *testing.T) {
		// Pure synthetic context -> no event
		syntheticLine := `{
			"type": "event_msg",
			"timestamp": "2026-06-12T16:08:38.000Z",
			"payload": {
				"type": "user_message",
				"message": "<environment_context>\nOS: Darwin\n</environment_context>"
			}
		}`
		evs1, err := ParseCodexLine([]byte(syntheticLine))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(evs1) != 0 {
			t.Errorf("expected 0 events for pure synthetic user message, got %d", len(evs1))
		}

		// User prompt with environment context header
		mixedLine := `{
			"type": "event_msg",
			"timestamp": "2026-06-12T16:08:39.000Z",
			"payload": {
				"type": "user_message",
				"message": "<environment_context>\nCWD: /repo\n</environment_context>\n\nImplement streaming tests"
			}
		}`
		evs2, err := ParseCodexLine([]byte(mixedLine))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(evs2) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs2))
		}
		if evs2[0].EventType != "turn_start" {
			t.Errorf("EventType = %q, want 'turn_start'", evs2[0].EventType)
		}
		if evs2[0].Data.Content == nil || *evs2[0].Data.Content != "Implement streaming tests" {
			t.Errorf("Content = %v, want 'Implement streaming tests'", evs2[0].Data.Content)
		}
		if evs2[0].Data.Title == nil || *evs2[0].Data.Title != "Implement streaming tests" {
			t.Errorf("Title = %v, want 'Implement streaming tests'", evs2[0].Data.Title)
		}
	})

	t.Run("event_msg agent_message and reasoning", func(t *testing.T) {
		agentMsgLine := `{
			"type": "event_msg",
			"timestamp": "2026-06-12T16:08:40.000Z",
			"payload": {
				"type": "agent_message",
				"message": "All unit tests pass."
			}
		}`
		evs1, err := ParseCodexLine([]byte(agentMsgLine))
		if err != nil || len(evs1) != 1 {
			t.Fatalf("failed agent_message parsing: evs=%v err=%v", evs1, err)
		}
		if evs1[0].EventType != "agent_message" || *evs1[0].Data.Content != "All unit tests pass." {
			t.Errorf("unexpected agent_message event: %+v", evs1[0])
		}

		reasoningLine := `{
			"type": "event_msg",
			"timestamp": "2026-06-12T16:08:41.000Z",
			"payload": {
				"type": "reasoning",
				"summary": [
					{"type": "text", "text": "Analyzing the parser code structure..."}
				]
			}
		}`
		evs2, err := ParseCodexLine([]byte(reasoningLine))
		if err != nil || len(evs2) != 1 {
			t.Fatalf("failed reasoning parsing: evs=%v err=%v", evs2, err)
		}
		if evs2[0].EventType != "thinking" || *evs2[0].Data.Content != "Analyzing the parser code structure..." {
			t.Errorf("unexpected thinking event: %+v", evs2[0])
		}
	})

	t.Run("event_msg token_count usage breakdown", func(t *testing.T) {
		line := `{
			"type": "event_msg",
			"timestamp": "2026-06-12T16:08:42.000Z",
			"payload": {
				"type": "token_count",
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
		evs, err := ParseCodexLine([]byte(line))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "turn_end" {
			t.Errorf("EventType = %q, want 'turn_end'", evs[0].EventType)
		}
		u := evs[0].Data.Usage
		if u == nil {
			t.Fatalf("expected non-nil Usage")
		}
		if u.Input != 600 || u.Output != 200 || u.CacheRead != 100 || u.Reasoning != 50 || u.Total != 800 {
			t.Errorf("Usage mismatch: %+v", u)
		}
	})

	t.Run("event_msg error", func(t *testing.T) {
		line := `{
			"type": "event_msg",
			"timestamp": "2026-06-12T16:08:43.000Z",
			"payload": {
				"type": "error",
				"message": "Connection rate limit exceeded",
				"code": 429
			}
		}`
		evs, err := ParseCodexLine([]byte(line))
		if err != nil || len(evs) != 1 {
			t.Fatalf("failed error event parsing: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "error" || *evs[0].Data.Content != "Connection rate limit exceeded" || !*evs[0].Data.IsError {
			t.Errorf("unexpected error event: %+v", evs[0])
		}
	})

	t.Run("response_item function_call exec_command with JSON-encoded arguments", func(t *testing.T) {
		line := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:44.000Z",
			"payload": {
				"type": "function_call",
				"name": "exec_command",
				"call_id": "call_exec_001",
				"arguments": "{\"cmd\": \"go test -v ./...\", \"workdir\": \"/project\"}"
			}
		}`
		evs, err := ParseCodexLine([]byte(line))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}
		if evs[0].EventType != "tool_call" {
			t.Errorf("EventType = %q, want 'tool_call'", evs[0].EventType)
		}
		if *evs[0].Data.ToolName != "exec_command" || *evs[0].Data.ToolCallID != "call_exec_001" {
			t.Errorf("ToolName or ToolCallID mismatch: %+v", evs[0].Data)
		}
		if evs[0].Data.ToolInput["cmd"] != "go test -v ./..." || evs[0].Data.ToolInput["command"] != "go test -v ./..." {
			t.Errorf("ToolInput command mismatch: %+v", evs[0].Data.ToolInput)
		}
	})

	t.Run("response_item function_call spawn_agent", func(t *testing.T) {
		line := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:45.000Z",
			"payload": {
				"type": "function_call",
				"name": "spawn_agent",
				"call_id": "call_spawn_002",
				"arguments": "{\"agent_type\": \"explorer\", \"message\": \"Find all endpoints in router.go\"}"
			}
		}`
		evs, err := ParseCodexLine([]byte(line))
		if err != nil || len(evs) != 1 {
			t.Fatalf("failed spawn_agent parsing: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "tool_call" || *evs[0].Data.ToolName != "spawn_agent" {
			t.Errorf("tool_call mismatch: %+v", evs[0])
		}
		if evs[0].Data.ToolInput["prompt"] != "Find all endpoints in router.go" {
			t.Errorf("prompt mapping mismatch: %+v", evs[0].Data.ToolInput)
		}
	})

	t.Run("response_item function_call AskUserQuestion", func(t *testing.T) {
		line := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:46.000Z",
			"payload": {
				"type": "function_call",
				"name": "AskUserQuestion",
				"call_id": "call_ask_003",
				"arguments": "{\"question\": \"Should I apply migration 002?\"}"
			}
		}`
		evs, err := ParseCodexLine([]byte(line))
		if err != nil || len(evs) != 1 {
			t.Fatalf("failed AskUserQuestion parsing: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "question_prompt" || *evs[0].Data.Question != "Should I apply migration 002?" {
			t.Errorf("question_prompt mismatch: %+v", evs[0])
		}
	})

	t.Run("response_item function_call_output success and error", func(t *testing.T) {
		successLine := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:47.000Z",
			"payload": {
				"type": "function_call_output",
				"call_id": "call_exec_001",
				"output": "Exit code: 0\nPASS\nok packages/agent-parser"
			}
		}`
		evs1, err := ParseCodexLine([]byte(successLine))
		if err != nil || len(evs1) != 1 {
			t.Fatalf("failed function_call_output parsing: evs=%v err=%v", evs1, err)
		}
		if evs1[0].EventType != "tool_result" || *evs1[0].Data.ToolCallID != "call_exec_001" || *evs1[0].Data.IsError {
			t.Errorf("unexpected success tool_result: %+v", evs1[0])
		}

		errorLine := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:48.000Z",
			"payload": {
				"type": "function_call_output",
				"call_id": "call_exec_002",
				"output": "Exit code: 1\nFAIL\ncommand failed with exit code 1"
			}
		}`
		evs2, err := ParseCodexLine([]byte(errorLine))
		if err != nil || len(evs2) != 1 {
			t.Fatalf("failed function_call_output error parsing: evs=%v err=%v", evs2, err)
		}
		if evs2[0].EventType != "tool_result" || !*evs2[0].Data.IsError {
			t.Errorf("expected isError true for exit code 1, got %+v", evs2[0])
		}
	})

	t.Run("response_item message user and assistant", func(t *testing.T) {
		userLine := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:49.000Z",
			"payload": {
				"type": "message",
				"role": "user",
				"content": [
					{"type": "input_text", "text": "Run the tests"}
				]
			}
		}`
		evs1, err := ParseCodexLine([]byte(userLine))
		if err != nil || len(evs1) != 1 {
			t.Fatalf("failed user response_item message: evs=%v err=%v", evs1, err)
		}
		if evs1[0].EventType != "turn_start" || *evs1[0].Data.Content != "Run the tests" {
			t.Errorf("unexpected turn_start: %+v", evs1[0])
		}

		assistantLine := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:50.000Z",
			"payload": {
				"type": "message",
				"role": "assistant",
				"content": [
					{"type": "output_text", "text": "Tests completed successfully."}
				]
			}
		}`
		evs2, err := ParseCodexLine([]byte(assistantLine))
		if err != nil || len(evs2) != 1 {
			t.Fatalf("failed assistant response_item message: evs=%v err=%v", evs2, err)
		}
		if evs2[0].EventType != "agent_message" || *evs2[0].Data.Content != "Tests completed successfully." {
			t.Errorf("unexpected agent_message: %+v", evs2[0])
		}
	})

	t.Run("response_item tool_search_call and tool_search_output", func(t *testing.T) {
		callLine := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:51.000Z",
			"payload": {
				"type": "tool_search_call",
				"call_id": "ts_001",
				"query": "find files matching *.go"
			}
		}`
		evs1, err := ParseCodexLine([]byte(callLine))
		if err != nil || len(evs1) != 1 {
			t.Fatalf("failed tool_search_call parsing: evs=%v err=%v", evs1, err)
		}
		if evs1[0].EventType != "tool_call" || *evs1[0].Data.ToolName != "tool_search" {
			t.Errorf("unexpected tool_search_call event: %+v", evs1[0])
		}

		outLine := `{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:52.000Z",
			"payload": {
				"type": "tool_search_output",
				"call_id": "ts_001",
				"output": "Found 12 matching files"
			}
		}`
		evs2, err := ParseCodexLine([]byte(outLine))
		if err != nil || len(evs2) != 1 {
			t.Fatalf("failed tool_search_output parsing: evs=%v err=%v", evs2, err)
		}
		if evs2[0].EventType != "tool_result" || *evs2[0].Data.ToolCallID != "ts_001" {
			t.Errorf("unexpected tool_search_output event: %+v", evs2[0])
		}
	})

	t.Run("session_meta subagent with only nickname", func(t *testing.T) {
		line := []byte(`{"type":"session_meta","timestamp":"2026-06-12T16:08:36.000Z","payload":{"id":"sub-1","agent_nickname":"Scout"}}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: %v", err)
		}
		if *evs[0].Data.Title != "Codex Subagent: Scout" {
			t.Errorf("got %q, want 'Codex Subagent: Scout'", *evs[0].Data.Title)
		}
	})

	t.Run("session_meta subagent with only parentThreadID", func(t *testing.T) {
		line := []byte(`{"type":"session_meta","timestamp":"2026-06-12T16:08:36.000Z","payload":{"id":"sub-2","parent_thread_id":"root-1"}}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: %v", err)
		}
		if *evs[0].Data.Title != "Codex Subagent sub-2" {
			t.Errorf("got %q, want 'Codex Subagent sub-2'", *evs[0].Data.Title)
		}
	})

	t.Run("session_meta with string subagent source", func(t *testing.T) {
		line := []byte(`{"type":"session_meta","timestamp":"2026-06-12T16:08:36.000Z","payload":{"id":"sub-3","source":{"subagent":"worker"}}}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: %v", err)
		}
		if *evs[0].Data.Title != "Codex Subagent: (worker)" {
			t.Errorf("got %q, want 'Codex Subagent: (worker)'", *evs[0].Data.Title)
		}
	})

	t.Run("invalid json payloads in codex line", func(t *testing.T) {
		_, err := ParseCodexLine([]byte(`{"type":"session_meta","payload":"invalid"}`))
		if err == nil {
			t.Error("expected error for invalid session_meta payload")
		}
		_, err = ParseCodexLine([]byte(`{"type":"turn_context","payload":"invalid"}`))
		if err == nil {
			t.Error("expected error for invalid turn_context payload")
		}
		_, err = ParseCodexLine([]byte(`{"type":"event_msg","payload":"invalid"}`))
		if err == nil {
			t.Error("expected error for invalid event_msg payload")
		}
		_, err = ParseCodexLine([]byte(`{"type":"response_item","payload":"invalid"}`))
		if err == nil {
			t.Error("expected error for invalid response_item payload")
		}
	})

	t.Run("session_meta with legacy session_id alias", func(t *testing.T) {
		line := []byte(`{"type":"session_meta","timestamp":"2026-06-12T16:08:36.000Z","payload":{"session_id":"legacy-ses-999","cwd":"/repo"}}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "run_started" {
			t.Fatalf("EventType = %q, want 'run_started'", evs[0].EventType)
		}
		if evs[0].Data.Title == nil || *evs[0].Data.Title != "Codex Session legacy-ses-999" {
			t.Errorf("Title = %v, want 'Codex Session legacy-ses-999'", evs[0].Data.Title)
		}
	})

	t.Run("response_item custom_tool_call with arguments", func(t *testing.T) {
		line := []byte(`{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:44.000Z",
			"payload": {
				"type": "custom_tool_call",
				"name": "mcp_browser_click",
				"call_id": "call_mcp_001",
				"arguments": "{\"element_id\": \"btn-submit\"}"
			}
		}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "tool_call" {
			t.Errorf("EventType = %q, want 'tool_call'", evs[0].EventType)
		}
		if *evs[0].Data.ToolName != "mcp_browser_click" || *evs[0].Data.ToolCallID != "call_mcp_001" {
			t.Errorf("ToolName/ToolCallID mismatch: %+v", evs[0].Data)
		}
		if evs[0].Data.ToolInput["element_id"] != "btn-submit" {
			t.Errorf("ToolInput mismatch: %+v", evs[0].Data.ToolInput)
		}
	})

	t.Run("response_item custom_tool_call with input map", func(t *testing.T) {
		line := []byte(`{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:44.000Z",
			"payload": {
				"type": "custom_tool_call",
				"name": "mcp_search",
				"call_id": "call_mcp_002",
				"input": {"query": "auth handlers"}
			}
		}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "tool_call" {
			t.Errorf("EventType = %q, want 'tool_call'", evs[0].EventType)
		}
		if evs[0].Data.ToolInput["query"] != "auth handlers" {
			t.Errorf("ToolInput mismatch: %+v", evs[0].Data.ToolInput)
		}
	})

	t.Run("response_item function_call with input string", func(t *testing.T) {
		line := []byte(`{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:44.000Z",
			"payload": {
				"type": "function_call",
				"name": "exec_command",
				"call_id": "call_exec_input_str",
				"input": "{\"cmd\": \"git status\"}"
			}
		}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "tool_call" {
			t.Errorf("EventType = %q, want 'tool_call'", evs[0].EventType)
		}
		if evs[0].Data.ToolInput["command"] != "git status" {
			t.Errorf("ToolInput command mismatch: %+v", evs[0].Data.ToolInput)
		}
	})

	t.Run("response_item custom_tool_call_output", func(t *testing.T) {
		line := []byte(`{
			"type": "response_item",
			"timestamp": "2026-06-12T16:08:46.000Z",
			"payload": {
				"type": "custom_tool_call_output",
				"call_id": "call_mcp_001",
				"output": "Element clicked successfully"
			}
		}`)
		evs, err := ParseCodexLine(line)
		if err != nil || len(evs) != 1 {
			t.Fatalf("unexpected result: evs=%v err=%v", evs, err)
		}
		if evs[0].EventType != "tool_result" {
			t.Errorf("EventType = %q, want 'tool_result'", evs[0].EventType)
		}
		if *evs[0].Data.ToolCallID != "call_mcp_001" || *evs[0].Data.ToolOutput != "Element clicked successfully" {
			t.Errorf("ToolCallID/ToolOutput mismatch: %+v", evs[0].Data)
		}
		if *evs[0].Data.IsError != false {
			t.Errorf("expected IsError=false, got true")
		}
	})
}
