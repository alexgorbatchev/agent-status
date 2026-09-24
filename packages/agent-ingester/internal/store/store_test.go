package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestOpen_Memory(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	if st.DB() == nil {
		t.Errorf("expected DB() to be non-nil")
	}

	var version int
	if err := st.DB().QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("query user_version failed: %v", err)
	}
	if version != 6 {
		t.Errorf("got user_version = %d, want 6", version)
	}
}

func TestOpen_DiskPath(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sub", "test.db")

	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(diskPath) failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("db file not found: %v", err)
	}

	if err := Migrate(st.DB()); err != nil {
		t.Errorf("Migrate() second run failed: %v", err)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	tempFile, err := os.CreateTemp("", "db-file")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer func() {
		_ = os.Remove(tempFile.Name())
	}()
	_ = tempFile.Close()

	invalidPath := filepath.Join(tempFile.Name(), "subdir", "test.db")
	_, err = Open(invalidPath)
	if err == nil {
		t.Errorf("expected error opening db with invalid dir path, got nil")
	}
}

func TestOpen_MigrationFailure(t *testing.T) {
	// Attempt to open /dev/null as a database which fails migration table creation
	_, err := Open("/dev/null")
	if err == nil {
		t.Errorf("expected error opening /dev/null, got nil")
	}
}

func TestMigrate_ErrorCases(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	_ = db.Close()
	if err := Migrate(db); err == nil {
		t.Errorf("expected error on closed db, got nil")
	}

	// Test v2 failure when agents table does not exist
	db2, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		_ = db2.Close()
	}()
	if _, err := db2.Exec("PRAGMA user_version = 1;"); err != nil {
		t.Fatalf("setting user_version = 1 failed: %v", err)
	}
	if err := Migrate(db2); err == nil {
		t.Errorf("expected error running v2 migration without agents table, got nil")
	}

	// Test v3 failure when agents table does not exist
	db3, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		_ = db3.Close()
	}()
	if _, err := db3.Exec("PRAGMA user_version = 2;"); err != nil {
		t.Fatalf("setting user_version = 2 failed: %v", err)
	}
	if err := Migrate(db3); err == nil {
		t.Errorf("expected error running v3 migration without agents table, got nil")
	}

	// Test v4 failure when agents table does not exist
	db4, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		_ = db4.Close()
	}()
	if _, err := db4.Exec("PRAGMA user_version = 3;"); err != nil {
		t.Fatalf("setting user_version = 3 failed: %v", err)
	}
	if err := Migrate(db4); err == nil {
		t.Errorf("expected error running v4 migration without agents table, got nil")
	}

	// Test v5 failure when agents table does not exist
	db5, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		_ = db5.Close()
	}()
	if _, err := db5.Exec("PRAGMA user_version = 4;"); err != nil {
		t.Fatalf("setting user_version = 4 failed: %v", err)
	}
	if err := Migrate(db5); err == nil {
		t.Errorf("expected error running v5 migration without agents table, got nil")
	}

	// Test v6 failure when agents table does not exist
	db6, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		_ = db6.Close()
	}()
	if _, err := db6.Exec("PRAGMA user_version = 5;"); err != nil {
		t.Fatalf("setting user_version = 5 failed: %v", err)
	}
	if err := Migrate(db6); err == nil {
		t.Errorf("expected error running v6 migration without agents table, got nil")
	}
}

