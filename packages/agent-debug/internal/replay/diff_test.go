package replay

import (
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestComputeStateDiff(t *testing.T) {
	t.Run("nil prev and curr", func(t *testing.T) {
		d := ComputeStateDiff(1, 10, "turn_start", nil, nil, nil)
		if len(d.Mutations) != 0 {
			t.Errorf("expected 0 mutations, got %d", len(d.Mutations))
		}
	})

	t.Run("nil prev and non-nil curr (initialization)", func(t *testing.T) {
		curr := &types.IAgentState{
			ID: "a-1",
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityIdle,
				Title:     "Initial Title",
				Usage: types.ITokenUsage{
					Input:  1000,
					Output: 200,
				},
				Cost: &types.ICostInfo{Total: 0.05},
			},
		}

		d := ComputeStateDiff(1, 1, "run_started", nil, nil, curr)
		if len(d.Mutations) != 3 {
			t.Fatalf("expected 3 mutations (lifecycle, activity, title), got %d", len(d.Mutations))
		}
		if d.InputDelta != 1000 || d.OutputDelta != 200 || d.CostDelta != 0.05 {
			t.Errorf("unexpected deltas: in=%d, out=%d, cost=%f", d.InputDelta, d.OutputDelta, d.CostDelta)
		}
	})

	t.Run("full state transitions", func(t *testing.T) {
		tool1 := "Bash"
		target1 := "ls -la"
		q1 := "Confirm deletion?"
		err1 := "permission denied"

		prev := &types.IAgentState{
			ID: "a-1",
			Activity: types.IAgentActivity{
				Lifecycle:     types.LifecycleRunning,
				Activity:      types.ActivityIdle,
				Title:         "Old Title",
				Action:        "Old Action",
				ActiveTool:    nil,
				ActiveTarget:  nil,
				Question:      nil,
				LastError:     nil,
				SubagentCount: 0,
				Usage:         types.ITokenUsage{Input: 100, Output: 20},
				Cost:          &types.ICostInfo{Total: 0.01},
			},
		}

		curr := &types.IAgentState{
			ID: "a-1",
			Activity: types.IAgentActivity{
				Lifecycle:     types.LifecycleExited,
				Activity:      types.ActivityBusy,
				Title:         "New Title",
				Action:        "New Action",
				ActiveTool:    &tool1,
				ActiveTarget:  &target1,
				Question:      &q1,
				LastError:     &err1,
				SubagentCount: 2,
				Usage:         types.ITokenUsage{Input: 300, Output: 80},
				Cost:          &types.ICostInfo{Total: 0.04},
			},
		}

		d := ComputeStateDiff(5, 42, "turn_end", nil, prev, curr)
		if len(d.Mutations) != 9 {
			t.Fatalf("expected 9 mutations, got %d", len(d.Mutations))
		}
		if d.InputDelta != 200 || d.OutputDelta != 60 {
			t.Errorf("unexpected token deltas: in=%d, out=%d", d.InputDelta, d.OutputDelta)
		}
		if d.CostDelta < 0.029 || d.CostDelta > 0.031 {
			t.Errorf("unexpected cost delta: %f", d.CostDelta)
		}
	})
}

func TestCloneAgentState(t *testing.T) {
	if CloneAgentState(nil) != nil {
		t.Errorf("CloneAgentState(nil) should be nil")
	}

	tool := "Edit"
	target := "main.go"
	q := "Proceed?"
	errStr := "boom"
	think := "reasoning..."
	phase := types.AgentPhase("execution")
	cost := types.ICostInfo{Currency: "USD", Total: 1.23}
	endedAt := int64(9999)
	run := types.IRunInfo{ID: "run-1", PID: 1234, StartedAt: 1000, EndedAt: &endedAt}

	orig := &types.IAgentState{
		ID: "a-1",
		Activity: types.IAgentActivity{
			Lifecycle:           types.LifecycleRunning,
			Activity:            types.ActivityBusy,
			ActiveTool:          &tool,
			ActiveTarget:        &target,
			Question:            &q,
			LastError:           &errStr,
			LastThinkingExcerpt: &think,
			Phase:               &phase,
			Cost:                &cost,
			CurrentRun:          &run,
		},
	}

	cloned := CloneAgentState(orig)
	if cloned == orig {
		t.Errorf("expected different pointers")
	}
	if *cloned.Activity.ActiveTool != tool {
		t.Errorf("cloned ActiveTool mismatch")
	}
	// Verify deep copy: mutating cloned pointer values must not affect orig
	newTool := "Read"
	cloned.Activity.ActiveTool = &newTool
	if *orig.Activity.ActiveTool != "Edit" {
		t.Errorf("mutation leaked from clone to original")
	}
}
