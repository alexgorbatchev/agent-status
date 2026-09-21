package replay

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/store"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestVerifyAgentState_DirectChecks(t *testing.T) {
	ctx := context.Background()

	t.Run("non-existent agent returns failure check", func(t *testing.T) {
		st, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("opening store: %v", err)
		}
		defer func() { _ = st.Close() }()

		report, err := VerifyAgentState(ctx, st.DB(), "missing-agent-id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Passed {
			t.Errorf("expected report.Passed == false for missing agent")
		}
	})

	t.Run("agent with negative tokens, negative cost, and missing workspace", func(t *testing.T) {
		st, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("opening store: %v", err)
		}
		defer func() { _ = st.Close() }()

		env := types.IEnvironmentInfo{
			ID:              "env-bad",
			Label:           "bad",
			Hostname:        "bad-host",
			Platform:        "darwin",
			LastHeartbeatAt: time.Now().UnixMilli(),
		}
		if err := st.UpsertEnvironment(env); err != nil {
			t.Fatalf("upserting env: %v", err)
		}

		// Insert an agent with empty project/workspace, negative tokens, and negative cost via raw SQL
		_, err = st.DB().Exec(`
			INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, activity, title, action, first_started_at, updated_at, tokens_input, tokens_output, tokens_total, cost_usd)
			VALUES (?, 'bad-1', 'env-bad', 'claude-code', '', '', '', '', '', 'IDLE', '', '', 1000, 1000, -10, -5, -15, -1.0)
		`, "env-bad:claude-code:bad-1")
		if err != nil {
			t.Fatalf("inserting bad agent: %v", err)
		}

		report, err := VerifyAgentState(ctx, st.DB(), "env-bad:claude-code:bad-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Passed {
			t.Errorf("expected report.Passed == false for corrupted agent")
		}

		// Verify that checks failed
		var failedChecks []string
		for _, c := range report.Checks {
			if !c.Passed {
				failedChecks = append(failedChecks, c.Name)
			}
		}
		if len(failedChecks) < 4 {
			t.Errorf("expected multiple failed checks, got %v", failedChecks)
		}
	})

	t.Run("empty logs check", func(t *testing.T) {
		st, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("opening store: %v", err)
		}
		defer func() { _ = st.Close() }()

		env := types.IEnvironmentInfo{
			ID:              "env-ok",
			Label:           "ok",
			Hostname:        "ok-host",
			Platform:        "linux",
			LastHeartbeatAt: time.Now().UnixMilli(),
		}
		_ = st.UpsertEnvironment(env)

		agent := types.IAgentState{
			ID:          "env-ok:claude-code:ok-1",
			SessionID:   "ok-1",
			Harness:     types.HarnessClaudeCode,
			Environment: env,
			Project:     types.IProjectInfo{ID: "p1", Name: "my-proj"},
			Workspace:   types.IWorkspaceInfo{CWD: "/repo", Branch: "main"},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleExited,
				Activity:  types.ActivityIdle,
				Usage: types.ITokenUsage{
					Input: 100, Output: 20, Total: 120,
				},
			},
		}
		_ = st.SaveAgentState(agent)

		// Agent exists, but no telemetry events were recorded
		report, err := VerifyAgentState(ctx, st.DB(), agent.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Passed {
			t.Errorf("expected failure when no telemetry logs exist")
		}
	})

	t.Run("corrupted log framing", func(t *testing.T) {
		st, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("opening store: %v", err)
		}
		defer func() { _ = st.Close() }()

		env := types.IEnvironmentInfo{
			ID:              "env-corrupt",
			Label:           "corrupt",
			Hostname:        "host",
			Platform:        "darwin",
			LastHeartbeatAt: time.Now().UnixMilli(),
		}
		_ = st.UpsertEnvironment(env)

		agent := types.IAgentState{
			ID:          "env-corrupt:pi:c1",
			SessionID:   "c1",
			Harness:     types.HarnessPi,
			Environment: env,
			Project:     types.IProjectInfo{ID: "p1", Name: "proj"},
			Workspace:   types.IWorkspaceInfo{CWD: "/repo", Branch: "main"},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityIdle,
			},
		}
		_ = st.SaveAgentState(agent)

		// Directly insert a corrupted telemetry frame (timestamp 0 and empty event_type)
		_, err = st.DB().Exec(`
			INSERT INTO telemetry_events (agent_id, event_type, timestamp, data_json)
			VALUES (?, '', 0, '{}')
		`, agent.ID)
		if err != nil {
			t.Fatalf("inserting corrupted event: %v", err)
		}

		report, err := VerifyAgentState(ctx, st.DB(), agent.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Passed {
			t.Errorf("expected report.Passed == false for corrupted log frame")
		}
	})

	t.Run("query failure on closed database", func(t *testing.T) {
		db, _ := sql.Open("sqlite3", ":memory:")
		_ = db.Close()

		report, err := VerifyAgentState(ctx, db, "any-agent")
		if err != nil {
			t.Fatalf("expected nil error (report handles it), got %v", err)
		}
		if report.Passed {
			t.Errorf("expected failure on closed db")
		}
	})
}
