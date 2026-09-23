package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/config"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/reconciler"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

func TestIngesterCommandHelp(t *testing.T) {
	cfg := config.Config{}
	cmd := newRootCmd(&cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("executing --help: %v", err)
	}

	out := buf.String()
	requiredHelpStrings := []string{
		"air-ingester",
		"--addr",
		"--token",
		"--db",
		"--notify-sock",
		"--no-summarize",
		"--litellm-url",
		"--litellm-key",
		"--summarizer-model",
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

func TestIngesterCommandVersion(t *testing.T) {
	cfg := config.Config{}
	cmd := newRootCmd(&cfg)
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

func TestIngesterCommandFlagBinding(t *testing.T) {
	cfg := config.Config{
		Addr:            "127.0.0.1:4501",
		DBPath:          "/tmp/default.db",
		NotifySock:      "/tmp/default.sock",
		NoSummarize:     false,
		SummarizerModel: "gemini-2.5-flash",
		LogLevel:        "info",
	}

	cmd := newRootCmd(&cfg)
	args := []string{
		"--addr", "0.0.0.0:9000",
		"--token", "secret-ingest-token",
		"--db", "/var/data/custom.db",
		"--notify-sock", "/var/run/custom.sock",
		"--no-summarize",
		"--litellm-url", "https://proxy.example.com/v1",
		"--litellm-key", "sk-litellm-xyz",
		"--summarizer-model", "claude-3-5-haiku",
		"--log-level", "debug",
		"--log-dir", "/tmp/logs",
		"--log-file", "/tmp/logs/ingester.log",
	}

	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags failed: %v", err)
	}

	if cfg.Addr != "0.0.0.0:9000" {
		t.Errorf("Addr = %q, want 0.0.0.0:9000", cfg.Addr)
	}
	if cfg.BearerToken != "secret-ingest-token" {
		t.Errorf("BearerToken = %q, want secret-ingest-token", cfg.BearerToken)
	}
	if cfg.DBPath != "/var/data/custom.db" {
		t.Errorf("DBPath = %q, want /var/data/custom.db", cfg.DBPath)
	}
	if cfg.NotifySock != "/var/run/custom.sock" {
		t.Errorf("NotifySock = %q, want /var/run/custom.sock", cfg.NotifySock)
	}
	if !cfg.NoSummarize {
		t.Errorf("NoSummarize = false, want true")
	}
	if cfg.LiteLLMBaseURL != "https://proxy.example.com/v1" {
		t.Errorf("LiteLLMBaseURL = %q, want https://proxy.example.com/v1", cfg.LiteLLMBaseURL)
	}
	if cfg.LiteLLMAPIKey != "sk-litellm-xyz" {
		t.Errorf("LiteLLMAPIKey = %q, want sk-litellm-xyz", cfg.LiteLLMAPIKey)
	}
	if cfg.SummarizerModel != "claude-3-5-haiku" {
		t.Errorf("SummarizerModel = %q, want claude-3-5-haiku", cfg.SummarizerModel)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.LogDir != "/tmp/logs" {
		t.Errorf("LogDir = %q, want /tmp/logs", cfg.LogDir)
	}
	if cfg.LogFile != "/tmp/logs/ingester.log" {
		t.Errorf("LogFile = %q, want /tmp/logs/ingester.log", cfg.LogFile)
	}
}

func TestIngesterDefaultConfig(t *testing.T) {
	cfg := config.Load()
	cmd := newRootCmd(cfg)

	// Verify boolean default is preserved
	flag := cmd.Flags().Lookup("no-summarize")
	if flag == nil {
		t.Fatal("missing --no-summarize flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected --no-summarize default value to be 'false', got %q", flag.DefValue)
	}
}

func TestLogSummarizerStartup(t *testing.T) {
	origLogger := logging.Default()
	t.Cleanup(func() {
		logging.SetDefault(origLogger)
	})

	t.Run("disabled by flag", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{NoSummarize: true}
		logSummarizerStartup(cfg, nil)

		out := buf.String()
		if !strings.Contains(out, "summarizer_disabled") {
			t.Errorf("expected summarizer_disabled log, got: %s", out)
		}
		if !strings.Contains(out, "flag --no-summarize was provided") {
			t.Errorf("expected reason in log, got: %s", out)
		}
	})

	t.Run("unconfigured credentials", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{NoSummarize: false}
		sumEngine, err := summarizer.New(summarizer.Config{NoSummarize: true})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_unconfigured") {
			t.Errorf("expected summarizer_unconfigured log, got: %s", out)
		}
		if !strings.Contains(out, "no LLM provider credentials found") {
			t.Errorf("expected warning reason in log, got: %s", out)
		}
	})

	t.Run("ready with active provider", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{NoSummarize: false}
		p, err := openai.New(anyllm.WithAPIKey("dummy-key"))
		if err != nil {
			t.Fatalf("openai.New failed: %v", err)
		}
		sumEngine, err := summarizer.New(summarizer.Config{
			Providers: []summarizer.ProviderConfig{
				{
					Name:     "openai",
					Provider: p,
					Model:    "gpt-4o-mini",
					Endpoint: "https://litellm.example.com",
				},
			},
		})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_ready") {
			t.Errorf("expected summarizer_ready log, got: %s", out)
		}
		if !strings.Contains(out, "openai") || !strings.Contains(out, "gpt-4o-mini") || !strings.Contains(out, "https://litellm.example.com") {
			t.Errorf("expected provider, model and endpoint in log, got: %s", out)
		}
	})

	t.Run("sanitizes endpoint credentials in log", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{NoSummarize: false}
		p, err := openai.New(anyllm.WithAPIKey("dummy-key"))
		if err != nil {
			t.Fatalf("openai.New failed: %v", err)
		}
		sumEngine, err := summarizer.New(summarizer.Config{
			Providers: []summarizer.ProviderConfig{
				{
					Name:     "openai",
					Provider: p,
					Model:    "gpt-4o-mini",
					Endpoint: "https://user:secret@proxy.example.com/v1?api_key=secret-token",
				},
			},
		})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if strings.Contains(out, "secret") || strings.Contains(out, "user") {
			t.Errorf("log output must not contain credentials or secret query params, got: %s", out)
		}
		if !strings.Contains(out, "https://proxy.example.com/v1") {
			t.Errorf("expected sanitized endpoint https://proxy.example.com/v1 in log, got: %s", out)
		}
	})
}

