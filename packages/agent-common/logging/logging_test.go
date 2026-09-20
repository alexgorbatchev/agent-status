package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    slog.Level
		wantErr bool
	}{
		{"", slog.LevelInfo, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"invalid", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, "test-service", "info")

	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Fatalf("expected no debug output at info level, got: %s", buf.String())
	}

	logger.Info("info message", "key", "val")
	out := buf.String()
	if !strings.Contains(out, "level=INFO") || !strings.Contains(out, "msg=\"info message\"") || !strings.Contains(out, "service=test-service") || !strings.Contains(out, "key=val") {
		t.Fatalf("unexpected log output: %s", out)
	}
}

func TestSetupAndLoggingHelpers(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, "helper-service", "debug")
	slog.SetDefault(logger)

	Debug("debug event", "k1", "v1")
	Info("info event", "k2", "v2")
	Warn("warn event", "k3", "v3")
	Error("error event", "k4", "v4")

	ctx := context.Background()
	DebugContext(ctx, "debug ctx", "k5", "v5")
	InfoContext(ctx, "info ctx", "k6", "v6")
	WarnContext(ctx, "warn ctx", "k7", "v7")
	ErrorContext(ctx, "error ctx", "k8", "v8")

	out := buf.String()
	for _, expected := range []string{
		"msg=\"debug event\"", "k1=v1",
		"msg=\"info event\"", "k2=v2",
		"msg=\"warn event\"", "k3=v3",
		"msg=\"error event\"", "k4=v4",
		"msg=\"debug ctx\"", "k5=v5",
		"msg=\"info ctx\"", "k6=v6",
		"msg=\"warn ctx\"", "k7=v7",
		"msg=\"error ctx\"", "k8=v8",
		"service=helper-service",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, out)
		}
	}
}
