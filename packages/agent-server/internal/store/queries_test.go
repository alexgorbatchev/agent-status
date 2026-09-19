package store

import (
	"context"
	"database/sql"
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
    harness               TEXT NOT NULL CHECK (harness IN ('claude-code','pi')),
    project_id            TEXT NOT NULL,
    project_name          TEXT NOT NULL,
    canonical_root        TEXT NOT NULL,
    branch                TEXT NOT NULL,
    is_detached           INTEGER NOT NULL DEFAULT 0,
    is_worktree           INTEGER NOT NULL DEFAULT 0,
    worktree_rel_path     TEXT,
    cwd                   TEXT NOT NULL,
    current_run_id        TEXT REFERENCES runs(id) ON DELETE SET NULL,
    activity              TEXT NOT NULL CHECK (activity IN ('BUSY','WAITING_QUESTION','ERROR','IDLE')),
    title                 TEXT NOT NULL DEFAULT '',
    action                TEXT NOT NULL DEFAULT '',
    action_stale          INTEGER NOT NULL DEFAULT 0,
    phase                 TEXT CHECK (phase IS NULL OR phase IN ('planning','editing','testing','debugging','investigating')),
    active_tool           TEXT,
    active_target         TEXT,
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
    first_started_at      INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL,
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
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("applying schema: %v", err)
	}

	return New(db)
}

func seedTestData(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// 1. Insert environment
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
		VALUES ('env1', 'work-macbook', 'macbook.local', 'darwin', 'alex', 0, ?)
	`, now)
	if err != nil {
		t.Fatalf("inserting environment: %v", err)
	}

	// 2. Insert run for agent 1
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
		VALUES ('run1', 'env1:claude-code:sess1', 1234, '2.1.277', ?, NULL)
	`, now-5000)
	if err != nil {
		t.Fatalf("inserting run: %v", err)
	}

	// 3. Insert agent 1 (running, waiting question)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, active_tool, active_target, question,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
			tokens_reasoning, tokens_total, cost_usd, subagent_count,
			first_started_at, updated_at
		) VALUES (
			'env1:claude-code:sess1', 'sess1', 'env1', 'claude-code',
			'github.com/org/repo1', 'repo1', '/path/to/repo1',
			'main', 0, 0, NULL, '/path/to/repo1',
			'run1', 'WAITING_QUESTION', 'Build Feature', 'Asking user a question', 0,
			'editing', 'AskUserQuestion', NULL, 'Should I proceed?',
			100, 50, 20, 10,
			0, 180, 1.25, 2,
			?, ?
		)
	`, now-10000, now)
	if err != nil {
		t.Fatalf("inserting agent 1: %v", err)
	}

	// 4. Insert agent 2 (exited, error)
	_, err = s.db.ExecContext(ctx, `
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
			'env1:claude-code:sess2', 'sess2', 'env1', 'claude-code',
			'github.com/org/repo1', 'repo1', '/path/to/repo1',
			'feat-x', 0, 1, '.workspaces/feat-x', '/path/to/repo1/.workspaces/feat-x',
			NULL, 'ERROR', 'Fix Bug', 'Compilation error', 0,
			'debugging', 'Build failed with syntax error',
			200, 100, 0, 0,
			0, 300, 2.50, 0,
			?, ?
		)
	`, now-20000, now-1000)
	if err != nil {
		t.Fatalf("inserting agent 2: %v", err)
	}

	// 5. Insert telemetry events
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (1, 'env1:claude-code:sess1', NULL, 'turn_start', ?, '{"role":"user","content":"start"}'),
		       (2, 'env1:claude-code:sess1', 'sub1', 'tool_call', ?, '{"toolName":"Bash","toolInput":{"command":"ls"}}')
	`, now-5000, now-2000)
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
}

func TestStore_GetStatus(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	status, err := s.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if status.ActiveAgentCount != 1 {
		t.Errorf("ActiveAgentCount = %d, want 1", status.ActiveAgentCount)
	}
	if status.RequiresAttentionCount != 1 {
		t.Errorf("RequiresAttentionCount = %d, want 1", status.RequiresAttentionCount)
	}
	if status.TotalFleetSpend != 3.75 {
		t.Errorf("TotalFleetSpend = %f, want 3.75", status.TotalFleetSpend)
	}
	if len(status.Environments) != 1 {
		t.Errorf("len(Environments) = %d, want 1", len(status.Environments))
	}
}

