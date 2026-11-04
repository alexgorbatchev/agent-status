// Package testenv isolates tests from the developer's real agent-status state.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedDir is the directory the last successful IsolateStateDirs call created.
var isolatedDir string

// IsolateStateDirs points every location agent-status writes state to (the log
// directory, and the XDG state, data, and cache homes it falls back to) at a new
// temporary directory, so a test that starts a daemon or clears AIR_LOG_DIR cannot write
// into the developer's home directory. Call it from TestMain before m.Run, and call the
// returned cleanup after. It returns the temporary directory.
func IsolateStateDirs(prefix string) (string, func(), error) {
	dir, err := os.MkdirTemp("", prefix+"-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating isolated state directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	for name, path := range stateEnv(dir) {
		if err := os.Setenv(name, path); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("isolating %s: %w", name, err)
		}
	}
	isolatedDir = dir
	return dir, cleanup, nil
}

// RequireIsolated fails the test unless IsolateStateDirs ran in this process and every
// state variable it sets still points inside the isolated directory. Call it from a test
// in each package whose TestMain isolates state, so removing that isolation fails the
// package instead of silently writing to the home directory.
func RequireIsolated(t testing.TB) {
	t.Helper()
	if isolatedDir == "" {
		t.Fatalf("state is not isolated: call testenv.IsolateStateDirs from TestMain")
	}
	for name := range stateEnv(isolatedDir) {
		value := os.Getenv(name)
		if value != isolatedDir && !strings.HasPrefix(value, isolatedDir+string(filepath.Separator)) {
			t.Fatalf("%s = %q is outside the isolated directory %q", name, value, isolatedDir)
		}
	}
}

// stateEnv returns the variables IsolateStateDirs sets, pointing inside dir.
var stateEnv = func(dir string) map[string]string {
	return map[string]string{
		"AIR_LOG_DIR":    dir,
		"XDG_STATE_HOME": filepath.Join(dir, "state"),
		"XDG_DATA_HOME":  filepath.Join(dir, "data"),
		"XDG_CACHE_HOME": filepath.Join(dir, "cache"),
	}
}
