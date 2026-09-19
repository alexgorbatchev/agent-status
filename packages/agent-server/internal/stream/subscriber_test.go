package stream

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/types"
)

type mockStoreReader struct {
	envs   []types.IEnvironmentInfo
	agents []types.IAgentState
	err    error
}

func (m *mockStoreReader) GetEnvironments(ctx context.Context) ([]types.IEnvironmentInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.envs, nil
}

func (m *mockStoreReader) GetAgents(ctx context.Context, filter store.AgentFilter) ([]types.IAgentState, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.agents, nil
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
			err: errors.New("db failure"),
		}
		sub := NewSubscriber(SubscriberConfig{}, mockStore, hub)
		err := sub.ResyncAndBroadcast(context.Background())
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}

func TestSubscriber_SocketStream(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "subtest")
	if err != nil {
		t.Fatalf("creating temp dir in /tmp: %v", err)
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

	globalCh := hub.SubscribeGlobal()

	mockStore := &mockStoreReader{}
	sub := NewSubscriber(SubscriberConfig{
		SocketPath:    sockPath,
		ReconnectWait: 50 * time.Millisecond,
	}, mockStore, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Server goroutine: accepts conn and writes newline-delimited JSON events
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		evt := &types.IServerEvent{
			Type: "agent_upsert",
			At:   time.Now().UnixMilli(),
			Agent: &types.IAgentState{
				ID: "streamed-agent-1",
			},
		}
		data, _ := json.Marshal(evt)
		data = append(data, '\n')
		_, _ = conn.Write(data)
	}()

	go sub.Run(ctx)

	// We expect first the initial snapshot broadcast on connect, then the agent_upsert
	receivedUpsert := false
	deadline := time.After(1 * time.Second)

	for !receivedUpsert {
		select {
		case evt := <-globalCh:
			if evt.Type == "agent_upsert" && evt.Agent != nil && evt.Agent.ID == "streamed-agent-1" {
				receivedUpsert = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for streamed agent_upsert event")
		}
	}
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
