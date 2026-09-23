package models_test

import (
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/models"
)

func TestResolveContextWindow(t *testing.T) {
	tests := []struct {
		name       string
		modelID    string
		wantWindow int64
	}{
		{"claude 3.5 sonnet", "claude-3-5-sonnet-20241022", 200000},
		{"claude 3.7 sonnet", "claude-3-7-sonnet-20250219", 200000},
		{"claude 3.7 sonnet base", "claude-3-7-sonnet", 200000},
		{"claude 3.7 sonnet 1m variant", "claude-3-7-sonnet-1m", 1000000},
		{"claude 3.7 sonnet 1m with colon", "claude-3-7-sonnet:1m", 1000000},
		{"claude 3.7 sonnet 1m dot notation", "claude-3.7-sonnet-1m", 1000000},
		{"claude 3 opus", "claude-3-opus-20240229", 200000},
		{"claude 3.5 haiku", "claude-3-5-haiku-20241022", 200000},
		{"gemini 2.5 pro", "gemini-2.5-pro", 2000000},
		{"gemini 1.5 pro", "gemini-1.5-pro", 2000000},
		{"gemini 1.5 flash", "gemini-1.5-flash", 1000000},
		{"gemini 2.5 flash", "gemini-2.5-flash", 1000000},
		{"gemini 3.7 flash", "gemini-3.7-flash", 1000000},
		{"gpt-4o", "gpt-4o", 128000},
		{"gpt-4o with date", "gpt-4o-2024-08-06", 128000},
		{"gpt-4o-mini", "gpt-4o-mini", 128000},
		{"o1", "o1", 200000},
		{"o3-mini", "o3-mini", 200000},
		{"provider prefix anthropic", "anthropic/claude-3-7-sonnet", 200000},
		{"provider prefix openai", "openai/gpt-4o", 128000},
		{"provider prefix google", "google/gemini-2.5-pro", 2000000},
		{"generic gemini fallback", "gemini-ultra", 1000000},
		{"generic claude fallback", "claude-next-gen", 200000},
		{"generic gpt-4 fallback", "gpt-4-custom", 8192},
		{"generic gpt-3.5 fallback", "gpt-3.5-custom", 16385},
		{"generic gpt-4o fallback", "gpt-4o-custom", 128000},
		{"generic o1 fallback", "o1-custom", 200000},
		{"unknown model fallback", "unknown-custom-model", 128000},
		{"empty model fallback", "", 128000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := models.ResolveContextWindow(tt.modelID)
			if got != tt.wantWindow {
				t.Errorf("ResolveContextWindow(%q) = %d, want %d", tt.modelID, got, tt.wantWindow)
			}
		})
	}
}

func TestCalculateContextFullness(t *testing.T) {
	tests := []struct {
		name          string
		usedTokens    int64
		contextWindow int64
		wantFullness  float64
	}{
		{"quarter full", 50000, 200000, 0.25},
		{"half full", 100000, 200000, 0.50},
		{"completely full", 200000, 200000, 1.0},
		{"zero context window", 50000, 0, 0.0},
		{"zero tokens", 0, 200000, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := models.CalculateContextFullness(tt.usedTokens, tt.contextWindow)
			if got != tt.wantFullness {
				t.Errorf("CalculateContextFullness(%d, %d) = %f, want %f", tt.usedTokens, tt.contextWindow, got, tt.wantFullness)
			}
		})
	}
}
