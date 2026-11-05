package classifier

import (
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func (c *Classifier) rateLimitSilenceDurationLocked(resetAt *int64, retrySec *int64, nowMs int64) time.Duration {
	baseTimeout := c.turnSilenceTimeout
	if baseTimeout <= 0 {
		return 0
	}
	var maxWait time.Duration
	if retrySec != nil && *retrySec > 0 {
		var retryDur time.Duration
		if *retrySec >= int64(24*time.Hour/time.Second) {
			retryDur = 24 * time.Hour
		} else {
			retryDur = time.Duration(*retrySec) * time.Second
		}
		if retryDur > maxWait {
			maxWait = retryDur
		}
	}
	refNow := nowMs
	if refNow <= 0 {
		refNow = time.Now().UnixMilli()
	}
	if resetAt != nil && *resetAt > refNow {
		diffMs := *resetAt - refNow
		var resetDur time.Duration
		if diffMs >= int64(24*time.Hour/time.Millisecond) {
			resetDur = 24 * time.Hour
		} else {
			resetDur = time.Duration(diffMs) * time.Millisecond
		}
		if resetDur > maxWait {
			maxWait = resetDur
		}
	}

	threshold := c.turnSilenceTimeout
	if maxWait > threshold {
		return maxWait + DefaultRateLimitSilenceBuffer
	}
	return baseTimeout
}

func (c *Classifier) cancelIdleTimerLocked(agentID string) {
	if t, exists := c.idleTimers[agentID]; exists {
		t.Stop()
		delete(c.idleTimers, agentID)
	}
}

func (c *Classifier) scheduleIdleTransitionLocked(agentID string, scheduledGen uint64, delay time.Duration) {
	if c.closed {
		return
	}
	c.cancelIdleTimerLocked(agentID)
	c.idleTimers[agentID] = time.AfterFunc(delay, func() {
		c.handleIdleTransition(agentID, scheduledGen)
	})
}

func (c *Classifier) cancelSilenceTimerLocked(agentID string) {
	if t, exists := c.silenceTimers[agentID]; exists {
		t.Stop()
		delete(c.silenceTimers, agentID)
	}
	delete(c.silenceTimerDurations, agentID)
}

func (c *Classifier) scheduleSilenceTimerLocked(agentID string, scheduledGen uint64, customDuration ...time.Duration) {
	c.cancelSilenceTimerLocked(agentID)
	if c.turnSilenceTimeout <= 0 || c.syncMode {
		return
	}
	timeout := c.turnSilenceTimeout
	if len(customDuration) > 0 && customDuration[0] > 0 {
		timeout = customDuration[0]
	}
	if c.silenceTimerDurations != nil {
		c.silenceTimerDurations[agentID] = timeout
	}
	c.silenceTimers[agentID] = time.AfterFunc(timeout, func() {
		c.handleSilenceTransition(agentID, scheduledGen)
	})
}

func (c *Classifier) cancelTrailingTimerLocked(agentID string) {
	if t, exists := c.trailingTimers[agentID]; exists {
		t.Stop()
		delete(c.trailingTimers, agentID)
	}
}

func (c *Classifier) scheduleTrailingTimerLocked(agentID string, delay time.Duration) {
	c.cancelTrailingTimerLocked(agentID)
	if delay <= 0 {
		delay = 1 * time.Millisecond
	}
	c.trailingTimers[agentID] = time.AfterFunc(delay, func() {
		c.flushTrailingCandidate(agentID)
	})
}

// handleIdleTransition ends the turn-end grace period: a root still BUSY from
// the scheduled turn becomes IDLE with an unseen completion. A newer root frame
// cancels the transition; a newer subagent frame postpones it.
func (c *Classifier) handleIdleTransition(agentID string, scheduledGen uint64) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	delete(c.idleTimers, agentID)

	actx, ok := c.agentCtx[agentID]
	if !ok || actx.rootGeneration != scheduledGen || actx.rootActivity != types.ActivityBusy {
		c.mu.Unlock()
		return
	}

	// In-Flight Floor (FlightFloor): do not drop to idle while a tool is in flight
	if actx.inFlightTool {
		c.mu.Unlock()
		return
	}

	// Thinking Floor (TurnFloor): do not drop to idle during turn floor latency window
	if actx.inTurn && time.Now().UnixMilli() < actx.turnFloorUntil {
		c.mu.Unlock()
		return
	}

	scheduledSaveGen := actx.saveGeneration
	completedAtTs := actx.turnEndTimestamp
	if completedAtTs == 0 {
		completedAtTs = actx.lastEventTimestamp
	}
	c.mu.Unlock()

	if c.beforeIdleSummarizeHook != nil {
		c.beforeIdleSummarizeHook(agentID)
	}
	if c.beforeIdleSaveHook != nil {
		c.beforeIdleSaveHook(agentID)
	}

	var job *summaryJob
	agent, err := c.persistAgent(agentID, actx, func(agent *types.IAgentState) bool {
		if actx.rootGeneration != scheduledGen || actx.rootActivity != types.ActivityBusy ||
			agent.Activity.Lifecycle != types.LifecycleRunning {
			return false
		}
		if actx.saveGeneration != scheduledSaveGen || actx.frameInFlight {
			// A subagent frame arrived after scheduling; try again once it settles.
			c.scheduleIdleTransitionLocked(agentID, actx.rootGeneration, frameRetryDelay)
			return false
		}

		now := time.Now().UnixMilli()
		if completedAtTs == 0 {
			completedAtTs = now
		}
		if agent.Activity.Activity == types.ActivityBusy {
			agent.Activity.HasUnseenCompletion = boolPtr(true)
			agent.Activity.CompletedAt = int64Ptr(completedAtTs)
		}
		c.settleIdleLocked(agent, actx, now)
		job = c.summarizeIdleLocked(agent, actx)
		recordRootStateLocked(actx, &agent.Activity)
		c.rollupActivityLocked(agent, actx, now)
		captureCompletionLocked(actx, &agent.Activity)
		return true
	})
	c.finishTransition(agentID, agent, job, err, "saving_idle_agent_failed")
}

