package classifier

import (
	"fmt"
	"sync"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

// DismissCompletion persists dismissal before clearing cached completion and broadcasting the updated agent.
func (c *Classifier) DismissCompletion(agentID string) (*types.IAgentState, error) {
	if c.store == nil {
		return nil, fmt.Errorf("classifier store is nil")
	}
	var persistMu *sync.Mutex
	c.mu.Lock()
	targetCtx := c.agentCtx[agentID]
	c.mu.Unlock()
	if targetCtx == nil {
		agent, err := c.store.GetAgent(agentID)
		if err != nil {
			return nil, err
		}
		if agent != nil {
			targetCtx = c.getOrCreateAgentContext(agent)
		}
	}
	if targetCtx != nil {
		persistMu = &targetCtx.persistMu
	}

	if persistMu != nil {
		persistMu.Lock()
		defer persistMu.Unlock()
	}

	c.mu.Lock()
	var unseen *bool
	var completedAt *int64
	if targetCtx != nil {
		unseen = targetCtx.hasUnseenCompletion
		completedAt = targetCtx.completedAt
	}
	c.mu.Unlock()

	agent, err := c.store.DismissAgentCompletion(agentID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	// Frames prepare state before taking persistMu. Only clear the completion
	// observed before the database write, never a newer turn's completion.
	if actx := c.agentCtx[agentID]; actx != nil && actx == targetCtx &&
		actx.hasUnseenCompletion == unseen && actx.completedAt == completedAt {
		actx.hasUnseenCompletion = nil
		actx.completedAt = nil
	}
	cb := c.onUpdate
	c.mu.Unlock()

	if cb != nil && agent != nil {
		cb(agent)
	}

	return agent, nil
}

// syncCompletionToAgentActivity synchronizes completion state from actx (or store fallback) to agent.Activity.
// Must be called with persistMu held, but outside c.mu.
func (c *Classifier) syncCompletionToAgentActivity(agentID string, agent *types.IAgentState) {
	var hasActx bool
	var actxHasUnseen *bool
	var actxCompletedAt *int64

	c.mu.Lock()
	if actx, ok := c.agentCtx[agentID]; ok {
		hasActx = true
		actxHasUnseen = actx.hasUnseenCompletion
		actxCompletedAt = actx.completedAt
	}
	c.mu.Unlock()

	if hasActx {
		if actxHasUnseen == nil || !*actxHasUnseen {
			agent.Activity.HasUnseenCompletion = nil
			agent.Activity.CompletedAt = nil
		} else {
			agent.Activity.HasUnseenCompletion = actxHasUnseen
			agent.Activity.CompletedAt = actxCompletedAt
		}
	} else if c.store != nil {
		if fresh, err := c.store.GetAgent(agentID); err == nil && fresh != nil {
			if fresh.Activity.HasUnseenCompletion == nil || !*fresh.Activity.HasUnseenCompletion {
				agent.Activity.HasUnseenCompletion = nil
				agent.Activity.CompletedAt = nil
			} else {
				agent.Activity.HasUnseenCompletion = fresh.Activity.HasUnseenCompletion
				agent.Activity.CompletedAt = fresh.Activity.CompletedAt
			}
		}
	}
}
