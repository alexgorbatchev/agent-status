package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
	"golang.org/x/term"
)

// PrintOptions configures filtering and output format of PrintClaudeSession.
type PrintOptions struct {
	JSON        bool
	AgentMode   bool
	SummaryOnly bool
	Handoff     bool
	LastTurn    bool
	ErrorsOnly  bool
	FilesOnly   bool
	ToolsOnly   bool
	PromptsOnly bool
	Tail        int
	Limit       int
	All         bool
}

// PrintClaudeSession reads and parses a Claude Code transcript file using agent-parser,
// rendering output like a harness display without trimming conversational content or injecting token telemetry.
func PrintClaudeSession(w io.Writer, filePath string, opts PrintOptions) error {
	p := parser.NewClaudeCodeParser()
	return printSessionWithParser(w, filePath, opts, func(line []byte) ([]parser.ParsedEvent, error) {
		return p.ParseLine(line)
	})
}

// PrintPiSession reads and parses a Pi session transcript file using agent-parser,
// rendering output like a harness display.
func PrintPiSession(w io.Writer, filePath string, opts PrintOptions) error {
	return printSessionWithParser(w, filePath, opts, func(line []byte) ([]parser.ParsedEvent, error) {
		return parser.ParsePiLine(line, "")
	})
}

func printSessionWithParser(w io.Writer, filePath string, opts PrintOptions, parseFn func([]byte) ([]parser.ParsedEvent, error)) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening transcript %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var allEvents []parser.ParsedEvent

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		events, pErr := parseFn(line)
		if pErr != nil {
			continue
		}

		allEvents = append(allEvents, events...)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading transcript lines: %w", err)
	}

	return processEvents(w, filePath, allEvents, opts)
}

func processEvents(w io.Writer, filePath string, allEvents []parser.ParsedEvent, opts PrintOptions) error {
	// 1. Handoff / Continuation mode
	if opts.Handoff {
		handoff := ComputeHandoffContext(filePath, allEvents, 5)
		return RenderHandoff(w, handoff, opts.AgentMode, opts.JSON)
	}

	// 2. Summary mode
	if opts.SummaryOnly {
		summary := ComputeSessionSummary(filePath, allEvents)
		return RenderSummary(w, summary, opts.AgentMode, opts.JSON)
	}

	// 3. Last turn filter
	eventsToProcess := allEvents
	if opts.LastTurn {
		lastTurnIdx := -1
		for i := len(allEvents) - 1; i >= 0; i-- {
			if allEvents[i].EventType == "turn_start" {
				lastTurnIdx = i
				break
			}
		}
		if lastTurnIdx >= 0 {
			eventsToProcess = allEvents[lastTurnIdx:]
		}
	}

	// 4. Apply filtering
	filtered := filterEvents(eventsToProcess, opts)

	// 5. Apply pagination / slicing
	effectiveEvents, _ := paginateEvents(filtered, opts)

	if opts.JSON {
		return renderJSON(w, effectiveEvents)
	}

	if opts.AgentMode {
		return renderAgentMode(w, filePath, effectiveEvents)
	}

	return renderHumanMode(w, filePath, effectiveEvents)
}

func filterEvents(events []parser.ParsedEvent, opts PrintOptions) []parser.ParsedEvent {
	if !opts.ErrorsOnly && !opts.FilesOnly && !opts.ToolsOnly && !opts.PromptsOnly {
		return events
	}

	var filtered []parser.ParsedEvent
	for _, ev := range events {
		if opts.ErrorsOnly {
			if ev.EventType == "error" || (ev.Data.IsError != nil && *ev.Data.IsError) {
				filtered = append(filtered, ev)
			}
			continue
		}
		if opts.FilesOnly {
			if ev.Data.FileModification != nil || ev.EventType == "file_modification" {
				filtered = append(filtered, ev)
			}
			continue
		}
		if opts.ToolsOnly {
			if ev.EventType == "tool_call" || ev.EventType == "tool_result" {
				filtered = append(filtered, ev)
			}
			continue
		}
		if opts.PromptsOnly {
			if ev.EventType == "turn_start" || (ev.EventType == "turn_end" && ev.Data.Content != nil && *ev.Data.Content != "") {
				filtered = append(filtered, ev)
			}
			continue
		}
	}
	return filtered
}

