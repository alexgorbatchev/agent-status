package main

import (
	"bytes"
	"context"
	"errors"
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

func TestSyncNoSummarizeSource(t *testing.T) {
	t.Run("nil config is safe", func(t *testing.T) {
		syncNoSummarizeSource(nil, nil)
	})

	t.Run("flag set overrides source to flag", func(t *testing.T) {
		cfg := &config.Config{
			NoSummarize:       false,
			NoSummarizeByEnv:  true,
			NoSummarizeSource: "env",
		}
		cmd := newRootCmd(cfg)
		if err := cmd.ParseFlags([]string{"--no-summarize"}); err != nil {
			t.Fatalf("ParseFlags failed: %v", err)
		}
		syncNoSummarizeSource(cmd, cfg)
		if !cfg.NoSummarizeByFlag {
			t.Errorf("expected NoSummarizeByFlag = true")
		}
		if cfg.NoSummarizeByEnv {
			t.Errorf("expected NoSummarizeByEnv = false")
		}
		if cfg.NoSummarizeSource != "flag" {
			t.Errorf("expected NoSummarizeSource = 'flag', got %q", cfg.NoSummarizeSource)
		}
	})

	t.Run("env set without flag retains env source", func(t *testing.T) {
		cfg := &config.Config{
			NoSummarize:       true,
			NoSummarizeByEnv:  true,
			NoSummarizeSource: "env",
		}
		cmd := newRootCmd(cfg)
		if err := cmd.ParseFlags([]string{}); err != nil {
			t.Fatalf("ParseFlags failed: %v", err)
		}
		syncNoSummarizeSource(cmd, cfg)
		if cfg.NoSummarizeByFlag {
			t.Errorf("expected NoSummarizeByFlag = false")
		}
		if !cfg.NoSummarizeByEnv {
			t.Errorf("expected NoSummarizeByEnv = true")
		}
		if cfg.NoSummarizeSource != "env" {
			t.Errorf("expected NoSummarizeSource = 'env', got %q", cfg.NoSummarizeSource)
		}
	})
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
		sumEngine, err := summarizer.New(summarizer.Config{NoSummarize: true})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_disabled") {
			t.Errorf("expected summarizer_disabled log, got: %s", out)
		}
		if !strings.Contains(out, "source=flag") {
			t.Errorf("expected source=flag in log, got: %s", out)
		}
		if strings.Contains(out, "flag --no-summarize was provided") || strings.Contains(out, "deterministic fallback heuristics") {
			t.Errorf("did not expect prose sentence in log, got: %s", out)
		}
	})

	t.Run("disabled by sumEngine DisabledByFlag", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		// cfg has NoSummarize: false, but sumEngine has NoSummarize: true
		cfg := &config.Config{NoSummarize: false}
		sumEngine, err := summarizer.New(summarizer.Config{NoSummarize: true})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_disabled") {
			t.Errorf("expected summarizer_disabled log when sumEngine.DisabledByFlag() is true, got: %s", out)
		}
		if !strings.Contains(out, "source=flag") {
			t.Errorf("expected source=flag in log, got: %s", out)
		}
		if strings.Contains(out, "flag --no-summarize was provided") || strings.Contains(out, "deterministic fallback heuristics") {
			t.Errorf("did not expect prose sentence in log, got: %s", out)
		}
	})

	t.Run("disabled by env var AIR_NO_SUMMARIZE", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		t.Setenv("AIR_NO_SUMMARIZE", "1")
		cfg := config.Load()
		sumEngine, err := summarizer.New(summarizer.Config{
			NoSummarize:       cfg.NoSummarize,
			NoSummarizeByEnv:  cfg.NoSummarizeByEnv,
			NoSummarizeSource: cfg.NoSummarizeSource,
		})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_disabled") {
			t.Errorf("expected summarizer_disabled log, got: %s", out)
		}
		if !strings.Contains(out, "source=env") {
			t.Errorf("expected source=env in log, got: %s", out)
		}
		if strings.Contains(out, "source=flag") {
			t.Errorf("did not expect source=flag when disabled by env var, got: %s", out)
		}
		if strings.Contains(out, "AIR_NO_SUMMARIZE=1 was provided") || strings.Contains(out, "deterministic fallback heuristics") {
			t.Errorf("did not expect prose sentence in log, got: %s", out)
		}
	})

	t.Run("provider constructor failure logs error and skips unconfigured warning", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{
			NoSummarize:    false,
			LiteLLMBaseURL: "invalid-url-no-scheme", // Causes anyllm/openai constructor to fail
			LiteLLMAPIKey:  "dummy-key",
		}
		sumEngine, err := summarizer.New(summarizer.Config{
			LiteLLMBaseURL: cfg.LiteLLMBaseURL,
			LiteLLMAPIKey:  cfg.LiteLLMAPIKey,
		})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_provider_init_failed") {
			t.Errorf("expected summarizer_provider_init_failed in log, got: %s", out)
		}
		if !strings.Contains(out, "provider=openai") {
			t.Errorf("expected provider=openai in log, got: %s", out)
		}
		if strings.Contains(out, "summarizer_unconfigured") {
			t.Errorf("did not expect summarizer_unconfigured when credentials were provided, got: %s", out)
		}
	})

	t.Run("multiple provider constructor failures log deterministically", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{NoSummarize: false}
		sumEngine, err := summarizer.New(summarizer.Config{
			InitErrors: map[string]error{
				"openai":    errors.New("openai init failure"),
				"gemini":    errors.New("gemini init failure"),
				"anthropic": errors.New("anthropic init failure"),
			},
		})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		// Must log all 3 providers
		if !strings.Contains(out, "provider=anthropic") || !strings.Contains(out, "provider=gemini") || !strings.Contains(out, "provider=openai") {
			t.Errorf("expected all 3 failed providers logged, got: %s", out)
		}
		// Must be sorted: anthropic before gemini before openai
		anthropicIdx := strings.Index(out, "provider=anthropic")
		geminiIdx := strings.Index(out, "provider=gemini")
		openaiIdx := strings.Index(out, "provider=openai")
		if anthropicIdx >= geminiIdx || geminiIdx >= openaiIdx {
			t.Errorf("expected alphabetical order anthropic < gemini < openai, got indices: %d, %d, %d", anthropicIdx, geminiIdx, openaiIdx)
		}
		if strings.Contains(out, "summarizer_unconfigured") {
			t.Errorf("did not expect summarizer_unconfigured when provider constructor errors exist, got: %s", out)
		}
	})

	t.Run("provider failure with active failover provider logs both init error and ready", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{NoSummarize: false}
		p, err := openai.New(anyllm.WithAPIKey("dummy-key"))
		if err != nil {
			t.Fatalf("openai.New failed: %v", err)
		}
		sumEngine, err := summarizer.New(summarizer.Config{
			InitErrors: map[string]error{
				"gemini": errors.New("bad gemini config"),
			},
			Providers: []summarizer.ProviderConfig{
				{
					Name:     "openai",
					Provider: p,
					Model:    "gpt-4o-mini",
					Endpoint: "https://api.openai.com/v1",
				},
			},
		})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_provider_init_failed") || !strings.Contains(out, "provider=gemini") {
			t.Errorf("expected gemini init failure logged, got: %s", out)
		}
		if !strings.Contains(out, "summarizer_ready") || !strings.Contains(out, "provider=openai") {
			t.Errorf("expected openai ready logged, got: %s", out)
		}
		if strings.Contains(out, "summarizer_unconfigured") {
			t.Errorf("did not expect summarizer_unconfigured when active providers exist, got: %s", out)
		}
	})

	t.Run("unconfigured credentials", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		cfg := &config.Config{NoSummarize: false}
		sumEngine, err := summarizer.New(summarizer.Config{NoSummarize: false})
		if err != nil {
			t.Fatalf("summarizer.New failed: %v", err)
		}
		logSummarizerStartup(cfg, sumEngine)

		out := buf.String()
		if !strings.Contains(out, "summarizer_unconfigured") {
			t.Errorf("expected summarizer_unconfigured log, got: %s", out)
		}
		if !strings.Contains(out, "reason=no_credentials") {
			t.Errorf("expected reason=no_credentials in log, got: %s", out)
		}
		expectedCheckedEnv := "checked_env=LITELLM_BASE_URL,LITELLM_API_KEY,OPENAI_API_KEY,GEMINI_API_KEY,ANTHROPIC_API_KEY"
		if !strings.Contains(out, expectedCheckedEnv) {
			t.Errorf("expected %s in log, got: %s", expectedCheckedEnv, out)
		}
		if strings.Contains(out, "no LLM provider credentials found") || strings.Contains(out, "To enable, set") {
			t.Errorf("did not expect prose sentence in log, got: %s", out)
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


