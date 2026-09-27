package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
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
	cleanLabel := SanitizeTranscriptText(rawLabel)

	switch activity {
	case types.ActivityError:
		return fmt.Sprintf("%s(%s)%s", AnsiRed, cleanLabel, AnsiReset)
	case types.ActivityWaitingQuestion, types.ActivityRateLimited:
		return fmt.Sprintf("%s(%s)%s", AnsiYellow, cleanLabel, AnsiReset)
	default:
		brandColor := SanitizeBrandColor(GetHarnessBrandColor(h))
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
	w = wrapWriter(w)
	if agentMode {
		renderAgentViewAgentMode(w, agent, nowMs)
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
	id           string
	name         string
	maxUpdatedAt int64
	rootAgents   []types.IAgentState
	worktrees    []*fleetWorktreeGroup
}

func agentSortRank(agent types.IAgentState) int {
	if agent.Activity.Lifecycle == types.LifecycleExited {
		return 2
	}
	if agent.Activity.Activity == types.ActivityIdle {
		return 1
	}
	return 0
}

func sortFleetAgents(agents []types.IAgentState) {
	sort.SliceStable(agents, func(i, j int) bool {
		rankI := agentSortRank(agents[i])
		rankJ := agentSortRank(agents[j])
		if rankI != rankJ {
			return rankI < rankJ
		}
		return agents[i].Activity.UpdatedAt > agents[j].Activity.UpdatedAt
	})
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
				id:           projID,
				name:         projName,
				maxUpdatedAt: agent.Activity.UpdatedAt,
			}
			projectMap[projID] = grp
			projectGroups = append(projectGroups, grp)
		} else if agent.Activity.UpdatedAt > grp.maxUpdatedAt {
			grp.maxUpdatedAt = agent.Activity.UpdatedAt
		}

		if !agent.Workspace.IsWorktree {
			grp.rootAgents = append(grp.rootAgents, agent)
		} else {
			wtKey := ""
			if agent.Workspace.RelPath != nil && *agent.Workspace.RelPath != "" {
				wtKey = *agent.Workspace.RelPath
			} else if agent.Workspace.Branch != "" {
				wtKey = agent.Workspace.Branch
			} else if agent.Workspace.CWD != "" {
				wtKey = agent.Workspace.CWD
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

	for _, grp := range projectGroups {
		sortFleetAgents(grp.rootAgents)
		for _, wt := range grp.worktrees {
			sortFleetAgents(wt.agents)
		}
	}

	sort.SliceStable(projectGroups, func(i, j int) bool {
		return projectGroups[i].maxUpdatedAt > projectGroups[j].maxUpdatedAt
	})

	return projectGroups
}

// RenderFleetView renders a terminal representation of multiple agent sessions grouped by project and worktree using the current wall clock.
func RenderFleetView(w io.Writer, agents []types.IAgentState, agentMode bool) {
	RenderFleetViewAt(w, agents, agentMode, time.Now().UnixMilli())
}

// RenderFleetViewAt renders a terminal representation of multiple agent sessions with a specified reference timestamp.
func RenderFleetViewAt(w io.Writer, agents []types.IAgentState, agentMode bool, nowMs int64) {
	w = wrapWriter(w)
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
			var prefixParts []string
			prefixParts = append(prefixParts, fmt.Sprintf("project:%s", SanitizeTranscriptText(projID)))
			prefixParts = append(prefixParts, fmt.Sprintf("is_worktree:%t", isWt))

			var buf strings.Builder
			renderAgentViewAgentMode(&buf, agent, nowMs)
			_, _ = fmt.Fprintf(w, "%s %s", strings.Join(prefixParts, " "), buf.String())
		}
		return
	}

	width := GetTerminalWidth()

	type fleetBranchItem struct {
		header string
		agents []types.IAgentState
	}

	projectGroups := groupAgentsForFleet(agents)
	for grpIndex, grp := range projectGroups {
		if grpIndex > 0 {
			_, _ = fmt.Fprintln(w)
		}
		grpName := SanitizeTranscriptText(grp.name)
		grpID := SanitizeTranscriptText(grp.id)
		projLine := TruncateVisual(fmt.Sprintf("v %s (%s)", grpName, grpID), width, "...")
		_, _ = fmt.Fprintln(w, projLine)

		if len(grp.worktrees) == 0 {
			for _, agent := range grp.rootAgents {
				renderFleetAgentLine(w, agent, grpName, "  ", width, nowMs)
			}
			continue
		}

		var branches []fleetBranchItem
		if len(grp.rootAgents) > 0 {
			rootBranch := SanitizeTranscriptText(grp.rootAgents[0].Workspace.Branch)
			if rootBranch == "" {
				rootBranch = "HEAD"
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
			branchLabel := SanitizeTranscriptText(wt.branch)
			if branchLabel == "" {
				branchLabel = "HEAD"
			}
			cleanRelPath := SanitizeTranscriptText(wt.relPath)
			var wtHeader string
			if cleanRelPath != "" {
				wtHeader = fmt.Sprintf("[worktree: %s] (%s) (%d %s)", branchLabel, cleanRelPath, len(wt.agents), sessionWord)
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
			branchPrefix := "  |- "
			childIndent := "  |    "
			if isLast {
				branchPrefix = "  +- "
				childIndent = "       "
			}

			branchLine := TruncateVisual(fmt.Sprintf("%s%s", branchPrefix, branch.header), width, "...")
			_, _ = fmt.Fprintln(w, branchLine)

			for _, agent := range branch.agents {
				renderFleetAgentLine(w, agent, grpName, childIndent, width, nowMs)
			}
		}
	}
}

func renderFleetAgentLine(w io.Writer, agent types.IAgentState, grpName string, childIndent string, width int, nowMs int64) {
	title := SanitizeTranscriptText(agent.Activity.Title)
	if title == "" {
		title = "Untitled Agent Session"
	}
	title = TrimProjectPrefix(title, grpName)
	title = strings.ReplaceAll(title, "\n", " ")
	harnessBadge := FormatHarnessBadge(agent.Harness, agent.Activity.Activity)
	statusStr := FormatActivityStatus(agent.Activity.Lifecycle, agent.Activity.Activity, agent.Activity.HasUnseenCompletion)
	line := fmt.Sprintf("%s%s %s (%s)", childIndent, harnessBadge, title, statusStr)
	line = TruncateVisual(line, width, "...")
	isStaleIdle := agent.Activity.Lifecycle == types.LifecycleRunning && agent.Activity.Activity == types.ActivityIdle && GetIdleFreshness(agent.Activity.UpdatedAt, nowMs) == FreshnessStale && (agent.Activity.HasUnseenCompletion == nil || !*agent.Activity.HasUnseenCompletion)
	if isStaleIdle {
		formatted := strings.ReplaceAll(line, AnsiReset, AnsiReset+AnsiDim)
		_, _ = fmt.Fprintf(w, "%s%s%s\n", AnsiDim, formatted, AnsiReset)
	} else {
		_, _ = fmt.Fprintln(w, line)
	}
}

func formatHumanDuration(durationMs int64) string {
	if durationMs < 60000 {
		seconds := durationMs / 1000
		return fmt.Sprintf("%ds", seconds)
	}
	hours := durationMs / 3600000
	mins := (durationMs % 3600000) / 60000
	secs := (durationMs % 60000) / 1000

	if hours > 0 {
		if mins > 0 {
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if secs > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	return fmt.Sprintf("%dm", mins)
}

func formatRateLimitWait(activity types.IAgentActivity, nowMs int64) string {
	if activity.RateLimitResetAt != nil {
		if *activity.RateLimitResetAt > nowMs {
			remainingSec := (*activity.RateLimitResetAt - nowMs + 999) / 1000
			if remainingSec > 120 {
				return fmt.Sprintf("retry in %s", formatHumanDuration(remainingSec*1000))
			}
			return fmt.Sprintf("retry in %ds", remainingSec)
		}
		return ""
	}
	if activity.RetryAfterSeconds != nil && *activity.RetryAfterSeconds > 0 {
		rem := *activity.RetryAfterSeconds - (nowMs-activity.UpdatedAt)/1000
		if rem > 0 {
			if rem > 120 {
				return fmt.Sprintf("retry in %s", formatHumanDuration(rem*1000))
			}
			return fmt.Sprintf("retry in %ds", rem)
		}
	}
	return ""
}

func renderAgentViewAgentMode(w io.Writer, agent types.IAgentState, nowMs int64) {
	harness := SanitizeTranscriptText(string(agent.Harness))
	if harness == "" {
		harness = "unknown"
	}
	env := SanitizeTranscriptText(agent.Environment.Label)
	if env == "" {
		env = SanitizeTranscriptText(agent.Environment.ID)
	}
	if env == "" {
		env = "local-dev"
	}
	title := SanitizeTranscriptText(agent.Activity.Title)
	if title == "" {
		title = "Untitled Agent Session"
	}
	action := SanitizeTranscriptText(agent.Activity.Action)
	if action == "" {
		if agent.Activity.ActiveTool != nil && *agent.Activity.ActiveTool != "" {
			action = fmt.Sprintf("Executing %s", SanitizeTranscriptText(*agent.Activity.ActiveTool))
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
	branch := SanitizeTranscriptText(agent.Workspace.Branch)
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
		parts = append(parts, fmt.Sprintf("question:%q", SanitizeTranscriptText(*agent.Activity.Question)))
	} else if agent.Activity.Activity == types.ActivityError && agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
		parts = append(parts, fmt.Sprintf("error:%q", SanitizeTranscriptText(*agent.Activity.LastError)))
	} else if agent.Activity.Activity == types.ActivityRateLimited {
		msg := "Rate limit reached — backoff/retry in progress"
		if agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
			msg = SanitizeTranscriptText(*agent.Activity.LastError)
		}
		waitStr := formatRateLimitWait(agent.Activity, nowMs)
		if waitStr != "" && !strings.Contains(msg, waitStr) {
			if agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
				msg = fmt.Sprintf("%s (%s)", msg, waitStr)
			} else {
				msg = fmt.Sprintf("Rate limit reached — %s", waitStr)
			}
		}
		parts = append(parts, fmt.Sprintf("rate_limit:%q", msg))
	}

	parts = append(parts, fmt.Sprintf("lifecycle:%s", lifecycle))
	parts = append(parts, fmt.Sprintf("branch:%s", branch))

	if agent.Workspace.IsWorktree && agent.Workspace.RelPath != nil && *agent.Workspace.RelPath != "" {
		parts = append(parts, fmt.Sprintf("worktree:%s", SanitizeTranscriptText(*agent.Workspace.RelPath)))
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
	} else if agent.Activity.Activity == types.ActivityRateLimited {
		statusLabel = AnsiYellow + "Rate Limited" + AnsiReset
	}
	headerRight := fmt.Sprintf("(%s)", statusLabel)

	headerSpace := width - VisualWidth(headerLeft) - VisualWidth(headerRight)
	if headerSpace < 1 {
		availLeft := width - VisualWidth(headerRight) - 1
		if availLeft > 0 {
			headerLeft = TruncateVisual(headerLeft, availLeft, "...")
		}
		headerSpace = width - VisualWidth(headerLeft) - VisualWidth(headerRight)
		if headerSpace < 1 {
			headerSpace = 1
		}
	}
	headerLine := headerLeft + strings.Repeat(" ", headerSpace) + headerRight
	if VisualWidth(headerLine) > width {
		headerLine = TruncateVisual(headerLine, width, "...")
	}

	// 2. Title & Action
	title := SanitizeTranscriptText(agent.Activity.Title)
	if title == "" {
		title = "Untitled Agent Session"
	}
	title = strings.ReplaceAll(title, "\n", " ")
	title = TruncateVisual(title, width, "...")
	action := SanitizeTranscriptText(agent.Activity.Action)
	if action == "" {
		if agent.Activity.ActiveTool != nil && *agent.Activity.ActiveTool != "" {
			action = fmt.Sprintf("Executing %s", SanitizeTranscriptText(*agent.Activity.ActiveTool))
		} else {
			action = "Idle"
		}
	}
	action = strings.ReplaceAll(action, "\n", " ")
	action = TruncateVisual(action, width, "...")

	// 3. Callout (question or error or rate limited)
	var callout string
	if agent.Activity.Activity == types.ActivityWaitingQuestion && agent.Activity.Question != nil && *agent.Activity.Question != "" {
		callout = fmt.Sprintf("%s%s %s%s", AnsiYellow, GlyphQuestion, SanitizeTranscriptText(*agent.Activity.Question), AnsiReset)
	} else if agent.Activity.Activity == types.ActivityError && agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
		callout = fmt.Sprintf("%s%s %s%s", AnsiRed, GlyphAlert, SanitizeTranscriptText(*agent.Activity.LastError), AnsiReset)
	} else if agent.Activity.Activity == types.ActivityRateLimited {
		msg := "Rate limit reached — backoff/retry in progress"
		if agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
			msg = SanitizeTranscriptText(*agent.Activity.LastError)
		}
		waitStr := formatRateLimitWait(agent.Activity, nowMs)
		if waitStr != "" && !strings.Contains(msg, waitStr) {
			if agent.Activity.LastError != nil && *agent.Activity.LastError != "" {
				msg = fmt.Sprintf("%s (%s)", msg, waitStr)
			} else {
				msg = fmt.Sprintf("Rate limit reached — %s", waitStr)
			}
		}
		callout = fmt.Sprintf("%s%s %s%s", AnsiYellow, GlyphWarn, msg, AnsiReset)
	}
	if callout != "" {
		callout = strings.ReplaceAll(callout, "\n", " ")
		callout = TruncateVisual(callout, width, "...")
	}

	// 4. Badges
	env := SanitizeTranscriptText(agent.Environment.Label)
	if env == "" {
		env = SanitizeTranscriptText(agent.Environment.ID)
	}
	if env == "" {
		env = "local-dev"
	}
	env = strings.ReplaceAll(env, "\n", " ")

	branch := SanitizeTranscriptText(agent.Workspace.Branch)
	if branch == "" {
		branch = "main"
	}
	branch = strings.ReplaceAll(branch, "\n", " ")

	var badges []string
	badges = append(badges, fmt.Sprintf("(%s)", env))
	badges = append(badges, fmt.Sprintf("(branch: %s)", branch))

	if agent.Workspace.IsWorktree && agent.Workspace.RelPath != nil && *agent.Workspace.RelPath != "" {
		cleanRelPath := strings.ReplaceAll(SanitizeTranscriptText(*agent.Workspace.RelPath), "\n", " ")
		badges = append(badges, fmt.Sprintf("(worktree: %s)", cleanRelPath))
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
		subID := strings.ReplaceAll(SanitizeTranscriptText(sub.ID), "\n", " ")
		if sub.ActiveTool != nil && *sub.ActiveTool != "" {
			tool := strings.ReplaceAll(SanitizeTranscriptText(*sub.ActiveTool), "\n", " ")
			subLabel = fmt.Sprintf("%s: %s", subID, tool)
		} else if sub.Action != "" {
			action := strings.ReplaceAll(SanitizeTranscriptText(sub.Action), "\n", " ")
			subLabel = fmt.Sprintf("%s: %s", subID, action)
		} else {
			act := strings.ReplaceAll(SanitizeTranscriptText(string(sub.Activity)), "\n", " ")
			subLabel = fmt.Sprintf("%s: %s", subID, act)
		}
		badges = append(badges, fmt.Sprintf("(subagent %s)", subLabel))
	}
	relTime := FormatRelativeTime(agent.Activity.UpdatedAt, nowMs)
	badges = append(badges, fmt.Sprintf("(active: %s)", relTime))

	for i := range badges {
		badges[i] = strings.ReplaceAll(badges[i], "\n", " ")
	}

	var badgeLines []string
	var currentBadgeLine string
	for _, badge := range badges {
		if currentBadgeLine == "" {
			currentBadgeLine = badge
		} else if VisualWidth(currentBadgeLine)+2+VisualWidth(badge) <= width {
			currentBadgeLine += "  " + badge
		} else {
			badgeLines = append(badgeLines, currentBadgeLine)
			currentBadgeLine = badge
		}
	}
	if currentBadgeLine != "" {
		badgeLines = append(badgeLines, currentBadgeLine)
	}

	// 5. Metrics row
	cost := 0.0
	if agent.Activity.Cost != nil {
		cost = agent.Activity.Cost.Total
	}
	tokens := agent.Activity.Usage.Total
	metricsLeft := fmt.Sprintf("$%.4f USD", cost)
	metricsRight := FormatTokens(tokens)
	metricsSpace := width - VisualWidth(metricsLeft) - VisualWidth(metricsRight)
	if metricsSpace < 1 {
		availLeft := width - VisualWidth(metricsRight) - 1
		if availLeft > 0 {
			metricsLeft = TruncateVisual(metricsLeft, availLeft, "...")
		}
		metricsSpace = width - VisualWidth(metricsLeft) - VisualWidth(metricsRight)
		if metricsSpace < 1 {
			metricsSpace = 1
		}
	}
	metricsLine := metricsLeft + strings.Repeat(" ", metricsSpace) + metricsRight
	if VisualWidth(metricsLine) > width {
		metricsLine = TruncateVisual(metricsLine, width, "...")
	}

	// Calculate idle freshness: if session is IDLE and age > 2h (stale), dim output with ANSI Dim
	isStaleIdle := agent.Activity.Activity == types.ActivityIdle && GetIdleFreshness(agent.Activity.UpdatedAt, nowMs) == FreshnessStale && (agent.Activity.HasUnseenCompletion == nil || !*agent.Activity.HasUnseenCompletion)

	printLine := func(line string) {
		if line == "" {
			_, _ = fmt.Fprintln(w)
			return
		}
		if VisualWidth(line) > width {
			line = TruncateVisual(line, width, "...")
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
	for _, bLine := range badgeLines {
		printLine(bLine)
	}
	printLine(divider)
	printLine(metricsLine)
	printLine(divider)
}
