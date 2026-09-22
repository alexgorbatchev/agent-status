package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
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
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())

	addAttr := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if len(h.groups) > 0 {
			a.Key = strings.Join(h.groups, ".") + "." + a.Key
		}
		// In human mode, hide redundant service=... attribute since dev-stack / runner provides service prefix
		if a.Key == "service" {
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

// FileConfig defines options for rotating log files on disk.
type FileConfig struct {
	// Filename is the full path to the log file. If empty, Dir and Service are used to construct it.
	Filename string
	// Dir is the directory where log files are written if Filename is not provided.
	Dir string
	// MaxSizeMB is the maximum size in megabytes of the log file before it gets rotated. Defaults to 10 MB.
	MaxSizeMB int
	// MaxBackups is the maximum number of old log files to retain. Defaults to 5.
	MaxBackups int
	// MaxAgeDays is the maximum number of days to retain old log files based on timestamp. Defaults to 14 days.
	MaxAgeDays int
	// Compress determines if the rotated log files should be compressed using gzip. Defaults to true.
	Compress bool
	// Disabled disables writing logs to disk when true.
	Disabled bool
}

const (
	// DefaultMaxSizeMB is the default maximum size of a log file in megabytes before rotation.
	DefaultMaxSizeMB = 10
	// DefaultMaxBackups is the default maximum number of old log files to retain.
	DefaultMaxBackups = 5
	// DefaultMaxAgeDays is the default maximum number of days to retain old log files.
	DefaultMaxAgeDays = 14
)

var (
	rotatorMu      sync.Mutex
	defaultRotator io.Closer
)

// DefaultLogDir returns the default directory for log files based on environment variables and XDG specifications.
// Priority:
// 1. AIR_LOG_DIR
// 2. $XDG_STATE_HOME/agent-status/logs
// 3. $XDG_DATA_HOME/agent-status/logs
// 4. ~/.local/state/agent-status/logs
func DefaultLogDir() string {
	if dir := strings.TrimSpace(os.Getenv("AIR_LOG_DIR")); dir != "" {
		return dir
	}
	if stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); stateHome != "" {
		return filepath.Join(stateHome, "agent-status", "logs")
	}
	if dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dataHome != "" {
		return filepath.Join(dataHome, "agent-status", "logs")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "agent-status", "logs")
	}
	return filepath.Join(home, ".local", "state", "agent-status", "logs")
}

// LoadFileConfigFromEnv populates FileConfig from environment variables and defaults.
func LoadFileConfigFromEnv(service string) FileConfig {
	disabled := false
	if val := strings.TrimSpace(os.Getenv("AIR_LOG_TO_DISK")); val != "" {
		lower := strings.ToLower(val)
		if lower == "0" || lower == "false" || lower == "no" || lower == "off" {
			disabled = true
		}
	}

	maxSize := DefaultMaxSizeMB
	if s := os.Getenv("AIR_LOG_MAX_SIZE_MB"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxSize = n
		}
	}

	maxBackups := DefaultMaxBackups
	if s := os.Getenv("AIR_LOG_MAX_BACKUPS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			maxBackups = n
		}
	}

	maxAge := DefaultMaxAgeDays
	if s := os.Getenv("AIR_LOG_MAX_AGE_DAYS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			maxAge = n
		}
	}

	compress := true
	if s := os.Getenv("AIR_LOG_COMPRESS"); s != "" {
		lower := strings.ToLower(s)
		if lower == "0" || lower == "false" || lower == "no" || lower == "off" {
			compress = false
		}
	}

	filename := os.Getenv("AIR_LOG_FILE")
	dir := os.Getenv("AIR_LOG_DIR")

	return FileConfig{
		Filename:   filename,
		Dir:        dir,
		MaxSizeMB:  maxSize,
		MaxBackups: maxBackups,
		MaxAgeDays: maxAge,
		Compress:   compress,
		Disabled:   disabled,
	}
}

// NewRotator creates a rolling file writer with the given configuration using lumberjack.
func NewRotator(service string, cfg FileConfig) (io.WriteCloser, error) {
	if cfg.Disabled {
		return nil, nil
	}
	filename := cfg.Filename
	if filename == "" {
		dir := cfg.Dir
		if dir == "" {
			dir = DefaultLogDir()
		}
		name := service
		if name == "" {
			name = "agent-status"
		}
		filename = filepath.Join(dir, name+".log")
	}

	maxSize := cfg.MaxSizeMB
	if maxSize <= 0 {
		maxSize = DefaultMaxSizeMB
	}
	maxBackups := cfg.MaxBackups
	if maxBackups <= 0 {
		maxBackups = DefaultMaxBackups
	}
	maxAge := cfg.MaxAgeDays
	if maxAge <= 0 {
		maxAge = DefaultMaxAgeDays
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating log directory %q: %w", dir, err)
	}

	return &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   cfg.Compress,
	}, nil
}

// SetupOptions holds optional parameters for configuring a service logger.
type SetupOptions struct {
	Service    string
	Level      string
	Writer     io.Writer
	FileConfig *FileConfig
	IsAgent    *bool
}

// SetupWithOptions configures and sets the default slog logger according to the given options.
func SetupWithOptions(opts SetupOptions) *slog.Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}

	var fileCfg FileConfig
	if opts.FileConfig != nil {
		fileCfg = *opts.FileConfig
	} else {
		fileCfg = LoadFileConfigFromEnv(opts.Service)
	}

	if !fileCfg.Disabled {
		rotator, err := NewRotator(opts.Service, fileCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] failed to initialize log rotator for %q: %v\n", opts.Service, err)
		} else if rotator != nil {
			rotatorMu.Lock()
			if defaultRotator != nil {
				_ = defaultRotator.Close()
			}
			defaultRotator = rotator
			rotatorMu.Unlock()
			w = io.MultiWriter(w, rotator)
		}
	}

	isAgent := IsAgentMode()
	if opts.IsAgent != nil {
		isAgent = *opts.IsAgent
	}

	logger := NewWithMode(w, opts.Service, opts.Level, isAgent)
	slog.SetDefault(logger)
	return logger
}

// Close closes the active disk rotator if one was configured by Setup or SetupWithOptions.
func Close() error {
	rotatorMu.Lock()
	defer rotatorMu.Unlock()
	if defaultRotator != nil {
		err := defaultRotator.Close()
		defaultRotator = nil
		return err
	}
	return nil
}

// Setup initializes a logger for the given service at the specified level and sets it as the default slog logger.
// It writes to os.Stderr and also rotates to disk under the default log directory unless disabled via AIR_LOG_TO_DISK=0.
func Setup(service string, levelStr string) *slog.Logger {
	return SetupWithOptions(SetupOptions{
		Service: service,
		Level:   levelStr,
	})
}

// SetDefault sets the default logger.
func SetDefault(logger *Logger) {
	slog.SetDefault(logger)
}

// Default returns the default logger.
func Default() *Logger {
	return slog.Default()
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
