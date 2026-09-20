package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestObserverCommandHelp(t *testing.T) {
	cmd, _ := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("executing --help: %v", err)
	}

	out := buf.String()
	requiredHelpStrings := []string{
		"air-observer",
		"--ingester-url",
		"--token",
		"--env",
		"--scan-interval",
		"--cursor-path",
		"--claude-sessions-dir",
		"--claude-projects-dir",
		"--pi-sessions-dir",
		"--codex-sessions-dir",
		"--opencode-db-path",
		"--log-level",
	}

	for _, s := range requiredHelpStrings {
		if !strings.Contains(out, s) {
			t.Errorf("expected help output to contain %q, but got:\n%s", s, out)
		}
	}
}

func TestObserverCommandVersion(t *testing.T) {
	cmd, _ := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("executing --version: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, version) {
		t.Errorf("expected version output to contain %q, got %q", version, out)
	}
}

func TestObserverCommandFlagDefaults(t *testing.T) {
	cmd, opts := newRootCmd()
	cmd.SetArgs([]string{})

	if opts.ingesterURL != "ws://127.0.0.1:4501/ws/ingest" {
		t.Errorf("default ingesterURL = %q, want ws://127.0.0.1:4501/ws/ingest", opts.ingesterURL)
	}
	if opts.scanInterval != 2*time.Second {
		t.Errorf("default scanInterval = %v, want 2s", opts.scanInterval)
	}
	if opts.logLevel != "info" {
		t.Errorf("default logLevel = %q, want info", opts.logLevel)
	}
	if opts.token != "" {
		t.Errorf("default token = %q, want empty", opts.token)
	}
}

func TestObserverCommandFlagParsing(t *testing.T) {
	cmd, opts := newRootCmd()
	args := []string{
		"--ingester-url", "ws://remote:9999/ws",
		"--token", "secret-token-123",
		"--env", "staging-box",
		"--scan-interval", "5s",
		"--cursor-path", "/tmp/cursors.json",
		"--claude-sessions-dir", "/tmp/claude/sessions",
		"--claude-projects-dir", "/tmp/claude/projects",
		"--pi-sessions-dir", "/tmp/pi/sessions",
		"--codex-sessions-dir", "/tmp/codex/sessions",
		"--opencode-db-path", "/tmp/opencode.db",
		"--log-level", "debug",
	}

	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags failed: %v", err)
	}

	if opts.ingesterURL != "ws://remote:9999/ws" {
		t.Errorf("ingesterURL = %q, want ws://remote:9999/ws", opts.ingesterURL)
	}
	if opts.token != "secret-token-123" {
		t.Errorf("token = %q, want secret-token-123", opts.token)
	}
	if opts.envFlag != "staging-box" {
		t.Errorf("envFlag = %q, want staging-box", opts.envFlag)
	}
	if opts.scanInterval != 5*time.Second {
		t.Errorf("scanInterval = %v, want 5s", opts.scanInterval)
	}
	if opts.cursorPath != "/tmp/cursors.json" {
		t.Errorf("cursorPath = %q, want /tmp/cursors.json", opts.cursorPath)
	}
	if opts.claudeSessionsDir != "/tmp/claude/sessions" {
		t.Errorf("claudeSessionsDir = %q, want /tmp/claude/sessions", opts.claudeSessionsDir)
	}
	if opts.claudeProjectsDir != "/tmp/claude/projects" {
		t.Errorf("claudeProjectsDir = %q, want /tmp/claude/projects", opts.claudeProjectsDir)
	}
	if opts.piSessionsDir != "/tmp/pi/sessions" {
		t.Errorf("piSessionsDir = %q, want /tmp/pi/sessions", opts.piSessionsDir)
	}
	if opts.codexSessionsDir != "/tmp/codex/sessions" {
		t.Errorf("codexSessionsDir = %q, want /tmp/codex/sessions", opts.codexSessionsDir)
	}
	if opts.opencodeDBPath != "/tmp/opencode.db" {
		t.Errorf("opencodeDBPath = %q, want /tmp/opencode.db", opts.opencodeDBPath)
	}
	if opts.logLevel != "debug" {
		t.Errorf("logLevel = %q, want debug", opts.logLevel)
	}
}

func TestObserverCommandEnvFallbacks(t *testing.T) {
	t.Setenv("AGENT_INGESTER_URL", "ws://env-ingester:8080/ws/ingest")
	t.Setenv("AGENT_STATUS_TOKEN", "env-token-xyz")

	_, opts := newRootCmd()

	if opts.ingesterURL != "ws://env-ingester:8080/ws/ingest" {
		t.Errorf("ingesterURL from env = %q, want ws://env-ingester:8080/ws/ingest", opts.ingesterURL)
	}
	if opts.token != "env-token-xyz" {
		t.Errorf("token from env = %q, want env-token-xyz", opts.token)
	}
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
				if pErr == nil || !strings.Contains(pErr.Error(), tt.errContains) {
					t.Errorf("error %v does not contain %q", pErr, tt.errContains)
				}
			}
			if !tt.wantErr && pErr == nil && len(events) == 0 {
				t.Errorf("expected at least 1 parsed event for valid line")
			}
		})
	}
}
