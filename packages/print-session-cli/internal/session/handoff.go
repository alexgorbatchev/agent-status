package session

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alexgorbatchev/agent-parser"
)

// RecentToolActivity captures a recent tool call and its execution result.
type RecentToolActivity struct {
	ToolName   string `json:"toolName"`
	CallID     string `json:"callId"`
	Summary    string `json:"summary"`
	Status     string `json:"status"` // "OK" | "ERR"
	OutputLen  int    `json:"outputLen"`
	OutputText string `json:"outputText,omitempty"`
}

// HandoffContext contains all essential semantic information needed by an incoming agent
// to immediately understand and resume work started in another session without truncation.
type HandoffContext struct {
	SessionID             string                 `json:"sessionId"`
	Title                 string                 `json:"title"`
	FilePath              string                 `json:"filePath"`
	CWD                   string                 `json:"cwd"`
	GitBranch             string                 `json:"gitBranch"`
	TotalEvents           int                    `json:"totalEvents"`
	TurnCount             int                    `json:"turnCount"`
	DurationMs            int64                  `json:"durationMs"`
	InitialObjective      string                 `json:"initialObjective"`
	LatestUserPrompt      string                 `json:"latestUserPrompt"`
	FinalAssistantMessage string                 `json:"finalAssistantMessage"`
	FilesModified         []FileModificationStat `json:"filesModified"`
	RecentActivity        []RecentToolActivity   `json:"recentActivity"`
	RecentErrors          []ErrorRecord          `json:"recentErrors"`
	ExitCode              *int                   `json:"exitCode,omitempty"`
}

// ComputeHandoffContext builds an untruncated continuation context from a session transcript.
func statusTag(status string) string {
	if status == "ERR" {
		return "[ERROR]"
	}
	return "[OK]"
}

// ComputeHandoffContext builds an untruncated continuation context from a session transcript.
func ComputeHandoffContext(filePath string, events []parser.ParsedEvent, recentLimit int) HandoffContext {
	if recentLimit <= 0 {
		recentLimit = 5
	}

	h := HandoffContext{
		FilePath:    filePath,
		TotalEvents: len(events),
	}

	filesMap := make(map[string]*FileModificationStat)
	toolCalls := make(map[string]*RecentToolActivity)
	var orderedActivities []*RecentToolActivity

	var firstTS, lastTS int64
	var allErrors []ErrorRecord

	for _, ev := range events {
		if ev.Timestamp > 0 {
			if firstTS == 0 || ev.Timestamp < firstTS {
				firstTS = ev.Timestamp
			}
			if ev.Timestamp > lastTS {
				lastTS = ev.Timestamp
			}
		}

		if ev.CWD != "" && h.CWD == "" {
			h.CWD = ev.CWD
		}
		if ev.GitBranch != "" && h.GitBranch == "" {
			h.GitBranch = ev.GitBranch
		}
		if ev.Data.Title != nil && *ev.Data.Title != "" {
			h.Title = *ev.Data.Title
		}

		switch ev.EventType {
		case "run_started":
			if ev.Data.Title != nil && *ev.Data.Title != "" {
				h.Title = *ev.Data.Title
			}

		case "turn_start":
			h.TurnCount++
			rawPrompt := ""
			if ev.Data.Content != nil {
				rawPrompt = *ev.Data.Content
			} else if ev.Data.LastPrompt != nil {
				rawPrompt = *ev.Data.LastPrompt
			}
			cleaned := strings.TrimSpace(parser.CleanClaudeCodePrompt(rawPrompt))
			if cleaned != "" {
				if h.InitialObjective == "" {
					h.InitialObjective = cleaned
				}
				h.LatestUserPrompt = cleaned
			}

		case "tool_call":
			tName := "unknown"
			if ev.Data.ToolName != nil && *ev.Data.ToolName != "" {
				tName = *ev.Data.ToolName
			}
			callID := ""
			if ev.Data.ToolCallID != nil {
				callID = *ev.Data.ToolCallID
			}

			summary := ""
			if len(ev.Data.ToolInput) > 0 {
				if cmd, ok := ev.Data.ToolInput["command"].(string); ok {
					summary = cmd
				} else if fp, ok := ev.Data.ToolInput["file_path"].(string); ok {
					summary = fp
				} else if path, ok := ev.Data.ToolInput["path"].(string); ok {
					summary = path
				}
			}

			act := &RecentToolActivity{
				ToolName: tName,
				CallID:   callID,
				Summary:  summary,
				Status:   "OK",
			}
			if callID != "" {
				toolCalls[callID] = act
			}
			orderedActivities = append(orderedActivities, act)

			if fm := ev.Data.FileModification; fm != nil {
				key := fm.FilePath + ":" + fm.Action
				fStat, fOk := filesMap[key]
				if !fOk {
					fStat = &FileModificationStat{
						FilePath: fm.FilePath,
						Action:   fm.Action,
						Hunks:    len(fm.Hunks),
					}
					filesMap[key] = fStat
				} else {
					fStat.Hunks += len(fm.Hunks)
				}
			}

		case "tool_result":
			callID := ""
			if ev.Data.ToolCallID != nil {
				callID = *ev.Data.ToolCallID
			}
			isErr := ev.Data.IsError != nil && *ev.Data.IsError
			outputStr := ""
			outLen := 0
			if ev.Data.ToolOutput != nil {
				outputStr = *ev.Data.ToolOutput
				outLen = len(outputStr)
			}

			if callID != "" {
				if act, ok := toolCalls[callID]; ok {
					if isErr {
						act.Status = "ERR"
					}
					act.OutputLen = outLen
					act.OutputText = outputStr
				}
			}

			if isErr {
				allErrors = append(allErrors, ErrorRecord{
					Timestamp: ev.Timestamp,
					Source:    fmt.Sprintf("tool (%s)", callID),
					Message:   outputStr,
				})
			}

		case "turn_end":
			if ev.Data.Content != nil && *ev.Data.Content != "" {
				text := strings.TrimSpace(*ev.Data.Content)
				if text != "" {
					h.FinalAssistantMessage = text
				}
			}

		case "run_exited":
			if ev.Data.ExitCode != nil {
				code := *ev.Data.ExitCode
				h.ExitCode = &code
			}

		case "error":
			msg := ""
			if ev.Data.Content != nil {
				msg = *ev.Data.Content
			}
			allErrors = append(allErrors, ErrorRecord{
				Timestamp: ev.Timestamp,
				Source:    "harness",
				Message:   msg,
			})
		}
	}

	if lastTS >= firstTS && firstTS > 0 {
		h.DurationMs = lastTS - firstTS
	}

	for _, f := range filesMap {
		h.FilesModified = append(h.FilesModified, *f)
	}
	sort.Slice(h.FilesModified, func(i, j int) bool {
		return h.FilesModified[i].FilePath < h.FilesModified[j].FilePath
	})

	// Keep only the most recent tool activities
	if len(orderedActivities) > recentLimit {
		h.RecentActivity = make([]RecentToolActivity, recentLimit)
		start := len(orderedActivities) - recentLimit
		for i := 0; i < recentLimit; i++ {
			h.RecentActivity[i] = *orderedActivities[start+i]
		}
	} else {
		h.RecentActivity = make([]RecentToolActivity, len(orderedActivities))
		for i, act := range orderedActivities {
			h.RecentActivity[i] = *act
		}
	}

	// Keep most recent errors (up to 5)
	if len(allErrors) > 5 {
		h.RecentErrors = allErrors[len(allErrors)-5:]
	} else {
		h.RecentErrors = allErrors
	}

	return h
}