func TestMigrate_V2ToV4(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
		t.Fatalf("v2 is_hidden failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
		t.Fatalf("v2 hidden_at failed: %v", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_agents_hidden ON agents(is_hidden);"); err != nil {
		t.Fatalf("v2 idx_agents_hidden failed: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 2;"); err != nil {
		t.Fatalf("setting user_version = 2 failed: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() from v2 failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 6 {
		t.Errorf("got user_version = %d, want 6", version)
	}
}

func TestMigrate_V4ToV5(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
		t.Fatalf("v2 is_hidden failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
		t.Fatalf("v2 hidden_at failed: %v", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_agents_hidden ON agents(is_hidden);"); err != nil {
		t.Fatalf("v2 idx_agents_hidden failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
		t.Fatalf("v3 active_tool_started_at failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_subagents TEXT;"); err != nil {
		t.Fatalf("v4 active_subagents failed: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 4;"); err != nil {
		t.Fatalf("setting user_version = 4 failed: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() from v4 failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 6 {
		t.Errorf("got user_version = %d, want 6", version)
	}

	// Verify new columns exist by selecting them
	var model sql.NullString
	var cw sql.NullInt64
	var fullness sql.NullFloat64
	err = db.QueryRow("SELECT model, context_window, context_fullness FROM agents LIMIT 1;").Scan(&model, &cw, &fullness)
	if err != nil && err != sql.ErrNoRows {
		t.Errorf("querying v5 columns failed: %v", err)
	}
}

func TestMigrate_V5ToV6(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Execute v1 schema manually with the old CHECK constraint (no RATE_LIMITED)
	const schemaV1Old = `
CREATE TABLE environments (
    id                TEXT PRIMARY KEY,
    label             TEXT NOT NULL,
    hostname          TEXT NOT NULL,
    platform          TEXT NOT NULL,
    user              TEXT NOT NULL,
    last_heartbeat_at INTEGER NOT NULL
);

CREATE TABLE runs (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    pid             INTEGER NOT NULL,
    harness_version TEXT NOT NULL,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER
);

CREATE TABLE agents (
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
    is_hidden             INTEGER NOT NULL DEFAULT 0,
    hidden_at             INTEGER,
    active_tool_started_at INTEGER,
    active_subagents      TEXT,
    model                 TEXT,
    context_window        INTEGER,
    context_fullness      REAL,
    UNIQUE (environment_id, harness, session_id)
);
`
	if _, err := db.Exec(schemaV1Old); err != nil {
		t.Fatalf("schemaV1Old failed: %v", err)
	}
	if _, err := db.Exec("INSERT INTO environments VALUES ('env-1', 'dev', 'host', 'linux', 'user', 1000);"); err != nil {
		t.Fatalf("insert env failed: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
		t.Fatalf("setting user_version = 5 failed: %v", err)
	}

	// In v5, attempting to insert an agent with RATE_LIMITED must fail check constraint
	const insertAgent = `INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, first_started_at, updated_at, activity)
		VALUES ('a1', 's1', 'env-1', 'claude-code', 'p1', 'p1', '/repo', 'main', '/repo', 1000, 1000, 'RATE_LIMITED');`
	if _, err := db.Exec(insertAgent); err == nil {
		t.Fatalf("expected CHECK constraint failure inserting RATE_LIMITED before migration v6")
	}

	// Now run Migrate to upgrade from v5 to v6
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() from v5 to v6 failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 6 {
		t.Errorf("got user_version = %d, want 6", version)
	}

	// Now inserting RATE_LIMITED must succeed
	if _, err := db.Exec(insertAgent); err != nil {
		t.Fatalf("failed to insert RATE_LIMITED agent after migration v6: %v", err)
	}

	var readActivity string
	if err := db.QueryRow("SELECT activity FROM agents WHERE id = 'a1';").Scan(&readActivity); err != nil {
		t.Fatalf("reading activity: %v", err)
	}
	if readActivity != "RATE_LIMITED" {
		t.Errorf("got activity %q, want 'RATE_LIMITED'", readActivity)
	}
}

func TestMigrate_V3ToV4(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
		t.Fatalf("v2 failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
		t.Fatalf("v2 failed: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
		t.Fatalf("v3 failed: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 3;"); err != nil {
		t.Fatalf("setting user_version = 3 failed: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() from v3 failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 6 {
		t.Errorf("got user_version = %d, want 6", version)
	}
}

func TestMigrate_V1ToV2(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	// Execute v1 schema manually and set user_version = 1
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 1;"); err != nil {
		t.Fatalf("setting user_version = 1 failed: %v", err)
	}

	// Now run Migrate, which should run v2
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() v1->v2 failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 6 {
		t.Errorf("got user_version = %d, want 6", version)
	}

	var indexCount int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_agents_hidden';").Scan(&indexCount); err != nil {
		t.Fatalf("querying index: %v", err)
	}
	if indexCount != 1 {
		t.Errorf("expected idx_agents_hidden to exist, got count %d", indexCount)
	}

	var hasCol int
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('agents') WHERE name = 'active_tool_started_at';").Scan(&hasCol); err != nil {
		t.Fatalf("querying active_tool_started_at column: %v", err)
	}
	if hasCol != 1 {
		t.Errorf("expected active_tool_started_at column to exist, got count %d", hasCol)
	}

	var hasSubCol int
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('agents') WHERE name = 'active_subagents';").Scan(&hasSubCol); err != nil {
		t.Fatalf("querying active_subagents column: %v", err)
	}
	if hasSubCol != 1 {
		t.Errorf("expected active_subagents column to exist, got count %d", hasSubCol)
	}
}

func TestCursors(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	envID := "env-cursor-test"

	seq, err := st.GetLastSeq(envID)
	if err != nil {
		t.Fatalf("GetLastSeq() failed: %v", err)
	}
	if seq != 0 {
		t.Errorf("expected 0 for non-existent cursor, got %d", seq)
	}

	env := types.IEnvironmentInfo{
		ID:              envID,
		Label:           "test-env",
		Hostname:        "localhost",
		Platform:        "darwin",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	if err := st.UpsertEnvironment(env); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	if err := st.SaveCursor(envID, 42); err != nil {
		t.Fatalf("SaveCursor failed: %v", err)
	}

	seq, err = st.GetLastSeq(envID)
	if err != nil {
		t.Fatalf("GetLastSeq() after save failed: %v", err)
	}
	if seq != 42 {
		t.Errorf("got seq = %d, want 42", seq)
	}

	if err := st.SaveCursor(envID, 100); err != nil {
		t.Fatalf("SaveCursor update failed: %v", err)
	}

	seq, err = st.GetLastSeq(envID)
	if err != nil {
		t.Fatalf("GetLastSeq() after update failed: %v", err)
	}
	if seq != 100 {
		t.Errorf("got seq = %d, want 100", seq)
	}
}

func TestEnvironments(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	emptyEnvs, err := st.GetAllEnvironments()
	if err != nil {
		t.Fatalf("GetAllEnvironments on empty db failed: %v", err)
	}
	if len(emptyEnvs) != 0 {
		t.Errorf("expected 0 environments, got %d", len(emptyEnvs))
	}

	now := time.Now().UnixMilli()
	env1 := types.IEnvironmentInfo{
		ID:              "env-1",
		Label:           "laptop",
		Hostname:        "macbook.local",
		Platform:        "darwin",
		User:            "alex",
		LastHeartbeatAt: now,
	}

	if err := st.UpsertEnvironment(env1); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	if err := st.IncrementDroppedFrames("env-1", 5); err != nil {
		t.Fatalf("IncrementDroppedFrames failed: %v", err)
	}
	if err := st.IncrementDroppedFrames("env-1", 3); err != nil {
		t.Fatalf("IncrementDroppedFrames 2 failed: %v", err)
	}

	newHeartbeat := now + 1000
	if err := st.RecordHeartbeat("env-1", newHeartbeat); err != nil {
		t.Fatalf("RecordHeartbeat failed: %v", err)
	}

	env2 := types.IEnvironmentInfo{
		ID:              "env-2",
		Label:           "server",
		Hostname:        "linux-box",
		Platform:        "linux",
		User:            "root",
		LastHeartbeatAt: now - 60000,
	}
	if err := st.UpsertEnvironment(env2); err != nil {
		t.Fatalf("UpsertEnvironment env2 failed: %v", err)
	}

	envs, err := st.GetAllEnvironments()
	if err != nil {
		t.Fatalf("GetAllEnvironments failed: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("got %d environments, want 2", len(envs))
	}

	envMap := make(map[string]types.IEnvironmentInfo)
	for _, e := range envs {
		envMap[e.ID] = e
	}

	got1, ok := envMap["env-1"]
	if !ok {
		t.Fatalf("env-1 not found")
	}
	if got1.Label != "laptop" || got1.LastHeartbeatAt != newHeartbeat || got1.IsStale {
		t.Errorf("env-1 mismatch: %+v", got1)
	}

	got2, ok := envMap["env-2"]
	if !ok {
		t.Fatalf("env-2 not found")
	}
	if !got2.IsStale {
		t.Errorf("expected env-2 to be stale (heartbeat 60s ago)")
	}

	env1Updated := env1
	env1Updated.Label = "laptop-renamed"
	if err := st.UpsertEnvironment(env1Updated); err != nil {
		t.Fatalf("UpsertEnvironment update failed: %v", err)
	}
	envsAfter, _ := st.GetAllEnvironments()
	for _, e := range envsAfter {
		if e.ID == "env-1" && e.Label != "laptop-renamed" {
			t.Errorf("got label %q, want laptop-renamed", e.Label)
		}
	}
}

func TestAgentStateAndRuns(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	agentNil, err := st.GetAgent("non-existent")
	if err != nil {
		t.Fatalf("GetAgent non-existent error: %v", err)
	}
	if agentNil != nil {
		t.Errorf("expected nil agent, got %+v", agentNil)
	}

	now := time.Now().UnixMilli()
	phase := types.PhaseEditing
	tool := "Edit"
	target := "src/main.ts"
	question := "Proceed?"
	thinking := "Refactoring logic..."
	lastErr := "compilation error"
	relPath := "worktrees/feat"

	agent := types.IAgentState{
		ID:        "env-1:claude-code:sess-1",
		SessionID: "sess-1",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:              "env-1",
			Label:           "dev-mac",
			Hostname:        "mac",
			Platform:        "darwin",
			User:            "alex",
			LastHeartbeatAt: now - 50000, // stale heartbeat to test IsStale in GetAgent
		},
		Project: types.IProjectInfo{
			ID:            "github.com/alex/agent-status",
			Name:          "agent-status",
			CanonicalRoot: "/dev/agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     "feat/test",
			IsDetached: true,
			IsWorktree: true,
			RelPath:    &relPath,
			CWD:        "/dev/agent-status/worktrees/feat",
		},
		Activity: types.IAgentActivity{
			Lifecycle:   types.LifecycleRunning,
			Activity:    types.ActivityBusy,
			Title:       "Feature Dev",
			Action:      "Editing src/main.ts",
			ActionStale: true,
			CurrentRun: &types.IRunInfo{
				ID:             "run-101",
				PID:            1234,
				HarnessVersion: "2.1.277",
				StartedAt:      now,
			},
			Phase:               &phase,
			ActiveTool:          &tool,
			ActiveTarget:        &target,
			Question:            &question,
			LastThinkingExcerpt: &thinking,
			LastError:           &lastErr,
			Usage: types.ITokenUsage{
				Input:      1000,
				Output:     200,
				CacheRead:  5000,
				CacheWrite: 1000,
				Reasoning:  100,
				Total:      7300,
			},
			Cost: &types.ICostInfo{
				Currency: "USD",
				Total:    0.056,
			},
			SubagentCount:  2,
			FirstStartedAt: now - 10000,
			UpdatedAt:      now,
		},
	}

	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	got, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected agent to be found")
	}

	if !got.Environment.IsStale {
		t.Errorf("expected got.Environment.IsStale to be true")
	}

	if got.ID != agent.ID || got.SessionID != "sess-1" || got.Harness != types.HarnessClaudeCode {
		t.Errorf("identity mismatch: %+v", got)
	}
	if !got.Workspace.IsDetached || !got.Workspace.IsWorktree || got.Workspace.RelPath == nil || *got.Workspace.RelPath != relPath {
		t.Errorf("workspace mismatch: %+v", got.Workspace)
	}
	if got.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("got lifecycle %s, want %s", got.Activity.Lifecycle, types.LifecycleRunning)
	}
	if !got.Activity.ActionStale {
		t.Errorf("expected ActionStale = true")
	}
	if got.Activity.CurrentRun == nil || got.Activity.CurrentRun.ID != "run-101" || got.Activity.CurrentRun.PID != 1234 {
		t.Errorf("currentRun mismatch: %+v", got.Activity.CurrentRun)
	}
	if got.Activity.Phase == nil || *got.Activity.Phase != types.PhaseEditing {
		t.Errorf("phase mismatch: %v", got.Activity.Phase)
	}
	if got.Activity.ActiveTool == nil || *got.Activity.ActiveTool != tool {
		t.Errorf("activeTool mismatch: %v", got.Activity.ActiveTool)
	}
	if got.Activity.ActiveTarget == nil || *got.Activity.ActiveTarget != target {
		t.Errorf("activeTarget mismatch: %v", got.Activity.ActiveTarget)
	}
	if got.Activity.Question == nil || *got.Activity.Question != question {
		t.Errorf("question mismatch: %v", got.Activity.Question)
	}
	if got.Activity.LastThinkingExcerpt == nil || *got.Activity.LastThinkingExcerpt != thinking {
		t.Errorf("thinking mismatch: %v", got.Activity.LastThinkingExcerpt)
	}
	if got.Activity.LastError == nil || *got.Activity.LastError != lastErr {
		t.Errorf("lastError mismatch: %v", got.Activity.LastError)
	}
	if got.Activity.Cost == nil || got.Activity.Cost.Total != 0.056 {
		t.Errorf("cost mismatch: %+v", got.Activity.Cost)
	}
	if got.Activity.SubagentCount != 2 {
		t.Errorf("subagentCount = %d, want 2", got.Activity.SubagentCount)
	}

	// Create second agent to test multiple items in GetAllActiveAgents
	agent2 := agent
	agent2.ID = "env-1:claude-code:sess-2"
	agent2.SessionID = "sess-2"
	agent2.Activity.CurrentRun = nil
	agent2.Activity.UpdatedAt = now + 100
	if err := st.SaveAgentState(agent2); err != nil {
		t.Fatalf("SaveAgentState(agent2) failed: %v", err)
	}

	allAgents, err := st.GetAllActiveAgents()
	if err != nil {
		t.Fatalf("GetAllActiveAgents failed: %v", err)
	}
	if len(allAgents) != 2 {
		t.Errorf("GetAllActiveAgents count = %d, want 2", len(allAgents))
	}

	// Update existing agent state with new run and updated fields
	agentUpdated := *got
	agentUpdated.Activity.Title = "Updated Title"
	agentUpdated.Activity.ActionStale = false
	agentUpdated.Activity.CurrentRun = &types.IRunInfo{
		ID:             "run-102",
		PID:            5678,
		HarnessVersion: "2.1.278",
		StartedAt:      now + 100,
	}
	if err := st.SaveAgentState(agentUpdated); err != nil {
		t.Fatalf("SaveAgentState(agentUpdated) failed: %v", err)
	}

	gotUpdated, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent updated error: %v", err)
	}
	if gotUpdated.Activity.Title != "Updated Title" || gotUpdated.Activity.CurrentRun.ID != "run-102" || gotUpdated.Activity.ActionStale {
		t.Errorf("gotUpdated mismatch: %+v", gotUpdated)
	}

	subID := "sub-99"
	frame := types.ITelemetryFrame{
		AgentID:     agent.ID,
		SubagentID:  &subID,
		EventType:   "tool_call",
		Timestamp:   now,
		Data: types.TelemetryFrameData{
			ToolName: &tool,
		},
	}
	if err := st.RecordTelemetryEvent(frame); err != nil {
		t.Fatalf("RecordTelemetryEvent failed: %v", err)
	}

	endedAt := now + 5000
	if err := st.EndCurrentRun(agent.ID, endedAt); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}

	afterEnd, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after end failed: %v", err)
	}
	if afterEnd.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("got lifecycle %s, want %s", afterEnd.Activity.Lifecycle, types.LifecycleExited)
	}

	if err := st.EndCurrentRun(agent.ID, endedAt+1000); err != nil {
		t.Fatalf("EndCurrentRun when null failed: %v", err)
	}

	if err := st.EndCurrentRun("non-existent-agent-id", endedAt+2000); err != nil {
		t.Fatalf("EndCurrentRun for non-existent agent error: %v", err)
	}
}

func TestAgentState_AllHarnessTypes(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	now := time.Now().UnixMilli()
	env := types.IEnvironmentInfo{
		ID:              "env-harness-test",
		Label:           "test-env",
		Hostname:        "localhost",
		Platform:        "darwin",
		User:            "alex",
		LastHeartbeatAt: now,
	}
	if err := st.UpsertEnvironment(env); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	harnesses := []types.HarnessType{
		types.HarnessClaudeCode,
		types.HarnessPi,
		types.HarnessCodex,
		types.HarnessOpencode,
	}

	for _, h := range harnesses {
		t.Run(string(h), func(t *testing.T) {
			agent := types.IAgentState{
				ID:        "env-harness-test:" + string(h) + ":sess-1",
				SessionID: "sess-1",
				Harness:   h,
				Environment: env,
				Project: types.IProjectInfo{
					ID:            "github.com/alex/test",
					Name:          "test",
					CanonicalRoot: "/test",
				},
				Workspace: types.IWorkspaceInfo{
					Branch: "main",
					CWD:    "/test",
				},
				Activity: types.IAgentActivity{
					Lifecycle:      types.LifecycleRunning,
					Activity:       types.ActivityBusy,
					Title:          "Testing " + string(h),
					FirstStartedAt: now,
					UpdatedAt:      now,
				},
			}

			if err := st.SaveAgentState(agent); err != nil {
				t.Fatalf("SaveAgentState(%s) failed: %v", h, err)
			}

			got, err := st.GetAgent(agent.ID)
			if err != nil {
				t.Fatalf("GetAgent(%s) failed: %v", h, err)
			}
			if got == nil || got.Harness != h {
				t.Fatalf("GetAgent(%s) got harness %v, want %v", h, got, h)
			}
		})
	}
}

func TestAgentState_NilFieldsAndExitedRun(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	now := time.Now().UnixMilli()
	ended := now - 1000

	agent := types.IAgentState{
		ID:        "env-nil:pi:sess-nil",
		SessionID: "sess-nil",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:              "env-nil",
			Label:           "minimal",
			Hostname:        "host",
			Platform:        "linux",
			User:            "user",
			LastHeartbeatAt: now,
		},
		Project: types.IProjectInfo{
			ID:            "project-1",
			Name:          "proj",
			CanonicalRoot: "/root",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     "main",
			IsDetached: false,
			IsWorktree: false,
			RelPath:    nil,
			CWD:        "/root",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			CurrentRun: &types.IRunInfo{
				ID:             "run-ended",
				PID:            5555,
				HarnessVersion: "1.0.0",
				StartedAt:      now - 5000,
				EndedAt:        &ended,
			},
			Cost:           nil,
			FirstStartedAt: now - 5000,
			UpdatedAt:      now,
		},
	}

	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	got, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil {
		t.Fatalf("GetAgent returned nil")
	}

	if got.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("got lifecycle %s, want %s", got.Activity.Lifecycle, types.LifecycleExited)
	}
	if got.Activity.Cost != nil {
		t.Errorf("expected nil cost, got %+v", got.Activity.Cost)
	}
	if got.Activity.Phase != nil || got.Activity.ActiveTool != nil || got.Activity.ActiveTarget != nil || got.Activity.Question != nil || got.Activity.LastThinkingExcerpt != nil || got.Activity.LastError != nil {
		t.Errorf("expected all optional activity fields to be nil, got %+v", got.Activity)
	}

	agentNoRun := *got
	agentNoRun.Activity.CurrentRun = nil
	if err := st.SaveAgentState(agentNoRun); err != nil {
		t.Fatalf("SaveAgentState(agentNoRun) failed: %v", err)
	}

	gotNoRun, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after no run failed: %v", err)
	}
	if gotNoRun.Activity.CurrentRun != nil {
		t.Errorf("expected CurrentRun to be nil, got %+v", gotNoRun.Activity.CurrentRun)
	}
}

