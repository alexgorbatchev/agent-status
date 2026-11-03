package classifier

import (
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

// getOrCreateAgentContext shares hydrated state and its writer lock between frames and dismissal.
func (c *Classifier) getOrCreateAgentContext(agent *types.IAgentState) *agentContext {
	c.mu.Lock()
	actx, exists := c.agentCtx[agent.ID]
	c.mu.Unlock()

	if !exists {
		preloaded := make(map[string]struct{})
		if c.store != nil && agent.Activity.SubagentCount > 0 {
			if ids, err := c.store.GetDistinctSubagentIDs(agent.ID); err == nil {
				for _, id := range ids {
					preloaded[id] = struct{}{}
				}
			}
		}
		c.mu.Lock()
		actx, exists = c.agentCtx[agent.ID]
		if !exists {
			subagentsMap := make(map[string]*subagentState)
			for _, summary := range agent.Activity.ActiveSubagents {
				st := summary.Status
				if st == "" {
					if summary.Activity == types.ActivityBusy {
						st = types.SubagentRunning
					} else {
						st = types.SubagentCompleted
					}
				}
				var rlResetAt *int64
				var rlRetrySec *int64
				if summary.Activity == types.ActivityRateLimited && agent.Activity.Activity == types.ActivityRateLimited {
					rlResetAt = agent.Activity.RateLimitResetAt
					rlRetrySec = agent.Activity.RetryAfterSeconds
				}
				subagentsMap[summary.ID] = &subagentState{
					id:                summary.ID,
					activity:          summary.Activity,
					activeTool:        summary.ActiveTool,
					action:            summary.Action,
					lastError:         summary.LastError,
					rateLimitResetAt:  rlResetAt,
					retryAfterSeconds: rlRetrySec,
					startedAt:         summary.StartedAt,
					completedAt:       summary.CompletedAt,
					status:            st,
				}
				preloaded[summary.ID] = struct{}{}
			}
			initialRootActivity := agent.Activity.Activity
			initialRootAction := agent.Activity.Action
			initialRootQuestion := agent.Activity.Question
			initialRootLastError := agent.Activity.LastError
			if len(subagentsMap) > 0 && agent.Activity.ActiveTool == nil {
				hasSubBusy := false
				for _, sub := range subagentsMap {
					if sub.activity == types.ActivityBusy {
						hasSubBusy = true
						break
					}
				}
				if hasSubBusy && initialRootActivity == types.ActivityBusy {
					initialRootActivity = types.ActivityIdle
					if strings.HasPrefix(initialRootAction, "Running subagent ") {
						initialRootAction = "Idle"
					}
				}
			}
			var initialRootRateLimitResetAt *int64
			var initialRootRetryAfterSeconds *int64
			if initialRootActivity == types.ActivityRateLimited {
				initialRootRateLimitResetAt = agent.Activity.RateLimitResetAt
				initialRootRetryAfterSeconds = agent.Activity.RetryAfterSeconds
			}
			actx = &agentContext{
				seenSubagents:         preloaded,
				subagents:             subagentsMap,
				rootActivity:          initialRootActivity,
				rootAction:            initialRootAction,
				rootQuestion:          initialRootQuestion,
				rootLastError:         initialRootLastError,
				rootRateLimitResetAt:  initialRootRateLimitResetAt,
				rootRetryAfterSeconds: initialRootRetryAfterSeconds,
				hasUnseenCompletion:   agent.Activity.HasUnseenCompletion,
				completedAt:           agent.Activity.CompletedAt,
			}
			c.agentCtx[agent.ID] = actx
		}
		c.mu.Unlock()
	}

	return actx
}
