package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
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
	SetDefault(setupLogger)

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

func TestDefaultLogDir(t *testing.T) {
	t.Run("AIR_LOG_DIR precedence", func(t *testing.T) {
		t.Setenv("AIR_LOG_DIR", "/custom/air/logs")
		t.Setenv("XDG_STATE_HOME", "/custom/state")
		t.Setenv("XDG_DATA_HOME", "/custom/data")

		if got := DefaultLogDir(); got != "/custom/air/logs" {
			t.Errorf("DefaultLogDir() = %q, want %q", got, "/custom/air/logs")
		}
	})

	t.Run("XDG_STATE_HOME precedence over XDG_DATA_HOME", func(t *testing.T) {
		t.Setenv("AIR_LOG_DIR", "")
		t.Setenv("XDG_STATE_HOME", "/custom/state")
		t.Setenv("XDG_DATA_HOME", "/custom/data")

		want := "/custom/state/agent-status/logs"
		if got := DefaultLogDir(); got != want {
			t.Errorf("DefaultLogDir() = %q, want %q", got, want)
		}
	})

	t.Run("XDG_DATA_HOME fallback", func(t *testing.T) {
		t.Setenv("AIR_LOG_DIR", "")
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("XDG_DATA_HOME", "/custom/data")

		want := "/custom/data/agent-status/logs"
		if got := DefaultLogDir(); got != want {
			t.Errorf("DefaultLogDir() = %q, want %q", got, want)
		}
	})

	t.Run("User home default fallback", func(t *testing.T) {
		t.Setenv("AIR_LOG_DIR", "")
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("XDG_DATA_HOME", "")

		got := DefaultLogDir()
		if !strings.Contains(got, "agent-status/logs") {
			t.Errorf("DefaultLogDir() = %q, expected path ending in agent-status/logs", got)
		}
	})
}

func TestLoadFileConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("AIR_LOG_TO_DISK", "")
		t.Setenv("AIR_LOG_FILE", "")
		t.Setenv("AIR_LOG_DIR", "")
		t.Setenv("AIR_LOG_MAX_SIZE_MB", "")
		t.Setenv("AIR_LOG_MAX_BACKUPS", "")
		t.Setenv("AIR_LOG_MAX_AGE_DAYS", "")
		t.Setenv("AIR_LOG_COMPRESS", "")

		cfg := LoadFileConfigFromEnv("test-svc")
		if cfg.Disabled {
			t.Errorf("expected Disabled=false by default")
		}
		if cfg.MaxSizeMB != DefaultMaxSizeMB {
			t.Errorf("MaxSizeMB = %d, want %d", cfg.MaxSizeMB, DefaultMaxSizeMB)
		}
		if cfg.MaxBackups != DefaultMaxBackups {
			t.Errorf("MaxBackups = %d, want %d", cfg.MaxBackups, DefaultMaxBackups)
		}
		if cfg.MaxAgeDays != DefaultMaxAgeDays {
			t.Errorf("MaxAgeDays = %d, want %d", cfg.MaxAgeDays, DefaultMaxAgeDays)
		}
		if !cfg.Compress {
			t.Errorf("expected Compress=true by default")
		}
	})

	t.Run("custom env overrides and disabled variations", func(t *testing.T) {
		for _, val := range []string{"0", "false", "no", "off"} {
			t.Setenv("AIR_LOG_TO_DISK", val)
			cfg := LoadFileConfigFromEnv("test-svc")
			if !cfg.Disabled {
				t.Errorf("expected Disabled=true for AIR_LOG_TO_DISK=%q", val)
			}
		}

		t.Setenv("AIR_LOG_TO_DISK", "1")
		t.Setenv("AIR_LOG_FILE", "/custom/file.log")
		t.Setenv("AIR_LOG_DIR", "/custom/dir")
		t.Setenv("AIR_LOG_MAX_SIZE_MB", "50")
		t.Setenv("AIR_LOG_MAX_BACKUPS", "10")
		t.Setenv("AIR_LOG_MAX_AGE_DAYS", "30")
		t.Setenv("AIR_LOG_COMPRESS", "false")

		cfg := LoadFileConfigFromEnv("test-svc")
		if cfg.Disabled {
			t.Errorf("expected Disabled=false when AIR_LOG_TO_DISK=1")
		}
		if cfg.Filename != "/custom/file.log" {
			t.Errorf("Filename = %q, want /custom/file.log", cfg.Filename)
		}
		if cfg.Dir != "/custom/dir" {
			t.Errorf("Dir = %q, want /custom/dir", cfg.Dir)
		}
		if cfg.MaxSizeMB != 50 {
			t.Errorf("MaxSizeMB = %d, want 50", cfg.MaxSizeMB)
		}
		if cfg.MaxBackups != 10 {
			t.Errorf("MaxBackups = %d, want 10", cfg.MaxBackups)
		}
		if cfg.MaxAgeDays != 30 {
			t.Errorf("MaxAgeDays = %d, want 30", cfg.MaxAgeDays)
		}
		if cfg.Compress {
			t.Errorf("expected Compress=false when AIR_LOG_COMPRESS=false")
		}

		// Test invalid numbers keep defaults
		t.Setenv("AIR_LOG_MAX_SIZE_MB", "invalid")
		t.Setenv("AIR_LOG_MAX_BACKUPS", "-5")
		t.Setenv("AIR_LOG_MAX_AGE_DAYS", "abc")
		cfgInvalid := LoadFileConfigFromEnv("test-svc")
		if cfgInvalid.MaxSizeMB != DefaultMaxSizeMB {
			t.Errorf("MaxSizeMB = %d, want default %d", cfgInvalid.MaxSizeMB, DefaultMaxSizeMB)
		}
		if cfgInvalid.MaxBackups != DefaultMaxBackups {
			t.Errorf("MaxBackups = %d, want default %d", cfgInvalid.MaxBackups, DefaultMaxBackups)
		}
		if cfgInvalid.MaxAgeDays != DefaultMaxAgeDays {
			t.Errorf("MaxAgeDays = %d, want default %d", cfgInvalid.MaxAgeDays, DefaultMaxAgeDays)
		}
	})
}

