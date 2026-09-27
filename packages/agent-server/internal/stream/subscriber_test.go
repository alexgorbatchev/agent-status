package stream

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

type mockStoreReader struct {
	mu           sync.Mutex
	envs         []types.IEnvironmentInfo
	agents       []types.IAgentState
	envsErr      error
	agentsErr    error
	resyncCounts int32
}

func (m *mockStoreReader) GetEnvironments(ctx context.Context) ([]types.IEnvironmentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddInt32(&m.resyncCounts, 1)
	if m.envsErr != nil {
		return nil, m.envsErr
	}
	return m.envs, nil
}

func (m *mockStoreReader) GetAgents(ctx context.Context, filter store.AgentFilter) ([]types.IAgentState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.agentsErr != nil {
		return nil, m.agentsErr
	}
	return m.agents, nil
}

func (m *mockStoreReader) setAgents(agents []types.IAgentState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents = agents
}

func (m *mockStoreReader) setEnvironments(envs []types.IEnvironmentInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.envs = envs
}

func (m *mockStoreReader) setEnvsErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.envsErr = err
}

func TestNewSubscriber_Defaults(t *testing.T) {
	mockStore := &mockStoreReader{}
	hub := NewHub()
	defer hub.Close()

	sub := NewSubscriber(SubscriberConfig{}, mockStore, hub)
	if sub.cfg.SocketPath == "" {
		t.Errorf("expected non-empty default SocketPath")
	}
	if sub.cfg.ReconnectWait != 1*time.Second {
		t.Errorf("expected ReconnectWait 1s, got %v", sub.cfg.ReconnectWait)
	}
	if sub.logger == nil {
		t.Errorf("expected non-nil default logger")
	}
}

