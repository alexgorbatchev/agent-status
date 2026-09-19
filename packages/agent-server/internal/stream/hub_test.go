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
}

func TestHub_AgentTailSubscription(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	tailCh1 := hub.SubscribeAgentTail("agent-1")
	tailCh2 := hub.SubscribeAgentTail("agent-2")

	if hub.AgentSubscriberCount("agent-1") != 1 {
		t.Fatalf("AgentSubscriberCount(agent-1) = %d, want 1", hub.AgentSubscriberCount("agent-1"))
	}
	if hub.AgentSubscriberCount("agent-2") != 1 {
		t.Fatalf("AgentSubscriberCount(agent-2) = %d, want 1", hub.AgentSubscriberCount("agent-2"))
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

	// tailCh2 should NOT receive frame for agent-1
	select {
	case received := <-tailCh2:
		t.Fatalf("tailCh2 should not have received event, got %+v", received)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	hub.UnsubscribeAgentTail("agent-1", tailCh1)
	if hub.AgentSubscriberCount("agent-1") != 0 {
		t.Fatalf("AgentSubscriberCount(agent-1) after unsubscribe = %d, want 0", hub.AgentSubscriberCount("agent-1"))
	}
}

func TestHub_Close(t *testing.T) {
	hub := NewHub()
	ch := hub.SubscribeGlobal()
	tailCh := hub.SubscribeAgentTail("agent-1")

	hub.Close()

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Errorf("expected global subscriber channel to be closed")
	}

	_, ok = <-tailCh
	if ok {
		t.Errorf("expected agent tail channel to be closed")
	}
}
