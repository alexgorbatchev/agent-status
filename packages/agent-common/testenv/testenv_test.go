package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
)

func TestIsolateStateDirsKeepsLogsOutOfHome(t *testing.T) {
	for _, name := range []string{"AIR_LOG_DIR", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, os.Getenv(name)) // restored after the test
	}
	dir, cleanup, err := IsolateStateDirs("testenv")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if got := logging.DefaultLogDir(); got != dir {
		t.Errorf("DefaultLogDir = %q, want the isolated directory %q", got, dir)
	}
	// A test that clears AIR_LOG_DIR still resolves logs inside the isolated directory.
	t.Setenv("AIR_LOG_DIR", "")
	if got := logging.DefaultLogDir(); !strings.HasPrefix(got, dir+string(filepath.Separator)) {
		t.Errorf("DefaultLogDir without AIR_LOG_DIR = %q, want it under %q", got, dir)
	}

	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup left %q behind: %v", dir, err)
	}
}

func TestIsolateStateDirsReportsUnusableTempDir(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, _, err := IsolateStateDirs("testenv"); err == nil {
		t.Fatal("expected an error when the temporary directory cannot be created")
	}
}

func TestIsolateStateDirsCleansUpWhenAVariableCannotBeSet(t *testing.T) {
	orig := stateEnv
	defer func() { stateEnv = orig }()
	var created string
	stateEnv = func(dir string) map[string]string {
		created = dir
		return map[string]string{"": dir} // os.Setenv rejects an empty name
	}
	if _, _, err := IsolateStateDirs("testenv"); err == nil {
		t.Fatal("expected an error when a variable cannot be set")
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("the temporary directory %q was not removed: %v", created, err)
	}
}
