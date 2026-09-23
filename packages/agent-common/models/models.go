// Package models provides model metadata registry and context window capacity resolution.
package models

import (
	"strings"
)

// DefaultContextWindow is the fallback context capacity when a model cannot be resolved.
const DefaultContextWindow int64 = 128000

// exactModelContextWindows maps canonical model identifiers to context window sizes.
var exactModelContextWindows = map[string]int64{
	// Claude 3 / 3.5 / 3.7 models
	"claude-3-7-sonnet":          200000,
	"claude-3-7-sonnet-20250219": 200000,
	"claude-3.7-sonnet":          200000,
	"claude-3-7-sonnet-1m":       1000000,
	"claude-3-7-sonnet:1m":       1000000,
	"claude-3.7-sonnet-1m":       1000000,
	"claude-3.7-sonnet:1m":       1000000,
	"claude-3-5-sonnet":          200000,
	"claude-3-5-sonnet-20241022": 200000,
	"claude-3-5-sonnet-20240620": 200000,
	"claude-3.5-sonnet":          200000,
	"claude-3-5-haiku":           200000,
	"claude-3-5-haiku-20241022":  200000,
	"claude-3.5-haiku":           200000,
	"claude-3-opus":              200000,
	"claude-3-opus-20240229":     200000,
	"claude-3.0-opus":            200000,
	"claude-3-sonnet":            200000,
	"claude-3-sonnet-20240229":   200000,
	"claude-3-haiku":             200000,
	"claude-3-haiku-20240307":    200000,
	"claude-haiku-4-5":           200000,
	"claude-haiku-4-5-20251001":  200000,
	"claude-sonnet-4-5":          200000,
	"claude-opus-5":              200000,
	"claude-sonnet-5":            200000,
	"claude-fable-5":             200000,
	"claude-fable-5-1":           200000,

	// Google Gemini models
	"gemini-2.5-pro":         2000000,
	"gemini-2.5-pro-preview": 2000000,
	"gemini-1.5-pro":         2000000,
	"gemini-1.5-pro-latest":  2000000,
	"gemini-1.5-pro-001":     2000000,
	"gemini-1.5-pro-002":     2000000,
	"gemini-1.5-flash":       1000000,
	"gemini-1.5-flash-latest": 1000000,
	"gemini-1.5-flash-8b":    1000000,
	"gemini-2.0-flash":       1000000,
	"gemini-2.0-flash-exp":   1000000,
	"gemini-2.0-pro-exp":     1000000,
	"gemini-2.5-flash":       1000000,
	"gemini-3.7-flash":       1000000,

	// OpenAI models
	"gpt-4o":             128000,
	"gpt-4o-2024-05-13":  128000,
	"gpt-4o-2024-08-06":  128000,
	"gpt-4o-2024-11-20":  128000,
	"gpt-4o-mini":        128000,
	"gpt-4o-mini-2024-07-18": 128000,
	"gpt-4-turbo":        128000,
	"gpt-4-turbo-2024-04-09": 128000,
	"gpt-4-turbo-preview": 128000,
	"gpt-4-0125-preview": 128000,
	"gpt-4-1106-preview": 128000,
	"gpt-4":              8192,
	"gpt-4-0613":         8192,
	"gpt-4-32k":          32768,
	"gpt-3.5-turbo":      16385,
	"gpt-3.5-turbo-0125": 16385,
	"gpt-3.5-turbo-16k":  16385,
	"o1":                 200000,
	"o1-2024-12-17":      200000,
	"o1-preview":         128000,
	"o1-mini":            128000,
	"o3":                 200000,
	"o3-mini":            200000,
	"o3-mini-2025-01-31": 200000,
	"gpt-5":              200000,
	"gpt-5.4":            200000,
	"gpt-5.5":            200000,
}

// ResolveContextWindow returns the maximum context window size in tokens for the given model identifier.
// If the model cannot be resolved, DefaultContextWindow is returned.
func ResolveContextWindow(modelID string) int64 {
	trimmed := strings.TrimSpace(modelID)
	if trimmed == "" {
		return DefaultContextWindow
	}

	norm := strings.ToLower(trimmed)
	// Strip provider namespace (e.g. "anthropic/claude-3-7-sonnet" -> "claude-3-7-sonnet")
	if idx := strings.LastIndex(norm, "/"); idx != -1 {
		norm = norm[idx+1:]
	}

	// Check 1M variants first
	if strings.Contains(norm, "-1m") || strings.Contains(norm, ":1m") {
		return 1000000
	}

	// Exact match
	if size, ok := exactModelContextWindows[norm]; ok {
		return size
	}

	// Pattern-based heuristics in descending order of specificity
	if strings.Contains(norm, "gemini-2.5-pro") || strings.Contains(norm, "gemini-1.5-pro") {
		return 2000000
	}
	if strings.HasPrefix(norm, "gemini") {
		return 1000000
	}
	if strings.HasPrefix(norm, "claude") {
		return 200000
	}
	if strings.HasPrefix(norm, "gpt-4o") || strings.HasPrefix(norm, "gpt-4-turbo") {
		return 128000
	}
	if strings.HasPrefix(norm, "o1") || strings.HasPrefix(norm, "o3") {
		return 200000
	}
	if strings.HasPrefix(norm, "gpt-4") {
		return 8192
	}
	if strings.HasPrefix(norm, "gpt-3.5") {
		return 16385
	}

	// Prefix match against exact catalog
	for prefix, size := range exactModelContextWindows {
		if strings.HasPrefix(norm, prefix) {
			return size
		}
	}

	return DefaultContextWindow
}

// CalculateContextFullness computes context window utilization as usedTokens / contextWindow.
// Returns 0.0 if contextWindow is <= 0.
func CalculateContextFullness(usedTokens int64, contextWindow int64) float64 {
	if contextWindow <= 0 {
		return 0.0
	}
	return float64(usedTokens) / float64(contextWindow)
}
