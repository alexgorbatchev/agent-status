package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "air-ingester-test-logs-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test log dir: %v\n", err)
		os.Exit(1)
	}

	if err := os.Setenv("AIR_LOG_DIR", tmpDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set AIR_LOG_DIR: %v\n", err)
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

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

	idleFlag := cmd.Flags().Lookup("idle-grace-period")
	if idleFlag == nil {
		t.Fatal("missing --idle-grace-period flag")
	}
	if idleFlag.DefValue != "5s" {
		t.Errorf("expected --idle-grace-period default value to be '5s', got %q", idleFlag.DefValue)
	}
}

func TestIngesterIdleGracePeriodFlag(t *testing.T) {
	t.Run("parse valid duration flag", func(t *testing.T) {
		cfg := &config.Config{
			IdleGracePeriod: 5 * time.Second,
		}
		cmd := newRootCmd(cfg)
		if err := cmd.ParseFlags([]string{"--idle-grace-period", "2500ms"}); err != nil {
			t.Fatalf("ParseFlags failed: %v", err)
		}
		if cfg.IdleGracePeriod != 2500*time.Millisecond {
			t.Errorf("got IdleGracePeriod %v, want 2500ms", cfg.IdleGracePeriod)
		}
	})

	t.Run("parse zero duration flag disables debounce", func(t *testing.T) {
		cfg := &config.Config{
			IdleGracePeriod: 5 * time.Second,
		}
		cmd := newRootCmd(cfg)
		if err := cmd.ParseFlags([]string{"--idle-grace-period=0s"}); err != nil {
			t.Fatalf("ParseFlags failed: %v", err)
		}
		if cfg.IdleGracePeriod != 0 {
			t.Errorf("got IdleGracePeriod %v, want 0", cfg.IdleGracePeriod)
		}
	})

	t.Run("rejects negative duration via command validation", func(t *testing.T) {
		cfg := &config.Config{
			IdleGracePeriod: 5 * time.Second,
		}
		cmd := newRootCmd(cfg)
		cmd.SetArgs([]string{"--idle-grace-period", "-3s"})
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error executing command with negative idle-grace-period, got nil")
		}
		if !strings.Contains(err.Error(), "idle grace period must be non-negative") {
			t.Errorf("expected error message to mention non-negative, got %v", err)
		}
	})

	t.Run("falls back to default duration via invalid negative environment variable", func(t *testing.T) {
		t.Setenv("AIR_IDLE_GRACE_PERIOD", "-4s")
		cfg := config.Load()
		if cfg.IdleGracePeriod != 5*time.Second {
			t.Fatalf("expected IdleGracePeriod to fall back to default 5s, got %v", cfg.IdleGracePeriod)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected Validate to succeed, got %v", err)
		}
	})
}

