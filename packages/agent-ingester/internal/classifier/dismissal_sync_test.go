package classifier

import (
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

func TestClassifier_DismissCompletion_SynchronousTrailingSummary(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "database_rejection"
		}
		t.Run(name, func(t *testing.T) {
			c, st, frame := completedAgent(t, ":memory:")
			c.SetSyncMode(true)
			if fail {
				_, err := st.DB().Exec(`CREATE TRIGGER reject_dismissal BEFORE UPDATE OF has_unseen_completion ON agents
					WHEN NEW.has_unseen_completion = 0 BEGIN SELECT RAISE(ABORT, 'dismissal rejected'); END`)
				if err != nil {
					t.Fatal(err)
				}
			}
			c.mu.Lock()
			c.agentCtx[frame.AgentID].pendingCandidate = &pendingSummaryCandidate{
				input: summarizer.SummaryInput{
					AgentID: frame.AgentID, Activity: types.ActivityIdle,
					LastAssistantResponse: "Work is complete",
				},
				timestamp: frame.Timestamp, toolKey: "completed-response",
			}
			c.mu.Unlock()

			started, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			c.beforeSummarySaveHook = func(string) { close(started); <-resume }
			go func() {
				defer close(done)
				c.Flush(frame.AgentID)
			}()
			t.Cleanup(func() { close(resume); <-done })
			<-started
			agent, err := c.DismissCompletion(frame.AgentID)
			wantCompletedAt := int64(0)
			if fail {
				wantCompletedAt = frame.Timestamp
				if err == nil || !strings.Contains(err.Error(), "dismissal rejected") || agent != nil {
					t.Fatalf("DismissCompletion = %v, %v; want database rejection", agent, err)
				}
			} else if err != nil || agent == nil {
				t.Fatalf("DismissCompletion = %v, %v", agent, err)
			}
			assertCompletion(t, c, st, frame.AgentID, wantCompletedAt)
			resume <- struct{}{}
			<-done
			assertCompletion(t, c, st, frame.AgentID, wantCompletedAt)
			agent, err = st.GetAgent(frame.AgentID)
			if err != nil || agent == nil {
				t.Fatalf("GetAgent = %v, %v", agent, err)
			}
			if agent.Activity.Action != "Responded to user" {
				t.Errorf("trailing summary action = %q, want Responded to user", agent.Activity.Action)
			}
		})
	}
}
