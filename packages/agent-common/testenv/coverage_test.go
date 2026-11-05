package testenv

import (
	"bytes"
	"context"
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

func TestCoverageRunnerUsesNativeTempByDefault(t *testing.T) {
	testCoverageNativeTemp(t, "success", "")
}

func testCoverageNativeTemp(t *testing.T, mode, report string) {
	t.Helper()
	// The enclosing runner must own this fixture if its test process is killed.
	dir := t.TempDir()
	t.Setenv("TMPDIR", "")
	t.Setenv("GOTMPDIR", "")
	if report == "" {
		report = dir
	}
	writeCoverageFixture(t, filepath.Join(report, "fixture.path"), dir)
	data, err := os.ReadFile("../../../scripts/check-coverage.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "scripts/check-coverage.sh")
	writeCoverageFixture(t, script, string(data))
	writeCoverageModule(t, dir)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script, "-count=1", "./...")
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 15 * time.Second
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "FIXTURE_MODE="+mode, "FIXTURE_REPORT_DIR="+report, "THRESHOLD=90")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Write before waiting so the enclosing-cancellation test can suspend this
	// supervisor and verify its parent still owns every child and scratch path.
	if err := os.WriteFile(filepath.Join(report, "runner.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		cancel()
		_ = cmd.Wait() // join the failed fixture before reporting the write error
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("native-temp run: %v\n%s", err, &output)
	}
	path, err := os.ReadFile(filepath.Join(report, "scratch.path"))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(string(path)) != parent {
		t.Errorf("scratch parent = %q, want native temp %q", filepath.Dir(string(path)), parent)
	}
	if _, err := os.Stat(string(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("native scratch survived: %v", err)
	}
}

func TestCoverageRunnerCleanup(t *testing.T) {
	for _, mode := range []string{"success", "failure", "panic", "timeout", "coverage", "interrupt", "terminate", "hangup", "stubborn", "closed_output"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			scratch := filepath.Join(dir, "scratch")
			if err := os.Mkdir(scratch, 0o700); err != nil {
				t.Fatal(err)
			}
			preserved := filepath.Join(scratch, "other-command")
			writeCoverageFixture(t, preserved, "keep")
			writeCoverageModule(t, dir)
			script, err := filepath.Abs("../../../scripts/check-coverage.sh")
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", script, "-count=1", "-timeout=1s", "./...")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off", "TMPDIR="+scratch, "GOTMPDIR="+scratch, "FIXTURE_MODE="+mode, "THRESHOLD=90")
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			var reader, writer *os.File
			if mode == "closed_output" {
				reader, writer, err = os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = reader.Close() // also closed deliberately before signalling
					_ = writer.Close()
				}()
				cmd.Stdout, cmd.Stderr = writer, writer
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			finished := false
			t.Cleanup(func() {
				if !finished {
					// Also contain a broken runner when this regression fails.
					_ = cmd.Process.Signal(syscall.SIGTERM)
					select {
					case err := <-done:
						if err != nil {
							t.Logf("coverage runner stopped during cleanup: %v", err)
						}
					case <-time.After(15 * time.Second):
						_ = cmd.Process.Kill()
						if err := <-done; err != nil {
							t.Logf("coverage runner killed during cleanup: %v", err)
						}
					}
				}
			})
			signals := map[string]syscall.Signal{"interrupt": syscall.SIGINT, "terminate": syscall.SIGTERM, "hangup": syscall.SIGHUP, "stubborn": syscall.SIGTERM, "closed_output": syscall.SIGTERM}
			if signal, ok := signals[mode]; ok {
				waitCoverageFile(t, filepath.Join(dir, "ready"))
				if reader != nil {
					if err := reader.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if err := cmd.Process.Signal(signal); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err = <-done:
				finished = true
			case <-time.After(60 * time.Second):
				t.Fatal("coverage runner did not finish")
			}
			if (err == nil) != (mode == "success") {
				t.Fatalf("mode %s: exit %v\n%s", mode, err, &output)
			}
			if signal, ok := signals[mode]; ok {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 128+int(signal) {
					t.Errorf("signal exit = %v, want %d\n%s", err, 128+int(signal), &output)
				}
			}
			if mode == "timeout" && !strings.Contains(output.String(), "test timed out") {
				t.Fatalf("test never reached its timeout:\n%s", &output)
			}
			if mode == "coverage" && !strings.Contains(output.String(), "coverage below") {
				t.Fatalf("coverage floor was not enforced:\n%s", &output)
			}
			entries, err := os.ReadDir(scratch)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "other-command" {
				t.Errorf("runner left artifacts or removed another command's files: %v", entries)
			}
			data, err := os.ReadFile(preserved)
			if err != nil || string(data) != "keep" {
				t.Errorf("another command's file changed: %q, %v", data, err)
			}
			if mode == "terminate" || mode == "stubborn" || mode == "closed_output" {
				pidText, err := os.ReadFile(filepath.Join(dir, "child.pid"))
				if err != nil {
					t.Fatal(err)
				}
				pid, err := strconv.Atoi(string(pidText))
				if err != nil {
					t.Fatal(err)
				}
				state, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
				if err == nil && !strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
					_ = syscall.Kill(pid, syscall.SIGKILL) // contain the regression's owned child
					t.Errorf("test child %d survived cleanup: %s", pid, state)
				}
				if mode == "terminate" {
					if _, err := os.Stat(filepath.Join(dir, "graceful")); err != nil {
						t.Errorf("child did not receive graceful termination: %v", err)
					}
				}
			}
		})
	}
}

