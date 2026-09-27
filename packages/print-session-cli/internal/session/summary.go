package session

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-parser"
)

// ToolUsageStat tracks usage count and errors for a specific tool.
type ToolUsageStat struct {
	Name       string `json:"name"`
	Calls      int    `json:"calls"`
	ErrorCalls int    `json:"errorCalls"`
}

// FileModificationStat records a file modification event summary.
type FileModificationStat struct {
	FilePath string `json:"filePath"`
	Action   string `json:"action"`
	Hunks    int    `json:"hunks"`
}

// ErrorRecord records an error encountered during the session.
type ErrorRecord struct {
	Timestamp int64  `json:"timestamp"`
	Source    string `json:"source"`
	Message   string `json:"message"`
}

// SessionSummary provides a high-level token-efficient overview of an agent session.
type SessionSummary struct {
	FilePath      string                 `json:"filePath"`
	Title         string                 `json:"title"`
	CWD           string                 `json:"cwd"`
	StartTime     int64                  `json:"startTime"`
	EndTime       int64                  `json:"endTime"`
	DurationMs    int64                  `json:"durationMs"`
	TotalEvents   int                    `json:"totalEvents"`
	TurnCount     int                    `json:"turnCount"`
	Models        []string               `json:"models"`
	TokensIn      int64                  `json:"tokensIn"`
	TokensOut     int64                  `json:"tokensOut"`
	TokensCache   int64                  `json:"tokensCache"`
	TokensTotal   int64                  `json:"tokensTotal"`
	CostUSD       float64                `json:"costUSD"`
	Tools         []ToolUsageStat        `json:"tools"`
	FilesModified []FileModificationStat `json:"filesModified"`
	Errors        []ErrorRecord          `json:"errors"`
	ExitCode      *int                   `json:"exitCode,omitempty"`
}

// ComputeSessionSummary computes high-level aggregate metrics from parsed events.
func ComputeSessionSummary(filePath string, events []parser.ParsedEvent) SessionSummary {
	s := SessionSummary{
		FilePath:    filePath,
		TotalEvents: len(events),
	}

	modelSet := make(map[string]struct{})
	toolsMap := make(map[string]*ToolUsageStat)
	filesMap := make(map[string]*FileModificationStat)

	var firstTS, lastTS int64

	for _, ev := range events {
		if ev.Timestamp > 0 {
			if firstTS == 0 || ev.Timestamp < firstTS {
				firstTS = ev.Timestamp
			}
			if ev.Timestamp > lastTS {
				lastTS = ev.Timestamp
			}
		}

		if ev.CWD != "" && s.CWD == "" {
			s.CWD = ev.CWD
		}

		if ev.Data.Title != nil && *ev.Data.Title != "" {
			s.Title = *ev.Data.Title
		}

		if ev.Data.Model != nil && *ev.Data.Model != "" {
			modelSet[*ev.Data.Model] = struct{}{}
		}

		switch ev.EventType {
		case "turn_start":
			s.TurnCount++

		case "tool_call":
			tName := "unknown"
			if ev.Data.ToolName != nil && *ev.Data.ToolName != "" {
				tName = *ev.Data.ToolName
			}
			st, ok := toolsMap[tName]
			if !ok {
				st = &ToolUsageStat{Name: tName}
				toolsMap[tName] = st
			}
			st.Calls++

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
			if ev.Data.IsError != nil && *ev.Data.IsError {
				callID := ""
				if ev.Data.ToolCallID != nil {
					callID = *ev.Data.ToolCallID
				}
				msg := ""
				if ev.Data.ToolOutput != nil {
					msg = strings.TrimSpace(*ev.Data.ToolOutput)
				}
				s.Errors = append(s.Errors, ErrorRecord{
					Timestamp: ev.Timestamp,
					Source:    fmt.Sprintf("tool_call (%s)", callID),
					Message:   msg,
				})
			}

		case "turn_end":
			if u := ev.Data.Usage; u != nil {
				s.TokensIn += u.Input
				s.TokensOut += u.Output
				s.TokensCache += u.CacheRead
				s.TokensTotal += u.Total
			}
			if ev.Data.RawHarnessCost != nil {
				s.CostUSD += *ev.Data.RawHarnessCost
			}

		case "run_exited":
			if ev.Data.ExitCode != nil {
				code := *ev.Data.ExitCode
				s.ExitCode = &code
			}

		case "error":
			msg := ""
			if ev.Data.Content != nil {
				msg = strings.TrimSpace(*ev.Data.Content)
			}
			s.Errors = append(s.Errors, ErrorRecord{
				Timestamp: ev.Timestamp,
				Source:    "harness",
				Message:   msg,
			})
		}
	}

	s.StartTime = firstTS
	s.EndTime = lastTS
	if lastTS >= firstTS && firstTS > 0 {
		s.DurationMs = lastTS - firstTS
	}

	for m := range modelSet {
		s.Models = append(s.Models, m)
	}
	sort.Strings(s.Models)

	for _, t := range toolsMap {
		s.Tools = append(s.Tools, *t)
	}
	sort.Slice(s.Tools, func(i, j int) bool {
		return s.Tools[i].Calls > s.Tools[j].Calls
	})

	for _, f := range filesMap {
		s.FilesModified = append(s.FilesModified, *f)
	}
	sort.Slice(s.FilesModified, func(i, j int) bool {
		return s.FilesModified[i].FilePath < s.FilesModified[j].FilePath
	})

	return s
}

