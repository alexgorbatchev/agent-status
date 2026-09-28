package reconciler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
)

type mockBroadcaster struct {
	mu     sync.Mutex
	events []types.IServerEvent
}

func (m *mockBroadcaster) Publish(event types.IServerEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockBroadcaster) getEvents() []types.IServerEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]types.IServerEvent, len(m.events))
	copy(copied, m.events)
	return copied
}

type mockProcessChecker struct {
	alivePIDs map[int]bool
}

func (m *mockProcessChecker) IsProcessAlive(pid int, startedAt int64) bool {
	return m.alivePIDs[pid]
}

func TestReconciler_Reconcile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reconcile_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	broadcaster := &mockBroadcaster{}
	checker := &mockProcessChecker{
		alivePIDs: map[int]bool{
			2001: true,  // alive
			2002: false, // dead
			2003: false, // dead
		},
	}

	localHost := "macbook.local"
	remoteHost := "remote-linux.local"

	rec := New(
		st,
		broadcaster,
		WithProcessChecker(checker),
		WithLocalHostname(localHost),
		WithAbandonedLimit(5*time.Minute),
	)

	now := time.Now().UnixMilli()

	// 1. Agent with dead PID on local host -> MUST be reconciled to EXITED
	deadAgent := types.IAgentState{
		ID:        "agent-dead-pid",
		SessionID: "sess-dead",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-local",
			Hostname: localHost,
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now - 3600000,
			UpdatedAt:      now - 1800000,
			CurrentRun: &types.IRunInfo{
				ID:        "run-dead",
				PID:       2002,
				StartedAt: now - 3600000,
			},
		},
	}
	if err := st.SaveAgentState(deadAgent); err != nil {
		t.Fatalf("SaveAgentState(deadAgent) failed: %v", err)
	}

	// 2. Agent with alive PID on local host -> MUST remain RUNNING
	aliveAgent := types.IAgentState{
		ID:        "agent-alive-pid",
		SessionID: "sess-alive",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-local",
			Hostname: localHost,
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now - 3600000,
			UpdatedAt:      now - 10000,
			CurrentRun: &types.IRunInfo{
				ID:        "run-alive",
				PID:       2001,
				StartedAt: now - 3600000,
			},
		},
	}
	if err := st.SaveAgentState(aliveAgent); err != nil {
		t.Fatalf("SaveAgentState(aliveAgent) failed: %v", err)
	}

	// 3. Agent with PID 0 and abandoned UpdatedAt (>5m) -> MUST be reconciled to EXITED
	abandonedAgent := types.IAgentState{
		ID:        "agent-abandoned-pid0",
		SessionID: "sess-abandoned",
		Harness:   types.HarnessCodex,
		Environment: types.IEnvironmentInfo{
			ID:       "env-local",
			Hostname: localHost,
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now - 600000,
			UpdatedAt:      now - 400000, // > 5 minutes ago
			CurrentRun: &types.IRunInfo{
				ID:        "run-abandoned",
				PID:       0,
				StartedAt: now - 600000,
			},
		},
	}
	if err := st.SaveAgentState(abandonedAgent); err != nil {
		t.Fatalf("SaveAgentState(abandonedAgent) failed: %v", err)
	}

	// 4. Agent with PID 0 and recent UpdatedAt (<5m) -> MUST remain RUNNING
	recentPid0Agent := types.IAgentState{
		ID:        "agent-recent-pid0",
		SessionID: "sess-recent-pid0",
		Harness:   types.HarnessOpencode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-local",
			Hostname: localHost,
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now - 60000,
			UpdatedAt:      now - 30000, // 30s ago (< 5m)
			CurrentRun: &types.IRunInfo{
				ID:        "run-recent-pid0",
				PID:       0,
				StartedAt: now - 60000,
			},
		},
	}
	if err := st.SaveAgentState(recentPid0Agent); err != nil {
		t.Fatalf("SaveAgentState(recentPid0Agent) failed: %v", err)
	}

	// 5. Agent on remote host with dead PID -> MUST NOT be reconciled locally
	remoteAgent := types.IAgentState{
		ID:        "agent-remote-host",
		SessionID: "sess-remote",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-remote",
			Hostname: remoteHost,
			Platform: "linux",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now - 3600000,
			UpdatedAt:      now - 1800000,
			CurrentRun: &types.IRunInfo{
				ID:        "run-remote",
				PID:       2003,
				StartedAt: now - 3600000,
			},
		},
	}
	if err := st.SaveAgentState(remoteAgent); err != nil {
		t.Fatalf("SaveAgentState(remoteAgent) failed: %v", err)
	}

	// RUN RECONCILIATION
	count, err := rec.Reconcile(context.Background(), "")
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Exactly 2 agents should have been reconciled: deadAgent and abandonedAgent
	if count != 2 {
		t.Errorf("expected 2 reconciled agents, got %d", count)
	}

	// Verify deadAgent is now EXITED
	reloadedDead, err := st.GetAgent(deadAgent.ID)
	if err != nil || reloadedDead == nil {
		t.Fatalf("GetAgent(deadAgent) failed: %v", err)
	}
	if reloadedDead.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected deadAgent lifecycle EXITED, got %s", reloadedDead.Activity.Lifecycle)
	}
	if reloadedDead.Activity.CurrentRun != nil {
		t.Errorf("expected deadAgent CurrentRun nil, got %+v", reloadedDead.Activity.CurrentRun)
	}

	// Verify abandonedAgent is now EXITED
	reloadedAbandoned, err := st.GetAgent(abandonedAgent.ID)
	if err != nil || reloadedAbandoned == nil {
		t.Fatalf("GetAgent(abandonedAgent) failed: %v", err)
	}
	if reloadedAbandoned.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected abandonedAgent lifecycle EXITED, got %s", reloadedAbandoned.Activity.Lifecycle)
	}

	// Verify aliveAgent is still RUNNING
	reloadedAlive, err := st.GetAgent(aliveAgent.ID)
	if err != nil || reloadedAlive == nil {
		t.Fatalf("GetAgent(aliveAgent) failed: %v", err)
	}
	if reloadedAlive.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("expected aliveAgent lifecycle RUNNING, got %s", reloadedAlive.Activity.Lifecycle)
	}

	// Verify recentPid0Agent is still RUNNING
	reloadedRecent, err := st.GetAgent(recentPid0Agent.ID)
	if err != nil || reloadedRecent == nil {
		t.Fatalf("GetAgent(recentPid0Agent) failed: %v", err)
	}
	if reloadedRecent.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("expected recentPid0Agent lifecycle RUNNING, got %s", reloadedRecent.Activity.Lifecycle)
	}

	// Verify remoteAgent is still RUNNING
	reloadedRemote, err := st.GetAgent(remoteAgent.ID)
	if err != nil || reloadedRemote == nil {
		t.Fatalf("GetAgent(remoteAgent) failed: %v", err)
	}
	if reloadedRemote.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("expected remoteAgent lifecycle RUNNING, got %s", reloadedRemote.Activity.Lifecycle)
	}

	// Verify notifications were broadcast
	events := broadcaster.getEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 broadcast events, got %d", len(events))
	}
	for _, ev := range events {
		if ev.Type != "agent_upsert" {
			t.Errorf("expected event type agent_upsert, got %s", ev.Type)
		}
		if ev.Agent == nil || ev.Agent.Activity.Lifecycle != types.LifecycleExited {
			t.Errorf("expected broadcast agent to be EXITED, got %+v", ev.Agent)
		}
	}
}

