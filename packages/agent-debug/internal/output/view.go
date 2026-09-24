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

// FormatHarnessBadge formats the harness badge with semantic color precedence (#106).
// Rule 1: Error (ANSI Red) and Prompt (ANSI Yellow) styles override vendor badge coloring.
// Rule 2: In quiescent/working states, vendor brand color is applied (prohibiting Red or Green hues).
func FormatHarnessBadge(h types.HarnessType, activity types.ActivityState) string {
	rawLabel := FormatHarnessLabel(h)
	cleanLabel := StripANSI(rawLabel)

	switch activity {
	case types.ActivityError:
		return fmt.Sprintf("%s(%s)%s", AnsiRed, cleanLabel, AnsiReset)
	case types.ActivityWaitingQuestion, types.ActivityRateLimited:
		return fmt.Sprintf("%s(%s)%s", AnsiYellow, cleanLabel, AnsiReset)
	default:
		brandColor := GetHarnessBrandColor(h)
		if strings.Contains(rawLabel, "\x1b") {
			if strings.Contains(rawLabel, AnsiRed) || strings.Contains(rawLabel, AnsiGreen) {
				return fmt.Sprintf("%s(%s)%s", brandColor, cleanLabel, AnsiReset)
			}
			return fmt.Sprintf("(%s)", rawLabel)
		}
		return fmt.Sprintf("%s(%s)%s", brandColor, cleanLabel, AnsiReset)
	}
}

// IdleFreshness represents the freshness tier of an idle agent session.
type IdleFreshness string

const (
	FreshnessFresh    IdleFreshness = "fresh"
	FreshnessStandard IdleFreshness = "standard"
	FreshnessStale    IdleFreshness = "stale"
)

// Durations for freshness boundaries.
const (
	idleFreshThresholdMs int64 = 15 * 60 * 1000     // 15 minutes
	idleStaleThresholdMs int64 = 2 * 60 * 60 * 1000 // 2 hours
)

