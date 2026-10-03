package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestObserverCachePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, cacheHome := range []string{t.TempDir(), ""} {
		t.Run(cacheHome, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", cacheHome)
			base := cacheHome
			if base == "" {
				base = filepath.Join(home, ".cache")
			}
			if path := ObserverCachePath("observer-cursors.json"); path != filepath.Join(base, "agent-status", "observer-cursors.json") {
				t.Fatalf("unexpected cursor path: %s", path)
			}
		})
	}
}