// handleSilenceTransition idles an agent that has produced no root frame for the
// silence timeout, whether it was BUSY, rate limited, or had a rate-limited
// subagent, and marks every subagent completed.
func (c *Classifier) handleSilenceTransition(agentID string, scheduledGen uint64) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	delete(c.silenceTimers, agentID)
	delete(c.silenceTimerDurations, agentID)

	actx, ok := c.agentCtx[agentID]
	hasRateLimitedSub := false
	if ok {
		for _, s := range actx.subagents {
			if s.activity == types.ActivityRateLimited {
				hasRateLimitedSub = true
				break
			}
		}
	}
	if !ok || actx.rootGeneration != scheduledGen || (!silenceCanIdle(actx.rootActivity) && !hasRateLimitedSub) {
		c.mu.Unlock()
		return
	}
	scheduledSaveGen := actx.saveGeneration
	scheduledRootActivity := actx.rootActivity
	c.mu.Unlock()

	if c.beforeSilenceSaveHook != nil {
		c.beforeSilenceSaveHook(agentID)
	}

	var job *summaryJob
	agent, err := c.persistAgent(agentID, actx, func(agent *types.IAgentState) bool {
		agentRateLimited := agent.Activity.Activity == types.ActivityRateLimited
		if agent.Activity.Lifecycle != types.LifecycleRunning ||
			(!silenceCanIdle(scheduledRootActivity) && !agentRateLimited) ||
			actx.rootGeneration != scheduledGen || actx.saveGeneration != scheduledSaveGen || actx.frameInFlight ||
			(!silenceCanIdle(actx.rootActivity) && !agentRateLimited) {
			return false
		}

		now := time.Now().UnixMilli()
		clearCompletion(&agent.Activity)
		c.settleIdleLocked(agent, actx, now)
		job = c.summarizeIdleLocked(agent, actx)
		recordRootStateLocked(actx, &agent.Activity)
		for subID, sub := range actx.subagents {
			sub.activity = types.ActivityIdle
			sub.activeTool = nil
			sub.inFlightTool = false
			sub.rateLimitResetAt = nil
			sub.retryAfterSeconds = nil
			if sub.completedAt == nil {
				comp := now
				sub.completedAt = &comp
			}
			sub.status = types.SubagentCompleted
			c.scheduleSubagentDwellTimerLocked(agentID, subID, sub, now)
		}
		c.rollupActivityLocked(agent, actx, now)
		captureCompletionLocked(actx, &agent.Activity)
		return true
	})
	c.finishTransition(agentID, agent, job, err, "saving_silent_idle_agent_failed")
}

// silenceCanIdle reports whether the silence timeout applies to a root in this activity.
func silenceCanIdle(activity types.ActivityState) bool {
	return activity == types.ActivityBusy || activity == types.ActivityRateLimited
}

// settleIdleLocked moves the root to IDLE: no tool, question, reset targets,
// in-flight tool, thinking floor, or silence timer.
func (c *Classifier) settleIdleLocked(agent *types.IAgentState, actx *agentContext, now int64) {
	agent.Activity.Activity = types.ActivityIdle
	agent.Activity.ActiveTool = nil
	agent.Activity.ActiveTarget = nil
	agent.Activity.Question = nil
	clearRateLimit(&agent.Activity)
	agent.Activity.UpdatedAt = now
	c.cancelSilenceTimerLocked(agent.ID)
	resetTurnLocked(actx)
}

// summarizeIdleLocked applies the fallback action for the newly idle agent and
// returns the forced summary job to enqueue once the transition is saved.
func (c *Classifier) summarizeIdleLocked(agent *types.IAgentState, actx *agentContext) *summaryJob {
	input, currentToolKey := prepareSummaryInputLocked(agent, actx)
	if c.summarizer.IsNoSummarize() {
		agent.Activity.ActionStale = false
		return nil
	}
	return c.dispatchSummaryLocked(agent, actx, input, currentToolKey, true)
}

// finishTransition enqueues the transition's summary job and publishes the saved
// agent, or logs the failed write.
func (c *Classifier) finishTransition(agentID string, agent *types.IAgentState, job *summaryJob, err error, failedAction string) {
	if err != nil || agent == nil {
		c.broadcastWrite(agentID, agent, err, failedAction)
		return
	}
	if job != nil {
		c.enqueueJob(agent.ID, job.input, job.timestamp, job.force, job.saveGen)
	}
	c.broadcastWrite(agentID, agent, nil, failedAction)
}
