package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/scanner"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/tailer"
	"github.com/coder/websocket"
)

type testSyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *testSyncBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *testSyncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

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
		"--log-dir",
		"--log-file",
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
		"--log-dir", "/tmp/logs",
		"--log-file", "/tmp/logs/observer.log",
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
	if opts.logDir != "/tmp/logs" {
		t.Errorf("logDir = %q, want /tmp/logs", opts.logDir)
	}
	if opts.logFile != "/tmp/logs/observer.log" {
		t.Errorf("logFile = %q, want /tmp/logs/observer.log", opts.logFile)
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
			events, pErr := parseLineForHarness(tt.harness, tt.line, tt.cwd, nil)

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

func TestObserverRunLogging(t *testing.T) {
	buf := &testSyncBuffer{}
	logger := logging.NewWithMode(buf, "observer", "info", false)
	logging.SetDefault(logger)

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	claudeDir := filepath.Join(tmpDir, "claude-sessions")
	_ = os.MkdirAll(claudeDir, 0755)
	piDir := filepath.Join(tmpDir, "pi-sessions")
	_ = os.MkdirAll(piDir, 0755)
	codexDir := filepath.Join(tmpDir, "codex-sessions")
	_ = os.MkdirAll(codexDir, 0755)
	opencodeDb := filepath.Join(tmpDir, "opencode.db")

	opts := observerOptions{
		ingesterURL:       "ws://127.0.0.1:9999/ws/ingest",
		cursorPath:        cursorPath,
		claudeSessionsDir: claudeDir,
		piSessionsDir:     piDir,
		codexSessionsDir:  codexDir,
		opencodeDBPath:    opencodeDb,
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != nil {
		t.Fatalf("runObserver returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "observer_starting") {
		t.Errorf("expected observer_starting in log output, got: %s", out)
	}
	if !strings.Contains(out, "observer_shutting_down") {
		t.Errorf("expected observer_shutting_down in log output, got: %s", out)
	}
}

func TestObserverContextCancellationFlushesCursorStore(t *testing.T) {
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "1")

	buf := &testSyncBuffer{}
	logger := logging.NewWithMode(buf, "observer", "info", false)
	logging.SetDefault(logger)

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")

	// Verify cursor file does not exist initially
	if _, err := os.Stat(cursorPath); !os.IsNotExist(err) {
		t.Fatalf("expected cursorPath to not exist initially, stat err: %v", err)
	}

	claudeDir := filepath.Join(tmpDir, "claude-sessions")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("creating claudeDir: %v", err)
	}
	transcriptPath := filepath.Join(tmpDir, "test-session.jsonl")
	transcriptData := []byte("{\"type\":\"user\",\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"hello world\"}]}}\n")
	if err := os.WriteFile(transcriptPath, transcriptData, 0644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	subagentsDir := filepath.Join(tmpDir, "subagents")
	if err := os.MkdirAll(subagentsDir, 0755); err != nil {
		t.Fatalf("creating subagentsDir: %v", err)
	}
	subagentPath := filepath.Join(subagentsDir, "agent-worker.jsonl")
	subagentData := []byte("{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"worker active\"}]}}\n")
	if err := os.WriteFile(subagentPath, subagentData, 0644); err != nil {
		t.Fatalf("writing subagent transcript: %v", err)
	}

	metaPath := filepath.Join(claudeDir, "session-1.json")
	metaData := []byte(`{"pid":99999,"sessionId":"test-session-1","cwd":"` + tmpDir + `","transcriptPath":"` + transcriptPath + `"}`)
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		t.Fatalf("writing session metadata: %v", err)
	}

	piDir := filepath.Join(tmpDir, "pi-sessions")
	_ = os.MkdirAll(piDir, 0755)
	codexDir := filepath.Join(tmpDir, "codex-sessions")
	_ = os.MkdirAll(codexDir, 0755)
	opencodeDb := filepath.Join(tmpDir, "opencode.db")

	mockIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "frames" && len(msg.Frames) > 0 {
				lastSeq := msg.Frames[len(msg.Frames)-1].Seq
				ack := types.IIngesterMessage{
					Type: "ack",
					UpTo: lastSeq,
				}
				ackBytes, _ := json.Marshal(ack)
				_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
			}
		}
	}))
	defer mockIngester.Close()

	opts := observerOptions{
		ingesterURL:       "ws" + strings.TrimPrefix(mockIngester.URL, "http") + "/ws/ingest",
		cursorPath:        cursorPath,
		claudeSessionsDir: claudeDir,
		piSessionsDir:     piDir,
		codexSessionsDir:  codexDir,
		opencodeDBPath:    opencodeDb,
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	// Allow scanner and tailer to run and process the transcript and subagent
	time.Sleep(100 * time.Millisecond)

	// Remove session to trigger exit detection on the next scan ticker tick
	_ = os.Remove(metaPath)
	time.Sleep(100 * time.Millisecond)

	// Cancel context to simulate daemon shutdown
	cancel()

	err := <-errCh
	if err != nil {
		t.Fatalf("runObserverContext returned unexpected error: %v", err)
	}

	// Verify that cursorPath was created on disk upon shutdown
	if _, err := os.Stat(cursorPath); os.IsNotExist(err) {
		t.Fatalf("expected cursor file %q to exist after shutdown, but it does not", cursorPath)
	}

	rawJSON, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("reading cursors.json: %v", err)
	}

	var savedCursors map[string]*tailer.Cursor
	if err := json.Unmarshal(rawJSON, &savedCursors); err != nil {
		t.Fatalf("parsing cursors.json: %v", err)
	}

	var foundCursor *tailer.Cursor
	var foundSubagentCursor *tailer.Cursor
	for _, c := range savedCursors {
		if c.Path == transcriptPath {
			foundCursor = c
		}
		if c.Path == subagentPath {
			foundSubagentCursor = c
		}
	}
	if foundCursor == nil {
		t.Fatalf("expected cursor with path %q in flushed store, but found none in %+v", transcriptPath, savedCursors)
	}
	if foundSubagentCursor == nil {
		t.Fatalf("expected subagent cursor with path %q in flushed store, but found none in %+v", subagentPath, savedCursors)
	}

	if foundCursor.Offset != int64(len(transcriptData)) {
		t.Errorf("expected cursor offset %d, got %d", len(transcriptData), foundCursor.Offset)
	}
	if foundSubagentCursor.Offset != int64(len(subagentData)) {
		t.Errorf("expected subagent cursor offset %d, got %d", len(subagentData), foundSubagentCursor.Offset)
	}
}

