package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS environments (
    id                TEXT PRIMARY KEY,
    label             TEXT NOT NULL,
    hostname          TEXT NOT NULL,
    platform          TEXT NOT NULL CHECK (platform IN ('darwin','linux')),
    user              TEXT NOT NULL,
    dropped_frames    INTEGER NOT NULL DEFAULT 0,
    last_heartbeat_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    pid             INTEGER NOT NULL,
    harness_version TEXT NOT NULL,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER
);

CREATE TABLE IF NOT EXISTS agents (
    id                    TEXT PRIMARY KEY,
    session_id            TEXT NOT NULL,
    environment_id        TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    harness               TEXT NOT NULL CHECK (harness IN ('claude-code','pi','codex','opencode')),
    project_id            TEXT NOT NULL,
    project_name          TEXT NOT NULL,
    canonical_root        TEXT NOT NULL,
    branch                TEXT NOT NULL,
    is_detached           INTEGER NOT NULL DEFAULT 0,
    is_worktree           INTEGER NOT NULL DEFAULT 0,
    worktree_rel_path     TEXT,
    cwd                   TEXT NOT NULL,
    current_run_id        TEXT REFERENCES runs(id) ON DELETE SET NULL,
    activity              TEXT NOT NULL CHECK (activity IN ('BUSY','WAITING_QUESTION','ERROR','RATE_LIMITED','IDLE')),
    title                 TEXT NOT NULL DEFAULT '',
    action                TEXT NOT NULL DEFAULT '',
    action_stale          INTEGER NOT NULL DEFAULT 0,
    phase                 TEXT CHECK (phase IS NULL OR phase IN ('planning','editing','testing','debugging','investigating')),
    active_tool           TEXT,
    active_target         TEXT,
    active_tool_started_at INTEGER,
    question              TEXT,
    last_thinking_excerpt TEXT,
    last_error            TEXT,
    tokens_input          INTEGER NOT NULL DEFAULT 0,
    tokens_output         INTEGER NOT NULL DEFAULT 0,
    tokens_cache_read     INTEGER NOT NULL DEFAULT 0,
    tokens_cache_write    INTEGER NOT NULL DEFAULT 0,
    tokens_reasoning      INTEGER NOT NULL DEFAULT 0,
    tokens_total          INTEGER NOT NULL DEFAULT 0,
    cost_usd              REAL,
    subagent_count        INTEGER NOT NULL DEFAULT 0,
    active_subagents      TEXT,
    first_started_at      INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL,
    is_hidden             INTEGER NOT NULL DEFAULT 0,
    hidden_at             INTEGER,
    model                 TEXT,
    context_window        INTEGER,
    context_fullness      REAL,
    rate_limit_reset_at   INTEGER,
    retry_after_seconds   INTEGER,
    has_unseen_completion INTEGER NOT NULL DEFAULT 0,
    completed_at          INTEGER,
    UNIQUE (environment_id, harness, session_id)
);

