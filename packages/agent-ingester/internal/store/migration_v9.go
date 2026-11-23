package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// migrateV9 rebuilds the agents table to admit Antigravity without changing
// historical migration DDL. Foreign-key mode belongs to a connection, so the
// entire rebuild, integrity check, and restoration use one pinned connection.
func migrateV9(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("getting connection for migration v9: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF;"); err != nil {
		return fmt.Errorf("disabling foreign keys for migration v9: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;") }()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning migration v9: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var ddl string
	if err := tx.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'agents'").Scan(&ddl); err != nil {
		return fmt.Errorf("reading agents schema for migration v9: %w", err)
	}
	const oldConstraint = "harness IN ('claude-code','pi','codex','opencode')"
	if !strings.Contains(ddl, oldConstraint) {
		return fmt.Errorf("migration v9: agents harness constraint does not match schema v8")
	}
	// Preserve column order, defaults, and all unrelated constraints verbatim.
	ddl = "CREATE TABLE agents_new " + ddl[strings.IndexByte(ddl, '('):]
	ddl = strings.Replace(ddl, oldConstraint, "harness IN ('claude-code','pi','codex','opencode','agy')", 1)
	rows, err := tx.QueryContext(ctx, "SELECT sql FROM sqlite_schema WHERE tbl_name = 'agents' AND type IN ('index', 'trigger') AND sql IS NOT NULL")
	if err != nil {
		return fmt.Errorf("reading agents indexes for migration v9: %w", err)
	}
	var objects []string
	for rows.Next() {
		var statement string
		if err := rows.Scan(&statement); err != nil {
			_ = rows.Close()
			return fmt.Errorf("reading agents schema object for migration v9: %w", err)
		}
		objects = append(objects, statement)
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return fmt.Errorf("reading agents schema objects for migration v9: %w", readErr)
	}
	statements := append([]string{ddl, "INSERT INTO agents_new SELECT * FROM agents", "DROP TABLE agents", "ALTER TABLE agents_new RENAME TO agents"}, objects...)
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuilding agents for migration v9: %w", err)
		}
	}
	violations, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return fmt.Errorf("checking foreign keys for migration v9: %w", err)
	}
	hasViolation := violations.Next()
	checkErr := violations.Err()
	_ = violations.Close()
	if checkErr != nil {
		return fmt.Errorf("reading foreign key check for migration v9: %w", checkErr)
	}
	if hasViolation {
		return fmt.Errorf("foreign key check failed for migration v9")
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 9;"); err != nil {
		return fmt.Errorf("updating user_version to 9: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration v9: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("re-enabling foreign keys after migration v9: %w", err)
	}
	return nil
}