func TestRunServer_ConfigValidationRejectsNegativeDuration(t *testing.T) {
	cfg := &config.Config{
		IdleGracePeriod: -2 * time.Second,
	}
	cmd := newRootCmd(cfg)
	ctx := context.Background()
	err := runServer(ctx, cmd, cfg)
	if err == nil {
		t.Fatal("expected runServer to reject negative duration, got nil")
	}
	if !strings.Contains(err.Error(), "idle grace period must be non-negative") {
		t.Errorf("expected error to mention non-negative, got %v", err)
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

type countingPruner struct {
	mu            sync.Mutex
	calls         int
	lastRetention int64
	lastExited    int64
	err           error
}

func (p *countingPruner) PruneOldEventsAndExitedAgents(retentionCutoffMs int64, exitedCutoffMs int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.lastRetention = retentionCutoffMs
	p.lastExited = exitedCutoffMs
	return p.err
}

func TestPeriodicPruner(t *testing.T) {
	origLogger := logging.Default()
	t.Cleanup(func() {
		logging.SetDefault(origLogger)
	})

	t.Run("runPrunerSweep executes prune with 48h and 30d cutoffs", func(t *testing.T) {
		p := &countingPruner{}
		ctx := context.Background()
		before := time.Now().UnixMilli()
		if err := runPrunerSweep(ctx, p); err != nil {
			t.Fatalf("runPrunerSweep failed: %v", err)
		}
		after := time.Now().UnixMilli()

		p.mu.Lock()
		defer p.mu.Unlock()
		if p.calls != 1 {
			t.Fatalf("expected 1 prune call, got %d", p.calls)
		}
		expectedRetentionMin := before - (48 * 3600 * 1000)
		expectedRetentionMax := after - (48 * 3600 * 1000)
		if p.lastRetention < expectedRetentionMin || p.lastRetention > expectedRetentionMax {
			t.Errorf("lastRetention %d not in [%d, %d]", p.lastRetention, expectedRetentionMin, expectedRetentionMax)
		}
		expectedExitedMin := before - (30 * 24 * 3600 * 1000)
		expectedExitedMax := after - (30 * 24 * 3600 * 1000)
		if p.lastExited < expectedExitedMin || p.lastExited > expectedExitedMax {
			t.Errorf("lastExited %d not in [%d, %d]", p.lastExited, expectedExitedMin, expectedExitedMax)
		}
	})

	t.Run("runPrunerSweep logs warning on failure", func(t *testing.T) {
		buf := new(bytes.Buffer)
		logging.SetDefault(logging.NewWithMode(buf, "test-ingester", "info", true))

		p := &countingPruner{err: errors.New("disk is full")}
		ctx := context.Background()
		err := runPrunerSweep(ctx, p)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		out := buf.String()
		if !strings.Contains(out, "prune_cleanup_failed") {
			t.Errorf("expected prune_cleanup_failed in log, got: %s", out)
		}
		if !strings.Contains(out, "disk is full") {
			t.Errorf("expected error detail in log, got: %s", out)
		}
	})

	t.Run("runPrunerSweep respects canceled context", func(t *testing.T) {
		p := &countingPruner{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := runPrunerSweep(ctx, p)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.calls != 0 {
			t.Errorf("prune should not have been called on canceled context")
		}
	})

	t.Run("startPeriodicPruner executes on ticker and exits on context cancel", func(t *testing.T) {
		p := &countingPruner{}
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			startPeriodicPruner(ctx, p, 5*time.Millisecond)
			close(done)
		}()

		time.Sleep(25 * time.Millisecond)
		cancel()

		select {
		case <-done:
			// Clean exit
		case <-time.After(1 * time.Second):
			t.Fatal("startPeriodicPruner did not exit after context cancellation")
		}

		p.mu.Lock()
		defer p.mu.Unlock()
		if p.calls == 0 {
			t.Errorf("expected at least one prune call, got 0")
		}
	})
}

// shortTestSockPath returns a Unix domain socket path short enough to fit within
// OS sun_path length limits (104 bytes on Darwin, 108 on Linux), avoiding bind errors
// caused by lengthy t.TempDir() subtest paths.
func shortTestSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "air-sock-*")
	if err != nil {
		dir = t.TempDir()
	} else {
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
	}
	return filepath.Join(dir, "notify.sock")
}

func TestShortTestSockPath(t *testing.T) {
	sockPath := shortTestSockPath(t)
	if len(sockPath) >= 104 {
		t.Errorf("socket path %q length = %d, want < 104", sockPath, len(sockPath))
	}
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to bind test unix socket at %q: %v", sockPath, err)
	}
	_ = l.Close()
}