CREATE TABLE IF NOT EXISTS telemetry_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    subagent_id   TEXT,
    event_type    TEXT NOT NULL,
    timestamp     INTEGER NOT NULL,
    data_json     TEXT NOT NULL
);
`

func setupTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("opening in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("applying schema: %v", err)
	}

	return New(db)
}

func seedTestData(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// 1. Insert environments (one fresh, one stale)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
		VALUES ('env1', 'work-macbook', 'macbook.local', 'darwin', 'alex', 0, ?),
		       ('env2', 'stale-linux', 'linux.local', 'linux', 'ci', 0, ?)
	`, now-1000, now-40000)
	if err != nil {
		t.Fatalf("inserting environments: %v", err)
	}

	// 2. Insert runs
	endedAt := now - 1000
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
		VALUES ('run1', 'env1:claude-code:sess1', 1234, '2.1.277', ?, NULL),
		       ('run2', 'env1:claude-code:sess3', 5678, '2.1.277', ?, ?)
	`, now-5000, now-60000, endedAt)
	if err != nil {
		t.Fatalf("inserting runs: %v", err)
	}

	// 3. Insert agent 1 (running on repo1, waiting question, with cost, worktree=0)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, active_tool, active_target, question,
			last_thinking_excerpt, last_error,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_reasoning, tokens_total, cost_usd, subagent_count,
			first_started_at, updated_at,
			model, context_window, context_fullness
		) VALUES (
			'env1:claude-code:sess1', 'sess1', 'env1', 'claude-code',
			'github.com/org/repo1', 'repo1', '/path/to/repo1',
			'main', 0, 0, NULL, '/path/to/repo1',
			'run1', 'WAITING_QUESTION', 'Build Feature', 'Asking user a question', 0,
			'editing', 'AskUserQuestion', 'target1', 'Should I proceed?',
			'thinking about options', NULL,
			100, 50, 20, 10,
			0, 180, 1.25, 2,
			?, ?,
			'claude-3-7-sonnet', 200000, 0.25
		)
	`, now-10000, now)
	if err != nil {
		t.Fatalf("inserting agent 1: %v", err)
	}

	// 4. Insert agent 2 (exited on repo1, error, with cost, worktree=1)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, last_error,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_reasoning, tokens_total, cost_usd, subagent_count,
			first_started_at, updated_at,
			is_hidden, hidden_at
		) VALUES (
			'env1:claude-code:sess2', 'sess2', 'env1', 'claude-code',
			'github.com/org/repo1', 'repo1', '/path/to/repo1',
			'feat-x', 1, 1, '.workspaces/feat-x', '/path/to/repo1/.workspaces/feat-x',
			NULL, 'ERROR', 'Fix Bug', 'Compilation error', 1,
			'debugging', 'Build failed with syntax error',
			200, 100, 0, 0,
			0, 300, 2.50, 0,
			?, ?,
			1, ?
		)
	`, now-20000, now-1000, now-1000)
	if err != nil {
		t.Fatalf("inserting agent 2: %v", err)
	}

	// 5. Insert agent 3 (running on repo2 with run2 having ended_at set, IDLE, null cost_usd)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, active_tool, active_target, question,
			last_thinking_excerpt, last_error,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_reasoning, tokens_total, cost_usd, subagent_count,
			first_started_at, updated_at
		) VALUES (
			'env2:pi:sess3', 'sess3', 'env2', 'pi',
			'github.com/org/repo2', 'repo2', '/path/to/repo2',
			'main', 0, 0, NULL, '/path/to/repo2',
			'run2', 'IDLE', 'Idle agent', 'Waiting for input', 0,
			NULL, NULL, NULL, NULL,
			NULL, NULL,
			50, 25, 0, 0,
			0, 75, NULL, 0,
			?, ?
		)
	`, now-30000, now-2000)
	if err != nil {
		t.Fatalf("inserting agent 3: %v", err)
	}

	// 6. Insert telemetry events for agent 1
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (1, 'env1:claude-code:sess1', NULL, 'turn_start', ?, '{"role":"user","content":"start"}'),
		       (2, 'env1:claude-code:sess1', 'sub1', 'tool_call', ?, '{"toolName":"Bash","toolInput":{"command":"ls"}}'),
		       (3, 'env1:claude-code:sess1', 'sub1', 'tool_result', ?, 'invalid json data'),
		       (4, 'env1:claude-code:sess1', 'sub2', 'thinking', ?, '')
	`, now-5000, now-4000, now-3000, now-2000)
	if err != nil {
		t.Fatalf("inserting telemetry events: %v", err)
	}
}

func TestStore_OpenReadOnlyAndDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("creating test db: %v", err)
	}
	_, err = db.Exec(schemaSQL)
	if err != nil {
		t.Fatalf("creating schema in file: %v", err)
	}
	_ = db.Close()

	roStore, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	if roStore.DB() == nil {
		t.Errorf("expected DB() not nil")
	}
	if err := roStore.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Close on empty/nil store
	emptyStore := &Store{}
	if err := emptyStore.Close(); err != nil {
		t.Errorf("Close() on nil db returned error: %v", err)
	}
}

func TestStore_GetStatus(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	t.Run("success with seeded data", func(t *testing.T) {
		status, err := s.GetStatus(context.Background())
		if err != nil {
			t.Fatalf("GetStatus() error = %v", err)
		}

		// Agent 1 (run1, current_run_id != null) -> active
		// Agent 3 (run2, current_run_id != null) -> active
		if status.ActiveAgentCount != 2 {
			t.Errorf("ActiveAgentCount = %d, want 2", status.ActiveAgentCount)
		}
		// Agent 1 has WAITING_QUESTION and current_run_id != null -> attention count = 1
		if status.RequiresAttentionCount != 1 {
			t.Errorf("RequiresAttentionCount = %d, want 1", status.RequiresAttentionCount)
		}
		if status.TotalFleetSpend != 3.75 {
			t.Errorf("TotalFleetSpend = %f, want 3.75", status.TotalFleetSpend)
		}
		if len(status.Environments) != 2 {
			t.Errorf("len(Environments) = %d, want 2", len(status.Environments))
		}
	})

	t.Run("empty database", func(t *testing.T) {
		emptyStore := setupTestDB(t)
		status, err := emptyStore.GetStatus(context.Background())
		if err != nil {
			t.Fatalf("GetStatus() on empty DB error = %v", err)
		}
		if status.ActiveAgentCount != 0 {
			t.Errorf("ActiveAgentCount = %d, want 0", status.ActiveAgentCount)
		}
		if status.TotalFleetSpend != 0.0 {
			t.Errorf("TotalFleetSpend = %f, want 0.0", status.TotalFleetSpend)
		}
	})

	t.Run("db error", func(t *testing.T) {
		sErr := setupTestDB(t)
		_ = sErr.Close()
		_, err := sErr.GetStatus(context.Background())
		if err == nil {
			t.Fatal("expected error on closed db, got nil")
		}
	})

	t.Run("stalled tool increases requires attention count", func(t *testing.T) {
		sStall := setupTestDB(t)
		ctx := context.Background()
		now := time.Now().UnixMilli()

		_, err := sStall.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-stall', 'macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
		`, now)
		if err != nil {
			t.Fatalf("inserting environment: %v", err)
		}

		_, err = sStall.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-stall', 'agent-stall', 1234, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run: %v", err)
		}

		// Agent running in BUSY state with active tool started 200 seconds ago (>= 180s = stalled)
		stalledToolStart := now - 200000
		_, err = sStall.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'agent-stall', 'sess-stall', 'env-stall', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-stall',
				'BUSY', 'Rebasing', 'Executing git rebase', 0, 'editing', 'bash', 'git rebase',
				?,
				0, 0, 0, 0, 0, 0, 0, 0, ?, ?
			)
		`, stalledToolStart, now-300000, now)
		if err != nil {
			t.Fatalf("inserting stalled agent: %v", err)
		}

		status, err := sStall.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if status.ActiveAgentCount != 1 {
			t.Errorf("ActiveAgentCount = %d, want 1", status.ActiveAgentCount)
		}
		// Since agent is in BUSY with tool >= 180s, it must be counted in RequiresAttentionCount
		if status.RequiresAttentionCount != 1 {
			t.Errorf("RequiresAttentionCount = %d, want 1", status.RequiresAttentionCount)
		}
	})

	t.Run("stale environment suppresses requires attention count", func(t *testing.T) {
		sStale := setupTestDB(t)
		ctx := context.Background()
		now := time.Now().UnixMilli()

		// Stale environment: last_heartbeat_at was 40 seconds ago (> 30s threshold)
		staleHeartbeat := now - 40000
		_, err := sStale.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-stale', 'macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
		`, staleHeartbeat)
		if err != nil {
			t.Fatalf("inserting environment: %v", err)
		}

		_, err = sStale.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-stale', 'agent-stale', 1234, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run: %v", err)
		}

		// Agent has WAITING_QUESTION (which requires attention in active environments), but environment is stale
		_, err = sStale.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'agent-stale', 'sess-stale', 'env-stale', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-stale',
				'WAITING_QUESTION', 'Question', 'Awaiting input', 0, 'editing', NULL, NULL,
				NULL,
				0, 0, 0, 0, 0, 0, 0, 0, ?, ?
			)
		`, now-300000, now)
		if err != nil {
			t.Fatalf("inserting agent: %v", err)
		}

		status, err := sStale.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if status.ActiveAgentCount != 1 {
			t.Errorf("ActiveAgentCount = %d, want 1", status.ActiveAgentCount)
		}
		if status.RequiresAttentionCount != 0 {
			t.Errorf("RequiresAttentionCount = %d, want 0 (stale environment must suppress attention)", status.RequiresAttentionCount)
		}
	})

	t.Run("non-positive last_heartbeat_at is treated as stale", func(t *testing.T) {
		tests := []struct {
			name        string
			heartbeatAt int64
		}{
			{name: "zero heartbeat", heartbeatAt: 0},
			{name: "negative heartbeat", heartbeatAt: -100},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				st := setupTestDB(t)
				ctx := context.Background()
				now := int64(20000)

				_, err := st.db.ExecContext(ctx, `
					INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
					VALUES ('env-nonpositive', 'macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
				`, tt.heartbeatAt)
				if err != nil {
					t.Fatalf("inserting environment: %v", err)
				}

				_, err = st.db.ExecContext(ctx, `
					INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
					VALUES ('run-nonpositive', 'agent-nonpositive', 1234, '1.0', ?, NULL)
				`, now-10000)
				if err != nil {
					t.Fatalf("inserting run: %v", err)
				}

				_, err = st.db.ExecContext(ctx, `
					INSERT INTO agents (
						id, session_id, environment_id, harness, project_id, project_name, canonical_root,
						branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
						activity, title, action, action_stale, phase, active_tool, active_target,
						active_tool_started_at,
						tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
						tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
					) VALUES (
						'agent-nonpositive', 'sess-nonpositive', 'env-nonpositive', 'claude-code', 'proj', 'proj', '/repo',
						'main', 0, 0, NULL, '/repo', 'run-nonpositive',
						'WAITING_QUESTION', 'Question', 'Awaiting input', 0, 'editing', NULL, NULL,
						NULL,
						0, 0, 0, 0, 0, 0, 0, 0, ?, ?
					)
				`, now-10000, now)
				if err != nil {
					t.Fatalf("inserting agent: %v", err)
				}

				status, err := st.GetStatusAt(ctx, now)
				if err != nil {
					t.Fatalf("GetStatusAt failed: %v", err)
				}

				if status.ActiveAgentCount != 1 {
					t.Errorf("ActiveAgentCount = %d, want 1", status.ActiveAgentCount)
				}
				if status.RequiresAttentionCount != 0 {
					t.Errorf("RequiresAttentionCount = %d, want 0 (non-positive heartbeat must suppress attention)", status.RequiresAttentionCount)
				}
			})
		}
	})

	t.Run("non-busy agent with old tool start does not increase requires attention count", func(t *testing.T) {
		sNonBusy := setupTestDB(t)
		ctx := context.Background()
		now := time.Now().UnixMilli()

		_, err := sNonBusy.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-active', 'macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
		`, now)
		if err != nil {
			t.Fatalf("inserting environment: %v", err)
		}

		_, err = sNonBusy.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-nonbusy', 'agent-nonbusy', 1234, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run: %v", err)
		}

		// Agent is IDLE, but has active_tool_started_at from 200s ago (stalled tool if BUSY, but here activity is IDLE)
		stalledToolStart := now - 200000
		_, err = sNonBusy.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'agent-nonbusy', 'sess-nonbusy', 'env-active', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-nonbusy',
				'IDLE', 'Idle', 'Waiting', 0, 'editing', 'bash', 'git rebase',
				?,
				0, 0, 0, 0, 0, 0, 0, 0, ?, ?
			)
		`, stalledToolStart, now-300000, now-10000)
		if err != nil {
			t.Fatalf("inserting agent: %v", err)
		}

		status, err := sNonBusy.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if status.ActiveAgentCount != 1 {
			t.Errorf("ActiveAgentCount = %d, want 1", status.ActiveAgentCount)
		}
		if status.RequiresAttentionCount != 0 {
			t.Errorf("RequiresAttentionCount = %d, want 0 (non-busy agent with old tool start must not require attention)", status.RequiresAttentionCount)
		}
	})

	t.Run("busy agent with old tool start but empty tool or zero timestamps does not require attention", func(t *testing.T) {
		sEdge := setupTestDB(t)
		ctx := context.Background()
		now := time.Now().UnixMilli()

		_, err := sEdge.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-edge', 'macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
		`, now)
		if err != nil {
			t.Fatalf("inserting environment: %v", err)
		}

		// Agent 1: BUSY, active_tool = '', active_tool_started_at is 200s ago, updated_at fresh
		stalledToolStart := now - 200000
		_, err = sEdge.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-edge-1', 'agent-edge-1', 1001, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run 1: %v", err)
		}
		_, err = sEdge.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'agent-edge-1', 'sess-edge-1', 'env-edge', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-edge-1',
				'BUSY', 'Working', 'Executing', 0, 'editing', '', 'target',
				?,
				0, 0, 0, 0, 0, 0, 0, 0, ?, ?
			)
		`, stalledToolStart, now-300000, now-10000)
		if err != nil {
			t.Fatalf("inserting agent 1: %v", err)
		}

		// Agent 2: BUSY, updated_at = 0
		_, err = sEdge.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-edge-2', 'agent-edge-2', 1002, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run 2: %v", err)
		}
		_, err = sEdge.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'agent-edge-2', 'sess-edge-2', 'env-edge', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-edge-2',
				'BUSY', 'Working', 'Executing', 0, 'editing', NULL, NULL,
				NULL,
				0, 0, 0, 0, 0, 0, 0, 0, ?, 0
			)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting agent 2: %v", err)
		}

		status, err := sEdge.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if status.ActiveAgentCount != 2 {
			t.Errorf("ActiveAgentCount = %d, want 2", status.ActiveAgentCount)
		}
		if status.RequiresAttentionCount != 0 {
			t.Errorf("RequiresAttentionCount = %d, want 0", status.RequiresAttentionCount)
		}
	})

	t.Run("stalled silence increases requires attention count", func(t *testing.T) {
		sSilence := setupTestDB(t)
		ctx := context.Background()
		now := time.Now().UnixMilli()

		_, err := sSilence.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-silence', 'macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
		`, now)
		if err != nil {
			t.Fatalf("inserting environment: %v", err)
		}

		// Run 1: stalled silence (updated_at = now - 130000, >= 120s)
		_, err = sSilence.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-silence-1', 'agent-silence-1', 1001, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run 1: %v", err)
		}

		silenceStalledUpdate := now - 130000
		_, err = sSilence.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'agent-silence-1', 'sess-silence-1', 'env-silence', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-silence-1',
				'BUSY', 'Working', 'Executing task', 0, 'editing', NULL, NULL,
				NULL,
				0, 0, 0, 0, 0, 0, 0, 0, ?, ?
			)
		`, now-300000, silenceStalledUpdate)
		if err != nil {
			t.Fatalf("inserting agent 1: %v", err)
		}

		// Run 2: healthy active (updated_at = now - 30000, < 120s)
		_, err = sSilence.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-silence-2', 'agent-silence-2', 1002, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run 2: %v", err)
		}

		activeUpdate := now - 30000
		_, err = sSilence.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'agent-silence-2', 'sess-silence-2', 'env-silence', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-silence-2',
				'BUSY', 'Working', 'Executing task', 0, 'editing', NULL, NULL,
				NULL,
				0, 0, 0, 0, 0, 0, 0, 0, ?, ?
			)
		`, now-300000, activeUpdate)
		if err != nil {
			t.Fatalf("inserting agent 2: %v", err)
		}

		status, err := sSilence.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if status.ActiveAgentCount != 2 {
			t.Errorf("ActiveAgentCount = %d, want 2", status.ActiveAgentCount)
		}
		// Agent 1 has silence >= 120s (stalled) so attention count = 1
		if status.RequiresAttentionCount != 1 {
			t.Errorf("RequiresAttentionCount = %d, want 1", status.RequiresAttentionCount)
		}
	})

	t.Run("rate limited attention respects rate_limit_reset_at", func(t *testing.T) {
		sRL := setupTestDB(t)
		ctx := context.Background()
		now := time.Now().UnixMilli()

		_, err := sRL.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-rl-att', 'macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
		`, now)
		if err != nil {
			t.Fatalf("inserting environment: %v", err)
		}

		_, err = sRL.db.ExecContext(ctx, `
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES ('run-rl-att', 'agent-rl-att', 1001, '1.0', ?, NULL)
		`, now-300000)
		if err != nil {
			t.Fatalf("inserting run: %v", err)
		}

		stalledUpdate := now - 130000
		futureResetAt := now + 60000
		_, err = sRL.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, worktree_rel_path, cwd, current_run_id,
				activity, title, action, action_stale, phase, active_tool, active_target,
				active_tool_started_at,
				tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at,
				rate_limit_reset_at, retry_after_seconds
			) VALUES (
				'agent-rl-att', 'sess-rl-att', 'env-rl-att', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, NULL, '/repo', 'run-rl-att',
				'RATE_LIMITED', 'Throttled', 'Waiting for quota', 0, 'editing', NULL, NULL,
				NULL,
				0, 0, 0, 0, 0, 0, 0, 0, ?, ?,
				?, 60
			)
		`, now-300000, stalledUpdate, futureResetAt)
		if err != nil {
			t.Fatalf("inserting agent: %v", err)
		}

		// 1. Future reset_at: attention suppressed (count = 0) even though retry_after_seconds (60) has elapsed (130s >= 60s)
		status, err := sRL.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.ActiveAgentCount != 1 {
			t.Errorf("ActiveAgentCount = %d, want 1", status.ActiveAgentCount)
		}
		if status.RequiresAttentionCount != 0 {
			t.Errorf("RequiresAttentionCount = %d, want 0 when resetAt in future", status.RequiresAttentionCount)
		}

		// 2. Past reset_at: attention triggers (count = 1) even though retry_after_seconds (3600) has NOT elapsed
		pastResetAt := now - 10000
		_, err = sRL.db.ExecContext(ctx, `UPDATE agents SET rate_limit_reset_at = ?, retry_after_seconds = 3600 WHERE id = 'agent-rl-att'`, pastResetAt)
		if err != nil {
			t.Fatalf("updating agent resetAt: %v", err)
		}
		status, err = sRL.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.RequiresAttentionCount != 1 {
			t.Errorf("RequiresAttentionCount = %d, want 1 when resetAt in past (precedence over unelapsed retry_after_seconds)", status.RequiresAttentionCount)
		}

		// 3. NULL reset_at with unelapsed retry_after_seconds (3600): attention suppressed (count = 0)
		_, err = sRL.db.ExecContext(ctx, `UPDATE agents SET rate_limit_reset_at = NULL, retry_after_seconds = 3600 WHERE id = 'agent-rl-att'`)
		if err != nil {
			t.Fatalf("updating agent resetAt to NULL: %v", err)
		}
		status, err = sRL.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.RequiresAttentionCount != 0 {
			t.Errorf("RequiresAttentionCount = %d, want 0 when resetAt is NULL and retry_after_seconds not elapsed", status.RequiresAttentionCount)
		}

		// 4. NULL reset_at with elapsed retry_after_seconds (60): attention triggers (count = 1)
		_, err = sRL.db.ExecContext(ctx, `UPDATE agents SET rate_limit_reset_at = NULL, retry_after_seconds = 60 WHERE id = 'agent-rl-att'`)
		if err != nil {
			t.Fatalf("updating agent retry_after_seconds: %v", err)
		}
		status, err = sRL.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.RequiresAttentionCount != 1 {
			t.Errorf("RequiresAttentionCount = %d, want 1 when resetAt is NULL and retry_after_seconds elapsed", status.RequiresAttentionCount)
		}

		// 5. NULL reset_at with NULL retry_after_seconds and updated_at >= 120s: attention triggers (count = 1)
		_, err = sRL.db.ExecContext(ctx, `UPDATE agents SET rate_limit_reset_at = NULL, retry_after_seconds = NULL WHERE id = 'agent-rl-att'`)
		if err != nil {
			t.Fatalf("updating agent retry_after_seconds to NULL: %v", err)
		}
		status, err = sRL.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.RequiresAttentionCount != 1 {
			t.Errorf("RequiresAttentionCount = %d, want 1 when resetAt is NULL and retry_after_seconds is NULL", status.RequiresAttentionCount)
		}
	})
}

