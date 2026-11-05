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
			writeCoverageFixture(t, filepath.Join(dir, "go.mod"), "module fixture\n\ngo 1.26.2\n")
			writeCoverageFixture(t, filepath.Join(dir, "fixture.go"), "package fixture\nfunc covered() int { return 1 }\nfunc uncovered() int { return 2 }\n")
			writeCoverageFixture(t, filepath.Join(dir, "fixture_test.go"), coverageFixture)
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
					case <-done:
					case <-time.After(15 * time.Second):
						_ = cmd.Process.Kill()
						<-done
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
 "strconv"
 "testing"
 "time"
)
func TestFixture(t *testing.T) {
 mode := os.Getenv("FIXTURE_MODE")
 if _, err := os.MkdirTemp("", "unmanaged-"); err != nil { t.Fatal(err) }
 t.TempDir()
 covered()
 if mode != "coverage" { uncovered() }
 switch mode {
 case "failure": t.Fatal("intentional failure")
 case "panic": panic("intentional panic")
 case "timeout": time.Sleep(time.Minute)
 case "interrupt", "terminate", "hangup", "stubborn", "closed_output":
   if mode == "terminate" || mode == "stubborn" || mode == "closed_output" {
     action := "trap 'printf done > graceful; exit 0' TERM"
     if mode != "terminate" { action = "trap '' TERM" }
     child := exec.Command("bash", "-c", action+"; printf ready > child.ready; while :; do sleep 0.1; done")
     if err := child.Start(); err != nil { t.Fatal(err) }
     if err := os.WriteFile("child.pid", []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil { t.Fatal(err) }
     for { if _, err := os.Stat("child.ready"); err == nil { break }; time.Sleep(time.Millisecond) }
   }
   if err := os.WriteFile("ready", nil, 0600); err != nil { t.Fatal(err) }
   time.Sleep(time.Minute)
 }
}
`
