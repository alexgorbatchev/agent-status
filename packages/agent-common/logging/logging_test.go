package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLevel slog.Level
		wantErr   bool
	}{
		{"debug lowercase", "debug", slog.LevelDebug, false},
		{"debug uppercase", "DEBUG", slog.LevelDebug, false},
		{"debug mixed", "DeBuG", slog.LevelDebug, false},
		{"info lowercase", "info", slog.LevelInfo, false},
		{"info uppercase", "INFO", slog.LevelInfo, false},
		{"empty defaults to info", "", slog.LevelInfo, false},
		{"warn lowercase", "warn", slog.LevelWarn, false},
		{"warn uppercase", "WARN", slog.LevelWarn, false},
		{"warning lowercase", "warning", slog.LevelWarn, false},
		{"error lowercase", "error", slog.LevelError, false},
		{"error uppercase", "ERROR", slog.LevelError, false},
		{"invalid level", "unknown", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.wantLevel {
				t.Fatalf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.wantLevel)
			}
		})
	}
}

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, "info")

	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Fatalf("expected no debug output at info level, got: %s", buf.String())
	}

	logger.Info("info message", "key", "val")
	out := buf.String()
	if !strings.Contains(out, "level=INFO") || !strings.Contains(out, "msg=\"info message\"") || !strings.Contains(out, "key=val") {
		t.Fatalf("unexpected log output: %s", out)
	}
}

func TestSetup(t *testing.T) {
	logger := Setup("debug")
	if logger == nil {
		t.Fatal("Setup returned nil logger")
	}
	if slog.Default() != logger {
		t.Fatal("Setup did not set default logger")
	}
}
