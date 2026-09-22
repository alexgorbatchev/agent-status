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
	AgentID           string      `json:"agentId,omitempty"`
	Timestamp         interface{} `json:"timestamp"`
	CWD               string      `json:"cwd"`
	GitBranch         string      `json:"gitBranch"`
	IsSidechain       bool        `json:"isSidechain"`
	Title             string      `json:"title"`
	CustomTitle       string      `json:"customTitle,omitempty"`
	AITitle           string      `json:"aiTitle"`
	AITitleSnake      string      `json:"ai_title,omitempty"`
	AgentName         string      `json:"agentName,omitempty"`
	Name              string      `json:"name,omitempty"`
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

// CleanClaudeCodePrompt cleans synthetic XML blocks and caveat wrappers from Claude Code prompt text.
func CleanClaudeCodePrompt(promptText string) string {
	trimmed := strings.TrimSpace(promptText)
	syntheticTags := []string{
		"local-command-caveat",
		"command-name",
		"command-message",
		"command-args",
		"local-command-stdout",
		"task-notification",
		"system-reminder",
		"fork-boilerplate",
		"environment_context",
		"session_context",
	}

	for {
		changed := false
		for _, tag := range syntheticTags {
			openTag := "<" + tag + ">"
			closeTag := "</" + tag + ">"
			if strings.HasPrefix(trimmed, openTag) {
				if idx := strings.Index(trimmed, closeTag); idx != -1 {
					trimmed = strings.TrimSpace(trimmed[idx+len(closeTag):])
					changed = true
				} else {
					return ""
				}
			}
		}
		if !changed {
			break
		}
	}

	return strings.TrimSpace(trimmed)
}

// ExtractClaudeCodeTitleFromPrompt extracts a session title from prompt text, ignoring synthetic context blocks.
func ExtractClaudeCodeTitleFromPrompt(promptText string) (string, bool) {
	cleanPrompt := CleanClaudeCodePrompt(promptText)
	if cleanPrompt == "" {
		return "", false
	}
	firstLine := strings.Split(cleanPrompt, "\n")[0]
	firstLine = strings.TrimSpace(firstLine)
	runes := []rune(firstLine)
	if len(runes) > 100 {
		firstLine = string(runes[:100])
	}
	return firstLine, firstLine != ""
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
	isSub := raw.IsSidechain || raw.AgentID != ""
	if raw.AITitle != "" {
		titlePtr = stringPtr(raw.AITitle)
	} else if raw.AITitleSnake != "" {
		titlePtr = stringPtr(raw.AITitleSnake)
	} else if raw.CustomTitle != "" {
		titlePtr = stringPtr(raw.CustomTitle)
	} else if raw.Title != "" {
		titlePtr = stringPtr(raw.Title)
	} else if raw.AgentName != "" {
		titlePtr = stringPtr(raw.AgentName)
	} else if raw.Name != "" {
		titlePtr = stringPtr(raw.Name)
	} else if !isSub && raw.LastPrompt != "" {
		if title, ok := ExtractClaudeCodeTitleFromPrompt(raw.LastPrompt); ok {
			titlePtr = stringPtr(title)
		}
	} else if isSub && raw.AgentID != "" {
		titlePtr = stringPtr("Claude Code Subagent: " + raw.AgentID)
	} else if !isSub && raw.SessionID != "" {
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
	mu             sync.Mutex
	hasStarted     bool
	seenSessions   map[string]bool
	explicitTitles map[string]bool
	sessionTitles  map[string]string
	lastTimestamp  int64
}

// NewClaudeCodeParser creates a new stateful parser stream for Claude Code transcripts.
func NewClaudeCodeParser() *ClaudeCodeParser {
	return &ClaudeCodeParser{
		seenSessions:   make(map[string]bool),
		explicitTitles: make(map[string]bool),
		sessionTitles:  make(map[string]string),
	}
}

// Reset resets the parser stream state.
func (p *ClaudeCodeParser) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hasStarted = false
	p.seenSessions = make(map[string]bool)
	p.explicitTitles = make(map[string]bool)
	p.sessionTitles = make(map[string]string)
	p.lastTimestamp = 0
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

	p.mu.Lock()
	var ts int64
	if raw.Timestamp == nil && p.lastTimestamp > 0 {
		ts = p.lastTimestamp
	} else {
		ts = parseTimestamp(raw.Timestamp)
		if raw.Timestamp != nil && ts > 0 {
			p.lastTimestamp = ts
		}
	}

	events := parseClaudeCodeLineEvents(raw, ts, p.explicitTitles, p.sessionTitles)
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
	defaultClaudeMu             sync.Mutex
	defaultClaudeSessions       = make(map[string]bool)
	defaultClaudeExplicitTitles = make(map[string]bool)
	defaultClaudeTitles         = make(map[string]string)
	defaultClaudeLastTS         int64
)

// ResetClaudeCodeSessions clears tracked sessions for the package-level ParseClaudeCodeLine.
func ResetClaudeCodeSessions() {
	defaultClaudeMu.Lock()
	defer defaultClaudeMu.Unlock()
	defaultClaudeSessions = make(map[string]bool)
	defaultClaudeExplicitTitles = make(map[string]bool)
	defaultClaudeTitles = make(map[string]string)
	defaultClaudeLastTS = 0
}

