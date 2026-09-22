package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Addr            string
	BearerToken     string
	DBPath          string
	NotifySock      string
	NoSummarize     bool
	LiteLLMBaseURL  string
	LiteLLMAPIKey   string
	GeminiAPIKey    string
	AnthropicAPIKey string
	SummarizerModel string
	LogLevel        string
	LogDir          string
	LogFile         string
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

	return &Config{
		Addr:            addr,
		BearerToken:     token,
		DBPath:          dbPath,
		NotifySock:      notifySock,
		NoSummarize:     os.Getenv("AIR_NO_SUMMARIZE") == "1",
		LiteLLMBaseURL:  liteLLMURL,
		LiteLLMAPIKey:   liteLLMKey,
		GeminiAPIKey:    os.Getenv("GEMINI_API_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		SummarizerModel: os.Getenv("AIR_SUMMARIZER_MODEL"),
		LogLevel:        "info",
		LogDir:          os.Getenv("AIR_LOG_DIR"),
		LogFile:         os.Getenv("AIR_LOG_FILE"),
	}
}
