package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// harnessEnv lists the variables that relocate harness session storage or the
// agent-status database away from their defaults under HOME.
var harnessEnv = []string{
	"CLAUDE_CONFIG_DIR",
	"CLAUDE_PROJECTS_DIR",
	"CLAUDE_SESSIONS_DIR",
	"PI_CODING_AGENT_DIR",
	"PI_SESSIONS_DIR",
	"CODEX_HOME",
	"CODEX_SESSIONS_DIR",
	"OPENCODE_DB_PATH",
	"AGENT_STATUS_DB_PATH",
}

// isolatedHome is the HOME the last successful IsolateHarnessEnv call set.
var isolatedHome string

// IsolateHarnessHome gives the calling test an empty HOME, unsets XDG_DATA_HOME,
// and clears every harness location override, so session discovery and
// resolution cannot read the developer's real sessions. The variables are
// restored when the test ends. It returns the new HOME.
func IsolateHarnessHome(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	for _, name := range harnessEnv {
		t.Setenv(name, "")
	}
	return home
}

// IsolateHarnessEnv applies the same isolation to the whole test process: HOME
// becomes home, which is created if needed, and every harness location override
// is unset. Call it from TestMain before m.Run, after IsolateStateDirs so that
// XDG_DATA_HOME already points into the isolated directory. Tests that need
// specific locations still set them with t.Setenv.
func IsolateHarnessEnv(home string) error {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("creating isolated home: %w", err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		return fmt.Errorf("isolating HOME: %w", err)
	}
	for _, name := range harnessEnv {
		if err := os.Unsetenv(name); err != nil {
			return fmt.Errorf("clearing %s: %w", name, err)
		}
	}
	isolatedHome = home
	return nil
}

// RequireHarnessIsolated fails the test unless IsolateHarnessEnv ran in this
// process, HOME is still inside the isolated home, and no harness location
// override is set. Call it from a test in each package whose TestMain isolates
// harness locations, so removing that isolation fails the package instead of
// silently reading the developer's sessions.
func RequireHarnessIsolated(t testing.TB) {
	t.Helper()
	if isolatedHome == "" {
		t.Fatalf("harness locations are not isolated: call testenv.IsolateHarnessEnv from TestMain")
	}
	if home := os.Getenv("HOME"); home != isolatedHome && !strings.HasPrefix(home, isolatedHome+string(filepath.Separator)) {
		t.Fatalf("HOME = %q is outside the isolated home %q", home, isolatedHome)
	}
	for _, name := range harnessEnv {
		if value := os.Getenv(name); value != "" {
			t.Fatalf("%s = %q relocates harness storage outside the isolated home", name, value)
		}
	}
}
