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

func TestSubscriber_ResyncAndBroadcast(t *testing.T) {
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
		if err := sub.ResyncAndBroadcast(context.Background()); err != nil {
			t.Fatalf("ResyncAndBroadcast() error = %v", err)
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
		err := sub.ResyncAndBroadcast(context.Background())
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
		err := sub.ResyncAndBroadcast(context.Background())
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
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
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
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	sockPath := filepath.Join(tmpDir, "n.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create test unix socket: %v", err)
	}
	defer listener.Close()

	hub := NewHub()
	defer hub.Close()

	mockStore := &mockStoreReader{
		envsErr: errors.New("db fail"),
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
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)
}

func TestDefaultSocketPath(t *testing.T) {
	os.Unsetenv("XDG_RUNTIME_DIR")
	path := DefaultSocketPath()
	if path != "/tmp/agent-status/notify.sock" {
		t.Errorf("DefaultSocketPath() = %q, want /tmp/agent-status/notify.sock", path)
	}

	os.Setenv("XDG_RUNTIME_DIR", "/custom/runtime")
	defer os.Unsetenv("XDG_RUNTIME_DIR")
	path = DefaultSocketPath()
	if path != "/custom/runtime/agent-status/notify.sock" {
		t.Errorf("DefaultSocketPath() with XDG_RUNTIME_DIR = %q, want /custom/runtime/agent-status/notify.sock", path)
	}
}
