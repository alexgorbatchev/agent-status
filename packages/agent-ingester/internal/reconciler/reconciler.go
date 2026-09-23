package reconciler

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/shirou/gopsutil/v4/process"
)

type ProcessChecker interface {
	IsProcessAlive(pid int, startedAt int64) bool
}

type Broadcaster interface {
	Publish(event types.IServerEvent)
}

type Store interface {
	GetRunningAgents(envID string) ([]types.IAgentState, error)
	EndCurrentRun(agentID string, endedAt int64) error
	GetAgent(id string) (*types.IAgentState, error)
}

type OSProcessChecker struct{}

func (c *OSProcessChecker) IsProcessAlive(pid int, startedAt int64) bool {
	if pid <= 0 {
		return false
	}
	if os.Getenv("AGENT_STATUS_TEST_MOCK_PROC") == "1" {
		return true
	}
	exists, err := process.PidExists(int32(pid))
	if err != nil || !exists {
		return false
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return false
	}
	running, err := p.IsRunning()
	if err != nil || !running {
		return false
	}
	if startedAt > 0 {
		createTime, err := p.CreateTime() // in milliseconds
		if err == nil && createTime > 0 {
			// If process was created significantly after the run started (>60s), PID was recycled
			diff := float64(createTime - startedAt)
			if diff > 60000 {
				return false
			}
		}
	}
	return true
}

type Reconciler struct {
	store          Store
	broadcaster    Broadcaster
	checker        ProcessChecker
	localHostname  string
	abandonedLimit time.Duration
}

type Option func(*Reconciler)

func WithProcessChecker(checker ProcessChecker) Option {
	return func(r *Reconciler) {
		r.checker = checker
	}
}

func WithLocalHostname(hostname string) Option {
	return func(r *Reconciler) {
		r.localHostname = hostname
	}
}

func WithAbandonedLimit(d time.Duration) Option {
	return func(r *Reconciler) {
		r.abandonedLimit = d
	}
}

func New(st Store, b Broadcaster, opts ...Option) *Reconciler {
	hostname, _ := os.Hostname()
	r := &Reconciler{
		store:          st,
		broadcaster:    b,
		checker:        &OSProcessChecker{},
		localHostname:  hostname,
		abandonedLimit: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, envID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	now := time.Now().UnixMilli()
	runningAgents, err := r.store.GetRunningAgents(envID)
	if err != nil {
		return 0, fmt.Errorf("reconciliation get running agents: %w", err)
	}

	reconciledCount := 0
	for _, agent := range runningAgents {
		select {
		case <-ctx.Done():
			return reconciledCount, ctx.Err()
		default:
		}

		// Skip agents whose lifecycle is already EXITED.
		// In steady-state, GetRunningAgents filters to active runs with ended_at IS NULL.
		// However, this guard protects against the list-then-load race condition where a
		// running agent exits between the initial ID query in GetRunningAgents and the
		// subsequent GetAgent call, causing the loaded agent to have LifecycleExited.
		if agent.Activity.Lifecycle == types.LifecycleExited {
			continue
		}

		// Only reconcile agents belonging to this local machine if envID is empty
		if envID == "" && r.localHostname != "" && agent.Environment.Hostname != "" && agent.Environment.Hostname != r.localHostname {
			continue
		}

		pid := 0
		var startedAt int64
		if agent.Activity.CurrentRun != nil {
			pid = agent.Activity.CurrentRun.PID
			startedAt = agent.Activity.CurrentRun.StartedAt
		}

		isTerminated := false
		reason := ""
		if pid > 0 {
			if !r.checker.IsProcessAlive(pid, startedAt) {
				isTerminated = true
				reason = fmt.Sprintf("process %d is not running", pid)
			}
		} else {
			// PID <= 0: check if abandoned
			if now-agent.Activity.UpdatedAt >= r.abandonedLimit.Milliseconds() {
				isTerminated = true
				reason = fmt.Sprintf("abandoned run with pid %d", pid)
			}
		}

		if isTerminated {
			if err := r.store.EndCurrentRun(agent.ID, now); err != nil {
				logging.Warn("reconciliation_end_run_failed", "agentId", agent.ID, "err", err)
				continue
			}

			reloaded, err := r.store.GetAgent(agent.ID)
			if err != nil || reloaded == nil {
				agentCopy := agent
				agentCopy.Activity.Lifecycle = types.LifecycleExited
				agentCopy.Activity.CurrentRun = nil
				reloaded = &agentCopy
			}

			if r.broadcaster != nil {
				r.broadcaster.Publish(types.IServerEvent{
					Type:  "agent_upsert",
					At:    now,
					Agent: reloaded,
				})
			}

			reconciledCount++
			logging.Debug("run_reconciled_exited", "agentId", agent.ID, "pid", pid, "reason", reason)
		}
	}

	return reconciledCount, nil
}
