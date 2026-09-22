// Package parser provides streaming log parsers for AI agent harnesses.
//
// Attribution:
// Derived from and inspired by Chitragupt (https://github.com/sonnes/chitragupt),
// OpenUsage (https://github.com/janekbaraniewski/openusage), and
// Claude Agent SDK Golang (https://github.com/ProjAnvil/claude-agent-sdk-golang).
package parser

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

type ParsedEvent struct {
	EventType string                   `json:"eventType"`
	Timestamp int64                    `json:"timestamp"`
	CWD       string                   `json:"cwd"`
	GitBranch string                   `json:"gitBranch"`
	Data      types.TelemetryFrameData `json:"data"`
}

type rawContentBlock struct {
	Type      string                 `json:"type"` // "thinking" | "tool_use" | "text" | "tool_result"
	Text      string                 `json:"text,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"`
	Name      string                 `json:"name,omitempty"`
	ID        string                 `json:"id,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	IsError   *bool                  `json:"is_error,omitempty"`
}

type rawUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	ThinkingTokens           int64 `json:"thinking_tokens"`
	ReasoningTokens          int64 `json:"reasoning_tokens"`
}

type rawMessage struct {
	Role           string      `json:"role"`
	Model          string      `json:"model"`
	StopReason     string      `json:"stop_reason"`
	Content        interface{} `json:"content"`
	Usage          *rawUsage   `json:"usage"`
	Error          string      `json:"error,omitempty"`
	ApiErrorStatus *int        `json:"apiErrorStatus,omitempty"`
}

type rawJSONLine struct {
	Type              string      `json:"type"`
	UUID              string      `json:"uuid"`
	ParentUUID        *string     `json:"parentUuid"`
	SessionID         string      `json:"sessionId"`
	Timestamp         interface{} `json:"timestamp"`
	CWD               string      `json:"cwd"`
	GitBranch         string      `json:"gitBranch"`
	IsSidechain       bool        `json:"isSidechain"`
	Title             string      `json:"title"`
	AITitle           string      `json:"aiTitle"`
	LastPrompt        string      `json:"lastPrompt"`
	Message           *rawMessage `json:"message"`
	Content           interface{} `json:"content"`
	Question          string      `json:"question"`
	Error             string      `json:"error,omitempty"`
	ApiErrorStatus    *int        `json:"apiErrorStatus,omitempty"`
	IsApiErrorMessage bool        `json:"isApiErrorMessage,omitempty"`
}

func parseTimestamp(v interface{}) int64 {
	if v == nil {
		return time.Now().UnixMilli()
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	case string:
		if val == "" {
			return time.Now().UnixMilli()
		}
		if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
			return t.UnixMilli()
		}
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return t.UnixMilli()
		}
	}
	return time.Now().UnixMilli()
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func extractContentString(c interface{}) string {
	if c == nil {
		return ""
	}
	switch v := c.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func parseClaudeContentBlocks(content interface{}) []rawContentBlock {
	if content == nil {
		return nil
	}

	switch v := content.(type) {
	case string:
		return []rawContentBlock{{Type: "text", Text: v}}
	case []interface{}:
		var blocks []rawContentBlock
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				var b rawContentBlock
				data, err := json.Marshal(m)
				if err == nil && json.Unmarshal(data, &b) == nil {
					if b.Type == "" && b.Text != "" {
						b.Type = "text"
					}
					blocks = append(blocks, b)
				}
			}
		}
		return blocks
	case map[string]interface{}:
		var b rawContentBlock
		data, err := json.Marshal(v)
		if err == nil && json.Unmarshal(data, &b) == nil {
			if b.Type == "" && b.Text != "" {
				b.Type = "text"
			}
			return []rawContentBlock{b}
		}
	}
	return nil
}

func makeClaudeCodeRunStartedEvent(raw rawJSONLine, ts int64) ParsedEvent {
	var titlePtr *string
	if raw.AITitle != "" {
		titlePtr = stringPtr(raw.AITitle)
	} else if raw.Title != "" {
		titlePtr = stringPtr(raw.Title)
	} else if raw.SessionID != "" {
		titlePtr = stringPtr("Claude Code Session " + raw.SessionID)
	}

	var modelPtr *string
	if raw.Message != nil && raw.Message.Model != "" {
		modelPtr = stringPtr(raw.Message.Model)
	}

	return ParsedEvent{
		EventType: "run_started",
		Timestamp: ts,
		CWD:       raw.CWD,
		GitBranch: raw.GitBranch,
		Data: types.TelemetryFrameData{
			Title: titlePtr,
			Model: modelPtr,
		},
	}
}

// ClaudeCodeParser maintains session stream state when parsing Claude Code transcripts.
type ClaudeCodeParser struct {
	mu           sync.Mutex
	hasStarted   bool
	seenSessions map[string]bool
}

// NewClaudeCodeParser creates a new stateful parser stream for Claude Code transcripts.
func NewClaudeCodeParser() *ClaudeCodeParser {
	return &ClaudeCodeParser{
		seenSessions: make(map[string]bool),
	}
}

// Reset resets the parser stream state.
func (p *ClaudeCodeParser) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hasStarted = false
	p.seenSessions = make(map[string]bool)
}

// ParseLine parses a single line of Claude Code JSONL log output in a session stream.
// It synthesizes a run_started event on the first line encountered.
func (p *ClaudeCodeParser) ParseLine(line []byte) ([]ParsedEvent, error) {
	if len(line) == 0 {
		return nil, nil
	}

	var raw rawJSONLine
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling jsonl line: %w", err)
	}

	ts := parseTimestamp(raw.Timestamp)
	events := parseClaudeCodeLineEvents(raw, ts)

	p.mu.Lock()
	emitRunStarted := false
	if raw.SessionID != "" {
		if !p.seenSessions[raw.SessionID] {
			p.seenSessions[raw.SessionID] = true
			p.hasStarted = true
			emitRunStarted = true
		}
	} else if !p.hasStarted {
		p.hasStarted = true
		emitRunStarted = true
	}
	p.mu.Unlock()

	if emitRunStarted {
		runStarted := makeClaudeCodeRunStartedEvent(raw, ts)
		events = append([]ParsedEvent{runStarted}, events...)
	}

	return events, nil
}

var (
	defaultClaudeMu       sync.Mutex
	defaultClaudeSessions = make(map[string]bool)
)

// ResetClaudeCodeSessions clears tracked sessions for the package-level ParseClaudeCodeLine.
func ResetClaudeCodeSessions() {
	defaultClaudeMu.Lock()
	defer defaultClaudeMu.Unlock()
	defaultClaudeSessions = make(map[string]bool)
}

// ResetClaudeCodeSession clears a specific tracked session ID.
func ResetClaudeCodeSession(sessionID string) {
	defaultClaudeMu.Lock()
	defer defaultClaudeMu.Unlock()
	delete(defaultClaudeSessions, sessionID)
}

// ParseClaudeCodeLine parses a single line of Claude Code JSONL log output into ParsedEvent structs.
// If the line contains a SessionID and run_started has not yet been emitted for that session,
// a run_started event is synthesized and prepended.
func ParseClaudeCodeLine(line []byte) ([]ParsedEvent, error) {
	if len(line) == 0 {
		return nil, nil
	}

	var raw rawJSONLine
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling jsonl line: %w", err)
	}

	ts := parseTimestamp(raw.Timestamp)
	events := parseClaudeCodeLineEvents(raw, ts)

	emitRunStarted := false
	if raw.SessionID != "" {
		defaultClaudeMu.Lock()
		if !defaultClaudeSessions[raw.SessionID] {
			defaultClaudeSessions[raw.SessionID] = true
			emitRunStarted = true
		}
		defaultClaudeMu.Unlock()
	}

	if emitRunStarted {
		runStarted := makeClaudeCodeRunStartedEvent(raw, ts)
		events = append([]ParsedEvent{runStarted}, events...)
	}

	return events, nil
}

func parseClaudeCodeLineEvents(raw rawJSONLine, ts int64) []ParsedEvent {
	var events []ParsedEvent

	switch raw.Type {
	case "ai-title", "title":
		titleVal := raw.AITitle
		if titleVal == "" {
			titleVal = raw.Title
		}
		if titleVal == "" && raw.Content != nil {
			titleVal = extractContentString(raw.Content)
		}
		if titleVal != "" {
			events = append(events, ParsedEvent{
				EventType: "title_change",
				Timestamp: ts,
				CWD:       raw.CWD,
				GitBranch: raw.GitBranch,
				Data: types.TelemetryFrameData{
					Title: stringPtr(titleVal),
				},
			})
		}

	case "last-prompt":
		promptVal := raw.LastPrompt
		if promptVal == "" && raw.Content != nil {
			promptVal = extractContentString(raw.Content)
		}
		if promptVal != "" {
			events = append(events, ParsedEvent{
				EventType: "turn_start",
				Timestamp: ts,
				CWD:       raw.CWD,
				GitBranch: raw.GitBranch,
				Data: types.TelemetryFrameData{
					LastPrompt: stringPtr(promptVal),
				},
			})
		}

	case "user":
		if raw.Message != nil {
			blocks := parseClaudeContentBlocks(raw.Message.Content)
			var textParts []string
			hasToolResult := false

			for _, b := range blocks {
				switch b.Type {
				case "text":
					if b.Text != "" {
						textParts = append(textParts, b.Text)
					}
				case "tool_result":
					hasToolResult = true
					output := extractContentString(b.Content)
					if output == "" && b.Text != "" {
						output = b.Text
					}
					isErr := false
					if b.IsError != nil {
						isErr = *b.IsError
					}

					var toolCallID *string
					if b.ToolUseID != "" {
						toolCallID = stringPtr(b.ToolUseID)
					} else if b.ID != "" {
						toolCallID = stringPtr(b.ID)
					}

					events = append(events, ParsedEvent{
						EventType: "tool_result",
						Timestamp: ts,
						CWD:       raw.CWD,
						GitBranch: raw.GitBranch,
						Data: types.TelemetryFrameData{
							Role:       stringPtr("user"),
							ToolCallID: toolCallID,
							ToolOutput: stringPtr(output),
							IsError:    boolPtr(isErr),
						},
					})
				}
			}

			if len(textParts) > 0 && !hasToolResult {
				fullPrompt := strings.Join(textParts, "\n")
				events = append(events, ParsedEvent{
					EventType: "turn_start",
					Timestamp: ts,
					CWD:       raw.CWD,
					GitBranch: raw.GitBranch,
					Data: types.TelemetryFrameData{
						Role:       stringPtr("user"),
						Content:    stringPtr(fullPrompt),
						LastPrompt: stringPtr(fullPrompt),
					},
				})
			}
		}

	case "assistant":
		var modelPtr *string
		if raw.Message != nil && raw.Message.Model != "" {
			modelPtr = stringPtr(raw.Message.Model)
		}

		if raw.Message != nil {
			blocks := parseClaudeContentBlocks(raw.Message.Content)
			var textParts []string

			for _, b := range blocks {
				switch b.Type {
				case "text":
					if b.Text != "" {
						textParts = append(textParts, b.Text)
					}
				case "thinking":
					if b.Thinking != "" {
						events = append(events, ParsedEvent{
							EventType: "thinking",
							Timestamp: ts,
							CWD:       raw.CWD,
							GitBranch: raw.GitBranch,
							Data: types.TelemetryFrameData{
								Role:    stringPtr("assistant"),
								Content: stringPtr(b.Thinking),
								Model:   modelPtr,
							},
						})
					}
				case "tool_use":
					toolName := b.Name
					var callID *string
					if b.ID != "" {
						callID = stringPtr(b.ID)
					}

					if toolName == "AskUserQuestion" {
						qText := ""
						if b.Input != nil {
							if q, ok := b.Input["question"].(string); ok {
								qText = q
							} else if p, ok := b.Input["prompt"].(string); ok {
								qText = p
							} else if qs, ok := b.Input["questions"].([]interface{}); ok && len(qs) > 0 {
								if qObj, ok := qs[0].(map[string]interface{}); ok {
									if q, ok := qObj["question"].(string); ok {
										qText = q
									}
								}
							}
						}
						events = append(events, ParsedEvent{
							EventType: "question_prompt",
							Timestamp: ts,
							CWD:       raw.CWD,
							GitBranch: raw.GitBranch,
							Data: types.TelemetryFrameData{
								Role:       stringPtr("assistant"),
								ToolName:   stringPtr(toolName),
								ToolCallID: callID,
								ToolInput:  b.Input,
								Question:   stringPtr(qText),
								Model:      modelPtr,
							},
						})
					} else {
						events = append(events, ParsedEvent{
							EventType: "tool_call",
							Timestamp: ts,
							CWD:       raw.CWD,
							GitBranch: raw.GitBranch,
							Data: types.TelemetryFrameData{
								Role:       stringPtr("assistant"),
								ToolName:   stringPtr(toolName),
								ToolCallID: callID,
								ToolInput:  b.Input,
								Model:      modelPtr,
							},
						})
					}
				}
			}

			// Turn End with Usage
			var usage *types.ITokenUsage
			if raw.Message.Usage != nil {
				u := raw.Message.Usage
				reasoning := u.ThinkingTokens
				if reasoning == 0 {
					reasoning = u.ReasoningTokens
				}
				total := u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
				usage = &types.ITokenUsage{
					Input:      u.InputTokens,
					Output:     u.OutputTokens,
					CacheRead:  u.CacheReadInputTokens,
					CacheWrite: u.CacheCreationInputTokens,
					Reasoning:  reasoning,
					Total:      total,
				}
			}

			var stopReason *string
			if raw.Message.StopReason != "" {
				stopReason = stringPtr(raw.Message.StopReason)
			}

			var contentPtr *string
			if len(textParts) > 0 {
				contentPtr = stringPtr(strings.Join(textParts, "\n"))
			}

			events = append(events, ParsedEvent{
				EventType: "turn_end",
				Timestamp: ts,
				CWD:       raw.CWD,
				GitBranch: raw.GitBranch,
				Data: types.TelemetryFrameData{
					Role:       stringPtr("assistant"),
					Model:      modelPtr,
					StopReason: stopReason,
					Usage:      usage,
					Content:    contentPtr,
				},
			})
		}

		// Check for error fields
		hasError := raw.Error != "" ||
			raw.IsApiErrorMessage ||
			(raw.ApiErrorStatus != nil && *raw.ApiErrorStatus > 0) ||
			(raw.Message != nil && (raw.Message.Error != "" || (raw.Message.ApiErrorStatus != nil && *raw.Message.ApiErrorStatus > 0)))

		if hasError {
			errText := raw.Error
			if errText == "" && raw.Message != nil && raw.Message.Error != "" {
				errText = raw.Message.Error
			}
			if errText == "" && raw.Message != nil && raw.Message.Content != nil {
				errText = extractContentString(raw.Message.Content)
			}
			if errText == "" && raw.Content != nil {
				errText = extractContentString(raw.Content)
			}
			if errText == "" {
				if raw.ApiErrorStatus != nil && *raw.ApiErrorStatus > 0 {
					errText = fmt.Sprintf("API error: status %d", *raw.ApiErrorStatus)
				} else if raw.Message != nil && raw.Message.ApiErrorStatus != nil && *raw.Message.ApiErrorStatus > 0 {
					errText = fmt.Sprintf("API error: status %d", *raw.Message.ApiErrorStatus)
				} else {
					errText = "API error"
				}
			}

			events = append(events, ParsedEvent{
				EventType: "error",
				Timestamp: ts,
				CWD:       raw.CWD,
				GitBranch: raw.GitBranch,
				Data: types.TelemetryFrameData{
					Role:    stringPtr("assistant"),
					Content: stringPtr(errText),
					Model:   modelPtr,
					IsError: boolPtr(true),
				},
			})
		}
	}

	return events
}
