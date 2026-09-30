package store

import (
	"database/sql"
	"strings"
	"testing"
)

func TestSchemaV1_OriginalV1Activities(t *testing.T) {
	// schemaV1 must only contain the original 4 activity states:
	// 'BUSY', 'WAITING_QUESTION', 'ERROR', 'IDLE'
	// RATE_LIMITED was introduced in migration v6.
	if strings.Contains(schemaV1, "'RATE_LIMITED'") {
		t.Errorf("schemaV1 must not contain 'RATE_LIMITED'; historical migration DDL must remain immutable")
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("executing schemaV1: %v", err)
	}

	// In schemaV1, attempting to insert an agent with RATE_LIMITED must fail the CHECK constraint
	_, err = db.Exec(`
		INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
		VALUES ('env-v1-check', 'Local', 'host', 'linux', 'user', 0, 1000);
		INSERT INTO agents (
			id, session_id, environment_id, harness, project_id, project_name,
			canonical_root, branch, cwd, first_started_at, updated_at, activity
		) VALUES (
			'a1', 's1', 'env-v1-check', 'claude-code', 'p1', 'p1',
			'/repo', 'main', '/repo', 1000, 1000, 'RATE_LIMITED'
		);
	`)
	if err == nil {
		t.Errorf("expected CHECK constraint failure inserting RATE_LIMITED into schemaV1, got nil")
	}
}

func TestTableColumns(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Nonexistent table returns empty map without error
	cols, err := tableColumns(tx, "nonexistent")
	if err != nil {
		t.Fatalf("tableColumns on nonexistent table failed: %v", err)
	}
	if len(cols) != 0 {
		t.Errorf("expected 0 columns, got %d", len(cols))
	}

	// Query error on invalid table syntax
	if _, err := tableColumns(tx, "'unclosed_quote"); err == nil {
		t.Errorf("expected error from tableColumns on syntax error, got nil")
	}

	// Create table and check column detection
	if _, err := tx.Exec("CREATE TABLE test_tab (id INT PRIMARY KEY, name TEXT, opt REAL DEFAULT 1.0);"); err != nil {
		t.Fatalf("create test_tab failed: %v", err)
	}

	cols, err = tableColumns(tx, "test_tab")
	if err != nil {
		t.Fatalf("tableColumns on test_tab failed: %v", err)
	}
	if !cols["id"] || !cols["name"] || !cols["opt"] {
		t.Errorf("expected id, name, opt in cols, got %v", cols)
	}
	if cols["missing_column"] {
		t.Errorf("unexpected missing_column reported as present in cols")
	}
}