func paginateEvents(events []parser.ParsedEvent, opts PrintOptions) ([]parser.ParsedEvent, string) {
	total := len(events)
	if total == 0 {
		return events, ""
	}

	if opts.Limit > 0 && opts.Limit < total {
		return events[:opts.Limit], ""
	}

	if opts.Tail > 0 && opts.Tail < total {
		return events[total-opts.Tail:], ""
	}

	return events, ""
}

func renderJSON(w io.Writer, events []parser.ParsedEvent) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

// renderAgentMode renders conversational transcript close to what a harness displays:
// untruncated user prompts, untruncated assistant responses, tool invocations, and diffs.
// All telemetry noise (token counts, model IDs, micro timestamps) is completely omitted.
func renderAgentMode(w io.Writer, filePath string, events []parser.ParsedEvent) error {
	title, branch, cwd := extractSessionMeta(events)

	_, _ = fmt.Fprintf(w, "SESSION: %s\n", filePath)
	if title != "" {
		_, _ = fmt.Fprintf(w, "TITLE: %s\n", title)
	}
	if branch != "" {
		_, _ = fmt.Fprintf(w, "BRANCH: %s\n", branch)
	}
	if cwd != "" {
		_, _ = fmt.Fprintf(w, "CWD: %s\n", cwd)
	}

	for _, ev := range events {
		switch ev.EventType {
		case "turn_start":
			prompt := extractPromptText(ev)
			if prompt != "" {
				_, _ = fmt.Fprintf(w, "\nUSER:\n%s\n", prompt)
			}

		case "turn_end":
			if ev.Data.Content != nil && *ev.Data.Content != "" {
				text := strings.TrimSpace(*ev.Data.Content)
				if text != "" {
					_, _ = fmt.Fprintf(w, "\nASSISTANT:\n%s\n", text)
				}
			}

		case "tool_call":
			toolName := "unknown"
			if ev.Data.ToolName != nil && *ev.Data.ToolName != "" {
				toolName = *ev.Data.ToolName
			}
			_, _ = fmt.Fprintf(w, "\nTOOL %s:\n", toolName)
			if toolName == "Bash" && ev.Data.ToolInput != nil {
				if cmd, ok := ev.Data.ToolInput["command"].(string); ok {
					_, _ = fmt.Fprintf(w, "$ %s\n", cmd)
				}
			} else if fm := ev.Data.FileModification; fm != nil {
				_, _ = fmt.Fprintf(w, "Action: %s %s\n", fm.Action, fm.FilePath)
				if len(fm.Hunks) > 0 {
					diffText := formatDiffHunks(fm.Hunks)
					if diffText != "" {
						_, _ = fmt.Fprintf(w, "%s\n", diffText)
					}
				}
			} else if p, ok := ev.Data.ToolInput["path"].(string); ok && p != "" {
				_, _ = fmt.Fprintf(w, "Path: %s\n", p)
			} else if len(ev.Data.ToolInput) > 0 {
				inBytes, _ := json.MarshalIndent(ev.Data.ToolInput, "", "  ")
				_, _ = fmt.Fprintf(w, "%s\n", string(inBytes))
			}

		case "tool_result":
			isErr := ev.Data.IsError != nil && *ev.Data.IsError
			outputStr := ""
			if ev.Data.ToolOutput != nil {
				outputStr = strings.TrimSpace(*ev.Data.ToolOutput)
			}
			status := "OK"
			if isErr {
				status = "ERROR"
			}
			if outputStr != "" {
				_, _ = fmt.Fprintf(w, "RESULT (%s):\n%s\n", status, outputStr)
			} else {
				_, _ = fmt.Fprintf(w, "RESULT (%s)\n", status)
			}

		case "file_modification":
			if fm := ev.Data.FileModification; fm != nil {
				_, _ = fmt.Fprintf(w, "\nFILE MOD: %s %s (hunks: %d)\n", fm.Action, fm.FilePath, len(fm.Hunks))
				if len(fm.Hunks) > 0 {
					diffText := formatDiffHunks(fm.Hunks)
					if diffText != "" {
						_, _ = fmt.Fprintf(w, "%s\n", diffText)
					}
				}
			}

		case "error":
			msg := ""
			if ev.Data.Content != nil {
				msg = *ev.Data.Content
			}
			_, _ = fmt.Fprintf(w, "\nERROR:\n%s\n", msg)

		case "run_exited":
			code := 0
			if ev.Data.ExitCode != nil {
				code = *ev.Data.ExitCode
			}
			_, _ = fmt.Fprintf(w, "\nEXIT: code %d\n", code)
		}
	}

	return nil
}