func TestStore_GetEnvironments(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	t.Run("success", func(t *testing.T) {
		envs, err := s.GetEnvironments(context.Background())
		if err != nil {
			t.Fatalf("GetEnvironments() error = %v", err)
		}
		if len(envs) != 2 {
			t.Fatalf("len(envs) = %d, want 2", len(envs))
		}
		// Ordered by label ASC
		// env2 ('stale-linux') comes before env1 ('work-macbook')
		if envs[0].ID != "env2" || !envs[0].IsStale {
			t.Errorf("expected envs[0] to be stale env2, got: %+v", envs[0])
		}
		if envs[1].ID != "env1" || envs[1].IsStale {
			t.Errorf("expected envs[1] to be active env1, got: %+v", envs[1])
		}
	})

	t.Run("db error", func(t *testing.T) {
		sErr := setupTestDB(t)
		_ = sErr.Close()
		_, err := sErr.GetEnvironments(context.Background())
		if err == nil {
			t.Fatal("expected error on closed db, got nil")
		}
	})

	t.Run("non-positive heartbeat is stale even when nowMs is small", func(t *testing.T) {
		sZero := setupTestDB(t)
		ctx := context.Background()
		_, err := sZero.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-zero', 'zero-env', 'zero.local', 'darwin', 'alex', 0, 0)
		`)
		if err != nil {
			t.Fatalf("inserting zero environment: %v", err)
		}
		envs, err := sZero.GetEnvironmentsAt(ctx, 10000)
		if err != nil {
			t.Fatalf("GetEnvironmentsAt() error = %v", err)
		}
		if len(envs) != 1 {
			t.Fatalf("len(envs) = %d, want 1", len(envs))
		}
		if !envs[0].IsStale {
			t.Errorf("expected env-zero to be stale, got IsStale = false")
		}
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func TestStore_GetAgents(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	tests := []struct {
		name      string
		filter    AgentFilter
		wantCount int
	}{
		{"all", AgentFilter{}, 3},
		{"by project repo1", AgentFilter{Project: "github.com/org/repo1"}, 2},
		{"by project repo2", AgentFilter{Project: "github.com/org/repo2"}, 1},
		{"by non-existent project", AgentFilter{Project: "non-existent"}, 0},
		{"by env1", AgentFilter{Environment: "env1"}, 2},
		{"by env2", AgentFilter{Environment: "env2"}, 1},
		{"by non-existent env", AgentFilter{Environment: "env-nonexistent"}, 0},
		{"running only", AgentFilter{Lifecycle: types.LifecycleRunning}, 2},
		{"exited only", AgentFilter{Lifecycle: types.LifecycleExited}, 1},
		{"activity waiting question", AgentFilter{Activity: types.ActivityWaitingQuestion}, 1},
		{"activity error", AgentFilter{Activity: types.ActivityError}, 1},
		{"activity idle", AgentFilter{Activity: types.ActivityIdle}, 1},
		{"combined project and running", AgentFilter{Project: "github.com/org/repo1", Lifecycle: types.LifecycleRunning}, 1},
		{"combined project and exited", AgentFilter{Project: "github.com/org/repo1", Lifecycle: types.LifecycleExited}, 1},
		{"combined env and activity", AgentFilter{Environment: "env1", Activity: types.ActivityWaitingQuestion}, 1},
		{"hidden only", AgentFilter{Hidden: boolPtr(true)}, 1},
		{"visible only", AgentFilter{Hidden: boolPtr(false)}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agents, err := s.GetAgents(context.Background(), tt.filter)
			if err != nil {
				t.Fatalf("GetAgents() error = %v", err)
			}
			if len(agents) != tt.wantCount {
				t.Fatalf("len(agents) = %d, want %d", len(agents), tt.wantCount)
			}
		})
	}

	t.Run("db error", func(t *testing.T) {
		sErr := setupTestDB(t)
		_ = sErr.Close()
		_, err := sErr.GetAgents(context.Background(), AgentFilter{})
		if err == nil {
			t.Fatal("expected error on closed db, got nil")
		}
	})
}

func TestStore_GetAgentByID(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	t.Run("found running agent with all fields", func(t *testing.T) {
		agent, err := s.GetAgentByID(context.Background(), "env1:claude-code:sess1")
		if err != nil {
			t.Fatalf("GetAgentByID() error = %v", err)
		}
		if agent.ID != "env1:claude-code:sess1" {
			t.Errorf("agent.ID = %q, want env1:claude-code:sess1", agent.ID)
		}
		if agent.Activity.Lifecycle != types.LifecycleRunning {
			t.Errorf("agent lifecycle = %q, want RUNNING", agent.Activity.Lifecycle)
		}
		if agent.Activity.CurrentRun == nil || agent.Activity.CurrentRun.PID != 1234 {
			t.Errorf("agent current run PID = %+v, want 1234", agent.Activity.CurrentRun)
		}
		if agent.Activity.CurrentRun.EndedAt != nil {
			t.Errorf("agent current run endedAt = %v, want nil", agent.Activity.CurrentRun.EndedAt)
		}
		if agent.Activity.Question == nil || *agent.Activity.Question != "Should I proceed?" {
			t.Errorf("agent question = %v, want 'Should I proceed?'", agent.Activity.Question)
		}
		if agent.Activity.Phase == nil || *agent.Activity.Phase != types.PhaseEditing {
			t.Errorf("agent phase = %v, want editing", agent.Activity.Phase)
		}
		if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != "AskUserQuestion" {
			t.Errorf("agent active tool = %v, want AskUserQuestion", agent.Activity.ActiveTool)
		}
		if agent.Activity.ActiveTarget == nil || *agent.Activity.ActiveTarget != "target1" {
			t.Errorf("agent active target = %v, want target1", agent.Activity.ActiveTarget)
		}
		if agent.Activity.LastThinkingExcerpt == nil || *agent.Activity.LastThinkingExcerpt != "thinking about options" {
			t.Errorf("agent thinking excerpt = %v, want 'thinking about options'", agent.Activity.LastThinkingExcerpt)
		}
		if agent.Activity.Cost == nil || agent.Activity.Cost.Total != 1.25 {
			t.Errorf("agent cost = %v, want 1.25", agent.Activity.Cost)
		}
		if agent.IsHidden {
			t.Errorf("expected agent.IsHidden == false")
		}
		if agent.Activity.Model == nil || *agent.Activity.Model != "claude-3-7-sonnet" {
			t.Errorf("agent model = %v, want claude-3-7-sonnet", agent.Activity.Model)
		}
		if agent.Activity.ContextWindow == nil || *agent.Activity.ContextWindow != 200000 {
			t.Errorf("agent context window = %v, want 200000", agent.Activity.ContextWindow)
		}
		if agent.Activity.ContextFullness == nil || *agent.Activity.ContextFullness != 0.25 {
			t.Errorf("agent context fullness = %v, want 0.25", agent.Activity.ContextFullness)
		}
	})

	t.Run("found exited agent with worktree and error", func(t *testing.T) {
		agent, err := s.GetAgentByID(context.Background(), "env1:claude-code:sess2")
		if err != nil {
			t.Fatalf("GetAgentByID() error = %v", err)
		}
		if !agent.IsHidden {
			t.Errorf("expected agent.IsHidden == true")
		}
		if agent.HiddenAt == nil {
			t.Errorf("expected agent.HiddenAt != nil")
		}
		if agent.Activity.Lifecycle != types.LifecycleExited {
			t.Errorf("agent lifecycle = %q, want EXITED", agent.Activity.Lifecycle)
		}
		if agent.Activity.CurrentRun != nil {
			t.Errorf("agent current run = %+v, want nil", agent.Activity.CurrentRun)
		}
		if !agent.Workspace.IsWorktree || agent.Workspace.RelPath == nil || *agent.Workspace.RelPath != ".workspaces/feat-x" {
			t.Errorf("unexpected workspace info: %+v", agent.Workspace)
		}
		if !agent.Workspace.IsDetached {
			t.Errorf("expected isDetached = true")
		}
		if agent.Activity.LastError == nil || *agent.Activity.LastError != "Build failed with syntax error" {
			t.Errorf("agent last error = %v, want syntax error", agent.Activity.LastError)
		}
		if !agent.Activity.ActionStale {
			t.Errorf("expected actionStale = true")
		}
	})

	t.Run("found agent with endedAt in run and nil cost", func(t *testing.T) {
		agent, err := s.GetAgentByID(context.Background(), "env2:pi:sess3")
		if err != nil {
			t.Fatalf("GetAgentByID() error = %v", err)
		}
		if agent.Activity.CurrentRun == nil || agent.Activity.CurrentRun.EndedAt == nil {
			t.Errorf("expected current run with endedAt, got: %+v", agent.Activity.CurrentRun)
		}
		if agent.Activity.Cost != nil {
			t.Errorf("expected cost to be nil, got: %+v", agent.Activity.Cost)
		}
		if agent.Activity.Phase != nil {
			t.Errorf("expected phase to be nil, got: %v", agent.Activity.Phase)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := s.GetAgentByID(context.Background(), "does-not-exist")
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		sErr := setupTestDB(t)
		_ = sErr.Close()
		_, err := sErr.GetAgentByID(context.Background(), "env1:claude-code:sess1")
		if err == nil {
			t.Fatal("expected error on closed db, got nil")
		}
	})

	t.Run("non-positive environment heartbeat marks agent environment as stale", func(t *testing.T) {
		sZero := setupTestDB(t)
		ctx := context.Background()
		_, err := sZero.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-zero', 'zero-env', 'zero.local', 'darwin', 'alex', 0, 0)
		`)
		if err != nil {
			t.Fatalf("inserting zero environment: %v", err)
		}
		_, err = sZero.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, cwd, activity, title, action, action_stale,
				phase, tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'env-zero:claude-code:sess-zero', 'sess-zero', 'env-zero', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, '/repo', 'IDLE', 'Idle', 'Waiting', 0,
				'editing', 0, 0, 0, 0, 0, 0, 0, 0, 1000, 1000
			)
		`)
		if err != nil {
			t.Fatalf("inserting agent: %v", err)
		}
		row := sZero.db.QueryRowContext(ctx, selectAgentBaseQuery+" WHERE a.id = ?", "env-zero:claude-code:sess-zero")
		agent, err := scanAgent(row, 10000)
		if err != nil {
			t.Fatalf("scanAgent error: %v", err)
		}
		if !agent.Environment.IsStale {
			t.Errorf("expected agent.Environment.IsStale = true for 0 heartbeat with small nowMs")
		}
	})
}

func TestStore_GetProjects(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	t.Run("success with multiple projects", func(t *testing.T) {
		projects, err := s.GetProjects(context.Background())
		if err != nil {
			t.Fatalf("GetProjects() error = %v", err)
		}

		if len(projects) != 2 {
			t.Fatalf("len(projects) = %d, want 2", len(projects))
		}

		// First project should be repo1
		p1 := projects[0]
		if p1.Project.ID != "github.com/org/repo1" {
			t.Errorf("project 1 ID = %q, want github.com/org/repo1", p1.Project.ID)
		}
		if p1.ActiveAgentCount != 1 {
			t.Errorf("p1 ActiveAgentCount = %d, want 1", p1.ActiveAgentCount)
		}
		if p1.TotalCost != 3.75 {
			t.Errorf("p1 TotalCost = %f, want 3.75", p1.TotalCost)
		}
		if len(p1.Agents) != 2 {
			t.Errorf("len(p1.Agents) = %d, want 2", len(p1.Agents))
		}

		// Second project should be repo2
		p2 := projects[1]
		if p2.Project.ID != "github.com/org/repo2" {
			t.Errorf("project 2 ID = %q, want github.com/org/repo2", p2.Project.ID)
		}
		if p2.ActiveAgentCount != 1 {
			t.Errorf("p2 ActiveAgentCount = %d, want 1", p2.ActiveAgentCount)
		}
		if p2.TotalCost != 0.0 {
			t.Errorf("p2 TotalCost = %f, want 0.0", p2.TotalCost)
		}
	})

	t.Run("db error", func(t *testing.T) {
		sErr := setupTestDB(t)
		_ = sErr.Close()
		_, err := sErr.GetProjects(context.Background())
		if err == nil {
			t.Fatal("expected error on closed db, got nil")
		}
	})
}

func TestStore_GetAgentLogs(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	t.Run("all logs for agent", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs() error = %v", err)
		}
		if len(logs) != 4 {
			t.Fatalf("len(logs) = %d, want 4", len(logs))
		}
		// Ordered DESC by id (4, 3, 2, 1)
		if logs[0].Seq != 4 {
			t.Errorf("logs[0].Seq = %d, want 4", logs[0].Seq)
		}
		if logs[0].SubagentID == nil || *logs[0].SubagentID != "sub2" {
			t.Errorf("logs[0].SubagentID = %v, want sub2", logs[0].SubagentID)
		}
	})

	t.Run("filter by subagent", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 0, "sub1")
		if err != nil {
			t.Fatalf("GetAgentLogs() error = %v", err)
		}
		if len(logs) != 2 {
			t.Fatalf("len(logs) = %d, want 2", len(logs))
		}
		if logs[0].Seq != 3 || logs[1].Seq != 2 {
			t.Errorf("unexpected logs seqs: %d, %d", logs[0].Seq, logs[1].Seq)
		}
	})

	t.Run("pagination with beforeID", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 3, "")
		if err != nil {
			t.Fatalf("GetAgentLogs() error = %v", err)
		}
		if len(logs) != 2 {
			t.Fatalf("len(logs) = %d, want 2", len(logs))
		}
		if logs[0].Seq != 2 || logs[1].Seq != 1 {
			t.Errorf("logs seqs = %d, %d, want 2, 1", logs[0].Seq, logs[1].Seq)
		}
	})

	t.Run("filter with both beforeID and subagent", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 3, "sub1")
		if err != nil {
			t.Fatalf("GetAgentLogs() error = %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("len(logs) = %d, want 1", len(logs))
		}
		if logs[0].Seq != 2 {
			t.Errorf("logs[0].Seq = %d, want 2", logs[0].Seq)
		}
	})

	t.Run("limit clamping", func(t *testing.T) {
		// limit <= 0 clamped to 100
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", -5, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs(limit=-5) error = %v", err)
		}
		if len(logs) != 4 {
			t.Errorf("expected 4 logs, got %d", len(logs))
		}

		logsZero, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 0, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs(limit=0) error = %v", err)
		}
		if len(logsZero) != 4 {
			t.Errorf("expected 4 logs, got %d", len(logsZero))
		}

		// limit > 1000 clamped to 1000
		logsMax, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 5000, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs(limit=5000) error = %v", err)
		}
		if len(logsMax) != 4 {
			t.Errorf("expected 4 logs, got %d", len(logsMax))
		}
	})

	t.Run("db error", func(t *testing.T) {
		sErr := setupTestDB(t)
		_ = sErr.Close()
		_, err := sErr.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 0, "")
		if err == nil {
			t.Fatal("expected error on closed db, got nil")
		}
	})

	t.Run("non-positive environment heartbeat marks frame environment as stale", func(t *testing.T) {
		sZero := setupTestDB(t)
		ctx := context.Background()
		_, err := sZero.db.ExecContext(ctx, `
			INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
			VALUES ('env-zero', 'zero-env', 'zero.local', 'darwin', 'alex', 0, 0)
		`)
		if err != nil {
			t.Fatalf("inserting zero environment: %v", err)
		}
		_, err = sZero.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, session_id, environment_id, harness, project_id, project_name, canonical_root,
				branch, is_detached, is_worktree, cwd, activity, title, action, action_stale,
				phase, tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
				tokens_reasoning, tokens_total, cost_usd, subagent_count, first_started_at, updated_at
			) VALUES (
				'env-zero:claude-code:sess-zero', 'sess-zero', 'env-zero', 'claude-code', 'proj', 'proj', '/repo',
				'main', 0, 0, '/repo', 'IDLE', 'Idle', 'Waiting', 0,
				'editing', 0, 0, 0, 0, 0, 0, 0, 0, 1000, 1000
			)
		`)
		if err != nil {
			t.Fatalf("inserting agent: %v", err)
		}
		_, err = sZero.db.ExecContext(ctx, `
			INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
			VALUES (1, 'env-zero:claude-code:sess-zero', NULL, 'turn_start', 1000, '{}')
		`)
		if err != nil {
			t.Fatalf("inserting telemetry event: %v", err)
		}
		frames, err := sZero.GetAgentLogs(ctx, "env-zero:claude-code:sess-zero", 10, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs error: %v", err)
		}
		if len(frames) != 1 {
			t.Fatalf("len(frames) = %d, want 1", len(frames))
		}
		if !frames[0].Environment.IsStale {
			t.Errorf("expected frames[0].Environment.IsStale = true for 0 heartbeat")
		}
	})
}

