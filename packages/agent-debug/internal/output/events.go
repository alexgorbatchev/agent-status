package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
	"github.com/olekukonko/tablewriter"
)

// EventJSONItem represents the structured JSON output for an event.
type EventJSONItem struct {
	ID           int64       `json:"id"`
	AgentID      string      `json:"agentId"`
	SessionID    string      `json:"sessionId"`
	Harness      string      `json:"harness,omitempty"`
	SubagentID   *string     `json:"subagentId"`
	EventType    string      `json:"eventType"`
	Timestamp    int64       `json:"timestamp"`
	TimestampUTC string      `json:"timestampUTC"`
	Summary      string      `json:"summary"`
	Data         interface{} `json:"data"`
}

func toJSONItem(rec db.EventRecord) EventJSONItem {
	tsUTC := time.UnixMilli(rec.Timestamp).UTC().Format("2006-01-02T15:04:05.000Z")
	var rawData interface{}
	if rec.DataJSON != "" {
		if err := json.Unmarshal([]byte(rec.DataJSON), &rawData); err != nil {
			rawData = rec.Data
		}
	} else {
		rawData = rec.Data
	}

	return EventJSONItem{
		ID:           rec.ID,
		AgentID:      rec.AgentID,
		SessionID:    rec.SessionID,
		Harness:      rec.Harness,
		SubagentID:   rec.SubagentID,
		EventType:    rec.EventType,
		Timestamp:    rec.Timestamp,
		TimestampUTC: tsUTC,
		Summary:      FormatEventSummary(rec),
		Data:         rawData,
	}
}

