package classifier

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

type agentContext struct {
	lastPrompt            string
	lastAssistantResponse string
	lastToolArgs          string
	recentTools           []summarizer.ToolExecution
}

type summaryJob struct {
	input     summarizer.SummaryInput
	timestamp int64
	force     bool
	flushDone chan struct{}
}

type agentWorker struct {
	jobs chan summaryJob
	stop chan struct{}
}

const (
	DefaultIdleGracePeriod = 5 * time.Second
)

type Classifier struct {
	store           *store.Store
	pricing         *pricing.Engine
	summarizer      *summarizer.Summarizer
	agentCtx        map[string]*agentContext
	workers         map[string]*agentWorker
	idleTimers      map[string]*time.Timer
	idleGracePeriod time.Duration
	onUpdate        func(agent *types.IAgentState)
	mu              sync.Mutex
	wg              sync.WaitGroup
	closed          bool
	syncMode        bool
}

func New(s *store.Store, p *pricing.Engine, sum *summarizer.Summarizer) *Classifier {
	return &Classifier{
		store:           s,
		pricing:         p,
		summarizer:      sum,
		agentCtx:        make(map[string]*agentContext),
		workers:         make(map[string]*agentWorker),
		idleTimers:      make(map[string]*time.Timer),
		idleGracePeriod: DefaultIdleGracePeriod,
	}
}

// SetIdleGracePeriod sets the debounce window before transitioning from BUSY to IDLE on turn_end.
func (c *Classifier) SetIdleGracePeriod(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.idleGracePeriod = d
}

// SetOnUpdate registers a callback invoked when background summarization updates an agent state.
func (c *Classifier) SetOnUpdate(fn func(agent *types.IAgentState)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onUpdate = fn
}

// SetSyncMode enables or disables synchronous summarization (useful for deterministic testing).
func (c *Classifier) SetSyncMode(sync bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncMode = sync
}

// Close gracefully stops all per-agent async workers and waits for in-flight jobs to complete.
func (c *Classifier) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	for _, w := range c.workers {
		close(w.stop)
	}
	for id, t := range c.idleTimers {
		t.Stop()
		delete(c.idleTimers, id)
	}
	c.mu.Unlock()

	c.wg.Wait()
	return nil
}

func (c *Classifier) cancelIdleTimerLocked(agentID string) {
	if t, exists := c.idleTimers[agentID]; exists {
		t.Stop()
		delete(c.idleTimers, agentID)
	}
}

// Flush blocks until all pending jobs in the queue for a specific agent have completed.
func (c *Classifier) Flush(agentID string) {
	c.mu.Lock()
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
	ids := make([]string, 0, len(c.workers))
	for id := range c.workers {
		ids = append(ids, id)
	}
	c.mu.Unlock()

	for _, id := range ids {
		c.Flush(id)
	}
}

