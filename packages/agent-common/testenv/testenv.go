// Package testenv isolates tests from the developer's real agent-status state.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
)

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
	return dir, cleanup, nil
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