type testBroadcaster struct {
	mu     sync.Mutex
	events []types.IServerEvent
}

func (b *testBroadcaster) Publish(event types.IServerEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
}

type testProcessChecker struct {
	alivePIDs map[int]bool
}

func (c *testProcessChecker) IsProcessAlive(pid int, startedAt int64) bool {
	return c.alivePIDs[pid]
}

type errReconciler struct {
	err error
}

func (e *errReconciler) Reconcile(ctx context.Context, envID string) (int, error) {
	return 0, e.err
}

type countingReconciler struct {
	mu          sync.Mutex
	calls       int
	returnCount int
}

func (c *countingReconciler) Reconcile(ctx context.Context, envID string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.returnCount, nil
}

func TestPeriodicReconciliationSweep(t *testing.T) {
	origLogger := logging.Default()
	t.Cleanup(func() {
		logging.SetDefault(origLogger)
	})

	t.Run("logs aggregate count when dead pid is reconciled", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		dbPath := filepath.Join(t.TempDir(), "test_periodic_reconcile.db")
		st, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("store.Open failed: %v", err)
		}
		defer func() { _ = st.Close() }()

		broadcaster := &testBroadcaster{}
		checker := &testProcessChecker{
			alivePIDs: map[int]bool{
				3001: false, // dead PID
			},
		}

		localHost := "local-test-box"
		rec := reconciler.New(
			st,
			broadcaster,
			reconciler.WithProcessChecker(checker),
			reconciler.WithLocalHostname(localHost),
		)

		now := time.Now().UnixMilli()
		deadAgent := types.IAgentState{
			ID:        "agent-periodic-dead",
			SessionID: "sess-dead-periodic",
			Harness:   types.HarnessPi,
			Environment: types.IEnvironmentInfo{
				ID:       "env-local",
				Hostname: localHost,
				Platform: "linux",
			},
			Activity: types.IAgentActivity{
				Lifecycle:      types.LifecycleRunning,
				Activity:       types.ActivityBusy,
				FirstStartedAt: now - 3600000,
				UpdatedAt:      now - 1800000,
				CurrentRun: &types.IRunInfo{
					ID:        "run-dead-periodic",
					PID:       3001,
					StartedAt: now - 3600000,
				},
			},
		}
		if err := st.SaveAgentState(deadAgent); err != nil {
			t.Fatalf("SaveAgentState failed: %v", err)
		}

		ctx := context.Background()
		count, err := runReconciliationSweep(ctx, rec, "periodic")
		if err != nil {
			t.Fatalf("runReconciliationSweep failed: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected reconciled count 1, got %d", count)
		}

		out := buf.String()
		if !strings.Contains(out, "process_reconciliation_periodic_complete") {
			t.Errorf("expected process_reconciliation_periodic_complete in log, got: %s", out)
		}
		if !strings.Contains(out, "reconciledCount=1") {
			t.Errorf("expected reconciledCount=1 in log, got: %s", out)
		}
	})

	t.Run("periodic sweep with zero reconciled agents remains silent", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		dbPath := filepath.Join(t.TempDir(), "test_periodic_silent.db")
		st, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("store.Open failed: %v", err)
		}
		defer func() { _ = st.Close() }()

		rec := reconciler.New(
			st,
			&testBroadcaster{},
			reconciler.WithProcessChecker(&testProcessChecker{alivePIDs: map[int]bool{}}),
			reconciler.WithLocalHostname("test-box"),
		)

		ctx := context.Background()
		count, err := runReconciliationSweep(ctx, rec, "periodic")
		if err != nil {
			t.Fatalf("runReconciliationSweep failed: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected reconciled count 0, got %d", count)
		}

		out := buf.String()
		if strings.Contains(out, "process_reconciliation_periodic_complete") {
			t.Errorf("expected no completion log for count=0, got: %s", out)
		}
	})

	t.Run("periodic sweep failure logs warning", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		rec := &errReconciler{err: fmt.Errorf("disk failure")}
		ctx := context.Background()
		_, err := runReconciliationSweep(ctx, rec, "periodic")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		out := buf.String()
		if !strings.Contains(out, "process_reconciliation_periodic_failed") {
			t.Errorf("expected process_reconciliation_periodic_failed in log, got: %s", out)
		}
	})

	t.Run("startPeriodicReconciler executes on ticker and exits on context cancel", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		rec := &countingReconciler{returnCount: 2}
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			startPeriodicReconciler(ctx, rec, 5*time.Millisecond)
			close(done)
		}()

		// Wait for at least one tick
		time.Sleep(25 * time.Millisecond)
		cancel()

		select {
		case <-done:
			// Clean shutdown confirmed
		case <-time.After(1 * time.Second):
			t.Fatal("startPeriodicReconciler did not exit after context cancellation")
		}

		out := buf.String()
		if !strings.Contains(out, "process_reconciliation_periodic_complete") {
			t.Errorf("expected periodic completion log, got: %s", out)
		}
	})

	t.Run("startup sweep logs aggregate count", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		rec := &countingReconciler{returnCount: 3}
		ctx := context.Background()
		count, err := runReconciliationSweep(ctx, rec, "startup")
		if err != nil {
			t.Fatalf("runReconciliationSweep failed: %v", err)
		}
		if count != 3 {
			t.Fatalf("expected count 3, got %d", count)
		}

		out := buf.String()
		if !strings.Contains(out, "process_reconciliation_startup_complete") {
			t.Errorf("expected process_reconciliation_startup_complete in log, got: %s", out)
		}
		if !strings.Contains(out, "reconciledCount=3") {
			t.Errorf("expected reconciledCount=3 in log, got: %s", out)
		}
	})
}


