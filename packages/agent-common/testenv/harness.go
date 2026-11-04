package testenv

import (
	"fmt"
	"os"
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
	return nil
}
