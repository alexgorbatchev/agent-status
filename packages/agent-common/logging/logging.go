package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// ParseLevel parses a case-insensitive log level string ("debug", "info", "warn", "warning", "error").
// An empty string defaults to slog.LevelInfo with no error.
func ParseLevel(s string) (slog.Level, error) {
	trimmed := strings.TrimSpace(strings.ToLower(s))
	switch trimmed {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level %q (valid: debug, info, warn, error)", s)
	}
}

// New creates a new structured slog.Logger using TextHandler writing to w.
func New(w io.Writer, levelStr string) *slog.Logger {
	level, err := ParseLevel(levelStr)
	if err != nil {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
}

// Setup initializes a logger writing to os.Stderr at the given level and sets it as the default slog logger.
func Setup(levelStr string) *slog.Logger {
	logger := New(os.Stderr, levelStr)
	slog.SetDefault(logger)
	return logger
}