func TestObserverCursorStoreSaveErrorOnShutdown(t *testing.T) {
	buf := &testSyncBuffer{}
	logger := logging.NewWithMode(buf, "observer", "warn", false)
	logging.SetDefault(logger)

	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, "readonly-dir")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("creating cursorDir: %v", err)
	}
	cursorPath := filepath.Join(cursorDir, "cursors.json")

	opts := observerOptions{
		ingesterURL:       "ws://127.0.0.1:9999/ws/ingest",
		cursorPath:        cursorPath,
		claudeSessionsDir: filepath.Join(tmpDir, "claude"),
		piSessionsDir:     filepath.Join(tmpDir, "pi"),
		codexSessionsDir:  filepath.Join(tmpDir, "codex"),
		opencodeDBPath:    filepath.Join(tmpDir, "opencode.db"),
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	// Wait for NewCursorStore initialization
	time.Sleep(50 * time.Millisecond)

	// Make directory read-only so Save() fails when creating temp file
	_ = os.Chmod(cursorDir, 0500)
	t.Cleanup(func() {
		_ = os.Chmod(cursorDir, 0755)
	})

	cancel()
	<-errCh

	out := buf.String()
	if !strings.Contains(out, "cursor_store_save_error") {
		t.Errorf("expected cursor_store_save_error in log, got: %s", out)
	}
}