func TestReconciler_ContextCancellation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cancel_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	rec := New(st, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = rec.Reconcile(ctx, "")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestReconciler_LoopContextCancellation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "loop_cancel_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()
	for i := 1; i <= 3; i++ {
		err := st.SaveAgentState(types.IAgentState{
			ID:        fmt.Sprintf("agent-loop-%d", i),
			SessionID: fmt.Sprintf("sess-loop-%d", i),
			Harness:   types.HarnessPi,
			Environment: types.IEnvironmentInfo{
				ID:       "env-loop",
				Platform: "darwin",
			},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityBusy,
				CurrentRun: &types.IRunInfo{
					ID:        fmt.Sprintf("run-loop-%d", i),
					PID:       9990 + i,
					StartedAt: now,
				},
			},
		})
		if err != nil {
			t.Fatalf("SaveAgentState failed: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	checker := &dynamicChecker{
		onCheck: func(pid int) bool {
			cancel() // cancel during processing
			return false
		},
	}
	rec := New(st, nil, WithProcessChecker(checker))

	_, err = rec.Reconcile(ctx, "env-loop")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

type dynamicChecker struct {
	onCheck func(pid int) bool
}

func (d *dynamicChecker) IsProcessAlive(pid int, startedAt int64) bool {
	if d.onCheck != nil {
		return d.onCheck(pid)
	}
	return false
}

func TestReconciler_GetAgentReloadFallback(t *testing.T) {
	now := time.Now().UnixMilli()
	testAgent := types.IAgentState{
		ID:        "agent-reload-fallback",
		SessionID: "sess-fallback",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID: "env-fallback",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			CurrentRun: &types.IRunInfo{
				ID:        "run-fallback",
				PID:       8888,
				StartedAt: now,
			},
		},
	}

	mockSt := &mockStore{
		runningAgents: []types.IAgentState{testAgent},
		getAgentErr:   fmt.Errorf("simulated reload failure"),
	}

	broadcaster := &mockBroadcaster{}
	checker := &mockProcessChecker{alivePIDs: map[int]bool{8888: false}}
	rec := New(mockSt, broadcaster, WithProcessChecker(checker))

	count, err := rec.Reconcile(context.Background(), "env-fallback")
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 reconciled agent, got %d", count)
	}

	events := broadcaster.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 broadcast event, got %d", len(events))
	}
	if events[0].Agent == nil || events[0].Agent.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected fallback agent with LifecycleExited, got %+v", events[0].Agent)
	}
}

