package replay

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/classifier"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/summarizer"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/git"
)

// ReplayOptions configures an offline replay run.
type ReplayOptions struct {
	FilePath    string
	Harness     types.HarnessType
	SessionID   string
	CWD         string
	EnvLabel    string
	Diff        bool
	UntilLine   int
	UntilEvent  string
	StopOnError bool
	ExportDB    string
	Summarize   bool
	AgentMode       bool
	StrictLifecycle bool
	OnDiff          func(diff output.StateDiff)
	OnStep      func(step int, frame types.ITelemetryFrame)
}

// ReplayResult contains the terminal state and audit metrics of a replay.
type ReplayResult struct {
	Harness         types.HarnessType          `json:"harness"`
	SessionID       string                     `json:"sessionId"`
	AgentID         string                     `json:"agentId"`
	Agent           *types.IAgentState         `json:"agent"`
	FramesProcessed int                        `json:"framesProcessed"`
	EventCounts     map[string]int             `json:"eventCounts"`
	Subagents       []string                   `json:"subagents"`
	Diffs           []output.StateDiff         `json:"diffs,omitempty"`
	Verification    *VerificationReport        `json:"verification"`
	Duration        time.Duration              `json:"duration"`
	ExportDBPath      string                     `json:"exportDbPath,omitempty"`
	Errors            []string                   `json:"errors,omitempty"`
	HasNativeStart    bool                       `json:"hasNativeStart,omitempty"`
	HasNativeEnd      bool                       `json:"hasNativeEnd,omitempty"`
	HasSyntheticStart bool                       `json:"hasSyntheticStart,omitempty"`
	HasSyntheticEnd   bool                       `json:"hasSyntheticEnd,omitempty"`
	StrictLifecycle   bool                       `json:"strictLifecycle,omitempty"`
	LastEventTimestamp int64                     `json:"lastEventTimestamp,omitempty"`
	Comparison        *ComparisonResult          `json:"comparison,omitempty"`
}

// Summary builds a ReplaySummary presentation model from the replay result.
func (r *ReplayResult) Summary() output.ReplaySummary {
	s := output.ReplaySummary{
		Harness:          r.Harness,
		SessionID:        r.SessionID,
		AgentID:          r.AgentID,
		SubagentCount:    len(r.Subagents),
		FramesProcessed:  r.FramesProcessed,
		VerificationOK:   r.Verification != nil && r.Verification.Passed,
		VerificationLogs: 0,
		DurationMs:       r.Duration.Milliseconds(),
	}

	if r.Verification != nil {
		s.VerificationLogs = r.Verification.LogCount
	}

	if r.Agent != nil {
		s.ProjectName = r.Agent.Project.Name
		s.Branch = r.Agent.Workspace.Branch
		s.CWD = r.Agent.Workspace.CWD
		s.Activity = r.Agent.Activity.Activity
		s.Lifecycle = r.Agent.Activity.Lifecycle
		if r.Agent.Activity.Lifecycle == types.LifecycleExited {
			s.IsLifecycleSynthetic = r.HasSyntheticEnd
		}
		s.HasUnseenCompletion = r.Agent.Activity.HasUnseenCompletion
		s.Title = r.Agent.Activity.Title
		s.Action = r.Agent.Activity.Action
		s.TokensInput = r.Agent.Activity.Usage.Input
		s.TokensOutput = r.Agent.Activity.Usage.Output
		s.TokensTotal = r.Agent.Activity.Usage.Total
		if r.Agent.Activity.Cost != nil {
			s.CostUSD = r.Agent.Activity.Cost.Total
		}
		if r.Agent.Activity.SubagentCount > s.SubagentCount {
			s.SubagentCount = r.Agent.Activity.SubagentCount
		}
	}

	return s
}