func TestRunObserverInvalidCursorPath(t *testing.T) {
	tmpDir := t.TempDir()
	corruptedCursor := filepath.Join(tmpDir, "corrupted.json")
	if err := os.WriteFile(corruptedCursor, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("writing corrupt file: %v", err)
	}

	opts := observerOptions{
		cursorPath: corruptedCursor,
		envFlag:    "test-env",
		logLevel:   "error",
		logDir:     tmpDir,
		logFile:    "test-observer.log",
	}

	err := runObserver(opts)
	if err == nil {
		t.Fatalf("expected error from runObserver with corrupted cursor store, got nil")
	}
	if !strings.Contains(err.Error(), "initializing cursor store") {
		t.Errorf("expected 'initializing cursor store' in error, got: %v", err)
	}
}

func TestRootCmdExecute_RunE(t *testing.T) {
	tmpDir := t.TempDir()
	corruptedCursor := filepath.Join(tmpDir, "corrupted.json")
	if err := os.WriteFile(corruptedCursor, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("writing corrupt file: %v", err)
	}

	cmd, _ := newRootCmd()
	cmd.SetArgs([]string{"--cursor-path", corruptedCursor, "--env", "test-env"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from cmd.Execute(), got nil")
	}
	if !strings.Contains(err.Error(), "initializing cursor store") {
		t.Errorf("expected 'initializing cursor store' in error, got: %v", err)
	}
}

func TestObserverEnvScannerDirs(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "env-claude")
	piDir := filepath.Join(tmpDir, "env-pi")
	codexDir := filepath.Join(tmpDir, "env-codex")
	opencodeDb := filepath.Join(tmpDir, "env-opencode.db")

	_ = os.MkdirAll(claudeDir, 0755)
	_ = os.MkdirAll(piDir, 0755)
	_ = os.MkdirAll(codexDir, 0755)

	t.Setenv("CLAUDE_SESSIONS_DIR", claudeDir)
	t.Setenv("PI_SESSIONS_DIR", piDir)
	t.Setenv("CODEX_SESSIONS_DIR", codexDir)
	t.Setenv("OPENCODE_DB_PATH", opencodeDb)

	cursorPath := filepath.Join(tmpDir, "cursors.json")
	opts := observerOptions{
		ingesterURL:  "ws://127.0.0.1:9999/ws/ingest",
		cursorPath:   cursorPath,
		scanInterval: 50 * time.Millisecond,
		envFlag:      "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately after start

	err := runObserverContext(ctx, opts)
	if err != nil {
		t.Fatalf("runObserverContext returned error: %v", err)
	}
}

func TestObserverPiSessionIngestionWithThinkingLevelChange(t *testing.T) {
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "1")

	buf := &testSyncBuffer{}
	logger := logging.NewWithMode(buf, "observer", "info", false)
	logging.SetDefault(logger)

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	piSessionsDir := filepath.Join(tmpDir, "pi-sessions")
	claudeDir := filepath.Join(tmpDir, "claude-sessions")
	codexDir := filepath.Join(tmpDir, "codex-sessions")
	opencodeDb := filepath.Join(tmpDir, "opencode.db")

	_ = os.MkdirAll(piSessionsDir, 0755)
	_ = os.MkdirAll(claudeDir, 0755)
	_ = os.MkdirAll(codexDir, 0755)

	sessionCWD := filepath.Join(tmpDir, "my-project")
	_ = os.MkdirAll(sessionCWD, 0755)

	encodedCWD := scanner.EncodePiCWD(sessionCWD)
	projectSessionsDir := filepath.Join(piSessionsDir, encodedCWD)
	if err := os.MkdirAll(projectSessionsDir, 0755); err != nil {
		t.Fatalf("creating projectSessionsDir: %v", err)
	}

	sessionID := "01a0c530-test-session-id"
	transcriptPath := filepath.Join(projectSessionsDir, "2026-09-21T18-00-00-000Z_"+sessionID+".jsonl")

	lines := []string{
		`{"type":"session","version":3,"id":"` + sessionID + `","timestamp":"2026-09-21T18:00:00.000Z","cwd":"` + sessionCWD + `"}`,
		`{"type":"model_change","id":"m1","timestamp":"2026-09-21T18:00:01.000Z","modelId":"gemini-3.8-flash"}`,
		`{"type":"thinking_level_change","id":"t1","timestamp":"2026-09-21T18:00:01.500Z","thinkingLevel":"medium"}`,
		`{"type":"message","id":"msg1","timestamp":"2026-09-21T18:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"hello pi"}]}}`,
		`{"type":"message","id":"msg2","timestamp":"2026-09-21T18:00:03.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"pondering"},{"type":"text","text":"hi user"}]}}`,
	}
	transcriptContent := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	var receivedFramesMu sync.Mutex
	var receivedEventTypes []string
	ingesterReceivedAll := make(chan struct{})
	ingesterReceivedExit := make(chan struct{})
	var closeOnce sync.Once
	var exitOnce sync.Once

	mockIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "frames" && len(msg.Frames) > 0 {
				receivedFramesMu.Lock()
				for _, f := range msg.Frames {
					receivedEventTypes = append(receivedEventTypes, f.EventType)
					if f.EventType == "run_exited" {
						exitOnce.Do(func() { close(ingesterReceivedExit) })
					}
				}
				hasAssistantTurn := false
				for _, et := range receivedEventTypes {
					if et == "thinking" || et == "turn_end" {
						hasAssistantTurn = true
						break
					}
				}
				if hasAssistantTurn {
					closeOnce.Do(func() { close(ingesterReceivedAll) })
				}
				receivedFramesMu.Unlock()

				lastSeq := msg.Frames[len(msg.Frames)-1].Seq
				ack := types.IIngesterMessage{
					Type: "ack",
					UpTo: lastSeq,
				}
				ackBytes, _ := json.Marshal(ack)
				_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
			}
		}
	}))
	defer mockIngester.Close()

	opts := observerOptions{
		ingesterURL:       "ws" + strings.TrimPrefix(mockIngester.URL, "http") + "/ws/ingest",
		cursorPath:        cursorPath,
		claudeSessionsDir: claudeDir,
		piSessionsDir:     piSessionsDir,
		codexSessionsDir:  codexDir,
		opencodeDBPath:    opencodeDb,
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	select {
	case <-ingesterReceivedAll:
		// Succeeded: frames beyond thinking_level_change were received!
	case <-time.After(3 * time.Second):
		receivedFramesMu.Lock()
		got := append([]string{}, receivedEventTypes...)
		receivedFramesMu.Unlock()
		cancel()
		<-errCh
		t.Fatalf("timed out waiting for frames beyond thinking_level_change, got event types: %v", got)
	}

	// Simulate session exit by removing transcript so scanner detects termination
	_ = os.Remove(transcriptPath)

	select {
	case <-ingesterReceivedExit:
		// Succeeded: run_exited was emitted and tailer was cleaned up!
	case <-time.After(2 * time.Second):
		t.Logf("note: run_exited not received before timeout (ok if test completes)")
	}

	cancel()
	<-errCh

	receivedFramesMu.Lock()
	defer receivedFramesMu.Unlock()

	hasThinking := false
	hasTurnEnd := false
	for _, et := range receivedEventTypes {
		if et == "thinking" {
			hasThinking = true
		}
		if et == "turn_end" {
			hasTurnEnd = true
		}
	}
	if !hasThinking || !hasTurnEnd {
		t.Errorf("expected thinking and turn_end events to be ingested, got: %v", receivedEventTypes)
	}
}