type mockStore struct {
	runningAgents []types.IAgentState
	endRunErr     error
	getAgentErr   error
}

func (m *mockStore) GetRunningAgents(envID string) ([]types.IAgentState, error) {
	return m.runningAgents, nil
}

func (m *mockStore) EndCurrentRun(agentID string, endedAt int64) error {
	return m.endRunErr
}

func (m *mockStore) GetAgent(id string) (*types.IAgentState, error) {
	if m.getAgentErr != nil {
		return nil, m.getAgentErr
	}
	for _, a := range m.runningAgents {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, nil
}

func TestReconciler_EndCurrentRunError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "end_err_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}

	now := time.Now().UnixMilli()
	_ = st.SaveAgentState(types.IAgentState{
		ID:        "agent-end-err",
		SessionID: "sess-end-err",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID: "env-end-err",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			CurrentRun: &types.IRunInfo{
				ID:        "run-end-err",
				PID:       7777,
				StartedAt: now,
			},
		},
	})

	checker := &dynamicChecker{
		onCheck: func(pid int) bool {
			_ = st.Close() // Close database right before EndCurrentRun
			return false
		},
	}
	rec := New(st, nil, WithProcessChecker(checker))

	count, _ := rec.Reconcile(context.Background(), "env-end-err")
	if count != 0 {
		t.Errorf("expected 0 reconciled when EndCurrentRun fails, got %d", count)
	}
}

func TestReconciler_ExplicitEnvID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "env_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	checker := &mockProcessChecker{
		alivePIDs: map[int]bool{9999: false},
	}
	rec := New(st, nil, WithProcessChecker(checker))

	now := time.Now().UnixMilli()
	agent := types.IAgentState{
		ID:        "agent-env-target",
		SessionID: "sess-target",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "target-env",
			Hostname: "other-machine",
			Platform: "linux",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			CurrentRun: &types.IRunInfo{
				ID:        "run-target",
				PID:       9999,
				StartedAt: now,
			},
		},
	}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	count, err := rec.Reconcile(context.Background(), "target-env")
	if err != nil {
		t.Fatalf("Reconcile with explicit envID failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 reconciled agent with explicit envID, got %d", count)
	}
}

