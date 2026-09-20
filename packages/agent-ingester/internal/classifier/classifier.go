package classifier

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

type agentContext struct {
	lastPrompt  string
	recentTools []summarizer.ToolExecution
}

type Classifier struct {
	store      *store.Store
	pricing    *pricing.Engine
	summarizer *summarizer.Summarizer
	agentCtx   map[string]*agentContext
	mu         sync.Mutex
}

func New(s *store.Store, p *pricing.Engine, sum *summarizer.Summarizer) *Classifier {
	return &Classifier{
		store:      s,
		pricing:    p,
		summarizer: sum,
		agentCtx:   make(map[string]*agentContext),
	}
}

// ProcessFrame applies 0-ms deterministic rules and debounced action summarization.
func (c *Classifier) ProcessFrame(frame types.ITelemetryFrame) (*types.IAgentState, error) {
	agent, err := c.store.GetAgent(frame.AgentID)
	if err != nil {
		return nil, fmt.Errorf("getting existing agent: %w", err)
	}

	now := frame.Timestamp
	if now == 0 {
		now = time.Now().UnixMilli()
	}

	if agent == nil {
		logging.Info("session discovered", "sessionId", frame.SessionID, "agentId", frame.AgentID, "harness", frame.Harness, "envId", frame.EnvID, "project", frame.Project.Name)
		// Initialize new agent state
		agent = &types.IAgentState{
			ID:          frame.AgentID,
			SessionID:   frame.SessionID,
			Harness:     frame.Harness,
			Environment: frame.Environment,
			Project:     frame.Project,
			Workspace:   frame.Workspace,
			Activity: types.IAgentActivity{
				Lifecycle:      types.LifecycleRunning,
				Activity:       types.ActivityIdle,
				FirstStartedAt: now,
				UpdatedAt:      now,
			},
		}
	} else {
		// Update mutable workspace/environment metadata
		agent.Environment = frame.Environment
		agent.Workspace = frame.Workspace
		agent.Project = frame.Project
		agent.Activity.UpdatedAt = now
	}

	isSubagent := frame.SubagentID != nil && *frame.SubagentID != ""

	// Accumulate Usage & Cost
	if frame.Data.Usage != nil {
		u := frame.Data.Usage
		agent.Activity.Usage.Input += u.Input
		agent.Activity.Usage.Output += u.Output
		agent.Activity.Usage.CacheRead += u.CacheRead
		agent.Activity.Usage.CacheWrite += u.CacheWrite
		agent.Activity.Usage.Reasoning += u.Reasoning
		if u.Total > 0 {
			agent.Activity.Usage.Total += u.Total
		} else {
			agent.Activity.Usage.Total += (u.Input + u.Output + u.CacheRead + u.CacheWrite)
		}

		if frame.Data.Model != nil && c.pricing != nil {
			cost, ok := c.pricing.CalculateCost(*frame.Data.Model, pricing.Usage{
				Input:      u.Input,
				Output:     u.Output,
				CacheRead:  u.CacheRead,
				CacheWrite: u.CacheWrite,
				Reasoning:  u.Reasoning,
			})
			if ok {
				if agent.Activity.Cost == nil {
					agent.Activity.Cost = &types.ICostInfo{
						Currency: "USD",
						Total:    0,
					}
				}
				agent.Activity.Cost.Total += cost
			}
		}
	}

	// Update Title if provided
	if frame.Data.Title != nil && strings.TrimSpace(*frame.Data.Title) != "" {
		agent.Activity.Title = strings.TrimSpace(*frame.Data.Title)
	}

	// Update Prompt Context if provided
	c.mu.Lock()
	actx, exists := c.agentCtx[agent.ID]
	if !exists {
		actx = &agentContext{}
		c.agentCtx[agent.ID] = actx
	}
	if frame.Data.LastPrompt != nil && strings.TrimSpace(*frame.Data.LastPrompt) != "" {
		actx.lastPrompt = strings.TrimSpace(*frame.Data.LastPrompt)
	}
	c.mu.Unlock()

	// If not a subagent, apply deterministic state machine transitions
	if !isSubagent {
		switch frame.EventType {
		case "run_started":
			agent.Activity.Lifecycle = types.LifecycleRunning
			agent.Activity.Activity = types.ActivityIdle
			runID := fmt.Sprintf("%s:%d:%d", agent.ID, frame.PID, now)
			agent.Activity.CurrentRun = &types.IRunInfo{
				ID:             runID,
				PID:            frame.PID,
				HarnessVersion: frame.HarnessVersion,
				StartedAt:      now,
				EndedAt:        nil,
			}
			c.ensureAction(agent)

		case "run_exited":
			agent.Activity.Lifecycle = types.LifecycleExited
			if err := c.store.EndCurrentRun(agent.ID, now); err != nil {
				return nil, fmt.Errorf("ending current run: %w", err)
			}
			if agent.Activity.CurrentRun != nil {
				agent.Activity.CurrentRun.EndedAt = &now
			}

		case "question_prompt":
			agent.Activity.Activity = types.ActivityWaitingQuestion
			if frame.Data.Question != nil {
				agent.Activity.Question = frame.Data.Question
			}
			c.updateSummarization(agent, true)

		case "error":
			agent.Activity.Activity = types.ActivityError
			if frame.Data.Content != nil {
				errStr := *frame.Data.Content
				if len(errStr) > 2000 {
					errStr = errStr[:2000]
				}
				agent.Activity.LastError = &errStr
			}
			c.updateSummarization(agent, true)

		case "tool_result":
			if frame.Data.IsError != nil && *frame.Data.IsError {
				agent.Activity.Activity = types.ActivityError
				if frame.Data.ToolOutput != nil {
					out := *frame.Data.ToolOutput
					if len(out) > 2000 {
						out = out[:2000]
					}
					agent.Activity.LastError = &out
				}
				c.updateSummarization(agent, true)
			} else {
				// Normal tool result
				agent.Activity.Activity = types.ActivityBusy
			}

		case "tool_call":
			agent.Activity.Activity = types.ActivityBusy
			if frame.Data.ToolName != nil {
				toolName := *frame.Data.ToolName
				agent.Activity.ActiveTool = &toolName

				target := extractTarget(toolName, frame.Data.ToolInput)
				if target != "" {
					agent.Activity.ActiveTarget = &target
				}

				c.mu.Lock()
				actx.recentTools = append(actx.recentTools, summarizer.ToolExecution{
					ToolName: toolName,
					Target:   target,
				})
				if len(actx.recentTools) > 3 {
					actx.recentTools = actx.recentTools[len(actx.recentTools)-3:]
				}
				c.mu.Unlock()

				if toolName == "AskUserQuestion" {
					agent.Activity.Activity = types.ActivityWaitingQuestion
					if frame.Data.Question != nil {
						agent.Activity.Question = frame.Data.Question
					}
					c.updateSummarization(agent, true)
				} else {
					// Continuous tool activity: summarize if debounce allows, otherwise mark stale
					c.updateSummarization(agent, false)
				}
			}

		case "turn_start":
			agent.Activity.Activity = types.ActivityBusy

		case "thinking":
			agent.Activity.Activity = types.ActivityBusy
			if frame.Data.Content != nil {
				agent.Activity.LastThinkingExcerpt = frame.Data.Content
			}

		case "turn_end":
			// Summarize on turn_end before clearing active tool fields
			c.updateSummarization(agent, true)
			if frame.Data.StopReason == nil || *frame.Data.StopReason != "tool_use" {
				agent.Activity.Activity = types.ActivityIdle
				agent.Activity.ActiveTool = nil
				agent.Activity.ActiveTarget = nil
				agent.Activity.Question = nil
			}
		}
	} else {
		// Subagent event: increment subagent count
		agent.Activity.SubagentCount++
	}

	if err := c.store.SaveAgentState(*agent); err != nil {
		return nil, fmt.Errorf("saving agent state: %w", err)
	}

	// Record the raw event after agent exists in DB
	if err := c.store.RecordTelemetryEvent(frame); err != nil {
		return nil, fmt.Errorf("recording telemetry event: %w", err)
	}

	return agent, nil
}