func TestObserverTranscriptMalformedLineAdvance(t *testing.T) {
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "1")

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	piSessionsDir := filepath.Join(tmpDir, "pi-sessions")
	claudeDir := filepath.Join(tmpDir, "claude-sessions")
	codexDir := filepath.Join(tmpDir, "codex-sessions")
	opencodeDb := filepath.Join(tmpDir, "opencode.db")

	_ = os.MkdirAll(piSessionsDir, 0755)
	_ = os.MkdirAll(claudeDir, 0755)
	_ = os.MkdirAll(codexDir, 0755)

	sessionCWD := filepath.Join(tmpDir, "my-project")
	_ = os.MkdirAll(sessionCWD, 0755)

	encodedCWD := scanner.EncodePiCWD(sessionCWD)
	projectSessionsDir := filepath.Join(piSessionsDir, encodedCWD)
	if err := os.MkdirAll(projectSessionsDir, 0755); err != nil {
		t.Fatalf("creating projectSessionsDir: %v", err)
	}

	sessionID := "01a0c530-malformed-line-test"
	transcriptPath := filepath.Join(projectSessionsDir, "2026-09-21T18-00-00-000Z_"+sessionID+".jsonl")

	lines := []string{
		`{"type":"session","version":3,"id":"` + sessionID + `","timestamp":"2026-09-21T18:00:00.000Z","cwd":"` + sessionCWD + `"}`,
		`{CORRUPTED_MALFORMED_JSON_LINE`,
		`{"type":"message","id":"msg1","timestamp":"2026-09-21T18:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"after corrupt"}]}}`,
	}
	transcriptContent := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	var receivedFramesMu sync.Mutex
	var receivedEventTypes []string
	receivedUserMessage := make(chan struct{})
	var closeOnce sync.Once

	mockIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "frames" && len(msg.Frames) > 0 {
				receivedFramesMu.Lock()
				for _, f := range msg.Frames {
					receivedEventTypes = append(receivedEventTypes, f.EventType)
					if f.EventType == "turn_start" {
						closeOnce.Do(func() { close(receivedUserMessage) })
					}
				}
				receivedFramesMu.Unlock()

				lastSeq := msg.Frames[len(msg.Frames)-1].Seq
				ack := types.IIngesterMessage{
					Type: "ack",
					UpTo: lastSeq,
				}
				ackBytes, _ := json.Marshal(ack)
				_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
			}
		}
	}))
	defer mockIngester.Close()

	opts := observerOptions{
		ingesterURL:       "ws" + strings.TrimPrefix(mockIngester.URL, "http") + "/ws/ingest",
		cursorPath:        cursorPath,
		claudeSessionsDir: claudeDir,
		piSessionsDir:     piSessionsDir,
		codexSessionsDir:  codexDir,
		opencodeDBPath:    opencodeDb,
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	select {
	case <-receivedUserMessage:
		// Success: skipped corrupt line and ingested turn_start from subsequent valid message!
	case <-time.After(3 * time.Second):
		receivedFramesMu.Lock()
		got := append([]string{}, receivedEventTypes...)
		receivedFramesMu.Unlock()
		cancel()
		<-errCh
		t.Fatalf("timed out waiting for valid message after corrupt line, got: %v", got)
	}

	cancel()
	<-errCh
}