func TestStore_GetEnvironments(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	envs, err := s.GetEnvironments(context.Background())
	if err != nil {
		t.Fatalf("GetEnvironments() error = %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("len(envs) = %d, want 1", len(envs))
	}
	if envs[0].ID != "env1" || envs[0].Label != "work-macbook" || envs[0].IsStale {
		t.Errorf("unexpected env: %+v", envs[0])
	}
}

func TestStore_GetAgents(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	tests := []struct {
		name      string
		filter    AgentFilter
		wantCount int
	}{
		{"all", AgentFilter{}, 2},
		{"by project", AgentFilter{Project: "github.com/org/repo1"}, 2},
		{"by non-existent project", AgentFilter{Project: "non-existent"}, 0},
		{"by env", AgentFilter{Environment: "env1"}, 2},
		{"running only", AgentFilter{Lifecycle: types.LifecycleRunning}, 1},
		{"exited only", AgentFilter{Lifecycle: types.LifecycleExited}, 1},
		{"activity waiting question", AgentFilter{Activity: types.ActivityWaitingQuestion}, 1},
		{"activity error", AgentFilter{Activity: types.ActivityError}, 1},
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
}

func TestStore_GetAgentByID(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	t.Run("found running agent", func(t *testing.T) {
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
		if agent.Activity.Question == nil || *agent.Activity.Question != "Should I proceed?" {
			t.Errorf("agent question = %v, want 'Should I proceed?'", agent.Activity.Question)
		}
	})

	t.Run("found exited agent", func(t *testing.T) {
		agent, err := s.GetAgentByID(context.Background(), "env1:claude-code:sess2")
		if err != nil {
			t.Fatalf("GetAgentByID() error = %v", err)
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
	})

	t.Run("not found", func(t *testing.T) {
		_, err := s.GetAgentByID(context.Background(), "does-not-exist")
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestStore_GetProjects(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	projects, err := s.GetProjects(context.Background())
	if err != nil {
		t.Fatalf("GetProjects() error = %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}

	p := projects[0]
	if p.Project.ID != "github.com/org/repo1" {
		t.Errorf("project ID = %q, want github.com/org/repo1", p.Project.ID)
	}
	if p.ActiveAgentCount != 1 {
		t.Errorf("ActiveAgentCount = %d, want 1", p.ActiveAgentCount)
	}
	if p.TotalCost != 3.75 {
		t.Errorf("TotalCost = %f, want 3.75", p.TotalCost)
	}
	if len(p.Agents) != 2 {
		t.Errorf("len(Agents) = %d, want 2", len(p.Agents))
	}
}

func TestStore_GetAgentLogs(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)

	t.Run("all logs for agent", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs() error = %v", err)
		}
		if len(logs) != 2 {
			t.Fatalf("len(logs) = %d, want 2", len(logs))
		}
		if logs[0].Seq != 2 { // ordered DESC
			t.Errorf("logs[0].Seq = %d, want 2", logs[0].Seq)
		}
	})

	t.Run("filter by subagent", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 0, "sub1")
		if err != nil {
			t.Fatalf("GetAgentLogs() error = %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("len(logs) = %d, want 1", len(logs))
		}
		if logs[0].SubagentID == nil || *logs[0].SubagentID != "sub1" {
			t.Errorf("subagent ID = %v, want sub1", logs[0].SubagentID)
		}
	})

	t.Run("pagination with beforeID", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 10, 2, "")
		if err != nil {
			t.Fatalf("GetAgentLogs() error = %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("len(logs) = %d, want 1", len(logs))
		}
		if logs[0].Seq != 1 {
			t.Errorf("logs[0].Seq = %d, want 1", logs[0].Seq)
		}
	})

	t.Run("default and max limit bounds", func(t *testing.T) {
		logs, err := s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 0, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs(limit=0) error = %v", err)
		}
		if len(logs) != 2 {
			t.Errorf("expected 2 logs, got %d", len(logs))
		}

		logs, err = s.GetAgentLogs(context.Background(), "env1:claude-code:sess1", 2000, 0, "")
		if err != nil {
			t.Fatalf("GetAgentLogs(limit=2000) error = %v", err)
		}
		if len(logs) != 2 {
			t.Errorf("expected 2 logs, got %d", len(logs))
		}
	})
}