func (c *Classifier) updateSummarization(agent *types.IAgentState, forceTrigger bool) {
	c.mu.Lock()
	actx := c.agentCtx[agent.ID]
	var lastPrompt string
	var recentTools []summarizer.ToolExecution
	if actx != nil {
		lastPrompt = actx.lastPrompt
		recentTools = make([]summarizer.ToolExecution, len(actx.recentTools))
		copy(recentTools, actx.recentTools)
	}
	c.mu.Unlock()

	var thinking string
	if agent.Activity.LastThinkingExcerpt != nil {
		thinking = *agent.Activity.LastThinkingExcerpt
	}

	input := summarizer.SummaryInput{
		AgentID:             agent.ID,
		LastPrompt:          lastPrompt,
		RecentTools:         recentTools,
		LastThinkingExcerpt: thinking,
		Activity:            agent.Activity.Activity,
		ActiveTool:          agent.Activity.ActiveTool,
		ActiveTarget:        agent.Activity.ActiveTarget,
		LastError:           agent.Activity.LastError,
		Question:            agent.Activity.Question,
	}

	if c.summarizer.IsNoSummarize() {
		fb := summarizer.FallbackSummary(input)
		agent.Activity.Action = fb.Action
		agent.Activity.Phase = fb.Phase
		agent.Activity.ActionStale = false
		if fb.ActiveTarget != nil {
			agent.Activity.ActiveTarget = fb.ActiveTarget
		}
		return
	}

	// Check rate limit debounce for external LLM calls
	if c.summarizer.Allow(agent.ID) {
		res := c.summarizer.Summarize(context.Background(), input)
		agent.Activity.Action = res.Action
		agent.Activity.Phase = res.Phase
		agent.Activity.ActionStale = false
		if res.ActiveTarget != nil {
			agent.Activity.ActiveTarget = res.ActiveTarget
		}
	} else {
		// Rate limited: on forced state changes, apply fallback immediately
		if forceTrigger {
			fb := summarizer.FallbackSummary(input)
			agent.Activity.Action = fb.Action
			agent.Activity.Phase = fb.Phase
			agent.Activity.ActionStale = false
			if fb.ActiveTarget != nil {
				agent.Activity.ActiveTarget = fb.ActiveTarget
			}
		} else {
			if agent.Activity.Action == "" {
				fb := summarizer.FallbackSummary(input)
				agent.Activity.Action = fb.Action
				agent.Activity.Phase = fb.Phase
			}
			if agent.Activity.Activity == types.ActivityBusy {
				agent.Activity.ActionStale = true
			}
		}
	}
}

func (c *Classifier) ensureAction(agent *types.IAgentState) {
	if agent.Activity.Action == "" {
		c.updateSummarization(agent, false)
	}
}

func extractTarget(toolName string, input map[string]interface{}) string {
	if input == nil {
		return ""
	}
	keys := []string{
		"file_path",
		"path",
		"notebook_path",
		"description",
		"command",
		"url",
		"query",
		"skill",
		"pattern",
		"prompt",
	}
	for _, k := range keys {
		if val, ok := input[k]; ok && val != nil {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