func TestObserverClaudeSubagentRunStartedEmission(t *testing.T) {
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "1")

	buf := &testSyncBuffer{}
	logger := logging.NewWithMode(buf, "observer", "info", false)
	logging.SetDefault(logger)

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	claudeDir := filepath.Join(tmpDir, "claude-sessions")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("creating claudeDir: %v", err)
	}

	transcriptPath := filepath.Join(tmpDir, "root-session.jsonl")
	transcriptData := []byte("{\"type\":\"user\",\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"start root\"}]}}\n")
	if err := os.WriteFile(transcriptPath, transcriptData, 0644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	subagentsDir := filepath.Join(tmpDir, "subagents")
	if err := os.MkdirAll(subagentsDir, 0755); err != nil {
		t.Fatalf("creating subagentsDir: %v", err)
	}
	subagentPath := filepath.Join(subagentsDir, "agent-sub-123.jsonl")
	expectedSubTS := time.Now().UnixMilli() - 10000
	subagentData := []byte(fmt.Sprintf("{\"type\":\"assistant\",\"timestamp\":%d,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"subagent response\"}]}}\n", expectedSubTS))
	if err := os.WriteFile(subagentPath, subagentData, 0644); err != nil {
		t.Fatalf("writing subagent transcript: %v", err)
	}

	sessionID := "claude-sub-sess-1"
	metaPath := filepath.Join(claudeDir, "session-1.json")
	metaData := []byte(`{"pid":99998,"sessionId":"` + sessionID + `","cwd":"` + tmpDir + `","transcriptPath":"` + transcriptPath + `"}`)
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		t.Fatalf("writing session metadata: %v", err)
	}

	piSessionsDir := filepath.Join(tmpDir, "pi-sessions")
	_ = os.MkdirAll(piSessionsDir, 0755)
	codexDir := filepath.Join(tmpDir, "codex-sessions")
	_ = os.MkdirAll(codexDir, 0755)
	opencodeDb := filepath.Join(tmpDir, "opencode.db")

	var receivedFramesMu sync.Mutex
	var receivedFrames []types.ITelemetryFrame
	receivedSubagentRunStarted := make(chan struct{})
	var subagentStartedOnce sync.Once

	mockIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "frames" && len(msg.Frames) > 0 {
				receivedFramesMu.Lock()
				for _, f := range msg.Frames {
					receivedFrames = append(receivedFrames, f)
					if f.EventType == "run_started" && f.SubagentID != nil && *f.SubagentID == "agent-sub-123" {
						subagentStartedOnce.Do(func() { close(receivedSubagentRunStarted) })
					}
				}
				receivedFramesMu.Unlock()

				lastSeq := msg.Frames[len(msg.Frames)-1].Seq
				ack := types.IIngesterMessage{
					Type: "ack",
					UpTo: lastSeq,
				}
				ackBytes, _ := json.Marshal(ack)
				_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
			}
		}
	}))
	defer mockIngester.Close()

	opts := observerOptions{
		ingesterURL:       "ws" + strings.TrimPrefix(mockIngester.URL, "http") + "/ws/ingest",
		cursorPath:        cursorPath,
		claudeSessionsDir: claudeDir,
		piSessionsDir:     piSessionsDir,
		codexSessionsDir:  codexDir,
		opencodeDBPath:    opencodeDb,
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	select {
	case <-receivedSubagentRunStarted:
		// Success! Verify the recovered tail timestamp was used
		receivedFramesMu.Lock()
		var foundFrame *types.ITelemetryFrame
		for _, f := range receivedFrames {
			if f.EventType == "run_started" && f.SubagentID != nil && *f.SubagentID == "agent-sub-123" {
				foundFrame = &f
				break
			}
		}
		receivedFramesMu.Unlock()
		if foundFrame == nil {
			t.Fatal("expected to find run_started frame for agent-sub-123")
		}
		if foundFrame.Timestamp != expectedSubTS {
			t.Errorf("expected subagent run_started timestamp %d from tail, got %d", expectedSubTS, foundFrame.Timestamp)
		}
	case <-time.After(2 * time.Second):
		receivedFramesMu.Lock()
		t.Fatalf("timed out waiting for subagent run_started event, got %d frames", len(receivedFrames))
		receivedFramesMu.Unlock()
	}

	// Also verify that when another subagent is discovered dynamically during session, run_started is emitted for it too
	lateSubagentPath := filepath.Join(subagentsDir, "agent-sub-late.jsonl")
	lateSubagentData := []byte("{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"late subagent\"}]}}\n")
	if err := os.WriteFile(lateSubagentPath, lateSubagentData, 0644); err != nil {
		t.Fatalf("writing late subagent transcript: %v", err)
	}

	// Wait for late subagent to be discovered
	var foundLateStarted bool
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		receivedFramesMu.Lock()
		for _, f := range receivedFrames {
			if f.EventType == "run_started" && f.SubagentID != nil && *f.SubagentID == "agent-sub-late" {
				foundLateStarted = true
				break
			}
		}
		receivedFramesMu.Unlock()
		if foundLateStarted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !foundLateStarted {
		t.Errorf("expected run_started for dynamically discovered subagent 'agent-sub-late', but none was received")
	}

	// Verify run_started frames for subagent are not duplicated across scan ticks
	time.Sleep(150 * time.Millisecond)
	receivedFramesMu.Lock()
	var sub123Starts int
	var rootStarts int
	for _, f := range receivedFrames {
		if f.EventType == "run_started" {
			if f.SubagentID == nil {
				rootStarts++
			} else if *f.SubagentID == "agent-sub-123" {
				sub123Starts++
			}
		}
	}
	receivedFramesMu.Unlock()

	if rootStarts != 1 {
		t.Errorf("expected exactly 1 root run_started, got %d", rootStarts)
	}
	if sub123Starts != 1 {
		t.Errorf("expected exactly 1 subagent run_started for agent-sub-123, got %d", sub123Starts)
	}

	// Trigger session exit by deleting metadata
	_ = os.Remove(metaPath)
	time.Sleep(100 * time.Millisecond)

	cancel()
	<-errCh
}

