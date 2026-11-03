package classifier

import (
	"errors"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
)

func TestClassifier_DismissCompletion_RestartedContext(t *testing.T) {
	for _, tt := range []struct {
		name          string
		failDismissal bool
		newCompletion bool
	}{
		{name: "dismissed_completion"},
		{name: "failed_dismissal", failDismissal: true},
		{name: "new_completion", newCompletion: true},
		{name: "new_completion_after_failed_dismissal", failDismissal: true, newCompletion: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original, st, frame := completedAgent(t, ":memory:")
			previousCompletion := frame.Timestamp
			if tt.newCompletion {
				frame.Seq++
				frame.Timestamp += 1000
				frame.EventType = "turn_start"
				if _, err := original.ProcessFrame(frame); err != nil {
					t.Fatal(err)
				}
				previousCompletion = 0
			}
			if err := original.Close(); err != nil {
				t.Fatal(err)
			}
			c := New(st, nil, original.summarizer)
			c.SetIdleGracePeriod(0)
			t.Cleanup(func() { _ = c.Close() })
			if tt.failDismissal {
				if _, err := st.DB().Exec(`CREATE TRIGGER reject_dismissal BEFORE UPDATE OF has_unseen_completion ON agents
					WHEN NEW.has_unseen_completion = 0 BEGIN SELECT RAISE(ABORT, 'dismissal rejected'); END`); err != nil {
					t.Fatal(err)
				}
			}

			// A restarted classifier has no agent context yet. Hold the frame's
			// initial SQLite snapshot while dismissal hydrates and updates it.
			started, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			c.afterFrameReadHook = func(string) { close(started); <-resume }
			frame.Seq++
			frame.Timestamp += 1000
			frame.EventType = "text"
			if tt.newCompletion {
				frame.EventType = "turn_end"
			}
			processed := make(chan error, 1)
			go func() {
				defer close(done)
				_, err := c.ProcessFrame(frame)
				processed <- err
			}()
			t.Cleanup(func() { close(resume); <-done })
			<-started

			_, err := c.DismissCompletion(frame.AgentID)
			if tt.failDismissal {
				if err == nil || !strings.Contains(err.Error(), "dismissal rejected") {
					t.Fatalf("expected database rejection, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				previousCompletion = 0
			}
			assertCompletion(t, c, st, frame.AgentID, previousCompletion)
			resume <- struct{}{}
			<-done
			if err := <-processed; err != nil {
				t.Fatal(err)
			}
			wantCompletion := previousCompletion
			if tt.newCompletion {
				wantCompletion = frame.Timestamp
			}
			assertCompletion(t, c, st, frame.AgentID, wantCompletion)
		})
	}
}

func TestClassifier_DismissCompletion_ContextLoadErrors(t *testing.T) {
	for _, failure := range []string{"absent_row", "closed_database", "nil_store"} {
		t.Run(failure, func(t *testing.T) {
			original, st, frame := completedAgent(t, ":memory:")
			if err := original.Close(); err != nil {
				t.Fatal(err)
			}
			id := frame.AgentID
			switch failure {
			case "absent_row":
				id = "absent"
			case "closed_database":
				if err := st.Close(); err != nil {
					t.Fatal(err)
				}
			case "nil_store":
				st = nil
			}
			c := New(st, nil, original.summarizer)
			t.Cleanup(func() { _ = c.Close() })
			c.SetOnUpdate(func(*types.IAgentState) { t.Error("failed dismissal broadcast an update") })
			agent, err := c.DismissCompletion(id)
			if agent != nil || err == nil {
				t.Fatalf("DismissCompletion = %v, %v; want error without agent", agent, err)
			}
			if errors.Is(err, store.ErrAgentNotFound) != (failure == "absent_row") {
				t.Errorf("incorrect not-found classification: %v", err)
			}
			if len(c.agentCtx) != 0 {
				t.Error("failed context load installed an agent context")
			}
		})
	}
}
