package classifier

import (
	"context"
	"fmt"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

func (c *Classifier) flushTrailingCandidate(agentID string) {
	c.mu.Lock()
	c.flushTrailingCandidateLocked(agentID)
	c.mu.Unlock()
}

func (c *Classifier) flushTrailingCandidateLocked(agentID string) {
	if c.closed {
		return
	}
	delete(c.trailingTimers, agentID)

	actx := c.agentCtx[agentID]
	if actx == nil || actx.pendingCandidate == nil {
		return
	}

	cand := actx.pendingCandidate
	actx.pendingCandidate = nil

	if cand.input.Activity == types.ActivityRateLimited || actx.rootActivity == types.ActivityRateLimited {
		return
	}

	// Check duplicate suppression: if candidate matches lastSummarizedToolKey, skip
	if cand.toolKey != "" && cand.toolKey == actx.lastSummarizedToolKey {
		return
	}

	actx.lastSummarizedToolKey = cand.toolKey
	actx.lastSummaryDispatchedAt = cand.timestamp

	input := cand.input
	ts := cand.timestamp
	isSync := c.syncMode

	if isSync {
		c.mu.Unlock()
		res := c.summarizer.Summarize(context.Background(), input)
		if c.beforeSummarySaveHook != nil {
			c.beforeSummarySaveHook(agentID)
		}
		agent, err := c.persistAgentAfterFrame(agentID, actx, func(agent *types.IAgentState) bool {
			if (cand.saveGen != 0 && actx.saveGeneration != cand.saveGen) || !runIsActive(agent) {
				return false
			}
			if shouldApplySummary(&agent.Activity, res) {
				applySummary(&agent.Activity, res)
			}
			agent.Activity.ActionStale = false
			syncCompletionLocked(actx, agent)
			return true
		})
		c.broadcastWrite(agentID, agent, err, "saving_trailing_summary_error")
		c.mu.Lock()
		return
	}

	c.enqueueJobLocked(agentID, input, ts, false, cand.saveGen)
}

// Flush blocks until all pending jobs in the queue for a specific agent have completed.
func (c *Classifier) Flush(agentID string) {
	c.mu.Lock()
	c.flushTrailingCandidateLocked(agentID)
	w, exists := c.workers[agentID]
	c.mu.Unlock()
	if !exists {
		return
	}

	done := make(chan struct{})
	select {
	case w.jobs <- summaryJob{flushDone: done}:
		<-done
	case <-time.After(5 * time.Second):
	}
}

// FlushAll blocks until all pending jobs across all agent queues have completed.
func (c *Classifier) FlushAll() {
	c.mu.Lock()
	ids := make([]string, 0, len(c.workers)+len(c.trailingTimers))
	for id := range c.workers {
		ids = append(ids, id)
	}
	for id := range c.trailingTimers {
		found := false
		for _, existing := range ids {
			if existing == id {
				found = true
				break
			}
		}
		if !found {
			ids = append(ids, id)
		}
	}
	c.mu.Unlock()

	for _, id := range ids {
		c.Flush(id)
	}
}

func (c *Classifier) prepareSummaryInput(agent *types.IAgentState) (summarizer.SummaryInput, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return prepareSummaryInputLocked(agent, c.agentCtx[agent.ID])
}

// prepareSummaryInputLocked builds the summarizer input from agent and the
// context's recent prompt and tools, and applies the deterministic fallback
// action to agent so its state is valid before any LLM summary arrives.
func prepareSummaryInputLocked(agent *types.IAgentState, actx *agentContext) (summarizer.SummaryInput, string) {
	var lastPrompt string
	var lastAssistantResponse string
	var lastToolArgs string
	var recentTools []summarizer.ToolExecution
	if actx != nil {
		lastPrompt = actx.lastPrompt
		lastAssistantResponse = actx.lastAssistantResponse
		lastToolArgs = actx.lastToolArgs
		recentTools = make([]summarizer.ToolExecution, len(actx.recentTools))
		copy(recentTools, actx.recentTools)
	}

	var thinking string
	if agent.Activity.LastThinkingExcerpt != nil {
		thinking = *agent.Activity.LastThinkingExcerpt
	}

	var toolArgsPtr *string
	if lastToolArgs != "" {
		toolArgsPtr = &lastToolArgs
	}

	input := summarizer.SummaryInput{
		AgentID:               agent.ID,
		CurrentAction:         agent.Activity.Action,
		LastPrompt:            lastPrompt,
		LastAssistantResponse: lastAssistantResponse,
		RecentTools:           recentTools,
		LastThinkingExcerpt:   thinking,
		Activity:              agent.Activity.Activity,
		ActiveTool:            agent.Activity.ActiveTool,
		ActiveTarget:          agent.Activity.ActiveTarget,
		ActiveToolArgs:        toolArgsPtr,
		LastError:             agent.Activity.LastError,
		Question:              agent.Activity.Question,
	}

	// Always compute fallback summary so the returned agent state is immediately valid
	fb := summarizer.FallbackSummary(input)
	agent.Activity.Action = fb.Action
	agent.Activity.Phase = fb.Phase
	if fb.ActiveTarget != nil {
		agent.Activity.ActiveTarget = fb.ActiveTarget
	}

	currentToolKey := ""
	if agent.Activity.ActiveTool != nil {
		targetStr := ""
		if agent.Activity.ActiveTarget != nil {
			targetStr = *agent.Activity.ActiveTarget
		}
		currentToolKey = fmt.Sprintf("%s|%s|%s", *agent.Activity.ActiveTool, targetStr, lastToolArgs)
	}

	return input, currentToolKey
}

func (c *Classifier) dispatchSummaryLocked(agent *types.IAgentState, actx *agentContext, input summarizer.SummaryInput, currentToolKey string, forceTrigger bool) *summaryJob {
	if agent.Activity.Activity == types.ActivityRateLimited {
		c.showRateLimitActionLocked(agent, actx)
		return nil
	}

	eventTime := time.Now()
	if agent.Activity.UpdatedAt > 0 {
		eventTime = time.UnixMilli(agent.Activity.UpdatedAt)
	}
	nowMs := eventTime.UnixMilli()

	windowMs := c.debounceWindow.Milliseconds()
	if windowMs <= 0 {
		windowMs = DefaultDebounceWindow.Milliseconds()
	}

	// 1. Force trigger (e.g. AskUserQuestion, error, turn_end, idle transition)
	if forceTrigger {
		c.cancelTrailingTimerLocked(agent.ID)
		actx.pendingCandidate = nil
		actx.lastSummaryDispatchedAt = nowMs
		actx.lastSummarizedToolKey = currentToolKey
		c.summarizer.AllowAt(agent.ID, eventTime)

		if c.syncMode {
			c.mu.Unlock()
			res := c.summarizer.Summarize(context.Background(), input)
			c.mu.Lock()
			if shouldApplySummary(&agent.Activity, res) {
				applySummary(&agent.Activity, res)
			}
			agent.Activity.ActionStale = false
			return nil
		}

		agent.Activity.ActionStale = false
		return &summaryJob{
			input:     input,
			timestamp: agent.Activity.UpdatedAt,
			force:     true,
			saveGen:   actx.saveGeneration,
		}
	}

	// 2. Completed tool with no active tool (e.g. tool_result)
	if agent.Activity.ActiveTool == nil {
		return nil
	}

	// 3. Duplicate suppression: if tool and target/args are unchanged, skip redundant summarization
	if currentToolKey != "" && currentToolKey == actx.lastSummarizedToolKey {
		agent.Activity.ActionStale = false
		return nil
	}

	// 4. Cadence check: 10s debounce window
	elapsed := nowMs - actx.lastSummaryDispatchedAt
	canDispatchImmediately := actx.lastSummaryDispatchedAt == 0 || elapsed >= windowMs

	if canDispatchImmediately {
		c.cancelTrailingTimerLocked(agent.ID)
		actx.pendingCandidate = nil
		actx.lastSummaryDispatchedAt = nowMs
		actx.lastSummarizedToolKey = currentToolKey
		c.summarizer.AllowAt(agent.ID, eventTime)

		if c.syncMode {
			c.mu.Unlock()
			res := c.summarizer.Summarize(context.Background(), input)
			c.mu.Lock()
			if shouldApplySummary(&agent.Activity, res) {
				applySummary(&agent.Activity, res)
			}
			agent.Activity.ActionStale = false
			return nil
		}

		agent.Activity.ActionStale = false
		return &summaryJob{
			input:     input,
			timestamp: agent.Activity.UpdatedAt,
			force:     false,
			saveGen:   actx.saveGeneration,
		}
	}

	// 5. Within debounce window: buffer tool call as pending candidate and schedule trailing-edge flush
	actx.pendingCandidate = &pendingSummaryCandidate{
		input:     input,
		timestamp: nowMs,
		toolKey:   currentToolKey,
		saveGen:   actx.saveGeneration,
	}
	if agent.Activity.Activity == types.ActivityBusy {
		agent.Activity.ActionStale = true
	}

	remaining := time.Duration(windowMs-elapsed) * time.Millisecond
	c.scheduleTrailingTimerLocked(agent.ID, remaining)

	return nil
}

func (c *Classifier) updateSummarization(agent *types.IAgentState, forceTrigger bool) *summaryJob {
	input, currentToolKey := c.prepareSummaryInput(agent)

	if agent.Activity.Activity == types.ActivityRateLimited {
		c.mu.Lock()
		c.showRateLimitActionLocked(agent, c.agentCtx[agent.ID])
		c.mu.Unlock()
		return nil
	}

	// If external LLM summarization is disabled or summarizer is nil, run deterministic fallback immediately
	if c.summarizer.IsNoSummarize() {
		agent.Activity.ActionStale = false
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	actx := c.agentCtx[agent.ID]
	if actx == nil {
		actx = &agentContext{seenSubagents: make(map[string]struct{})}
		c.agentCtx[agent.ID] = actx
	}

	return c.dispatchSummaryLocked(agent, actx, input, currentToolKey, forceTrigger)
}

func (c *Classifier) enqueueJob(agentID string, input summarizer.SummaryInput, timestamp int64, force bool, saveGen ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enqueueJobLocked(agentID, input, timestamp, force, saveGen...)
}

func (c *Classifier) enqueueJobLocked(agentID string, input summarizer.SummaryInput, timestamp int64, force bool, saveGen ...uint64) {
	if c.closed {
		return
	}

	w, exists := c.workers[agentID]
	if !exists {
		w = &agentWorker{
			jobs: make(chan summaryJob, 10),
			stop: make(chan struct{}),
		}
		c.workers[agentID] = w
		c.wg.Add(1)
		go c.runWorker(agentID, w)
	}

	var sg uint64
	if len(saveGen) > 0 && saveGen[0] > 0 {
		sg = saveGen[0]
	} else if actx, ok := c.agentCtx[agentID]; ok {
		sg = actx.saveGeneration
	}

	job := summaryJob{
		input:     input,
		timestamp: timestamp,
		force:     force,
		saveGen:   sg,
	}

	select {
	case w.jobs <- job:
	default:
		// Drop older unstarted job from queue to maintain latest context
		select {
		case dropped := <-w.jobs:
			if dropped.flushDone != nil {
				close(dropped.flushDone)
			}
		default:
		}
		w.jobs <- job
	}
}

func (c *Classifier) runWorker(agentID string, w *agentWorker) {
	defer c.wg.Done()

	for {
		select {
		case <-w.stop:
			return
		case job, ok := <-w.jobs:
			if !ok {
				return
			}
			if job.flushDone != nil {
				close(job.flushDone)
				continue
			}
			c.processSummaryJob(agentID, job)
		}
	}
}

// processSummaryJob runs one queued LLM summary and saves it unless a newer
// frame arrived or the agent's state no longer matches the summarized input.
func (c *Classifier) processSummaryJob(agentID string, job summaryJob) {
	if job.input.Activity == types.ActivityRateLimited {
		return
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	actx, ok := c.agentCtx[agentID]
	if !ok {
		// Summary jobs only apply to active runs.
		c.mu.Unlock()
		return
	}
	scheduledSaveGen := job.saveGen
	if scheduledSaveGen == 0 {
		scheduledSaveGen = actx.saveGeneration
	} else if actx.saveGeneration != scheduledSaveGen {
		// Newer frame arrived before Summarize was dispatched.
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	res := c.summarizer.Summarize(context.Background(), job.input)
	if res.Action == "" {
		return
	}

	if c.beforeSummarySaveHook != nil {
		c.beforeSummarySaveHook(agentID)
	}

	agent, err := c.persistAgentAfterFrame(agentID, actx, func(agent *types.IAgentState) bool {
		if actx.saveGeneration != scheduledSaveGen || !summaryStillApplies(agent, job.input) {
			return false
		}
		if shouldApplySummary(&agent.Activity, res) {
			applySummary(&agent.Activity, res)
			agent.Activity.UpdatedAt = time.Now().UnixMilli()
		} else if !agent.Activity.ActionStale {
			// A near-duplicate of a current action changes nothing.
			return false
		}
		agent.Activity.ActionStale = false
		syncCompletionLocked(actx, agent)
		return true
	})
	c.broadcastWrite(agentID, agent, err, "saving_async_summary_error")
}

// summaryStillApplies reports whether a summary of input still describes agent:
// the run is active, not rate limited, and in the same activity and tool.
func summaryStillApplies(agent *types.IAgentState, input summarizer.SummaryInput) bool {
	return runIsActive(agent) &&
		agent.Activity.Activity != types.ActivityRateLimited &&
		agent.Activity.Activity == input.Activity &&
		equalPtr(agent.Activity.ActiveTool, input.ActiveTool)
}

// runIsActive reports whether the agent has not exited and its current run has not ended.
func runIsActive(agent *types.IAgentState) bool {
	return agent.Activity.Lifecycle != types.LifecycleExited &&
		(agent.Activity.CurrentRun == nil || agent.Activity.CurrentRun.EndedAt == nil)
}

// broadcastWrite logs a failed background write, or publishes a saved agent.
// A write that saved nothing publishes nothing.
func (c *Classifier) broadcastWrite(agentID string, agent *types.IAgentState, err error, failedAction string) {
	if err != nil {
		logging.Error(failedAction, "err", err, "agentId", agentID)
		return
	}
	if agent == nil {
		return
	}
	c.mu.Lock()
	cb := c.onUpdate
	c.mu.Unlock()
	if cb != nil {
		cb(agent)
	}
}

// shouldApplySummary determines whether a new summary result changes the current agent activity.
// It returns true if the action is not a near-duplicate, or if either phase or target differs.
func shouldApplySummary(current *types.IAgentActivity, res summarizer.SummaryResult) bool {
	if current == nil {
		return true
	}
	isDup := summarizer.IsNearDuplicate(current.Action, res.Action, summarizer.DefaultSimilarityThreshold)
	phaseSame := equalPtr(current.Phase, res.Phase)
	targetSame := equalPtr(current.ActiveTarget, res.ActiveTarget)
	return !isDup || !phaseSame || !targetSame
}

// applySummary updates the current agent activity fields from a summary result.
func applySummary(current *types.IAgentActivity, res summarizer.SummaryResult) {
	current.Action = res.Action
	current.Phase = res.Phase
	if current.ActiveTool != nil && res.ActiveTarget != nil && *res.ActiveTarget != "" {
		current.ActiveTarget = res.ActiveTarget
	}
}

// rateLimitAction is the deterministic action shown while an agent is rate limited.
const rateLimitAction = "Rate limit reached — backoff/retry in progress"

// showRateLimitActionLocked replaces the action with rateLimitAction and drops any
// pending summary, because rate-limited agents are never sent to the summarizer.
func (c *Classifier) showRateLimitActionLocked(agent *types.IAgentState, actx *agentContext) {
	agent.Activity.Action = rateLimitAction
	agent.Activity.Phase = nil
	agent.Activity.ActionStale = false
	c.cancelTrailingTimerLocked(agent.ID)
	if actx != nil {
		actx.pendingCandidate = nil
	}
}