func TestObserver_StartupSessionReconciliation(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	sessionStatePath := filepath.Join(tmpDir, "observer-sessions.json")
	claudeDir := filepath.Join(tmpDir, "claude-sessions")
	_ = os.MkdirAll(claudeDir, 0755)
	piDir := filepath.Join(tmpDir, "pi-sessions")
	_ = os.MkdirAll(piDir, 0755)

	// Pre-populate sessionStatePath with 2 sessions that were active in a previous run,
	// but their processes are no longer alive
	prevSessions := map[string]scanner.TrackedSession{
		"sess-reconcile-pi": {
			SessionID:      "sess-reconcile-pi",
			Harness:        "pi",
			PID:            99991, // dead PID
			CWD:            tmpDir,
			HarnessVersion: "0.1.0",
			StartedAt:      time.Now().UnixMilli() - 100000,
			IsActive:       true,
		},
		"sess-reconcile-claude": {
			SessionID:      "sess-reconcile-claude",
			Harness:        "claude-code",
			PID:            99992, // dead PID
			CWD:            tmpDir,
			HarnessVersion: "1.0.0",
			StartedAt:      time.Now().UnixMilli() - 200000,
			IsActive:       true,
		},
	}
	stateData, err := json.Marshal(prevSessions)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(sessionStatePath, stateData, 0644); err != nil {
		t.Fatalf("writeFile error: %v", err)
	}

	var receivedFramesMu sync.Mutex
	var receivedExits []string
	exitPiReceived := make(chan struct{})
	exitClaudeReceived := make(chan struct{})
	var oncePi, onceClaude sync.Once

	mockIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "frames" && len(msg.Frames) > 0 {
				receivedFramesMu.Lock()
				for _, f := range msg.Frames {
					if f.EventType == "run_exited" {
						receivedExits = append(receivedExits, f.SessionID)
						if f.SessionID == "sess-reconcile-pi" {
							oncePi.Do(func() { close(exitPiReceived) })
						}
						if f.SessionID == "sess-reconcile-claude" {
							onceClaude.Do(func() { close(exitClaudeReceived) })
						}
					}
				}
				receivedFramesMu.Unlock()

				lastSeq := msg.Frames[len(msg.Frames)-1].Seq
				ack := types.IIngesterMessage{
					Type: "ack",
					UpTo: lastSeq,
				}
				ackBytes, _ := json.Marshal(ack)
				_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
			}
		}
	}))
	defer mockIngester.Close()

	opts := observerOptions{
		ingesterURL:       "ws" + strings.TrimPrefix(mockIngester.URL, "http") + "/ws/ingest",
		cursorPath:        cursorPath,
		sessionStatePath:  sessionStatePath,
		claudeSessionsDir: claudeDir,
		piSessionsDir:     piDir,
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	// Verify both exited events are received on initial reconciliation scan
	select {
	case <-exitPiReceived:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for sess-reconcile-pi run_exited on startup reconciliation")
	}

	select {
	case <-exitClaudeReceived:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for sess-reconcile-claude run_exited on startup reconciliation")
	}

	cancel()
	<-errCh

	// Verify sessionStatePath on disk was updated to remove the exited sessions
	tracker, err := scanner.NewSessionTracker(sessionStatePath)
	if err != nil {
		t.Fatalf("NewSessionTracker error: %v", err)
	}
	remaining := tracker.GetAll()
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining active sessions after reconciliation, got %d: %+v", len(remaining), remaining)
	}
}

