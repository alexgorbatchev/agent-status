package classifier

import (
	"fmt"
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
	saveGen   uint64
}

type subagentState struct {
	id                string
	activity          types.ActivityState
	activeTool        *string
	action            string
	lastError         *string
	question          *string
	rateLimitResetAt  *int64
	retryAfterSeconds *int64
	inFlightTool      bool
	startedAt         int64
	completedAt       *int64
	status            types.SubagentStatus
}

type agentContext struct {
	persistMu      sync.Mutex
	saveGeneration uint64
	// frameInFlight is set from the moment a frame bumps saveGeneration until
	// it has saved. Background writers must not save or change the cache then:
	// the frame already copied the cache into the row it is about to save.
	frameInFlight           bool
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
	rootRateLimitResetAt    *int64
	rootRetryAfterSeconds   *int64
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
	turnEndTimestamp        int64
	lastEventTimestamp      int64
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
	saveGen   uint64
	flushDone chan struct{}
}

type agentWorker struct {
	jobs chan summaryJob
	stop chan struct{}
}

const (
	DefaultIdleGracePeriod        = 5 * time.Second
	DefaultDebounceWindow         = 10 * time.Second
	DefaultTurnFloor              = 20 * time.Second
	DefaultTurnSilenceTimeout     = 30 * time.Minute
	DefaultRateLimitSilenceBuffer = 2 * time.Minute
	DefaultSubagentDwell          = 30 * time.Second
)

type Classifier struct {
	store                 *store.Store
	pricing               *pricing.Engine
	summarizer            *summarizer.Summarizer
	agentCtx              map[string]*agentContext
	workers               map[string]*agentWorker
	idleTimers            map[string]*time.Timer
	trailingTimers        map[string]*time.Timer
	silenceTimers         map[string]*time.Timer
	silenceTimerDurations map[string]time.Duration
	subagentTimers        map[string]*time.Timer
	frameLocks            map[string]*frameLock
	idleGracePeriod       time.Duration
	debounceWindow        time.Duration
	turnFloor             time.Duration
	turnSilenceTimeout    time.Duration
	subagentDwell         time.Duration
	onUpdate              func(agent *types.IAgentState)
	mu                    sync.Mutex
	wg                    sync.WaitGroup
	closed                bool
	syncMode              bool

	beforeIdleSummarizeHook     func(agentID string)
	beforeIdleSaveHook          func(agentID string)
	beforeSilenceSaveHook       func(agentID string)
	beforeSummarySaveHook       func(agentID string)
	beforeFramePublishHook      func(agentID string)
	beforeFrameSaveHook         func(agentID string)
	afterFrameReadHook          func(agentID string)
	beforeSubagentDwellSaveHook func(agentID, subagentID string)
	afterSubagentDwellHook      func(agentID, subagentID string)
}