func writeCoverageModule(t *testing.T, dir string) {
	t.Helper()
	writeCoverageFixture(t, filepath.Join(dir, "go.mod"), "module fixture\n\ngo 1.26.2\n")
	writeCoverageFixture(t, filepath.Join(dir, "fixture.go"), "package fixture\nfunc covered() int { return 1 }\nfunc uncovered() int { return 2 }\n")
	writeCoverageFixture(t, filepath.Join(dir, "fixture_test.go"), coverageFixture)
}

func writeCoverageFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitCoverageFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

const coverageFixture = `package fixture
import (
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "testing"
 "time"
)
func TestFixture(t *testing.T) {
 mode := os.Getenv("FIXTURE_MODE")
 report := os.Getenv("FIXTURE_REPORT_DIR")
 if report == "" { report = "." }
 if err := os.WriteFile(filepath.Join(report, "scratch.path"), []byte(os.TempDir()), 0600); err != nil { t.Fatal(err) }
 if _, err := os.MkdirTemp("", "unmanaged-"); err != nil { t.Fatal(err) }
 t.TempDir()
 covered()
 if mode != "coverage" { uncovered() }
 switch mode {
 case "nested":
   cmd := exec.Command(os.Getenv("COVERAGE_TEST_BINARY"), "-test.run=^TestCoverageRunnerEnclosingFixture$")
   cmd.Dir = os.Getenv("COVERAGE_TEST_DIR")
   if output, err := cmd.CombinedOutput(); err != nil { t.Fatalf("nested fixture: %v\n%s", err, output) }
 case "failure": t.Fatal("intentional failure")
 case "panic": panic("intentional panic")
 case "timeout": time.Sleep(time.Minute)
 case "interrupt", "terminate", "hangup", "stubborn", "closed_output":
   if mode == "terminate" || mode == "stubborn" || mode == "closed_output" {
     action := "trap 'printf done > graceful; exit 0' TERM"
     if mode != "terminate" { action = "trap '' TERM" }
     child := exec.Command("bash", "-c", action+"; printf ready > child.ready; while :; do sleep 0.1; done")
     child.Dir = report
     if err := child.Start(); err != nil { t.Fatal(err) }
     if err := os.WriteFile(filepath.Join(report, "child.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil { t.Fatal(err) }
     for { if _, err := os.Stat(filepath.Join(report, "child.ready")); err == nil { break }; time.Sleep(time.Millisecond) }
   }
   if err := os.WriteFile(filepath.Join(report, "ready"), nil, 0600); err != nil { t.Fatal(err) }
   time.Sleep(time.Minute)
 }
}
`