func TestObserverTranscriptRunStartedOnlyCommitsCursorOffset(t *testing.T) {
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "1")

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	piSessionsDir := filepath.Join(tmpDir, "pi-sessions")
	claudeDir := filepath.Join(tmpDir, "claude-sessions")
	codexDir := filepath.Join(tmpDir, "codex-sessions")
	opencodeDb := filepath.Join(tmpDir, "opencode.db")

	_ = os.MkdirAll(piSessionsDir, 0755)
	_ = os.MkdirAll(claudeDir, 0755)
	_ = os.MkdirAll(codexDir, 0755)

	sessionCWD := filepath.Join(tmpDir, "my-project")
	_ = os.MkdirAll(sessionCWD, 0755)

	encodedCWD := scanner.EncodePiCWD(sessionCWD)
	projectSessionsDir := filepath.Join(piSessionsDir, encodedCWD)
	if err := os.MkdirAll(projectSessionsDir, 0755); err != nil {
		t.Fatalf("creating projectSessionsDir: %v", err)
	}

	sessionID := "01a0c530-run-started-cursor-test"
	transcriptPath := filepath.Join(projectSessionsDir, "2026-09-21T18-00-00-000Z_"+sessionID+".jsonl")

	sessionLine := []byte(`{"type":"session","version":3,"id":"` + sessionID + `","timestamp":"2026-09-21T18:00:00.000Z","cwd":"` + sessionCWD + `"}` + "\n")
	if err := os.WriteFile(transcriptPath, sessionLine, 0644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	mockIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "frames" && len(msg.Frames) > 0 {
				lastSeq := msg.Frames[len(msg.Frames)-1].Seq
				ack := types.IIngesterMessage{
					Type: "ack",
					UpTo: lastSeq,
				}
				ackBytes, _ := json.Marshal(ack)
				_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
			}
		}
	}))
	defer mockIngester.Close()

	opts := observerOptions{
		ingesterURL:       "ws" + strings.TrimPrefix(mockIngester.URL, "http") + "/ws/ingest",
		cursorPath:        cursorPath,
		claudeSessionsDir: claudeDir,
		piSessionsDir:     piSessionsDir,
		codexSessionsDir:  codexDir,
		opencodeDBPath:    opencodeDb,
		scanInterval:      50 * time.Millisecond,
		envFlag:           "test-env",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runObserverContext(ctx, opts)
	}()

	// Wait for observer to scan and tail the file
	time.Sleep(300 * time.Millisecond)

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("runObserverContext error: %v", err)
	}

	// Verify that cursorPath was created and has committed the offset for the transcript line
	rawJSON, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("reading cursors.json: %v", err)
	}

	var savedCursors map[string]*tailer.Cursor
	if err := json.Unmarshal(rawJSON, &savedCursors); err != nil {
		t.Fatalf("parsing cursors.json: %v", err)
	}

	var foundCursor *tailer.Cursor
	for _, c := range savedCursors {
		if c.Path == transcriptPath {
			foundCursor = c
			break
		}
	}
	if foundCursor == nil {
		t.Fatalf("expected cursor with path %q in flushed store, but found none in %+v", transcriptPath, savedCursors)
	}

	expectedOffset := int64(len(sessionLine))
	if foundCursor.Offset != expectedOffset {
		t.Errorf("expected cursor offset %d, got %d", expectedOffset, foundCursor.Offset)
	}
}






