package classifier

import (
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
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
	targetActx := actx
	completedAtTs := actx.turnEndTimestamp
	if completedAtTs == 0 {
		completedAtTs = actx.lastEventTimestamp
	}
	c.mu.Unlock()

	agent, err := c.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	// Only transition to IDLE if agent is still running
	if agent.Activity.Lifecycle != types.LifecycleRunning {
		return
	}

	now := time.Now().UnixMilli()
	if completedAtTs == 0 {
		completedAtTs = now
	}
	if agent.Activity.Activity == types.ActivityBusy {
		agent.Activity.HasUnseenCompletion = boolPtr(true)
		agent.Activity.CompletedAt = int64Ptr(completedAtTs)
	}
	agent.Activity.Activity = types.ActivityIdle
	agent.Activity.ActiveTool = nil
	agent.Activity.ActiveTarget = nil
	agent.Activity.Question = nil
	agent.Activity.RateLimitResetAt = nil
	agent.Activity.RetryAfterSeconds = nil
	agent.Activity.UpdatedAt = now

	// Compute fallback summary purely without mutating agentContext or cancelling trailing timers
	input, currentToolKey := c.prepareSummaryInput(agent)

	if c.beforeIdleSummarizeHook != nil {
		c.beforeIdleSummarizeHook(agentID)
	}

	c.mu.Lock()
	actx, ok = c.agentCtx[agentID]
	if !ok || actx != targetActx || actx.rootGeneration != scheduledGen || actx.rootActivity != types.ActivityBusy {
		c.mu.Unlock()
		return
	}
	if actx.saveGeneration != scheduledSaveGen {
		c.scheduleIdleTransitionLocked(agentID, actx.rootGeneration, 100*time.Millisecond)
		c.mu.Unlock()
		return
	}
	persistMu := &actx.persistMu
	c.mu.Unlock()

	if c.beforeIdleSaveHook != nil {
		c.beforeIdleSaveHook(agentID)
	}

	persistMu.Lock()

	c.mu.Lock()
	actx, ok = c.agentCtx[agentID]
	if !ok || actx != targetActx || actx.rootGeneration != scheduledGen || actx.rootActivity != types.ActivityBusy {
		c.mu.Unlock()
		persistMu.Unlock()
		return
	}
	if actx.saveGeneration != scheduledSaveGen {
		c.scheduleIdleTransitionLocked(agentID, actx.rootGeneration, 100*time.Millisecond)
		c.mu.Unlock()
		persistMu.Unlock()
		return
	}

	c.cancelSilenceTimerLocked(agentID)
	actx.inFlightTool = false
	actx.inFlightToolName = ""
	actx.inTurn = false
	actx.turnFloorUntil = 0

	var job *summaryJob
	if !c.summarizer.IsNoSummarize() {
		job = c.dispatchSummaryLocked(agent, actx, input, currentToolKey, true)
	} else {
		agent.Activity.ActionStale = false
	}

	actx.rootActivity = types.ActivityIdle
	actx.rootRateLimitResetAt = nil
	actx.rootRetryAfterSeconds = nil
	actx.rootAction = agent.Activity.Action
	actx.rootQuestion = nil
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt
	c.rollupActivityLocked(agent, actx, now)
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt
	c.mu.Unlock()

	saveErr := c.store.SaveAgentState(*agent)
	persistMu.Unlock()

	if saveErr != nil {
		logging.Error("saving_idle_agent_failed", "err", saveErr, "agentId", agentID)
		return
	}

	if job != nil {
		c.enqueueJob(agent.ID, job.input, job.timestamp, job.force, job.saveGen)
	}

	c.mu.Lock()
	cb := c.onUpdate
	c.mu.Unlock()

	if cb != nil {
		cb(agent)
	}
}

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
	if !ok || actx.rootGeneration != scheduledGen || (actx.rootActivity != types.ActivityBusy && actx.rootActivity != types.ActivityRateLimited && !hasRateLimitedSub) {
		c.mu.Unlock()
		return
	}
	scheduledSaveGen := actx.saveGeneration
	scheduledRootActivity := actx.rootActivity
	targetActx := actx
	c.mu.Unlock()

	agent, err := c.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	// Only transition to IDLE if agent is still running
	if agent.Activity.Lifecycle != types.LifecycleRunning {
		return
	}

	if scheduledRootActivity != types.ActivityBusy && scheduledRootActivity != types.ActivityRateLimited && agent.Activity.Activity != types.ActivityRateLimited {
		return
	}

	origAgentActivity := agent.Activity.Activity

	now := time.Now().UnixMilli()
	agent.Activity.HasUnseenCompletion = nil
	agent.Activity.CompletedAt = nil
	agent.Activity.Activity = types.ActivityIdle
	agent.Activity.ActiveTool = nil
	agent.Activity.ActiveTarget = nil
	agent.Activity.Question = nil
	agent.Activity.RateLimitResetAt = nil
	agent.Activity.RetryAfterSeconds = nil
	agent.Activity.UpdatedAt = now

	input, currentToolKey := c.prepareSummaryInput(agent)

	c.mu.Lock()
	actx, ok = c.agentCtx[agentID]
	if !ok || actx != targetActx || actx.rootGeneration != scheduledGen || actx.saveGeneration != scheduledSaveGen || (actx.rootActivity != types.ActivityBusy && actx.rootActivity != types.ActivityRateLimited && origAgentActivity != types.ActivityRateLimited) {
		c.mu.Unlock()
		return
	}
	persistMu := &actx.persistMu
	c.mu.Unlock()

	if c.beforeSilenceSaveHook != nil {
		c.beforeSilenceSaveHook(agentID)
	}

	persistMu.Lock()

	c.mu.Lock()
	actx, ok = c.agentCtx[agentID]
	if !ok || actx != targetActx || actx.rootGeneration != scheduledGen || actx.saveGeneration != scheduledSaveGen || (actx.rootActivity != types.ActivityBusy && actx.rootActivity != types.ActivityRateLimited && origAgentActivity != types.ActivityRateLimited) {
		c.mu.Unlock()
		persistMu.Unlock()
		return
	}

	actx.inFlightTool = false
	actx.inFlightToolName = ""
	actx.inTurn = false
	actx.turnFloorUntil = 0

	var job *summaryJob
	if !c.summarizer.IsNoSummarize() {
		job = c.dispatchSummaryLocked(agent, actx, input, currentToolKey, true)
	} else {
		agent.Activity.ActionStale = false
	}

	actx.rootActivity = types.ActivityIdle
	actx.rootRateLimitResetAt = nil
	actx.rootRetryAfterSeconds = nil
	actx.rootAction = agent.Activity.Action
	actx.rootQuestion = nil
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt

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
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt
	c.mu.Unlock()

	saveErr := c.store.SaveAgentState(*agent)
	persistMu.Unlock()

	if saveErr != nil {
		logging.Error("saving_silent_idle_agent_failed", "err", saveErr, "agentId", agentID)
		return
	}

	if job != nil {
		c.enqueueJob(agent.ID, job.input, job.timestamp, job.force, job.saveGen)
	}

	c.mu.Lock()
	cb := c.onUpdate
	c.mu.Unlock()

	if cb != nil {
		cb(agent)
	}
}
