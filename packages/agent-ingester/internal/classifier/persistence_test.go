package classifier

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

func TestClassifier_SynchronousTrailingSummaryPersistence(t *testing.T) {
	for _, tt := range []struct {
		name          string
		rejectSave    bool
		staleFrame    bool
		wantSummary   bool
		wantBroadcast int32
	}{
		{name: "saved", wantSummary: true, wantBroadcast: 1},
		{name: "database_rejection", rejectSave: true},
		{name: "newer_frame", staleFrame: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, st, frame := completedAgent(t, ":memory:")
			c.SetSyncMode(true)
			before, err := st.GetAgent(frame.AgentID)
			if err != nil || before == nil {
				t.Fatalf("GetAgent = %v, %v", before, err)
			}
			wantAction := before.Activity.Action
			if tt.wantSummary {
				wantAction = "Responded to user"
			}
			if before.Activity.Action == "Responded to user" {
				t.Fatalf("fixture already shows the summary action")
			}
			if tt.rejectSave {
				if _, err := st.DB().Exec(`CREATE TRIGGER reject_summary BEFORE UPDATE OF action ON agents
					WHEN NEW.action = 'Responded to user' BEGIN SELECT RAISE(ABORT, 'summary rejected'); END`); err != nil {
					t.Fatal(err)
				}
			}

			c.mu.Lock()
			actx := c.agentCtx[frame.AgentID]
			actx.pendingCandidate = &pendingSummaryCandidate{
				input: summarizer.SummaryInput{
					AgentID: frame.AgentID, Activity: types.ActivityIdle,
					LastAssistantResponse: "Work is complete",
				},
				timestamp: frame.Timestamp, toolKey: "completed-response",
				saveGen: actx.saveGeneration,
			}
			if tt.staleFrame {
				actx.saveGeneration++
			}
			c.mu.Unlock()

			var broadcasts atomic.Int32
			c.SetOnUpdate(func(*types.IAgentState) { broadcasts.Add(1) })
			c.Flush(frame.AgentID)

			saved, err := st.GetAgent(frame.AgentID)
			if err != nil || saved == nil {
				t.Fatalf("GetAgent = %v, %v", saved, err)
			}
			if saved.Activity.Action != wantAction {
				t.Errorf("persisted action = %q, want %q", saved.Activity.Action, wantAction)
			}
			if got := broadcasts.Load(); got != tt.wantBroadcast {
				t.Errorf("broadcasts = %d, want %d", got, tt.wantBroadcast)
			}
			assertCompletion(t, c, st, frame.AgentID, frame.Timestamp)
		})
	}
}

func TestClassifier_SubagentHistoryLoadFailureIsRetried(t *testing.T) {
	original, st, frame := completedAgent(t, ":memory:")
	original.SetSubagentDwell(0)
	worker := "worker"
	subFrame := frame
	subFrame.SubagentID = &worker
	subFrame.Data = types.TelemetryFrameData{}
	for _, event := range []string{"turn_start", "turn_end"} {
		subFrame.Seq++
		subFrame.Timestamp += 1000
		subFrame.EventType = event
		if _, err := original.ProcessFrame(subFrame); err != nil {
			t.Fatal(err)
		}
	}
	assertSubagents := func(wantCount int) {
		t.Helper()
		agent, err := st.GetAgent(frame.AgentID)
		if err != nil || agent == nil {
			t.Fatalf("GetAgent = %v, %v", agent, err)
		}
		if agent.Activity.SubagentCount != wantCount || len(agent.Activity.ActiveSubagents) != 0 {
			t.Fatalf("subagentCount=%d active=%d, want count %d and no active subagents",
				agent.Activity.SubagentCount, len(agent.Activity.ActiveSubagents), wantCount)
		}
	}
	// With no dwell the completed subagent is pruned at once, so only the
	// telemetry history still knows it was seen.
	assertSubagents(1)
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}

	c := New(st, nil, original.summarizer)
	c.SetSubagentDwell(0)
	t.Cleanup(func() { _ = c.Close() })
	if _, err := st.DB().Exec("ALTER TABLE telemetry_events RENAME TO telemetry_events_offline"); err != nil {
		t.Fatal(err)
	}
	subFrame.Seq++
	subFrame.Timestamp += 1000
	subFrame.EventType = "turn_start"
	if _, err := c.ProcessFrame(subFrame); err == nil || !strings.Contains(err.Error(), "loading subagent history") {
		t.Fatalf("ProcessFrame with unreadable history = %v, want subagent history error", err)
	}
	c.mu.Lock()
	_, cached := c.agentCtx[frame.AgentID]
	c.mu.Unlock()
	if cached {
		t.Fatal("context with incomplete subagent history was cached")
	}
	assertSubagents(1)

	if _, err := st.DB().Exec("ALTER TABLE telemetry_events_offline RENAME TO telemetry_events"); err != nil {
		t.Fatal(err)
	}
	subFrame.EventType = "turn_end"
	if _, err := c.ProcessFrame(subFrame); err != nil {
		t.Fatal(err)
	}
	assertSubagents(1)

	helper := "helper"
	subFrame.SubagentID = &helper
	subFrame.Seq++
	subFrame.Timestamp += 1000
	if _, err := c.ProcessFrame(subFrame); err != nil {
		t.Fatal(err)
	}
	assertSubagents(2)
}