func TestNewRotator(t *testing.T) {
	t.Run("disabled returns nil", func(t *testing.T) {
		rotator, err := NewRotator("test", FileConfig{Disabled: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rotator != nil {
			t.Fatalf("expected nil rotator when disabled, got %v", rotator)
		}
	})

	t.Run("creates log file and writes cleanly with defaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "logs", "custom.log")

		rotator, err := NewRotator("", FileConfig{
			Filename:   logFile,
			MaxSizeMB:  0, // tests default fallback
			MaxBackups: 0, // tests default fallback
			MaxAgeDays: 0, // tests default fallback
			Compress:   false,
		})
		if err != nil {
			t.Fatalf("unexpected error from NewRotator: %v", err)
		}
		if rotator == nil {
			t.Fatal("expected non-nil rotator")
		}
		defer func() { _ = rotator.Close() }()

		n, err := rotator.Write([]byte("test log entry\n"))
		if err != nil {
			t.Fatalf("writing to rotator: %v", err)
		}
		if n == 0 {
			t.Errorf("expected >0 bytes written")
		}

		_ = rotator.Close()

		data, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatalf("reading written log file: %v", err)
		}
		if string(data) != "test log entry\n" {
			t.Errorf("log file contents = %q, want 'test log entry\\n'", string(data))
		}
	})

	t.Run("derives filename from Dir and empty service", func(t *testing.T) {
		tmpDir := t.TempDir()
		rotator, err := NewRotator("", FileConfig{Dir: tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer func() { _ = rotator.Close() }()

		expectedPath := filepath.Join(tmpDir, "agent-status.log")
		_, err = rotator.Write([]byte("msg\n"))
		if err != nil {
			t.Fatalf("write error: %v", err)
		}
		_ = rotator.Close()

		if _, err := os.Stat(expectedPath); err != nil {
			t.Errorf("expected file %q to exist, err: %v", expectedPath, err)
		}
	})

	t.Run("mkdir failure returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		blockerFile := filepath.Join(tmpDir, "blocker")
		if err := os.WriteFile(blockerFile, []byte("file"), 0644); err != nil {
			t.Fatal(err)
		}

		invalidPath := filepath.Join(blockerFile, "sub", "app.log")
		rotator, err := NewRotator("test", FileConfig{Filename: invalidPath})
		if err == nil {
			_ = rotator.Close()
			t.Errorf("expected error when creating directory under file, got nil")
		}
	})
}

func TestSetupWithOptionsAndDiskLogging(t *testing.T) {
	t.Run("writes to both stderr buffer and disk file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "air-test.log")

		var stderrBuf bytes.Buffer
		isAgent := true

		logger := SetupWithOptions(SetupOptions{
			Service: "test-service",
			Level:   "info",
			Writer:  &stderrBuf,
			FileConfig: &FileConfig{
				Filename:   logFile,
				MaxSizeMB:  5,
				MaxBackups: 3,
				MaxAgeDays: 7,
				Compress:   false,
			},
			IsAgent: &isAgent,
		})

		if logger == nil {
			t.Fatal("expected non-nil logger")
		}

		logger.Info("disk_log_event", "key1", "val1", "num", 42)

		// Verify stderr output
		if !strings.Contains(stderrBuf.String(), "action=disk_log_event") {
			t.Errorf("expected action=disk_log_event in stderr, got: %s", stderrBuf.String())
		}
		if !strings.Contains(stderrBuf.String(), "key1=val1") || !strings.Contains(stderrBuf.String(), "num=42") {
			t.Errorf("expected key1=val1 and num=42 in stderr, got: %s", stderrBuf.String())
		}

		// Close rotator to flush
		if err := Close(); err != nil {
			t.Fatalf("Close() error: %v", err)
		}

		// Verify disk file output
		diskData, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatalf("reading disk log file: %v", err)
		}
		diskContent := string(diskData)
		if !strings.Contains(diskContent, "action=disk_log_event") {
			t.Errorf("expected action=disk_log_event in disk log file, got: %s", diskContent)
		}
		if !strings.Contains(diskContent, "key1=val1") || !strings.Contains(diskContent, "num=42") {
			t.Errorf("expected key1=val1 in disk log file, got: %s", diskContent)
		}
	})

	t.Run("rotator error falls back gracefully to base writer", func(t *testing.T) {
		tmpDir := t.TempDir()
		blockerFile := filepath.Join(tmpDir, "blocker")
		if err := os.WriteFile(blockerFile, []byte("file"), 0644); err != nil {
			t.Fatal(err)
		}

		invalidPath := filepath.Join(blockerFile, "sub", "app.log")
		var stderrBuf bytes.Buffer
		isAgent := false

		logger := SetupWithOptions(SetupOptions{
			Service: "fallback-svc",
			Level:   "info",
			Writer:  &stderrBuf,
			FileConfig: &FileConfig{
				Filename: invalidPath,
			},
			IsAgent: &isAgent,
		})

		if logger == nil {
			t.Fatal("expected non-nil logger on fallback")
		}

		logger.Info("fallback_event", "tag", "ok")
		if !strings.Contains(stderrBuf.String(), "fallback_event") {
			t.Errorf("expected log output in base writer on fallback, got: %s", stderrBuf.String())
		}
	})
}

func TestClose(t *testing.T) {
	_ = Close()
	if err := Close(); err != nil {
		t.Errorf("Close() when nil = %v, want nil", err)
	}
}