func TestOSProcessChecker(t *testing.T) {
	checker := &OSProcessChecker{}

	// PID <= 0
	if checker.IsProcessAlive(0, 0) {
		t.Errorf("expected PID 0 to not be alive")
	}
	if checker.IsProcessAlive(-1, 0) {
		t.Errorf("expected PID -1 to not be alive")
	}

	// Mock proc env var
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "1")
	if !checker.IsProcessAlive(999999, 0) {
		t.Errorf("expected PID 999999 to be alive with AGENT_STATUS_TEST_MOCK_PROC=1")
	}
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "")

	// Real current process PID
	currentPID := os.Getpid()
	if !checker.IsProcessAlive(currentPID, time.Now().UnixMilli()) {
		t.Errorf("expected current process PID %d to be alive", currentPID)
	}

	// Non-existent PID
	if checker.IsProcessAlive(99999999, 0) {
		t.Errorf("expected nonexistent PID 99999999 to not be alive")
	}

	// Recycled PID with startedAt in future or ancient past
	ancientStartedAt := time.Now().Add(-24 * time.Hour).UnixMilli()
	// Current process was created much more recently than 24 hours ago (diff > 60000ms)
	if checker.IsProcessAlive(currentPID, ancientStartedAt) {
		t.Errorf("expected current process to be detected as recycled PID with ancient startedAt")
	}
}

func TestReconciler_LogLevel_DebugForIndividualExits(t *testing.T) {
	origLogger := logging.Default()
	t.Cleanup(func() {
		logging.SetDefault(origLogger)
	})

	dbPath := filepath.Join(t.TempDir(), "reconcile_log_level_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	checker := &mockProcessChecker{
		alivePIDs: map[int]bool{
			7001: false, // dead
		},
	}

	localHost := "macbook.local"
	rec := New(
		st,
		nil,
		WithProcessChecker(checker),
		WithLocalHostname(localHost),
	)

	now := time.Now().UnixMilli()
	deadAgent := types.IAgentState{
		ID:        "agent-dead-for-log-test",
		SessionID: "sess-dead-log",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-local",
			Hostname: localHost,
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now - 3600000,
			UpdatedAt:      now - 1800000,
			CurrentRun: &types.IRunInfo{
				ID:        "run-dead-log",
				PID:       7001,
				StartedAt: now - 3600000,
			},
		},
	}
	if err := st.SaveAgentState(deadAgent); err != nil {
		t.Fatalf("SaveAgentState(deadAgent) failed: %v", err)
	}

	// 1. With INFO log level: run_reconciled_exited MUST NOT be logged
	infoBuf := &bytes.Buffer{}
	infoLogger := logging.NewWithMode(infoBuf, "test-ingester", "info", true)
	logging.SetDefault(infoLogger)

	count, err := rec.Reconcile(context.Background(), "")
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 reconciled agent, got %d", count)
	}

	infoOutput := infoBuf.String()
	if strings.Contains(infoOutput, "run_reconciled_exited") {
		t.Errorf("at INFO level, run_reconciled_exited should not be logged, but found: %q", infoOutput)
	}

	// 2. Re-create dead agent and reconcile with DEBUG log level: run_reconciled_exited MUST be logged
	deadAgent2 := deadAgent
	deadAgent2.ID = "agent-dead-for-debug-log-test"
	deadAgent2.SessionID = "sess-dead-debug-log"
	deadAgent2.Activity.Lifecycle = types.LifecycleRunning
	deadAgent2.Activity.CurrentRun = &types.IRunInfo{
		ID:        "run-dead-debug-log",
		PID:       7001,
		StartedAt: now - 3600000,
	}
	if err := st.SaveAgentState(deadAgent2); err != nil {
		t.Fatalf("SaveAgentState(deadAgent2) failed: %v", err)
	}

	debugBuf := &bytes.Buffer{}
	debugLogger := logging.NewWithMode(debugBuf, "test-ingester", "debug", true)
	logging.SetDefault(debugLogger)

	count2, err := rec.Reconcile(context.Background(), "")
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if count2 != 1 {
		t.Fatalf("expected 1 reconciled agent, got %d", count2)
	}

	debugOutput := debugBuf.String()
	if !strings.Contains(debugOutput, "run_reconciled_exited") {
		t.Errorf("at DEBUG level, run_reconciled_exited MUST be logged, but output was: %q", debugOutput)
	}
}