// ProcessFrame applies 0-ms deterministic rules and schedules asynchronous action summarization.
func (c *Classifier) ProcessFrame(frame types.ITelemetryFrame) (*types.IAgentState, error) {
	agent, err := c.store.GetAgent(frame.AgentID)
	if err != nil {
		return nil, fmt.Errorf("getting existing agent: %w", err)
	}

	now := frame.Timestamp
	if now == 0 {
		now = time.Now().UnixMilli()
	}

	if agent == nil {
		envName := frame.Environment.Label
		if envName == "" {
			envName = frame.Environment.Hostname
		}
		logging.Info("session_discovered",
			"project", frame.Project.Name,
			"env", envName,
			"envId", frame.EnvID,
			"harness", frame.Harness,
			"sessionId", frame.SessionID,
			"agentId", frame.AgentID,
		)
		// Initialize new agent state
		agent = &types.IAgentState{
			ID:          frame.AgentID,
			SessionID:   frame.SessionID,
			Harness:     frame.Harness,
			Environment: frame.Environment,
			Project:     frame.Project,
			Workspace:   frame.Workspace,
			Activity: types.IAgentActivity{
				Lifecycle:      types.LifecycleRunning,
				Activity:       types.ActivityIdle,
				FirstStartedAt: now,
				UpdatedAt:      now,
			},
		}
	} else {
		// Update mutable workspace/environment metadata
		agent.Environment = frame.Environment
		agent.Workspace = frame.Workspace
		agent.Project = frame.Project
		agent.Activity.UpdatedAt = now
	}

	isSubagent := frame.SubagentID != nil && *frame.SubagentID != ""

	// Accumulate Usage & Cost
	if frame.Data.Usage != nil {
		u := frame.Data.Usage
		agent.Activity.Usage.Input += u.Input
		agent.Activity.Usage.Output += u.Output
		agent.Activity.Usage.CacheRead += u.CacheRead
		agent.Activity.Usage.CacheWrite += u.CacheWrite
		agent.Activity.Usage.Reasoning += u.Reasoning
		if u.Total > 0 {
			agent.Activity.Usage.Total += u.Total
		} else {
			agent.Activity.Usage.Total += (u.Input + u.Output + u.CacheRead + u.CacheWrite)
		}

		if frame.Data.Model != nil && c.pricing != nil {
			cost, ok := c.pricing.CalculateCost(*frame.Data.Model, pricing.Usage{
				Input:      u.Input,
				Output:     u.Output,
				CacheRead:  u.CacheRead,
				CacheWrite: u.CacheWrite,
				Reasoning:  u.Reasoning,
			})
			if ok {
				if agent.Activity.Cost == nil {
					agent.Activity.Cost = &types.ICostInfo{
						Currency: "USD",
						Total:    0,
					}
				}
				agent.Activity.Cost.Total += cost
			}
		}
	}

	// Update Title if provided
	if frame.Data.Title != nil && strings.TrimSpace(*frame.Data.Title) != "" {
		agent.Activity.Title = strings.TrimSpace(*frame.Data.Title)
	}

	// Update Prompt Context if provided
	c.mu.Lock()
	actx, exists := c.agentCtx[agent.ID]
	if !exists {
		actx = &agentContext{}
		c.agentCtx[agent.ID] = actx
	}
	if frame.Data.LastPrompt != nil && strings.TrimSpace(*frame.Data.LastPrompt) != "" {
		newPrompt := strings.TrimSpace(*frame.Data.LastPrompt)
		if newPrompt != actx.lastPrompt {
			actx.lastPrompt = newPrompt
			actx.lastAssistantResponse = ""
			actx.lastToolArgs = ""
			actx.recentTools = nil
		}
	}
	c.mu.Unlock()

	var pendingJob *summaryJob

	// If not a subagent, apply deterministic state machine transitions
	if !isSubagent {
		switch frame.EventType {
		case "run_started":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.mu.Unlock()
			agent.Activity.Lifecycle = types.LifecycleRunning
			agent.Activity.Activity = types.ActivityIdle
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
			c.mu.Unlock()

			agent.Activity.Lifecycle = types.LifecycleExited
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

		case "question_prompt":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityWaitingQuestion
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			if frame.Data.Question != nil {
				agent.Activity.Question = frame.Data.Question
			}
			pendingJob = c.updateSummarization(agent, true)

		case "error":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityError
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			if frame.Data.Content != nil {
				errStr := *frame.Data.Content
				if len(errStr) > 2000 {
					errStr = errStr[:2000]
				}
				agent.Activity.LastError = &errStr
			}
			pendingJob = c.updateSummarization(agent, true)

		case "tool_result":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.mu.Unlock()
			// Tool results (even non-zero exits) are a normal part of shell/agent workflows;
			// the agent remains busy processing the result, not in an error state.
			agent.Activity.Activity = types.ActivityBusy
			agent.Activity.LastError = nil
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			pendingJob = c.updateSummarization(agent, false)

		case "tool_call":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityBusy
			agent.Activity.LastError = nil
			if frame.Data.ToolName != nil {
				toolName := *frame.Data.ToolName
				agent.Activity.ActiveTool = &toolName

				target := extractTarget(toolName, frame.Data.ToolInput)
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
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityBusy
			agent.Activity.LastError = nil
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			c.mu.Lock()
			actx.lastAssistantResponse = ""
			actx.lastToolArgs = ""
			actx.recentTools = nil
			c.mu.Unlock()

		case "thinking":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityBusy
			agent.Activity.LastError = nil
			if frame.Data.Content != nil {
				agent.Activity.LastThinkingExcerpt = frame.Data.Content
			}

		case "turn_end":
			if frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != "" {
				c.mu.Lock()
				actx.lastAssistantResponse = strings.TrimSpace(*frame.Data.Content)
				c.mu.Unlock()
			}
			isToolUse := false
			if frame.Data.StopReason != nil {
				reason := *frame.Data.StopReason
				if reason == "tool_use" || reason == "toolUse" || reason == "tool_calls" {
					isToolUse = true
				}
			}
			if agent.Activity.ActiveTool != nil {
				isToolUse = true
			}

			if isToolUse {
				c.mu.Lock()
				c.cancelIdleTimerLocked(agent.ID)
				c.mu.Unlock()
				if agent.Activity.Activity != types.ActivityWaitingQuestion {
					agent.Activity.Activity = types.ActivityBusy
				}
				pendingJob = c.updateSummarization(agent, true)
			} else if agent.Activity.Activity == types.ActivityWaitingQuestion {
				// Do not overwrite WAITING_QUESTION or clear Question prompt on turn_end
				c.mu.Lock()
				c.cancelIdleTimerLocked(agent.ID)
				c.mu.Unlock()
				pendingJob = c.updateSummarization(agent, true)
			} else if agent.Activity.Activity == types.ActivityError {
				// Do not overwrite ERROR state on turn_end
				c.mu.Lock()
				c.cancelIdleTimerLocked(agent.ID)
				c.mu.Unlock()
				pendingJob = c.updateSummarization(agent, true)
			} else {
				// Turn ended without tool use.
				// If the agent is BUSY and an idle grace period is configured, debounce the transition
				// so pauses between API calls do not cause BUSY/IDLE flipping.
				agent.Activity.ActiveTool = nil
				agent.Activity.ActiveTarget = nil
				agent.Activity.Question = nil

				c.mu.Lock()
				gracePeriod := c.idleGracePeriod
				isSync := c.syncMode
				c.mu.Unlock()

				if gracePeriod <= 0 || isSync || agent.Activity.Activity != types.ActivityBusy {
					c.mu.Lock()
					c.cancelIdleTimerLocked(agent.ID)
					c.mu.Unlock()
					agent.Activity.Activity = types.ActivityIdle
					pendingJob = c.updateSummarization(agent, true)
				} else {
					// Keep agent BUSY during grace period and schedule transition to IDLE if no new events arrive.
					c.mu.Lock()
					c.cancelIdleTimerLocked(agent.ID)
					targetAgentID := agent.ID
					scheduledUpdatedAt := agent.Activity.UpdatedAt
					c.idleTimers[targetAgentID] = time.AfterFunc(gracePeriod, func() {
						c.handleIdleTransition(targetAgentID, scheduledUpdatedAt)
					})
					c.mu.Unlock()
					pendingJob = c.updateSummarization(agent, false)
				}
			}
		}
	} else {
		// Subagent event: cancel parent idle timer and increment subagent count
		c.mu.Lock()
		c.cancelIdleTimerLocked(agent.ID)
		c.mu.Unlock()
		agent.Activity.SubagentCount++
	}

	if err := c.store.SaveAgentState(*agent); err != nil {
		return nil, fmt.Errorf("saving agent state: %w", err)
	}

	// Record the raw event after agent exists in DB
	if err := c.store.RecordTelemetryEvent(frame); err != nil {
		return nil, fmt.Errorf("recording telemetry event: %w", err)
	}

	// Enqueue async LLM summarization job after agent is safely committed to database
	if pendingJob != nil {
		c.enqueueJob(agent.ID, pendingJob.input, pendingJob.timestamp, pendingJob.force)
	}

	return agent, nil
}

func (c *Classifier) updateSummarization(agent *types.IAgentState, forceTrigger bool) *summaryJob {
	c.mu.Lock()
	actx := c.agentCtx[agent.ID]
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
	isSync := c.syncMode
	c.mu.Unlock()

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

	// If external LLM summarization is disabled or summarizer is nil, run deterministic fallback immediately
	if c.summarizer.IsNoSummarize() {
		fb := summarizer.FallbackSummary(input)
		agent.Activity.Action = fb.Action
		agent.Activity.Phase = fb.Phase
		agent.Activity.ActionStale = false
		if fb.ActiveTarget != nil {
			agent.Activity.ActiveTarget = fb.ActiveTarget
		}
		return nil
	}

	// Always compute fallback summary so the returned agent state is immediately valid
	fb := summarizer.FallbackSummary(input)
	if agent.Activity.Action == "" || forceTrigger {
		agent.Activity.Action = fb.Action
		agent.Activity.Phase = fb.Phase
		if fb.ActiveTarget != nil {
			agent.Activity.ActiveTarget = fb.ActiveTarget
		}
	}

	// Synchronous mode execution (e.g. for deterministic unit test assertions)
	if isSync {
		if forceTrigger || c.summarizer.Allow(agent.ID) {
			res := c.summarizer.Summarize(context.Background(), input)
			isDup := summarizer.IsNearDuplicate(agent.Activity.Action, res.Action, summarizer.DefaultSimilarityThreshold)
			phaseSame := (agent.Activity.Phase == nil && res.Phase == nil) || (agent.Activity.Phase != nil && res.Phase != nil && *agent.Activity.Phase == *res.Phase)
			targetSame := (agent.Activity.ActiveTarget == nil && res.ActiveTarget == nil) || (agent.Activity.ActiveTarget != nil && res.ActiveTarget != nil && *agent.Activity.ActiveTarget == *res.ActiveTarget)

			if !isDup || !phaseSame || !targetSame {
				agent.Activity.Action = res.Action
				agent.Activity.Phase = res.Phase
				if res.ActiveTarget != nil && *res.ActiveTarget != "" {
					agent.Activity.ActiveTarget = res.ActiveTarget
				}
			}
			agent.Activity.ActionStale = false
		} else {
			if agent.Activity.Activity == types.ActivityBusy {
				agent.Activity.ActionStale = true
			}
		}
		return nil
	}

	// Check rate limit debounce for external LLM calls
	if forceTrigger || c.summarizer.Allow(agent.ID) {
		agent.Activity.ActionStale = false
		return &summaryJob{
			input:     input,
			timestamp: agent.Activity.UpdatedAt,
			force:     forceTrigger,
		}
	}

	// Debounced/rate-limited continuous turn
	if agent.Activity.Activity == types.ActivityBusy {
		agent.Activity.ActionStale = true
	}
	return nil
}

func (c *Classifier) enqueueJob(agentID string, input summarizer.SummaryInput, timestamp int64, force bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

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

	job := summaryJob{
		input:     input,
		timestamp: timestamp,
		force:     force,
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

func (c *Classifier) processSummaryJob(agentID string, job summaryJob) {
	res := c.summarizer.Summarize(context.Background(), job.input)
	if res.Action == "" {
		return
	}

	agent, err := c.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	// Discard stale summary if agent has exited or transitioned to another activity state
	if agent.Activity.CurrentRun != nil && agent.Activity.CurrentRun.EndedAt != nil {
		return
	}
	if agent.Activity.Activity != job.input.Activity {
		return
	}

	// Deduplication check: reject near-duplicates when phase and target are unchanged
	isDup := summarizer.IsNearDuplicate(agent.Activity.Action, res.Action, summarizer.DefaultSimilarityThreshold)
	phaseSame := (agent.Activity.Phase == nil && res.Phase == nil) || (agent.Activity.Phase != nil && res.Phase != nil && *agent.Activity.Phase == *res.Phase)
	targetSame := (agent.Activity.ActiveTarget == nil && res.ActiveTarget == nil) || (agent.Activity.ActiveTarget != nil && res.ActiveTarget != nil && *agent.Activity.ActiveTarget == *res.ActiveTarget)

	if isDup && phaseSame && targetSame {
		if agent.Activity.ActionStale {
			agent.Activity.ActionStale = false
			_ = c.store.SaveAgentState(*agent)
		}
		return
	}

	// Apply updated summary
	agent.Activity.Action = res.Action
	agent.Activity.Phase = res.Phase
	if res.ActiveTarget != nil && *res.ActiveTarget != "" {
		agent.Activity.ActiveTarget = res.ActiveTarget
	}
	agent.Activity.ActionStale = false
	agent.Activity.UpdatedAt = time.Now().UnixMilli()

	if err := c.store.SaveAgentState(*agent); err != nil {
		logging.Error("saving_async_summary_error", "err", err, "agentId", agentID)
		return
	}

	c.mu.Lock()
	cb := c.onUpdate
	c.mu.Unlock()

	if cb != nil {
		cb(agent)
	}
}

func (c *Classifier) handleIdleTransition(agentID string, scheduledUpdatedAt int64) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	delete(c.idleTimers, agentID)
	c.mu.Unlock()

	agent, err := c.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	// Only transition to IDLE if agent is still running, busy, and has not received newer events since the timer was scheduled
	if agent.Activity.Lifecycle != types.LifecycleRunning || agent.Activity.Activity != types.ActivityBusy || agent.Activity.UpdatedAt != scheduledUpdatedAt {
		return
	}

	agent.Activity.Activity = types.ActivityIdle
	agent.Activity.ActiveTool = nil
	agent.Activity.ActiveTarget = nil
	agent.Activity.Question = nil
	agent.Activity.UpdatedAt = time.Now().UnixMilli()

	job := c.updateSummarization(agent, true)
	if err := c.store.SaveAgentState(*agent); err != nil {
		logging.Error("saving_idle_agent_failed", "err", err, "agentId", agentID)
		return
	}

	if job != nil {
		c.enqueueJob(agent.ID, job.input, job.timestamp, job.force)
	}

	c.mu.Lock()
	cb := c.onUpdate
	c.mu.Unlock()
	if cb != nil {
		cb(agent)
	}
}

func (c *Classifier) ensureAction(agent *types.IAgentState) *summaryJob {
	if agent.Activity.Action == "" {
		return c.updateSummarization(agent, false)
	}
	return nil
}

func extractTarget(toolName string, input map[string]interface{}) string {
	if input == nil {
		return ""
	}
	keys := []string{
		"file_path",
		"path",
		"notebook_path",
		"description",
		"command",
		"url",
		"query",
		"skill",
		"pattern",
		"prompt",
	}
	for _, k := range keys {
		if val, ok := input[k]; ok && val != nil {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