func New(s *store.Store, p *pricing.Engine, sum *summarizer.Summarizer) *Classifier {
	debounce := DefaultDebounceWindow
	if sum != nil && sum.DebounceWindow() > 0 {
		debounce = sum.DebounceWindow()
	}
	return &Classifier{
		store:                 s,
		pricing:               p,
		summarizer:            sum,
		agentCtx:              make(map[string]*agentContext),
		workers:               make(map[string]*agentWorker),
		idleTimers:            make(map[string]*time.Timer),
		trailingTimers:        make(map[string]*time.Timer),
		silenceTimers:         make(map[string]*time.Timer),
		silenceTimerDurations: make(map[string]time.Duration),
		subagentTimers:        make(map[string]*time.Timer),
		frameLocks:            make(map[string]*frameLock),
		idleGracePeriod:       DefaultIdleGracePeriod,
		debounceWindow:        debounce,
		turnFloor:             DefaultTurnFloor,
		turnSilenceTimeout:    DefaultTurnSilenceTimeout,
		subagentDwell:         DefaultSubagentDwell,
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

// SilenceTimerDuration returns the configured duration of an active silence timer on the agent, or 0 if none.
func (c *Classifier) SilenceTimerDuration(agentID string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.silenceTimerDurations == nil {
		return 0
	}
	return c.silenceTimerDurations[agentID]
}

// IdleGracePeriod returns the idle grace period duration.
func (c *Classifier) IdleGracePeriod() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.idleGracePeriod
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
	c.silenceTimerDurations = make(map[string]time.Duration)
	for id, t := range c.subagentTimers {
		t.Stop()
		delete(c.subagentTimers, id)
	}
	c.mu.Unlock()

	c.wg.Wait()
	return nil
}

// ProcessFrame applies 0-ms deterministic rules and schedules asynchronous action summarization.
func (c *Classifier) ProcessFrame(frame types.ITelemetryFrame) (*types.IAgentState, error) {
	unlockFrames := c.lockFrames(frame.AgentID)
	defer unlockFrames()

	agent, err := c.store.GetAgent(frame.AgentID)
	if err != nil {
		return nil, fmt.Errorf("getting existing agent: %w", err)
	}
	if c.afterFrameReadHook != nil {
		c.afterFrameReadHook(frame.AgentID)
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
				frame.EventType == "tool_call" ||
				frame.EventType == "tool_result" ||
				frame.EventType == "thinking" ||
				frame.EventType == "question_prompt" ||
				frame.EventType == "waiting_question" ||
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

	actx, err := c.getOrCreateAgentContext(agent)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	actx.saveGeneration++
	actx.frameInFlight = true
	defer func() {
		c.mu.Lock()
		actx.frameInFlight = false
		c.mu.Unlock()
	}()
	actx.lastEventTimestamp = now
	if !isSubagent && isRootStateEvent(frame.EventType) {
		actx.rootGeneration++
	}
	if !isSubagent {
		agent.Activity.Activity = actx.rootActivity
		agent.Activity.Action = actx.rootAction
		agent.Activity.Question = actx.rootQuestion
		agent.Activity.LastError = actx.rootLastError
		if actx.rootActivity == types.ActivityRateLimited {
			agent.Activity.RateLimitResetAt = actx.rootRateLimitResetAt
			agent.Activity.RetryAfterSeconds = actx.rootRetryAfterSeconds
		} else {
			clearRateLimit(&agent.Activity)
		}
	}
	initialCompletion := actx.hasUnseenCompletion
	initialCompletedAt := actx.completedAt
	agent.Activity.HasUnseenCompletion = initialCompletion
	agent.Activity.CompletedAt = initialCompletedAt
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
	if !isSubagent {
		pendingJob, err = c.applyRootEvent(agent, actx, frame, now)
		if err != nil {
			return nil, err
		}
	} else {
		// Subagent frames never cancel the parent idle timer.
		c.mu.Lock()
		isNewSubagent := c.applySubagentEventLocked(agent.ID, actx, frame, now)
		c.mu.Unlock()
		if isNewSubagent {
			agent.Activity.SubagentCount++
		}
	}

	if c.beforeFramePublishHook != nil {
		c.beforeFramePublishHook(agent.ID)
	}
	c.mu.Lock()
	// An unchanged completion belongs to the cache, not this frame's snapshot.
	// Reconcile it before publishing so dismissal during classification cannot
	// be undone. New completions allocated by this frame remain authoritative.
	if agent.Activity.HasUnseenCompletion == initialCompletion && agent.Activity.CompletedAt == initialCompletedAt {
		agent.Activity.HasUnseenCompletion = actx.hasUnseenCompletion
		agent.Activity.CompletedAt = actx.completedAt
	}
	if !isSubagent && agent.Activity.Lifecycle != types.LifecycleExited {
		recordRootStateLocked(actx, &agent.Activity)
		actx.rootLastError = agent.Activity.LastError
	}
	if agent.Activity.Lifecycle != types.LifecycleExited {
		c.pruneExpiredSubagentsLocked(agent.ID, actx, now)
		c.rollupActivityLocked(agent, actx, now)
		captureCompletionLocked(actx, &agent.Activity)
	} else {
		agent.Activity.ActiveSubagents = nil
		clearRateLimit(&agent.Activity)
		actx.hasUnseenCompletion = nil
		actx.completedAt = nil
		actx.rootRateLimitResetAt = nil
		actx.rootRetryAfterSeconds = nil
	}
	c.mu.Unlock()

	if c.beforeFrameSaveHook != nil {
		c.beforeFrameSaveHook(agent.ID)
	}
	actx.persistMu.Lock()
	if agent.Activity.Lifecycle != types.LifecycleExited {
		// A dismissal may have committed after this frame prepared its snapshot.
		c.mu.Lock()
		syncCompletionLocked(actx, agent)
		c.mu.Unlock()
	}
	saveErr := c.store.SaveAgentState(*agent)
	actx.persistMu.Unlock()
	if saveErr != nil {
		return nil, fmt.Errorf("saving agent state: %w", saveErr)
	}

	// Record the raw event after agent exists in DB
	if err := c.store.RecordTelemetryEvent(frame); err != nil {
		return nil, fmt.Errorf("recording telemetry event: %w", err)
	}

	// Enqueue async LLM summarization job after agent is safely committed to database
	if pendingJob != nil {
		c.enqueueJob(agent.ID, pendingJob.input, pendingJob.timestamp, pendingJob.force, pendingJob.saveGen)
	}

	return agent, nil
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
	if data.IsRateLimit != nil {
		return *data.IsRateLimit
	}
	if data.RateLimitResetAt != nil || data.RetryAfterSeconds != nil {
		return true
	}
	return false
}

// isRootStateEvent reports whether an event type drives root state machine transitions.
// Conversational text/messages and metadata events do not alter classifier activity states.
func isRootStateEvent(eventType string) bool {
	switch eventType {
	case "run_started", "run_exited", "turn_start", "thinking", "tool_call", "tool_result", "turn_end", "question_prompt", "waiting_question", "error":
		return true
	default:
		return false
	}
}

// truncateRunes truncates string s to at most maxRunes, preserving valid UTF-8 on rune boundaries.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i]
		}
		count++
	}
	return s
}