func TestSubscriber_ReconcileSnapshot(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		hub := NewHub()
		defer hub.Close()

		ch := hub.SubscribeGlobal()

		mockStore := &mockStoreReader{
			envs: []types.IEnvironmentInfo{
				{ID: "env1", Label: "test-env"},
			},
			agents: []types.IAgentState{
				{ID: "agent-1"},
			},
		}

		sub := NewSubscriber(SubscriberConfig{}, mockStore, hub)
		if err := sub.reconcileSnapshot(context.Background()); err != nil {
			t.Fatalf("reconcileSnapshot() error = %v", err)
		}

		select {
		case event := <-ch:
			if event.Type != "snapshot" {
				t.Errorf("event.Type = %q, want 'snapshot'", event.Type)
			}
			if len(event.Agents) != 1 || len(event.Environments) != 1 {
				t.Errorf("unexpected snapshot payload: %+v", event)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for snapshot event")
		}
	})

	t.Run("environments error", func(t *testing.T) {
		hub := NewHub()
		defer hub.Close()

		mockStore := &mockStoreReader{
			envsErr: errors.New("db failure"),
		}
		sub := NewSubscriber(SubscriberConfig{}, mockStore, hub)
		err := sub.reconcileSnapshot(context.Background())
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})

	t.Run("agents error", func(t *testing.T) {
		hub := NewHub()
		defer hub.Close()

		mockStore := &mockStoreReader{
			agentsErr: errors.New("agents db failure"),
		}
		sub := NewSubscriber(SubscriberConfig{}, mockStore, hub)
		err := sub.reconcileSnapshot(context.Background())
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}

func TestSubscriber_SocketStreamAndReconnect(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "subtest")
	if err != nil {
		t.Fatalf("creating temp dir in /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	sockPath := filepath.Join(tmpDir, "n.sock")

	hub := NewHub()
	defer hub.Close()
	globalCh := hub.SubscribeGlobal()

	mockStore := &mockStoreReader{
		envs: []types.IEnvironmentInfo{{ID: "env1"}},
		agents: []types.IAgentState{{ID: "agent1"}},
	}

	sub := NewSubscriber(SubscriberConfig{
		SocketPath:    sockPath,
		ReconnectWait: 20 * time.Millisecond,
	}, mockStore, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start subscriber in background BEFORE socket is listening to test connect failure / retry
	go sub.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	// Now create the listener
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create test unix socket: %v", err)
	}

	// First connection: write an empty line, invalid json line, valid json line, then close connection
	conn1, err := listener.Accept()
	if err != nil {
		t.Fatalf("failed to accept first conn: %v", err)
	}

	// Write empty line (should be skipped)
	_, _ = conn1.Write([]byte("\n"))
	// Write malformed JSON (should be logged and skipped)
	_, _ = conn1.Write([]byte("{malformed json\n"))
	// Write valid JSON event
	evt1 := &types.IServerEvent{
		Type:    "agent_upsert",
		At:      time.Now().UnixMilli(),
		Agent:   &types.IAgentState{ID: "streamed-agent-1"},
	}
	data1, _ := json.Marshal(evt1)
	_, _ = conn1.Write(append(data1, '\n'))

	// Verify receipt of agent_upsert
	received1 := false
	deadline := time.After(1 * time.Second)
	for !received1 {
		select {
		case evt := <-globalCh:
			if evt.Type == "agent_upsert" && evt.Agent != nil && evt.Agent.ID == "streamed-agent-1" {
				received1 = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for streamed agent_upsert event on first conn")
		}
	}

	// Close conn1 to trigger reconnect loop
	_ = conn1.Close()

	// Accept second connection after reconnect
	conn2, err := listener.Accept()
	if err != nil {
		t.Fatalf("failed to accept second conn after reconnect: %v", err)
	}

	evt2 := &types.IServerEvent{
		Type:    "agent_removed",
		At:      time.Now().UnixMilli(),
		AgentID: "streamed-agent-1",
	}
	data2, _ := json.Marshal(evt2)
	_, _ = conn2.Write(append(data2, '\n'))

	received2 := false
	deadline2 := time.After(1 * time.Second)
	for !received2 {
		select {
		case evt := <-globalCh:
			if evt.Type == "agent_removed" && evt.AgentID == "streamed-agent-1" {
				received2 = true
			}
		case <-deadline2:
			t.Fatal("timed out waiting for agent_removed event on reconnected conn")
		}
	}

	_ = conn2.Close()
	_ = listener.Close()

	// Verify snapshot resync count increased on reconnect
	if atomic.LoadInt32(&mockStore.resyncCounts) < 2 {
		t.Errorf("expected at least 2 snapshot resyncs across connects, got %d", mockStore.resyncCounts)
	}
}

func TestSubscriber_ContextCancelDuringRun(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	mockStore := &mockStoreReader{}
	sub := NewSubscriber(SubscriberConfig{
		SocketPath:    "/nonexistent/path/test.sock",
		ReconnectWait: 50 * time.Millisecond,
	}, mockStore, hub)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		sub.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Clean exit
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Subscriber.Run did not exit promptly after context cancellation")
	}
}

func TestSubscriber_ContextAlreadyCanceled(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	mockStore := &mockStoreReader{}
	sub := NewSubscriber(SubscriberConfig{
		SocketPath:    "/nonexistent/path/test.sock",
		ReconnectWait: 50 * time.Millisecond,
	}, mockStore, hub)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		sub.Run(ctx)
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Subscriber.Run did not exit immediately with canceled context")
	}
}

func TestSubscriber_ResyncFailureOnConnect(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "subtest")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	sockPath := filepath.Join(tmpDir, "n.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create test unix socket: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	hub := NewHub()
	defer hub.Close()
	globalCh := hub.SubscribeGlobal()

	mockStore := &mockStoreReader{
		envsErr: errors.New("db fail"),
		envs:    []types.IEnvironmentInfo{{ID: "env1"}},
		agents:  []types.IAgentState{{ID: "agent1"}},
	}

	sub := NewSubscriber(SubscriberConfig{
		SocketPath:    sockPath,
		ReconnectWait: 20 * time.Millisecond,
	}, mockStore, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Run(ctx)

	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("failed to accept conn: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	// 1. Send hello message while store returns error
	helloMsg, _ := json.Marshal(types.IServerEvent{Type: "hello", At: time.Now().UnixMilli()})
	_, _ = conn.Write(append(helloMsg, '\n'))

	// Verify no snapshot was broadcast due to resync failure
	select {
	case evt := <-globalCh:
		t.Fatalf("unexpected event received on failed resync: %+v", evt)
	case <-time.After(50 * time.Millisecond):
	}

	// 2. Clear store error so subsequent retry can succeed
	mockStore.setEnvsErr(nil)

	// 3. Send regular event; subscriber should retry reconcileSnapshot since it wasn't marked reconciled
	evt := &types.IServerEvent{
		Type:    "agent_upsert",
		At:      time.Now().UnixMilli(),
		Agent:   &types.IAgentState{ID: "agent-retry"},
	}
	data, _ := json.Marshal(evt)
	_, _ = conn.Write(append(data, '\n'))

	// 4. Verify snapshot is now received on retry
	receivedSnapshot := false
	receivedUpsert := false
	deadline := time.After(1 * time.Second)
	for !receivedSnapshot || !receivedUpsert {
		select {
		case e := <-globalCh:
			if e.Type == "snapshot" {
				receivedSnapshot = true
			}
			if e.Type == "agent_upsert" && e.Agent != nil && e.Agent.ID == "agent-retry" {
				receivedUpsert = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for events; receivedSnapshot=%v, receivedUpsert=%v", receivedSnapshot, receivedUpsert)
		}
	}
}

func TestSubscriber_ResyncFailureOnFallbackEvent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "subtest")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	sockPath := filepath.Join(tmpDir, "n.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create test unix socket: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	hub := NewHub()
	defer hub.Close()
	globalCh := hub.SubscribeGlobal()

	mockStore := &mockStoreReader{
		envsErr: errors.New("db fail"),
		envs:    []types.IEnvironmentInfo{{ID: "env1"}},
		agents:  []types.IAgentState{{ID: "agent1"}},
	}

	sub := NewSubscriber(SubscriberConfig{
		SocketPath:    sockPath,
		ReconnectWait: 20 * time.Millisecond,
	}, mockStore, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Run(ctx)

	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("failed to accept conn: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	// 1. Send regular event without hello; fallback resync runs and fails
	evt1 := &types.IServerEvent{
		Type:    "agent_upsert",
		At:      time.Now().UnixMilli(),
		Agent:   &types.IAgentState{ID: "agent-1"},
	}
	data1, _ := json.Marshal(evt1)
	_, _ = conn.Write(append(data1, '\n'))

	// Event itself should be broadcast, but snapshot should not
	select {
	case e := <-globalCh:
		if e.Type != "agent_upsert" {
			t.Fatalf("expected agent_upsert, got %q", e.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for agent_upsert")
	}

	// 2. Clear store error
	mockStore.setEnvsErr(nil)

	// 3. Send another event; fallback resync should retry because reconciled was not set on error
	evt2 := &types.IServerEvent{
		Type:    "agent_removed",
		At:      time.Now().UnixMilli(),
		AgentID: "agent-1",
	}
	data2, _ := json.Marshal(evt2)
	_, _ = conn.Write(append(data2, '\n'))

	// 4. Verify snapshot and agent_removed are received
	receivedSnapshot := false
	receivedRemoved := false
	deadline := time.After(1 * time.Second)
	for !receivedSnapshot || !receivedRemoved {
		select {
		case e := <-globalCh:
			if e.Type == "snapshot" {
				receivedSnapshot = true
			}
			if e.Type == "agent_removed" && e.AgentID == "agent-1" {
				receivedRemoved = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for events; receivedSnapshot=%v, receivedRemoved=%v", receivedSnapshot, receivedRemoved)
		}
	}
}

func TestDefaultSocketPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	path := DefaultSocketPath()
	if path != "/tmp/agent-status/notify.sock" {
		t.Errorf("DefaultSocketPath() = %q, want /tmp/agent-status/notify.sock", path)
	}

	t.Setenv("XDG_RUNTIME_DIR", "/custom/runtime")
	path = DefaultSocketPath()
	if path != "/custom/runtime/agent-status/notify.sock" {
		t.Errorf("DefaultSocketPath() with XDG_RUNTIME_DIR = %q, want /custom/runtime/agent-status/notify.sock", path)
	}
}

func TestSubscriber_ReconcileSnapshotOnReconnectionAfterMissedEvents(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "subtest-reconnect-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	sockPath := filepath.Join(tmpDir, "notify.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("creating test unix socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	mockStore := &mockStoreReader{
		envs: []types.IEnvironmentInfo{
			{ID: "env-1", Label: "local"},
		},
		agents: []types.IAgentState{
			{
				ID:        "agent-1",
				SessionID: "sess-1",
				Activity: types.IAgentActivity{
					Lifecycle: types.LifecycleRunning,
					Activity:  types.ActivityBusy,
					Title:     "Initial task",
				},
			},
		},
	}

	hub := NewHub()
	t.Cleanup(func() { hub.Close() })
	globalCh := hub.SubscribeGlobal()

	sub := NewSubscriber(SubscriberConfig{
		SocketPath:    sockPath,
		ReconnectWait: 20 * time.Millisecond,
	}, mockStore, hub)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go sub.Run(ctx)

	// Accept initial connection
	conn1, err := listener.Accept()
	if err != nil {
		t.Fatalf("failed to accept initial conn: %v", err)
	}

	// Verify that merely accepting connection does NOT emit snapshot until connection readiness (hello)
	select {
	case evt := <-globalCh:
		t.Fatalf("unexpected snapshot before hello: %+v", evt)
	case <-time.After(50 * time.Millisecond):
	}

	// Send hello handshake to confirm connection readiness
	helloMsg, _ := json.Marshal(types.IServerEvent{Type: "hello", At: time.Now().UnixMilli()})
	_, _ = conn1.Write(append(helloMsg, '\n'))

	// 1. Verify initial snapshot received upon connection readiness
	select {
	case evt := <-globalCh:
		if evt.Type != "snapshot" {
			t.Fatalf("expected initial snapshot event, got %q", evt.Type)
		}
		if len(evt.Agents) != 1 || evt.Agents[0].ID != "agent-1" {
			t.Fatalf("unexpected initial snapshot agents: %+v", evt.Agents)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}

	// 2. Disconnect socket to simulate network drop / ingester restart
	_ = conn1.Close()

	// 3. While disconnected, simulate mutations occurring in SQLite that missed socket delivery
	updatedAgent1 := types.IAgentState{
		ID:        "agent-1",
		SessionID: "sess-1",
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			Title:     "Initial task (finished)",
		},
	}
	newAgent2 := types.IAgentState{
		ID:        "agent-2",
		SessionID: "sess-2",
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			Title:     "Second task",
		},
	}
	mockStore.setAgents([]types.IAgentState{updatedAgent1, newAgent2})
	mockStore.setEnvironments([]types.IEnvironmentInfo{
		{ID: "env-1", Label: "local"},
		{ID: "env-2", Label: "remote-worker"},
	})

	// 4. Accept reconnected socket connection
	conn2, err := listener.Accept()
	if err != nil {
		t.Fatalf("failed to accept reconnected conn: %v", err)
	}
	t.Cleanup(func() { _ = conn2.Close() })

	// Verify no snapshot before hello on reconnected socket
	select {
	case evt := <-globalCh:
		t.Fatalf("unexpected snapshot before hello on reconnect: %+v", evt)
	case <-time.After(50 * time.Millisecond):
	}

	// Send hello handshake to confirm connection readiness after reconnect
	_, _ = conn2.Write(append(helloMsg, '\n'))

	// 5. Verify that upon connection readiness, reconcileSnapshot queries the store and broadcasts the full snapshot
	select {
	case evt := <-globalCh:
		if evt.Type != "snapshot" {
			t.Fatalf("expected snapshot event on reconnection, got %q", evt.Type)
		}
		if len(evt.Agents) != 2 {
			t.Fatalf("expected 2 agents in reconnected snapshot, got %d", len(evt.Agents))
		}
		if len(evt.Environments) != 2 {
			t.Fatalf("expected 2 environments in reconnected snapshot, got %d", len(evt.Environments))
		}
		agentMap := make(map[string]types.IAgentState)
		for _, a := range evt.Agents {
			agentMap[a.ID] = a
		}
		if a1, ok := agentMap["agent-1"]; !ok || a1.Activity.Lifecycle != types.LifecycleExited {
			t.Errorf("expected agent-1 to be EXITED in reconnected snapshot, got %+v", a1)
		}
		if a2, ok := agentMap["agent-2"]; !ok || a2.Activity.Lifecycle != types.LifecycleRunning {
			t.Errorf("expected agent-2 to be RUNNING in reconnected snapshot, got %+v", a2)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for snapshot event on reconnection after missed events")
	}
}

