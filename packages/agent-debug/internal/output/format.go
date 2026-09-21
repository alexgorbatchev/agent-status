package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// IsAgentMode returns true if AGENT=1, true, or yes in the environment.
func IsAgentMode() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT")))
	return v == "1" || v == "true" || v == "yes"
}

// GetTerminalWidth returns the current stdout terminal width, defaulting to 80.
func GetTerminalWidth() int {
	width := 80
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			width = w
		}
	}
	return width
}

// RenderDivider prints a horizontal line spanning the terminal width in human mode.
func RenderDivider(w io.Writer, char rune) {
	if IsAgentMode() {
		return
	}
	width := GetTerminalWidth()
	_, _ = fmt.Fprintln(w, strings.Repeat(string(char), width))
}

// SessionSummary represents a session listed from OpenCode database or multi-session files.
type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Directory string    `json:"directory"`
	CreatedAt time.Time `json:"createdAt"`
	CostUSD   float64   `json:"costUSD"`
	Tokens    int64     `json:"tokens"`
}

// RenderSessionList writes the session list in either human table or agent flat mode.
func RenderSessionList(w io.Writer, sessions []SessionSummary, agentMode bool) {
	if agentMode {
		for _, s := range sessions {
			title := s.Title
			if title == "" {
				title = "(untitled)"
			}
			_, _ = fmt.Fprintf(w, "session_id:%s title:%q cwd:%s created_at:%d cost_usd:%.4f tokens:%d\n",
				s.ID, title, s.Directory, s.CreatedAt.UnixMilli(), s.CostUSD, s.Tokens)
		}
		return
	}

	if len(sessions) == 0 {
		_, _ = fmt.Fprintln(w, "No sessions found.")
		return
	}

	table := tablewriter.NewTable(w, tablewriter.WithHeader([]string{"Session ID", "Title", "Directory", "Created", "Cost ($)", "Tokens"}))

	for _, s := range sessions {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}
		if len(title) > 35 {
			title = title[:32] + "..."
		}
		dir := s.Directory
		if len(dir) > 35 {
			dir = "..." + dir[len(dir)-32:]
		}

		_ = table.Append([]string{
			s.ID,
			title,
			dir,
			s.CreatedAt.Format("2006-01-02 15:04"),
			fmt.Sprintf("$%.4f", s.CostUSD),
			fmt.Sprintf("%d", s.Tokens),
		})
	}
	_ = table.Render()
}

// FieldMutation captures a single state property transition.
type FieldMutation struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// StateDiff contains the differences detected between replay steps.
type StateDiff struct {
	StepIndex   int             `json:"stepIndex"`
	LineNumber  int             `json:"lineNumber,omitempty"`
	EventType   string          `json:"eventType"`
	SubagentID  *string         `json:"subagentId,omitempty"`
	Mutations   []FieldMutation `json:"mutations"`
	InputDelta  int64           `json:"inputDelta,omitempty"`
	OutputDelta int64           `json:"outputDelta,omitempty"`
	CostDelta   float64         `json:"costDelta,omitempty"`
}

// RenderDiff formats and prints a step-by-step state transition diff.
func RenderDiff(w io.Writer, diff StateDiff, agentMode bool) {
	if agentMode {
		muts := make([]string, 0, len(diff.Mutations))
		for _, m := range diff.Mutations {
			muts = append(muts, fmt.Sprintf("%s:%s->%s", m.Field, m.From, m.To))
		}
		subStr := ""
		if diff.SubagentID != nil && *diff.SubagentID != "" {
			subStr = fmt.Sprintf(" subagent:%s", *diff.SubagentID)
		}
		deltaStr := ""
		if diff.InputDelta != 0 || diff.OutputDelta != 0 || diff.CostDelta != 0 {
			deltaStr = fmt.Sprintf(" in:+%d out:+%d cost:+$%.4f", diff.InputDelta, diff.OutputDelta, diff.CostDelta)
		}
		_, _ = fmt.Fprintf(w, "diff step:%d event:%s line:%d%s mutations:[%s]%s\n",
			diff.StepIndex, diff.EventType, diff.LineNumber, subStr, strings.Join(muts, ","), deltaStr)
		return
	}

	loc := ""
	if diff.LineNumber > 0 {
		loc = fmt.Sprintf(" (line %d)", diff.LineNumber)
	}
	if diff.SubagentID != nil && *diff.SubagentID != "" {
		loc += fmt.Sprintf(" [subagent: %s]", *diff.SubagentID)
	}

	_, _ = fmt.Fprintf(w, "Step %d [%s]%s:\n", diff.StepIndex, diff.EventType, loc)
	for _, m := range diff.Mutations {
		_, _ = fmt.Fprintf(w, "  %-14s %s -> %s\n", m.Field+":", m.From, m.To)
	}
	if diff.InputDelta != 0 || diff.OutputDelta != 0 {
		_, _ = fmt.Fprintf(w, "  %-14s In: +%d | Out: +%d\n", "Tokens:", diff.InputDelta, diff.OutputDelta)
	}
	if diff.CostDelta > 0 {
		_, _ = fmt.Fprintf(w, "  %-14s +$%.4f\n", "Cost:", diff.CostDelta)
	}
	_, _ = fmt.Fprintln(w)
}

