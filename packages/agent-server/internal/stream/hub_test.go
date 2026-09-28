package stream

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
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

			data, err := json.Marshal(evt)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			jsonStr := string(data)
			if !strings.Contains(jsonStr, `"agents":[]`) {
				t.Errorf("expected serialized json to contain '\"agents\":[]', got %s", jsonStr)
			}
			if !strings.Contains(jsonStr, `"environments":[]`) {
				t.Errorf("expected serialized json to contain '\"environments\":[]', got %s", jsonStr)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for snapshot event")
		}
	})

	t.Run("evicts slow subscriber when buffer is full on snapshot", func(t *testing.T) {
		hub := NewHub()
		defer hub.Close()

		ch := hub.SubscribeGlobal()
		if hub.GlobalSubscriberCount() != 1 {
			t.Fatalf("GlobalSubscriberCount() = %d, want 1", hub.GlobalSubscriberCount())
		}

		// Fill the subscriber buffer to capacity (ClientChanBufferSize = 64)
		for i := 0; i < ClientChanBufferSize; i++ {
			hub.Broadcast(&types.IServerEvent{
				Type: "agent_upsert",
				At:   time.Now().UnixMilli(),
				Agent: &types.IAgentState{
					ID: "agent-initial",
				},
			})
		}

		// Subscriber buffer is now completely full.
		// Broadcast snapshot: the subscriber cannot accept it.
		// Expected behavior: subscriber must be evicted and closed so client reconnects for a fresh snapshot.
		hub.BroadcastSnapshot([]types.IAgentState{{ID: "agent-authoritative"}}, nil)

		if got := hub.GlobalSubscriberCount(); got != 0 {
			t.Fatalf("GlobalSubscriberCount() = %d, want 0 (subscriber should be evicted on snapshot drop)", got)
		}

		// Drain the pre-existing buffer items
		for i := 0; i < ClientChanBufferSize; i++ {
			select {
			case <-ch:
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("timed out reading pre-existing event %d", i)
			}
		}

		// Channel must be closed now
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatal("expected subscriber channel to be closed after eviction, but was still open")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for channel to close after eviction")
		}

		// Subsequent unsubscribe on already-evicted channel should be a safe no-op
		hub.UnsubscribeGlobal(ch)
		if hub.GlobalSubscriberCount() != 0 {
			t.Errorf("GlobalSubscriberCount() = %d, want 0", hub.GlobalSubscriberCount())
		}
	})

	t.Run("evicts only full subscriber when multiple subscribers exist", func(t *testing.T) {
		hub := NewHub()
		defer hub.Close()

		slowCh := hub.SubscribeGlobalWithID("sub-slow")
		fastCh := hub.SubscribeGlobalWithID("sub-fast")

		if hub.GlobalSubscriberCount() != 2 {
			t.Fatalf("GlobalSubscriberCount() = %d, want 2", hub.GlobalSubscriberCount())
		}

		// Fill only slow subscriber's buffer
		for i := 0; i < ClientChanBufferSize; i++ {
			slowCh <- &types.IServerEvent{Type: "dummy"}
		}

		// Fast subscriber buffer is empty.
		// Broadcast snapshot: slow subscriber should be evicted, fast subscriber should receive snapshot.
		hub.BroadcastSnapshot([]types.IAgentState{{ID: "agent-authoritative"}}, nil)

		if got := hub.GlobalSubscriberCount(); got != 1 {
			t.Fatalf("GlobalSubscriberCount() = %d, want 1", got)
		}

		// Fast subscriber should receive the snapshot
		select {
		case evt := <-fastCh:
			if evt.Type != "snapshot" {
				t.Fatalf("fastCh received unexpected event type %q", evt.Type)
			}
			if len(evt.Agents) != 1 || evt.Agents[0].ID != "agent-authoritative" {
				t.Fatalf("fastCh received unexpected snapshot: %+v", evt)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for snapshot on fastCh")
		}

		// Slow subscriber should be closed after draining
		for i := 0; i < ClientChanBufferSize; i++ {
			<-slowCh
		}
		select {
		case _, ok := <-slowCh:
			if ok {
				t.Fatal("expected slowCh to be closed after eviction")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for slowCh to close")
		}
	})

	t.Run("logs structured events on eviction and non-snapshot drop", func(t *testing.T) {
		var logBuf bytes.Buffer
		customLogger := logging.NewWithMode(&logBuf, "test-server", "debug", true)
		hub := NewHub(customLogger)
		defer hub.Close()

		globalCh := hub.SubscribeGlobalWithID("sub-custom-global")
		tailCh := hub.SubscribeAgentTailWithID("agent-1", "tail-custom-agent")

		// Fill globalCh and tailCh
		for i := 0; i < ClientChanBufferSize; i++ {
			globalCh <- &types.IServerEvent{Type: "fill"}
			tailCh <- &types.ITelemetryFrame{AgentID: "agent-1", Seq: int64(i)}
		}

		// Broadcast a non-snapshot event: both should log subscriber_dropped
		hub.Broadcast(&types.IServerEvent{
			Type:    "log_append",
			AgentID: "agent-1",
			Event:   &types.ITelemetryFrame{AgentID: "agent-1", Seq: 999},
		})

		logs := logBuf.String()
		if !strings.Contains(logs, "action=subscriber_dropped") || !strings.Contains(logs, "subscriber_id=sub-custom-global") {
			t.Errorf("expected logs to contain subscriber_dropped for global subscriber, got:\n%s", logs)
		}
		if !strings.Contains(logs, "subscriber_id=tail-custom-agent") {
			t.Errorf("expected logs to contain subscriber_dropped for tail subscriber, got:\n%s", logs)
		}

		// Now broadcast a snapshot: global subscriber should be evicted and log subscriber_evicted
		hub.BroadcastSnapshot([]types.IAgentState{{ID: "agent-snap"}}, nil)

		logs = logBuf.String()
		if !strings.Contains(logs, "action=subscriber_evicted") || !strings.Contains(logs, "subscriber_id=sub-custom-global") {
			t.Errorf("expected logs to contain subscriber_evicted with subscriber_id, got:\n%s", logs)
		}
	})

	t.Run("log helper fallback on nil logger", func(t *testing.T) {
		hub := &Hub{}
		l := hub.log()
		if l == nil {
			t.Fatal("expected non-nil logger from log() fallback")
		}
	})
}
