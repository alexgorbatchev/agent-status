package parser

import (
	"encoding/json"
	"fmt"
	"strings"
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
	Role       string            `json:"role"`
	Model      string            `json:"model"`
	StopReason string            `json:"stop_reason"`
	Content    []rawContentBlock `json:"content"`
	Usage      *rawUsage         `json:"usage"`
}

type rawJSONLine struct {
	Type        string      `json:"type"`
	UUID        string      `json:"uuid"`
	ParentUUID  *string     `json:"parentUuid"`
	SessionID   string      `json:"sessionId"`
	Timestamp   string      `json:"timestamp"`
	CWD         string      `json:"cwd"`
	GitBranch   string      `json:"gitBranch"`
	IsSidechain bool        `json:"isSidechain"`
	Title       string      `json:"title"`
	AITitle     string      `json:"aiTitle"`
	LastPrompt  string      `json:"lastPrompt"`
	Message     *rawMessage `json:"message"`
	Content     interface{} `json:"content"`
	Question    string      `json:"question"`
}

func parseTimestamp(tsStr string) int64 {
	if tsStr == "" {
		return time.Now().UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
		return t.UnixMilli()
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

func ParseClaudeCodeLine(line []byte) ([]ParsedEvent, error) {
	if len(line) == 0 {
		return nil, nil
	}

	var raw rawJSONLine
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling jsonl line: %w", err)
	}

	ts := parseTimestamp(raw.Timestamp)
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
		if raw.Message != nil && len(raw.Message.Content) > 0 {
			var textParts []string
			hasToolResult := false

			for _, b := range raw.Message.Content {
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
		if raw.Message != nil {
			var lastThinking string
			for _, b := range raw.Message.Content {
				switch b.Type {
				case "thinking":
					if b.Thinking != "" {
						lastThinking = b.Thinking
						events = append(events, ParsedEvent{
							EventType: "thinking",
							Timestamp: ts,
							CWD:       raw.CWD,
							GitBranch: raw.GitBranch,
							Data: types.TelemetryFrameData{
								Role:    stringPtr("assistant"),
								Content: stringPtr(b.Thinking),
								Model:   stringPtr(raw.Message.Model),
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
								Model:      stringPtr(raw.Message.Model),
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
								Model:      stringPtr(raw.Message.Model),
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
				total := u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens + reasoning
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

			var modelPtr *string
			if raw.Message.Model != "" {
				modelPtr = stringPtr(raw.Message.Model)
			}

			var thinkingExcerpt *string
			if lastThinking != "" {
				thinkingExcerpt = stringPtr(lastThinking)
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
					Content:    thinkingExcerpt,
				},
			})
		}
	}

	return events, nil
}
