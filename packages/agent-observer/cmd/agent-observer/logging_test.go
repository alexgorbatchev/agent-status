package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
)

var processDefaultLogger = logging.Default()

func preserveDefaultLogger(t *testing.T) {
	t.Helper()
	previous := logging.Default()
	writer, flags := log.Writer(), log.Flags()
	t.Cleanup(func() {
		if err := logging.Close(); err != nil {
			t.Errorf("closing test log rotator: %v", err)
		}
		logging.SetDefault(previous)
		// SetDefault changes log's writer and flags; restoring slog alone
		// leaves the initial default handler forwarding into the old rotator.
		log.SetOutput(writer)
		log.SetFlags(flags)
	})
}

func TestObserverCommandLoggerCleanup(t *testing.T) {
	for _, tt := range []struct {
		name       string
		run        func(*testing.T)
		structured bool
	}{
		{"run_observer/standard", TestRunObserverInvalidCursorPath, false},
		{"root_command/standard", TestRootCmdExecute_RunE, false},
		{"run_observer/structured", TestRunObserverInvalidCursorPath, true},
		{"root_command/structured", TestRootCmdExecute_RunE, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			previous := logging.Default()
			writer, flags := log.Writer(), log.Flags()
			t.Cleanup(func() {
				logging.SetDefault(previous)
				log.SetOutput(writer)
				log.SetFlags(flags)
			})
			var output bytes.Buffer
			logging.SetDefault(processDefaultLogger)
			log.SetOutput(&output)
			if tt.structured {
				logging.SetDefault(logging.New(&output, "test", "debug"))
			}
			log.SetFlags(log.Lshortfile)

			tmpDir := t.TempDir()
			t.Setenv("TMPDIR", tmpDir)
			t.Setenv("GOTMPDIR", tmpDir)
			t.Setenv("AGENT_STATUS_LOG_TO_DISK", "1")
			t.Setenv("AGENT_STATUS_LOG_FILE", "")
			t.Run("command", tt.run)

			// The command subtest and its TempDir cleanup have both finished.
			logging.Error("after_command_cleanup")
			log.Print("standard_log_after_cleanup")
			if !strings.Contains(output.String(), "after_command_cleanup") {
				t.Error("later logging did not reach the previous logger")
			}
			if !strings.Contains(output.String(), "standard_log_after_cleanup") {
				t.Error("later standard logging did not reach the previous writer")
			}
			if !strings.Contains(output.String(), "logging_test.go:") {
				t.Error("later logging lost the caller's file prefix")
			}
			entries, err := os.ReadDir(tmpDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Errorf("later logging recreated command scratch: %v", entries)
			}
		})
	}
}