func TestStore_PruneOldEventsAndExitedAgents_ErrorPaths(t *testing.T) {
	// 1. Error on deleting telemetry_events
	t.Run("error deleting telemetry_events", func(t *testing.T) {
		st, err := Open(":memory:")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer func() { _ = st.Close() }()

		if _, err := st.db.Exec("INSERT INTO environments VALUES ('env-1', 'd', 'h', 'linux', 'u', 1000, 0);"); err != nil {
			t.Fatalf("insert env failed: %v", err)
		}
		if _, err := st.db.Exec("INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, first_started_at, updated_at, activity) VALUES ('a', 's', 'env-1', 'claude-code', 'p', 'p', '/r', 'm', '/r', 50, 50, 'IDLE');"); err != nil {
			t.Fatalf("insert ag failed: %v", err)
		}
		if _, err := st.db.Exec("INSERT INTO telemetry_events (agent_id, event_type, timestamp, data_json) VALUES ('a', 'e', 50, '{}');"); err != nil {
			t.Fatalf("insert te failed: %v", err)
		}
		if _, err := st.db.Exec("CREATE TRIGGER prevent_te_del BEFORE DELETE ON telemetry_events BEGIN SELECT RAISE(ABORT, 'te error'); END;"); err != nil {
			t.Fatalf("create trigger failed: %v", err)
		}
		if err := st.PruneOldEventsAndExitedAgents(100, 100); err == nil {
			t.Errorf("expected error deleting telemetry_events")
		}
	})

	// 2. Error on deleting agents
	t.Run("error deleting agents", func(t *testing.T) {
		st, err := Open(":memory:")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer func() { _ = st.Close() }()

		if _, err := st.db.Exec("INSERT INTO environments VALUES ('env-1', 'd', 'h', 'linux', 'u', 1000, 0);"); err != nil {
			t.Fatalf("insert env failed: %v", err)
		}
		if _, err := st.db.Exec("INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, first_started_at, updated_at, activity) VALUES ('a', 's', 'env-1', 'claude-code', 'p', 'p', '/r', 'm', '/r', 50, 50, 'IDLE');"); err != nil {
			t.Fatalf("insert ag failed: %v", err)
		}
		if _, err := st.db.Exec("CREATE TRIGGER prevent_ag_del BEFORE DELETE ON agents BEGIN SELECT RAISE(ABORT, 'ag error'); END;"); err != nil {
			t.Fatalf("create trigger failed: %v", err)
		}
		if err := st.PruneOldEventsAndExitedAgents(100, 100); err == nil {
			t.Errorf("expected error deleting agents")
		}
	})

	// 3. Error on deleting runs
	t.Run("error deleting runs", func(t *testing.T) {
		st, err := Open(":memory:")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer func() { _ = st.Close() }()

		if _, err := st.db.Exec("INSERT INTO environments VALUES ('env-1', 'd', 'h', 'linux', 'u', 1000, 0);"); err != nil {
			t.Fatalf("insert env failed: %v", err)
		}
		if _, err := st.db.Exec("INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, first_started_at, updated_at, activity) VALUES ('a', 's', 'env-1', 'claude-code', 'p', 'p', '/r', 'm', '/r', 50, 200, 'IDLE');"); err != nil {
			t.Fatalf("insert ag failed: %v", err)
		}
		if _, err := st.db.Exec("INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at) VALUES ('r', 'a', 1, '1', 50, 50);"); err != nil {
			t.Fatalf("insert run failed: %v", err)
		}
		if _, err := st.db.Exec("CREATE TRIGGER prevent_runs_del BEFORE DELETE ON runs BEGIN SELECT RAISE(ABORT, 'runs error'); END;"); err != nil {
			t.Fatalf("create trigger failed: %v", err)
		}
		if err := st.PruneOldEventsAndExitedAgents(100, 100); err == nil {
			t.Errorf("expected error deleting runs")
		}
	})

	// 4. Closed DB
	t.Run("closed db", func(t *testing.T) {
		st, err := Open(":memory:")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		_ = st.Close()
		if err := st.PruneOldEventsAndExitedAgents(100, 100); err == nil {
			t.Errorf("expected error from PruneOldEventsAndExitedAgents on closed DB")
		}
	})
}