// GetIdleFreshness returns the freshness tier for an idle session based on updatedAt timestamp.
func GetIdleFreshness(updatedAt int64, nowMs int64) IdleFreshness {
	if updatedAt <= 0 {
		return FreshnessStale
	}
	diffMs := nowMs - updatedAt
	if diffMs < idleFreshThresholdMs {
		return FreshnessFresh
	}
	if diffMs < idleStaleThresholdMs {
		return FreshnessStandard
	}
	return FreshnessStale
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

type fleetWorktreeGroup struct {
	key     string
	branch  string
	relPath string
	agents  []types.IAgentState
}

type fleetProjectGroup struct {
	id         string
	name       string
	rootAgents []types.IAgentState
	worktrees  []*fleetWorktreeGroup
}

func groupAgentsForFleet(agents []types.IAgentState) []*fleetProjectGroup {
	var projectGroups []*fleetProjectGroup
	projectMap := make(map[string]*fleetProjectGroup)

	for _, agent := range agents {
		projID := agent.Project.ID
		if projID == "" {
			projID = "unknown"
		}
		projName := agent.Project.Name
		if projName == "" {
			projName = projID
		}

		grp, exists := projectMap[projID]
		if !exists {
			grp = &fleetProjectGroup{
				id:   projID,
				name: projName,
			}
			projectMap[projID] = grp
			projectGroups = append(projectGroups, grp)
		}

		if !agent.Workspace.IsWorktree {
			grp.rootAgents = append(grp.rootAgents, agent)
		} else {
			wtKey := ""
			if agent.Workspace.RelPath != nil && *agent.Workspace.RelPath != "" {
				wtKey = *agent.Workspace.RelPath
			} else if agent.Workspace.Branch != "" {
				wtKey = agent.Workspace.Branch
			} else {
				wtKey = "unknown"
			}

			var wtGroup *fleetWorktreeGroup
			for _, wt := range grp.worktrees {
				if wt.key == wtKey {
					wtGroup = wt
					break
				}
			}
			if wtGroup == nil {
				relPath := ""
				if agent.Workspace.RelPath != nil {
					relPath = *agent.Workspace.RelPath
				}
				wtGroup = &fleetWorktreeGroup{
					key:     wtKey,
					branch:  agent.Workspace.Branch,
					relPath: relPath,
				}
				grp.worktrees = append(grp.worktrees, wtGroup)
			}
			wtGroup.agents = append(wtGroup.agents, agent)
		}
	}

	return projectGroups
}

// RenderFleetView renders a terminal representation of multiple agent sessions grouped by project and worktree using the current wall clock.
func RenderFleetView(w io.Writer, agents []types.IAgentState, agentMode bool) {
	RenderFleetViewAt(w, agents, agentMode, time.Now().UnixMilli())
}

// RenderFleetViewAt renders a terminal representation of multiple agent sessions with a specified reference timestamp.
func RenderFleetViewAt(w io.Writer, agents []types.IAgentState, agentMode bool, nowMs int64) {
	if len(agents) == 0 {
		if !agentMode {
			_, _ = fmt.Fprintln(w, "No active or recent agent runs detected.")
		}
		return
	}

	if agentMode {
		for _, agent := range agents {
			projID := agent.Project.ID
			if projID == "" {
				projID = "unknown"
			}
			isWt := agent.Workspace.IsWorktree
			wtPath := ""
			if agent.Workspace.RelPath != nil {
				wtPath = *agent.Workspace.RelPath
			}
			var prefixParts []string
			prefixParts = append(prefixParts, fmt.Sprintf("project:%s", projID))
			prefixParts = append(prefixParts, fmt.Sprintf("is_worktree:%t", isWt))
			if isWt && wtPath != "" {
				prefixParts = append(prefixParts, fmt.Sprintf("worktree:%s", wtPath))
			}

			var buf strings.Builder
			renderAgentViewAgentMode(&buf, agent)
			_, _ = fmt.Fprintf(w, "%s %s", strings.Join(prefixParts, " "), buf.String())
		}
		return
	}

	type fleetBranchItem struct {
		header string
		agents []types.IAgentState
	}

	projectGroups := groupAgentsForFleet(agents)
	for grpIndex, grp := range projectGroups {
		if grpIndex > 0 {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintf(w, "▼ %s (%s)\n", grp.name, grp.id)

		var branches []fleetBranchItem
		if len(grp.rootAgents) > 0 {
			rootBranch := grp.rootAgents[0].Workspace.Branch
			if rootBranch == "" {
				rootBranch = "main"
			}
			sessionWord := "sessions"
			if len(grp.rootAgents) == 1 {
				sessionWord = "session"
			}
			branches = append(branches, fleetBranchItem{
				header: fmt.Sprintf("[%s] (%d %s)", rootBranch, len(grp.rootAgents), sessionWord),
				agents: grp.rootAgents,
			})
		}

		for _, wt := range grp.worktrees {
			sessionWord := "sessions"
			if len(wt.agents) == 1 {
				sessionWord = "session"
			}
			branchLabel := wt.branch
			if branchLabel == "" {
				branchLabel = "HEAD"
			}
			var wtHeader string
			if wt.relPath != "" {
				wtHeader = fmt.Sprintf("[worktree: %s] (%s) (%d %s)", branchLabel, wt.relPath, len(wt.agents), sessionWord)
			} else {
				wtHeader = fmt.Sprintf("[worktree: %s] (%d %s)", branchLabel, len(wt.agents), sessionWord)
			}
			branches = append(branches, fleetBranchItem{
				header: wtHeader,
				agents: wt.agents,
			})
		}

		numBranches := len(branches)
		for bIdx, branch := range branches {
			isLast := bIdx == numBranches-1
			branchPrefix := "  ├─ "
			childIndent := "  │    "
			if isLast {
				branchPrefix = "  └─ "
				childIndent = "       "
			}

			_, _ = fmt.Fprintf(w, "%s%s\n", branchPrefix, branch.header)

			for _, agent := range branch.agents {
				title := agent.Activity.Title
				if title == "" {
					title = "Untitled Agent Session"
				}
				title = TrimProjectPrefix(title, grp.name)
				harnessBadge := FormatHarnessBadge(agent.Harness, agent.Activity.Activity)
				activityStr := strings.ToLower(string(agent.Activity.Activity))
				if activityStr == "" {
					activityStr = "idle"
				}
				if agent.Activity.Lifecycle == types.LifecycleRunning && agent.Activity.Activity == types.ActivityIdle && agent.Activity.HasUnseenCompletion != nil && *agent.Activity.HasUnseenCompletion {
					activityStr = "completed"
				}
				_, _ = fmt.Fprintf(w, "%s%s %s (%s)\n", childIndent, harnessBadge, title, activityStr)
			}
		}
	}
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
	isCompleted := agent.Activity.Lifecycle == types.LifecycleRunning && agent.Activity.Activity == types.ActivityIdle && agent.Activity.HasUnseenCompletion != nil && *agent.Activity.HasUnseenCompletion
	if isCompleted {
		activity = "COMPLETED"
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
	if isCompleted {
		parts = append(parts, "has_unseen_completion:true")
	}

	if agent.Activity.Activity == types.ActivityWaitingQuestion && agent.Activity.Question != nil && *agent.Activity.Question != "" {
		parts = append(parts, fmt.Sprintf("question:%q", *agent.Activity.Question))
	} else if agent.Activity.Activity == types.ActivityError && agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
		parts = append(parts, fmt.Sprintf("error:%q", *agent.Activity.LastError))
	} else if agent.Activity.Activity == types.ActivityRateLimited && agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
		parts = append(parts, fmt.Sprintf("rate_limit:%q", *agent.Activity.LastError))
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

	harnessBadge := FormatHarnessBadge(agent.Harness, agent.Activity.Activity)
	hideButton := "[Hide]"
	if agent.IsHidden {
		hideButton = "[Unhide]"
	}
	var buttons []string
	if lifecycle == string(types.LifecycleRunning) {
		buttons = append(buttons, "[Focus]")
	}
	buttons = append(buttons, hideButton)
	headerLeft := fmt.Sprintf("%s %s", harnessBadge, strings.Join(buttons, " "))
	statusLabel := lifecycle
	if agent.Activity.Lifecycle == types.LifecycleRunning && agent.Activity.Activity == types.ActivityIdle && agent.Activity.HasUnseenCompletion != nil && *agent.Activity.HasUnseenCompletion {
		statusLabel = AnsiGreen + "Completed" + AnsiReset
	} else if agent.Activity.Lifecycle == types.LifecycleRunning && agent.Activity.Activity == types.ActivityRateLimited {
		statusLabel = AnsiYellow + "Rate Limited" + AnsiReset
	}
	headerRight := fmt.Sprintf("(%s)", statusLabel)

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

	// 3. Callout (question or error or rate limited)
	var callout string
	if agent.Activity.Activity == types.ActivityWaitingQuestion && agent.Activity.Question != nil && *agent.Activity.Question != "" {
		callout = fmt.Sprintf("%s%s %s%s", AnsiYellow, GlyphQuestion, *agent.Activity.Question, AnsiReset)
	} else if agent.Activity.Activity == types.ActivityError && agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
		callout = fmt.Sprintf("%s%s %s%s", AnsiRed, GlyphAlert, *agent.Activity.LastError, AnsiReset)
	} else if agent.Activity.Activity == types.ActivityRateLimited {
		msg := "Rate limit reached — backoff/retry in progress"
		if agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
			msg = *agent.Activity.LastError
		}
		callout = fmt.Sprintf("%s%s %s%s", AnsiYellow, GlyphWarn, msg, AnsiReset)
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

	// Calculate idle freshness: if session is IDLE and age > 2h (stale), dim output with ANSI Dim
	isStaleIdle := agent.Activity.Activity == types.ActivityIdle && GetIdleFreshness(agent.Activity.UpdatedAt, nowMs) == FreshnessStale && (agent.Activity.HasUnseenCompletion == nil || !*agent.Activity.HasUnseenCompletion)

	printLine := func(line string) {
		if line == "" {
			_, _ = fmt.Fprintln(w)
			return
		}
		if isStaleIdle {
			formatted := strings.ReplaceAll(line, AnsiReset, AnsiReset+AnsiDim)
			_, _ = fmt.Fprintf(w, "%s%s%s\n", AnsiDim, formatted, AnsiReset)
		} else {
			_, _ = fmt.Fprintln(w, line)
		}
	}

	// Print lines
	printLine(divider)
	printLine(headerLine)
	printLine(divider)
	printLine(title)
	printLine(action)
	if callout != "" {
		printLine("")
		printLine(callout)
	}
	printLine("")
	printLine(badgesLine)
	printLine(divider)
	printLine(metricsLine)
	printLine(divider)
}
