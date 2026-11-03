package classifier

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

func completedAgent(t *testing.T, dbPath string) (*Classifier, *store.Store, types.ITelemetryFrame) {
	t.Helper()
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatal(err)
	}
	c := New(st, nil, sum)
	c.SetIdleGracePeriod(0)
	t.Cleanup(func() { _ = c.Close() })
	frame := types.ITelemetryFrame{
		Version: "1.0", EnvID: "env", AgentID: "env:pi:dismiss", SessionID: "dismiss",
		Harness: types.HarnessPi, PID: 1,
		Environment: types.IEnvironmentInfo{ID: "env", Hostname: "host", Platform: "linux"},
		Project:     types.IProjectInfo{ID: "project", Name: "project"},
		Workspace:   types.IWorkspaceInfo{CWD: "/workspace"},
	}
	for _, event := range []string{"run_started", "turn_start", "turn_end"} {
		frame.Seq++
		frame.Timestamp += 1000
		frame.EventType = event
		if event == "turn_end" {
			stop := "end_turn"
			frame.Data.StopReason = &stop
		}
		if _, err := c.ProcessFrame(frame); err != nil {
			t.Fatal(err)
		}
	}
	assertCompletion(t, c, st, frame.AgentID, frame.Timestamp)
	return c, st, frame
}

func assertCompletion(t *testing.T, c *Classifier, st *store.Store, id string, completedAt int64) {
	t.Helper()
	c.mu.Lock()
	actx := c.agentCtx[id]
	unseen, at := actx.hasUnseenCompletion, actx.completedAt
	c.mu.Unlock()
	check := func(where string, unseen *bool, at *int64) {
		t.Helper()
		if completedAt == 0 {
			if (unseen != nil && *unseen) || at != nil {
				t.Errorf("%s retains dismissed completion: unseen=%v completedAt=%v", where, unseen, at)
			}
		} else if unseen == nil || !*unseen || at == nil || *at != completedAt {
			t.Errorf("%s lost completion at %d: unseen=%v completedAt=%v", where, completedAt, unseen, at)
		}
	}
	check("cache", unseen, at)
	agent, err := st.GetAgent(id)
	if err != nil || agent == nil {
		t.Fatalf("GetAgent: agent=%v err=%v", agent, err)
	}
	check("store", agent.Activity.HasUnseenCompletion, agent.Activity.CompletedAt)
}

func TestClassifier_DismissCompletion_PersistenceAndPendingSummary(t *testing.T) {
	for _, fail := range []bool{true, false} {
		name := "success"
		if fail {
			name = "database_rejection"
		}
		t.Run(name, func(t *testing.T) {
			c, st, frame := completedAgent(t, ":memory:")
			if fail {
				_, err := st.DB().Exec(`CREATE TRIGGER reject_dismissal BEFORE UPDATE OF has_unseen_completion ON agents
					WHEN NEW.has_unseen_completion = 0 BEGIN SELECT RAISE(ABORT, 'dismissal rejected'); END`)
				if err != nil {
					t.Fatal(err)
				}
			}

			// Pause a real summary before it acquires the per-agent writer lock.
			started, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			c.beforeSummarySaveHook = func(string) { close(started); <-resume }
			var broadcasts atomic.Int32
			c.SetOnUpdate(func(*types.IAgentState) { broadcasts.Add(1) })
			go func() {
				defer close(done)
				c.processSummaryJob(frame.AgentID, summaryJob{input: summarizer.SummaryInput{
					Activity: types.ActivityIdle, LastAssistantResponse: "Work is complete",
				}})
			}()
			t.Cleanup(func() { close(resume); <-done })
			<-started

			agent, err := c.DismissCompletion(frame.AgentID)
			wantCompletedAt := int64(0)
			wantBroadcasts := int32(1)
			if fail {
				wantCompletedAt = frame.Timestamp
				wantBroadcasts = 0
				if err == nil || !strings.Contains(err.Error(), "dismissal rejected") || agent != nil {
					t.Fatalf("DismissCompletion = %v, %v; want database rejection", agent, err)
				}
			} else if err != nil || agent == nil {
				t.Fatalf("DismissCompletion = %v, %v", agent, err)
			}
			assertCompletion(t, c, st, frame.AgentID, wantCompletedAt)
			if got := broadcasts.Load(); got != wantBroadcasts {
				t.Errorf("dismissal broadcasts = %d, want %d", got, wantBroadcasts)
			}
			if fail {
				if _, err := st.DB().Exec("DROP TRIGGER reject_dismissal"); err != nil {
					t.Fatal(err)
				}
			}
			resume <- struct{}{}
			<-done
			assertCompletion(t, c, st, frame.AgentID, wantCompletedAt)
			saved, err := st.GetAgent(frame.AgentID)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Activity.Action != "Responded to user" || broadcasts.Load() != wantBroadcasts+1 {
				t.Fatalf("pending summary did not persist and broadcast: action=%q broadcasts=%d", saved.Activity.Action, broadcasts.Load())
			}
		})
	}
}

func waitForDismissalCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for concurrent dismissal state")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestClassifier_DismissCompletion_ConcurrentFrame(t *testing.T) {
	for _, newCompletion := range []bool{false, true} {
		name := "retained_completion"
		if newCompletion {
			name = "new_completion"
		}
		t.Run(name, func(t *testing.T) {
			c, st, frame := completedAgent(t, filepath.Join(t.TempDir(), "agents.db"))
			if newCompletion {
				frame.Seq++
				frame.Timestamp += 1000
				frame.EventType = "turn_start"
				if _, err := c.ProcessFrame(frame); err != nil {
					t.Fatal(err)
				}
			}
			c.mu.Lock()
			generation := c.agentCtx[frame.AgentID].saveGeneration
			c.mu.Unlock()

			// A real SQLite writer holds dismissal in its UPDATE, while WAL permits
			// ProcessFrame to read and prepare the next state before its writer lock.
			tx, err := st.DB().Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.Exec("UPDATE agents SET title = title WHERE id = ?", frame.AgentID); err != nil {
				t.Fatal(err)
			}
			dismissed := make(chan error, 1)
			var writers sync.WaitGroup
			t.Cleanup(writers.Wait)
			writers.Add(1)
			go func() {
				defer writers.Done()
				_, err := c.DismissCompletion(frame.AgentID)
				dismissed <- err
			}()
			waitForDismissalCondition(t, func() bool { return st.DB().Stats().InUse >= 2 })

			frame.Seq++
			frame.Timestamp += 1000
			frame.EventType = "turn_end"
			response := "The requested work is complete"
			frame.Data.Content = &response
			processed := make(chan error, 1)
			writers.Add(1)
			go func() {
				defer writers.Done()
				_, err := c.ProcessFrame(frame)
				processed <- err
			}()
			waitForDismissalCondition(t, func() bool {
				c.mu.Lock()
				defer c.mu.Unlock()
				actx := c.agentCtx[frame.AgentID]
				return actx.saveGeneration > generation && actx.rootAction == "Responded to user"
			})
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-dismissed; err != nil {
				t.Fatal(err)
			}
			if err := <-processed; err != nil {
				t.Fatal(err)
			}
			wantCompletedAt := int64(0)
			if newCompletion {
				wantCompletedAt = frame.Timestamp
			}
			assertCompletion(t, c, st, frame.AgentID, wantCompletedAt)
		})
	}
}
