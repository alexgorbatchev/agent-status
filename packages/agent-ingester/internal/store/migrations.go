package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const schemaV1 = `
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

CREATE INDEX IF NOT EXISTS idx_agents_project     ON agents(project_id);
CREATE INDEX IF NOT EXISTS idx_agents_environment ON agents(environment_id);
CREATE INDEX IF NOT EXISTS idx_agents_activity    ON agents(activity);
CREATE INDEX IF NOT EXISTS idx_agents_updated     ON agents(updated_at);
CREATE INDEX IF NOT EXISTS idx_runs_agent         ON runs(agent_id);

CREATE TABLE IF NOT EXISTS telemetry_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    subagent_id   TEXT,
    event_type    TEXT NOT NULL,
    timestamp     INTEGER NOT NULL,
    data_json     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_agent_ts ON telemetry_events(agent_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_subagent ON telemetry_events(agent_id, subagent_id);
CREATE INDEX IF NOT EXISTS idx_events_ts       ON telemetry_events(timestamp);

CREATE TABLE IF NOT EXISTS observer_cursors (
    environment_id   TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    last_seq         INTEGER NOT NULL,
    PRIMARY KEY (environment_id)
);
`

func Migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		return fmt.Errorf("reading user_version: %w", err)
	}

	if version < 1 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if _, err := tx.Exec(schemaV1); err != nil {
			return fmt.Errorf("executing migration v1: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration v1: %w", err)
		}

		if _, err := db.Exec("PRAGMA user_version = 1;"); err != nil {
			return fmt.Errorf("updating user_version to 1: %w", err)
		}
		version = 1
	}

	if version < 2 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration v2 transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;"); err != nil {
			return fmt.Errorf("executing migration v2 is_hidden: %w", err)
		}
		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN hidden_at INTEGER;"); err != nil {
			return fmt.Errorf("executing migration v2 hidden_at: %w", err)
		}
		if _, err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_agents_hidden ON agents(is_hidden);"); err != nil {
			return fmt.Errorf("executing migration v2 idx_agents_hidden: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration v2: %w", err)
		}

		if _, err := db.Exec("PRAGMA user_version = 2;"); err != nil {
			return fmt.Errorf("updating user_version to 2: %w", err)
		}
		version = 2
	}

	if version < 3 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration v3 transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;"); err != nil {
			return fmt.Errorf("executing migration v3 active_tool_started_at: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration v3: %w", err)
		}

		if _, err := db.Exec("PRAGMA user_version = 3;"); err != nil {
			return fmt.Errorf("updating user_version to 3: %w", err)
		}
		version = 3
	}

	if version < 4 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration v4 transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN active_subagents TEXT;"); err != nil {
			return fmt.Errorf("executing migration v4 active_subagents: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration v4: %w", err)
		}

		if _, err := db.Exec("PRAGMA user_version = 4;"); err != nil {
			return fmt.Errorf("updating user_version to 4: %w", err)
		}
		version = 4
	}

	if version < 5 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration v5 transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN model TEXT;"); err != nil {
			return fmt.Errorf("executing migration v5 model: %w", err)
		}
		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN context_window INTEGER;"); err != nil {
			return fmt.Errorf("executing migration v5 context_window: %w", err)
		}
		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN context_fullness REAL;"); err != nil {
			return fmt.Errorf("executing migration v5 context_fullness: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration v5: %w", err)
		}

		if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
			return fmt.Errorf("updating user_version to 5: %w", err)
		}
		version = 5
	}

	if version < 6 {
		if err := migrateV6(db); err != nil {
			return err
		}
		version = 6
	}

	if version < 7 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration v7 transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN rate_limit_reset_at INTEGER;"); err != nil {
			return fmt.Errorf("executing migration v7 rate_limit_reset_at: %w", err)
		}
		if _, err := tx.Exec("ALTER TABLE agents ADD COLUMN retry_after_seconds INTEGER;"); err != nil {
			return fmt.Errorf("executing migration v7 retry_after_seconds: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration v7: %w", err)
		}

		if _, err := db.Exec("PRAGMA user_version = 7;"); err != nil {
			return fmt.Errorf("updating user_version to 7: %w", err)
		}
		version = 7
	}

	return nil
}

