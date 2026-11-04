package classifier

import (
	"fmt"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

// applyRootEvent runs the root agent state machine for one frame and returns any summary job to enqueue after the frame is saved.
func (c *Classifier) applyRootEvent(agent *types.IAgentState, actx *agentContext, frame types.ITelemetryFrame, now int64) (*summaryJob, error) {
	var pendingJob *summaryJob
	switch frame.EventType {
	case "run_started":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		c.cancelSilenceTimerLocked(agent.ID)
		actx.inFlightTool = false
		actx.inFlightToolName = ""
		actx.inTurn = false
		actx.turnFloorUntil = 0
		c.mu.Unlock()

		// Handle run_started idempotently: if an active run already exists with matching PID and unended lifecycle,
		// preserve existing run ID, StartedAt, and do not re-trigger action summarization.
		if agent.Activity.CurrentRun != nil &&
			agent.Activity.CurrentRun.EndedAt == nil &&
			agent.Activity.Lifecycle == types.LifecycleRunning &&
			(agent.Activity.CurrentRun.PID == frame.PID || frame.PID == 0 || agent.Activity.CurrentRun.PID == 0) {
			if agent.Activity.CurrentRun.PID == 0 && frame.PID != 0 {
				agent.Activity.CurrentRun.PID = frame.PID
			}
			break
		}

		agent.Activity.Lifecycle = types.LifecycleRunning
		agent.Activity.Activity = types.ActivityIdle
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.CompletedAt = nil
		runID := fmt.Sprintf("%s:%d:%d", agent.ID, frame.PID, now)
		agent.Activity.CurrentRun = &types.IRunInfo{
			ID:             runID,
			PID:            frame.PID,
			HarnessVersion: frame.HarnessVersion,
			StartedAt:      now,
			EndedAt:        nil,
		}
		pendingJob = c.ensureAction(agent)

	case "run_exited":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		c.cancelTrailingTimerLocked(agent.ID)
		c.cancelSilenceTimerLocked(agent.ID)
		c.cancelAllSubagentTimersLocked(agent.ID)
		actx.inFlightTool = false
		actx.inFlightToolName = ""
		actx.inTurn = false
		actx.turnFloorUntil = 0
		c.mu.Unlock()

		agent.Activity.Lifecycle = types.LifecycleExited
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.CompletedAt = nil
		agent.Activity.ActiveTool = nil
		agent.Activity.ActiveTarget = nil
		agent.Activity.ActiveToolStartedAt = nil
		agent.Activity.ActiveSubagents = nil
		if err := c.store.EndCurrentRun(agent.ID, now); err != nil {
			return nil, fmt.Errorf("ending current run: %w", err)
		}
		if agent.Activity.CurrentRun != nil {
			agent.Activity.CurrentRun.EndedAt = &now
		}

		c.mu.Lock()
		if w, ok := c.workers[agent.ID]; ok {
			close(w.stop)
			delete(c.workers, agent.ID)
		}
		delete(c.agentCtx, agent.ID)
		c.mu.Unlock()

	case "question_prompt", "waiting_question":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		c.cancelSilenceTimerLocked(agent.ID)
		c.mu.Unlock()
		agent.Activity.Activity = types.ActivityWaitingQuestion
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.CompletedAt = nil
		agent.Activity.ActiveTool = nil
		agent.Activity.ActiveTarget = nil
		agent.Activity.RateLimitResetAt = nil
		agent.Activity.RetryAfterSeconds = nil
		if frame.Data.Question != nil {
			agent.Activity.Question = frame.Data.Question
		}
		pendingJob = c.updateSummarization(agent, true)

	case "error":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		if isRateLimitFrame(frame.Data) {
			dur := c.rateLimitSilenceDurationLocked(frame.Data.RateLimitResetAt, frame.Data.RetryAfterSeconds, now)
			c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration, dur)
		} else {
			c.cancelSilenceTimerLocked(agent.ID)
		}
		c.mu.Unlock()
		if isRateLimitFrame(frame.Data) {
			agent.Activity.Activity = types.ActivityRateLimited
			agent.Activity.RateLimitResetAt = frame.Data.RateLimitResetAt
			agent.Activity.RetryAfterSeconds = frame.Data.RetryAfterSeconds
		} else {
			agent.Activity.Activity = types.ActivityError
			agent.Activity.RateLimitResetAt = nil
			agent.Activity.RetryAfterSeconds = nil
		}
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.CompletedAt = nil
		agent.Activity.ActiveTool = nil
		agent.Activity.ActiveTarget = nil
		agent.Activity.ActiveToolStartedAt = nil
		if frame.Data.Content != nil {
			errStr := truncateRunes(*frame.Data.Content, 2000)
			agent.Activity.LastError = &errStr
		}
		pendingJob = c.updateSummarization(agent, true)

	case "tool_result":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		actx.inFlightTool = false
		actx.inFlightToolName = ""
		c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
		c.mu.Unlock()
		// Tool results (even non-zero exits) are a normal part of shell/agent workflows;
		// the agent remains busy processing the result, not in an error state.
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.RateLimitResetAt = nil
		agent.Activity.RetryAfterSeconds = nil
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.LastError = nil
		agent.Activity.ActiveTool = nil
		agent.Activity.ActiveTarget = nil
		agent.Activity.ActiveToolStartedAt = nil
		pendingJob = c.updateSummarization(agent, false)

	case "tool_call":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		actx.inFlightTool = true
		actx.turnFloorUntil = 0
		if frame.Data.ToolName != nil {
			actx.inFlightToolName = *frame.Data.ToolName
		}
		c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
		c.mu.Unlock()
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.CompletedAt = nil
		agent.Activity.LastError = nil
		agent.Activity.ActiveToolStartedAt = &now
		if frame.Data.ToolName != nil {
			toolName := *frame.Data.ToolName
			agent.Activity.ActiveTool = &toolName

			var target string
			if frame.Data.FileModification != nil && frame.Data.FileModification.FilePath != "" {
				target = frame.Data.FileModification.FilePath
			} else {
				target = extractTarget(toolName, frame.Data.ToolInput)
			}
			if target != "" {
				agent.Activity.ActiveTarget = &target
			}

			sanitizedArgs := summarizer.SanitizeToolInput(frame.Data.ToolInput, summarizer.MaxToolArgsLength)

			c.mu.Lock()
			actx.lastToolArgs = sanitizedArgs
			actx.recentTools = append(actx.recentTools, summarizer.ToolExecution{
				ToolName: toolName,
				Target:   target,
			})
			if len(actx.recentTools) > 3 {
				actx.recentTools = actx.recentTools[len(actx.recentTools)-3:]
			}
			c.mu.Unlock()

			if toolName == "AskUserQuestion" {
				agent.Activity.Activity = types.ActivityWaitingQuestion
				agent.Activity.HasUnseenCompletion = nil
				agent.Activity.CompletedAt = nil
				agent.Activity.RateLimitResetAt = nil
				agent.Activity.RetryAfterSeconds = nil
				if frame.Data.Question != nil {
					agent.Activity.Question = frame.Data.Question
				}
				pendingJob = c.updateSummarization(agent, true)
			} else {
				// Continuous tool activity: summarize if debounce allows, otherwise mark stale
				pendingJob = c.updateSummarization(agent, false)
			}
		}

	case "turn_start":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		c.cancelTrailingTimerLocked(agent.ID)
		actx.lastAssistantResponse = ""
		actx.lastToolArgs = ""
		actx.recentTools = nil
		actx.lastSummarizedToolKey = ""
		actx.pendingCandidate = nil
		actx.inFlightTool = false
		actx.inFlightToolName = ""
		actx.inTurn = true
		actx.turnFloorUntil = now + c.turnFloor.Milliseconds()
		c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
		c.mu.Unlock()
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.CompletedAt = nil
		agent.Activity.LastError = nil
		agent.Activity.RateLimitResetAt = nil
		agent.Activity.RetryAfterSeconds = nil
		agent.Activity.ActiveTool = nil
		agent.Activity.ActiveTarget = nil
		agent.Activity.ActiveToolStartedAt = nil
		agent.Activity.Action = "Thinking"
		p := types.PhasePlanning
		agent.Activity.Phase = &p

	case "thinking":
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		hasRecentTools := len(actx.recentTools) > 0
		c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
		c.mu.Unlock()
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.CompletedAt = nil
		agent.Activity.LastError = nil
		agent.Activity.RateLimitResetAt = nil
		agent.Activity.RetryAfterSeconds = nil
		if frame.Data.Content != nil {
			agent.Activity.LastThinkingExcerpt = frame.Data.Content
		}
		// Action = "Thinking" is only for initial turn start before tools have run
		if !hasRecentTools && agent.Activity.ActiveTool == nil && (agent.Activity.Action == "" || agent.Activity.Action == "Thinking") {
			agent.Activity.Action = "Thinking"
			p := types.PhasePlanning
			agent.Activity.Phase = &p
		}

	case "turn_end":
		if frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != "" {
			c.mu.Lock()
			actx.lastAssistantResponse = strings.TrimSpace(*frame.Data.Content)
			c.mu.Unlock()
		}

		c.mu.Lock()
		inFlight := actx.inFlightTool
		inTurn := actx.inTurn
		floorUntil := actx.turnFloorUntil
		c.mu.Unlock()

		isToolUse := false
		if frame.Data.StopReason != nil && *frame.Data.StopReason != "" {
			isToolUse = isToolUseStopReason(*frame.Data.StopReason)
		} else if inFlight || agent.Activity.ActiveTool != nil {
			isToolUse = true
		}

		hasTerminalContent := frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != ""
		isTerminalStop := frame.Data.StopReason != nil && (*frame.Data.StopReason == "end_turn" || *frame.Data.StopReason == "stop")

		if isToolUse {
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.cancelTrailingTimerLocked(agent.ID)
			c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
			c.mu.Unlock()
			if agent.Activity.Activity != types.ActivityWaitingQuestion {
				agent.Activity.Activity = types.ActivityBusy
				agent.Activity.HasUnseenCompletion = nil
			}
			pendingJob = c.updateSummarization(agent, true)
		} else if agent.Activity.Activity == types.ActivityWaitingQuestion {
			// Do not overwrite WAITING_QUESTION or clear Question prompt on turn_end
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.cancelTrailingTimerLocked(agent.ID)
			c.cancelSilenceTimerLocked(agent.ID)
			c.mu.Unlock()
			pendingJob = c.updateSummarization(agent, true)
		} else if agent.Activity.Activity == types.ActivityError {
			// Do not overwrite ERROR state on turn_end
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.cancelTrailingTimerLocked(agent.ID)
			c.cancelSilenceTimerLocked(agent.ID)
			c.mu.Unlock()
			pendingJob = c.updateSummarization(agent, true)
		} else if agent.Activity.Activity == types.ActivityRateLimited && !isTerminalStop && !hasTerminalContent {
			// Do not overwrite RATE_LIMITED state on turn_end, and reschedule silence timer
			resetAt := agent.Activity.RateLimitResetAt
			if frame.Data.RateLimitResetAt != nil {
				resetAt = frame.Data.RateLimitResetAt
				agent.Activity.RateLimitResetAt = frame.Data.RateLimitResetAt
			}
			retrySec := agent.Activity.RetryAfterSeconds
			if frame.Data.RetryAfterSeconds != nil {
				retrySec = frame.Data.RetryAfterSeconds
				agent.Activity.RetryAfterSeconds = frame.Data.RetryAfterSeconds
			}
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.cancelTrailingTimerLocked(agent.ID)
			dur := c.rateLimitSilenceDurationLocked(resetAt, retrySec, now)
			c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration, dur)
			c.mu.Unlock()
			pendingJob = c.updateSummarization(agent, true)
		} else {
			hasToolsRun := len(actx.recentTools) > 0
			isTrulyFinished := hasTerminalContent || isTerminalStop || hasToolsRun

			isWithinTurnFloor := inTurn && now < floorUntil && !isTrulyFinished

			if isWithinTurnFloor {
				agent.Activity.Activity = types.ActivityBusy
				agent.Activity.HasUnseenCompletion = nil
				agent.Activity.Action = "Thinking"
				p := types.PhasePlanning
				agent.Activity.Phase = &p
				c.mu.Lock()
				c.cancelIdleTimerLocked(agent.ID)
				c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
				c.mu.Unlock()
				pendingJob = c.updateSummarization(agent, false)
			} else {
				// Turn ended without tool use.
				// If the agent is BUSY and an idle grace period is configured, debounce the transition
				// so pauses between API calls do not cause BUSY/IDLE flipping.
				agent.Activity.ActiveTool = nil
				agent.Activity.ActiveTarget = nil
				agent.Activity.ActiveToolStartedAt = nil
				agent.Activity.Question = nil

				c.mu.Lock()
				actx.inTurn = false
				actx.turnFloorUntil = 0
				actx.lastToolArgs = ""
				actx.recentTools = nil
				actx.lastSummarizedToolKey = ""
				actx.pendingCandidate = nil
				c.cancelTrailingTimerLocked(agent.ID)
				c.cancelSilenceTimerLocked(agent.ID)
				gracePeriod := c.idleGracePeriod
				isSync := c.syncMode
				c.mu.Unlock()

				if gracePeriod <= 0 || isSync || agent.Activity.Activity != types.ActivityBusy {
					c.mu.Lock()
					c.cancelIdleTimerLocked(agent.ID)
					actx.turnEndTimestamp = now
					c.mu.Unlock()
					if agent.Activity.Activity == types.ActivityBusy {
						agent.Activity.HasUnseenCompletion = boolPtr(true)
						agent.Activity.CompletedAt = int64Ptr(now)
					}
					agent.Activity.Activity = types.ActivityIdle
					agent.Activity.RateLimitResetAt = nil
					agent.Activity.RetryAfterSeconds = nil
					pendingJob = c.updateSummarization(agent, true)
				} else {
					// Keep agent BUSY during grace period and schedule transition to IDLE if no new events arrive.
					agent.Activity.Activity = types.ActivityBusy
					agent.Activity.RateLimitResetAt = nil
					agent.Activity.RetryAfterSeconds = nil
					c.mu.Lock()
					actx.turnEndTimestamp = now
					c.scheduleIdleTransitionLocked(agent.ID, actx.rootGeneration, gracePeriod)
					c.mu.Unlock()
					pendingJob = c.updateSummarization(agent, false)
				}
			}
		}
	}
	return pendingJob, nil
}
