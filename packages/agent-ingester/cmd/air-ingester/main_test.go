package main

import (
	"bytes"
	"strings"
	"testing"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/config"
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

