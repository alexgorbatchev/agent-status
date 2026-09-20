// Package parser provides streaming log parsers for AI agent harnesses.
//
// Attribution:
// Derived from and inspired by Nick Nisi Sessions (https://github.com/nicknisi/sessions)
// and OpenUsage (https://github.com/janekbaraniewski/openusage).
package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

type rawPiContentBlock struct {
	Type       string                 `json:"type"` // "text" | "thinking" | "toolCall" | "toolResult"
	Text       string                 `json:"text,omitempty"`
	Thinking   string                 `json:"thinking,omitempty"`
	ToolName   string                 `json:"toolName,omitempty"`
	ToolCallID string                 `json:"toolCallId,omitempty"`
	Name       string                 `json:"name,omitempty"`
	ID         string                 `json:"id,omitempty"`
	Input      map[string]interface{} `json:"input,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
	Content    interface{}            `json:"content,omitempty"`
	IsError    *bool                  `json:"isError,omitempty"`
}

type rawPiUsage struct {
	Input      int64    `json:"input"`
	Output     int64    `json:"output"`
	CacheRead  int64    `json:"cacheRead"`
	CacheWrite int64    `json:"cacheWrite"`
	Reasoning  int64    `json:"reasoning"`
	Total      int64    `json:"total"`
	Cost       *float64 `json:"cost,omitempty"`
}

type rawPiMessage struct {
	Role       string              `json:"role"` // "user" | "assistant" | "toolResult" | "system"
	Model      string              `json:"model,omitempty"`
	Provider   string              `json:"provider,omitempty"`
	Content    interface{}         `json:"content,omitempty"`
	Usage      *rawPiUsage         `json:"usage,omitempty"`
	StopReason string              `json:"stopReason,omitempty"`
	ToolName   string              `json:"toolName,omitempty"`
	ToolCallID string              `json:"toolCallId,omitempty"`
	IsError    *bool               `json:"isError,omitempty"`
}

type rawPiLine struct {
	Type      string        `json:"type"` // "session" | "message" | "model_change" | "thinking_level_change" | "custom_message" | "label" | "session_info"
	ID        string        `json:"id"`
	ParentID  *string       `json:"parentId"`
	Timestamp interface{}   `json:"timestamp"`
	CWD       string        `json:"cwd,omitempty"`
	Message   *rawPiMessage `json:"message,omitempty"`
	Model     string        `json:"model,omitempty"`
	Provider  string        `json:"provider,omitempty"`
	Title     string        `json:"title,omitempty"`
	Name      string        `json:"name,omitempty"`
}

func parsePiContentBlocks(content interface{}) []rawPiContentBlock {
	if content == nil {
		return nil
	}

	switch v := content.(type) {
	case string:
		return []rawPiContentBlock{{Type: "text", Text: v}}
	case []interface{}:
		var blocks []rawPiContentBlock
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				var b rawPiContentBlock
				data, err := json.Marshal(m)
				if err == nil && json.Unmarshal(data, &b) == nil {
					blocks = append(blocks, b)
				}
			}
		}
		return blocks
	case map[string]interface{}:
		var b rawPiContentBlock
		data, err := json.Marshal(v)
		if err == nil && json.Unmarshal(data, &b) == nil {
			return []rawPiContentBlock{b}
		}
	}
	return nil
}

// ParsePiLine parses a single JSONL line from a Pi session file.
func ParsePiLine(line []byte, fallbackCWD string) ([]ParsedEvent, error) {
	if len(line) == 0 {
		return nil, nil
	}

	var raw rawPiLine
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling pi line: %w", err)
	}

	ts := parseTimestamp(raw.Timestamp)
	cwd := raw.CWD
	if cwd == "" {
		cwd = fallbackCWD
	}

	var events []ParsedEvent

	switch raw.Type {
	case "session":
		// Session header
		events = append(events, ParsedEvent{
			EventType: "run_started",
			Timestamp: ts,
			CWD:       cwd,
			Data: types.TelemetryFrameData{
				Title: stringPtr("Pi Session " + raw.ID),
			},
		})

	case "label", "session_info":
		titleText := raw.Title
		if titleText == "" {
			titleText = raw.Name
		}
		if titleText != "" {
			events = append(events, ParsedEvent{
				EventType: "title_change",
				Timestamp: ts,
				CWD:       cwd,
				Data: types.TelemetryFrameData{
					Title: stringPtr(titleText),
				},
			})
		}

	case "model_change":
		modelName := raw.Model
		if modelName != "" {
			events = append(events, ParsedEvent{
				EventType: "model_change",
				Timestamp: ts,
				CWD:       cwd,
				Data: types.TelemetryFrameData{
					Model: stringPtr(modelName),
				},
			})
		}

	case "message":
		if raw.Message == nil {
			return nil, nil
		}
		msg := raw.Message

		switch msg.Role {
		case "user":
			blocks := parsePiContentBlocks(msg.Content)
			var userTexts []string
			for _, b := range blocks {
				if b.Text != "" {
					userTexts = append(userTexts, b.Text)
				}
			}
			fullPrompt := strings.Join(userTexts, "\n")
			if fullPrompt == "" {
				fullPrompt = extractContentString(msg.Content)
			}

			events = append(events, ParsedEvent{
				EventType: "turn_start",
				Timestamp: ts,
				CWD:       cwd,
				Data: types.TelemetryFrameData{
					Role:       stringPtr("user"),
					Content:    stringPtr(fullPrompt),
					LastPrompt: stringPtr(fullPrompt),
				},
			})

		case "toolResult":
			output := extractContentString(msg.Content)
			isErr := false
			if msg.IsError != nil {
				isErr = *msg.IsError
			}

			var toolCallID *string
			if msg.ToolCallID != "" {
				toolCallID = stringPtr(msg.ToolCallID)
			}
			var toolName *string
			if msg.ToolName != "" {
				toolName = stringPtr(msg.ToolName)
			}

			events = append(events, ParsedEvent{
				EventType: "tool_result",
				Timestamp: ts,
				CWD:       cwd,
				Data: types.TelemetryFrameData{
					Role:       stringPtr("toolResult"),
					ToolName:   toolName,
					ToolCallID: toolCallID,
					ToolOutput: stringPtr(output),
					IsError:    boolPtr(isErr),
				},
			})

		case "assistant":
			blocks := parsePiContentBlocks(msg.Content)
			var lastThinking string

			for _, b := range blocks {
				switch b.Type {
				case "thinking":
					if b.Thinking != "" {
						lastThinking = b.Thinking
					} else if b.Text != "" {
						lastThinking = b.Text
					}
					if lastThinking != "" {
						events = append(events, ParsedEvent{
							EventType: "thinking",
							Timestamp: ts,
							CWD:       cwd,
							Data: types.TelemetryFrameData{
								Role:    stringPtr("assistant"),
								Content: stringPtr(lastThinking),
								Model:   stringPtr(msg.Model),
							},
						})
					}

				case "toolCall", "tool_use":
					toolName := b.ToolName
					if toolName == "" {
						toolName = b.Name
					}
					callID := b.ToolCallID
					if callID == "" {
						callID = b.ID
					}

					params := b.Input
					if params == nil {
						params = b.Parameters
					}
					if params == nil {
						params = b.Arguments
					}

					if toolName == "AskUserQuestion" || toolName == "ask_user" || toolName == "ask_question" {
						qText := ""
						if params != nil {
							if q, ok := params["question"].(string); ok {
								qText = q
							} else if p, ok := params["prompt"].(string); ok {
								qText = p
							}
						}
						events = append(events, ParsedEvent{
							EventType: "question_prompt",
							Timestamp: ts,
							CWD:       cwd,
							Data: types.TelemetryFrameData{
								Role:       stringPtr("assistant"),
								ToolName:   stringPtr(toolName),
								ToolCallID: stringPtr(callID),
								ToolInput:  params,
								Question:   stringPtr(qText),
								Model:      stringPtr(msg.Model),
							},
						})
					} else {
						events = append(events, ParsedEvent{
							EventType: "tool_call",
							Timestamp: ts,
							CWD:       cwd,
							Data: types.TelemetryFrameData{
								Role:       stringPtr("assistant"),
								ToolName:   stringPtr(toolName),
								ToolCallID: stringPtr(callID),
								ToolInput:  params,
								Model:      stringPtr(msg.Model),
							},
						})
					}
				}
			}

			// Usage & Turn End
			var usage *types.ITokenUsage
			var harnessCost *float64

			if msg.Usage != nil {
				u := msg.Usage
				total := u.Total
				if total == 0 {
					total = u.Input + u.Output + u.CacheRead + u.CacheWrite + u.Reasoning
				}
				usage = &types.ITokenUsage{
					Input:      u.Input,
					Output:     u.Output,
					CacheRead:  u.CacheRead,
					CacheWrite: u.CacheWrite,
					Reasoning:  u.Reasoning,
					Total:      total,
				}
				harnessCost = u.Cost
			}

			var stopReason *string
			if msg.StopReason != "" {
				stopReason = stringPtr(msg.StopReason)
			}

			var modelPtr *string
			if msg.Model != "" {
				modelPtr = stringPtr(msg.Model)
			}

			events = append(events, ParsedEvent{
				EventType: "turn_end",
				Timestamp: ts,
				CWD:       cwd,
				Data: types.TelemetryFrameData{
					Role:           stringPtr("assistant"),
					Model:          modelPtr,
					StopReason:     stopReason,
					Usage:          usage,
					RawHarnessCost: harnessCost,
				},
			})
		}
	}

	return events, nil
}