func migrateV6(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("getting connection for migration v6: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	fkReenabled := false
	defer func() {
		if !fkReenabled {
			_, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;")
		}
	}()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF;"); err != nil {
		return fmt.Errorf("disabling foreign keys for migration v6: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning migration v6 transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const createAgentsV6 = `
CREATE TABLE agents_new (
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
	if _, err := tx.ExecContext(ctx, createAgentsV6); err != nil {
		return fmt.Errorf("creating agents_new table for migration v6: %w", err)
	}

	const copyAgentsV6 = `
INSERT INTO agents_new (
    id, session_id, environment_id, harness,
    project_id, project_name, canonical_root,
    branch, is_detached, is_worktree, worktree_rel_path, cwd,
    current_run_id, activity, title, action, action_stale,
    phase, active_tool, active_target, active_tool_started_at, question,
    last_thinking_excerpt, last_error,
    tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
    tokens_reasoning, tokens_total,
    cost_usd, subagent_count, active_subagents,
    first_started_at, updated_at,
    is_hidden, hidden_at,
    model, context_window, context_fullness
)
SELECT
    id, session_id, environment_id, harness,
    project_id, project_name, canonical_root,
    branch, is_detached, is_worktree, worktree_rel_path, cwd,
    current_run_id, activity, title, action, action_stale,
    phase, active_tool, active_target, active_tool_started_at, question,
    last_thinking_excerpt, last_error,
    tokens_input, tokens_output, tokens_cache_read, tokens_cache_write,
    tokens_reasoning, tokens_total,
    cost_usd, subagent_count, active_subagents,
    first_started_at, updated_at,
    is_hidden, hidden_at,
    model, context_window, context_fullness
FROM agents;
`
	if _, err := tx.ExecContext(ctx, copyAgentsV6); err != nil {
		return fmt.Errorf("copying data to agents_new for migration v6: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DROP TABLE agents;"); err != nil {
		return fmt.Errorf("dropping old agents table for migration v6: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "ALTER TABLE agents_new RENAME TO agents;"); err != nil {
		return fmt.Errorf("renaming agents_new to agents for migration v6: %w", err)
	}

	const recreateIndexesV6 = `
CREATE INDEX IF NOT EXISTS idx_agents_project     ON agents(project_id);
CREATE INDEX IF NOT EXISTS idx_agents_environment ON agents(environment_id);
CREATE INDEX IF NOT EXISTS idx_agents_activity    ON agents(activity);
CREATE INDEX IF NOT EXISTS idx_agents_updated     ON agents(updated_at);
CREATE INDEX IF NOT EXISTS idx_agents_hidden      ON agents(is_hidden);
`
	if _, err := tx.ExecContext(ctx, recreateIndexesV6); err != nil {
		return fmt.Errorf("recreating indexes for migration v6: %w", err)
	}

	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return fmt.Errorf("checking foreign keys for migration v6: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var fkViolations []string
	for rows.Next() {
		var table, parent string
		var rowid any
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("scanning foreign key check row for migration v6: %w", err)
		}
		fkViolations = append(fkViolations, fmt.Sprintf("table %s (rowid %v, parent %s)", table, rowid, parent))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating foreign key check rows for migration v6: %w", err)
	}
	_ = rows.Close()

	if len(fkViolations) > 0 {
		return fmt.Errorf("foreign key check failed for migration v6: %s", strings.Join(fkViolations, "; "))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration v6: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("re-enabling foreign keys after migration v6: %w", err)
	}
	fkReenabled = true

	if _, err := conn.ExecContext(ctx, "PRAGMA user_version = 6;"); err != nil {
		return fmt.Errorf("updating user_version to 6: %w", err)
	}

	return nil
}
