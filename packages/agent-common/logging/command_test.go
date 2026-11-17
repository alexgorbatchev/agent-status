package logging

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCommand(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })
	for _, mode := range []string{"0", "1"} {
		for _, fails := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/success", true: "/failure"}[fails], func(t *testing.T) {
				t.Setenv("AGENT", mode)
				var out bytes.Buffer
				code := RunCommand("test-cli", &out, func() error {
					if fails {
						return errors.New("operation failed")
					}
					return nil
				})
				if !fails {
					if code != 0 || out.Len() != 0 {
						t.Fatalf("success produced diagnostics: code=%d log=%s", code, &out)
					}
					return
				}
				if code != 1 || strings.Count(out.String(), "command_failed") != 1 || !strings.Contains(out.String(), "err=\"operation failed\"") {
					t.Fatalf("failure was not reported once: code=%d log=%s", code, &out)
				}
				if mode == "1" && (!strings.Contains(out.String(), "action=command_failed") || !strings.Contains(out.String(), "service=test-cli")) {
					t.Fatalf("agent log is not structured: %s", &out)
				}
				if mode == "0" && !strings.Contains(out.String(), "[ERROR]") {
					t.Fatalf("human log has no level: %s", &out)
				}
			})
		}
	}
}

func TestRunCommandKeepsDiskLoggerUntilFailureReported(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })
	var out bytes.Buffer
	file := filepath.Join(t.TempDir(), "command.log")
	code := RunCommand("test-cli", &out, func() error {
		SetupWithOptions(SetupOptions{Service: "test-cli", Writer: &out, Level: "info", FileConfig: &FileConfig{Filename: file}})
		return errors.New("startup failed")
	})
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(content), "command_failed") || !strings.Contains(string(content), "startup failed") || defaultRotator != nil {
		t.Fatalf("fatal error lost from disk or rotator left open: code=%d log=%s", code, content)
	}
}

func TestRunCommandLogCloseFailure(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })
	file, err := os.CreateTemp(t.TempDir(), "closed-log")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	defaultRotator = file
	var out bytes.Buffer
	if code := RunCommand("test-cli", &out, func() error { return nil }); code != 1 || !strings.Contains(out.String(), "log_close_failed") {
		t.Fatalf("log close failure lost: code=%d log=%s", code, &out)
	}
}
