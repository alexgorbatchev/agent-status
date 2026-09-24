package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
		"LITELLM_KEY",
		"OPENAI_API_KEY",
		"GEMINI_API_KEY",
		"ANTHROPIC_API_KEY",
		"AIR_SUMMARIZER_MODEL",
		"AIR_LOG_DIR",
		"AIR_LOG_FILE",
		"AIR_TURN_END_GRACE",
		"AIR_TURN_END_GRACE_MS",
		"AIR_TURN_END_GRACE_WINDOW",
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
	if cfg.NoSummarizeByEnv {
		t.Errorf("expected NoSummarizeByEnv false, got true")
	}
	if cfg.NoSummarizeByFlag {
		t.Errorf("expected NoSummarizeByFlag false, got true")
	}
	if cfg.NoSummarizeSource != "" {
		t.Errorf("expected NoSummarizeSource empty, got %q", cfg.NoSummarizeSource)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("got LogLevel %q, want info", cfg.LogLevel)
	}
	if cfg.TurnEndGraceWindow != 2500*time.Millisecond {
		t.Errorf("got TurnEndGraceWindow %v, want 2500ms", cfg.TurnEndGraceWindow)
	}
	if cfg.TurnEndGrace != 2500*time.Millisecond {
		t.Errorf("got TurnEndGrace %v, want 2500ms", cfg.TurnEndGrace)
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
	if !cfg.NoSummarizeByEnv {
		t.Errorf("expected NoSummarizeByEnv true, got false")
	}
	if cfg.NoSummarizeByFlag {
		t.Errorf("expected NoSummarizeByFlag false, got true")
	}
	if cfg.NoSummarizeSource != "env" {
		t.Errorf("expected NoSummarizeSource 'env', got %q", cfg.NoSummarizeSource)
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
	t.Setenv("LITELLM_KEY", "")
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

func TestLoad_LiteLLMKeyAlias(t *testing.T) {
	t.Setenv("AIR_INGESTER_ADDR", "")
	t.Setenv("AIR_BEARER_TOKEN", "")
	t.Setenv("LITELLM_BASE_URL", "https://litellm.custom/v1")
	t.Setenv("LITELLM_API_KEY", "")
	t.Setenv("LITELLM_KEY", "litellm-alias-key-999")
	t.Setenv("OPENAI_API_KEY", "")

	cfg := Load()
	if cfg.LiteLLMAPIKey != "litellm-alias-key-999" {
		t.Errorf("got LiteLLMAPIKey %q, want litellm-alias-key-999", cfg.LiteLLMAPIKey)
	}
}

func TestLoad_Precedence_BaseURL(t *testing.T) {
	tests := []struct {
		name          string
		liteLLMBaseURL string
		openaiBaseURL  string
		want          string
	}{
		{
			name:          "both set prefers LITELLM_BASE_URL",
			liteLLMBaseURL: "https://litellm.example.com/v1",
			openaiBaseURL:  "https://openai.example.com/v1",
			want:          "https://litellm.example.com/v1",
		},
		{
			name:          "only OPENAI_BASE_URL set",
			liteLLMBaseURL: "",
			openaiBaseURL:  "https://openai.example.com/v1",
			want:          "https://openai.example.com/v1",
		},
		{
			name:          "only LITELLM_BASE_URL set",
			liteLLMBaseURL: "https://litellm.example.com/v1",
			openaiBaseURL:  "",
			want:          "https://litellm.example.com/v1",
		},
		{
			name:          "neither set",
			liteLLMBaseURL: "",
			openaiBaseURL:  "",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LITELLM_BASE_URL", tt.liteLLMBaseURL)
			t.Setenv("OPENAI_BASE_URL", tt.openaiBaseURL)

			cfg := Load()
			if cfg.LiteLLMBaseURL != tt.want {
				t.Errorf("got LiteLLMBaseURL %q, want %q", cfg.LiteLLMBaseURL, tt.want)
			}
		})
	}
}

func TestLoad_Precedence_APIKey(t *testing.T) {
	tests := []struct {
		name         string
		litellmAPIKey string
		litellmKey   string
		openaiAPIKey string
		want         string
	}{
		{
			name:         "all three set prefers LITELLM_API_KEY",
			litellmAPIKey: "key-litellm-api",
			litellmKey:   "key-litellm-alias",
			openaiAPIKey: "key-openai-fallback",
			want:         "key-litellm-api",
		},
		{
			name:         "LITELLM_API_KEY empty prefers LITELLM_KEY over OPENAI_API_KEY",
			litellmAPIKey: "",
			litellmKey:   "key-litellm-alias",
			openaiAPIKey: "key-openai-fallback",
			want:         "key-litellm-alias",
		},
		{
			name:         "only OPENAI_API_KEY set",
			litellmAPIKey: "",
			litellmKey:   "",
			openaiAPIKey: "key-openai-fallback",
			want:         "key-openai-fallback",
		},
		{
			name:         "only LITELLM_KEY set",
			litellmAPIKey: "",
			litellmKey:   "key-litellm-alias",
			openaiAPIKey: "",
			want:         "key-litellm-alias",
		},
		{
			name:         "only LITELLM_API_KEY set",
			litellmAPIKey: "key-litellm-api",
			litellmKey:   "",
			openaiAPIKey: "",
			want:         "key-litellm-api",
		},
		{
			name:         "none set",
			litellmAPIKey: "",
			litellmKey:   "",
			openaiAPIKey: "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LITELLM_API_KEY", tt.litellmAPIKey)
			t.Setenv("LITELLM_KEY", tt.litellmKey)
			t.Setenv("OPENAI_API_KEY", tt.openaiAPIKey)

			cfg := Load()
			if cfg.LiteLLMAPIKey != tt.want {
				t.Errorf("got LiteLLMAPIKey %q, want %q", cfg.LiteLLMAPIKey, tt.want)
			}
		})
	}
}

