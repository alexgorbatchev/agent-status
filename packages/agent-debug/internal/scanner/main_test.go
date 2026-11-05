package scanner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/testenv"
)

func TestMain(m *testing.M) {
	dir, cleanup, err := testenv.IsolateStateDirs("agent-debug-scanner-test-state")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := testenv.IsolateHarnessEnv(filepath.Join(dir, "home")); err != nil {
		cleanup()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestStateAndHarnessAreIsolated(t *testing.T) {
	testenv.RequireIsolated(t)
	testenv.RequireHarnessIsolated(t)
}
