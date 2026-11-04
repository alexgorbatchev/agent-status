package classifier

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func (c *Classifier) cancelSubagentTimerLocked(agentID, subID string) {
	key := fmt.Sprintf("%s:%s", agentID, subID)
	if t, exists := c.subagentTimers[key]; exists {
		t.Stop()
		delete(c.subagentTimers, key)
	}
}

func (c *Classifier) cancelAllSubagentTimersLocked(agentID string) {
	prefix := agentID + ":"
	for key, t := range c.subagentTimers {
		if strings.HasPrefix(key, prefix) {
			t.Stop()
			delete(c.subagentTimers, key)
		}
	}
}

func (c *Classifier) scheduleSubagentDwellTimerLocked(agentID, subID string, sub *subagentState, now int64) {
	c.cancelSubagentTimerLocked(agentID, subID)
	if c.subagentDwell <= 0 || c.syncMode {
		return
	}
	dwellMs := int64(c.subagentDwell / time.Millisecond)
	targetTime := sub.startedAt + dwellMs
	delayMs := targetTime - now
	if delayMs <= 0 {
		delayMs = 1
	}
	key := fmt.Sprintf("%s:%s", agentID, subID)
	c.subagentTimers[key] = time.AfterFunc(time.Duration(delayMs)*time.Millisecond, func() {
		c.handleSubagentDwellExpired(agentID, subID)
	})
}

// handleSubagentDwellExpired removes a completed subagent once its minimum
// visibility dwell has passed and saves the re-rolled agent, unless a newer
// frame arrived in the meantime.
func (c *Classifier) handleSubagentDwellExpired(agentID, subID string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	key := fmt.Sprintf("%s:%s", agentID, subID)
	delete(c.subagentTimers, key)
	actx := c.agentCtx[agentID]
	if actx == nil {
		c.mu.Unlock()
		return
	}
	sub, ok := actx.subagents[subID]
	if !ok || sub.completedAt == nil {
		c.mu.Unlock()
		return
	}
	now := time.Now().UnixMilli()
	dwellMs := int64(c.subagentDwell / time.Millisecond)
	targetTime := sub.startedAt + dwellMs
	if diff := targetTime - now; diff > 1 {
		c.subagentTimers[key] = time.AfterFunc(time.Duration(diff)*time.Millisecond, func() {
			c.handleSubagentDwellExpired(agentID, subID)
		})
		c.mu.Unlock()
		return
	}
	scheduledSaveGen := actx.saveGeneration
	hook := c.afterSubagentDwellHook
	c.mu.Unlock()
	if hook != nil {
		defer hook(agentID, subID)
	}

	if c.beforeSubagentDwellSaveHook != nil {
		c.beforeSubagentDwellSaveHook(agentID, subID)
	}

	agent, err := c.persistAgent(agentID, actx, func(agent *types.IAgentState) bool {
		if actx.saveGeneration != scheduledSaveGen {
			return false
		}
		delete(actx.subagents, subID)
		if agent.Activity.Lifecycle == types.LifecycleExited {
			return false
		}
		c.rollupActivityLocked(agent, actx, now)
		captureCompletionLocked(actx, &agent.Activity)
		return true
	})
	if err != nil {
		logging.Error("saving_subagent_dwell_expired_failed", "err", err, "agentId", agentID, "subagentId", subID)
		return
	}
	c.broadcastWrite(agentID, agent, nil, "")
}

func (c *Classifier) pruneExpiredSubagentsLocked(agentID string, actx *agentContext, now int64) {
	if c.subagentDwell <= 0 {
		for id, sub := range actx.subagents {
			if sub.completedAt != nil {
				c.cancelSubagentTimerLocked(agentID, id)
				delete(actx.subagents, id)
			}
		}
		return
	}
	dwellMs := int64(c.subagentDwell / time.Millisecond)
	for id, sub := range actx.subagents {
		if sub.completedAt != nil && now >= sub.startedAt+dwellMs {
			c.cancelSubagentTimerLocked(agentID, id)
			delete(actx.subagents, id)
		}
	}
}

