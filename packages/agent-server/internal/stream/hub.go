package stream

import (
	"fmt"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

const (
	ClientChanBufferSize = 64
	HeartbeatInterval    = 15 * time.Second
)

type Hub struct {
	mu                sync.RWMutex
	logger            *logging.Logger
	subSeq            uint64
	globalSubscribers map[chan *types.IServerEvent]string
	agentSubscribers  map[string]map[chan *types.ITelemetryFrame]string
	closed            bool
}

func NewHub(loggers ...*logging.Logger) *Hub {
	var l *logging.Logger
	if len(loggers) > 0 && loggers[0] != nil {
		l = loggers[0]
	} else {
		l = logging.With()
	}
	return &Hub{
		logger:            l,
		globalSubscribers: make(map[chan *types.IServerEvent]string),
		agentSubscribers:  make(map[string]map[chan *types.ITelemetryFrame]string),
	}
}

func (h *Hub) log() *logging.Logger {
	if h.logger != nil {
		return h.logger
	}
	return logging.With()
}

// SubscribeGlobal registers a channel to receive all server events with an auto-generated subscriber ID.
func (h *Hub) SubscribeGlobal() chan *types.IServerEvent {
	return h.SubscribeGlobalWithID("")
}

// SubscribeGlobalWithID registers a channel to receive all server events with a specific subscriber ID.
func (h *Hub) SubscribeGlobalWithID(id string) chan *types.IServerEvent {
	ch := make(chan *types.IServerEvent, ClientChanBufferSize)
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		if id == "" {
			h.subSeq++
			id = fmt.Sprintf("sub-%d", h.subSeq)
		}
		h.globalSubscribers[ch] = id
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
	return h.SubscribeAgentTailWithID(agentID, "")
}

// SubscribeAgentTailWithID registers a channel to receive live telemetry frames for a specific agent with a specific subscriber ID.
func (h *Hub) SubscribeAgentTailWithID(agentID string, id string) chan *types.ITelemetryFrame {
	ch := make(chan *types.ITelemetryFrame, ClientChanBufferSize)
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		subs, ok := h.agentSubscribers[agentID]
		if !ok {
			subs = make(map[chan *types.ITelemetryFrame]string)
			h.agentSubscribers[agentID] = subs
		}
		if id == "" {
			h.subSeq++
			id = fmt.Sprintf("tail-%d", h.subSeq)
		}
		subs[ch] = id
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
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}

	for ch, subID := range h.globalSubscribers {
		select {
		case ch <- event:
		default:
			if event.Type == "snapshot" {
				delete(h.globalSubscribers, ch)
				close(ch)
				h.log().Warn("subscriber_evicted", "subscriber_id", subID, "event_type", event.Type)
			} else {
				h.log().Warn("subscriber_dropped", "subscriber_id", subID, "event_type", event.Type)
			}
		}
	}

	if event.Type == "log_append" && event.Event != nil && event.AgentID != "" {
		if subs, ok := h.agentSubscribers[event.AgentID]; ok {
			for ch, subID := range subs {
				select {
				case ch <- event.Event:
				default:
					h.log().Warn("subscriber_dropped", "subscriber_id", subID, "agent_id", event.AgentID)
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