// RenderSummary outputs the session summary to the writer in human, agent, or JSON format.
func RenderSummary(w io.Writer, s SessionSummary, agentMode bool, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}

	durStr := formatDuration(s.DurationMs)

	if agentMode {
		// Clean continuation summary for agents: focus strictly on actions, files, tools, and errors
		_, _ = fmt.Fprintf(w, "SESSION_SUMMARY file=%s events=%d turns=%d duration=%s\n", s.FilePath, s.TotalEvents, s.TurnCount, durStr)
		if s.Title != "" {
			_, _ = fmt.Fprintf(w, "TITLE: %s\n", s.Title)
		}
		if s.CWD != "" {
			_, _ = fmt.Fprintf(w, "CWD: %s\n", s.CWD)
		}

		if len(s.Tools) > 0 {
			var toolParts []string
			for _, t := range s.Tools {
				if t.ErrorCalls > 0 {
					toolParts = append(toolParts, fmt.Sprintf("%s(%d,err:%d)", t.Name, t.Calls, t.ErrorCalls))
				} else {
					toolParts = append(toolParts, fmt.Sprintf("%s(%d)", t.Name, t.Calls))
				}
			}
			_, _ = fmt.Fprintf(w, "TOOLS: %s\n", strings.Join(toolParts, " "))
		}

		if len(s.FilesModified) > 0 {
			_, _ = fmt.Fprintf(w, "FILES_MODIFIED (%d):\n", len(s.FilesModified))
			for _, fm := range s.FilesModified {
				_, _ = fmt.Fprintf(w, "* %s %s (hunks: %d)\n", fm.Action, fm.FilePath, fm.Hunks)
			}
		}

		if len(s.Errors) > 0 {
			_, _ = fmt.Fprintf(w, "ERRORS (%d):\n", len(s.Errors))
			for _, errRec := range s.Errors {
				_, _ = fmt.Fprintf(w, "* [%s] %s\n", errRec.Source, errRec.Message)
			}
		}

		if s.ExitCode != nil {
			_, _ = fmt.Fprintf(w, "EXIT_CODE: %d\n", *s.ExitCode)
		}
		return nil
	}

	// Human mode
	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	_, _ = fmt.Fprintf(w, "[SESSION SUMMARY] %s\n", s.FilePath)
	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	if s.Title != "" {
		_, _ = fmt.Fprintf(w, "Title:          %s\n", s.Title)
	}
	if s.CWD != "" {
		_, _ = fmt.Fprintf(w, "CWD:            %s\n", s.CWD)
	}
	_, _ = fmt.Fprintf(w, "Events / Turns: %d events across %d turns\n", s.TotalEvents, s.TurnCount)
	_, _ = fmt.Fprintf(w, "Duration:       %s\n", durStr)
	if len(s.Models) > 0 {
		_, _ = fmt.Fprintf(w, "Models:         %s\n", strings.Join(s.Models, ", "))
	}
	_, _ = fmt.Fprintf(w, "Tokens:         In: %d | Out: %d | Cache Read: %d | Total: %d\n", s.TokensIn, s.TokensOut, s.TokensCache, s.TokensTotal)
	_, _ = fmt.Fprintf(w, "Harness Cost:   $%.4f\n", s.CostUSD)

	if len(s.Tools) > 0 {
		_, _ = fmt.Fprintf(w, "\nTool Invocations:\n")
		for _, t := range s.Tools {
			errNote := ""
			if t.ErrorCalls > 0 {
				errNote = fmt.Sprintf(" (%d errors)", t.ErrorCalls)
			}
			_, _ = fmt.Fprintf(w, "  - %-16s %3d calls%s\n", t.Name+":", t.Calls, errNote)
		}
	}

	if len(s.FilesModified) > 0 {
		_, _ = fmt.Fprintf(w, "\nFiles Modified (%d):\n", len(s.FilesModified))
		for _, fm := range s.FilesModified {
			_, _ = fmt.Fprintf(w, "  - [%s] %s (%d hunks)\n", fm.Action, fm.FilePath, fm.Hunks)
		}
	}

	if len(s.Errors) > 0 {
		_, _ = fmt.Fprintf(w, "\nErrors Encountered (%d):\n", len(s.Errors))
		for _, errRec := range s.Errors {
			_, _ = fmt.Fprintf(w, "  - [%s] %s\n", errRec.Source, errRec.Message)
		}
	}

	if s.ExitCode != nil {
		_, _ = fmt.Fprintf(w, "\nExit Code:      %d\n", *s.ExitCode)
	}

	return nil
}

func formatDuration(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%02ds", mins, secs)
	}
	hrs := mins / 60
	remMins := mins % 60
	return fmt.Sprintf("%dh%02dm", hrs, remMins)
}
