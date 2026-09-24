package output

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/mattn/go-runewidth"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// Standard single-cell terminal status glyphs to prevent multi-cell/EAW visual misalignment.
const (
	GlyphOK       = "[OK]"
	GlyphFail     = "[FAIL]"
	GlyphWarn     = "[WARN]"
	GlyphError    = "[ERROR]"
	GlyphQuestion = "[?]"
	GlyphAlert    = "[!]"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\].*?(?:\x07|\x1b\\)`)

// stripANSI removes ANSI escape codes from s.
func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiRegex.ReplaceAllString(s, "")
}

// StripANSI removes ANSI escape codes from s.
func StripANSI(s string) string {
	return stripANSI(s)
}

func extractProjectCandidates(projectName string) []string {
	trimmed := strings.TrimSpace(projectName)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}

	normalized := strings.TrimRight(trimmed, "/\\")
	lastSlash := strings.LastIndexAny(normalized, "/\\")
	var basename string
	if lastSlash != -1 {
		basename = normalized[lastSlash+1:]
	} else {
		basename = normalized
	}
	if strings.HasSuffix(strings.ToLower(basename), ".git") {
		basename = basename[:len(basename)-4]
	}
	if basename != "" && basename != trimmed {
		candidates = append(candidates, basename)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i]) > len(candidates[j])
	})
	return candidates
}

// TrimProjectPrefix removes redundant project name prefixes from session titles
// when displayed under a project header or within a project context.
func TrimProjectPrefix(title, projectName string) string {
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return title
	}
	candidates := extractProjectCandidates(projectName)
	if len(candidates) == 0 {
		return title
	}

	for _, candidate := range candidates {
		escaped := regexp.QuoteMeta(candidate)

		// Pattern 1: Bracketed project prefix e.g. "[agent-status]" or "[agent-status]:"
		bracketedPattern := `(?i)^\s*\[\s*` + escaped + `\s*\]\s*(?:[:·|–—]|-+)?\s*`
		if re, err := regexp.Compile(bracketedPattern); err == nil {
			loc := re.FindStringIndex(title)
			if loc != nil && loc[0] == 0 {
				remainder := strings.TrimSpace(title[loc[1]:])
				substantive := strings.TrimFunc(remainder, func(r rune) bool {
					return r == ':' || r == '·' || r == '|' || r == '-' || r == '–' || r == '—' || r == ']' || unicode.IsSpace(r)
				})
				if len(substantive) > 0 {
					return remainder
				}
				return title
			}
		}

		// Pattern 2: Non-bracketed project prefix followed by separator
		// Separators: ':', '·', '|', ']', or '-' (with whitespace boundary to avoid matching kebab-case words)
		nonBracketedPattern := `(?i)^\s*` + escaped + `(?:\s*[:·|–—\]]\s*|\s+[-–—]+\s*|\s*[-–—]+\s+)`
		if re, err := regexp.Compile(nonBracketedPattern); err == nil {
			loc := re.FindStringIndex(title)
			if loc != nil && loc[0] == 0 {
				remainder := strings.TrimSpace(title[loc[1]:])
				substantive := strings.TrimFunc(remainder, func(r rune) bool {
					return r == ':' || r == '·' || r == '|' || r == '-' || r == '–' || r == '—' || r == ']' || unicode.IsSpace(r)
				})
				if len(substantive) > 0 {
					return remainder
				}
				return title
			}
		}
	}

	return title
}


// VisualWidth returns the visual terminal cell width of s, excluding ANSI escape sequences.
func VisualWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

// TruncateLeftVisual truncates s from the left, keeping the rightmost characters
// such that prefix + tail has visual display width <= maxWidth.
func TruncateLeftVisual(s string, maxWidth int, prefix string) string {
	plain := stripANSI(s)
	if runewidth.StringWidth(plain) <= maxWidth {
		return s
	}
	prefixWidth := runewidth.StringWidth(stripANSI(prefix))
	available := maxWidth - prefixWidth
	if available <= 0 {
		return runewidth.Truncate(prefix, maxWidth, "")
	}
	runes := []rune(s)
	totalWidth := 0
	cutIdx := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		w := runewidth.RuneWidth(runes[i])
		if totalWidth+w > available {
			break
		}
		totalWidth += w
		cutIdx = i
	}
	return prefix + string(runes[cutIdx:])
}

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
	rw := runewidth.RuneWidth(char)
	if rw < 1 {
		rw = 1
	}
	count := width / rw
	if count < 1 {
		count = 1
	}
	_, _ = fmt.Fprintln(w, strings.Repeat(string(char), count))
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
		if runewidth.StringWidth(stripANSI(title)) > 35 {
			title = runewidth.Truncate(title, 35, "...")
		}
		dir := s.Directory
		if runewidth.StringWidth(stripANSI(dir)) > 35 {
			dir = TruncateLeftVisual(dir, 35, "...")
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
	IsSynthetic bool            `json:"isSynthetic,omitempty"`
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
		syntheticStr := ""
		if diff.IsSynthetic {
			syntheticStr = " synthetic:true"
		}
		deltaStr := ""
		if diff.InputDelta != 0 || diff.OutputDelta != 0 || diff.CostDelta != 0 {
			deltaStr = fmt.Sprintf(" in:+%d out:+%d cost:+$%.4f", diff.InputDelta, diff.OutputDelta, diff.CostDelta)
		}
		_, _ = fmt.Fprintf(w, "diff step:%d event:%s%s line:%d%s mutations:[%s]%s\n",
			diff.StepIndex, diff.EventType, syntheticStr, diff.LineNumber, subStr, strings.Join(muts, ","), deltaStr)
		return
	}

	loc := ""
	if diff.LineNumber > 0 {
		loc = fmt.Sprintf(" (line %d)", diff.LineNumber)
	}
	if diff.SubagentID != nil && *diff.SubagentID != "" {
		loc += fmt.Sprintf(" [subagent: %s]", *diff.SubagentID)
	}
	synth := ""
	if diff.IsSynthetic {
		synth = " (synthetic)"
	}

	_, _ = fmt.Fprintf(w, "Step %d [%s]%s%s:\n", diff.StepIndex, diff.EventType, synth, loc)
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
	Harness              types.HarnessType    `json:"harness"`
	SessionID            string               `json:"sessionId"`
	AgentID              string               `json:"agentId"`
	ProjectName          string               `json:"projectName"`
	Branch               string               `json:"branch"`
	CWD                  string               `json:"cwd"`
	Activity             types.ActivityState  `json:"activity"`
	Lifecycle            types.LifecycleState `json:"lifecycle"`
	IsLifecycleSynthetic bool                 `json:"isLifecycleSynthetic,omitempty"`
	Title                string               `json:"title"`
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
		lifecycleStr := string(s.Lifecycle)
		if s.Lifecycle == types.LifecycleExited {
			if s.IsLifecycleSynthetic {
				lifecycleStr = "EXITED(synthetic)"
			} else {
				lifecycleStr = "EXITED(native)"
			}
		}
		_, _ = fmt.Fprintf(w, "harness:%s session_id:%s agent_id:%s project:%s branch:%s activity:%s lifecycle:%s tokens_in:%d tokens_out:%d tokens_total:%d cost_usd:%.4f subagents:%d frames:%d verification:%s ver_logs:%d duration_ms:%d\n",
			s.Harness, s.SessionID, s.AgentID, s.ProjectName, s.Branch, s.Activity, lifecycleStr,
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
	lifecycleStr := string(s.Lifecycle)
	if s.Lifecycle == types.LifecycleExited {
		if s.IsLifecycleSynthetic {
			lifecycleStr += " (synthetic)"
		} else {
			lifecycleStr += " (native)"
		}
	}
	_, _ = fmt.Fprintf(w, "  Lifecycle:       %s\n", lifecycleStr)
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
		_, _ = fmt.Fprintf(w, "%s Read-side query verification passed (GetAgentByID, %d logs scanned)\n", GlyphOK, s.VerificationLogs)
	} else {
		_, _ = fmt.Fprintf(w, "%s Read-side query verification failed!\n", GlyphError)
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
		tag := GlyphOK
		if !c.Passed {
			tag = GlyphFail
		}
		_, _ = fmt.Fprintf(w, "%-6s %-32s %s\n", tag, c.Name, c.Details)
	}

	RenderDivider(w, '-')
	if r.Passed {
		_, _ = fmt.Fprintf(w, "%s Session transcript is VALID and passes all assertions.\n", GlyphOK)
	} else {
		_, _ = fmt.Fprintf(w, "%s Session transcript failed one or more validation checks.\n", GlyphFail)
	}
	RenderDivider(w, '=')
}
