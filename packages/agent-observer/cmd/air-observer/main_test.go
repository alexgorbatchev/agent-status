package main

import (
	"bytes"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestCLIFlagsAndDefaults(t *testing.T) {
	opts := &observerOptions{
		ingesterURL: "ws://127.0.0.1:4501/ws/ingest",
		logLevel:    "info",
	}

	cmd := newRootCmd(opts)
	cmd.SetArgs([]string{
		"--codex-sessions-dir", "/tmp/test-codex-sessions",
		"--opencode-db-path", "/tmp/test-opencode.db",
		"--claude-sessions-dir", "/tmp/test-claude-sessions",
		"--pi-sessions-dir", "/tmp/test-pi-sessions",
		"--env", "staging",
		"--log-level", "debug",
	})

	// Execute without running RunE
	if err := cmd.ParseFlags([]string{
		"--codex-sessions-dir", "/tmp/test-codex-sessions",
		"--opencode-db-path", "/tmp/test-opencode.db",
		"--claude-sessions-dir", "/tmp/test-claude-sessions",
		"--pi-sessions-dir", "/tmp/test-pi-sessions",
		"--env", "staging",
		"--log-level", "debug",
	}); err != nil {
		t.Fatalf("cmd.ParseFlags() error: %v", err)
	}

	if opts.codexSessionsDir != "/tmp/test-codex-sessions" {
		t.Errorf("codexSessionsDir = %q, want '/tmp/test-codex-sessions'", opts.codexSessionsDir)
	}
	if opts.opencodeDBPath != "/tmp/test-opencode.db" {
		t.Errorf("opencodeDBPath = %q, want '/tmp/test-opencode.db'", opts.opencodeDBPath)
	}
	if opts.claudeSessionsDir != "/tmp/test-claude-sessions" {
		t.Errorf("claudeSessionsDir = %q, want '/tmp/test-claude-sessions'", opts.claudeSessionsDir)
	}
	if opts.piSessionsDir != "/tmp/test-pi-sessions" {
		t.Errorf("piSessionsDir = %q, want '/tmp/test-pi-sessions'", opts.piSessionsDir)
	}
	if opts.envFlag != "staging" {
		t.Errorf("envFlag = %q, want 'staging'", opts.envFlag)
	}
	if opts.logLevel != "debug" {
		t.Errorf("logLevel = %q, want 'debug'", opts.logLevel)
	}
}

func TestCLIHelpAndVersion(t *testing.T) {
	opts := &observerOptions{}

	t.Run("help output", func(t *testing.T) {
		cmd := newRootCmd(opts)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--help"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("help command error: %v", err)
		}

		out := buf.String()
		if out == "" {
			t.Errorf("expected non-empty help output")
		}
	})

	t.Run("version output", func(t *testing.T) {
		cmd := newRootCmd(opts)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--version"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("version command error: %v", err)
		}

		out := buf.String()
		if out == "" {
			t.Errorf("expected non-empty version output")
		}
	})
}

func TestParserDispatchForHarnessTypes(t *testing.T) {
	tests := []struct {
		name        string
		harness     types.HarnessType
		line        []byte
		cwd         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "claude-code valid line",
			harness: types.HarnessClaudeCode,
			line:    []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`),
			wantErr: false,
		},
		{
			name:    "pi valid line",
			harness: types.HarnessPi,
			line:    []byte(`{"type":"session","id":"pi-sess-1","cwd":"/path"}`),
			cwd:     "/path",
			wantErr: false,
		},
		{
			name:    "codex valid line",
			harness: types.HarnessCodex,
			line:    []byte(`{"type":"session_meta","payload":{"id":"codex-1","cwd":"/path"}}`),
			wantErr: false,
		},
		{
			name:        "opencode in line tailer returns error",
			harness:     types.HarnessOpencode,
			line:        []byte(`{}`),
			wantErr:     true,
			errContains: "unsupported harness type",
		},
		{
			name:        "unknown harness returns error",
			harness:     types.HarnessType("unknown"),
			line:        []byte(`{}`),
			wantErr:     true,
			errContains: "unsupported harness type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, pErr := parseLineForHarness(tt.harness, tt.line, tt.cwd)

			if (pErr != nil) != tt.wantErr {
				t.Fatalf("parseLineForHarness(%v) error = %v, wantErr %v", tt.harness, pErr, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" {
				if pErr == nil || !bytes.Contains([]byte(pErr.Error()), []byte(tt.errContains)) {
					t.Errorf("error %v does not contain %q", pErr, tt.errContains)
				}
			}
			if !tt.wantErr && pErr == nil && len(events) == 0 {
				t.Errorf("expected at least 1 parsed event for valid line")
			}
		})
	}
}