// RenderHandoff outputs the continuation context in human, agent, or JSON format.
// Content is NOT trimmed so the continuing agent receives full context.
func RenderHandoff(w io.Writer, h HandoffContext, agentMode bool, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(h)
	}

	durStr := formatDuration(h.DurationMs)

	if agentMode {
		exitStr := "none"
		if h.ExitCode != nil {
			exitStr = fmt.Sprintf("%d", *h.ExitCode)
		}
		_, _ = fmt.Fprintf(w, "CONTINUATION_CONTEXT\n")
		_, _ = fmt.Fprintf(w, "FILE: %s\n", h.FilePath)
		if h.Title != "" {
			_, _ = fmt.Fprintf(w, "TITLE: %s\n", h.Title)
		}
		if h.GitBranch != "" {
			_, _ = fmt.Fprintf(w, "BRANCH: %s\n", h.GitBranch)
		}
		if h.CWD != "" {
			_, _ = fmt.Fprintf(w, "CWD: %s\n", h.CWD)
		}
		_, _ = fmt.Fprintf(w, "DURATION: %s (turns: %d, events: %d, exit: %s)\n", durStr, h.TurnCount, h.TotalEvents, exitStr)

		if h.InitialObjective != "" {
			_, _ = fmt.Fprintf(w, "\nINITIAL_OBJECTIVE:\n%s\n", h.InitialObjective)
		}
		if h.LatestUserPrompt != "" && h.LatestUserPrompt != h.InitialObjective {
			_, _ = fmt.Fprintf(w, "\nLATEST_USER_PROMPT:\n%s\n", h.LatestUserPrompt)
		}

		if h.FinalAssistantMessage != "" {
			_, _ = fmt.Fprintf(w, "\nFINAL_ASSISTANT_NOTE:\n%s\n", h.FinalAssistantMessage)
		}

		if len(h.FilesModified) > 0 {
			_, _ = fmt.Fprintf(w, "\nFILES_MODIFIED (%d):\n", len(h.FilesModified))
			for _, fm := range h.FilesModified {
				_, _ = fmt.Fprintf(w, "* %s %s (hunks: %d)\n", fm.Action, fm.FilePath, fm.Hunks)
			}
		}

		if len(h.RecentActivity) > 0 {
			_, _ = fmt.Fprintf(w, "\nRECENT_ACTIVITY (last %d):\n", len(h.RecentActivity))
			for _, act := range h.RecentActivity {
				sum := act.Summary
				if sum != "" {
					sum = " " + sum
				}
				_, _ = fmt.Fprintf(w, "* %s%s -> %s (%d bytes)\n", act.ToolName, sum, act.Status, act.OutputLen)
				if act.OutputText != "" {
					wrapped := WrapTextWithIndent(act.OutputText, "  ", 0)
					_, _ = fmt.Fprintf(w, "%s\n", wrapped)
				}
			}
		}

		if len(h.RecentErrors) > 0 {
			_, _ = fmt.Fprintf(w, "\nRECENT_ERRORS (%d):\n", len(h.RecentErrors))
			for _, e := range h.RecentErrors {
				_, _ = fmt.Fprintf(w, "* [%s] %s\n", e.Source, e.Message)
			}
		} else {
			_, _ = fmt.Fprintf(w, "\nRECENT_ERRORS: none\n")
		}

		return nil
	}

	// Human mode
	termWidth := GetTerminalWidth()
	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	_, _ = fmt.Fprintf(w, "[SESSION HANDOFF & CONTINUATION CONTEXT] %s\n", h.FilePath)
	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	if h.Title != "" {
		_, _ = fmt.Fprintf(w, "Title:        %s\n", h.Title)
	}
	if h.CWD != "" {
		_, _ = fmt.Fprintf(w, "Directory:    %s\n", h.CWD)
	}
	if h.GitBranch != "" {
		_, _ = fmt.Fprintf(w, "Git Branch:   %s\n", h.GitBranch)
	}
	_, _ = fmt.Fprintf(w, "Turns:        %d turns, %d total events (%s duration)\n", h.TurnCount, h.TotalEvents, durStr)
	if h.ExitCode != nil {
		_, _ = fmt.Fprintf(w, "Exit Status:  code %d\n", *h.ExitCode)
	}

	if h.InitialObjective != "" {
		wrapped := WrapTextWithIndent(h.InitialObjective, "    ", termWidth)
		_, _ = fmt.Fprintf(w, "\n[INITIAL USER OBJECTIVE]\n%s\n", wrapped)
	}

	if h.LatestUserPrompt != "" && h.LatestUserPrompt != h.InitialObjective {
		wrapped := WrapTextWithIndent(h.LatestUserPrompt, "    ", termWidth)
		_, _ = fmt.Fprintf(w, "\n[LATEST USER PROMPT]\n%s\n", wrapped)
	}

	if h.FinalAssistantMessage != "" {
		wrapped := WrapTextWithIndent(h.FinalAssistantMessage, "    ", termWidth)
		_, _ = fmt.Fprintf(w, "\n[FINAL ASSISTANT PROGRESS / STATUS]\n%s\n", wrapped)
	}

	if len(h.FilesModified) > 0 {
		_, _ = fmt.Fprintf(w, "\n[FILES TOUCHED IN SESSION] (%d files)\n", len(h.FilesModified))
		for _, fm := range h.FilesModified {
			_, _ = fmt.Fprintf(w, "  - [%s] %s (hunks: %d)\n", fm.Action, fm.FilePath, fm.Hunks)
		}
	}

	if len(h.RecentActivity) > 0 {
		_, _ = fmt.Fprintf(w, "\n[RECENT ACTIONS BEFORE HANDOFF]\n")
		for _, act := range h.RecentActivity {
			_, _ = fmt.Fprintf(w, "  [TOOL: %s]\n", act.ToolName)
			if act.Summary != "" {
				summaryPrefix := ""
				if act.ToolName == "Bash" {
					summaryPrefix = "$ "
				}
				wrappedDetail := WrapTextWithIndent(summaryPrefix+act.Summary, "      ", termWidth)
				_, _ = fmt.Fprintf(w, "%s\n", wrappedDetail)
			}
			tag := "\n      [RESULT]"
			if act.Status == "ERR" {
				tag = "\n      [TOOL ERROR]"
			}
			if act.OutputText != "" {
				wrapped := WrapTextWithIndent(act.OutputText, "          ", termWidth)
				_, _ = fmt.Fprintf(w, "%s (%d bytes)\n%s\n", tag, act.OutputLen, wrapped)
			} else {
				_, _ = fmt.Fprintf(w, "%s (status: %s)\n", tag, statusTag(act.Status))
			}
		}
	}

	if len(h.RecentErrors) > 0 {
		_, _ = fmt.Fprintf(w, "\n[RECENT UNRESOLVED ERRORS] (%d)\n", len(h.RecentErrors))
		for _, e := range h.RecentErrors {
			wrapped := WrapTextWithIndent(e.Message, "      ", termWidth)
			_, _ = fmt.Fprintf(w, "  - [%s]\n%s\n", e.Source, wrapped)
		}
	} else {
		_, _ = fmt.Fprintf(w, "\n[RECENT ERRORS]\n  None (all recent commands succeeded)\n")
	}

	return nil
}