func TestStore_EndCurrentRun_TriggerErrors(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	if _, err := st.db.Exec("INSERT INTO environments VALUES ('env-1', 'd', 'h', 'linux', 'u', 1000, 0);"); err != nil {
		t.Fatalf("insert env failed: %v", err)
	}
	if _, err := st.db.Exec("INSERT INTO runs (id, agent_id, pid, harness_version, started_at) VALUES ('r1', 'a1', 1, '1', 1000);"); err != nil {
		t.Fatalf("insert run failed: %v", err)
	}
	if _, err := st.db.Exec("INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, first_started_at, updated_at, activity, current_run_id) VALUES ('a1', 's1', 'env-1', 'claude-code', 'p1', 'p1', '/r', 'm', '/r', 1000, 1000, 'IDLE', 'r1');"); err != nil {
		t.Fatalf("insert agent failed: %v", err)
	}

	// 1. Error updating runs
	if _, err := st.db.Exec("CREATE TRIGGER prevent_run_upd BEFORE UPDATE ON runs BEGIN SELECT RAISE(ABORT, 'runs upd error'); END;"); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}
	if err := st.EndCurrentRun("a1", 2000); err == nil {
		t.Errorf("expected error updating runs in EndCurrentRun")
	}
	if _, err := st.db.Exec("DROP TRIGGER prevent_run_upd;"); err != nil {
		t.Fatalf("drop trigger failed: %v", err)
	}

	// 2. Error updating agents
	if _, err := st.db.Exec("CREATE TRIGGER prevent_agent_upd BEFORE UPDATE OF current_run_id ON agents BEGIN SELECT RAISE(ABORT, 'agents upd error'); END;"); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}
	if err := st.EndCurrentRun("a1", 2000); err == nil {
		t.Errorf("expected error updating agents in EndCurrentRun")
	}
}

func TestStore_GetDistinctSubagentIDs_ClosedDB(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	_ = st.Close()
	if _, err := st.GetDistinctSubagentIDs("a1"); err == nil {
		t.Errorf("expected error from GetDistinctSubagentIDs on closed DB")
	}
}

func TestStore_EndCurrentRun_ClosedDB(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	_ = st.Close()
	if err := st.EndCurrentRun("agent-1", 100); err == nil {
		t.Errorf("expected error from EndCurrentRun on closed DB")
	}
}

func TestStore_ErrorHandlingOnClosedDB(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	_ = st.Close()

	if _, err := st.GetLastSeq("env"); err == nil {
		t.Errorf("expected error on closed db for GetLastSeq")
	}
	if err := st.SaveCursor("env", 1); err == nil {
		t.Errorf("expected error on closed db for SaveCursor")
	}
	if err := st.UpsertEnvironment(types.IEnvironmentInfo{ID: "e", Label: "l", Hostname: "h", Platform: "darwin", User: "u"}); err == nil {
		t.Errorf("expected error on closed db for UpsertEnvironment")
	}
	if err := st.RecordHeartbeat("e", 100); err == nil {
		t.Errorf("expected error on closed db for RecordHeartbeat")
	}
	if err := st.IncrementDroppedFrames("e", 1); err == nil {
		t.Errorf("expected error on closed db for IncrementDroppedFrames")
	}
	if err := st.RecordTelemetryEvent(types.ITelemetryFrame{AgentID: "a", Timestamp: 100}); err == nil {
		t.Errorf("expected error on closed db for RecordTelemetryEvent")
	}
	if err := st.SaveAgentState(types.IAgentState{ID: "a"}); err == nil {
		t.Errorf("expected error on closed db for SaveAgentState")
	}
	if _, err := st.GetAgent("a"); err == nil {
		t.Errorf("expected error on closed db for GetAgent")
	}
	if _, err := st.GetAllActiveAgents(); err == nil {
		t.Errorf("expected error on closed db for GetAllActiveAgents")
	}
	if _, err := st.GetRunningAgents(""); err == nil {
		t.Errorf("expected error on closed db for GetRunningAgents")
	}
	if _, err := st.GetAllEnvironments(); err == nil {
		t.Errorf("expected error on closed db for GetAllEnvironments")
	}
	if _, err := st.GetDistinctSubagentIDs("a"); err == nil {
		t.Errorf("expected error on closed db for GetDistinctSubagentIDs")
	}
	if _, err := st.SetAgentVisibility("a", true, nil); err == nil {
		t.Errorf("expected error on closed db for SetAgentVisibility")
	}
	if err := st.RecordTelemetryEvent(types.ITelemetryFrame{}); err == nil {
		t.Errorf("expected error on closed db for RecordTelemetryEvent")
	}
	if err := st.EndCurrentRun("a", 100); err == nil {
		t.Errorf("expected error on closed db for EndCurrentRun")
	}
	if _, err := st.SetAgentVisibility("a", true, nil); err == nil {
		t.Errorf("expected error on closed db for SetAgentVisibility")
	}
	if _, err := st.GetAllActiveAgents(); err == nil {
		t.Errorf("expected error on closed db for GetAllActiveAgents")
	}
	if _, err := st.GetAllEnvironments(); err == nil {
		t.Errorf("expected error on closed db for GetAllEnvironments")
	}
	if err := st.PruneOldEventsAndExitedAgents(100, 100); err == nil {
		t.Errorf("expected error on closed db for PruneOldEventsAndExitedAgents")
	}
}

func TestPruneOldEventsAndExitedAgents(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	now := time.Now().UnixMilli()
	oldTime := now - (50 * 3600 * 1000) // 50 hours ago
	recentTime := now - (10 * 1000)

	env := types.IEnvironmentInfo{
		ID:              "env-prune",
		Label:           "prune-env",
		Hostname:        "localhost",
		Platform:        "darwin",
		User:            "tester",
		LastHeartbeatAt: now,
	}
	if err := st.UpsertEnvironment(env); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	agent1 := types.IAgentState{
		ID:          "env-prune:claude-code:sess-old",
		SessionID:   "sess-old",
		Harness:     types.HarnessClaudeCode,
		Environment: env,
		Project: types.IProjectInfo{
			ID:            "github.com/test/repo",
			Name:          "repo",
			CanonicalRoot: "/repo",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/repo",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleExited,
			Activity:       types.ActivityIdle,
			FirstStartedAt: now - (45 * 24 * 3600 * 1000),
			UpdatedAt:      now - (40 * 24 * 3600 * 1000),
		},
	}
	if err := st.SaveAgentState(agent1); err != nil {
		t.Fatalf("SaveAgentState(agent1) failed: %v", err)
	}

	agent2 := types.IAgentState{
		ID:          "env-prune:claude-code:sess-active",
		SessionID:   "sess-active",
		Harness:     types.HarnessClaudeCode,
		Environment: env,
		Project: types.IProjectInfo{
			ID:            "github.com/test/repo",
			Name:          "repo",
			CanonicalRoot: "/repo",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/repo",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: recentTime,
			UpdatedAt:      recentTime,
		},
	}
	if err := st.SaveAgentState(agent2); err != nil {
		t.Fatalf("SaveAgentState(agent2) failed: %v", err)
	}

	oldFrame := types.ITelemetryFrame{
		AgentID:   agent2.ID,
		EventType: "tool_call",
		Timestamp: oldTime,
		Data:      types.TelemetryFrameData{},
	}
	if err := st.RecordTelemetryEvent(oldFrame); err != nil {
		t.Fatalf("RecordTelemetryEvent(old) failed: %v", err)
	}

	recentFrame := types.ITelemetryFrame{
		AgentID:   agent2.ID,
		EventType: "tool_call",
		Timestamp: recentTime,
		Data:      types.TelemetryFrameData{},
	}
	if err := st.RecordTelemetryEvent(recentFrame); err != nil {
		t.Fatalf("RecordTelemetryEvent(recent) failed: %v", err)
	}

	eventCutoff := now - (48 * 3600 * 1000)
	exitedAgentCutoff := now - (30 * 24 * 3600 * 1000)

	if err := st.PruneOldEventsAndExitedAgents(eventCutoff, exitedAgentCutoff); err != nil {
		t.Fatalf("PruneOldEventsAndExitedAgents failed: %v", err)
	}

	prunedAgent, err := st.GetAgent(agent1.ID)
	if err != nil {
		t.Fatalf("GetAgent(agent1) error: %v", err)
	}
	if prunedAgent != nil {
		t.Errorf("expected old exited agent to be pruned, but found %+v", prunedAgent)
	}

	activeAgent, err := st.GetAgent(agent2.ID)
	if err != nil {
		t.Fatalf("GetAgent(agent2) error: %v", err)
	}
	if activeAgent == nil {
		t.Errorf("expected active agent to remain")
	}

	var eventCount int
	if err := st.DB().QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&eventCount); err != nil {
		t.Fatalf("query telemetry_events count failed: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("got %d telemetry events, want 1", eventCount)
	}

	// Test error in PruneOldEventsAndExitedAgents when table is dropped
	stDrop, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = stDrop.Close() }()
	if _, err := stDrop.DB().Exec("DROP TABLE telemetry_events;"); err != nil {
		t.Fatalf("dropping table failed: %v", err)
	}
	if err := stDrop.PruneOldEventsAndExitedAgents(100, 100); err == nil {
		t.Errorf("expected error in PruneOldEventsAndExitedAgents on dropped table")
	}
}

