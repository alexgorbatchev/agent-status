package classifier

import (
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"testing"
)

func TestNativeSummaryPreservesActivityAndLatestRecap(t *testing.T) {
	c, _ := newTestClassifier(t, nil)
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	frame := types.ITelemetryFrame{
		AgentID: "env:claude-code:native", SessionID: "native", EnvID: "env", Harness: types.HarnessClaudeCode, PID: 123,
		Environment: types.IEnvironmentInfo{ID: "env", Platform: "linux"},
		EventType:   "turn_start", Timestamp: 1000,
	}
	before, err := c.ProcessFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	text := "We're finishing the Yarn-to-Bun migration PR and its review fixes"
	frame.EventType = "summary"
	frame.Timestamp = 2000
	frame.Data.Content = &text
	agent, err := c.ProcessFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Activity.Summary == nil || *agent.Activity.Summary != text || agent.Activity.SummaryUpdatedAt == nil || *agent.Activity.SummaryUpdatedAt != 2000 {
		t.Fatalf("recap missing: %+v", agent.Activity)
	}
	if agent.Activity.Activity != before.Activity.Activity || agent.Activity.Action != before.Activity.Action || agent.Activity.HasUnseenCompletion != nil {
		t.Fatalf("recap changed activity: %+v", agent.Activity)
	}
	if agent.Activity.UpdatedAt != before.Activity.UpdatedAt {
		t.Fatalf("recap refreshed activity timestamp: %d, want %d", agent.Activity.UpdatedAt, before.Activity.UpdatedAt)
	}
	for _, tt := range []struct {
		name, text string
		ts         int64
		subagent   *string
	}{
		{"older recap", "old", 1500, nil},
		{"empty recap", "  ", 3000, nil},
		{"subagent recap", "subagent recap", 4000, new("subagent")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			frame.Data.Content = &tt.text
			frame.Timestamp = tt.ts
			frame.SubagentID = tt.subagent
			agent, err := c.ProcessFrame(frame)
			if err != nil {
				t.Fatal(err)
			}
			if agent.Activity.Summary == nil || *agent.Activity.Summary != text || *agent.Activity.SummaryUpdatedAt != 2000 {
				t.Fatalf("recap overwritten: %+v", agent.Activity)
			}
			if agent.Activity.SubagentCount != 0 {
				t.Fatalf("recap created a subagent: %+v", agent.Activity)
			}
		})
	}
	frame.EventType = "thinking"
	frame.Timestamp = 5000
	frame.SubagentID = nil
	agent, err = c.ProcessFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Activity.Summary == nil || *agent.Activity.Summary != text {
		t.Fatalf("recap lost after activity: %+v", agent.Activity)
	}
}
