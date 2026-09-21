package replay

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestLoadSession_EdgeCases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("unsupported harness", func(t *testing.T) {
		f := filepath.Join(tmpDir, "dummy.txt")
		_ = os.WriteFile(f, []byte("test"), 0644)
		_, err := LoadSession(ctx, f, types.HarnessType("unsupported_harness"), "", "")
		if err == nil {
			t.Errorf("expected error for unsupported harness")
		}
	})

	t.Run("auto-detect failure", func(t *testing.T) {
		f := filepath.Join(tmpDir, "unknown.xyz")
		_ = os.WriteFile(f, []byte("completely random text"), 0644)
		_, err := LoadSession(ctx, f, "", "", "")
		if err == nil {
			t.Errorf("expected error when auto-detection fails")
		}
	})

	t.Run("opencode empty database", func(t *testing.T) {
		dbFile := filepath.Join(tmpDir, "empty_opencode.db")
		db, err := sql.Open("sqlite3", "file:"+dbFile)
		if err != nil {
			t.Fatalf("opening sqlite: %v", err)
		}
		schema := `
			CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL);
			CREATE TABLE session (id TEXT PRIMARY KEY, project_id TEXT, directory TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL);
			CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
			CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
		`
		_, _ = db.Exec(schema)
		_ = db.Close()

		_, err = LoadSession(ctx, dbFile, types.HarnessOpencode, "", "")
		if err == nil {
			t.Errorf("expected error when opencode database has 0 sessions")
		}
	})

	t.Run("opencode non-existent session ID", func(t *testing.T) {
		dbFile := filepath.Join(tmpDir, "single_sess.db")
		db, err := sql.Open("sqlite3", "file:"+dbFile)
		if err != nil {
			t.Fatalf("opening sqlite: %v", err)
		}
		schema := `
			CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL);
			CREATE TABLE session (id TEXT PRIMARY KEY, project_id TEXT, directory TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL);
			CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
			CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
		`
		_, _ = db.Exec(schema)
		_, _ = db.Exec(`INSERT INTO session (id, directory, time_created, time_updated) VALUES ('s1', '/repo', 1000, 2000)`)
		_ = db.Close()

		_, err = LoadSession(ctx, dbFile, types.HarnessOpencode, "missing_session_id", "")
		if err == nil {
			t.Errorf("expected error for non-existent session ID")
		}
	})

	t.Run("ensureSyntheticLifecycle with empty events", func(t *testing.T) {
		res := ensureSyntheticLifecycle(nil, 1000, 2000, "/repo")
		if len(res) != 0 {
			t.Errorf("expected 0 events, got %d", len(res))
		}
	})

	t.Run("readJSONLLines with missing file", func(t *testing.T) {
		_, _, err := readJSONLLines(filepath.Join(tmpDir, "does-not-exist.jsonl"), nil)
		if err == nil {
			t.Errorf("expected error for missing file")
		}
	})
}
