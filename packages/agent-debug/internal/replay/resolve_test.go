package replay

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestResolveSessionTarget(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("empty target returns error", func(t *testing.T) {
		_, err := ResolveSessionTarget(ctx, "   ", "")
		if err == nil {
			t.Errorf("expected error for empty target")
		}
	})

	t.Run("direct file path resolution", func(t *testing.T) {
		p := filepath.Join(tmpDir, "direct-file.jsonl")
		content := `{"type":"session","id":"direct-sess","cwd":"/repo"}` + "\n"
		_ = os.WriteFile(p, []byte(content), 0644)

		res, err := ResolveSessionTarget(ctx, p, "")
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if res.FilePath != p {
			t.Errorf("expected FilePath %s, got %s", p, res.FilePath)
		}
		if res.Harness != types.HarnessPi {
			t.Errorf("expected harness Pi, got %s", res.Harness)
		}
	})

	t.Run("directory path resolution", func(t *testing.T) {
		d := filepath.Join(tmpDir, "sess-dir")
		_ = os.MkdirAll(d, 0755)
		inner := filepath.Join(d, "inner.jsonl")
		_ = os.WriteFile(inner, []byte(`{"type":"session","id":"s1","cwd":"/repo"}`+"\n"), 0644)

		res, err := ResolveSessionTarget(ctx, d, "")
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if res.FilePath != inner {
			t.Errorf("expected inner file %s, got %s", inner, res.FilePath)
		}
	})

	t.Run("resolve session ID in Pi dir", func(t *testing.T) {
		piDir := filepath.Join(tmpDir, "pi-sessions")
		_ = os.MkdirAll(piDir, 0755)
		sessFile := filepath.Join(piDir, "pi-uuid-12345.jsonl")
		_ = os.WriteFile(sessFile, []byte(`{"type":"session","id":"pi-uuid-12345","cwd":"/repo"}`+"\n"), 0644)

		orig := os.Getenv("PI_SESSIONS_DIR")
		_ = os.Setenv("PI_SESSIONS_DIR", piDir)
		defer func() { _ = os.Setenv("PI_SESSIONS_DIR", orig) }()

		res, err := ResolveSessionTarget(ctx, "pi-uuid-12345", types.HarnessPi)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if res.FilePath != sessFile {
			t.Errorf("expected %s, got %s", sessFile, res.FilePath)
		}
		if res.SessionID != "pi-uuid-12345" {
			t.Errorf("expected session ID pi-uuid-12345, got %s", res.SessionID)
		}
	})

	t.Run("resolve session ID in Codex dir", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex-sessions")
		_ = os.MkdirAll(codexDir, 0755)
		sessFile := filepath.Join(codexDir, "rollout-cdx-999.jsonl")
		_ = os.WriteFile(sessFile, []byte(`{"type":"session_meta","timestamp":"2026-09-21T10:00:00Z"}`+"\n"), 0644)

		orig := os.Getenv("CODEX_SESSIONS_DIR")
		_ = os.Setenv("CODEX_SESSIONS_DIR", codexDir)
		defer func() { _ = os.Setenv("CODEX_SESSIONS_DIR", orig) }()

		res, err := ResolveSessionTarget(ctx, "cdx-999", types.HarnessCodex)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if res.FilePath != sessFile {
			t.Errorf("expected %s, got %s", sessFile, res.FilePath)
		}
	})

	t.Run("resolve session ID in OpenCode database", func(t *testing.T) {
		dbPath := filepath.Join(tmpDir, "test-opencode.db")
		db, err := sql.Open("sqlite3", "file:"+dbPath)
		if err != nil {
			t.Fatalf("opening sqlite: %v", err)
		}
		schema := `
			CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL);
			CREATE TABLE session (
				id TEXT PRIMARY KEY,
				project_id TEXT,
				parent_id TEXT,
				slug TEXT,
				directory TEXT NOT NULL,
				title TEXT,
				version TEXT,
				agent TEXT,
				model TEXT,
				cost REAL,
				tokens_input INTEGER NOT NULL DEFAULT 0,
				tokens_output INTEGER NOT NULL DEFAULT 0,
				tokens_reasoning INTEGER NOT NULL DEFAULT 0,
				tokens_cache_read INTEGER NOT NULL DEFAULT 0,
				tokens_cache_write INTEGER NOT NULL DEFAULT 0,
				time_created INTEGER NOT NULL,
				time_updated INTEGER NOT NULL,
				time_archived INTEGER,
				metadata TEXT
			);
			CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
			CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
		`
		_, _ = db.Exec(schema)
		_, _ = db.Exec(`
			INSERT INTO session (id, directory, time_created, time_updated)
			VALUES ('ses_resolved_1', '/repo', 1000, 2000)
		`)
		_ = db.Close()

		orig := os.Getenv("OPENCODE_DB_PATH")
		_ = os.Setenv("OPENCODE_DB_PATH", dbPath)
		defer func() { _ = os.Setenv("OPENCODE_DB_PATH", orig) }()

		res, err := ResolveSessionTarget(ctx, "ses_resolved_1", types.HarnessOpencode)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if res.FilePath != dbPath || res.SessionID != "ses_resolved_1" {
			t.Errorf("unexpected resolution: %+v", res)
		}
	})

	t.Run("resolve session ID in Claude XDG projects dir", func(t *testing.T) {
		xdgDir := filepath.Join(tmpDir, "xdg_data")
		claudeProjDir := filepath.Join(xdgDir, "ai-registry", "claude-code", "projects", "-repo")
		_ = os.MkdirAll(claudeProjDir, 0755)
		sessFile := filepath.Join(claudeProjDir, "claude-xdg-12345.jsonl")
		_ = os.WriteFile(sessFile, []byte(`{"type":"session","sessionId":"claude-xdg-12345"}`+"\n"), 0644)

		t.Setenv("XDG_DATA_HOME", xdgDir)

		res, err := ResolveSessionTarget(ctx, "claude-xdg-12345", types.HarnessClaudeCode)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if res.FilePath != sessFile {
			t.Errorf("expected %s, got %s", sessFile, res.FilePath)
		}
		if res.SessionID != "claude-xdg-12345" {
			t.Errorf("expected session ID claude-xdg-12345, got %s", res.SessionID)
		}
	})

	t.Run("resolve session ID in CLAUDE_PROJECTS_DIR", func(t *testing.T) {
		customDir := filepath.Join(tmpDir, "custom-claude-projects", "my-slug")
		_ = os.MkdirAll(customDir, 0755)
		sessFile := filepath.Join(customDir, "claude-custom-67890.jsonl")
		_ = os.WriteFile(sessFile, []byte(`{"type":"session","sessionId":"claude-custom-67890"}`+"\n"), 0644)

		t.Setenv("CLAUDE_PROJECTS_DIR", filepath.Dir(customDir))

		res, err := ResolveSessionTarget(ctx, "claude-custom-67890", types.HarnessClaudeCode)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if res.FilePath != sessFile {
			t.Errorf("expected %s, got %s", sessFile, res.FilePath)
		}
	})

	t.Run("not found with harness hint", func(t *testing.T) {
		_, err := ResolveSessionTarget(ctx, "nonexistent-id", types.HarnessPi)
		if err == nil {
			t.Errorf("expected error for nonexistent ID")
		}
		if !strings.Contains(err.Error(), "standard pi storage directories") {
			t.Errorf("expected helpful error message, got: %v", err)
		}
	})

	t.Run("not found without harness hint", func(t *testing.T) {
		_, err := ResolveSessionTarget(ctx, "nonexistent-id", "")
		if err == nil {
			t.Errorf("expected error for nonexistent ID")
		}
		if !strings.Contains(err.Error(), "not found as a file or in any standard harness directory") {
			t.Errorf("expected helpful multi-harness error message, got: %v", err)
		}
	})
}
