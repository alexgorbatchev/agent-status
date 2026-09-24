package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/mattn/go-runewidth"
)

var harnessLabels = map[types.HarnessType]string{
	types.HarnessClaudeCode: "Claude Code",
	types.HarnessPi:         "Pi",
	types.HarnessCodex:      "Codex",
	types.HarnessOpencode:   "OpenCode",
}

// FormatHarnessLabel returns the human display name for a harness.
func FormatHarnessLabel(h types.HarnessType) string {
	if label, ok := harnessLabels[h]; ok {
		return label
	}
	if h == "" {
		return "Unknown"
	}
	return string(h)
}

// FormatTokens formats an integer token count with commas, e.g. 14520 -> "14,520 tokens".
func FormatTokens(tokens int64) string {
	str := fmt.Sprintf("%d", tokens)
	n := len(str)
	if n <= 3 {
		return str + " tokens"
	}
	var b strings.Builder
	remainder := n % 3
	if remainder > 0 {
		b.WriteString(str[:remainder])
		if n > remainder {
			b.WriteString(",")
		}
	}
	for i := remainder; i < n; i += 3 {
		b.WriteString(str[i : i+3])
		if i+3 < n {
			b.WriteString(",")
		}
	}
	b.WriteString(" tokens")
	return b.String()
}

// FormatRelativeTime formats a millisecond timestamp relative to nowMs.
func FormatRelativeTime(timestampMs int64, nowMs int64) string {
	if timestampMs <= 0 {
		return "never"
	}
	diffMs := nowMs - timestampMs
	if diffMs < 5000 {
		return "just now"
	}
	if diffMs < 60000 {
		seconds := diffMs / 1000
		return fmt.Sprintf("%ds ago", seconds)
	}
	if diffMs < 3600000 {
		minutes := diffMs / 60000
		return fmt.Sprintf("%dm ago", minutes)
	}
	if diffMs < 86400000 {
		hours := diffMs / 3600000
		return fmt.Sprintf("%dh ago", hours)
	}
	days := diffMs / 86400000
	return fmt.Sprintf("%dd ago", days)
}

// RenderAgentView renders a terminal representation of an agent state card using the current wall clock.
func RenderAgentView(w io.Writer, agent types.IAgentState, agentMode bool) {
	RenderAgentViewAt(w, agent, agentMode, time.Now().UnixMilli())
}

// RenderAgentViewAt renders a terminal representation of an agent state card with a specified reference timestamp.
func RenderAgentViewAt(w io.Writer, agent types.IAgentState, agentMode bool, nowMs int64) {
	if agentMode {
		renderAgentViewAgentMode(w, agent)
		return
	}
	renderAgentViewHumanMode(w, agent, nowMs)
}

func renderAgentViewAgentMode(w io.Writer, agent types.IAgentState) {
	harness := string(agent.Harness)
	if harness == "" {
		harness = "unknown"
	}
	env := agent.Environment.Label
	if env == "" {
		env = agent.Environment.ID
	}
	if env == "" {
		env = "local-dev"
	}
	title := agent.Activity.Title
	if title == "" {
		title = "Untitled Agent Session"
	}
	projectName := agent.Project.Name
	if projectName == "" {
		projectName = agent.Project.ID
	}
	title = TrimProjectPrefix(title, projectName)
	action := agent.Activity.Action
	if action == "" {
		if agent.Activity.ActiveTool != nil && *agent.Activity.ActiveTool != "" {
			action = fmt.Sprintf("Executing %s", *agent.Activity.ActiveTool)
		} else {
			action = "Idle"
		}
	}
	activity := string(agent.Activity.Activity)
	if activity == "" {
		activity = string(types.ActivityIdle)
	}
	lifecycle := string(agent.Activity.Lifecycle)
	if lifecycle == "" {
		lifecycle = string(types.LifecycleRunning)
	}
	branch := agent.Workspace.Branch
	if branch == "" {
		branch = "main"
	}

	cost := 0.0
	if agent.Activity.Cost != nil {
		cost = agent.Activity.Cost.Total
	}
	tokens := agent.Activity.Usage.Total

	var parts []string
	parts = append(parts, fmt.Sprintf("harness:%s", harness))
	parts = append(parts, fmt.Sprintf("env:%s", env))
	parts = append(parts, fmt.Sprintf("title:%q", title))
	parts = append(parts, fmt.Sprintf("action:%q", action))
	parts = append(parts, fmt.Sprintf("activity:%s", activity))

	if agent.Activity.Activity == types.ActivityWaitingQuestion && agent.Activity.Question != nil && *agent.Activity.Question != "" {
		parts = append(parts, fmt.Sprintf("question:%q", *agent.Activity.Question))
	} else if agent.Activity.Activity == types.ActivityError && agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
		parts = append(parts, fmt.Sprintf("error:%q", *agent.Activity.LastError))
	}

	parts = append(parts, fmt.Sprintf("lifecycle:%s", lifecycle))
	parts = append(parts, fmt.Sprintf("branch:%s", branch))

	if agent.Workspace.IsWorktree && agent.Workspace.RelPath != nil && *agent.Workspace.RelPath != "" {
		parts = append(parts, fmt.Sprintf("worktree:%s", *agent.Workspace.RelPath))
	}
	if agent.Activity.SubagentCount > 0 {
		parts = append(parts, fmt.Sprintf("subagents:%d", agent.Activity.SubagentCount))
	}

	parts = append(parts, fmt.Sprintf("cost_usd:%.4f", cost))
	parts = append(parts, fmt.Sprintf("tokens:%d", tokens))

	_, _ = fmt.Fprintln(w, strings.Join(parts, " "))
}