func TestStore_GetAgents_ActiveSubagents_RoundTrip(t *testing.T) {
	st := setupTestDB(t)
	seedTestData(t, st)
	tool := "bash"
	sub := types.ISubagentSummary{
		ID:         "sub-test-1",
		Activity:   types.ActivityBusy,
		ActiveTool: &tool,
		Action:     "Running bash",
	}
	subJSON, _ := json.Marshal([]types.ISubagentSummary{sub})

	// Update agent row in db with active_subagents column
	_, err := st.db.Exec(`UPDATE agents SET active_subagents = ? WHERE id = 'env1:claude-code:sess1'`, string(subJSON))
	if err != nil {
		t.Fatalf("updating active_subagents: %v", err)
	}

	agent, err := st.GetAgentByID(context.Background(), "env1:claude-code:sess1")
	if err != nil {
		t.Fatalf("GetAgentByID failed: %v", err)
	}
	if len(agent.Activity.ActiveSubagents) != 1 {
		t.Fatalf("expected 1 ActiveSubagent, got %d", len(agent.Activity.ActiveSubagents))
	}
	if agent.Activity.ActiveSubagents[0].ID != "sub-test-1" {
		t.Errorf("expected subagent ID 'sub-test-1', got %q", agent.Activity.ActiveSubagents[0].ID)
	}
}

