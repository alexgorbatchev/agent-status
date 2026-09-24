package classifier

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/models"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

type pendingSummaryCandidate struct {
	input     summarizer.SummaryInput
	timestamp int64
	toolKey   string
}

type subagentState struct {
	id           string
	activity     types.ActivityState
	activeTool   *string
	action       string
	lastError    *string
	question     *string
	inFlightTool bool
	startedAt    int64
	completedAt  *int64
	status       types.SubagentStatus
}

type agentContext struct {
	lastPrompt              string
	lastAssistantResponse   string
	lastToolArgs            string
	recentTools             []summarizer.ToolExecution
	seenSubagents           map[string]struct{}
	subagents               map[string]*subagentState
	rootActivity            types.ActivityState
	rootAction              string
	rootQuestion            *string
	rootLastError           *string
	rootGeneration          uint64
	lastSummarizedToolKey   string
	lastSummaryDispatchedAt int64
	pendingCandidate        *pendingSummaryCandidate
	inFlightTool            bool
	inFlightToolName        string
	inTurn                  bool
	turnFloorUntil          int64
	hasUnseenCompletion     *bool
	completedAt             *int64
}

func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
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
	DefaultIdleGracePeriod    = 2500 * time.Millisecond
	DefaultTurnEndGraceWindow = DefaultIdleGracePeriod
	DefaultTurnEndGrace       = DefaultIdleGracePeriod
	DefaultDebounceWindow     = 10 * time.Second
	DefaultTurnFloor          = 20 * time.Second
	DefaultTurnSilenceTimeout = 30 * time.Minute
	DefaultSubagentDwell      = 30 * time.Second
)

type Classifier struct {
	store              *store.Store
	pricing            *pricing.Engine
	summarizer         *summarizer.Summarizer
	agentCtx           map[string]*agentContext
	workers            map[string]*agentWorker
	idleTimers         map[string]*time.Timer
	trailingTimers     map[string]*time.Timer
	silenceTimers      map[string]*time.Timer
	subagentTimers     map[string]*time.Timer
	idleGracePeriod    time.Duration
	debounceWindow     time.Duration
	turnFloor          time.Duration
	turnSilenceTimeout time.Duration
	subagentDwell      time.Duration
	onUpdate           func(agent *types.IAgentState)
	mu                 sync.Mutex
	wg                 sync.WaitGroup
	closed             bool
	syncMode           bool

	beforeIdleSummarizeHook func(agentID string)
}

func New(s *store.Store, p *pricing.Engine, sum *summarizer.Summarizer) *Classifier {
	debounce := DefaultDebounceWindow
	if sum != nil && sum.DebounceWindow() > 0 {
		debounce = sum.DebounceWindow()
	}
	return &Classifier{
		store:               s,
		pricing:             p,
		summarizer:          sum,
		agentCtx:            make(map[string]*agentContext),
		workers:             make(map[string]*agentWorker),
		idleTimers:          make(map[string]*time.Timer),
		trailingTimers:      make(map[string]*time.Timer),
		silenceTimers:       make(map[string]*time.Timer),
		subagentTimers:      make(map[string]*time.Timer),
		idleGracePeriod:     DefaultIdleGracePeriod,
		debounceWindow:      debounce,
		turnFloor:           DefaultTurnFloor,
		turnSilenceTimeout:  DefaultTurnSilenceTimeout,
		subagentDwell:       DefaultSubagentDwell,
	}
}

// SetDebounceWindow sets the cadence window for tool summarization debounce.
func (c *Classifier) SetDebounceWindow(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.debounceWindow = d
}

// SetIdleGracePeriod sets the debounce window before transitioning from BUSY to IDLE on turn_end.
func (c *Classifier) SetIdleGracePeriod(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.idleGracePeriod = d
}

// SetTurnEndGraceWindow sets the debounce window before transitioning from BUSY to IDLE on turn_end.
func (c *Classifier) SetTurnEndGraceWindow(d time.Duration) {
	c.SetIdleGracePeriod(d)
}