type mockStoreWithAgents struct {
	store.Store
	agents       []types.IAgentState
	endRunCalled bool
}

func (m *mockStoreWithAgents) GetRunningAgents(envID string) ([]types.IAgentState, error) {
	return m.agents, nil
}

func (m *mockStoreWithAgents) EndCurrentRun(agentID string, endedAt int64) error {
	m.endRunCalled = true
	return nil
}

func (m *mockStoreWithAgents) GetAgent(agentID string) (*types.IAgentState, error) {
	return nil, nil
}

func TestReconciler_SkipsExitedLifecycle(t *testing.T) {
	st := &mockStoreWithAgents{
		agents: []types.IAgentState{
			{
				ID: "agent-exited",
				Activity: types.IAgentActivity{
					Lifecycle: types.LifecycleExited,
					CurrentRun: &types.IRunInfo{
						ID:  "run-exited",
						PID: 999999, // dead PID
					},
				},
			},
		},
	}
	checker := &mockProcessChecker{
		alivePIDs: map[int]bool{999999: false},
	}
	rec := New(st, nil, WithProcessChecker(checker))

	count, err := rec.Reconcile(context.Background(), "")
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 reconciled agents because agent lifecycle is EXITED, got %d", count)
	}
	if st.endRunCalled {
		t.Errorf("expected EndCurrentRun not to be called for already EXITED agent")
	}
}

type raceListThenLoadStore struct {
	*store.Store
	targetAgentID string
	now           int64
}

func (s *raceListThenLoadStore) GetRunningAgents(envID string) ([]types.IAgentState, error) {
	// Simulate list-then-load race:
	// Agent was selected as running by the initial query, but its run ends before GetAgent loads it.
	if err := s.EndCurrentRun(s.targetAgentID, s.now); err != nil {
		return nil, err
	}
	// Calling the real Store.GetAgent returns the agent with Lifecycle == LifecycleExited
	agent, err := s.GetAgent(s.targetAgentID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("agent %s not found", s.targetAgentID)
	}
	return []types.IAgentState{*agent}, nil
}

func TestReconciler_ListThenLoadExitedRace(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "race_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()
	localHost := "macbook.local"

	// Create an agent that has an active run and an older UpdatedAt that would trigger
	// the abandoned check if the LifecycleExited guard did not skip it.
	agent := types.IAgentState{
		ID:        "agent-race-1",
		SessionID: "sess-race-1",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-local",
			Hostname: localHost,
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now - 3600000,
			UpdatedAt:      now - 1800000, // 30 minutes old (> 5m abandoned limit)
			CurrentRun: &types.IRunInfo{
				ID:        "run-race-1",
				PID:       0, // PID 0 so it would fall into the abandoned check
				StartedAt: now - 3600000,
			},
		},
	}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	rStore := &raceListThenLoadStore{
		Store:         st,
		targetAgentID: agent.ID,
		now:           now,
	}

	broadcaster := &mockBroadcaster{}
	rec := New(
		rStore,
		broadcaster,
		WithLocalHostname(localHost),
		WithAbandonedLimit(5*time.Minute),
	)

	count, err := rec.Reconcile(context.Background(), "")
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 reconciled agents because raced exited agent was skipped, got %d", count)
	}
	if len(broadcaster.getEvents()) != 0 {
		t.Errorf("expected 0 broadcast events, got %d", len(broadcaster.getEvents()))
	}
}
