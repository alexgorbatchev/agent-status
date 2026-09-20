package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Type aliases and constants re-exported from log/slog for consumers.
type (
	Logger         = slog.Logger
	Level          = slog.Level
	Handler        = slog.Handler
	HandlerOptions = slog.HandlerOptions
	Attr           = slog.Attr
	Value          = slog.Value
)

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
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

// New creates a new structured logger using TextHandler writing to w with an optional service attribute.
func New(w io.Writer, service string, levelStr string) *slog.Logger {
	level, err := ParseLevel(levelStr)
	if err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
	if service != "" {
		logger = logger.With("service", service)
	}
	return logger
}

// Setup initializes a logger writing to os.Stderr for the given service at the specified level and sets it as the default slog logger.
func Setup(service string, levelStr string) *slog.Logger {
	logger := New(os.Stderr, service, levelStr)
	slog.SetDefault(logger)
	return logger
}

// Debug logs at LevelDebug using the default logger.
func Debug(msg string, args ...any) {
	slog.Default().Debug(msg, args...)
}

// Info logs at LevelInfo using the default logger.
func Info(msg string, args ...any) {
	slog.Default().Info(msg, args...)
}

// Warn logs at LevelWarn using the default logger.
func Warn(msg string, args ...any) {
	slog.Default().Warn(msg, args...)
}

// Error logs at LevelError using the default logger.
func Error(msg string, args ...any) {
	slog.Default().Error(msg, args...)
}

// DebugContext logs at LevelDebug with the given context using the default logger.
func DebugContext(ctx context.Context, msg string, args ...any) {
	slog.Default().DebugContext(ctx, msg, args...)
}

// InfoContext logs at LevelInfo with the given context using the default logger.
func InfoContext(ctx context.Context, msg string, args ...any) {
	slog.Default().InfoContext(ctx, msg, args...)
}

// WarnContext logs at LevelWarn with the given context using the default logger.
func WarnContext(ctx context.Context, msg string, args ...any) {
	slog.Default().WarnContext(ctx, msg, args...)
}

// ErrorContext logs at LevelError with the given context using the default logger.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	slog.Default().ErrorContext(ctx, msg, args...)
}

// With returns a Logger that includes the given attributes in each output operation.
func With(args ...any) *slog.Logger {
	return slog.Default().With(args...)
}
