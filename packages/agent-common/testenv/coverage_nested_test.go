package testenv

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This subprocess runs the actual native-temp regression while its parent runner
// is cancelled. Keeping it in a subprocess also exercises abrupt Go test exit.
func TestCoverageRunnerEnclosingFixture(t *testing.T) {
	report := os.Getenv("COVERAGE_ENCLOSING_REPORT")
	if report == "" {
		t.Skip("subprocess fixture")
	}
	testCoverageTemp(t, "stubborn", report, "")
}

func TestCoverageRunnerEnclosingCancellation(t *testing.T) {
	for _, suspend := range []bool{false, true} {
		t.Run("suspend="+strconv.FormatBool(suspend), func(t *testing.T) {
			testCoverageEnclosingCancellation(t, suspend)
		})
	}
}

func testCoverageEnclosingCancellation(t *testing.T, suspend bool) {
	t.Helper()
	dir := t.TempDir()
	report := filepath.Join(dir, "report")
	scratch := filepath.Join(dir, "scratch")
	for _, path := range []string{report, scratch} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeCoverageModule(t, dir)
	script, err := filepath.Abs("../../../scripts/check-coverage.sh")
	if err != nil {
		t.Fatal(err)
	}
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script, "-count=1", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "TMPDIR="+scratch, "GOTMPDIR="+scratch,
		"FIXTURE_MODE=nested", "FIXTURE_REPORT_DIR=", "COVERAGE_TEST_BINARY="+os.Args[0],
		"COVERAGE_TEST_DIR="+workdir, "COVERAGE_ENCLOSING_REPORT="+report, "THRESHOLD=90")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	cmd.WaitDelay = 15 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	finished := false
	group := 0
	t.Cleanup(func() {
		if !finished {
			_ = cmd.Process.Signal(syscall.SIGTERM) // best effort on a failed regression
			select {
			case err := <-done:
				if err != nil {
					t.Logf("enclosing runner stopped during cleanup: %v", err)
				}
			case <-time.After(20 * time.Second):
				_ = cmd.Process.Kill() // contain the owned broken supervisor
				if err := <-done; err != nil {
					t.Logf("enclosing runner killed during cleanup: %v", err)
				}
			}
		}
		if group > 0 {
			_ = syscall.Kill(-group, syscall.SIGKILL) // contain escaped fixture processes
		}
		for _, name := range []string{"fixture.path", "scratch.path"} {
			if data, err := os.ReadFile(filepath.Join(report, name)); err == nil {
				if err := os.RemoveAll(string(data)); err != nil {
					t.Errorf("remove owned %s: %v", name, err)
				}
			}
		}
	})
	waitCoverageFile(t, filepath.Join(report, "ready"))
	pidText, err := os.ReadFile(filepath.Join(report, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(pidText))
	if err != nil {
		t.Fatal(err)
	}
	group, err = syscall.Getpgid(pid)
	if err != nil {
		t.Fatal(err)
	}
	if suspend {
		runnerText, err := os.ReadFile(filepath.Join(report, "runner.pid"))
		if err != nil {
			t.Fatal(err)
		}
		runner, err := strconv.Atoi(string(runnerText))
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(runner, syscall.SIGSTOP); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		finished = true
	case <-time.After(30 * time.Second):
		t.Fatal("enclosing runner did not stop")
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 143 {
		t.Errorf("enclosing runner exit = %v, want 143\n%s", err, &output)
	}
	state, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err == nil && !strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
		t.Errorf("stubborn grandchild %d survived enclosing cancellation: %s", pid, state)
	}
	for _, name := range []string{"fixture.path", "scratch.path"} {
		path, err := os.ReadFile(filepath.Join(report, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(string(path)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived enclosing cancellation: %s (%v)", name, path, err)
		}
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Errorf("enclosing scratch not empty: %v, %v", entries, err)
	}
}