// ResetClaudeCodeSession clears a specific tracked session ID.
func ResetClaudeCodeSession(sessionID string) {
	defaultClaudeMu.Lock()
	defer defaultClaudeMu.Unlock()
	delete(defaultClaudeSessions, sessionID)
	delete(defaultClaudeExplicitTitles, sessionID)
	delete(defaultClaudeTitles, sessionID)
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

	defaultClaudeMu.Lock()
	var ts int64
	if raw.Timestamp == nil && defaultClaudeLastTS > 0 {
		ts = defaultClaudeLastTS
	} else {
		ts = parseTimestamp(raw.Timestamp)
		if raw.Timestamp != nil && ts > 0 {
			defaultClaudeLastTS = ts
		}
	}

	events := parseClaudeCodeLineEvents(raw, ts, defaultClaudeExplicitTitles, defaultClaudeTitles)
	emitRunStarted := false
	if raw.SessionID != "" {
		if !defaultClaudeSessions[raw.SessionID] {
			defaultClaudeSessions[raw.SessionID] = true
			emitRunStarted = true
		}
	}
	defaultClaudeMu.Unlock()

	if emitRunStarted {
		runStarted := makeClaudeCodeRunStartedEvent(raw, ts)
		events = append([]ParsedEvent{runStarted}, events...)
	}

	return events, nil
}

func parseClaudeCodeLineEvents(
	raw rawJSONLine,
	ts int64,
	explicitTitles map[string]bool,
	sessionTitles map[string]string,
) []ParsedEvent {
	var events []ParsedEvent
	isSubagent := raw.IsSidechain || raw.AgentID != ""
	sessKey := raw.SessionID
	if sessKey == "" {
		sessKey = "__default__"
	}

	switch raw.Type {
	case "ai-title", "title":
		titleVal := raw.AITitle
		if titleVal == "" {
			titleVal = raw.AITitleSnake
		}
		if titleVal == "" {
			titleVal = raw.Title
		}
		if titleVal == "" && raw.Content != nil {
			titleVal = extractContentString(raw.Content)
		}
		if titleVal != "" {
			if !isSubagent && explicitTitles != nil {
				explicitTitles[sessKey] = true
				if sessionTitles != nil {
					sessionTitles[sessKey] = titleVal
				}
			}
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

	case "custom-title":
		titleVal := raw.CustomTitle
		if titleVal == "" {
			titleVal = raw.Title
		}
		if titleVal == "" && raw.Content != nil {
			titleVal = extractContentString(raw.Content)
		}
		if titleVal != "" {
			if !isSubagent && explicitTitles != nil {
				explicitTitles[sessKey] = true
				if sessionTitles != nil {
					sessionTitles[sessKey] = titleVal
				}
			}
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

	case "agent-name":
		nameVal := raw.AgentName
		if nameVal == "" {
			nameVal = raw.Name
		}
		if nameVal != "" {
			events = append(events, ParsedEvent{
				EventType: "title_change",
				Timestamp: ts,
				CWD:       raw.CWD,
				GitBranch: raw.GitBranch,
				Data: types.TelemetryFrameData{
					Title: stringPtr(nameVal),
				},
			})
		}

	case "last-prompt":
		promptVal := raw.LastPrompt
		if promptVal == "" && raw.Content != nil {
			promptVal = extractContentString(raw.Content)
		}
		if promptVal != "" {
			var titlePtr *string
			shouldSetTitle := !isSubagent && (explicitTitles == nil || (!explicitTitles[sessKey] && sessionTitles[sessKey] == ""))
			if shouldSetTitle {
				if title, ok := ExtractClaudeCodeTitleFromPrompt(promptVal); ok {
					if sessionTitles != nil {
						sessionTitles[sessKey] = title
					}
					titlePtr = stringPtr(title)
				}
			}
			events = append(events, ParsedEvent{
				EventType: "turn_start",
				Timestamp: ts,
				CWD:       raw.CWD,
				GitBranch: raw.GitBranch,
				Data: types.TelemetryFrameData{
					LastPrompt: stringPtr(promptVal),
					Title:      titlePtr,
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
				var titlePtr *string
				shouldSetTitle := !isSubagent && (explicitTitles == nil || (!explicitTitles[sessKey] && sessionTitles[sessKey] == ""))
				if shouldSetTitle {
					if title, ok := ExtractClaudeCodeTitleFromPrompt(fullPrompt); ok {
						if sessionTitles != nil {
							sessionTitles[sessKey] = title
						}
						titlePtr = stringPtr(title)
					}
				}
				events = append(events, ParsedEvent{
					EventType: "turn_start",
					Timestamp: ts,
					CWD:       raw.CWD,
					GitBranch: raw.GitBranch,
					Data: types.TelemetryFrameData{
						Role:       stringPtr("user"),
						Content:    stringPtr(fullPrompt),
						LastPrompt: stringPtr(fullPrompt),
						Title:      titlePtr,
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
						fileMod := NormalizeFileModification(toolName, b.Input)
						events = append(events, ParsedEvent{
							EventType: "question_prompt",
							Timestamp: ts,
							CWD:       raw.CWD,
							GitBranch: raw.GitBranch,
							Data: types.TelemetryFrameData{
								Role:             stringPtr("assistant"),
								ToolName:         stringPtr(toolName),
								ToolCallID:       callID,
								ToolInput:        b.Input,
								FileModification: fileMod,
								Question:         stringPtr(qText),
								Model:            modelPtr,
							},
						})
					} else {
						fileMod := NormalizeFileModification(toolName, b.Input)
						events = append(events, ParsedEvent{
							EventType: "tool_call",
							Timestamp: ts,
							CWD:       raw.CWD,
							GitBranch: raw.GitBranch,
							Data: types.TelemetryFrameData{
								Role:             stringPtr("assistant"),
								ToolName:         stringPtr(toolName),
								ToolCallID:       callID,
								ToolInput:        b.Input,
								FileModification: fileMod,
								Model:            modelPtr,
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