func TestClassifier_ConcurrentFramesForOneAgentAreSerialized(t *testing.T) {
	c, st, frame := completedAgent(t, ":memory:")
	before, err := st.GetAgent(frame.AgentID)
	if err != nil || before == nil {
		t.Fatalf("GetAgent = %v, %v", before, err)
	}

	// The first frame pauses after reading the agent row, as a slow connection
	// would; the second frame arrives from another connection meanwhile.
	var paused atomic.Bool
	started, resume := make(chan struct{}), make(chan struct{})
	c.afterFrameReadHook = func(string) {
		if paused.CompareAndSwap(false, true) {
			close(started)
			<-resume
		}
	}
	usageFrame := func(total int64, title string) types.ITelemetryFrame {
		f := frame
		f.Seq++
		f.Timestamp += 1000
		f.EventType = "text"
		f.Data = types.TelemetryFrameData{Usage: &types.ITokenUsage{Input: total, Total: total}}
		if title != "" {
			f.Data.Title = &title
		}
		return f
	}
	first, second := usageFrame(100, ""), usageFrame(7, "Second connection")
	second.Seq++

	results := make(chan error, 2)
	go func() {
		_, err := c.ProcessFrame(first)
		results <- err
	}()
	<-started
	go func() {
		_, err := c.ProcessFrame(second)
		results <- err
	}()
	// The second frame either waits for the first or, without serialization,
	// finishes on the stale row the first frame is about to overwrite.
	waitForDismissalCondition(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		lock := c.frameLocks[frame.AgentID]
		return (lock != nil && lock.refs == 2) || len(results) > 0
	})
	close(resume)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}

	saved, err := st.GetAgent(frame.AgentID)
	if err != nil || saved == nil {
		t.Fatalf("GetAgent = %v, %v", saved, err)
	}
	if want := before.Activity.Usage.Total + 107; saved.Activity.Usage.Total != want {
		t.Errorf("usage total = %d, want %d (both frames counted)", saved.Activity.Usage.Total, want)
	}
	if saved.Activity.Title != "Second connection" {
		t.Errorf("title = %q, want the second frame's title", saved.Activity.Title)
	}
	c.mu.Lock()
	remaining := len(c.frameLocks)
	c.mu.Unlock()
	if remaining != 0 {
		t.Errorf("frame locks left after both frames finished: %d", remaining)
	}
}

func TestClassifier_SubagentDwellExpiryCachesSavedCompletion(t *testing.T) {
	c, st, frame := completedAgent(t, ":memory:")
	c.SetSubagentDwell(time.Hour)
	for _, step := range []struct{ sub, event string }{
		{"first", "tool_call"}, {"second", "tool_call"}, {"first", "turn_end"}, {"second", "turn_end"},
	} {
		sub := step.sub
		f := frame
		f.Seq += 10
		frame.Seq = f.Seq
		f.Timestamp += 1000
		frame.Timestamp = f.Timestamp
		f.SubagentID = &sub
		f.EventType = step.event
		f.Data = types.TelemetryFrameData{}
		if step.event == "tool_call" {
			tool := "Read"
			f.Data.ToolName = &tool
		}
		if _, err := c.ProcessFrame(f); err != nil {
			t.Fatal(err)
		}
	}

	// Give dwell expiry a different rollup than the last frame saw: the row is
	// still BUSY with no completion while both subagents and the root are idle.
	agent, err := st.GetAgent(frame.AgentID)
	if err != nil || agent == nil {
		t.Fatalf("GetAgent = %v, %v", agent, err)
	}
	agent.Activity.Activity = types.ActivityBusy
	clearCompletion(&agent.Activity)
	if err := st.SaveAgentState(*agent); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	actx := c.agentCtx[frame.AgentID]
	actx.hasUnseenCompletion, actx.completedAt = nil, nil
	actx.subagents["first"].startedAt = time.Now().Add(-2 * time.Hour).UnixMilli()
	c.mu.Unlock()

	c.handleSubagentDwellExpired(frame.AgentID, "first")

	saved, err := st.GetAgent(frame.AgentID)
	if err != nil || saved == nil {
		t.Fatalf("GetAgent = %v, %v", saved, err)
	}
	if saved.Activity.HasUnseenCompletion == nil || !*saved.Activity.HasUnseenCompletion || saved.Activity.CompletedAt == nil {
		t.Fatalf("dwell expiry did not save the rolled-up completion: %+v", saved.Activity)
	}
	assertCompletion(t, c, st, frame.AgentID, *saved.Activity.CompletedAt)

	// The next frame starts from the cache and must keep that completion.
	frame.Seq++
	frame.Timestamp += 1000
	frame.EventType = "text"
	frame.SubagentID = nil
	frame.Data = types.TelemetryFrameData{}
	if _, err := c.ProcessFrame(frame); err != nil {
		t.Fatal(err)
	}
	assertCompletion(t, c, st, frame.AgentID, *saved.Activity.CompletedAt)
}