// SetTurnEndGrace sets the debounce window before transitioning from BUSY to IDLE on turn_end.
func (c *Classifier) SetTurnEndGrace(d time.Duration) {
	c.SetIdleGracePeriod(d)
}

// TurnEndGraceWindow returns the turn-end grace window duration.
func (c *Classifier) TurnEndGraceWindow() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.idleGracePeriod
}

// Stop gracefully stops all per-agent async workers and cancels pending timers.
func (c *Classifier) Stop() error {
	return c.Close()
}

// SetTurnFloor sets the thinking latency floor during which an agent remains latched to BUSY (Thinking).
func (c *Classifier) SetTurnFloor(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turnFloor = d
}

// SetSubagentDwell sets the minimum visibility dwell window for short-lived subagents.
func (c *Classifier) SetSubagentDwell(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subagentDwell = d
}

// SetTurnSilenceTimeout sets the timeout window after which an inactive session transitions to IDLE.
func (c *Classifier) SetTurnSilenceTimeout(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turnSilenceTimeout = d
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
	for id, t := range c.trailingTimers {
		t.Stop()
		delete(c.trailingTimers, id)
	}
	for id, t := range c.silenceTimers {
		t.Stop()
		delete(c.silenceTimers, id)
	}
	for id, t := range c.subagentTimers {
		t.Stop()
		delete(c.subagentTimers, id)
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

func (c *Classifier) cancelSilenceTimerLocked(agentID string) {
	if t, exists := c.silenceTimers[agentID]; exists {
		t.Stop()
		delete(c.silenceTimers, agentID)
	}
}

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

func (c *Classifier) handleSubagentDwellExpired(agentID, subID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	key := fmt.Sprintf("%s:%s", agentID, subID)
	delete(c.subagentTimers, key)
	actx := c.agentCtx[agentID]
	if actx == nil {
		return
	}
	sub, ok := actx.subagents[subID]
	if !ok || sub.completedAt == nil {
		return
	}
	now := time.Now().UnixMilli()
	dwellMs := int64(c.subagentDwell / time.Millisecond)
	if now < sub.startedAt+dwellMs {
		return
	}
	delete(actx.subagents, subID)

	if c.store == nil {
		return
	}
	agent, err := c.store.GetAgent(agentID)
	if err != nil || agent == nil || agent.Activity.Lifecycle == types.LifecycleExited {
		return
	}
	c.rollupActivityLocked(agent, actx)
	_ = c.store.SaveAgentState(*agent)
	cb := c.onUpdate
	if cb != nil {
		go cb(agent)
	}
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

func (c *Classifier) scheduleSilenceTimerLocked(agentID string, scheduledGen uint64) {
	c.cancelSilenceTimerLocked(agentID)
	if c.turnSilenceTimeout <= 0 || c.syncMode {
		return
	}
	c.silenceTimers[agentID] = time.AfterFunc(c.turnSilenceTimeout, func() {
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
		agent, err := c.store.GetAgent(agentID)
		if err == nil && agent != nil && agent.Activity.Lifecycle != types.LifecycleExited && (agent.Activity.CurrentRun == nil || agent.Activity.CurrentRun.EndedAt == nil) {
			if shouldApplySummary(&agent.Activity, res) {
				applySummary(&agent.Activity, res)
			}
			agent.Activity.ActionStale = false
			_ = c.store.SaveAgentState(*agent)
			c.mu.Lock()
			cb := c.onUpdate
			c.mu.Unlock()
			if cb != nil {
				cb(agent)
			}
		}
		c.mu.Lock()
		return
	}

	c.enqueueJobLocked(agentID, input, ts, false)
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
		if frame.EventType != "run_exited" {
			runID := fmt.Sprintf("%s:%d:%d", agent.ID, frame.PID, now)
			agent.Activity.CurrentRun = &types.IRunInfo{
				ID:             runID,
				PID:            frame.PID,
				HarnessVersion: frame.HarnessVersion,
				StartedAt:      now,
				EndedAt:        nil,
			}
		}
	} else {
		// Update mutable workspace/environment metadata
		agent.Environment = frame.Environment
		agent.Workspace = frame.Workspace
		agent.Project = frame.Project
		if frame.EventType != "run_exited" {
			agent.Activity.UpdatedAt = now
		}
	}

	// Auto-unhide if agent was hidden and shows new activity
	if agent.IsHidden && frame.EventType != "run_exited" {
		isAfterHide := agent.HiddenAt == nil || now > *agent.HiddenAt
		if isAfterHide {
			isActiveEvent := frame.EventType == "run_started" ||
				frame.EventType == "turn_start" ||
				frame.EventType == "user_prompt" ||
				frame.EventType == "tool_call" ||
				frame.EventType == "tool_use" ||
				frame.EventType == "tool_result" ||
				frame.EventType == "thinking" ||
				frame.EventType == "text" ||
				frame.EventType == "agent_message" ||
				frame.EventType == "question_prompt" ||
				frame.EventType == "error"
			if isActiveEvent {
				agent.IsHidden = false
				agent.HiddenAt = nil
				logging.Info("agent_unhidden_on_activity", "id", agent.ID, "event", frame.EventType)
			}
		}
	}

	isSubagent := frame.SubagentID != nil && *frame.SubagentID != ""

	// Update Model, Context Window & Context Fullness
	if !isSubagent {
		if frame.Data.Model != nil && strings.TrimSpace(*frame.Data.Model) != "" {
			m := strings.TrimSpace(*frame.Data.Model)
			agent.Activity.Model = &m
		}
		if frame.Data.ModelContextWindow != nil && *frame.Data.ModelContextWindow > 0 {
			cw := *frame.Data.ModelContextWindow
			agent.Activity.ContextWindow = &cw
		} else if agent.Activity.Model != nil && *agent.Activity.Model != "" {
			cw := models.ResolveContextWindow(*agent.Activity.Model)
			if cw > 0 {
				agent.Activity.ContextWindow = &cw
			}
		}
		if frame.Data.ContextFullness != nil {
			agent.Activity.ContextFullness = frame.Data.ContextFullness
		} else if agent.Activity.ContextWindow != nil && *agent.Activity.ContextWindow > 0 && frame.Data.Usage != nil {
			fn := models.CalculateContextFullness(frame.Data.Usage.Total, *agent.Activity.ContextWindow)
			agent.Activity.ContextFullness = &fn
		}
	}

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

	// Update Title if provided on root agent frames
	if !isSubagent && frame.Data.Title != nil && strings.TrimSpace(*frame.Data.Title) != "" {
		agent.Activity.Title = strings.TrimSpace(*frame.Data.Title)
	}

	// Update Prompt Context if provided
	c.mu.Lock()
	actx, exists := c.agentCtx[agent.ID]
	c.mu.Unlock()

	if !exists {
		preloaded := make(map[string]struct{})
		if c.store != nil && agent.Activity.SubagentCount > 0 {
			if ids, err := c.store.GetDistinctSubagentIDs(agent.ID); err == nil {
				for _, id := range ids {
					preloaded[id] = struct{}{}
				}
			}
		}
		c.mu.Lock()
		actx, exists = c.agentCtx[agent.ID]
		if !exists {
			subagentsMap := make(map[string]*subagentState)
			for _, summary := range agent.Activity.ActiveSubagents {
				st := summary.Status
				if st == "" {
					if summary.Activity == types.ActivityBusy {
						st = types.SubagentRunning
					} else {
						st = types.SubagentCompleted
					}
				}
				subagentsMap[summary.ID] = &subagentState{
					id:          summary.ID,
					activity:    summary.Activity,
					activeTool:  summary.ActiveTool,
					action:      summary.Action,
					lastError:   summary.LastError,
					startedAt:   summary.StartedAt,
					completedAt: summary.CompletedAt,
					status:      st,
				}
				preloaded[summary.ID] = struct{}{}
			}
			initialRootActivity := agent.Activity.Activity
			initialRootAction := agent.Activity.Action
			initialRootQuestion := agent.Activity.Question
			initialRootLastError := agent.Activity.LastError
			if len(subagentsMap) > 0 && agent.Activity.ActiveTool == nil {
				hasSubBusy := false
				for _, sub := range subagentsMap {
					if sub.activity == types.ActivityBusy {
						hasSubBusy = true
						break
					}
				}
				if hasSubBusy && initialRootActivity == types.ActivityBusy {
					initialRootActivity = types.ActivityIdle
					if strings.HasPrefix(initialRootAction, "Running subagent ") {
						initialRootAction = "Idle"
					}
				}
			}
			actx = &agentContext{
				seenSubagents:       preloaded,
				subagents:           subagentsMap,
				rootActivity:        initialRootActivity,
				rootAction:          initialRootAction,
				rootQuestion:        initialRootQuestion,
				rootLastError:       initialRootLastError,
				hasUnseenCompletion: agent.Activity.HasUnseenCompletion,
				completedAt:         agent.Activity.CompletedAt,
			}
			c.agentCtx[agent.ID] = actx
		}
		c.mu.Unlock()
	}

	c.mu.Lock()
	if !isSubagent {
		actx.rootGeneration++
		agent.Activity.Activity = actx.rootActivity
		agent.Activity.Action = actx.rootAction
		agent.Activity.Question = actx.rootQuestion
		agent.Activity.LastError = actx.rootLastError
		agent.Activity.HasUnseenCompletion = actx.hasUnseenCompletion
		agent.Activity.CompletedAt = actx.completedAt
	}
	if actx.seenSubagents == nil {
		actx.seenSubagents = make(map[string]struct{})
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

		case "question_prompt":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.cancelSilenceTimerLocked(agent.ID)
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityWaitingQuestion
			agent.Activity.HasUnseenCompletion = nil
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			if frame.Data.Question != nil {
				agent.Activity.Question = frame.Data.Question
			}
			pendingJob = c.updateSummarization(agent, true)

		case "error":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.cancelSilenceTimerLocked(agent.ID)
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
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			agent.Activity.ActiveToolStartedAt = nil
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
			actx.inFlightTool = false
			actx.inFlightToolName = ""
			c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
			c.mu.Unlock()
			// Tool results (even non-zero exits) are a normal part of shell/agent workflows;
			// the agent remains busy processing the result, not in an error state.
			agent.Activity.Activity = types.ActivityBusy
			agent.Activity.HasUnseenCompletion = nil
			agent.Activity.LastError = nil
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			agent.Activity.ActiveToolStartedAt = nil
			pendingJob = c.updateSummarization(agent, false)

		case "tool_call", "tool_use":
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
					if frame.Data.Question != nil {
						agent.Activity.Question = frame.Data.Question
					}
					pendingJob = c.updateSummarization(agent, true)
				} else {
					// Continuous tool activity: summarize if debounce allows, otherwise mark stale
					pendingJob = c.updateSummarization(agent, false)
				}
			}

		case "turn_start", "user_prompt":
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
			agent.Activity.LastError = nil
			agent.Activity.ActiveTool = nil
			agent.Activity.ActiveTarget = nil
			agent.Activity.ActiveToolStartedAt = nil
			agent.Activity.Action = "Thinking"
			p := types.PhasePlanning
			agent.Activity.Phase = &p

		case "text", "agent_message":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityBusy
			agent.Activity.HasUnseenCompletion = nil
			agent.Activity.LastError = nil
			if frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != "" {
				c.mu.Lock()
				actx.lastAssistantResponse = strings.TrimSpace(*frame.Data.Content)
				c.mu.Unlock()
			}

		case "thinking":
			c.mu.Lock()
			c.cancelIdleTimerLocked(agent.ID)
			hasRecentTools := len(actx.recentTools) > 0
			c.scheduleSilenceTimerLocked(agent.ID, actx.rootGeneration)
			c.mu.Unlock()
			agent.Activity.Activity = types.ActivityBusy
			agent.Activity.HasUnseenCompletion = nil
			agent.Activity.LastError = nil
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
			} else if agent.Activity.Activity == types.ActivityError || agent.Activity.Activity == types.ActivityRateLimited {
				// Do not overwrite ERROR or RATE_LIMITED state on turn_end
				c.mu.Lock()
				c.cancelIdleTimerLocked(agent.ID)
				c.cancelTrailingTimerLocked(agent.ID)
				c.cancelSilenceTimerLocked(agent.ID)
				c.mu.Unlock()
				pendingJob = c.updateSummarization(agent, true)
			} else {
				hasTerminalContent := frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != ""
				isTerminalStop := frame.Data.StopReason != nil && (*frame.Data.StopReason == "end_turn" || *frame.Data.StopReason == "stop")
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
						c.mu.Unlock()
						if agent.Activity.Activity == types.ActivityBusy {
							agent.Activity.HasUnseenCompletion = boolPtr(true)
							agent.Activity.CompletedAt = int64Ptr(now)
						}
						agent.Activity.Activity = types.ActivityIdle
						pendingJob = c.updateSummarization(agent, true)
					} else {
						// Keep agent BUSY during grace period and schedule transition to IDLE if no new events arrive.
						c.mu.Lock()
						c.cancelIdleTimerLocked(agent.ID)
						targetAgentID := agent.ID
						scheduledGen := actx.rootGeneration
						c.idleTimers[targetAgentID] = time.AfterFunc(gracePeriod, func() {
							c.handleIdleTransition(targetAgentID, scheduledGen)
						})
						c.mu.Unlock()
						pendingJob = c.updateSummarization(agent, false)
					}
				}
			}
		}
	} else {
		// Subagent event: track subagent state (do not cancel parent idle timer #64)
		c.mu.Lock()
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
					c.cancelSubagentTimerLocked(agent.ID, subID)
				}
			}

			switch frame.EventType {
			case "tool_call", "tool_use":
				sub.completedAt = nil
				sub.status = types.SubagentRunning
				c.cancelSubagentTimerLocked(agent.ID, subID)
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
				}
			case "tool_result":
				sub.inFlightTool = false
				if frame.Data.IsError != nil && *frame.Data.IsError {
					if isRateLimitFrame(frame.Data) {
						sub.activity = types.ActivityRateLimited
					} else {
						sub.activity = types.ActivityError
					}
					if frame.Data.ToolOutput != nil {
						out := *frame.Data.ToolOutput
						if len(out) > 2000 {
							out = out[:2000]
						}
						sub.lastError = &out
					}
					sub.activeTool = nil
				} else {
					sub.activity = types.ActivityBusy
					sub.activeTool = nil
				}
			case "error":
				sub.inFlightTool = false
				if isRateLimitFrame(frame.Data) {
					sub.activity = types.ActivityRateLimited
				} else {
					sub.activity = types.ActivityError
				}
				if frame.Data.Content != nil {
					out := *frame.Data.Content
					sub.lastError = &out
				} else if frame.Data.ToolOutput != nil {
					out := *frame.Data.ToolOutput
					sub.lastError = &out
				}
				sub.activeTool = nil
			case "question_prompt", "waiting_question":
				sub.inFlightTool = false
				sub.activity = types.ActivityWaitingQuestion
				if frame.Data.Question != nil && strings.TrimSpace(*frame.Data.Question) != "" {
					q := strings.TrimSpace(*frame.Data.Question)
					sub.question = &q
				} else if frame.Data.Content != nil && strings.TrimSpace(*frame.Data.Content) != "" {
					q := strings.TrimSpace(*frame.Data.Content)
					sub.question = &q
				}
				sub.activeTool = nil
			case "turn_start", "user_prompt":
				sub.activity = types.ActivityBusy
				sub.activeTool = nil
				sub.lastError = nil
				sub.completedAt = nil
				sub.status = types.SubagentRunning
				c.cancelSubagentTimerLocked(agent.ID, subID)
			case "text", "agent_message":
				sub.activity = types.ActivityBusy
				sub.completedAt = nil
				sub.status = types.SubagentRunning
				c.cancelSubagentTimerLocked(agent.ID, subID)
			case "turn_end", "run_exited":
				sub.activity = types.ActivityIdle
				sub.activeTool = nil
				sub.inFlightTool = false
				if sub.completedAt == nil {
					comp := now
					sub.completedAt = &comp
				}
				sub.status = types.SubagentCompleted
				c.scheduleSubagentDwellTimerLocked(agent.ID, subID, sub, now)
			}
		}
		c.mu.Unlock()

		if isNewSubagent {
			agent.Activity.SubagentCount++
		}
	}

	c.mu.Lock()
	if !isSubagent && agent.Activity.Lifecycle != types.LifecycleExited {
		actx.rootActivity = agent.Activity.Activity
		actx.rootAction = agent.Activity.Action
		actx.rootQuestion = agent.Activity.Question
		actx.rootLastError = agent.Activity.LastError
		actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
		actx.completedAt = agent.Activity.CompletedAt
	}
	if agent.Activity.Lifecycle != types.LifecycleExited {
		c.pruneExpiredSubagentsLocked(agent.ID, actx, now)
		c.rollupActivityLocked(agent, actx)
		actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
		actx.completedAt = agent.Activity.CompletedAt
	} else {
		agent.Activity.ActiveSubagents = nil
		actx.hasUnseenCompletion = nil
	}
	c.mu.Unlock()

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

