package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindClaudeSession_EmptyTarget(t *testing.T) {
	_, err := FindClaudeSession("")
	if err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
	_, err = FindClaudeSession("   ")
	if err == nil {
		t.Fatal("expected error for whitespace target, got nil")
	}
}

func TestFindClaudeSession_DirectFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "session-123.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	found, err := FindClaudeSession(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != filePath {
		t.Fatalf("expected %s, got %s", filePath, found)
	}
}

func TestFindClaudeSession_SearchDirectories(t *testing.T) {
	tmpRoot := t.TempDir()
	projectsDir := filepath.Join(tmpRoot, "projects", "my-project")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	exactFile := filepath.Join(projectsDir, "session-alpha-123.jsonl")
	if err := os.WriteFile(exactFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	prefixFile := filepath.Join(projectsDir, "session-beta-456.jsonl")
	if err := os.WriteFile(prefixFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	t.Setenv("CLAUDE_PROJECTS_DIR", filepath.Join(tmpRoot, "projects"))

	// 1. Exact match
	found, err := FindClaudeSession("session-alpha-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != exactFile {
		t.Fatalf("expected exact match %s, got %s", exactFile, found)
	}

	// 2. Prefix match
	found, err = FindClaudeSession("session-beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != prefixFile {
		t.Fatalf("expected prefix match %s, got %s", prefixFile, found)
	}

	// 3. Substring match
	found, err = FindClaudeSession("alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != exactFile {
		t.Fatalf("expected substring match %s, got %s", exactFile, found)
	}

	// 4. Not found
	_, err = FindClaudeSession("non-existent-session-id")
	if err == nil {
		t.Fatal("expected error for non-existent session, got nil")
	}
}

func TestFindClaudeSession_SessionsDirAndXDG(t *testing.T) {
	tmpRoot := t.TempDir()
	sessDir := filepath.Join(tmpRoot, "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessFile := filepath.Join(sessDir, "ses-custom.jsonl")
	if err := os.WriteFile(sessFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLAUDE_SESSIONS_DIR", sessDir)
	found, err := FindClaudeSession("ses-custom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != sessFile {
		t.Fatalf("expected %s, got %s", sessFile, found)
	}

	// Test XDG
	xdgRoot := filepath.Join(tmpRoot, "xdg")
	xdgProjects := filepath.Join(xdgRoot, "ai-registry", "claude-code", "projects")
	if err := os.MkdirAll(xdgProjects, 0o755); err != nil {
		t.Fatal(err)
	}
	xdgFile := filepath.Join(xdgProjects, "ses-xdg.jsonl")
	if err := os.WriteFile(xdgFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLAUDE_PROJECTS_DIR", "")
	t.Setenv("CLAUDE_SESSIONS_DIR", "")
	t.Setenv("XDG_DATA_HOME", xdgRoot)

	found, err = FindClaudeSession("ses-xdg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != xdgFile {
		t.Fatalf("expected %s, got %s", xdgFile, found)
	}
}

func TestFindTranscriptInDir_InvalidDir(t *testing.T) {
	res := findTranscriptInDir("/invalid/non/existent/path/for/sure", "anything")
	if res != "" {
		t.Fatalf("expected empty string, got %s", res)
	}
}

func TestFindPiSession_EmptyAndDirect(t *testing.T) {
	if _, err := FindPiSession(""); err == nil {
		t.Fatal("expected error on empty target, got nil")
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "pi-sess.jsonl")
	if err := os.WriteFile(filePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := FindPiSession(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != filePath {
		t.Fatalf("expected %s, got %s", filePath, found)
	}
}

func TestFindPiSession_SearchDirs(t *testing.T) {
	tmpRoot := t.TempDir()
	cwdDir := filepath.Join(tmpRoot, "--encoded-cwd--")
	if err := os.MkdirAll(cwdDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// File with Pi timestamp prefix: 2026-09-25T21-07-10-103Z_<id>.jsonl
	piFile := filepath.Join(cwdDir, "2026-09-25T21-07-10-103Z_pi-test-123.jsonl")
	if err := os.WriteFile(piFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PI_SESSIONS_DIR", tmpRoot)

	// Exact session ID search
	found, err := FindPiSession("pi-test-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != piFile {
		t.Fatalf("expected %s, got %s", piFile, found)
	}

	// Not found
	if _, err := FindPiSession("non-existent-pi-id"); err == nil {
		t.Fatal("expected error on missing session, got nil")
	}
}

func TestFindPiSession_XDG(t *testing.T) {
	tmpRoot := t.TempDir()
	xdgDir := filepath.Join(tmpRoot, "ai-registry", "pi", "sessions")
	if err := os.MkdirAll(xdgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	xdgFile := filepath.Join(xdgDir, "xdg-pi-session.jsonl")
	if err := os.WriteFile(xdgFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PI_SESSIONS_DIR", "")
	t.Setenv("XDG_DATA_HOME", tmpRoot)

	found, err := FindPiSession("xdg-pi-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != xdgFile {
		t.Fatalf("expected %s, got %s", xdgFile, found)
	}
}