func TestRunServer(t *testing.T) {
	origLogger := logging.Default()
	t.Cleanup(func() {
		logging.SetDefault(origLogger)
	})

	t.Run("lifecycle with ephemeral port and clean shutdown", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "agent-status.db")
		sockPath := shortTestSockPath(t)

		buf := new(bytes.Buffer)
		cfg := &config.Config{
			Addr:        "127.0.0.1:0",
			DBPath:      dbPath,
			NotifySock:  sockPath,
			NoSummarize: true,
			BearerToken: "test-secret-token",
			LogLevel:    "info",
			LogDir:      t.TempDir(),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := newRootCmd(cfg)
		cmd.SetErr(buf)

		ready := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			errCh <- runServer(ctx, cmd, cfg, ready)
		}()

		var boundAddr string
		select {
		case boundAddr = <-ready:
		case err := <-errCh:
			cancel()
			t.Fatalf("runServer failed before ready: %v", err)
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("timed out waiting for server to bind ephemeral port")
		}

		// Verify /healthz responds
		healthURL := fmt.Sprintf("http://%s/healthz", boundAddr)
		resp, err := http.Get(healthURL)
		if err != nil {
			cancel()
			t.Fatalf("GET /healthz failed: %v", err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "ok\n" {
			t.Errorf("expected 'ok\\n', got %q", string(body))
		}

		// Also verify internal endpoints routed through rcv
		visURL := fmt.Sprintf("http://%s/api/v1/internal/agents/agent-123/visibility", boundAddr)
		req, _ := http.NewRequest(http.MethodPatch, visURL, strings.NewReader(`{"isHidden":true}`))
		req.Header.Set("Authorization", "Bearer test-secret-token")
		visResp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("PATCH visibility failed: %v", err)
		} else {
			_ = visResp.Body.Close()
		}

		focusURL := fmt.Sprintf("http://%s/api/v1/internal/agents/agent-123/focus", boundAddr)
		focusReq, _ := http.NewRequest(http.MethodPost, focusURL, nil)
		focusReq.Header.Set("Authorization", "Bearer test-secret-token")
		focusResp, err := http.DefaultClient.Do(focusReq)
		if err != nil {
			t.Logf("POST focus failed: %v", err)
		} else {
			_ = focusResp.Body.Close()
		}

		// Cancel context to initiate graceful shutdown
		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runServer returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runServer did not shut down within timeout")
		}

		out := buf.String()
		if !strings.Contains(out, "server_starting") {
			t.Errorf("expected server_starting in log, got: %s", out)
		}
		if !strings.Contains(out, "server_shutting_down") {
			t.Errorf("expected server_shutting_down in log, got: %s", out)
		}
	})

	t.Run("store open error", func(t *testing.T) {
		cfg := &config.Config{
			Addr:        "127.0.0.1:0",
			DBPath:      "/invalid-nonexistent-dir/sub/path/agent.db",
			NotifySock:  shortTestSockPath(t),
			NoSummarize: true,
			LogLevel:    "info",
			LogDir:      t.TempDir(),
		}
		ctx := context.Background()
		cmd := newRootCmd(cfg)
		err := runServer(ctx, cmd, cfg)
		if err == nil {
			t.Fatal("expected error opening invalid store path, got nil")
		}
		if !strings.Contains(err.Error(), "opening store") {
			t.Errorf("expected 'opening store' error context, got: %v", err)
		}
	})

	t.Run("notify publisher error", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "agent-status.db")
		cfg := &config.Config{
			Addr:        "127.0.0.1:0",
			DBPath:      dbPath,
			NotifySock:  "/invalid-nonexistent-dir/sub/path/notify.sock",
			NoSummarize: true,
			LogLevel:    "info",
			LogDir:      t.TempDir(),
		}
		ctx := context.Background()
		cmd := newRootCmd(cfg)
		err := runServer(ctx, cmd, cfg)
		if err == nil {
			t.Fatal("expected error initializing notify publisher, got nil")
		}
		if !strings.Contains(err.Error(), "initializing notify publisher") {
			t.Errorf("expected 'initializing notify publisher' error context, got: %v", err)
		}
	})

	t.Run("listen error on invalid address", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "agent-status.db")
		sockPath := shortTestSockPath(t)
		cfg := &config.Config{
			Addr:        "999.999.999.999:9999",
			DBPath:      dbPath,
			NotifySock:  sockPath,
			NoSummarize: true,
			LogLevel:    "info",
			LogDir:      t.TempDir(),
		}
		ctx := context.Background()
		cmd := newRootCmd(cfg)
		err := runServer(ctx, cmd, cfg)
		if err == nil {
			t.Fatal("expected error listening on invalid address, got nil")
		}
		if !strings.Contains(err.Error(), "listening on") {
			t.Errorf("expected 'listening on' error context, got: %v", err)
		}
	})

	t.Run("log file and log dir config", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "agent-status.db")
		sockPath := shortTestSockPath(t)
		logDir := filepath.Join(tmpDir, "logs")
		logFile := filepath.Join(logDir, "test-ingester.log")

		cfg := &config.Config{
			Addr:        "127.0.0.1:0",
			DBPath:      dbPath,
			NotifySock:  sockPath,
			NoSummarize: true,
			LogLevel:    "info",
			LogDir:      logDir,
			LogFile:     logFile,
		}
		ctx, cancel := context.WithCancel(context.Background())
		cmd := newRootCmd(cfg)
		errCh := make(chan error, 1)
		go func() {
			errCh <- runServer(ctx, cmd, cfg)
		}()

		// Wait briefly then cancel
		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runServer with file logging failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runServer did not shut down in time")
		}
	})
}