func renderHumanMode(w io.Writer, filePath string, events []parser.ParsedEvent) error {
	title, branch, cwd := extractSessionMeta(events)
	termWidth := GetTerminalWidth()

	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	_, _ = fmt.Fprintf(w, "[SESSION LOG] %s\n", filePath)
	if title != "" {
		_, _ = fmt.Fprintf(w, "Title:  %s\n", title)
	}
	if branch != "" {
		_, _ = fmt.Fprintf(w, "Branch: %s\n", branch)
	}
	if cwd != "" {
		_, _ = fmt.Fprintf(w, "CWD:    %s\n", cwd)
	}
	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n\n")

	for _, ev := range events {
		switch ev.EventType {
		case "turn_start":
			prompt := extractPromptText(ev)
			if prompt != "" {
				wrapped := WrapTextWithIndent(prompt, "    ", termWidth)
				_, _ = fmt.Fprintf(w, "[USER]\n%s\n\n", wrapped)
			}

		case "turn_end":
			if ev.Data.Content != nil && *ev.Data.Content != "" {
				text := strings.TrimSpace(*ev.Data.Content)
				if text != "" {
					wrapped := WrapTextWithIndent(text, "    ", termWidth)
					_, _ = fmt.Fprintf(w, "[ASSISTANT]\n%s\n\n", wrapped)
				}
			}

		case "tool_call":
			toolName := "unknown"
			if ev.Data.ToolName != nil && *ev.Data.ToolName != "" {
				toolName = *ev.Data.ToolName
			}
			_, _ = fmt.Fprintf(w, "[TOOL: %s]\n", toolName)
			if toolName == "Bash" && ev.Data.ToolInput != nil {
				if cmd, ok := ev.Data.ToolInput["command"].(string); ok {
					wrapped := WrapTextWithIndent("$ "+cmd, "    ", termWidth)
					_, _ = fmt.Fprintf(w, "%s\n", wrapped)
				}
			} else if fm := ev.Data.FileModification; fm != nil {
				actionLine := fmt.Sprintf("%s %s", fm.Action, fm.FilePath)
				wrappedAction := WrapTextWithIndent(actionLine, "    ", termWidth)
				_, _ = fmt.Fprintf(w, "%s\n", wrappedAction)
				if len(fm.Hunks) > 0 {
					diffText := formatDiffHunks(fm.Hunks)
					if diffText != "" {
						wrapped := WrapTextWithIndent(diffText, "    ", termWidth)
						_, _ = fmt.Fprintf(w, "%s\n", wrapped)
					}
				}
			} else if p, ok := ev.Data.ToolInput["path"].(string); ok && p != "" {
				wrapped := WrapTextWithIndent("Path: "+p, "    ", termWidth)
				_, _ = fmt.Fprintf(w, "%s\n", wrapped)
			} else if len(ev.Data.ToolInput) > 0 {
				inBytes, _ := json.MarshalIndent(ev.Data.ToolInput, "", "  ")
				wrapped := WrapTextWithIndent(string(inBytes), "    ", termWidth)
				_, _ = fmt.Fprintf(w, "%s\n", wrapped)
			}

		case "tool_result":
			isErr := ev.Data.IsError != nil && *ev.Data.IsError
			outputStr := ""
			if ev.Data.ToolOutput != nil {
				outputStr = strings.TrimSpace(*ev.Data.ToolOutput)
			}
			tag := "\n    [RESULT]"
			if isErr {
				tag = "\n    [TOOL ERROR]"
			}
			if outputStr != "" {
				wrapped := WrapTextWithIndent(outputStr, "        ", termWidth)
				_, _ = fmt.Fprintf(w, "%s\n%s\n\n", tag, wrapped)
			} else {
				_, _ = fmt.Fprintf(w, "%s (empty output)\n\n", tag)
			}

		case "file_modification":
			if fm := ev.Data.FileModification; fm != nil {
				_, _ = fmt.Fprintf(w, "[FILE MOD] %s %s (hunks: %d)\n", fm.Action, fm.FilePath, len(fm.Hunks))
				if len(fm.Hunks) > 0 {
					diffText := formatDiffHunks(fm.Hunks)
					if diffText != "" {
						wrapped := WrapTextWithIndent(diffText, "    ", termWidth)
						_, _ = fmt.Fprintf(w, "%s\n", wrapped)
					}
				}
			}

		case "error":
			msg := ""
			if ev.Data.Content != nil {
				msg = *ev.Data.Content
			}
			wrapped := WrapTextWithIndent(msg, "    ", termWidth)
			_, _ = fmt.Fprintf(w, "[ERROR]\n%s\n\n", wrapped)

		case "run_exited":
			code := 0
			if ev.Data.ExitCode != nil {
				code = *ev.Data.ExitCode
			}
			_, _ = fmt.Fprintf(w, "[EXIT] Session exited with code %d\n", code)
		}
	}

	return nil
}