// ReplaySummary contains high-level metrics from a completed replay.
type ReplaySummary struct {
	Harness          types.HarnessType   `json:"harness"`
	SessionID        string              `json:"sessionId"`
	AgentID          string              `json:"agentId"`
	ProjectName      string              `json:"projectName"`
	Branch           string              `json:"branch"`
	CWD              string              `json:"cwd"`
	Activity         types.ActivityState `json:"activity"`
	Lifecycle        types.LifecycleState`json:"lifecycle"`
	Title            string              `json:"title"`
	Action           string              `json:"action"`
	TokensInput      int64               `json:"tokensInput"`
	TokensOutput     int64               `json:"tokensOutput"`
	TokensTotal      int64               `json:"tokensTotal"`
	CostUSD          float64             `json:"costUSD"`
	SubagentCount    int                 `json:"subagentCount"`
	FramesProcessed  int                 `json:"framesProcessed"`
	VerificationOK   bool                `json:"verificationOk"`
	VerificationLogs int                 `json:"verificationLogs"`
	DurationMs       int64               `json:"durationMs"`
}

// RenderReplaySummary writes the summary of the replayed session.
func RenderReplaySummary(w io.Writer, s ReplaySummary, agentMode bool) {
	if agentMode {
		verStatus := "OK"
		if !s.VerificationOK {
			verStatus = "FAIL"
		}
		_, _ = fmt.Fprintf(w, "harness:%s session_id:%s agent_id:%s project:%s branch:%s activity:%s lifecycle:%s tokens_in:%d tokens_out:%d tokens_total:%d cost_usd:%.4f subagents:%d frames:%d verification:%s ver_logs:%d duration_ms:%d\n",
			s.Harness, s.SessionID, s.AgentID, s.ProjectName, s.Branch, s.Activity, s.Lifecycle,
			s.TokensInput, s.TokensOutput, s.TokensTotal, s.CostUSD, s.SubagentCount,
			s.FramesProcessed, verStatus, s.VerificationLogs, s.DurationMs)
		return
	}

	RenderDivider(w, '-')
	_, _ = fmt.Fprintf(w, "Session Replay Summary (%s)\n", s.Harness)
	RenderDivider(w, '-')
	_, _ = fmt.Fprintf(w, "  Session ID:      %s\n", s.SessionID)
	_, _ = fmt.Fprintf(w, "  Agent ID:        %s\n", s.AgentID)
	_, _ = fmt.Fprintf(w, "  Project:         %s (branch: %s)\n", s.ProjectName, s.Branch)
	_, _ = fmt.Fprintf(w, "  Directory:       %s\n", s.CWD)
	if s.Title != "" {
		_, _ = fmt.Fprintf(w, "  Title:           %s\n", s.Title)
	}
	if s.Action != "" {
		_, _ = fmt.Fprintf(w, "  Action:          %s\n", s.Action)
	}
	_, _ = fmt.Fprintf(w, "  Lifecycle:       %s\n", s.Lifecycle)
	_, _ = fmt.Fprintf(w, "  Activity:        %s\n", s.Activity)
	_, _ = fmt.Fprintf(w, "  Token Usage:     Input: %d | Output: %d | Total: %d\n",
		s.TokensInput, s.TokensOutput, s.TokensTotal)
	_, _ = fmt.Fprintf(w, "  Total Cost:      $%.4f USD\n", s.CostUSD)
	if s.SubagentCount > 0 {
		_, _ = fmt.Fprintf(w, "  Subagents:       %d active/completed\n", s.SubagentCount)
	}
	_, _ = fmt.Fprintf(w, "  Frames:          %d frames replayed in %d ms\n", s.FramesProcessed, s.DurationMs)
	RenderDivider(w, '-')

	if s.VerificationOK {
		_, _ = fmt.Fprintf(w, "[OK] Read-side query verification passed (GetAgentByID, %d logs scanned)\n", s.VerificationLogs)
	} else {
		_, _ = fmt.Fprintf(w, "[ERROR] Read-side query verification failed!\n")
	}
	RenderDivider(w, '-')
}

// ValidationCheck represents an individual assertion during validation.
type ValidationCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}

// ValidationReport collects all checks for a validated transcript.
type ValidationReport struct {
	Harness types.HarnessType `json:"harness"`
	Target  string            `json:"target"`
	Passed  bool              `json:"passed"`
	Checks  []ValidationCheck `json:"checks"`
}

// RenderValidationReport writes the validation checklist.
func RenderValidationReport(w io.Writer, r ValidationReport, agentMode bool) {
	if agentMode {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		_, _ = fmt.Fprintf(w, "validation target:%s harness:%s status:%s\n", r.Target, r.Harness, status)
		for _, c := range r.Checks {
			cStatus := "OK"
			if !c.Passed {
				cStatus = "FAIL"
			}
			_, _ = fmt.Fprintf(w, "  check:%s status:%s details:%q\n", c.Name, cStatus, c.Details)
		}
		return
	}

	RenderDivider(w, '=')
	_, _ = fmt.Fprintf(w, "Validation Report: %s (%s)\n", r.Target, r.Harness)
	RenderDivider(w, '=')

	for _, c := range r.Checks {
		tag := "[OK]"
		if !c.Passed {
			tag = "[FAIL]"
		}
		_, _ = fmt.Fprintf(w, "%-6s %-32s %s\n", tag, c.Name, c.Details)
	}

	RenderDivider(w, '-')
	if r.Passed {
		_, _ = fmt.Fprintln(w, "[OK] Session transcript is VALID and passes all assertions.")
	} else {
		_, _ = fmt.Fprintln(w, "[FAIL] Session transcript failed one or more validation checks.")
	}
	RenderDivider(w, '=')
}