func TestMigrate_CrashAfterV7PartialCommit(t *testing.T) {
	// Simulate crash after partial commit:
	// Database is at v6, but v7 columns were already added to the agents table
	// while user_version remained 6.
	// Migrate(db) must succeed, skip duplicate column addition idempotently,
	// and bring user_version to 8 with tables intact.
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize database to v6 schema
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	for _, col := range []string{
		"ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;",
		"ALTER TABLE agents ADD COLUMN hidden_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_subagents TEXT;",
		"ALTER TABLE agents ADD COLUMN model TEXT;",
		"ALTER TABLE agents ADD COLUMN context_window INTEGER;",
		"ALTER TABLE agents ADD COLUMN context_fullness REAL;",
	} {
		if _, err := db.Exec(col); err != nil {
			t.Fatalf("adding col failed: %v", err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
		t.Fatalf("setting user_version = 5 failed: %v", err)
	}
	if err := migrateV6(db); err != nil {
		t.Fatalf("migrateV6 failed: %v", err)
	}

	// Verify database is at v6
	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 6 {
		t.Fatalf("expected initial user_version = 6, got %d", version)
	}

	// Insert existing agent data at v6
	if _, err := db.Exec(`
		INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
		VALUES ('env-test-v7', 'Local', 'host', 'linux', 'user', 0, 1000);
		INSERT INTO agents (
			id, session_id, environment_id, harness, project_id, project_name,
			canonical_root, branch, cwd, first_started_at, updated_at, activity
		) VALUES (
			'agent-v7', 'sess-v7', 'env-test-v7', 'claude-code', 'p1', 'p1',
			'/repo', 'main', '/repo', 1000, 1000, 'BUSY'
		);
	`); err != nil {
		t.Fatalf("inserting seed data failed: %v", err)
	}

	// Manually apply v7 columns with user_version remaining 6 (simulating a crash)
	if _, err := db.Exec(`
		ALTER TABLE agents ADD COLUMN rate_limit_reset_at INTEGER;
		ALTER TABLE agents ADD COLUMN retry_after_seconds INTEGER;
	`); err != nil {
		t.Fatalf("manually applying v7 columns failed: %v", err)
	}

	// Migrate must succeed and advance to version 8
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed after simulated v7 partial crash: %v", err)
	}

	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 8 {
		t.Fatalf("expected user_version = 8, got %d", version)
	}

	// Assert tables and existing data are intact
	var agentID, activity string
	var resetAt, retryAfter sql.NullInt64
	var unseenComp int
	var compAt sql.NullInt64
	err = db.QueryRow(`
		SELECT id, activity, rate_limit_reset_at, retry_after_seconds, has_unseen_completion, completed_at
		FROM agents WHERE id = 'agent-v7';
	`).Scan(&agentID, &activity, &resetAt, &retryAfter, &unseenComp, &compAt)
	if err != nil {
		t.Fatalf("querying agent failed: %v", err)
	}
	if agentID != "agent-v7" || activity != "BUSY" || unseenComp != 0 {
		t.Errorf("unexpected agent data: id=%s, activity=%s, unseenComp=%d", agentID, activity, unseenComp)
	}

	// Verify updates to both v7 and v8 columns work
	if _, err := db.Exec(`
		UPDATE agents
		SET rate_limit_reset_at = 1700000000,
		    retry_after_seconds = 45,
		    has_unseen_completion = 1,
		    completed_at = 1700000010
		WHERE id = 'agent-v7';
	`); err != nil {
		t.Fatalf("updating agent with v7 and v8 fields failed: %v", err)
	}

	err = db.QueryRow(`
		SELECT rate_limit_reset_at, retry_after_seconds, has_unseen_completion, completed_at
		FROM agents WHERE id = 'agent-v7';
	`).Scan(&resetAt, &retryAfter, &unseenComp, &compAt)
	if err != nil {
		t.Fatalf("querying updated agent failed: %v", err)
	}
	if !resetAt.Valid || resetAt.Int64 != 1700000000 {
		t.Errorf("expected rate_limit_reset_at = 1700000000, got %v", resetAt)
	}
	if !retryAfter.Valid || retryAfter.Int64 != 45 {
		t.Errorf("expected retry_after_seconds = 45, got %v", retryAfter)
	}
	if unseenComp != 1 {
		t.Errorf("expected has_unseen_completion = 1, got %d", unseenComp)
	}
	if !compAt.Valid || compAt.Int64 != 1700000010 {
		t.Errorf("expected completed_at = 1700000010, got %v", compAt)
	}
}

func TestMigrate_CrashAfterV7SingleColumnCommit(t *testing.T) {
	// Simulate crash when only the first column of v7 was committed
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	for _, col := range []string{
		"ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;",
		"ALTER TABLE agents ADD COLUMN hidden_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_subagents TEXT;",
		"ALTER TABLE agents ADD COLUMN model TEXT;",
		"ALTER TABLE agents ADD COLUMN context_window INTEGER;",
		"ALTER TABLE agents ADD COLUMN context_fullness REAL;",
	} {
		if _, err := db.Exec(col); err != nil {
			t.Fatalf("adding col failed: %v", err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
		t.Fatalf("setting user_version = 5 failed: %v", err)
	}
	if err := migrateV6(db); err != nil {
		t.Fatalf("migrateV6 failed: %v", err)
	}

	// Apply only rate_limit_reset_at with user_version = 6
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN rate_limit_reset_at INTEGER;"); err != nil {
		t.Fatalf("manually applying single v7 column failed: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 8 {
		t.Fatalf("expected user_version = 8, got %d", version)
	}
}

func TestMigrate_CrashAfterV8PartialCommit(t *testing.T) {
	// Simulate crash after partial commit:
	// Database is at v7, but v8 columns were already added to the agents table
	// while user_version remained 7.
	// Migrate(db) must succeed, skip duplicate column addition idempotently,
	// and bring user_version to 8 with tables intact.
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Apply schema up to v6
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	for _, col := range []string{
		"ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;",
		"ALTER TABLE agents ADD COLUMN hidden_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_subagents TEXT;",
		"ALTER TABLE agents ADD COLUMN model TEXT;",
		"ALTER TABLE agents ADD COLUMN context_window INTEGER;",
		"ALTER TABLE agents ADD COLUMN context_fullness REAL;",
	} {
		if _, err := db.Exec(col); err != nil {
			t.Fatalf("adding col failed: %v", err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
		t.Fatalf("setting user_version = 5 failed: %v", err)
	}
	if err := migrateV6(db); err != nil {
		t.Fatalf("migrateV6 failed: %v", err)
	}

	// Apply v7 columns and set user_version = 7
	if _, err := db.Exec(`
		ALTER TABLE agents ADD COLUMN rate_limit_reset_at INTEGER;
		ALTER TABLE agents ADD COLUMN retry_after_seconds INTEGER;
		PRAGMA user_version = 7;
	`); err != nil {
		t.Fatalf("setting up v7 failed: %v", err)
	}

	// Insert test data
	if _, err := db.Exec(`
		INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
		VALUES ('env-test-v8', 'Local', 'host', 'linux', 'user', 0, 1000);
		INSERT INTO agents (
			id, session_id, environment_id, harness, project_id, project_name,
			canonical_root, branch, cwd, first_started_at, updated_at, activity
		) VALUES (
			'agent-v8', 'sess-v8', 'env-test-v8', 'claude-code', 'p1', 'p1',
			'/repo', 'main', '/repo', 1000, 1000, 'BUSY'
		);
	`); err != nil {
		t.Fatalf("inserting seed data failed: %v", err)
	}

	// Manually apply v8 columns with user_version remaining 7 (simulating a crash)
	if _, err := db.Exec(`
		ALTER TABLE agents ADD COLUMN has_unseen_completion INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE agents ADD COLUMN completed_at INTEGER;
	`); err != nil {
		t.Fatalf("manually applying v8 columns failed: %v", err)
	}

	// Migrate must succeed and advance to version 8
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed after simulated v8 partial crash: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 8 {
		t.Fatalf("expected user_version = 8, got %d", version)
	}

	// Assert tables and existing data are intact
	var agentID, activity string
	var unseenComp int
	var compAt sql.NullInt64
	err = db.QueryRow(`
		SELECT id, activity, has_unseen_completion, completed_at
		FROM agents WHERE id = 'agent-v8';
	`).Scan(&agentID, &activity, &unseenComp, &compAt)
	if err != nil {
		t.Fatalf("querying agent failed: %v", err)
	}
	if agentID != "agent-v8" || activity != "BUSY" || unseenComp != 0 {
		t.Errorf("unexpected agent data: id=%s, activity=%s, unseenComp=%d", agentID, activity, unseenComp)
	}

	// Verify updates to v8 columns work
	if _, err := db.Exec(`
		UPDATE agents
		SET has_unseen_completion = 1,
		    completed_at = 1700000020
		WHERE id = 'agent-v8';
	`); err != nil {
		t.Fatalf("updating agent with v8 fields failed: %v", err)
	}

	err = db.QueryRow(`
		SELECT has_unseen_completion, completed_at
		FROM agents WHERE id = 'agent-v8';
	`).Scan(&unseenComp, &compAt)
	if err != nil {
		t.Fatalf("querying updated agent failed: %v", err)
	}
	if unseenComp != 1 {
		t.Errorf("expected has_unseen_completion = 1, got %d", unseenComp)
	}
	if !compAt.Valid || compAt.Int64 != 1700000020 {
		t.Errorf("expected completed_at = 1700000020, got %v", compAt)
	}
}

func TestMigrate_CrashAfterV8SingleColumnCommit(t *testing.T) {
	// Simulate crash when only the first column of v8 was committed
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("schemaV1 failed: %v", err)
	}
	for _, col := range []string{
		"ALTER TABLE agents ADD COLUMN is_hidden INTEGER NOT NULL DEFAULT 0;",
		"ALTER TABLE agents ADD COLUMN hidden_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_tool_started_at INTEGER;",
		"ALTER TABLE agents ADD COLUMN active_subagents TEXT;",
		"ALTER TABLE agents ADD COLUMN model TEXT;",
		"ALTER TABLE agents ADD COLUMN context_window INTEGER;",
		"ALTER TABLE agents ADD COLUMN context_fullness REAL;",
	} {
		if _, err := db.Exec(col); err != nil {
			t.Fatalf("adding col failed: %v", err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 5;"); err != nil {
		t.Fatalf("setting user_version = 5 failed: %v", err)
	}
	if err := migrateV6(db); err != nil {
		t.Fatalf("migrateV6 failed: %v", err)
	}

	if _, err := db.Exec(`
		ALTER TABLE agents ADD COLUMN rate_limit_reset_at INTEGER;
		ALTER TABLE agents ADD COLUMN retry_after_seconds INTEGER;
		PRAGMA user_version = 7;
	`); err != nil {
		t.Fatalf("setting up v7 failed: %v", err)
	}

	// Apply only has_unseen_completion
	if _, err := db.Exec("ALTER TABLE agents ADD COLUMN has_unseen_completion INTEGER NOT NULL DEFAULT 0;"); err != nil {
		t.Fatalf("manually applying single v8 column failed: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != 8 {
		t.Fatalf("expected user_version = 8, got %d", version)
	}
}

func TestMigrate_RollbackOnErrorPreservesUserVersion(t *testing.T) {
	// When a migration fails, the transaction must roll back and user_version
	// must remain at the pre-migration value, not incremented.

	t.Run("rollback preserves user_version at 0 on v1 failure", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		// Create a view named agents which causes schemaV1 CREATE TABLE agents to fail
		if _, err := db.Exec("CREATE VIEW agents AS SELECT 1 AS id;"); err != nil {
			t.Fatalf("creating view failed: %v", err)
		}

		err = Migrate(db)
		if err == nil {
			t.Fatalf("expected Migrate to fail on table/view conflict, got nil")
		}

		var version int
		if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
			t.Fatalf("reading user_version: %v", err)
		}
		if version != 0 {
			t.Errorf("expected user_version = 0 after rollback, got %d", version)
		}
	})

	t.Run("rollback preserves user_version at 6 on v7 failure", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		// Set user_version = 6 and create agents as a view so ALTER TABLE fails
		if _, err := db.Exec("CREATE VIEW agents AS SELECT 1 AS id; PRAGMA user_version = 6;"); err != nil {
			t.Fatalf("creating view failed: %v", err)
		}

		err = Migrate(db)
		if err == nil {
			t.Fatalf("expected Migrate to fail on altering view, got nil")
		}

		var version int
		if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
			t.Fatalf("reading user_version: %v", err)
		}
		if version != 6 {
			t.Errorf("expected user_version = 6 after rollback, got %d", version)
		}
	})

	t.Run("rollback preserves user_version at 7 on v8 failure", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer func() { _ = db.Close() }()

		// Set user_version = 7 and create agents as a view so ALTER TABLE fails
		if _, err := db.Exec("CREATE VIEW agents AS SELECT 1 AS id; PRAGMA user_version = 7;"); err != nil {
			t.Fatalf("creating view failed: %v", err)
		}

		err = Migrate(db)
		if err == nil {
			t.Fatalf("expected Migrate to fail on altering view, got nil")
		}

		var version int
		if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
			t.Fatalf("reading user_version: %v", err)
		}
		if version != 7 {
			t.Errorf("expected user_version = 7 after rollback, got %d", version)
		}
	})
}

func TestMigrate_UserVersionTransactionalRollback(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify SQLite PRAGMA user_version is transactional and rolls back with the transaction
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if _, err := tx.Exec("PRAGMA user_version = 7;"); err != nil {
		t.Fatalf("executing PRAGMA user_version inside tx failed: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("querying user_version failed: %v", err)
	}
	if version != 0 {
		t.Errorf("expected user_version to remain 0 after rollback, got %d", version)
	}
}
