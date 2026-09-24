package stream

import (
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

const (
	ClientChanBufferSize = 64
	HeartbeatInterval    = 15 * time.Second
)

type Hub struct {
	mu           sync.RWMutex
	globalSubscribers map[chan *types.IServerEvent]struct{}
	agentSubscribers  map[string]map[chan *types.ITelemetryFrame]struct{}
	closed       bool
}

func NewHub() *Hub {
	return &Hub{
		globalSubscribers: make(map[chan *types.IServerEvent]struct{}),
		agentSubscribers:  make(map[string]map[chan *types.ITelemetryFrame]struct{}),
	}
}

// SubscribeGlobal registers a channel to receive all server events.
func (h *Hub) SubscribeGlobal() chan *types.IServerEvent {
	ch := make(chan *types.IServerEvent, ClientChanBufferSize)
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		h.globalSubscribers[ch] = struct{}{}
	} else {
		close(ch)
	}
	return ch
}

// UnsubscribeGlobal unregisters a global event channel.
func (h *Hub) UnsubscribeGlobal(ch chan *types.IServerEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.globalSubscribers[ch]; ok {
		delete(h.globalSubscribers, ch)
		close(ch)
	}
}

// SubscribeAgentTail registers a channel to receive live telemetry frames for a specific agent.
func (h *Hub) SubscribeAgentTail(agentID string) chan *types.ITelemetryFrame {
	ch := make(chan *types.ITelemetryFrame, ClientChanBufferSize)
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		subs, ok := h.agentSubscribers[agentID]
		if !ok {
			subs = make(map[chan *types.ITelemetryFrame]struct{})
			h.agentSubscribers[agentID] = subs
		}
		subs[ch] = struct{}{}
	} else {
		close(ch)
	}
	return ch
}

// UnsubscribeAgentTail unregisters an agent tail channel.
func (h *Hub) UnsubscribeAgentTail(agentID string, ch chan *types.ITelemetryFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs, ok := h.agentSubscribers[agentID]; ok {
		if _, exists := subs[ch]; exists {
			delete(subs, ch)
			close(ch)
		}
		if len(subs) == 0 {
			delete(h.agentSubscribers, agentID)
		}
	}
}

// Broadcast sends an event to all global subscribers and any relevant agent tail subscribers.
func (h *Hub) Broadcast(event *types.IServerEvent) {
	if event == nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return
	}

	for ch := range h.globalSubscribers {
		select {
		case ch <- event:
		default:
			// Non-blocking drop if channel buffer is full to prevent slow consumers from blocking hub
		}
	}

	if event.Type == "log_append" && event.Event != nil && event.AgentID != "" {
		if subs, ok := h.agentSubscribers[event.AgentID]; ok {
			for ch := range subs {
				select {
				case ch <- event.Event:
				default:
					// Non-blocking drop for slow consumers
				}
			}
		}
	}
}

// BroadcastSnapshot creates and broadcasts a synthetic snapshot event containing agents and environments to all global subscribers.
func (h *Hub) BroadcastSnapshot(agents []types.IAgentState, envs []types.IEnvironmentInfo) {
	if agents == nil {
		agents = []types.IAgentState{}
	}
	if envs == nil {
		envs = []types.IEnvironmentInfo{}
	}
	h.Broadcast(&types.IServerEvent{
		Type:         "snapshot",
		At:           time.Now().UnixMilli(),
		Agents:       agents,
		Environments: envs,
	})
}

// Close closes the hub and all active subscriber channels.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.closed = true

	for ch := range h.globalSubscribers {
		delete(h.globalSubscribers, ch)
		close(ch)
	}
	for agentID, subs := range h.agentSubscribers {
		for ch := range subs {
			close(ch)
		}
		delete(h.agentSubscribers, agentID)
	}
}

// GlobalSubscriberCount returns the number of active global subscribers.
func (h *Hub) GlobalSubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.globalSubscribers)
}

// AgentSubscriberCount returns the number of active subscribers for a specific agent.
func (h *Hub) AgentSubscriberCount(agentID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if subs, ok := h.agentSubscribers[agentID]; ok {
		return len(subs)
	}
	return 0
}
