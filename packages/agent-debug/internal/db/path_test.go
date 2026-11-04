package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDB(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// isolateDBLookup runs the test from an empty working directory with an empty home and
// no overrides, so only the fixtures it writes can be found.
func isolateDBLookup(t *testing.T) (home, workDir string) {
	t.Helper()
	home = t.TempDir()
	workDir = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("AIR_DB_PATH", "")
	t.Chdir(workDir)
	return home, workDir
}

func TestResolveDBPathSearchOrder(t *testing.T) {
	t.Run("explicit path wins and expands ~", func(t *testing.T) {
		home, _ := isolateDBLookup(t)
		explicit := filepath.Join(home, "custom", "agent-status.db")
		writeDB(t, explicit)
		writeDB(t, devStackDBPath)
		got, err := ResolveDBPath("~/custom/agent-status.db")
		if err != nil || got != explicit {
			t.Fatalf("ResolveDBPath = %q, %v; want %q", got, err, explicit)
		}
	})
	t.Run("AIR_DB_PATH beats the dev stack", func(t *testing.T) {
		home, _ := isolateDBLookup(t)
		override := filepath.Join(home, "override.db")
		writeDB(t, override)
		writeDB(t, devStackDBPath)
		t.Setenv("AIR_DB_PATH", override)
		if got, err := ResolveDBPath("auto"); err != nil || got != override {
			t.Fatalf("ResolveDBPath = %q, %v; want %q", got, err, override)
		}
	})
	t.Run("explicit path beats AIR_DB_PATH", func(t *testing.T) {
		home, _ := isolateDBLookup(t)
		explicit := filepath.Join(home, "explicit.db")
		override := filepath.Join(home, "override.db")
		writeDB(t, explicit)
		writeDB(t, override)
		t.Setenv("AIR_DB_PATH", override)
		if got, err := ResolveDBPath(explicit); err != nil || got != explicit {
			t.Fatalf("ResolveDBPath = %q, %v; want %q", got, err, explicit)
		}
	})
	t.Run("AIR_DB_PATH beats the installed database", func(t *testing.T) {
		home, _ := isolateDBLookup(t)
		override := filepath.Join(home, "override.db")
		writeDB(t, override)
		writeDB(t, filepath.Join(home, ".local/share/agent-status/agent-status.db"))
		t.Setenv("AIR_DB_PATH", override)
		if got, err := ResolveDBPath("auto"); err != nil || got != override {
			t.Fatalf("ResolveDBPath = %q, %v; want %q", got, err, override)
		}
	})
	t.Run("installed database beats the dev stack", func(t *testing.T) {
		home, _ := isolateDBLookup(t)
		want := filepath.Join(home, ".local/share/agent-status/agent-status.db")
		writeDB(t, want)
		writeDB(t, devStackDBPath)
		if got, err := ResolveDBPath(""); err != nil || got != want {
			t.Fatalf("ResolveDBPath = %q, %v; want %q", got, err, want)
		}
	})
	t.Run("dev stack when nothing is installed", func(t *testing.T) {
		isolateDBLookup(t)
		writeDB(t, devStackDBPath)
		if got, err := ResolveDBPath("auto"); err != nil || got != devStackDBPath {
			t.Fatalf("ResolveDBPath = %q, %v; want the dev stack", got, err)
		}
	})
	t.Run("installed database under XDG_DATA_HOME", func(t *testing.T) {
		_, _ = isolateDBLookup(t)
		xdg := t.TempDir()
		t.Setenv("XDG_DATA_HOME", xdg)
		want := filepath.Join(xdg, "agent-status", "agent-status.db")
		writeDB(t, want)
		if got, err := ResolveDBPath("auto"); err != nil || got != want {
			t.Fatalf("ResolveDBPath = %q, %v; want %q", got, err, want)
		}
	})
	t.Run("installed database under the home default", func(t *testing.T) {
		home, _ := isolateDBLookup(t)
		want := filepath.Join(home, ".local/share/agent-status/agent-status.db")
		writeDB(t, want)
		if got, err := ResolveDBPath("auto"); err != nil || got != want {
			t.Fatalf("ResolveDBPath = %q, %v; want %q", got, err, want)
		}
	})
}

func TestResolveDBPathErrors(t *testing.T) {
	t.Run("missing explicit path", func(t *testing.T) {
		isolateDBLookup(t)
		if _, err := ResolveDBPath("/nonexistent/agent-status.db"); err == nil || !strings.Contains(err.Error(), "/nonexistent/agent-status.db") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing AIR_DB_PATH does not fall back", func(t *testing.T) {
		isolateDBLookup(t)
		writeDB(t, devStackDBPath)
		t.Setenv("AIR_DB_PATH", "/nonexistent/override.db")
		if _, err := ResolveDBPath("auto"); err == nil || !strings.Contains(err.Error(), "AIR_DB_PATH") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("nothing found names every place searched", func(t *testing.T) {
		home, _ := isolateDBLookup(t)
		_, err := ResolveDBPath("auto")
		if err == nil || !strings.Contains(err.Error(), devStackDBPath) || !strings.Contains(err.Error(), filepath.Join(home, ".local/share/agent-status/agent-status.db")) {
			t.Fatalf("err = %v", err)
		}
	})
}
