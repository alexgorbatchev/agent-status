package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionTracker_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sessions.json")

	st, err := NewSessionTracker(filePath)
	if err != nil {
		t.Fatalf("NewSessionTracker failed: %v", err)
	}

	st.Set("sess-1", TrackedSession{
		SessionID:      "sess-1",
		Harness:        "pi",
		PID:            1234,
		CWD:            "/tmp/test",
		HarnessVersion: "0.1.0",
		StartedAt:      1000,
		IsActive:       true,
	})
	st.Set("sess-2", TrackedSession{
		SessionID:      "sess-2",
		Harness:        "claude-code",
		PID:            5678,
		CWD:            "/tmp/test2",
		HarnessVersion: "1.0.0",
		StartedAt:      2000,
		IsActive:       true,
	})

	if err := st.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload from file
	st2, err := NewSessionTracker(filePath)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	all := st2.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}
	s1, exists := all["sess-1"]
	if !exists || s1.PID != 1234 || s1.Harness != "pi" {
		t.Errorf("sess-1 mismatch: %+v", s1)
	}

	// Delete
	st2.Delete("sess-1")
	if err := st2.Save(); err != nil {
		t.Fatalf("Save after delete failed: %v", err)
	}

	st3, err := NewSessionTracker(filePath)
	if err != nil {
		t.Fatalf("Reload 3 failed: %v", err)
	}
	if len(st3.GetAll()) != 1 {
		t.Errorf("expected 1 session after delete, got %d", len(st3.GetAll()))
	}
}

func TestSessionTracker_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "corrupted.json")
	_ = os.WriteFile(filePath, []byte("{invalid json"), 0644)

	st, err := NewSessionTracker(filePath)
	if err != nil {
		t.Fatalf("expected graceful fallback on corrupted json, got: %v", err)
	}
	if len(st.GetAll()) != 0 {
		t.Errorf("expected empty sessions on corrupted file")
	}
}

func TestSessionTracker_DefaultPath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	p := DefaultSessionStatePath()
	if p != "/custom/cache/agent-status/observer-sessions.json" {
		t.Errorf("expected /custom/cache path, got %s", p)
	}

	t.Setenv("XDG_CACHE_HOME", "")
	p2 := DefaultSessionStatePath()
	if p2 == "" {
		t.Errorf("expected non-empty default path")
	}

	// NewSessionTracker with empty path
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)
	st, err := NewSessionTracker("")
	if err != nil {
		t.Fatalf("NewSessionTracker(\"\") failed: %v", err)
	}
	if st == nil {
		t.Fatalf("expected non-nil tracker")
	}
}
