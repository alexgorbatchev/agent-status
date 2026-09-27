package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
)

type Config struct {
	Addr              string
	BearerToken       string
	DBPath            string
	NotifySock        string
	NoSummarize       bool
	NoSummarizeByFlag bool
	NoSummarizeByEnv  bool
	NoSummarizeSource string
	LiteLLMBaseURL    string
	LiteLLMAPIKey     string
	GeminiAPIKey      string
	AnthropicAPIKey   string
	SummarizerModel   string
	LogLevel          string
	LogDir            string
	LogFile           string
	IdleGracePeriod   time.Duration
}

func parseIdleGracePeriod() time.Duration {
	v := os.Getenv("AIR_IDLE_GRACE_PERIOD")
	if v == "" {
		return classifier.DefaultIdleGracePeriod
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		logging.Error("invalid_idle_grace_period", "raw", v, "err", err, "default", classifier.DefaultIdleGracePeriod)
		return classifier.DefaultIdleGracePeriod
	}
	if d < 0 {
		logging.Error("invalid_idle_grace_period", "value", v, "err", "duration cannot be negative")
		return classifier.DefaultIdleGracePeriod
	}
	return d
}

// Validate checks the configuration for invalid values.
func (c *Config) Validate() error {
	if c.IdleGracePeriod < 0 {
		return fmt.Errorf("idle grace period must be non-negative: %v", c.IdleGracePeriod)
	}
	return nil
}

func Load() *Config {
	addr := os.Getenv("AIR_INGESTER_ADDR")
	if addr == "" {
		addr = ":4501"
	}

	token := os.Getenv("AIR_BEARER_TOKEN")
	if token == "" {
		token = "dev-secret-token"
	}

	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	dbPath := filepath.Join(dataDir, "agent-status", "agent-status.db")

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/tmp"
	}
	notifySock := filepath.Join(runtimeDir, "agent-status", "notify.sock")

	liteLLMURL := os.Getenv("LITELLM_BASE_URL")
	if liteLLMURL == "" {
		liteLLMURL = os.Getenv("OPENAI_BASE_URL")
	}

	liteLLMKey := os.Getenv("LITELLM_API_KEY")
	if liteLLMKey == "" {
		liteLLMKey = os.Getenv("LITELLM_KEY")
	}
	if liteLLMKey == "" {
		liteLLMKey = os.Getenv("OPENAI_API_KEY")
	}

	noSummarizeEnv := os.Getenv("AIR_NO_SUMMARIZE") == "1"
	var noSummarizeSource string
	if noSummarizeEnv {
		noSummarizeSource = "env"
	}

	idleGrace := parseIdleGracePeriod()

	return &Config{
		Addr:              addr,
		BearerToken:       token,
		DBPath:            dbPath,
		NotifySock:        notifySock,
		NoSummarize:       noSummarizeEnv,
		NoSummarizeByFlag: false,
		NoSummarizeByEnv:  noSummarizeEnv,
		NoSummarizeSource: noSummarizeSource,
		LiteLLMBaseURL:    liteLLMURL,
		LiteLLMAPIKey:     liteLLMKey,
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		AnthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
		SummarizerModel:   os.Getenv("AIR_SUMMARIZER_MODEL"),
		LogLevel:          "info",
		LogDir:            os.Getenv("AIR_LOG_DIR"),
		LogFile:           os.Getenv("AIR_LOG_FILE"),
		IdleGracePeriod:   idleGrace,
	}
}