func (c *Classifier) prepareSummaryInput(agent *types.IAgentState) (summarizer.SummaryInput, string) {
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
		}
	}

	// 5. Within debounce window: buffer tool call as pending candidate and schedule trailing-edge flush
	actx.pendingCandidate = &pendingSummaryCandidate{
		input:     input,
		timestamp: nowMs,
		toolKey:   currentToolKey,
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

func (c *Classifier) enqueueJob(agentID string, input summarizer.SummaryInput, timestamp int64, force bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enqueueJobLocked(agentID, input, timestamp, force)
}

func (c *Classifier) enqueueJobLocked(agentID string, input summarizer.SummaryInput, timestamp int64, force bool) {
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
	if agent.Activity.Lifecycle == types.LifecycleExited || (agent.Activity.CurrentRun != nil && agent.Activity.CurrentRun.EndedAt != nil) {
		return
	}
	if agent.Activity.Activity != job.input.Activity {
		return
	}

	// Deduplication check: reject near-duplicates when phase and target are unchanged
	if !shouldApplySummary(&agent.Activity, res) {
		if agent.Activity.ActionStale {
			agent.Activity.ActionStale = false
			_ = c.store.SaveAgentState(*agent)
		}
		return
	}

	// Apply updated summary
	applySummary(&agent.Activity, res)
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
	if agent.Activity.Activity == types.ActivityBusy {
		agent.Activity.HasUnseenCompletion = boolPtr(true)
		agent.Activity.CompletedAt = int64Ptr(now)
	}
	agent.Activity.Activity = types.ActivityIdle
	agent.Activity.ActiveTool = nil
	agent.Activity.ActiveTarget = nil
	agent.Activity.Question = nil
	agent.Activity.UpdatedAt = now

	// Compute fallback summary purely without mutating agentContext or cancelling trailing timers
	input, currentToolKey := c.prepareSummaryInput(agent)

	if c.beforeIdleSummarizeHook != nil {
		c.beforeIdleSummarizeHook(agentID)
	}

	c.mu.Lock()
	actx, ok = c.agentCtx[agentID]
	if !ok || actx.rootGeneration != scheduledGen || actx.rootActivity != types.ActivityBusy {
		c.mu.Unlock()
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
	actx.rootAction = agent.Activity.Action
	actx.rootQuestion = nil
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt
	c.rollupActivityLocked(agent, actx)
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt
	c.mu.Unlock()

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

func (c *Classifier) handleSilenceTransition(agentID string, scheduledGen uint64) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	delete(c.silenceTimers, agentID)

	actx, ok := c.agentCtx[agentID]
	if !ok || actx.rootGeneration != scheduledGen || actx.rootActivity != types.ActivityBusy {
		c.mu.Unlock()
		return
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
	if agent.Activity.Activity == types.ActivityBusy {
		agent.Activity.HasUnseenCompletion = boolPtr(true)
		agent.Activity.CompletedAt = int64Ptr(now)
	}
	agent.Activity.Activity = types.ActivityIdle
	agent.Activity.ActiveTool = nil
	agent.Activity.ActiveTarget = nil
	agent.Activity.Question = nil
	agent.Activity.UpdatedAt = now

	input, currentToolKey := c.prepareSummaryInput(agent)

	c.mu.Lock()
	actx, ok = c.agentCtx[agentID]
	if !ok || actx.rootGeneration != scheduledGen || actx.rootActivity != types.ActivityBusy {
		c.mu.Unlock()
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
	actx.rootAction = agent.Activity.Action
	actx.rootQuestion = nil
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt
	c.rollupActivityLocked(agent, actx)
	actx.hasUnseenCompletion = agent.Activity.HasUnseenCompletion
	actx.completedAt = agent.Activity.CompletedAt
	c.mu.Unlock()

	if err := c.store.SaveAgentState(*agent); err != nil {
		logging.Error("saving_silent_idle_agent_failed", "err", err, "agentId", agentID)
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

func (c *Classifier) rollupActivityLocked(agent *types.IAgentState, actx *agentContext) {
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
		return
	} else if subError != nil {
		agent.Activity.Activity = types.ActivityError
		agent.Activity.HasUnseenCompletion = nil
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
		return
	} else if subQuestion != nil {
		agent.Activity.Activity = types.ActivityWaitingQuestion
		agent.Activity.HasUnseenCompletion = nil
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
		return
	} else if subRateLimited != nil {
		agent.Activity.Activity = types.ActivityRateLimited
		agent.Activity.HasUnseenCompletion = nil
		if subRateLimited.LastError != nil {
			errStr := fmt.Sprintf("[Subagent %s] %s", subRateLimited.ID, *subRateLimited.LastError)
			agent.Activity.LastError = &errStr
		}
		return
	}

	// 3. Busy vs Idle
	if actx.rootActivity == types.ActivityBusy {
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.Action = actx.rootAction
		agent.Activity.HasUnseenCompletion = nil
	} else if subBusy != nil {
		agent.Activity.Activity = types.ActivityBusy
		agent.Activity.HasUnseenCompletion = nil
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
			now := time.Now().UnixMilli()
			agent.Activity.HasUnseenCompletion = boolPtr(true)
			agent.Activity.CompletedAt = int64Ptr(now)
		}
		agent.Activity.Activity = types.ActivityIdle
		if actx.rootAction != "" && actx.rootAction != "Thinking" {
			agent.Activity.Action = actx.rootAction
		} else {
			agent.Activity.Action = "Idle"
		}
	}
}

// equalPtr returns true if both pointers are nil or both are non-nil and point to equal values.
func equalPtr[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a != nil && b != nil {
		return *a == *b
	}
	return false
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

// isToolUseStopReason reports whether a stop reason string indicates the model stopped to call a tool.
// Supported values across harnesses: "tool_use" (Claude Code), "toolUse" (Pi), and "tool_calls"/"tool_call" (OpenAI/Codex/OpenCode).
func isToolUseStopReason(reason string) bool {
	switch reason {
	case "tool_use", "toolUse", "tool_calls", "tool_call":
		return true
	default:
		return false
	}
}

// isRateLimitFrame reports whether a telemetry frame represents an API rate limiting / throttling event.
func isRateLimitFrame(data types.TelemetryFrameData) bool {
	if data.IsRateLimit != nil && *data.IsRateLimit {
		return true
	}
	if data.ExitCode != nil && *data.ExitCode == 429 {
		return true
	}
	if data.RateLimitResetAt != nil || data.RetryAfterSeconds != nil {
		return true
	}
	for _, strPtr := range []*string{data.Content, data.ToolOutput} {
		if strPtr != nil && isRateLimitString(*strPtr) {
			return true
		}
	}
	return false
}

// isRateLimitString checks if an error message contains rate limiting indicators.
func isRateLimitString(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "rate_limit") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "429") ||
		strings.Contains(lower, "overloaded") ||
		strings.Contains(lower, "quota")
}

