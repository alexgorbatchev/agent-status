package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
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

// IsAgentMode returns true if the AGENT environment variable is set to "1", "true", or "yes".
func IsAgentMode() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT")))
	return v == "1" || v == "true" || v == "yes"
}

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

// humanHandler implements a clean, human-readable slog.Handler.
type humanHandler struct {
	w      io.Writer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func newHumanHandler(w io.Writer, level slog.Level) *humanHandler {
	return &humanHandler{
		w:     w,
		level: level,
		mu:    &sync.Mutex{},
	}
}

func (h *humanHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *humanHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &humanHandler{
		w:      h.w,
		level:  h.level,
		attrs:  newAttrs,
		groups: h.groups,
		mu:     h.mu,
	}
}

func (h *humanHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &humanHandler{
		w:      h.w,
		level:  h.level,
		attrs:  h.attrs,
		groups: newGroups,
		mu:     h.mu,
	}
}

func (h *humanHandler) Handle(_ context.Context, r slog.Record) error {
	var serviceName string
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())

	addAttr := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if len(h.groups) > 0 {
			a.Key = strings.Join(h.groups, ".") + "." + a.Key
		}
		if a.Key == "service" {
			serviceName = a.Value.String()
			return
		}
		allAttrs = append(allAttrs, a)
	}

	for _, a := range h.attrs {
		addAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		addAttr(a)
		return true
	})

	hasEnv := false
	hasProject := false
	for _, a := range allAttrs {
		if a.Key == "env" && a.Value.String() != "" {
			hasEnv = true
		}
		if a.Key == "project" && a.Value.String() != "" {
			hasProject = true
		}
	}

	var buf bytes.Buffer
	t := r.Time
	if t.IsZero() {
		buf.WriteString("00:00:00")
	} else {
		buf.WriteString(t.Format("15:04:05"))
	}

	buf.WriteString(" ")
	switch r.Level {
	case slog.LevelDebug:
		buf.WriteString("[DEBUG]")
	case slog.LevelInfo:
		buf.WriteString("[INFO]")
	case slog.LevelWarn:
		buf.WriteString("[WARN]")
	case slog.LevelError:
		buf.WriteString("[ERROR]")
	default:
		_, _ = fmt.Fprintf(&buf, "[%s]", r.Level.String())
	}

	if serviceName != "" {
		buf.WriteString(" [")
		buf.WriteString(serviceName)
		buf.WriteString("]")
	}

	if r.Message != "" {
		buf.WriteString(" ")
		buf.WriteString(r.Message)
	}

	for _, a := range allAttrs {
		if hasEnv && a.Key == "envId" {
			continue
		}
		if hasProject && a.Key == "projectId" {
			continue
		}

		buf.WriteString(" ")
		buf.WriteString(a.Key)
		buf.WriteString("=")
		buf.WriteString(formatAttrValue(a.Value))
	}
	buf.WriteString("\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf.Bytes())
	return err
}

func formatAttrValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\n\"\\=") || s == "" {
			return strconv.Quote(s)
		}
		return s
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	default:
		s := fmt.Sprint(v.Any())
		if strings.ContainsAny(s, " \t\n\"\\=") || s == "" {
			return strconv.Quote(s)
		}
		return s
	}
}

// NewWithMode creates a structured logger using either compact machine logfmt with action key (agent mode) or clean human formatting.
func NewWithMode(w io.Writer, service string, levelStr string, isAgent bool) *slog.Logger {
	level, err := ParseLevel(levelStr)
	if err != nil {
		level = slog.LevelInfo
	}
	var handler slog.Handler
	if isAgent {
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{
			Level: level,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if len(groups) == 0 && a.Key == slog.MessageKey {
					a.Key = "action"
				}
				return a
			},
		})
	} else {
		handler = newHumanHandler(w, level)
	}
	logger := slog.New(handler)
	if service != "" {
		logger = logger.With("service", service)
	}
	return logger
}

// New creates a new logger writing to w respecting the AGENT environment variable.
func New(w io.Writer, service string, levelStr string) *slog.Logger {
	return NewWithMode(w, service, levelStr, IsAgentMode())
}

// Setup initializes a logger writing to os.Stderr for the given service at the specified level and sets it as the default slog logger.
func Setup(service string, levelStr string) *slog.Logger {
	logger := New(os.Stderr, service, levelStr)
	slog.SetDefault(logger)
	return logger
}

// Debug logs at LevelDebug using the default logger.
func Debug(action string, args ...any) {
	slog.Default().Debug(action, args...)
}

// Info logs at LevelInfo using the default logger.
func Info(action string, args ...any) {
	slog.Default().Info(action, args...)
}

// Warn logs at LevelWarn using the default logger.
func Warn(action string, args ...any) {
	slog.Default().Warn(action, args...)
}

// Error logs at LevelError using the default logger.
func Error(action string, args ...any) {
	slog.Default().Error(action, args...)
}

// DebugContext logs at LevelDebug with the given context using the default logger.
func DebugContext(ctx context.Context, action string, args ...any) {
	slog.Default().DebugContext(ctx, action, args...)
}

// InfoContext logs at LevelInfo with the given context using the default logger.
func InfoContext(ctx context.Context, action string, args ...any) {
	slog.Default().InfoContext(ctx, action, args...)
}

// WarnContext logs at LevelWarn with the given context using the default logger.
func WarnContext(ctx context.Context, action string, args ...any) {
	slog.Default().WarnContext(ctx, action, args...)
}

// ErrorContext logs at LevelError with the given context using the default logger.
func ErrorContext(ctx context.Context, action string, args ...any) {
	slog.Default().ErrorContext(ctx, action, args...)
}

// With returns a Logger that includes the given attributes in each output operation.
func With(args ...any) *slog.Logger {
	return slog.Default().With(args...)
}
