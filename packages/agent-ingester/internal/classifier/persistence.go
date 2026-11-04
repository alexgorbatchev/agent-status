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
			if targetCtx, err = c.getOrCreateAgentContext(agent); err != nil {
				return nil, err
			}
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

// persistAgent is the single load-mutate-save path for writers that run outside
// ProcessFrame: summary jobs, idle and silence transitions, and subagent dwell
// expiry. Holding the agent's writer lock, it re-reads the agent row and calls
// mutate with c.mu held, but only while target is still the agent's context.
// mutate re-checks its own generations and returns false to skip the save.
// It returns the saved agent, nil when nothing was saved, or the store error.
func (c *Classifier) persistAgent(agentID string, target *agentContext, mutate func(agent *types.IAgentState) bool) (*types.IAgentState, error) {
	if c.store == nil {
		return nil, nil
	}
	target.persistMu.Lock()
	defer target.persistMu.Unlock()

	if !c.isCurrentContext(agentID, target) {
		return nil, nil
	}
	agent, err := c.store.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("loading agent: %w", err)
	}
	if agent == nil {
		return nil, nil
	}

	c.mu.Lock()
	apply := !c.closed && c.agentCtx[agentID] == target && mutate(agent)
	c.mu.Unlock()
	if !apply {
		return nil, nil
	}
	if err := c.store.SaveAgentState(*agent); err != nil {
		return nil, fmt.Errorf("saving agent: %w", err)
	}
	return agent, nil
}

func (c *Classifier) isCurrentContext(agentID string, target *agentContext) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.agentCtx[agentID] == target
}

// syncCompletionLocked copies the cached completion onto agent so a background
// write cannot restore a completion that dismissal already cleared.
func syncCompletionLocked(actx *agentContext, agent *types.IAgentState) {
	if actx.hasUnseenCompletion == nil || !*actx.hasUnseenCompletion {
		clearCompletion(&agent.Activity)
		return
	}
	agent.Activity.HasUnseenCompletion = actx.hasUnseenCompletion
	agent.Activity.CompletedAt = actx.completedAt
}

// frameLock serializes ProcessFrame for one agent. It is reference counted so
// the entry disappears once no frame holds or waits for it.
type frameLock struct {
	mu   sync.Mutex
	refs int
}

// lockFrames blocks until no other frame for agentID is being classified and
// returns the matching unlock. Frames read the full agent row and save it back,
// so two frames for one agent (for example from an old and a reconnected
// observer connection) must not interleave, or one frame's usage and state is
// overwritten by the other's stale copy. Dismissal and background writers do not
// take this lock; they use the agent context's persistMu.
func (c *Classifier) lockFrames(agentID string) func() {
	c.mu.Lock()
	lock := c.frameLocks[agentID]
	if lock == nil {
		lock = &frameLock{}
		c.frameLocks[agentID] = lock
	}
	lock.refs++
	c.mu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		c.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(c.frameLocks, agentID)
		}
		c.mu.Unlock()
	}
}
