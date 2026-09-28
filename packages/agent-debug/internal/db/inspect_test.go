package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func setupTestDB(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agent-status.db")

	db, err := sql.Open("sqlite3", "file:"+dbPath)
	if err != nil {
		t.Fatalf("opening test sqlite db: %v", err)
	}

	schema := `
	CREATE TABLE environments (
		id TEXT PRIMARY KEY,
		label TEXT NOT NULL,
		hostname TEXT NOT NULL,
		platform TEXT NOT NULL,
		user TEXT NOT NULL,
		dropped_frames INTEGER NOT NULL DEFAULT 0,
		last_heartbeat_at INTEGER NOT NULL
	);
	CREATE TABLE runs (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		pid INTEGER NOT NULL,
		harness_version TEXT NOT NULL,
		started_at INTEGER NOT NULL,
		ended_at INTEGER
	);
	CREATE TABLE agents (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		environment_id TEXT NOT NULL,
		harness TEXT NOT NULL,
		project_id TEXT NOT NULL,
		project_name TEXT NOT NULL,
		canonical_root TEXT NOT NULL,
		branch TEXT NOT NULL,
		is_detached INTEGER NOT NULL DEFAULT 0,
		is_worktree INTEGER NOT NULL DEFAULT 0,
		worktree_rel_path TEXT,
		cwd TEXT NOT NULL,
		current_run_id TEXT,
		activity TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL DEFAULT '',
		action_stale INTEGER NOT NULL DEFAULT 0,
		phase TEXT,
		active_tool TEXT,
		active_target TEXT,
		question TEXT,
		last_thinking_excerpt TEXT,
		last_error TEXT,
		tokens_input INTEGER NOT NULL DEFAULT 0,
		tokens_output INTEGER NOT NULL DEFAULT 0,
		tokens_cache_read INTEGER NOT NULL DEFAULT 0,
		tokens_cache_write INTEGER NOT NULL DEFAULT 0,
		tokens_reasoning INTEGER NOT NULL DEFAULT 0,
		tokens_total INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL,
		subagent_count INTEGER NOT NULL DEFAULT 0,
		rate_limit_reset_at INTEGER,
		retry_after_seconds INTEGER,
		has_unseen_completion INTEGER NOT NULL DEFAULT 0,
		completed_at INTEGER,
		first_started_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE TABLE telemetry_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id TEXT NOT NULL,
		subagent_id TEXT,
		event_type TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		data_json TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		t.Fatalf("creating test schema: %v", err)
	}

	now := time.Now().UnixMilli()
	t0 := now - 100000

	// Insert environment
	_, err = db.Exec(`
		INSERT INTO environments (id, label, hostname, platform, user, last_heartbeat_at)
		VALUES ('env-test', 'local-test', 'host-1', 'darwin', 'developer', ?)
	`, now)
	if err != nil {
		_ = db.Close()
		t.Fatalf("inserting test environment: %v", err)
	}

	// Insert agent 1 (running)
	agentID := "env-test:claude-code:ses-abc-123"
	sessionID := "ses-abc-123"
	runID := "run-1"
	_, err = db.Exec(`
		INSERT INTO agents (
			id, session_id, environment_id, harness, project_id, project_name,
			canonical_root, branch, cwd, current_run_id, activity, title,
			action, phase, active_tool, active_target, question, last_error,
			last_thinking_excerpt, tokens_input, tokens_output, tokens_total, cost_usd,
			first_started_at, updated_at
		) VALUES (
			?, ?, 'env-test', 'claude-code', 'proj-1', 'agent-status',
			'/repo', 'main', '/repo', ?, 'BUSY', 'Fix auth bug',
			'Running tests', 'planning', 'Bash', 'src/auth.go', 'Confirm edit?', 'no error',
			'thinking excerpt', 1000, 200, 1200, 0.05,
			?, ?
		)
	`, agentID, sessionID, runID, t0, now)
	if err != nil {
		_ = db.Close()
		t.Fatalf("inserting test agent: %v", err)
	}

	// Insert agent 2 (exited, null cost, null phase, ended run)
	agent2ID := "env-test:claude-code:ses-exited"
	_, err = db.Exec(`
		INSERT INTO agents (
			id, session_id, environment_id, harness, project_id, project_name,
			canonical_root, branch, cwd, current_run_id, activity, title,
			action, tokens_input, tokens_output, tokens_total, cost_usd,
			first_started_at, updated_at
		) VALUES (
			?, 'ses-exited', 'env-test', 'claude-code', 'proj-1', 'agent-status',
			'/repo', 'main', '/repo', NULL, 'IDLE', '',
			'', 0, 0, 0, NULL,
			?, ?
		)
	`, agent2ID, t0, now)
	if err != nil {
		_ = db.Close()
		t.Fatalf("inserting test agent 2: %v", err)
	}

	// Insert run
	_, err = db.Exec(`
		INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
		VALUES
			(?, ?, 12345, '1.0.0', ?, NULL),
			('run-ended', ?, 99999, '1.0.0', ?, ?)
	`, runID, agentID, t0, agent2ID, t0, now)
	if err != nil {
		_ = db.Close()
		t.Fatalf("inserting test runs: %v", err)
	}

	// Insert telemetry events
	_, err = db.Exec(`
		INSERT INTO telemetry_events (agent_id, event_type, timestamp, data_json)
		VALUES
			(?, 'run_started', ?, '{}'),
			(?, 'tool_call', ?, '{"tool":"bash"}'),
			(?, 'tool_result', ?, '{"output":"ok"}')
	`, agentID, t0, agentID, t0+1000, agentID, t0+2000)
	if err != nil {
		_ = db.Close()
		t.Fatalf("inserting test events: %v", err)
	}

	_ = db.Close()
	return dbPath, func() {
		_ = os.Remove(dbPath)
	}
}

func TestFindDefaultDBPath(t *testing.T) {
	temp := t.TempDir()

	// 1. XDG_DATA_HOME candidate
	xdgDir := filepath.Join(temp, "xdg", "agent-status")
	if err := os.MkdirAll(xdgDir, 0755); err != nil {
		t.Fatal(err)
	}
	xdgDB := filepath.Join(xdgDir, "agent-status.db")
	if err := os.WriteFile(xdgDB, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AIR_DB_PATH", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(temp, "xdg"))
	t.Setenv("HOME", temp)

	found := FindDefaultDBPath()
	if found != xdgDB {
		t.Errorf("expected %s, got %s", xdgDB, found)
	}

	// 2. HOME/.local/share candidate when XDG_DATA_HOME is empty
	t.Setenv("XDG_DATA_HOME", "")
	homeShare := filepath.Join(temp, ".local", "share", "agent-status")
	if err := os.MkdirAll(homeShare, 0755); err != nil {
		t.Fatal(err)
	}
	homeDB := filepath.Join(homeShare, "agent-status.db")
	if err := os.WriteFile(homeDB, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(xdgDB)

	foundHome := FindDefaultDBPath()
	if foundHome != homeDB {
		t.Errorf("expected %s, got %s", homeDB, foundHome)
	}

	// 3. HOME/.local/state candidate
	_ = os.Remove(homeDB)
	homeState := filepath.Join(temp, ".local", "state", "agent-status")
	if err := os.MkdirAll(homeState, 0755); err != nil {
		t.Fatal(err)
	}
	stateDB := filepath.Join(homeState, "agent-status.db")
	if err := os.WriteFile(stateDB, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	foundState := FindDefaultDBPath()
	if foundState != stateDB {
		t.Errorf("expected %s, got %s", stateDB, foundState)
	}
}

func TestResolveDBPath(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Explicit valid path
	resolved, err := ResolveDBPath(dbPath)
	if err != nil {
		t.Fatalf("expected resolved path, got error: %v", err)
	}
	if resolved != dbPath {
		t.Errorf("expected %s, got %s", dbPath, resolved)
	}

	// 2. Explicit invalid path
	_, err = ResolveDBPath("/non/existent/path/agent-status.db")
	if err == nil {
		t.Fatal("expected error for non-existent db path, got nil")
	}

	// 3. AIR_DB_PATH environment variable override
	t.Setenv("AIR_DB_PATH", dbPath)
	resolvedEnv, err := ResolveDBPath("auto")
	if err != nil {
		t.Fatalf("expected resolved from AIR_DB_PATH, got error: %v", err)
	}
	if resolvedEnv != dbPath {
		t.Errorf("expected %s, got %s", dbPath, resolvedEnv)
	}

	// 4. Auto fails if AIR_DB_PATH points nowhere and default locations empty
	t.Setenv("AIR_DB_PATH", "/non/existent/path/db.sqlite")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	_, err = ResolveDBPath("auto")
	if err == nil {
		t.Fatal("expected error when auto-detect cannot find db, got nil")
	}

	// 5. Tilde expansion ~/
	home := t.TempDir()
	t.Setenv("HOME", home)
	inHomePath := filepath.Join(home, "mydb.db")
	if err := os.WriteFile(inHomePath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	resolvedTilde, err := ResolveDBPath("~/mydb.db")
	if err != nil {
		t.Fatalf("expected tilde path to resolve, got error: %v", err)
	}
	if resolvedTilde != inHomePath {
		t.Errorf("expected %s, got %s", inHomePath, resolvedTilde)
	}
}

func TestInspectSession(t *testing.T) {
	ctx := context.Background()
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Inspect by session ID (running agent)
	rec, err := InspectSession(ctx, dbPath, "ses-abc-123")
	if err != nil {
		t.Fatalf("InspectSession failed: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil SessionRecord")
	}
	if rec.Agent == nil {
		t.Fatal("expected non-nil Agent in SessionRecord")
	}
	if rec.Agent.ID != "env-test:claude-code:ses-abc-123" {
		t.Errorf("expected Agent ID env-test:claude-code:ses-abc-123, got %s", rec.Agent.ID)
	}
	if rec.Agent.SessionID != "ses-abc-123" {
		t.Errorf("expected SessionID ses-abc-123, got %s", rec.Agent.SessionID)
	}
	if rec.Agent.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("expected Lifecycle RUNNING, got %s", rec.Agent.Activity.Lifecycle)
	}
	if rec.Agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected Activity BUSY, got %s", rec.Agent.Activity.Activity)
	}
	if rec.Agent.Activity.Usage.Total != 1200 {
		t.Errorf("expected 1200 total tokens, got %d", rec.Agent.Activity.Usage.Total)
	}
	if rec.Agent.Activity.Cost == nil || rec.Agent.Activity.Cost.Total != 0.05 {
		t.Errorf("expected cost 0.05, got %v", rec.Agent.Activity.Cost)
	}
	if rec.Agent.Activity.Phase == nil || *rec.Agent.Activity.Phase != types.PhasePlanning {
		t.Errorf("expected phase planning, got %v", rec.Agent.Activity.Phase)
	}
	if len(rec.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(rec.Runs))
	}
	if rec.Runs[0].PID != 12345 {
		t.Errorf("expected run PID 12345, got %d", rec.Runs[0].PID)
	}
	if rec.EventCount != 3 {
		t.Errorf("expected 3 telemetry events, got %d", rec.EventCount)
	}
	if rec.FirstEventTimestamp == nil || rec.LastEventTimestamp == nil {
		t.Fatal("expected non-nil first and last event timestamps")
	}

	// 2. Inspect by full agent ID
	rec2, err := InspectSession(ctx, dbPath, "env-test:claude-code:ses-abc-123")
	if err != nil {
		t.Fatalf("InspectSession by agent ID failed: %v", err)
	}
	if rec2.Agent.SessionID != "ses-abc-123" {
		t.Errorf("expected SessionID ses-abc-123, got %s", rec2.Agent.SessionID)
	}

	// 3. Inspect exited agent with null cost and ended run
	recExited, err := InspectSession(ctx, dbPath, "ses-exited")
	if err != nil {
		t.Fatalf("InspectSession for exited agent failed: %v", err)
	}
	if recExited.Agent.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected Lifecycle EXITED, got %s", recExited.Agent.Activity.Lifecycle)
	}
	if recExited.Agent.Activity.Cost != nil {
		t.Errorf("expected nil cost, got %v", recExited.Agent.Activity.Cost)
	}
	if len(recExited.Runs) != 1 || recExited.Runs[0].EndedAt == nil {
		t.Errorf("expected 1 ended run, got %+v", recExited.Runs)
	}
	if recExited.EventCount != 0 {
		t.Errorf("expected 0 events, got %d", recExited.EventCount)
	}

	// 4. Inspect non-existent session
	_, err = InspectSession(ctx, dbPath, "ses-does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent session, got nil")
	}

	// 5. Inspect with invalid db path
	_, err = InspectSession(ctx, filepath.Join(t.TempDir(), "nonexistent.db"), "ses-1")
	if err == nil {
		t.Fatal("expected error for invalid db path, got nil")
	}
}

func TestListAgents(t *testing.T) {
	ctx := context.Background()
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	agents, err := ListAgents(ctx, dbPath)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents from test db, got %d", len(agents))
	}
	// Ordered by updated_at DESC (ses-abc-123 has 1000, ses-exited-456 has 500)
	if agents[0].SessionID != "ses-abc-123" {
		t.Errorf("expected first agent ses-abc-123, got %s", agents[0].SessionID)
	}
	if agents[1].SessionID != "ses-exited" {
		t.Errorf("expected second agent ses-exited, got %s", agents[1].SessionID)
	}

	// Invalid DB path returns error
	_, err = ListAgents(ctx, filepath.Join(t.TempDir(), "nonexistent.db"))
	if err == nil {
		t.Fatal("expected error for invalid db path, got nil")
	}
}

func TestInspect_RateLimitMetadata(t *testing.T) {
	ctx := context.Background()
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	resetAt := int64(1789900060000)
	retrySec := int64(60)
	_, err = db.Exec(`UPDATE agents SET activity = 'RATE_LIMITED', rate_limit_reset_at = ?, retry_after_seconds = ? WHERE session_id = 'ses-abc-123'`, resetAt, retrySec)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	agent, err := InspectSession(ctx, dbPath, "ses-abc-123")
	if err != nil {
		t.Fatalf("InspectSession failed: %v", err)
	}
	if agent.Agent == nil {
		t.Fatalf("agent.Agent is nil")
	}
	if agent.Agent.Activity.RateLimitResetAt == nil || *agent.Agent.Activity.RateLimitResetAt != resetAt {
		t.Errorf("got RateLimitResetAt %v, want %d", agent.Agent.Activity.RateLimitResetAt, resetAt)
	}
	if agent.Agent.Activity.RetryAfterSeconds == nil || *agent.Agent.Activity.RetryAfterSeconds != retrySec {
		t.Errorf("got RetryAfterSeconds %v, want %d", agent.Agent.Activity.RetryAfterSeconds, retrySec)
	}
}

func TestInspect_CompletionMetadata(t *testing.T) {
	ctx := context.Background()
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	completedAt := int64(1789900065000)
	_, err = db.Exec(`UPDATE agents SET activity = 'IDLE', has_unseen_completion = 1, completed_at = ? WHERE session_id = 'ses-abc-123'`, completedAt)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	inspected, err := InspectSession(ctx, dbPath, "ses-abc-123")
	if err != nil {
		t.Fatalf("InspectSession failed: %v", err)
	}
	if inspected.Agent == nil {
		t.Fatalf("inspected.Agent is nil")
	}
	if inspected.Agent.Activity.HasUnseenCompletion == nil || !*inspected.Agent.Activity.HasUnseenCompletion {
		t.Errorf("got HasUnseenCompletion %v, want true", inspected.Agent.Activity.HasUnseenCompletion)
	}
	if inspected.Agent.Activity.CompletedAt == nil || *inspected.Agent.Activity.CompletedAt != completedAt {
		t.Errorf("got CompletedAt %v, want %d", inspected.Agent.Activity.CompletedAt, completedAt)
	}

	agents, err := ListAgents(ctx, dbPath)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	var found *types.IAgentState
	for i := range agents {
		if agents[i].SessionID == "ses-abc-123" {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("agent ses-abc-123 not found in ListAgents")
	}
	if found.Activity.HasUnseenCompletion == nil || !*found.Activity.HasUnseenCompletion {
		t.Errorf("ListAgents got HasUnseenCompletion %v, want true", found.Activity.HasUnseenCompletion)
	}
	if found.Activity.CompletedAt == nil || *found.Activity.CompletedAt != completedAt {
		t.Errorf("ListAgents got CompletedAt %v, want %d", found.Activity.CompletedAt, completedAt)
	}
}

