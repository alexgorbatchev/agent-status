package replay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestDetectHarness(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("non-existent file", func(t *testing.T) {
		_, err := DetectHarness(filepath.Join(tmpDir, "missing.jsonl"))
		if err == nil {
			t.Errorf("expected error for non-existent file")
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		emptyDir := filepath.Join(tmpDir, "empty-dir")
		_ = os.MkdirAll(emptyDir, 0755)
		_, err := DetectHarness(emptyDir)
		if err == nil {
			t.Errorf("expected error for empty directory")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		emptyFile := filepath.Join(tmpDir, "empty.jsonl")
		_ = os.WriteFile(emptyFile, []byte(""), 0644)
		_, err := DetectHarness(emptyFile)
		if err == nil {
			t.Errorf("expected error for empty file")
		}
	})

	t.Run("opencode sqlite magic", func(t *testing.T) {
		dbFile := filepath.Join(tmpDir, "test.db")
		magic := append([]byte("SQLite format 3\x00"), make([]byte, 100)...)
		_ = os.WriteFile(dbFile, magic, 0644)

		h, err := DetectHarness(dbFile)
		if err != nil {
			t.Fatalf("DetectHarness failed: %v", err)
		}
		if h != types.HarnessOpencode {
			t.Errorf("expected %s, got %s", types.HarnessOpencode, h)
		}
	})

	t.Run("directory containing sqlite db", func(t *testing.T) {
		dir := filepath.Join(tmpDir, "opencode-dir")
		_ = os.MkdirAll(dir, 0755)
		_ = os.WriteFile(filepath.Join(dir, "opencode.db"), []byte("SQLite format 3\x00"), 0644)

		h, err := DetectHarness(dir)
		if err != nil {
			t.Fatalf("DetectHarness failed: %v", err)
		}
		if h != types.HarnessOpencode {
			t.Errorf("expected %s, got %s", types.HarnessOpencode, h)
		}
	})

	t.Run("claude code signatures", func(t *testing.T) {
		ccFile := filepath.Join(tmpDir, "claude-session.jsonl")
		content := `{"type":"ai-title","aiTitle":"Test Session","timestamp":"2026-09-21T10:00:00Z"}` + "\n" +
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}` + "\n"
		_ = os.WriteFile(ccFile, []byte(content), 0644)

		h, err := DetectHarness(ccFile)
		if err != nil {
			t.Fatalf("DetectHarness failed: %v", err)
		}
		if h != types.HarnessClaudeCode {
			t.Errorf("expected %s, got %s", types.HarnessClaudeCode, h)
		}
	})

	t.Run("pi signatures", func(t *testing.T) {
		piFile := filepath.Join(tmpDir, "pi-session.jsonl")
		content := `{"type":"session","id":"pi-sess-1","cwd":"/repo","version":1}` + "\n"
		_ = os.WriteFile(piFile, []byte(content), 0644)

		h, err := DetectHarness(piFile)
		if err != nil {
			t.Fatalf("DetectHarness failed: %v", err)
		}
		if h != types.HarnessPi {
			t.Errorf("expected %s, got %s", types.HarnessPi, h)
		}
	})

	t.Run("codex rollout filename", func(t *testing.T) {
		codexFile := filepath.Join(tmpDir, "rollout-2026-09-21T10-00-00.jsonl")
		content := `{"type":"session_meta","timestamp":"2026-09-21T10:00:00Z","payload":{"id":"ses-1"}}` + "\n"
		_ = os.WriteFile(codexFile, []byte(content), 0644)

		h, err := DetectHarness(codexFile)
		if err != nil {
			t.Fatalf("DetectHarness failed: %v", err)
		}
		if h != types.HarnessCodex {
			t.Errorf("expected %s, got %s", types.HarnessCodex, h)
		}
	})

	t.Run("directory containing jsonl file", func(t *testing.T) {
		dir := filepath.Join(tmpDir, "jsonl-dir")
		_ = os.MkdirAll(dir, 0755)
		content := `{"type":"session","id":"pi-dir-sess","cwd":"/repo","version":1}` + "\n"
		_ = os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte(content), 0644)

		h, err := DetectHarness(dir)
		if err != nil {
			t.Fatalf("DetectHarness failed: %v", err)
		}
		if h != types.HarnessPi {
			t.Errorf("expected %s, got %s", types.HarnessPi, h)
		}
	})

	t.Run("fallback parser trial", func(t *testing.T) {
		fallbackFile := filepath.Join(tmpDir, "fallback.jsonl")
		// A line without keywords that ClaudeCodeParser still parses
		content := `{"type":"unknown_type_fallback"}` + "\n"
		_ = os.WriteFile(fallbackFile, []byte(content), 0644)
		_, err := DetectHarness(fallbackFile)
		if err == nil {
			t.Errorf("expected error for unknown type fallback")
		}
	})
}
