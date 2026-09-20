package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear all relevant environment variables
	envKeys := []string{
		"AIR_INGESTER_ADDR",
		"AIR_BEARER_TOKEN",
		"XDG_DATA_HOME",
		"XDG_RUNTIME_DIR",
		"AIR_NO_SUMMARIZE",
		"LITELLM_BASE_URL",
		"OPENAI_BASE_URL",
		"LITELLM_API_KEY",
		"OPENAI_API_KEY",
		"GEMINI_API_KEY",
		"ANTHROPIC_API_KEY",
		"AIR_SUMMARIZER_MODEL",
		"AIR_LOG_DIR",
		"AIR_LOG_FILE",
	}
	for _, k := range envKeys {
		t.Setenv(k, "")
	}

	cfg := Load()
	if cfg.Addr != ":4501" {
		t.Errorf("got Addr %q, want :4501", cfg.Addr)
	}
	if cfg.BearerToken != "dev-secret-token" {
		t.Errorf("got BearerToken %q, want dev-secret-token", cfg.BearerToken)
	}

	home, _ := os.UserHomeDir()
	expectedDBPath := filepath.Join(home, ".local", "share", "agent-status", "agent-status.db")
	if cfg.DBPath != expectedDBPath {
		t.Errorf("got DBPath %q, want %q", cfg.DBPath, expectedDBPath)
	}

	expectedNotifySock := filepath.Join("/tmp", "agent-status", "notify.sock")
	if cfg.NotifySock != expectedNotifySock {
		t.Errorf("got NotifySock %q, want %q", cfg.NotifySock, expectedNotifySock)
	}

	if cfg.NoSummarize {
		t.Errorf("expected NoSummarize false, got true")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("got LogLevel %q, want info", cfg.LogLevel)
	}
}

func TestLoad_EnvironmentOverrides(t *testing.T) {
	t.Setenv("AIR_INGESTER_ADDR", "0.0.0.0:9999")
	t.Setenv("AIR_BEARER_TOKEN", "custom-token-xyz")
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	t.Setenv("XDG_RUNTIME_DIR", "/custom/run")
	t.Setenv("AIR_NO_SUMMARIZE", "1")
	t.Setenv("LITELLM_BASE_URL", "https://litellm.custom/v1")
	t.Setenv("LITELLM_API_KEY", "litellm-key-123")
	t.Setenv("GEMINI_API_KEY", "gemini-key-456")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key-789")
	t.Setenv("AIR_SUMMARIZER_MODEL", "custom-model-fast")
	t.Setenv("AIR_LOG_DIR", "/custom/logs")
	t.Setenv("AIR_LOG_FILE", "/custom/logs/ingester.log")

	cfg := Load()
	if cfg.Addr != "0.0.0.0:9999" {
		t.Errorf("got Addr %q, want 0.0.0.0:9999", cfg.Addr)
	}
	if cfg.BearerToken != "custom-token-xyz" {
		t.Errorf("got BearerToken %q, want custom-token-xyz", cfg.BearerToken)
	}
	if cfg.DBPath != "/custom/data/agent-status/agent-status.db" {
		t.Errorf("got DBPath %q, want /custom/data/agent-status/agent-status.db", cfg.DBPath)
	}
	if cfg.NotifySock != "/custom/run/agent-status/notify.sock" {
		t.Errorf("got NotifySock %q, want /custom/run/agent-status/notify.sock", cfg.NotifySock)
	}
	if !cfg.NoSummarize {
		t.Errorf("expected NoSummarize true, got false")
	}
	if cfg.LiteLLMBaseURL != "https://litellm.custom/v1" {
		t.Errorf("got LiteLLMBaseURL %q, want https://litellm.custom/v1", cfg.LiteLLMBaseURL)
	}
	if cfg.LiteLLMAPIKey != "litellm-key-123" {
		t.Errorf("got LiteLLMAPIKey %q, want litellm-key-123", cfg.LiteLLMAPIKey)
	}
	if cfg.GeminiAPIKey != "gemini-key-456" {
		t.Errorf("got GeminiAPIKey %q, want gemini-key-456", cfg.GeminiAPIKey)
	}
	if cfg.AnthropicAPIKey != "anthropic-key-789" {
		t.Errorf("got AnthropicAPIKey %q, want anthropic-key-789", cfg.AnthropicAPIKey)
	}
	if cfg.SummarizerModel != "custom-model-fast" {
		t.Errorf("got SummarizerModel %q, want custom-model-fast", cfg.SummarizerModel)
	}
	if cfg.LogDir != "/custom/logs" {
		t.Errorf("got LogDir %q, want /custom/logs", cfg.LogDir)
	}
	if cfg.LogFile != "/custom/logs/ingester.log" {
		t.Errorf("got LogFile %q, want /custom/logs/ingester.log", cfg.LogFile)
	}
}

func TestLoad_OpenAIFallbacks(t *testing.T) {
	t.Setenv("AIR_INGESTER_ADDR", "")
	t.Setenv("AIR_BEARER_TOKEN", "")
	t.Setenv("LITELLM_BASE_URL", "")
	t.Setenv("LITELLM_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_API_KEY", "sk-openai-test-key")

	cfg := Load()
	if cfg.LiteLLMBaseURL != "https://api.openai.com/v1" {
		t.Errorf("got LiteLLMBaseURL %q, want https://api.openai.com/v1", cfg.LiteLLMBaseURL)
	}
	if cfg.LiteLLMAPIKey != "sk-openai-test-key" {
		t.Errorf("got LiteLLMAPIKey %q, want sk-openai-test-key", cfg.LiteLLMAPIKey)
	}
}
