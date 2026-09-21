package replay

import (
	"fmt"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
)

func ptrToStr(p *string) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%q", *p)
}

// ComputeStateDiff calculates mutations and token/cost deltas between two agent states.
func ComputeStateDiff(stepIndex int, lineNum int, eventType string, subagentID *string, prev *types.IAgentState, curr *types.IAgentState) output.StateDiff {
	diff := output.StateDiff{
		StepIndex:  stepIndex,
		LineNumber: lineNum,
		EventType:  eventType,
		SubagentID: subagentID,
	}

	if prev == nil && curr != nil {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "Lifecycle",
			From:  "none",
			To:    string(curr.Activity.Lifecycle),
		})
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "Activity",
			From:  "none",
			To:    string(curr.Activity.Activity),
		})
		if curr.Activity.Title != "" {
			diff.Mutations = append(diff.Mutations, output.FieldMutation{
				Field: "Title",
				From:  "\"\"",
				To:    fmt.Sprintf("%q", curr.Activity.Title),
			})
		}
		diff.InputDelta = curr.Activity.Usage.Input
		diff.OutputDelta = curr.Activity.Usage.Output
		if curr.Activity.Cost != nil {
			diff.CostDelta = curr.Activity.Cost.Total
		}
		return diff
	}

	if prev == nil || curr == nil {
		return diff
	}

	if prev.Activity.Lifecycle != curr.Activity.Lifecycle {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "Lifecycle",
			From:  string(prev.Activity.Lifecycle),
			To:    string(curr.Activity.Lifecycle),
		})
	}

	if prev.Activity.Activity != curr.Activity.Activity {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "Activity",
			From:  string(prev.Activity.Activity),
			To:    string(curr.Activity.Activity),
		})
	}

	if prev.Activity.Title != curr.Activity.Title {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "Title",
			From:  fmt.Sprintf("%q", prev.Activity.Title),
			To:    fmt.Sprintf("%q", curr.Activity.Title),
		})
	}

	if prev.Activity.Action != curr.Activity.Action {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "Action",
			From:  fmt.Sprintf("%q", prev.Activity.Action),
			To:    fmt.Sprintf("%q", curr.Activity.Action),
		})
	}

	if ptrToStr(prev.Activity.ActiveTool) != ptrToStr(curr.Activity.ActiveTool) {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "ActiveTool",
			From:  ptrToStr(prev.Activity.ActiveTool),
			To:    ptrToStr(curr.Activity.ActiveTool),
		})
	}

	if ptrToStr(prev.Activity.ActiveTarget) != ptrToStr(curr.Activity.ActiveTarget) {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "ActiveTarget",
			From:  ptrToStr(prev.Activity.ActiveTarget),
			To:    ptrToStr(curr.Activity.ActiveTarget),
		})
	}

	if ptrToStr(prev.Activity.Question) != ptrToStr(curr.Activity.Question) {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "Question",
			From:  ptrToStr(prev.Activity.Question),
			To:    ptrToStr(curr.Activity.Question),
		})
	}

	if ptrToStr(prev.Activity.LastError) != ptrToStr(curr.Activity.LastError) {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "LastError",
			From:  ptrToStr(prev.Activity.LastError),
			To:    ptrToStr(curr.Activity.LastError),
		})
	}

	if prev.Activity.SubagentCount != curr.Activity.SubagentCount {
		diff.Mutations = append(diff.Mutations, output.FieldMutation{
			Field: "SubagentCount",
			From:  fmt.Sprintf("%d", prev.Activity.SubagentCount),
			To:    fmt.Sprintf("%d", curr.Activity.SubagentCount),
		})
	}

	diff.InputDelta = curr.Activity.Usage.Input - prev.Activity.Usage.Input
	diff.OutputDelta = curr.Activity.Usage.Output - prev.Activity.Usage.Output

	prevCost := 0.0
	if prev.Activity.Cost != nil {
		prevCost = prev.Activity.Cost.Total
	}
	currCost := 0.0
	if curr.Activity.Cost != nil {
		currCost = curr.Activity.Cost.Total
	}
	if currCost > prevCost {
		diff.CostDelta = currCost - prevCost
	}

	return diff
}

// CloneAgentState creates a deep copy of an IAgentState for accurate diff tracking.
func CloneAgentState(s *types.IAgentState) *types.IAgentState {
	if s == nil {
		return nil
	}
	clone := *s
	clone.Activity = s.Activity

	if s.Activity.ActiveTool != nil {
		v := *s.Activity.ActiveTool
		clone.Activity.ActiveTool = &v
	}
	if s.Activity.ActiveTarget != nil {
		v := *s.Activity.ActiveTarget
		clone.Activity.ActiveTarget = &v
	}
	if s.Activity.Question != nil {
		v := *s.Activity.Question
		clone.Activity.Question = &v
	}
	if s.Activity.LastError != nil {
		v := *s.Activity.LastError
		clone.Activity.LastError = &v
	}
	if s.Activity.LastThinkingExcerpt != nil {
		v := *s.Activity.LastThinkingExcerpt
		clone.Activity.LastThinkingExcerpt = &v
	}
	if s.Activity.Phase != nil {
		v := *s.Activity.Phase
		clone.Activity.Phase = &v
	}
	if s.Activity.Cost != nil {
		c := *s.Activity.Cost
		clone.Activity.Cost = &c
	}
	if s.Activity.CurrentRun != nil {
		r := *s.Activity.CurrentRun
		if s.Activity.CurrentRun.EndedAt != nil {
			ended := *s.Activity.CurrentRun.EndedAt
			r.EndedAt = &ended
		}
		clone.Activity.CurrentRun = &r
	}

	return &clone
}