func renderAgentViewHumanMode(w io.Writer, agent types.IAgentState, nowMs int64) {
	width := GetTerminalWidth()
	if width < 40 {
		width = 40
	}
	divider := strings.Repeat("-", width)

	// 1. Header row
	lifecycle := string(agent.Activity.Lifecycle)
	if lifecycle == "" {
		lifecycle = string(types.LifecycleRunning)
	}

	harness := FormatHarnessLabel(agent.Harness)
	hideButton := "[Hide]"
	if agent.IsHidden {
		hideButton = "[Unhide]"
	}
	var buttons []string
	if lifecycle == string(types.LifecycleRunning) {
		buttons = append(buttons, "[Focus]")
	}
	buttons = append(buttons, hideButton)
	headerLeft := fmt.Sprintf("(%s) %s", harness, strings.Join(buttons, " "))
	headerRight := fmt.Sprintf("(%s)", lifecycle)

	headerSpace := width - runewidth.StringWidth(stripANSI(headerLeft)) - runewidth.StringWidth(stripANSI(headerRight))
	if headerSpace < 1 {
		headerSpace = 1
	}
	headerLine := headerLeft + strings.Repeat(" ", headerSpace) + headerRight

	// 2. Title & Action
	title := agent.Activity.Title
	if title == "" {
		title = "Untitled Agent Session"
	}
	projectName := agent.Project.Name
	if projectName == "" {
		projectName = agent.Project.ID
	}
	title = TrimProjectPrefix(title, projectName)
	action := agent.Activity.Action
	if action == "" {
		if agent.Activity.ActiveTool != nil && *agent.Activity.ActiveTool != "" {
			action = fmt.Sprintf("Executing %s", *agent.Activity.ActiveTool)
		} else {
			action = "Idle"
		}
	}

	// 3. Callout (question or error)
	var callout string
	if agent.Activity.Activity == types.ActivityWaitingQuestion && agent.Activity.Question != nil && *agent.Activity.Question != "" {
		callout = fmt.Sprintf("%s %s", GlyphQuestion, *agent.Activity.Question)
	} else if agent.Activity.Activity == types.ActivityError && agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
		callout = fmt.Sprintf("%s %s", GlyphAlert, *agent.Activity.LastError)
	}

	// 4. Badges
	env := agent.Environment.Label
	if env == "" {
		env = agent.Environment.ID
	}
	if env == "" {
		env = "local-dev"
	}
	branch := agent.Workspace.Branch
	if branch == "" {
		branch = "main"
	}
	var badges []string
	badges = append(badges, fmt.Sprintf("(%s)", env))
	badges = append(badges, fmt.Sprintf("(branch: %s)", branch))

	if agent.Workspace.IsWorktree && agent.Workspace.RelPath != nil && *agent.Workspace.RelPath != "" {
		badges = append(badges, fmt.Sprintf("(worktree: %s)", *agent.Workspace.RelPath))
	}
	if agent.Activity.SubagentCount > 0 {
		if agent.Activity.SubagentCount == 1 {
			badges = append(badges, "(1 subagent)")
		} else {
			badges = append(badges, fmt.Sprintf("(%d subagents)", agent.Activity.SubagentCount))
		}
	}
	for _, sub := range agent.Activity.ActiveSubagents {
		var subLabel string
		if sub.ActiveTool != nil && *sub.ActiveTool != "" {
			subLabel = fmt.Sprintf("%s: %s", sub.ID, *sub.ActiveTool)
		} else if sub.Action != "" {
			subLabel = fmt.Sprintf("%s: %s", sub.ID, sub.Action)
		} else {
			subLabel = fmt.Sprintf("%s: %s", sub.ID, sub.Activity)
		}
		badges = append(badges, fmt.Sprintf("(subagent %s)", subLabel))
	}
	relTime := FormatRelativeTime(agent.Activity.UpdatedAt, nowMs)
	badges = append(badges, fmt.Sprintf("(active: %s)", relTime))
	badgesLine := strings.Join(badges, "  ")

	// 5. Metrics row
	cost := 0.0
	if agent.Activity.Cost != nil {
		cost = agent.Activity.Cost.Total
	}
	tokens := agent.Activity.Usage.Total
	metricsLeft := fmt.Sprintf("$%.4f USD", cost)
	metricsRight := FormatTokens(tokens)
	metricsSpace := width - runewidth.StringWidth(stripANSI(metricsLeft)) - runewidth.StringWidth(stripANSI(metricsRight))
	if metricsSpace < 1 {
		metricsSpace = 1
	}
	metricsLine := metricsLeft + strings.Repeat(" ", metricsSpace) + metricsRight

	// Print lines
	_, _ = fmt.Fprintln(w, divider)
	_, _ = fmt.Fprintln(w, headerLine)
	_, _ = fmt.Fprintln(w, divider)
	_, _ = fmt.Fprintln(w, title)
	_, _ = fmt.Fprintln(w, action)
	if callout != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, callout)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, badgesLine)
	_, _ = fmt.Fprintln(w, divider)
	_, _ = fmt.Fprintln(w, metricsLine)
	_, _ = fmt.Fprintln(w, divider)
}