func TestNewRootCmd_Execution(t *testing.T) {
	origLogger := logging.Default()
	t.Cleanup(func() {
		logging.SetDefault(origLogger)
	})

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "agent-status.db")
	sockPath := shortTestSockPath(t)

	cfg := &config.Config{}
	cmd := newRootCmd(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetArgs([]string{
		"--addr", "127.0.0.1:0",
		"--db", dbPath,
		"--notify-sock", sockPath,
		"--token", "test-token",
		"--no-summarize",
		"--log-level", "info",
		"--log-dir", t.TempDir(),
		"--litellm-url", "http://localhost:4000",
		"--litellm-key", "sk-test",
		"--summarizer-model", "test-model",
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.ExecuteContext(ctx)
	}()

	// Wait briefly for server startup then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("cmd.ExecuteContext failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cmd.ExecuteContext did not exit after cancel")
	}

	if cfg.BearerToken != "test-token" {
		t.Errorf("expected BearerToken 'test-token', got %q", cfg.BearerToken)
	}
	if cfg.LiteLLMBaseURL != "http://localhost:4000" {
		t.Errorf("expected LiteLLMBaseURL 'http://localhost:4000', got %q", cfg.LiteLLMBaseURL)
	}
}

func TestMain_Version(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
	})
	os.Args = []string{"air-ingester", "--version"}
	main()
}

func TestRunServer_DoesNotLeakLogsToUserXDGStateHome(t *testing.T) {
	origLogger := logging.Default()
	t.Cleanup(func() {
		logging.SetDefault(origLogger)
	})

	fakeStateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", fakeStateHome)

	t.Run("isolated via config LogDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		logDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "agent-status.db")
		sockPath := shortTestSockPath(t)

		cfg := &config.Config{
			Addr:        "127.0.0.1:0",
			DBPath:      dbPath,
			NotifySock:  sockPath,
			NoSummarize: true,
			BearerToken: "test-secret-token",
			LogLevel:    "info",
			LogDir:      logDir,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := newRootCmd(cfg)
		ready := make(chan string, 1)
		errCh := make(chan error, 1)

		go func() {
			errCh <- runServer(ctx, cmd, cfg, ready)
		}()

		select {
		case <-ready:
		case err := <-errCh:
			cancel()
			t.Fatalf("runServer failed before ready: %v", err)
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("timed out waiting for server to bind ephemeral port")
		}

		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runServer returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runServer did not shut down within timeout")
		}

		_ = logging.Close()

		entries, err := os.ReadDir(logDir)
		if err != nil || len(entries) == 0 {
			t.Fatalf("expected log file in cfg.LogDir (%s), got %v (err: %v)", logDir, entries, err)
		}

		leakDir := filepath.Join(fakeStateHome, "agent-status", "logs")
		if entries, err := os.ReadDir(leakDir); err == nil && len(entries) > 0 {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Fatalf("expected no log files to be created under XDG_STATE_HOME, but found %v in %s", names, leakDir)
		}
	})

	t.Run("isolated via package TestMain AIR_LOG_DIR fallback", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "agent-status.db")
		sockPath := shortTestSockPath(t)

		cfg := &config.Config{
			Addr:        "127.0.0.1:0",
			DBPath:      dbPath,
			NotifySock:  sockPath,
			NoSummarize: true,
			BearerToken: "test-secret-token",
			LogLevel:    "info",
			// LogDir and LogFile deliberately empty; package TestMain AIR_LOG_DIR catches it
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := newRootCmd(cfg)
		ready := make(chan string, 1)
		errCh := make(chan error, 1)

		go func() {
			errCh <- runServer(ctx, cmd, cfg, ready)
		}()

		select {
		case <-ready:
		case err := <-errCh:
			cancel()
			t.Fatalf("runServer failed before ready: %v", err)
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("timed out waiting for server to bind ephemeral port")
		}

		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runServer returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runServer did not shut down within timeout")
		}

		_ = logging.Close()

		leakDir := filepath.Join(fakeStateHome, "agent-status", "logs")
		if entries, err := os.ReadDir(leakDir); err == nil && len(entries) > 0 {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Fatalf("expected no log files to be created under XDG_STATE_HOME, but found %v in %s", names, leakDir)
		}
	})
}