func (c *Classifier) rollupActivityLocked(agent *types.IAgentState, actx *agentContext, now int64) {
	// Build ActiveSubagents summary list
	var summaries []types.ISubagentSummary
	for _, sub := range actx.subagents {
		summaries = append(summaries, types.ISubagentSummary{
			ID:          sub.id,
			Activity:    sub.activity,
			ActiveTool:  sub.activeTool,
			Action:      sub.action,
			LastError:   sub.lastError,
			Status:      sub.status,
			StartedAt:   sub.startedAt,
			CompletedAt: sub.completedAt,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ID < summaries[j].ID
	})
	agent.Activity.ActiveSubagents = summaries

	if len(actx.subagents) == 0 {
		if agent.Activity.Activity != types.ActivityRateLimited {
			clearRateLimit(&agent.Activity)
		}
		return
	}

	var subError *types.ISubagentSummary
	var subQuestion *types.ISubagentSummary
	var subRateLimited *types.ISubagentSummary
	var subBusy *types.ISubagentSummary

	for i := range summaries {
		s := &summaries[i]
		if s.Activity == types.ActivityError && subError == nil {
			subError = s
		}
		if s.Activity == types.ActivityWaitingQuestion && subQuestion == nil {
			subQuestion = s
		}
		if s.Activity == types.ActivityRateLimited && subRateLimited == nil {
			subRateLimited = s
		}
		if s.Activity == types.ActivityBusy && subBusy == nil {
			subBusy = s
		}
	}

	// 1. Error Precedence
	if actx.rootActivity == types.ActivityError {
		agent.Activity.Activity = types.ActivityError
		agent.Activity.LastError = actx.rootLastError
		agent.Activity.HasUnseenCompletion = nil
		clearRateLimit(&agent.Activity)
		return
	} else if subError != nil {
		agent.Activity.Activity = types.ActivityError
		agent.Activity.HasUnseenCompletion = nil
		clearRateLimit(&agent.Activity)
		if subError.LastError != nil {
			errStr := fmt.Sprintf("[Subagent %s] %s", subError.ID, *subError.LastError)
			agent.Activity.LastError = &errStr
		}
		return
	} else {
		agent.Activity.LastError = actx.rootLastError
	}

	// 2. User Input Precedence
	if actx.rootActivity == types.ActivityWaitingQuestion {
		agent.Activity.Activity = types.ActivityWaitingQuestion
		agent.Activity.Question = actx.rootQuestion
		agent.Activity.HasUnseenCompletion = nil
		clearRateLimit(&agent.Activity)
		return
	} else if subQuestion != nil {
		agent.Activity.Activity = types.ActivityWaitingQuestion
		agent.Activity.HasUnseenCompletion = nil
		clearRateLimit(&agent.Activity)
		if sub, ok := actx.subagents[subQuestion.ID]; ok && sub.question != nil {
			qStr := fmt.Sprintf("[Subagent %s] %s", subQuestion.ID, *sub.question)
			agent.Activity.Question = &qStr
		}
		return
	} else {
		agent.Activity.Question = actx.rootQuestion
	}

	// 3. Rate Limited Precedence
	if actx.rootActivity == types.ActivityRateLimited {
		agent.Activity.Activity = types.ActivityRateLimited
		agent.Activity.HasUnseenCompletion = nil
		agent.Activity.RateLimitResetAt = actx.rootRateLimitResetAt
		agent.Activity.RetryAfterSeconds = actx.rootRetryAfterSeconds
		return
	} else if subRateLimited != nil {
		agent.Activity.Activity = types.ActivityRateLimited
		agent.Activity.HasUnseenCompletion = nil
		if subRateLimited.LastError != nil {
			errStr := fmt.Sprintf("[Subagent %s] %s", subRateLimited.ID, *subRateLimited.LastError)
			agent.Activity.LastError = &errStr
		}
		if sub, ok := actx.subagents[subRateLimited.ID]; ok {
			agent.Activity.RateLimitResetAt = sub.rateLimitResetAt
			agent.Activity.RetryAfterSeconds = sub.retryAfterSeconds
		}
		return
	} else {
		if agent.Activity.Activity == types.ActivityRateLimited {
			clearRateLimit(&agent.Activity)
		}
	}

	// 3. Busy vs Idle
	if actx.rootActivity == types.ActivityBusy {
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.Action = actx.rootAction
		agent.Activity.HasUnseenCompletion = nil
		clearRateLimit(&agent.Activity)
	} else if subBusy != nil {
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.HasUnseenCompletion = nil
		clearRateLimit(&agent.Activity)
		if subBusy.ActiveTool != nil && *subBusy.ActiveTool != "" {
			agent.Activity.Action = fmt.Sprintf("Running subagent %s: %s", subBusy.ID, *subBusy.ActiveTool)
		} else if subBusy.Action != "" {
			agent.Activity.Action = fmt.Sprintf("Running subagent %s: %s", subBusy.ID, subBusy.Action)
		} else {
			agent.Activity.Action = fmt.Sprintf("Running subagent %s task", subBusy.ID)
		}
	} else {
		// Root is idle and all subagents are idle
		if agent.Activity.Activity == types.ActivityBusy {
			agent.Activity.HasUnseenCompletion = boolPtr(true)
			agent.Activity.CompletedAt = int64Ptr(now)
		}
		agent.Activity.Activity = types.ActivityIdle
		clearRateLimit(&agent.Activity)
		if actx.rootAction != "" && actx.rootAction != "Thinking" {
			agent.Activity.Action = actx.rootAction
		} else {
			agent.Activity.Action = "Idle"
		}
	}

	if agent.Activity.Activity != types.ActivityRateLimited {
		clearRateLimit(&agent.Activity)
	}
}

// applySubagentEventLocked updates one subagent from a frame and reports whether the frame introduced it.
func (c *Classifier) applySubagentEventLocked(agentID string, actx *agentContext, frame types.ITelemetryFrame, now int64) bool {
	isNewSubagent := false
	if frame.SubagentID != nil && *frame.SubagentID != "" {
		subID := *frame.SubagentID
		if actx.seenSubagents == nil {
			actx.seenSubagents = make(map[string]struct{})
		}
		if _, seen := actx.seenSubagents[subID]; !seen {
			actx.seenSubagents[subID] = struct{}{}
			isNewSubagent = true
		}
		if actx.subagents == nil {
			actx.subagents = make(map[string]*subagentState)
		}
		sub, ok := actx.subagents[subID]
		if !ok {
			sub = &subagentState{
				id:        subID,
				activity:  types.ActivityBusy,
				startedAt: now,
				status:    types.SubagentRunning,
			}
			actx.subagents[subID] = sub
		} else {
			if sub.startedAt == 0 {
				sub.startedAt = now
			}
			if frame.EventType != "turn_end" && frame.EventType != "run_exited" {
				sub.completedAt = nil
				sub.status = types.SubagentRunning
				c.cancelSubagentTimerLocked(agentID, subID)
			}
		}

		switch frame.EventType {
		case "tool_call":
			sub.completedAt = nil
			sub.status = types.SubagentRunning
			c.cancelSubagentTimerLocked(agentID, subID)
			if frame.Data.ToolName != nil && *frame.Data.ToolName == "AskUserQuestion" {
				sub.activity = types.ActivityWaitingQuestion
				sub.activeTool = nil
				if frame.Data.Question != nil && strings.TrimSpace(*frame.Data.Question) != "" {
					sub.question = frame.Data.Question
				} else if frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != "" {
					sub.question = frame.Data.Content
				} else if frame.Data.ToolInput != nil {
					if qVal, ok := frame.Data.ToolInput["question"]; ok && qVal != nil {
						if s, ok := qVal.(string); ok && strings.TrimSpace(s) != "" {
							qStr := strings.TrimSpace(s)
							sub.question = &qStr
						}
					} else if pVal, ok := frame.Data.ToolInput["prompt"]; ok && pVal != nil {
						if s, ok := pVal.(string); ok && strings.TrimSpace(s) != "" {
							pStr := strings.TrimSpace(s)
							sub.question = &pStr
						}
					}
				}
				sub.lastError = nil
			} else {
				sub.activity = types.ActivityBusy
				sub.activeTool = frame.Data.ToolName
				sub.inFlightTool = true
				if frame.Data.ToolName != nil {
					sub.action = "Running " + *frame.Data.ToolName
				}
				sub.lastError = nil
				sub.question = nil
				sub.rateLimitResetAt = nil
				sub.retryAfterSeconds = nil
			}
		case "tool_result":
			sub.inFlightTool = false
			sub.activity = types.ActivityBusy
			sub.activeTool = nil
			sub.lastError = nil
			sub.rateLimitResetAt = nil
			sub.retryAfterSeconds = nil
		case "error":
			sub.inFlightTool = false
			if isRateLimitFrame(frame.Data) {
				sub.activity = types.ActivityRateLimited
				sub.rateLimitResetAt = frame.Data.RateLimitResetAt
				sub.retryAfterSeconds = frame.Data.RetryAfterSeconds
				dur := c.rateLimitSilenceDurationLocked(frame.Data.RateLimitResetAt, frame.Data.RetryAfterSeconds, now)
				c.scheduleSilenceTimerLocked(agentID, actx.rootGeneration, dur)
			} else {
				sub.activity = types.ActivityError
				sub.rateLimitResetAt = nil
				sub.retryAfterSeconds = nil
			}
			if frame.Data.Content != nil {
				out := truncateRunes(*frame.Data.Content, 2000)
				sub.lastError = &out
			} else if frame.Data.ToolOutput != nil {
				out := truncateRunes(*frame.Data.ToolOutput, 2000)
				sub.lastError = &out
			}
			sub.activeTool = nil
		case "question_prompt", "waiting_question":
			sub.inFlightTool = false
			sub.activity = types.ActivityWaitingQuestion
			sub.rateLimitResetAt = nil
			sub.retryAfterSeconds = nil
			if frame.Data.Question != nil && strings.TrimSpace(*frame.Data.Question) != "" {
				q := strings.TrimSpace(*frame.Data.Question)
				sub.question = &q
			} else if frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != "" {
				q := strings.TrimSpace(*frame.Data.Content)
				sub.question = &q
			}
			sub.activeTool = nil
		case "turn_start":
			sub.activity = types.ActivityBusy
			sub.activeTool = nil
			sub.lastError = nil
			sub.rateLimitResetAt = nil
			sub.retryAfterSeconds = nil
			sub.completedAt = nil
			sub.status = types.SubagentRunning
			c.cancelSubagentTimerLocked(agentID, subID)
		case "turn_end", "run_exited":
			sub.activity = types.ActivityIdle
			sub.rateLimitResetAt = nil
			sub.retryAfterSeconds = nil
			sub.activeTool = nil
			sub.inFlightTool = false
			if sub.completedAt == nil {
				comp := now
				sub.completedAt = &comp
			}
			sub.status = types.SubagentCompleted
			c.scheduleSubagentDwellTimerLocked(agentID, subID, sub, now)
		}
	}
	return isNewSubagent
}