func TestPruneOldEventsAndExitedAgents_AbandonedAgentsAndRuns(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	now := time.Now().UnixMilli()
	exitedAgentCutoff := now - (30 * 24 * 3600 * 1000) // 30 days ago
	eventCutoff := now - (48 * 3600 * 1000)

	env := types.IEnvironmentInfo{
		ID:              "env-prune-runs",
		Label:           "prune-env-runs",
		Hostname:        "localhost",
		Platform:        "darwin",
		User:            "tester",
		LastHeartbeatAt: now,
	}
	if err := st.UpsertEnvironment(env); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	// 1. Abandoned agent: active lifecycle with current_run_id populated, but untouched for 40 days
	abandonedAgentID := "env-prune-runs:claude-code:sess-abandoned"
	abandonedRunID := "run-abandoned-1"
	abandonedAgent := types.IAgentState{
		ID:          abandonedAgentID,
		SessionID:   "sess-abandoned",
		Harness:     types.HarnessClaudeCode,
		Environment: env,
		Project: types.IProjectInfo{
			ID:            "github.com/test/repo",
			Name:          "repo",
			CanonicalRoot: "/repo",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/repo",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			CurrentRun: &types.IRunInfo{
				ID:             abandonedRunID,
				PID:            12345,
				HarnessVersion: "1.0",
				StartedAt:      now - (45 * 24 * 3600 * 1000),
				EndedAt:        nil,
			},
			FirstStartedAt: now - (45 * 24 * 3600 * 1000),
			UpdatedAt:      now - (40 * 24 * 3600 * 1000), // older than 30-day cutoff
		},
	}
	if err := st.SaveAgentState(abandonedAgent); err != nil {
		t.Fatalf("SaveAgentState(abandonedAgent) failed: %v", err)
	}

	// 2. Active agent: current_run_id populated, updated recently (10s ago)
	activeAgentID := "env-prune-runs:claude-code:sess-active-runs"
	activeRunID := "run-active-current"
	activeAgent := types.IAgentState{
		ID:          activeAgentID,
		SessionID:   "sess-active-runs",
		Harness:     types.HarnessClaudeCode,
		Environment: env,
		Project: types.IProjectInfo{
			ID:            "github.com/test/repo",
			Name:          "repo",
			CanonicalRoot: "/repo",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/repo",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			CurrentRun: &types.IRunInfo{
				ID:             activeRunID,
				PID:            54321,
				HarnessVersion: "1.0",
				StartedAt:      now - (1 * 3600 * 1000),
				EndedAt:        nil,
			},
			FirstStartedAt: now - (1 * 3600 * 1000),
			UpdatedAt:      now - 10000, // recent
		},
	}
	if err := st.SaveAgentState(activeAgent); err != nil {
		t.Fatalf("SaveAgentState(activeAgent) failed: %v", err)
	}

	// 3. Historical old completed run for activeAgent: ended 35 days ago (older than 30-day cutoff)
	oldRunEndedAt := now - (35 * 24 * 3600 * 1000)
	oldRunID := "run-active-old-completed"
	_, err = st.DB().Exec(`
		INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, oldRunID, activeAgentID, 54320, "1.0", oldRunEndedAt-3600000, oldRunEndedAt)
	if err != nil {
		t.Fatalf("insert old completed run failed: %v", err)
	}

	// 4. Recent completed run for activeAgent: ended 5 days ago (newer than 30-day cutoff)
	recentRunEndedAt := now - (5 * 24 * 3600 * 1000)
	recentRunID := "run-active-recent-completed"
	_, err = st.DB().Exec(`
		INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, recentRunID, activeAgentID, 54322, "1.0", recentRunEndedAt-3600000, recentRunEndedAt)
	if err != nil {
		t.Fatalf("insert recent completed run failed: %v", err)
	}

	// 5. Orphaned run: agent_id does not exist in agents table
	orphanRunID := "run-orphan-no-agent"
	_, err = st.DB().Exec(`
		INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, orphanRunID, "non-existent-agent-id", 99999, "1.0", now-100000, nil)
	if err != nil {
		t.Fatalf("insert orphan run failed: %v", err)
	}

	// Run PruneOldEventsAndExitedAgents
	if err := st.PruneOldEventsAndExitedAgents(eventCutoff, exitedAgentCutoff); err != nil {
		t.Fatalf("PruneOldEventsAndExitedAgents failed: %v", err)
	}

	// Assertions:
	// 1. Abandoned agent must be deleted
	prunedAbandoned, err := st.GetAgent(abandonedAgentID)
	if err != nil {
		t.Fatalf("GetAgent(abandonedAgentID) error: %v", err)
	}
	if prunedAbandoned != nil {
		t.Errorf("expected abandoned agent with current_run_id to be pruned, but found %+v", prunedAbandoned)
	}

	// 2. Active agent must remain
	gotActive, err := st.GetAgent(activeAgentID)
	if err != nil {
		t.Fatalf("GetAgent(activeAgentID) error: %v", err)
	}
	if gotActive == nil {
		t.Errorf("expected active agent to remain")
	}

	// 3. Runs verification
	checkRunExists := func(runID string) bool {
		var count int
		_ = st.DB().QueryRow("SELECT COUNT(*) FROM runs WHERE id = ?", runID).Scan(&count)
		return count > 0
	}

	if checkRunExists(abandonedRunID) {
		t.Errorf("expected abandoned agent's run %q to be pruned", abandonedRunID)
	}
	if checkRunExists(orphanRunID) {
		t.Errorf("expected orphan run %q with non-existent agent_id to be pruned", orphanRunID)
	}
	if checkRunExists(oldRunID) {
		t.Errorf("expected old completed run %q (ended > 30 days ago) to be pruned", oldRunID)
	}
	if !checkRunExists(activeRunID) {
		t.Errorf("expected active agent's current run %q to remain", activeRunID)
	}
	if !checkRunExists(recentRunID) {
		t.Errorf("expected active agent's recent completed run %q (ended 5 days ago) to remain", recentRunID)
	}
}

func TestAgentVisibility(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	now := time.Now().UnixMilli()
	agent := types.IAgentState{
		ID:        "env-1:claude-code:session-vis",
		SessionID: "session-vis",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:              "env-1",
			Label:           "test-env",
			Hostname:        "host-1",
			Platform:        "linux",
			User:            "user-1",
			LastHeartbeatAt: now,
		},
		Project: types.IProjectInfo{
			ID:            "proj-1",
			Name:          "proj",
			CanonicalRoot: "/work/proj",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/work/proj",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityIdle,
			Title:          "Visibility test",
			FirstStartedAt: now,
			UpdatedAt:      now,
		},
	}

	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	loaded, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if loaded.IsHidden {
		t.Fatalf("expected loaded.IsHidden == false, got true")
	}
	if loaded.HiddenAt != nil {
		t.Fatalf("expected loaded.HiddenAt == nil, got %v", loaded.HiddenAt)
	}

	hiddenAt := now + 1000
	updated, err := st.SetAgentVisibility(agent.ID, true, &hiddenAt)
	if err != nil {
		t.Fatalf("SetAgentVisibility failed: %v", err)
	}
	if !updated.IsHidden {
		t.Fatalf("expected updated.IsHidden == true, got false")
	}
	if updated.HiddenAt == nil || *updated.HiddenAt != hiddenAt {
		t.Fatalf("expected updated.HiddenAt == %d, got %v", hiddenAt, updated.HiddenAt)
	}

	// Verify persistence in DB
	loaded, err = st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after hide failed: %v", err)
	}
	if !loaded.IsHidden {
		t.Fatalf("expected loaded.IsHidden == true, got false")
	}
	if loaded.HiddenAt == nil || *loaded.HiddenAt != hiddenAt {
		t.Fatalf("expected loaded.HiddenAt == %d, got %v", hiddenAt, loaded.HiddenAt)
	}

	// Now unhide
	updated, err = st.SetAgentVisibility(agent.ID, false, nil)
	if err != nil {
		t.Fatalf("SetAgentVisibility(false) failed: %v", err)
	}
	if updated.IsHidden {
		t.Fatalf("expected updated.IsHidden == false, got true")
	}
	if updated.HiddenAt != nil {
		t.Fatalf("expected updated.HiddenAt == nil, got %v", updated.HiddenAt)
	}

	// Verify persistence
	loaded, err = st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after unhide failed: %v", err)
	}

	// Setting visibility on non-existent agent returns error
	if _, err := st.SetAgentVisibility("non-existent-agent", true, nil); err == nil {
		t.Errorf("expected error when setting visibility for non-existent agent")
	}
	if loaded.IsHidden {
		t.Fatalf("expected loaded.IsHidden == false, got true")
	}
	if loaded.HiddenAt != nil {
		t.Fatalf("expected loaded.HiddenAt == nil, got %v", loaded.HiddenAt)
	}

	// Not found error check
	_, err = st.SetAgentVisibility("non-existent", true, &hiddenAt)
	if err == nil {
		t.Fatalf("expected error for non-existent agent, got nil")
	}
}

func TestStore_GetDistinctSubagentIDs(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	agentID := "agent-sub-test"
	sub1 := "subagent-alpha"
	sub2 := "subagent-beta"

	for _, id := range []string{agentID, "other-agent"} {
		if err := st.SaveAgentState(types.IAgentState{
			ID:          id,
			SessionID:   id,
			Harness:     types.HarnessClaudeCode,
			Environment: types.IEnvironmentInfo{ID: "env-1", Platform: "darwin"},
			Activity: types.IAgentActivity{
				Lifecycle: types.LifecycleRunning,
				Activity:  types.ActivityIdle,
			},
		}); err != nil {
			t.Fatalf("SaveAgentState(%s) failed: %v", id, err)
		}
	}

	// Initial check on empty store (no events recorded yet)
	ids, err := st.GetDistinctSubagentIDs(agentID)
	if err != nil {
		t.Fatalf("GetDistinctSubagentIDs failed on empty store: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 subagent IDs, got %d", len(ids))
	}

	// Record multiple frames with sub1, sub2, nil, empty, and duplicate sub1
	frames := []types.ITelemetryFrame{
		{AgentID: agentID, SubagentID: &sub1, EventType: "tool_call", Timestamp: 100},
		{AgentID: agentID, SubagentID: &sub1, EventType: "tool_result", Timestamp: 101},
		{AgentID: agentID, SubagentID: &sub2, EventType: "turn_start", Timestamp: 102},
		{AgentID: agentID, SubagentID: nil, EventType: "thinking", Timestamp: 103},
		{AgentID: agentID, SubagentID: new(string), EventType: "turn_end", Timestamp: 104}, // empty string
		{AgentID: "other-agent", SubagentID: &sub1, EventType: "tool_call", Timestamp: 105},
	}
	for _, f := range frames {
		if err := st.RecordTelemetryEvent(f); err != nil {
			t.Fatalf("RecordTelemetryEvent failed: %v", err)
		}
	}

	ids, err = st.GetDistinctSubagentIDs(agentID)
	if err != nil {
		t.Fatalf("GetDistinctSubagentIDs failed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 distinct subagent IDs, got %d: %v", len(ids), ids)
	}
	if ids[0] != "subagent-alpha" || ids[1] != "subagent-beta" {
		t.Fatalf("unexpected subagent IDs: %v", ids)
	}

	// Verify other-agent isolation
	otherIDs, err := st.GetDistinctSubagentIDs("other-agent")
	if err != nil {
		t.Fatalf("GetDistinctSubagentIDs(other-agent) failed: %v", err)
	}
	if len(otherIDs) != 1 || otherIDs[0] != "subagent-alpha" {
		t.Fatalf("expected [subagent-alpha] for other-agent, got: %v", otherIDs)
	}

	// Error check on closed db
	if err := st.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if _, err := st.GetDistinctSubagentIDs(agentID); err == nil {
		t.Fatalf("expected error querying closed database, got nil")
	}
}

func TestStore_EndCurrentRun_PreservesUpdatedAt(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	now := int64(1000000)
	agent := types.IAgentState{
		ID:        "agent-preserve-updated-at",
		SessionID: "sess-preserve",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityIdle,
			FirstStartedAt: now,
			UpdatedAt:      now,
			CurrentRun: &types.IRunInfo{
				ID:        "run-1",
				PID:       1234,
				StartedAt: now,
			},
		},
	}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	// 24 hours later, the run exits
	endedAt := now + 86400000
	if err := st.EndCurrentRun(agent.ID, endedAt); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}

	afterEnd, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if afterEnd.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected lifecycle %s, got %s", types.LifecycleExited, afterEnd.Activity.Lifecycle)
	}
	if afterEnd.Activity.UpdatedAt != now {
		t.Errorf("expected UpdatedAt to be preserved at %d, but got %d (diff: %d ms)", now, afterEnd.Activity.UpdatedAt, afterEnd.Activity.UpdatedAt-now)
	}
}

func TestStore_RecordTelemetryEvent_SubagentAndCoverage(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	now := int64(1000)
	agent := types.IAgentState{
		ID:        "agent-cov",
		SessionID: "sess-cov",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			UpdatedAt: now,
		},
	}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	subID := "sub-xyz"
	tool := "bash"
	frame := types.ITelemetryFrame{
		Seq:        1,
		AgentID:    "agent-cov",
		SubagentID: &subID,
		SessionID:  "sess-cov",
		EventType:  "tool_call",
		Timestamp:  1000,
		Data: types.TelemetryFrameData{
			ToolName: &tool,
		},
	}
	if err := st.RecordTelemetryEvent(frame); err != nil {
		t.Fatalf("RecordTelemetryEvent with subagent failed: %v", err)
	}

	subIDs, err := st.GetDistinctSubagentIDs("agent-cov")
	if err != nil {
		t.Fatalf("GetDistinctSubagentIDs failed: %v", err)
	}
	if len(subIDs) != 1 || subIDs[0] != "sub-xyz" {
		t.Errorf("unexpected subIDs: %v", subIDs)
	}

	// Closed db error testing for RecordTelemetryEvent and GetAllActiveAgents
	stClosed, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	_ = stClosed.Close()
	if err := stClosed.RecordTelemetryEvent(frame); err == nil {
		t.Errorf("expected error on closed db for RecordTelemetryEvent")
	}
	if _, err := stClosed.GetAllActiveAgents(); err == nil {
		t.Errorf("expected error on closed db for GetAllActiveAgents")
	}

	// Unmarshal error test for RecordTelemetryEvent
	badFrame := frame
	badFrame.Data.ToolInput = map[string]interface{}{"bad": make(chan int)}
	if err := st.RecordTelemetryEvent(badFrame); err == nil {
		t.Errorf("expected error for unmarshalable Data in RecordTelemetryEvent")
	}


	// Test ActionStale and Cost and Phase and Hidden in SaveAgentState
	phase := types.PhasePlanning
	cost := types.ICostInfo{Total: 1.23}
	agentStale := types.IAgentState{
		ID:        "agent-stale",
		SessionID: "sess-stale",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		IsHidden: true,
		Activity: types.IAgentActivity{
			Lifecycle:   types.LifecycleRunning,
			Activity:    types.ActivityBusy,
			ActionStale: true,
			Cost:        &cost,
			Phase:       &phase,
			UpdatedAt:   now,
		},
	}
	if err := st.SaveAgentState(agentStale); err != nil {
		t.Fatalf("SaveAgentState with actionStale failed: %v", err)
	}
	gotStale, err := st.GetAgent("agent-stale")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if !gotStale.Activity.ActionStale {
		t.Errorf("expected ActionStale = true")
	}
	if gotStale.Activity.Cost == nil || gotStale.Activity.Cost.Total != 1.23 {
		t.Errorf("expected Cost = 1.23")
	}
	if gotStale.Activity.Phase == nil || *gotStale.Activity.Phase != types.PhasePlanning {
		t.Errorf("expected Phase = planning")
	}

	// Test error cases in SaveAgentState
	badPlatformAgent := types.IAgentState{
		ID: "bad-agent-platform",
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "invalid-platform",
		},
	}
	if err := st.SaveAgentState(badPlatformAgent); err == nil {
		t.Errorf("expected error with invalid platform in SaveAgentState")
	}

	badActivityAgent := types.IAgentState{
		ID: "bad-agent-activity",
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityState("INVALID"),
		},
	}
	if err := st.SaveAgentState(badActivityAgent); err == nil {
		t.Errorf("expected error with invalid activity in SaveAgentState")
	}

	rateLimitedAgent := types.IAgentState{
		ID:        "agent-rl-saved",
		SessionID: "sess-rl",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityRateLimited,
			FirstStartedAt: now,
			UpdatedAt:      now,
		},
	}
	if err := st.SaveAgentState(rateLimitedAgent); err != nil {
		t.Fatalf("expected SaveAgentState to succeed with ActivityRateLimited: %v", err)
	}
	gotRL, err := st.GetAgent("agent-rl-saved")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if gotRL.Activity.Activity != types.ActivityRateLimited {
		t.Errorf("got activity %s, want %s", gotRL.Activity.Activity, types.ActivityRateLimited)
	}

	// Test Open error with inaccessible path
	if _, err := Open("file:/nonexistent_dir/sub/test.db?mode=ro"); err == nil {
		t.Errorf("expected error opening non-existent read-only path")
	}
}

func TestStore_GetRunningAgents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()

	// 1. Running agent on env-1
	running1 := types.IAgentState{
		ID:        "agent-running-1",
		SessionID: "sess-1",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now,
			UpdatedAt:      now,
			CurrentRun: &types.IRunInfo{
				ID:        "run-1",
				PID:       1234,
				StartedAt: now,
			},
		},
	}
	if err := st.SaveAgentState(running1); err != nil {
		t.Fatalf("SaveAgentState(running1) failed: %v", err)
	}

	// 2. Running agent on env-2
	running2 := types.IAgentState{
		ID:        "agent-running-2",
		SessionID: "sess-2",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-2",
			Platform: "linux",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityIdle,
			FirstStartedAt: now,
			UpdatedAt:      now + 10,
			CurrentRun: &types.IRunInfo{
				ID:        "run-2",
				PID:       5678,
				StartedAt: now,
			},
		},
	}
	if err := st.SaveAgentState(running2); err != nil {
		t.Fatalf("SaveAgentState(running2) failed: %v", err)
	}

	// 3. Exited agent on env-1
	exited := types.IAgentState{
		ID:        "agent-exited",
		SessionID: "sess-3",
		Harness:   types.HarnessCodex,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleExited,
			Activity:       types.ActivityIdle,
			FirstStartedAt: now,
			UpdatedAt:      now - 1000,
		},
	}
	if err := st.SaveAgentState(exited); err != nil {
		t.Fatalf("SaveAgentState(exited) failed: %v", err)
	}

	// Query all running agents
	allRunning, err := st.GetRunningAgents("")
	if err != nil {
		t.Fatalf("GetRunningAgents(\"\") failed: %v", err)
	}
	if len(allRunning) != 2 {
		t.Fatalf("expected 2 running agents, got %d", len(allRunning))
	}

	// Query running agents for env-1
	env1Running, err := st.GetRunningAgents("env-1")
	if err != nil {
		t.Fatalf("GetRunningAgents(\"env-1\") failed: %v", err)
	}
	if len(env1Running) != 1 {
		t.Fatalf("expected 1 running agent for env-1, got %d", len(env1Running))
	}
	if env1Running[0].ID != "agent-running-1" {
		t.Errorf("expected agent-running-1, got %s", env1Running[0].ID)
	}

	// End run on running1
	if err := st.EndCurrentRun("agent-running-1", now+2000); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}
	env1RunningAfter, err := st.GetRunningAgents("env-1")
	if err != nil {
		t.Fatalf("GetRunningAgents after EndCurrentRun failed: %v", err)
	}
	if len(env1RunningAfter) != 0 {
		t.Errorf("expected 0 running agents for env-1 after EndCurrentRun, got %d", len(env1RunningAfter))
	}

	// Test closed DB error
	_ = st.Close()
	if _, err := st.GetRunningAgents(""); err == nil {
		t.Errorf("expected error on closed DB for GetRunningAgents")
	}
	if _, err := st.GetAllActiveAgents(); err == nil {
		t.Errorf("expected error on closed DB for GetAllActiveAgents")
	}
	if _, err := st.GetAllEnvironments(); err == nil {
		t.Errorf("expected error on closed DB for GetAllEnvironments")
	}
	if _, err := st.GetDistinctSubagentIDs("x"); err == nil {
		t.Errorf("expected error on closed DB for GetDistinctSubagentIDs")
	}
	if err := st.RecordTelemetryEvent(types.ITelemetryFrame{}); err == nil {
		t.Errorf("expected error on closed DB for RecordTelemetryEvent")
	}
	if err := st.PruneOldEventsAndExitedAgents(0, 0); err == nil {
		t.Errorf("expected error on closed DB for PruneOldEventsAndExitedAgents")
	}
	if err := Migrate(st.DB()); err == nil {
		t.Errorf("expected error in Migrate on closed DB")
	}
}

func TestStore_EndCurrentRun_WithRun(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()
	runID := "run-test-123"
	agent := types.IAgentState{
		ID:        "agent-with-run",
		SessionID: "sess-run",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-r",
			Platform: "linux",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityBusy,
			CurrentRun: &types.IRunInfo{
				ID:        runID,
				StartedAt: now - 1000,
			},
			UpdatedAt: now,
		},
	}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	endedAt := now + 5000
	if err := st.EndCurrentRun(agent.ID, endedAt); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}
	if err := st.EndCurrentRun("non-existent-agent-id-12345", endedAt); err != nil {
		t.Fatalf("EndCurrentRun on non-existent agent failed: %v", err)
	}

	loaded, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if loaded.Activity.CurrentRun != nil {
		t.Errorf("expected CurrentRun to be nil after EndCurrentRun, got %+v", loaded.Activity.CurrentRun)
	}

	// Verify run ended_at in database
	var dbEndedAt sql.NullInt64
	if err := st.DB().QueryRow("SELECT ended_at FROM runs WHERE id = ?", runID).Scan(&dbEndedAt); err != nil {
		t.Fatalf("querying run ended_at failed: %v", err)
	}
	if !dbEndedAt.Valid || dbEndedAt.Int64 != endedAt {
		t.Errorf("run ended_at = %v, want %d", dbEndedAt, endedAt)
	}
}

func TestStore_Migrate_ErrorPaths(t *testing.T) {
	// 0. Error in v1 migration (e.g. conflicting table schema)
	t.Run("v1 error on invalid schema", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec("CREATE VIEW agents AS SELECT 1;"); err != nil {
			t.Fatalf("creating conflicting view failed: %v", err)
		}
		// PRAGMA user_version is 0, so Migrate will attempt to run schemaV1 which will fail on table conflict
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v1 on conflicting table schema")
		}
	})

	// 1. Error in v2 migration (e.g. duplicate is_hidden column)
	t.Run("v2 error on duplicate column", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER;"); err != nil {
			t.Fatalf("adding duplicate column failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 1;"); err != nil {
			t.Fatalf("setting user_version = 1 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v2 on duplicate column")
		}
	})

	// Error on v2 duplicate hidden_at
	t.Run("v2 error on duplicate hidden_at", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
			t.Fatalf("pre-adding hidden_at failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v2 on duplicate hidden_at")
		}
	})

	// 2. Error in v3 migration (e.g. duplicate active_tool_started_at column)
	t.Run("v3 error on duplicate column", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
			t.Fatalf("v2 is_hidden failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
			t.Fatalf("v2 hidden_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
			t.Fatalf("adding duplicate column failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 2;"); err != nil {
			t.Fatalf("setting user_version = 2 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v3 on duplicate column")
		}
	})

	// 3. Error in v4 migration (e.g. duplicate active_subagents column)
	t.Run("v4 error on duplicate column", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
			t.Fatalf("v2 is_hidden failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
			t.Fatalf("v2 hidden_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
			t.Fatalf("v3 active_tool_started_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_subagents TEXT;"); err != nil {
			t.Fatalf("adding duplicate column failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 3;"); err != nil {
			t.Fatalf("setting user_version = 3 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v4 on duplicate column")
		}
	})

	// 4. Error in v5 migration (e.g. duplicate model column)
	t.Run("v5 error on duplicate column", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
			t.Fatalf("v2 is_hidden failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
			t.Fatalf("v2 hidden_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
			t.Fatalf("v3 active_tool_started_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_subagents TEXT;"); err != nil {
			t.Fatalf("v4 active_subagents failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN model TEXT;"); err != nil {
			t.Fatalf("adding duplicate column failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 4;"); err != nil {
			t.Fatalf("setting user_version = 4 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v5 on duplicate column")
		}
	})

	// 5. Error in v5 migration on duplicate context_window
	t.Run("v5 error on duplicate context_window", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
			t.Fatalf("v2 is_hidden failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
			t.Fatalf("v2 hidden_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
			t.Fatalf("v3 active_tool_started_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_subagents TEXT;"); err != nil {
			t.Fatalf("v4 active_subagents failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN context_window INTEGER;"); err != nil {
			t.Fatalf("adding duplicate column failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 4;"); err != nil {
			t.Fatalf("setting user_version = 4 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v5 on duplicate context_window")
		}
	})

	// 6. Error in v5 migration on duplicate context_fullness
	t.Run("v5 error on duplicate context_fullness", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
			t.Fatalf("v2 is_hidden failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
			t.Fatalf("v2 hidden_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
			t.Fatalf("v3 active_tool_started_at failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN active_subagents TEXT;"); err != nil {
			t.Fatalf("v4 active_subagents failed: %v", err)
		}
		if _, err := db.Exec("ALTER TABLE agents ADD COLUMN context_fullness REAL;"); err != nil {
			t.Fatalf("adding duplicate column failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 4;"); err != nil {
			t.Fatalf("setting user_version = 4 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v5 on duplicate context_fullness")
		}
	})

	// 7. Error in v6 migration on conflicting agents_new
	t.Run("v6 error on conflicting agents_new", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec("CREATE VIEW agents_new AS SELECT 1;"); err != nil {
			t.Fatalf("creating conflicting view failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
			t.Fatalf("setting user_version = 5 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v6 on conflicting agents_new view")
		}
	})

	// 8. Error in v6 migration on copy data failure
	t.Run("v6 error on copy data failure", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec("CREATE TABLE agents (id TEXT PRIMARY KEY, invalid_col TEXT);"); err != nil {
			t.Fatalf("creating invalid table failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
			t.Fatalf("setting user_version = 5 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v6 on copy data failure")
		}
	})

	// 9. Error in v6 migration on index conflict
	t.Run("v6 error on index conflict", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		if _, err := db.Exec(schemaV1); err != nil {
			t.Fatalf("schemaV1 failed: %v", err)
		}
		if _, err := db.Exec("DROP INDEX IF EXISTS idx_agents_project; CREATE VIEW idx_agents_project AS SELECT 1;"); err != nil {
			t.Fatalf("creating conflicting view failed: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
			t.Fatalf("setting user_version = 5 failed: %v", err)
		}
		if err := Migrate(db); err == nil {
			t.Errorf("expected error in Migrate v6 on conflicting index name")
		}
	})
}

func TestStore_ActiveToolStartedAt_PersistenceAndClearing(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := int64(1700000000000)
	toolStart := now - 50000
	toolBash := "bash"

	agent := types.IAgentState{
		ID:        "agent-tool-stall-store",
		SessionID: "sess-stall",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-stall",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:           types.LifecycleRunning,
			Activity:            types.ActivityBusy,
			ActiveTool:          &toolBash,
			ActiveToolStartedAt: &toolStart,
			FirstStartedAt:      now,
			UpdatedAt:           now,
			CurrentRun: &types.IRunInfo{
				ID:        "run-stall-1",
				PID:       9999,
				StartedAt: now,
			},
		},
	}

	// 1. Save agent state with ActiveToolStartedAt populated
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	// 2. GetAgent and verify ActiveToolStartedAt is retrieved correctly
	loaded, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if loaded.Activity.ActiveToolStartedAt == nil {
		t.Fatalf("expected ActiveToolStartedAt to be non-nil, got nil")
	}
	if *loaded.Activity.ActiveToolStartedAt != toolStart {
		t.Errorf("expected ActiveToolStartedAt = %d, got %d", toolStart, *loaded.Activity.ActiveToolStartedAt)
	}

	// 3. EndCurrentRun and verify ActiveToolStartedAt is cleared to nil
	if err := st.EndCurrentRun(agent.ID, now+10000); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}

	afterEnd, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after EndCurrentRun failed: %v", err)
	}
	if afterEnd.Activity.ActiveToolStartedAt != nil {
		t.Errorf("expected ActiveToolStartedAt to be nil after EndCurrentRun, got %v", *afterEnd.Activity.ActiveToolStartedAt)
	}

	// 4. EndCurrentRun on agent where current_run_id is already NULL
	agentNoRun := agent
	agentNoRun.ID = "agent-no-run-test"
	agentNoRun.SessionID = "sess-no-run"
	agentNoRun.Activity.CurrentRun = nil
	if err := st.SaveAgentState(agentNoRun); err != nil {
		t.Fatalf("SaveAgentState for agentNoRun failed: %v", err)
	}
	if err := st.EndCurrentRun(agentNoRun.ID, now+20000); err != nil {
		t.Fatalf("EndCurrentRun on agent without current run failed: %v", err)
	}
}

func TestStore_ActiveSubagents_Persistence(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()
	tool := "grep"
	sub := types.ISubagentSummary{
		ID:         "sub-store-1",
		Activity:   types.ActivityBusy,
		ActiveTool: &tool,
		Action:     "Running grep",
	}

	agent := types.IAgentState{
		ID:        "agent-sub-store",
		SessionID: "sess-sub-store",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-sub-store",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:       types.LifecycleRunning,
			Activity:        types.ActivityBusy,
			ActiveSubagents: []types.ISubagentSummary{sub},
			FirstStartedAt:  now,
			UpdatedAt:       now,
		},
	}

	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	loaded, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if len(loaded.Activity.ActiveSubagents) != 1 {
		t.Fatalf("expected 1 ActiveSubagent, got %d", len(loaded.Activity.ActiveSubagents))
	}
	if loaded.Activity.ActiveSubagents[0].ID != "sub-store-1" {
		t.Errorf("expected sub ID 'sub-store-1', got %q", loaded.Activity.ActiveSubagents[0].ID)
	}
	if loaded.Activity.ActiveSubagents[0].Action != "Running grep" {
		t.Errorf("expected sub Action 'Running grep', got %q", loaded.Activity.ActiveSubagents[0].Action)
	}
}

func TestStore_SaveAgentState_EndedRun_NullCurrentRunID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_ended_run.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()
	ended := now + 1000

	// 1. Initial save of an agent whose run has already ended
	agent := types.IAgentState{
		ID:        "agent-ended-run",
		SessionID: "sess-ended",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleExited,
			Activity:       types.ActivityIdle,
			FirstStartedAt: now,
			UpdatedAt:      ended,
			CurrentRun: &types.IRunInfo{
				ID:        "run-ended-1",
				PID:       9999,
				StartedAt: now,
				EndedAt:   &ended,
			},
		},
	}

	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	// Direct DB check: current_run_id MUST be NULL
	var currentRunID sql.NullString
	err = st.DB().QueryRow("SELECT current_run_id FROM agents WHERE id = ?", agent.ID).Scan(&currentRunID)
	if err != nil {
		t.Fatalf("querying current_run_id: %v", err)
	}
	if currentRunID.Valid {
		t.Errorf("expected current_run_id to be NULL for ended run, got %q", currentRunID.String)
	}

	// 2. Active run updated to ended run via SaveAgentState
	activeAgent := types.IAgentState{
		ID:        "agent-active-then-ended",
		SessionID: "sess-active",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now,
			UpdatedAt:      now,
			CurrentRun: &types.IRunInfo{
				ID:        "run-active-1",
				PID:       8888,
				StartedAt: now,
				EndedAt:   nil,
			},
		},
	}
	if err := st.SaveAgentState(activeAgent); err != nil {
		t.Fatalf("SaveAgentState(active) failed: %v", err)
	}

	// Verify current_run_id is non-null while active
	err = st.DB().QueryRow("SELECT current_run_id FROM agents WHERE id = ?", activeAgent.ID).Scan(&currentRunID)
	if err != nil {
		t.Fatalf("querying current_run_id: %v", err)
	}
	if !currentRunID.Valid || currentRunID.String != "run-active-1" {
		t.Errorf("expected current_run_id = 'run-active-1', got %v", currentRunID)
	}

	// Now update to ended
	activeAgent.Activity.Lifecycle = types.LifecycleExited
	activeAgent.Activity.Activity = types.ActivityIdle
	activeAgent.Activity.CurrentRun.EndedAt = &ended
	if err := st.SaveAgentState(activeAgent); err != nil {
		t.Fatalf("SaveAgentState(ended update) failed: %v", err)
	}

	err = st.DB().QueryRow("SELECT current_run_id FROM agents WHERE id = ?", activeAgent.ID).Scan(&currentRunID)
	if err != nil {
		t.Fatalf("querying current_run_id after end: %v", err)
	}
	if currentRunID.Valid {
		t.Errorf("expected current_run_id to be NULL after run ended, got %q", currentRunID.String)
	}
}

func TestStore_GetRunningAgents_IgnoresEndedRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_running_ended.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()
	ended := now + 500

	// Insert an agent with an ended run where current_run_id points to an ended run
	_, err = st.DB().Exec(`INSERT INTO environments (id, label, hostname, platform, user, last_heartbeat_at) VALUES ('env-1', 'local', 'localhost', 'darwin', 'user', ?)`, now)
	if err != nil {
		t.Fatalf("inserting environment: %v", err)
	}
	_, err = st.DB().Exec(`INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at) VALUES ('run-1', 'agent-1', 1111, '1.0.0', ?, ?)`, now, ended)
	if err != nil {
		t.Fatalf("inserting run: %v", err)
	}
	_, err = st.DB().Exec(`
		INSERT INTO agents (id, environment_id, session_id, harness, project_id, project_name, canonical_root, branch, cwd, first_started_at, updated_at, activity, current_run_id)
		VALUES ('agent-1', 'env-1', 'sess-1', 'pi', 'proj-1', 'proj', '/tmp', 'main', '/tmp', ?, ?, 'BUSY', 'run-1')
	`, now, now)
	if err != nil {
		t.Fatalf("inserting agent: %v", err)
	}

	// GetRunningAgents must NOT return agent-1 because run-1 has ended_at set
	running, err := st.GetRunningAgents("")
	if err != nil {
		t.Fatalf("GetRunningAgents failed: %v", err)
	}
	if len(running) != 0 {
		t.Errorf("expected 0 running agents because run is ended, got %d", len(running))
	}

	// Calling GetAgent on an agent with an ended run exercises the historical run branch
	agentWithEndedRun, err := st.GetAgent("agent-1")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agentWithEndedRun == nil {
		t.Fatalf("expected non-nil agent")
	}
	if agentWithEndedRun.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected LifecycleExited, got %v", agentWithEndedRun.Activity.Lifecycle)
	}
	if agentWithEndedRun.Activity.CurrentRun == nil || agentWithEndedRun.Activity.CurrentRun.EndedAt == nil {
		t.Errorf("expected CurrentRun populated with ended run, got %+v", agentWithEndedRun.Activity.CurrentRun)
	}
}

func TestStore_GetAgent_ExitedWithHistoricalRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_exited_get.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UnixMilli()
	ended := now + 1000

	agent := types.IAgentState{
		ID:        "agent-get-exited",
		SessionID: "sess-get-exited",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: now,
			UpdatedAt:      now,
			CurrentRun: &types.IRunInfo{
				ID:             "run-to-end",
				PID:            1234,
				HarnessVersion: "1.0.0",
				StartedAt:      now,
			},
		},
	}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	// End current run
	if err := st.EndCurrentRun(agent.ID, ended); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}

	// Fetch via GetAgent
	fetched, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if fetched == nil {
		t.Fatalf("expected non-nil agent")
	}
	if fetched.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected LifecycleExited, got %s", fetched.Activity.Lifecycle)
	}
	if fetched.Activity.CurrentRun != nil {
		t.Errorf("expected CurrentRun nil for ended agent without current_run_id, got %+v", fetched.Activity.CurrentRun)
	}

	// Also test EndCurrentRun when current_run_id is already NULL
	if err := st.EndCurrentRun(agent.ID, ended+500); err != nil {
		t.Errorf("EndCurrentRun on already ended agent failed: %v", err)
	}

	// Test EndCurrentRun when agent does not exist (ErrNoRows)
	if err := st.EndCurrentRun("non-existent-agent", ended+1000); err != nil {
		t.Errorf("EndCurrentRun on non-existent agent failed: %v", err)
	}
}


