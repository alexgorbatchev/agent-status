package stream

import (
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestHub_GlobalSubscription(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	ch1 := hub.SubscribeGlobal()
	ch2 := hub.SubscribeGlobal()

	if hub.GlobalSubscriberCount() != 2 {
		t.Fatalf("GlobalSubscriberCount() = %d, want 2", hub.GlobalSubscriberCount())
	}

	event := &types.IServerEvent{
		Type: "agent_upsert",
		At:   time.Now().UnixMilli(),
		Agent: &types.IAgentState{
			ID: "agent-1",
		},
	}

	hub.Broadcast(event)

	select {
	case received := <-ch1:
		if received.Type != "agent_upsert" || received.Agent.ID != "agent-1" {
			t.Errorf("ch1 received unexpected event: %+v", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch1 timed out waiting for event")
	}

	select {
	case received := <-ch2:
		if received.Type != "agent_upsert" || received.Agent.ID != "agent-1" {
			t.Errorf("ch2 received unexpected event: %+v", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch2 timed out waiting for event")
	}

	hub.UnsubscribeGlobal(ch1)
	if hub.GlobalSubscriberCount() != 1 {
		t.Fatalf("GlobalSubscriberCount() after unsubscribe = %d, want 1", hub.GlobalSubscriberCount())
	}

	// Unsubscribing a non-existent channel or already unsubscribed channel should be a no-op
	hub.UnsubscribeGlobal(ch1)
	if hub.GlobalSubscriberCount() != 1 {
		t.Fatalf("GlobalSubscriberCount() after duplicate unsubscribe = %d, want 1", hub.GlobalSubscriberCount())
	}
}

func TestHub_AgentTailSubscription(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	tailCh1 := hub.SubscribeAgentTail("agent-1")
	tailCh1b := hub.SubscribeAgentTail("agent-1")
	tailCh2 := hub.SubscribeAgentTail("agent-2")

	if hub.AgentSubscriberCount("agent-1") != 2 {
		t.Fatalf("AgentSubscriberCount(agent-1) = %d, want 2", hub.AgentSubscriberCount("agent-1"))
	}
	if hub.AgentSubscriberCount("agent-2") != 1 {
		t.Fatalf("AgentSubscriberCount(agent-2) = %d, want 1", hub.AgentSubscriberCount("agent-2"))
	}
	if hub.AgentSubscriberCount("agent-unknown") != 0 {
		t.Fatalf("AgentSubscriberCount(agent-unknown) = %d, want 0", hub.AgentSubscriberCount("agent-unknown"))
	}

	frame1 := &types.ITelemetryFrame{
		AgentID:   "agent-1",
		EventType: "tool_call",
		Timestamp: time.Now().UnixMilli(),
	}

	hub.Broadcast(&types.IServerEvent{
		Type:    "log_append",
		At:      time.Now().UnixMilli(),
		AgentID: "agent-1",
		Event:   frame1,
	})

	select {
	case received := <-tailCh1:
		if received.AgentID != "agent-1" || received.EventType != "tool_call" {
			t.Errorf("tailCh1 received unexpected frame: %+v", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("tailCh1 timed out waiting for frame")
	}

	select {
	case received := <-tailCh1b:
		if received.AgentID != "agent-1" || received.EventType != "tool_call" {
			t.Errorf("tailCh1b received unexpected frame: %+v", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("tailCh1b timed out waiting for frame")
	}

	// tailCh2 should NOT receive frame for agent-1
	select {
	case received := <-tailCh2:
		t.Fatalf("tailCh2 should not have received event, got %+v", received)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// Unsubscribe one of the subscribers for agent-1 (agent-1 still has tailCh1b)
	hub.UnsubscribeAgentTail("agent-1", tailCh1)
	if hub.AgentSubscriberCount("agent-1") != 1 {
		t.Fatalf("AgentSubscriberCount(agent-1) after removing one = %d, want 1", hub.AgentSubscriberCount("agent-1"))
	}

	// Unsubscribe remaining subscriber for agent-1 (map entry should be cleaned up)
	hub.UnsubscribeAgentTail("agent-1", tailCh1b)
	if hub.AgentSubscriberCount("agent-1") != 0 {
		t.Fatalf("AgentSubscriberCount(agent-1) after removing last = %d, want 0", hub.AgentSubscriberCount("agent-1"))
	}

	// Unsubscribe non-existent channel or agent
	hub.UnsubscribeAgentTail("agent-unknown", tailCh1)
	hub.UnsubscribeAgentTail("agent-1", tailCh1)
}

func TestHub_ClientBufferOverflowHandling(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	globalCh := hub.SubscribeGlobal()
	tailCh := hub.SubscribeAgentTail("agent-1")

	// Fill the buffer to capacity (ClientChanBufferSize = 64)
	for i := 0; i < ClientChanBufferSize; i++ {
		hub.Broadcast(&types.IServerEvent{
			Type:    "log_append",
			At:      time.Now().UnixMilli(),
			AgentID: "agent-1",
			Event: &types.ITelemetryFrame{
				AgentID: "agent-1",
				Seq:     int64(i),
			},
		})
	}

	// Both buffers are now full. The next broadcast should not block and should drop gracefully.
	hub.Broadcast(&types.IServerEvent{
		Type:    "log_append",
		At:      time.Now().UnixMilli(),
		AgentID: "agent-1",
		Event: &types.ITelemetryFrame{
			AgentID: "agent-1",
			Seq:     999,
		},
	})

	// Read one item from globalCh and tailCh to verify it kept running
	select {
	case item := <-globalCh:
		if item == nil {
			t.Error("expected non-nil item from globalCh")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out reading from globalCh")
	}

	select {
	case item := <-tailCh:
		if item == nil {
			t.Error("expected non-nil item from tailCh")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out reading from tailCh")
	}
}

func TestHub_BroadcastEdgeCases(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	ch := hub.SubscribeGlobal()

	// Broadcast nil event
	hub.Broadcast(nil)

	// Broadcast log_append with nil Event or empty AgentID
	hub.Broadcast(&types.IServerEvent{
		Type:    "log_append",
		AgentID: "",
		Event:   nil,
	})

	select {
	case evt := <-ch:
		if evt.Type != "log_append" {
			t.Errorf("expected log_append event, got %v", evt)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestHub_ClosedHubOperations(t *testing.T) {
	hub := NewHub()
	ch := hub.SubscribeGlobal()
	tailCh := hub.SubscribeAgentTail("agent-1")

	hub.Close()

	// Repeated Close should be safe no-op
	hub.Close()

	// Channels should be closed
	_, ok := <-ch
	if ok {
		t.Errorf("expected global subscriber channel to be closed")
	}

	_, ok = <-tailCh
	if ok {
		t.Errorf("expected agent tail channel to be closed")
	}

	// Subscribing to closed hub should return immediately closed channels
	closedGlobal := hub.SubscribeGlobal()
	_, ok = <-closedGlobal
	if ok {
		t.Errorf("expected SubscribeGlobal on closed hub to return closed channel")
	}

	closedTail := hub.SubscribeAgentTail("agent-1")
	_, ok = <-closedTail
	if ok {
		t.Errorf("expected SubscribeAgentTail on closed hub to return closed channel")
	}

	// Broadcast on closed hub should be a no-op
	hub.Broadcast(&types.IServerEvent{Type: "test"})
}

func TestHub_BroadcastSnapshot(t *testing.T) {
	t.Run("with agents and environments", func(t *testing.T) {
		hub := NewHub()
		defer hub.Close()

		ch := hub.SubscribeGlobal()

		agents := []types.IAgentState{
			{ID: "agent-1", SessionID: "sess-1"},
		}
		envs := []types.IEnvironmentInfo{
			{ID: "env-1", Label: "local"},
		}

		hub.BroadcastSnapshot(agents, envs)

		select {
		case evt := <-ch:
			if evt.Type != "snapshot" {
				t.Errorf("expected snapshot event, got %q", evt.Type)
			}
			if len(evt.Agents) != 1 || evt.Agents[0].ID != "agent-1" {
				t.Errorf("unexpected agents in snapshot: %+v", evt.Agents)
			}
			if len(evt.Environments) != 1 || evt.Environments[0].ID != "env-1" {
				t.Errorf("unexpected environments in snapshot: %+v", evt.Environments)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for snapshot event")
		}
	})

	t.Run("with nil slices", func(t *testing.T) {
		hub := NewHub()
		defer hub.Close()

		ch := hub.SubscribeGlobal()

		hub.BroadcastSnapshot(nil, nil)

		select {
		case evt := <-ch:
			if evt.Type != "snapshot" {
				t.Errorf("expected snapshot event, got %q", evt.Type)
			}
			if evt.Agents == nil || len(evt.Agents) != 0 {
				t.Errorf("expected non-nil empty agents slice, got %+v", evt.Agents)
			}
			if evt.Environments == nil || len(evt.Environments) != 0 {
				t.Errorf("expected non-nil empty environments slice, got %+v", evt.Environments)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for snapshot event")
		}
	})
}