func TestLoad_TurnEndGrace(t *testing.T) {
	tests := []struct {
		name      string
		envGrace  string
		envMS     string
		wantGrace time.Duration
	}{
		{
			name:      "duration string 1500ms",
			envGrace:  "1500ms",
			wantGrace: 1500 * time.Millisecond,
		},
		{
			name:      "duration string 3s",
			envGrace:  "3s",
			wantGrace: 3 * time.Second,
		},
		{
			name:      "duration string 0s disabled",
			envGrace:  "0s",
			wantGrace: 0,
		},
		{
			name:      "integer string milliseconds 3000",
			envGrace:  "3000",
			wantGrace: 3000 * time.Millisecond,
		},
		{
			name:      "integer string milliseconds 0 disabled",
			envGrace:  "0",
			wantGrace: 0,
		},
		{
			name:      "fallback to AIR_TURN_END_GRACE_MS",
			envGrace:  "",
			envMS:     "1200",
			wantGrace: 1200 * time.Millisecond,
		},
		{
			name:      "invalid duration falls back to default 2500ms",
			envGrace:  "invalid-duration",
			wantGrace: 2500 * time.Millisecond,
		},
		{
			name:      "unspecified falls back to default 2500ms",
			envGrace:  "",
			wantGrace: 2500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AIR_TURN_END_GRACE", tt.envGrace)
			t.Setenv("AIR_TURN_END_GRACE_MS", tt.envMS)
			t.Setenv("AIR_TURN_END_GRACE_WINDOW", "")

			cfg := Load()
			if cfg.TurnEndGraceWindow != tt.wantGrace {
				t.Errorf("got TurnEndGraceWindow %v, want %v", cfg.TurnEndGraceWindow, tt.wantGrace)
			}
			if cfg.TurnEndGrace != tt.wantGrace {
				t.Errorf("got TurnEndGrace %v, want %v", cfg.TurnEndGrace, tt.wantGrace)
			}
		})
	}
}