func extractSessionMeta(events []parser.ParsedEvent) (title, branch, cwd string) {
	for _, ev := range events {
		if ev.Data.Title != nil && *ev.Data.Title != "" && title == "" {
			title = *ev.Data.Title
		}
		if ev.GitBranch != "" && branch == "" {
			branch = ev.GitBranch
		}
		if ev.CWD != "" && cwd == "" {
			cwd = ev.CWD
		}
	}
	return title, branch, cwd
}

func extractPromptText(ev parser.ParsedEvent) string {
	rawPrompt := ""
	if ev.Data.Content != nil {
		rawPrompt = *ev.Data.Content
	} else if ev.Data.LastPrompt != nil {
		rawPrompt = *ev.Data.LastPrompt
	}
	cleaned := strings.TrimSpace(parser.CleanClaudeCodePrompt(rawPrompt))
	if cleaned != "" {
		return cleaned
	}
	return strings.TrimSpace(rawPrompt)
}

func formatDiffHunks(hunks []types.DiffHunk) string {
	if len(hunks) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range hunks {
		if h.Header != "" {
			b.WriteString(h.Header)
			b.WriteString("\n")
		} else {
			b.WriteString("@@ diff @@\n")
		}
		for _, l := range h.Lines {
			prefix := " "
			switch l.Type {
			case "add":
				prefix = "+"
			case "delete":
				prefix = "-"
			}
			b.WriteString(prefix)
			b.WriteString(l.Content)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// GetTerminalWidth returns the stdout terminal width, defaulting to 80 if not a terminal.
func GetTerminalWidth() int {
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

// WrapTextWithIndent wraps lines in text to fit termWidth, ensuring every line has at least
// the specified indent prefix. Hanging indents are applied to lists and bulleted items.
func WrapTextWithIndent(text string, indent string, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80
	}
	indentLen := len(indent)
	maxContentWidth := termWidth - indentLen
	if maxContentWidth < 30 {
		maxContentWidth = 30
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, rawLine := range lines {
		trimmed := strings.TrimRight(rawLine, " \t\r")
		if trimmed == "" {
			result = append(result, "")
			continue
		}

		bulletIndent := ""
		lineContent := trimmed
		leadingSpaces := len(trimmed) - len(strings.TrimLeft(trimmed, " "))
		trimmedLead := strings.TrimLeft(trimmed, " ")

		if strings.HasPrefix(trimmedLead, "- ") || strings.HasPrefix(trimmedLead, "* ") || strings.HasPrefix(trimmedLead, "+ ") {
			bulletIndent = strings.Repeat(" ", leadingSpaces+2)
		} else if idx := strings.Index(trimmedLead, ". "); idx > 0 && idx <= 3 && isNumeric(trimmedLead[:idx]) {
			bulletIndent = strings.Repeat(" ", leadingSpaces+idx+2)
		} else if leadingSpaces > 0 {
			bulletIndent = strings.Repeat(" ", leadingSpaces)
		}

		if len(lineContent) <= maxContentWidth {
			result = append(result, indent+lineContent)
			continue
		}

		words := strings.Fields(lineContent)
		if len(words) == 0 {
			result = append(result, indent+lineContent)
			continue
		}

		var curLine strings.Builder
		curPrefix := indent
		if leadingSpaces > 0 && !strings.HasPrefix(trimmedLead, "- ") && !strings.HasPrefix(trimmedLead, "* ") {
			curPrefix = indent + strings.Repeat(" ", leadingSpaces)
		}

		firstWord := true
		for _, w := range words {
			if firstWord {
				curLine.WriteString(curPrefix)
				curLine.WriteString(w)
				firstWord = false
				continue
			}

			if curLine.Len()+1+len(w) > termWidth {
				result = append(result, curLine.String())
				curLine.Reset()
				curPrefix = indent + bulletIndent
				curLine.WriteString(curPrefix)
				curLine.WriteString(w)
			} else {
				curLine.WriteString(" ")
				curLine.WriteString(w)
			}
		}
		if curLine.Len() > 0 {
			result = append(result, curLine.String())
		}
	}

	return strings.Join(result, "\n")
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