func TestStore_RateLimitedFilterAndStatus(t *testing.T) {
	st := setupTestDB(t)
	now := time.Now().UnixMilli()

	// Seed environment and run
	_, err := st.db.Exec(`
		INSERT INTO environments (id, label, hostname, platform, user, last_heartbeat_at)
		VALUES ('env-rl', 'dev', 'host', 'linux', 'user', ?);
	`, now)
	if err != nil {
		t.Fatalf("seeding env: %v", err)
	}

	_, err = st.db.Exec(`
		INSERT INTO runs (id, agent_id, pid, harness_version, started_at)
		VALUES ('run-rl-1', 'env-rl:claude-code:sess-fresh', 1234, '1.0.0', ?),
		       ('run-rl-2', 'env-rl:claude-code:sess-stalled', 5678, '1.0.0', ?),
		       ('run-rl-3', 'env-rl:claude-code:sess-retry-suppressed', 9012, '1.0.0', ?);
	`, now, now-200000, now-200000)
	if err != nil {
		t.Fatalf("seeding runs: %v", err)
	}

	// Insert fresh rate-limited agent (updated 10s ago) with rate_limit metadata
	resetAtVal := now + 45000
	retrySecVal := int64(45)
	_, err = st.db.Exec(`
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, last_error,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_reasoning, tokens_total, cost_usd, subagent_count,
			first_started_at, updated_at,
			rate_limit_reset_at, retry_after_seconds
		) VALUES (
			'env-rl:claude-code:sess-fresh', 'sess-fresh', 'env-rl', 'claude-code',
			'github.com/org/repo-rl', 'repo-rl', '/path/to/repo-rl',
			'main', 0, 0, NULL, '/path/to/repo-rl',
			'run-rl-1', 'RATE_LIMITED', 'Rate limited fresh', 'Backing off', 0,
			'planning', 'rate_limit_exceeded',
			100, 50, 0, 0,
			0, 150, 0.25, 0,
			?, ?,
			?, ?
		);
	`, now-10000, now-10000, resetAtVal, retrySecVal)
	if err != nil {
		t.Fatalf("seeding fresh rate-limited agent: %v", err)
	}

	// Insert stalled rate-limited agent (updated 150s ago)
	_, err = st.db.Exec(`
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, last_error,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_reasoning, tokens_total, cost_usd, subagent_count,
			first_started_at, updated_at
		) VALUES (
			'env-rl:claude-code:sess-stalled', 'sess-stalled', 'env-rl', 'claude-code',
			'github.com/org/repo-rl', 'repo-rl', '/path/to/repo-rl',
			'main', 0, 0, NULL, '/path/to/repo-rl',
			'run-rl-2', 'RATE_LIMITED', 'Rate limited stalled', 'Backing off', 0,
			'planning', 'rate_limit_exceeded',
			100, 50, 0, 0,
			0, 150, 0.25, 0,
			?, ?
		);
	`, now-200000, now-150000)
	if err != nil {
		t.Fatalf("seeding stalled rate-limited agent: %v", err)
	}

	// Insert rate-limited agent with active retry_after_seconds (updated 150s ago, retry_after 300s)
	// Even though updated_at > 120s ago, retry_after_seconds has not elapsed yet (150s < 300s),
	// so attention must be suppressed!
	retry300 := int64(300)
	_, err = st.db.Exec(`
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, last_error,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_reasoning, tokens_total, cost_usd, subagent_count,
			first_started_at, updated_at,
			rate_limit_reset_at, retry_after_seconds
		) VALUES (
			'env-rl:claude-code:sess-retry-suppressed', 'sess-retry-suppressed', 'env-rl', 'claude-code',
			'github.com/org/repo-rl', 'repo-rl', '/path/to/repo-rl',
			'main', 0, 0, NULL, '/path/to/repo-rl',
			'run-rl-3', 'RATE_LIMITED', 'Rate limited retry suppressed', 'Backing off', 0,
			'planning', 'rate_limit_exceeded',
			100, 50, 0, 0,
			0, 150, 0.25, 0,
			?, ?,
			NULL, ?
		);
	`, now-200000, now-150000, retry300)
	if err != nil {
		t.Fatalf("seeding retry-suppressed rate-limited agent: %v", err)
	}

	ctx := context.Background()

	// Test filter by ActivityRateLimited
	agents, err := st.GetAgents(ctx, AgentFilter{Activity: types.ActivityRateLimited})
	if err != nil {
		t.Fatalf("GetAgents with ActivityRateLimited failed: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("expected 3 rate-limited agents, got %d", len(agents))
	}
	for _, a := range agents {
		if a.Activity.Activity != types.ActivityRateLimited {
			t.Errorf("expected activity %s, got %s", types.ActivityRateLimited, a.Activity.Activity)
		}
	}

	// Test GetStatus: all 3 agents are active (current_run_id IS NOT NULL),
	// but only the stalled one with no future reset/retry (run-rl-2) requires attention!
	// run-rl-1 has future resetAt, and run-rl-3 has future retryAfterSeconds.
	status, err := st.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.ActiveAgentCount != 3 {
		t.Errorf("expected ActiveAgentCount = 3, got %d", status.ActiveAgentCount)
	}
	if status.RequiresAttentionCount != 1 {
		t.Errorf("expected RequiresAttentionCount = 1 (only stalled with no future retry), got %d", status.RequiresAttentionCount)
	}

	agentFresh, err := st.GetAgentByID(ctx, "env-rl:claude-code:sess-fresh")
	if err != nil {
		t.Fatalf("GetAgentByID failed: %v", err)
	}
	if agentFresh.Activity.RateLimitResetAt == nil || *agentFresh.Activity.RateLimitResetAt != resetAtVal {
		t.Errorf("expected RateLimitResetAt %d, got %v", resetAtVal, agentFresh.Activity.RateLimitResetAt)
	}
	if agentFresh.Activity.RetryAfterSeconds == nil || *agentFresh.Activity.RetryAfterSeconds != retrySecVal {
		t.Errorf("expected RetryAfterSeconds %d, got %v", retrySecVal, agentFresh.Activity.RetryAfterSeconds)
	}
}

type attentionFixture struct {
	Name                       string  `json:"name"`
	Lifecycle                  string  `json:"lifecycle"`
	Activity                   string  `json:"activity"`
	UpdatedAt                  int64   `json:"updatedAt"`
	ActiveTool                 *string `json:"activeTool"`
	ActiveToolStartedAt        *int64  `json:"activeToolStartedAt"`
	RateLimitResetAt           *int64  `json:"rateLimitResetAt"`
	RetryAfterSeconds          *int64  `json:"retryAfterSeconds"`
	EnvironmentLastHeartbeatAt int64   `json:"environmentLastHeartbeatAt"`
	EnvironmentIsStale         bool    `json:"environmentIsStale"`
	NowMs                      int64   `json:"nowMs"`
	ExpectedRequiresAttention  bool    `json:"expectedRequiresAttention"`
}

func TestStore_GetStatus_SharedAttentionFixtures(t *testing.T) {
	fixtureData, err := os.ReadFile("../../../agent-common/testdata/attention-fixtures.json")
	if err != nil {
		t.Fatalf("reading shared attention fixtures: %v", err)
	}

	var fixtures []attentionFixture
	if err := json.Unmarshal(fixtureData, &fixtures); err != nil {
		t.Fatalf("unmarshaling attention fixtures: %v", err)
	}

	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			st := setupTestDB(t)
			ctx := context.Background()

			// Seed environment
			_, err := st.db.ExecContext(ctx, `
				INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
				VALUES ('env-test', 'test-host', 'test.local', 'linux', 'tester', 0, ?);
			`, f.EnvironmentLastHeartbeatAt)
			if err != nil {
				t.Fatalf("inserting environment: %v", err)
			}

			// If lifecycle is RUNNING, seed run
			var currentRunID sql.NullString
			if f.Lifecycle == string(types.LifecycleRunning) {
				currentRunID = sql.NullString{String: "run-test", Valid: true}
				_, err = st.db.ExecContext(ctx, `
					INSERT INTO runs (id, agent_id, pid, harness_version, started_at)
					VALUES ('run-test', 'env-test:claude-code:sess-1', 1234, '1.0.0', ?);
				`, f.NowMs-300000)
				if err != nil {
					t.Fatalf("inserting run: %v", err)
				}
			}

			// Seed agent
			_, err = st.db.ExecContext(ctx, `
				INSERT INTO agents (
					id, session_id, environment_id, harness,
					project_id, project_name, canonical_root,
					branch, is_detached, is_worktree, worktree_rel_path, cwd,
					current_run_id, activity, title, action, action_stale,
					phase, active_tool, active_tool_started_at,
					tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
					tokens_reasoning, tokens_total, cost_usd, subagent_count,
					first_started_at, updated_at,
					rate_limit_reset_at, retry_after_seconds
				) VALUES (
					'env-test:claude-code:sess-1', 'sess-1', 'env-test', 'claude-code',
					'github.com/org/repo', 'repo', '/path/to/repo',
					'main', 0, 0, NULL, '/path/to/repo',
					?, ?, 'Title', 'Action', 0,
					'editing', ?, ?,
					100, 50, 0, 0,
					0, 150, 0.25, 0,
					?, ?,
					?, ?
				);
			`, currentRunID, f.Activity, f.ActiveTool, f.ActiveToolStartedAt,
				f.NowMs-300000, f.UpdatedAt, f.RateLimitResetAt, f.RetryAfterSeconds)
			if err != nil {
				t.Fatalf("inserting agent: %v", err)
			}

			status, err := st.GetStatusAt(ctx, f.NowMs)
			if err != nil {
				t.Fatalf("GetStatusAt failed: %v", err)
			}

			wantAttention := 0
			if f.ExpectedRequiresAttention {
				wantAttention = 1
			}
			if status.RequiresAttentionCount != wantAttention {
				t.Errorf("RequiresAttentionCount = %d, want %d", status.RequiresAttentionCount, wantAttention)
			}

			wantActive := 0
			if f.Lifecycle == string(types.LifecycleRunning) {
				wantActive = 1
			}
			if status.ActiveAgentCount != wantActive {
				t.Errorf("ActiveAgentCount = %d, want %d", status.ActiveAgentCount, wantActive)
			}
		})
	}
}