// RunReplay executes the offline ingestion lifecycle against a session file or database.
func RunReplay(ctx context.Context, opts ReplayOptions) (*ReplayResult, error) {
	start := time.Now()

	// 1. Load session data across supported harnesses
	session, err := LoadSession(ctx, opts.FilePath, opts.Harness, opts.SessionID, opts.CWD, opts.StrictLifecycle)
	if err != nil {
		return nil, fmt.Errorf("loading session: %w", err)
	}

	// 2. Initialize in-memory or exported SQLite store
	dbTarget := ":memory:"
	if opts.ExportDB != "" {
		dbTarget = filepath.Clean(opts.ExportDB)
	}
	st, err := store.Open(dbTarget)
	if err != nil {
		return nil, fmt.Errorf("opening store at %q: %w", dbTarget, err)
	}
	defer func() { _ = st.Close() }()

	// 3. Initialize pricing engine
	priceEngine, err := pricing.NewPricingEngine()
	if err != nil {
		return nil, fmt.Errorf("initializing pricing engine: %w", err)
	}

	// 4. Initialize summarizer (default offline deterministic fallback)
	sumEngine, err := summarizer.New(summarizer.Config{
		NoSummarize: !opts.Summarize,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing summarizer: %w", err)
	}

	// 5. Initialize classifier with synchronous execution and zero debounce
	cls := classifier.New(st, priceEngine, sumEngine)
	cls.SetSyncMode(true)
	cls.SetIdleGracePeriod(0)
	defer func() { _ = cls.Close() }()

	// 6. Initialize git resolver
	gitResolver := git.NewResolver(0)

	// 7. Seed environment info
	envLabel := opts.EnvLabel
	if envLabel == "" {
		envLabel = "debug"
	}
	envID := "env-" + envLabel
	platform := runtime.GOOS
	if platform != "darwin" && platform != "linux" {
		platform = "linux"
	}
	env := types.IEnvironmentInfo{
		ID:              envID,
		Label:           envLabel,
		Hostname:        envLabel,
		Platform:        platform,
		User:            "local",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	// Restore UpsertEnvironment
	if err := st.UpsertEnvironment(env); err != nil {
		return nil, fmt.Errorf("upserting environment: %w", err)
	}

	agentID := fmt.Sprintf("%s:%s:%s", env.ID, session.Harness, session.SessionID)

	// 8. Replay frames
	var prevAgent *types.IAgentState
	var finalAgent *types.IAgentState
	var diffs []output.StateDiff
	var replayErrors []string
	eventCounts := make(map[string]int)
	framesProcessed := 0
	var lastEventTS int64

	for stepIdx, rev := range session.Events {
		lastEventTS = rev.Event.Timestamp
		// Line-based breakpoint
		if opts.UntilLine > 0 && rev.SourceLine > opts.UntilLine {
			break
		}

		proj, ws, _ := gitResolver.Resolve(ctx, rev.Event.CWD, rev.Event.GitBranch)

		frame := types.ITelemetryFrame{
			Version:        "1.0",
			EnvID:          env.ID,
			Environment:    env,
			AgentID:        agentID,
			SessionID:      session.SessionID,
			SubagentID:     rev.SubagentID,
			PID:            1000,
			Harness:        session.Harness,
			HarnessVersion: session.HarnessVersion,
			Project:        proj,
			Workspace:      ws,
			Timestamp:      rev.Event.Timestamp,
			EventType:      rev.Event.EventType,
			Data:           rev.Event.Data,
		}

		newAgent, pErr := cls.ProcessFrame(frame)
		if pErr != nil {
			errStr := fmt.Sprintf("step %d (line %d) event %s: %v", stepIdx+1, rev.SourceLine, frame.EventType, pErr)
			replayErrors = append(replayErrors, errStr)
			if opts.StopOnError {
				return nil, fmt.Errorf("halting on error: %s", errStr)
			}
		}

		framesProcessed++
		eventCounts[frame.EventType]++
		finalAgent = newAgent

		if opts.Diff || opts.OnDiff != nil {
			diff := ComputeStateDiff(stepIdx+1, rev.SourceLine, frame.EventType, rev.SubagentID, prevAgent, newAgent)
			diff.IsSynthetic = rev.IsSynthetic
			diffs = append(diffs, diff)
			if opts.OnDiff != nil {
				opts.OnDiff(diff)
			}
		}

		prevAgent = CloneAgentState(newAgent)

		if opts.OnStep != nil {
			opts.OnStep(stepIdx+1, frame)
		}

		// Event-type breakpoint
		if opts.UntilEvent != "" && frame.EventType == opts.UntilEvent {
			break
		}
	}

	// 9. Flush async jobs
	cls.FlushAll()

	hasNativeStart := false
	hasNativeEnd := false
	hasSyntheticStart := false
	hasSyntheticEnd := false
	for _, rev := range session.Events {
		if rev.Event.EventType == "run_started" {
			if rev.IsSynthetic {
				hasSyntheticStart = true
			} else {
				hasNativeStart = true
			}
		}
		if rev.Event.EventType == "run_exited" {
			if rev.IsSynthetic {
				hasSyntheticEnd = true
			} else {
				hasNativeEnd = true
			}
		}
	}

	if !hasNativeEnd && !hasSyntheticEnd && finalAgent != nil {
		finalAgent.Activity.Lifecycle = types.LifecycleRunning
	}

	// 10. Run server-side read query verification
	verReport, vErr := VerifyAgentState(ctx, st.DB(), agentID, VerifyOptions{
		StrictLifecycle: opts.StrictLifecycle,
		HasExplicitExit: hasNativeEnd,
	})
	if vErr != nil {
		replayErrors = append(replayErrors, fmt.Sprintf("verification driver error: %v", vErr))
	}

	result := &ReplayResult{
		Harness:           session.Harness,
		SessionID:         session.SessionID,
		AgentID:           agentID,
		Agent:             finalAgent,
		FramesProcessed:   framesProcessed,
		EventCounts:       eventCounts,
		Subagents:         session.Subagents,
		Diffs:             diffs,
		Verification:      verReport,
		Duration:          time.Since(start),
		ExportDBPath:      opts.ExportDB,
		Errors:            replayErrors,
		HasNativeStart:    hasNativeStart,
		HasNativeEnd:      hasNativeEnd,
		HasSyntheticStart: hasSyntheticStart,
		HasSyntheticEnd:   hasSyntheticEnd,
		StrictLifecycle:   opts.StrictLifecycle,
		LastEventTimestamp: lastEventTS,
	}

	return result, nil
}