// FormatEventSummary derives a compact summary string for a telemetry event.
func FormatEventSummary(rec db.EventRecord) string {
	switch rec.EventType {
	case "run_started":
		var parts []string
		if rec.PID > 0 {
			parts = append(parts, fmt.Sprintf("pid=%d", rec.PID))
		}
		if rec.Harness != "" {
			parts = append(parts, fmt.Sprintf("harness=%s", rec.Harness))
		}
		if rec.Data.Title != nil && *rec.Data.Title != "" {
			parts = append(parts, fmt.Sprintf("title=%q", *rec.Data.Title))
		}
		if rec.Data.Model != nil && *rec.Data.Model != "" {
			parts = append(parts, fmt.Sprintf("model=%s", *rec.Data.Model))
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
		return "run started"

	case "run_exited":
		if rec.PID > 0 {
			return fmt.Sprintf("pid=%d", rec.PID)
		}
		return "run exited"

	case "turn_end":
		isIntermediate := IsIntermediateToolTurnEnd(rec)
		usage := rec.Data.Usage
		if usage == nil && rec.DataJSON != "" {
			var d types.TelemetryFrameData
			if err := json.Unmarshal([]byte(rec.DataJSON), &d); err == nil && d.Usage != nil {
				usage = d.Usage
			}
		}

		if usage != nil {
			tokens := usage.Total
			if tokens == 0 {
				tokens = usage.Input + usage.Output
			}
			if isIntermediate {
				return fmt.Sprintf("(api_response) tokens=%d", tokens)
			}
			return fmt.Sprintf("tokens=%d", tokens)
		}
		if isIntermediate {
			return "(api_response)"
		}
		return "turn ended"

	case "tool_result":
		var parts []string
		if rec.Data.ToolName != nil && *rec.Data.ToolName != "" {
			parts = append(parts, fmt.Sprintf("tool=%s", *rec.Data.ToolName))
		}
		status := "success"
		if (rec.Data.IsError != nil && *rec.Data.IsError) || (rec.Data.ExitCode != nil && *rec.Data.ExitCode != 0) {
			status = "error"
		}
		parts = append(parts, fmt.Sprintf("status=%s", status))
		if rec.Data.ExitCode != nil && *rec.Data.ExitCode != 0 {
			parts = append(parts, fmt.Sprintf("exit=%d", *rec.Data.ExitCode))
		}
		return strings.Join(parts, " ")

	case "tool_call":
		var parts []string
		if rec.Data.ToolName != nil && *rec.Data.ToolName != "" {
			parts = append(parts, fmt.Sprintf("tool=%s", *rec.Data.ToolName))
		}
		if rec.Data.FileModification != nil && rec.Data.FileModification.FilePath != "" {
			parts = append(parts, fmt.Sprintf("file=%s", rec.Data.FileModification.FilePath))
		} else if rec.Data.ToolInput != nil {
			if cmdVal, ok := rec.Data.ToolInput["command"].(string); ok && cmdVal != "" {
				clean := strings.ReplaceAll(strings.TrimSpace(cmdVal), "\n", " ")
				if len(clean) > 30 {
					clean = clean[:27] + "..."
				}
				parts = append(parts, fmt.Sprintf("cmd=%q", clean))
			} else if pathVal, ok := rec.Data.ToolInput["path"].(string); ok && pathVal != "" {
				parts = append(parts, fmt.Sprintf("path=%s", pathVal))
			} else if fileVal, ok := rec.Data.ToolInput["file_path"].(string); ok && fileVal != "" {
				parts = append(parts, fmt.Sprintf("file=%s", fileVal))
			} else if patternVal, ok := rec.Data.ToolInput["pattern"].(string); ok && patternVal != "" {
				parts = append(parts, fmt.Sprintf("pattern=%q", patternVal))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
		return "tool call"

	case "thinking":
		if rec.Data.Content != nil {
			return fmt.Sprintf("len=%d chars", len(*rec.Data.Content))
		}
		return "thinking"

	case "user_message":
		if rec.Data.LastPrompt != nil && *rec.Data.LastPrompt != "" {
			prompt := strings.ReplaceAll(strings.TrimSpace(*rec.Data.LastPrompt), "\n", " ")
			if len(prompt) > 40 {
				prompt = prompt[:37] + "..."
			}
			return fmt.Sprintf("%q", prompt)
		}
		if rec.Data.Content != nil && *rec.Data.Content != "" {
			content := strings.ReplaceAll(strings.TrimSpace(*rec.Data.Content), "\n", " ")
			if len(content) > 40 {
				content = content[:37] + "..."
			}
			return fmt.Sprintf("%q", content)
		}
		return "user message"

	case "assistant_message":
		if rec.Data.Content != nil && *rec.Data.Content != "" {
			content := strings.ReplaceAll(strings.TrimSpace(*rec.Data.Content), "\n", " ")
			if len(content) > 40 {
				content = content[:37] + "..."
			}
			return fmt.Sprintf("%q", content)
		}
		return "assistant message"

	case "error":
		if rec.Data.Content != nil && *rec.Data.Content != "" {
			return *rec.Data.Content
		}
		return "error"

	case "waiting_question", "question_prompt":
		if rec.Data.Question != nil && *rec.Data.Question != "" {
			return fmt.Sprintf("question=%q", *rec.Data.Question)
		}
		return "waiting question"

	default:
		return rec.EventType
	}
}

// IsIntermediateToolTurnEnd reports whether a turn_end event stopped for tool use.
func IsIntermediateToolTurnEnd(rec db.EventRecord) bool {
	if rec.EventType != "turn_end" {
		return false
	}
	var reason string
	if rec.Data.StopReason != nil {
		reason = *rec.Data.StopReason
	} else if rec.DataJSON != "" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(rec.DataJSON), &raw); err == nil {
			if r, ok := raw["stopReason"].(string); ok {
				reason = r
			} else if r, ok := raw["stop_reason"].(string); ok {
				reason = r
			}
		}
	}
	switch reason {
	case "tool_use", "toolUse", "tool_calls", "tool_call":
		return true
	default:
		return false
	}
}

// FormatEventLabel returns the canonical display label for a telemetry event according to the UI/CLI specification
// (e.g. "API RESPONSE" for intermediate tool turns, "USER PROMPT" for user messages, or the event type).
func FormatEventLabel(rec db.EventRecord) string {
	if IsIntermediateToolTurnEnd(rec) {
		return "API RESPONSE"
	}
	if (rec.EventType == "turn_start" || (rec.Data.Role != nil && *rec.Data.Role == "user")) && rec.EventType != "tool_result" {
		return "USER PROMPT"
	}
	return rec.EventType
}

// RenderEventsTable writes events in tabular format (human) or key-value format (agent).
func RenderEventsTable(w io.Writer, events []db.EventRecord, agentMode bool) error {
	if agentMode {
		for _, rec := range events {
			subagentStr := "-"
			if rec.SubagentID != nil && *rec.SubagentID != "" {
				subagentStr = *rec.SubagentID
			}
			summary := FormatEventSummary(rec)
			if _, err := fmt.Fprintf(w, "id:%d ts:%d type:%s subagent:%s summary:%q\n",
				rec.ID, rec.Timestamp, rec.EventType, subagentStr, summary); err != nil {
				return err
			}
		}
		return nil
	}

	if len(events) == 0 {
		_, _ = fmt.Fprintln(w, "No telemetry events found.")
		return nil
	}

	table := tablewriter.NewTable(w, tablewriter.WithHeader([]string{"ID", "Timestamp (UTC)", "Event Type", "Subagent", "Summary"}))
	for _, rec := range events {
		tsUTC := time.UnixMilli(rec.Timestamp).UTC().Format("2006-01-02 15:04:05.000")
		subagentStr := "-"
		if rec.SubagentID != nil && *rec.SubagentID != "" {
			subagentStr = *rec.SubagentID
		}
		summary := FormatEventSummary(rec)
		eventType := rec.EventType
		if IsIntermediateToolTurnEnd(rec) {
			eventType = "turn_end (API RESPONSE)"
		}
		_ = table.Append([]string{
			fmt.Sprintf("%d", rec.ID),
			tsUTC,
			eventType,
			subagentStr,
			summary,
		})
	}
	return table.Render()
}

// RenderEventDetail writes a detailed view of a single telemetry event.
func RenderEventDetail(w io.Writer, rec db.EventRecord, agentMode bool) error {
	subagentStr := "-"
	if rec.SubagentID != nil && *rec.SubagentID != "" {
		subagentStr = *rec.SubagentID
	}
	summary := FormatEventSummary(rec)

	if agentMode {
		dataStr := rec.DataJSON
		if dataStr == "" {
			b, _ := json.Marshal(rec.Data)
			dataStr = string(b)
		}
		_, err := fmt.Fprintf(w, "id:%d session_id:%s agent_id:%s type:%s ts:%d subagent:%s summary:%q data:%s\n",
			rec.ID, rec.SessionID, rec.AgentID, rec.EventType, rec.Timestamp, subagentStr, summary, dataStr)
		return err
	}

	tsUTC := time.UnixMilli(rec.Timestamp).UTC().Format("2006-01-02 15:04:05.000")

	var formattedData string
	if rec.DataJSON != "" {
		var v interface{}
		if err := json.Unmarshal([]byte(rec.DataJSON), &v); err == nil {
			if b, err := json.MarshalIndent(v, "  ", "  "); err == nil {
				formattedData = "  " + string(b)
			}
		}
	}
	if formattedData == "" {
		if b, err := json.MarshalIndent(rec.Data, "  ", "  "); err == nil {
			formattedData = "  " + string(b)
		} else {
			formattedData = "  {}"
		}
	}

	divider := strings.Repeat("-", 80)
	_, _ = fmt.Fprintln(w, divider)
	_, _ = fmt.Fprintf(w, "Event ID:        %d\n", rec.ID)
	_, _ = fmt.Fprintf(w, "Session ID:      %s\n", rec.SessionID)
	_, _ = fmt.Fprintf(w, "Agent ID:        %s\n", rec.AgentID)
	if rec.Harness != "" {
		_, _ = fmt.Fprintf(w, "Harness:         %s\n", rec.Harness)
	}
	if rec.PID > 0 {
		_, _ = fmt.Fprintf(w, "PID:             %d\n", rec.PID)
	}
	eventType := rec.EventType
	if IsIntermediateToolTurnEnd(rec) {
		eventType = "turn_end (API RESPONSE)"
	}
	_, _ = fmt.Fprintf(w, "Event Type:      %s\n", eventType)
	_, _ = fmt.Fprintf(w, "Timestamp:       %s UTC (%d)\n", tsUTC, rec.Timestamp)
	_, _ = fmt.Fprintf(w, "Subagent:        %s\n", subagentStr)
	_, _ = fmt.Fprintf(w, "Summary:         %s\n", summary)
	_, _ = fmt.Fprintln(w, divider)
	_, _ = fmt.Fprintln(w, "Payload Data:")
	_, _ = fmt.Fprintln(w, formattedData)
	_, _ = fmt.Fprintln(w, divider)
	return nil
}

// RenderEventsJSON writes events as a formatted JSON array.
func RenderEventsJSON(w io.Writer, events []db.EventRecord) error {
	items := make([]EventJSONItem, 0, len(events))
	for _, e := range events {
		items = append(items, toJSONItem(e))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

// RenderEventJSON writes a single event as a formatted JSON object.
func RenderEventJSON(w io.Writer, rec db.EventRecord) error {
	item := toJSONItem(rec)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(item)
}