func TestClassifier_IdleTransitionWaitsForFrameInFlight(t *testing.T) {
	c, st, frame := completedAgent(t, ":memory:")
	c.SetIdleGracePeriod(time.Hour)
	send := func(event string, subagent string, stop string) {
		t.Helper()
		f := frame
		f.Seq += 10
		frame.Seq = f.Seq
		f.Timestamp += 1000
		frame.Timestamp = f.Timestamp
		f.EventType = event
		f.SubagentID = nil
		if subagent != "" {
			f.SubagentID = &subagent
		}
		f.Data = types.TelemetryFrameData{}
		if stop != "" {
			f.Data.StopReason = &stop
		}
		if event == "tool_call" {
			tool := "Read"
			f.Data.ToolName = &tool
		}
		if _, err := c.ProcessFrame(f); err != nil {
			t.Fatal(err)
		}
	}
	send("turn_start", "", "")
	send("tool_call", "worker", "")
	// The root finishes its turn and waits out the idle grace period.
	send("turn_end", "", "end_turn")
	c.mu.Lock()
	scheduledGen := c.agentCtx[frame.AgentID].rootGeneration
	c.mu.Unlock()

	// The subagent finishes. Its frame prepares a BUSY row (the root is still
	// in its grace period), then the grace period ends before the frame saves.
	paused, resume := make(chan struct{}), make(chan struct{})
	c.beforeFrameSaveHook = func(string) {
		c.beforeFrameSaveHook = nil
		close(paused)
		<-resume
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		send("turn_end", "worker", "end_turn")
	}()
	<-paused
	c.handleIdleTransition(frame.AgentID, scheduledGen)
	close(resume)
	<-done

	// The idle transition must not be lost behind the frame's later save.
	waitForDismissalCondition(t, func() bool {
		agent, err := st.GetAgent(frame.AgentID)
		return err == nil && agent != nil && agent.Activity.Activity == types.ActivityIdle &&
			agent.Activity.HasUnseenCompletion != nil && *agent.Activity.HasUnseenCompletion
	})
	c.mu.Lock()
	rootActivity := c.agentCtx[frame.AgentID].rootActivity
	c.mu.Unlock()
	if rootActivity != types.ActivityIdle {
		t.Errorf("cached root activity = %v, want IDLE", rootActivity)
	}
}

func TestClassifier_SubagentDwellExpiryWaitsForFrameInFlight(t *testing.T) {
	c, st, frame := completedAgent(t, ":memory:")
	c.SetSubagentDwell(time.Hour)
	worker := "worker"
	for _, event := range []string{"tool_call", "turn_end"} {
		f := frame
		f.Seq += 10
		frame.Seq = f.Seq
		f.Timestamp += 1000
		frame.Timestamp = f.Timestamp
		f.SubagentID = &worker
		f.EventType = event
		f.Data = types.TelemetryFrameData{}
		if event == "tool_call" {
			tool := "Read"
			f.Data.ToolName = &tool
		}
		if _, err := c.ProcessFrame(f); err != nil {
			t.Fatal(err)
		}
	}
	// Frame timestamps are far in the past, so only the wall-clock dwell timer
	// treats the completed worker as expired; frames keep it.
	c.mu.Lock()
	c.agentCtx[frame.AgentID].subagents[worker].startedAt = time.Now().Add(-2 * time.Hour).UnixMilli()
	c.mu.Unlock()

	paused, resume := make(chan struct{}), make(chan struct{})
	c.beforeFrameSaveHook = func(string) {
		c.beforeFrameSaveHook = nil
		close(paused)
		<-resume
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f := frame
		f.Seq += 10
		f.Timestamp += 1000
		f.EventType = "text"
		f.SubagentID = nil
		f.Data = types.TelemetryFrameData{}
		if _, err := c.ProcessFrame(f); err != nil {
			t.Error(err)
		}
	}()
	<-paused
	c.handleSubagentDwellExpired(frame.AgentID, worker)
	close(resume)
	<-done

	// The expiry must not be lost behind the frame's later save of the worker.
	waitForDismissalCondition(t, func() bool {
		agent, err := st.GetAgent(frame.AgentID)
		return err == nil && agent != nil && len(agent.Activity.ActiveSubagents) == 0
	})
}
