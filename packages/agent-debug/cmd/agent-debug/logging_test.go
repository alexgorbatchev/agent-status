package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
)

func TestCLI_ReplayDiagnostics(t *testing.T) {
	transcriptPath := createTestClaudeTranscript(t, t.TempDir())
	for _, mode := range []string{"0", "1"} {
		t.Run("AGENT="+mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			for _, debug := range []bool{false, true} {
				name := "default"
				if debug {
					name = "debug"
				}
				t.Run(name, func(t *testing.T) {
					previousLogger := logging.Default()
					t.Cleanup(func() { logging.SetDefault(previousLogger) })
					var stdout, stderr bytes.Buffer
					// Capture diagnostics even when the CLI has not configured its logger.
					logging.SetDefault(logging.New(&stderr, "", "debug"))
					cmd := newRootCmd()
					cmd.SetOut(&stdout)
					cmd.SetErr(&stderr)
					args := []string{"session", "replay", transcriptPath, "--json"}
					if debug {
						args = append(args, "--debug")
					}
					cmd.SetArgs(args)
					if err := cmd.Execute(); err != nil {
						t.Fatalf("replay failed: %v", err)
					}
					var result map[string]json.RawMessage
					if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
						t.Fatalf("stdout must contain only JSON: %v\n%s", err, stdout.String())
					}
					if len(result["agent"]) == 0 {
						t.Fatal("missing replayed agent in stdout")
					}
					if debug {
						logging.Debug("replay_debug_probe")
						if !strings.Contains(stderr.String(), "replay_debug_probe") {
							t.Fatalf("debug-level logs must reach stderr, got %q", stderr.String())
						}
						if !strings.Contains(stderr.String(), "session_discovered") {
							t.Fatalf("expected diagnostics on stderr, got %q", stderr.String())
						}
					} else if stderr.Len() != 0 {
						t.Fatalf("default replay must be quiet on stderr, got %q", stderr.String())
					}
					logging.Warn("replay_warning_probe")
					logging.Error("replay_error_probe")
					if !strings.Contains(stderr.String(), "replay_warning_probe") ||
						!strings.Contains(stderr.String(), "replay_error_probe") {
						t.Fatalf("warnings and errors must remain visible, got %q", stderr.String())
					}
				})
			}
		})
	}
}

func TestCLI_ReplayErrorLeavesStdoutEmpty(t *testing.T) {
	previousLogger := logging.Default()
	t.Cleanup(func() { logging.SetDefault(previousLogger) })
	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"session", "replay", filepath.Join(t.TempDir(), "missing.jsonl"), "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a replay error for a missing transcript")
	}
	if !strings.Contains(err.Error(), "missing.jsonl") {
		t.Fatalf("expected transcript path in error, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed JSON replay must not write stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("command must return its error for main to report once, got %q", stderr.String())
	}
}
