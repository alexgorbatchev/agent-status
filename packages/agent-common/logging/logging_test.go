package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
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

func TestIsAgentMode(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() {
		_ = os.Setenv("AGENT", orig)
	}()

	tests := []struct {
		envVal string
		want   bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"YES", true},
		{" TRUE ", true},
	}

	for _, tt := range tests {
		t.Run(tt.envVal, func(t *testing.T) {
			_ = os.Setenv("AGENT", tt.envVal)
			if got := IsAgentMode(); got != tt.want {
				t.Errorf("IsAgentMode() for AGENT=%q = %v, want %v", tt.envVal, got, tt.want)
			}
		})
	}
}

func TestHumanModeFormatting(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() {
		_ = os.Setenv("AGENT", orig)
	}()
	_ = os.Setenv("AGENT", "0")

	var buf bytes.Buffer
	logger := NewWithMode(&buf, "ingester", "info", false)

	logger.Info("hello_received",
		"env", "work-macbook",
		"envId", "885d9dd12b6d62a9",
		"dropped", int64(0),
		"since", int64(10),
	)

	out := buf.String()
	// Should contain human tags and action
	if !strings.Contains(out, "[INFO]") {
		t.Errorf("expected [INFO] in human log output, got: %s", out)
	}
	if strings.Contains(out, "service=ingester") {
		t.Errorf("expected service=ingester to be hidden in human mode, got: %s", out)
	}
	if !strings.Contains(out, "hello_received") {
		t.Errorf("expected action in human log output, got: %s", out)
	}
	// For humans: prefer name over ID, omit redundant envId when env is present
	if !strings.Contains(out, "env=work-macbook") {
		t.Errorf("expected env=work-macbook in human log output, got: %s", out)
	}
	if strings.Contains(out, "885d9dd12b6d62a9") {
		t.Errorf("expected envId hex hash to be omitted in human mode when env name is present, got: %s", out)
	}
	if !strings.Contains(out, "dropped=0") || !strings.Contains(out, "since=10") {
		t.Errorf("expected dropped and since attributes in output, got: %s", out)
	}
}

func TestHumanModeFallbackToID(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithMode(&buf, "ingester", "info", false)

	logger.Info("fallback_event", "envId", "885d9dd12b6d62a9", "count", 5)
	out := buf.String()
	if !strings.Contains(out, "envId=885d9dd12b6d62a9") {
		t.Errorf("expected envId when env name is absent, got: %s", out)
	}
	if !strings.Contains(out, "count=5") {
		t.Errorf("expected count=5, got: %s", out)
	}
}

func TestHumanModeProjectIDSuppression(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithMode(&buf, "classifier", "info", false)

	logger.Info("session_discovered",
		"project", "agent-status",
		"projectId", "proj-999",
		"harness", "claude-code",
	)
	out := buf.String()
	if !strings.Contains(out, "project=agent-status") {
		t.Errorf("expected project=agent-status, got: %s", out)
	}
	if strings.Contains(out, "proj-999") {
		t.Errorf("expected projectId to be omitted when project name is present, got: %s", out)
	}
}

func TestHumanModeFormattingEdgeCases(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithMode(&buf, "", "debug", false)

	logger.Debug("quoted_text",
		"note", "has spaces and \"quotes\"",
		"err", errors.New("boom"),
		"u", uint64(42),
		"f", 3.14,
		"dur", 500*time.Millisecond,
	)
	logger.Warn("warning_alert", "flag", true)
	logger.Error("error_occurred", "code", 500)

	// Log custom level
	customRecord := slog.NewRecord(time.Time{}, slog.Level(12), "custom_event", 0)
	_ = logger.Handler().Handle(context.Background(), customRecord)

	out := buf.String()
	if !strings.Contains(out, "[DEBUG]") || !strings.Contains(out, "[WARN]") || !strings.Contains(out, "[ERROR]") {
		t.Errorf("expected level tags in output, got:\n%s", out)
	}
	if !strings.Contains(out, "note=\"has spaces and \\\"quotes\\\"\"") {
		t.Errorf("expected escaped quotes in output, got:\n%s", out)
	}
	if !strings.Contains(out, "err=boom") {
		t.Errorf("expected err=boom in output, got:\n%s", out)
	}
	if !strings.Contains(out, "flag=true") {
		t.Errorf("expected flag=true in output, got:\n%s", out)
	}
	if !strings.Contains(out, "u=42") || !strings.Contains(out, "f=3.14") || !strings.Contains(out, "dur=500ms") {
		t.Errorf("expected formatted primitives in output, got:\n%s", out)
	}
	if !strings.Contains(out, "00:00:00") {
		t.Errorf("expected zero-time fallback in output, got:\n%s", out)
	}
}

func TestHumanModeWithGroupAndAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithMode(&buf, "group_service", "info", false)

	// Test empty group name
	handlerNoopGroup := logger.Handler().WithGroup("")
	if handlerNoopGroup == nil {
		t.Fatal("expected non-nil handler")
	}

	// Test real group and attrs
	groupedLogger := logger.WithGroup("meta").With("region", "us-east")
	groupedLogger.Info("grouped_event", "host", "srv-1")

	out := buf.String()
	if !strings.Contains(out, "grouped_event") {
		t.Errorf("expected grouped_event in output, got:\n%s", out)
	}
	if !strings.Contains(out, "meta.region=us-east") || !strings.Contains(out, "meta.host=srv-1") {
		t.Errorf("expected group-prefixed attributes in output, got:\n%s", out)
	}
}

func TestAgentModeFormatting(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithMode(&buf, "ingester", "info", true)

	logger.Info("hello_received",
		"env", "work-macbook",
		"envId", "885d9dd12b6d62a9",
		"dropped", int64(0),
		"since", int64(10),
	)

	out := buf.String()
	// Agent mode should output standard structured text (logfmt) with action= instead of msg=
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("expected level=INFO in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "action=hello_received") {
		t.Errorf("expected action=hello_received in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "service=ingester") {
		t.Errorf("expected service=ingester in agent mode, got: %s", out)
	}
	if !strings.Contains(out, "env=work-macbook") || !strings.Contains(out, "envId=885d9dd12b6d62a9") {
		t.Errorf("expected both env and envId to be preserved in agent mode, got: %s", out)
	}
}

func TestNewAndSetupFunctions(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer func() {
		_ = os.Setenv("AGENT", orig)
	}()

	_ = os.Setenv("AGENT", "1")
	var buf bytes.Buffer
	agentLogger := New(&buf, "test-svc", "info")
	agentLogger.Info("agent_test")
	if !strings.Contains(buf.String(), "action=agent_test") {
		t.Errorf("expected action=agent_test from New in agent mode, got: %s", buf.String())
	}

	_ = os.Setenv("AGENT", "0")
	buf.Reset()
	humanLogger := New(&buf, "test-svc", "info")
	humanLogger.Info("human_test")
	if !strings.Contains(buf.String(), "human_test") {
		t.Errorf("expected human_test from New in human mode, got: %s", buf.String())
	}

	setupLogger := Setup("test-setup", "info")
	if setupLogger == nil {
		t.Fatal("expected Setup to return non-nil logger")
	}

	withLogger := With("extra", "val")
	if withLogger == nil {
		t.Fatal("expected With to return non-nil logger")
	}
}

func TestSetupAndLoggingHelpers(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithMode(&buf, "helper_service", "debug", false)
	slog.SetDefault(logger)

	Debug("debug_event", "k1", "v1")
	Info("info_event", "k2", "v2")
	Warn("warn_event", "k3", "v3")
	Error("error_event", "k4", "v4")

	ctx := context.Background()
	DebugContext(ctx, "debug_ctx", "k5", "v5")
	InfoContext(ctx, "info_ctx", "k6", "v6")
	WarnContext(ctx, "warn_ctx", "k7", "v7")
	ErrorContext(ctx, "error_ctx", "k8", "v8")

	out := buf.String()
	for _, expected := range []string{
		"debug_event", "k1=v1",
		"info_event", "k2=v2",
		"warn_event", "k3=v3",
		"error_event", "k4=v4",
		"debug_ctx", "k5=v5",
		"info_ctx", "k6=v6",
		"warn_ctx", "k7=v7",
		"error_ctx", "k8=v8",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, out)
		}
	}
}
