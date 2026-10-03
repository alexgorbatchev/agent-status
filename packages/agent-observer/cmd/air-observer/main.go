package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/config"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/git"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/shipper"
	watcher "github.com/alexgorbatchev/agent-watcher"
	"github.com/alexgorbatchev/agent-watcher/scanner"
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"
)

var version = "dev"

type observerOptions struct {
	ingesterURL          string
	token                string
	envFlag              string
	scanInterval         time.Duration
	cursorPath           string
	cursorRetention      time.Duration
	cursorPruneInterval  time.Duration
	cursorFlushInterval  time.Duration
	sessionStatePath     string
	claudeSessionsDir    string
	claudeProjectsDir    string
	piSessionsDir        string
	codexSessionsDir     string
	opencodeDBPath       string
	logLevel             string
	logDir               string
	logFile              string
	procSnapshotProvider scanner.ProcessSnapshotProvider
}

func newRootCmd() (*cobra.Command, *observerOptions) {
	opts := &observerOptions{
		ingesterURL:         "ws://127.0.0.1:4501/ws/ingest",
		scanInterval:        2 * time.Second,
		cursorRetention:     24 * time.Hour,
		cursorPruneInterval: 1 * time.Hour,
		cursorFlushInterval: 2 * time.Second,
		logLevel:            "info",
	}

	if envURL := os.Getenv("AGENT_INGESTER_URL"); envURL != "" {
		opts.ingesterURL = envURL
	}
	if envToken := os.Getenv("AGENT_STATUS_TOKEN"); envToken != "" {
		opts.token = envToken
	}
	if envState := os.Getenv("AIR_SESSION_STATE_PATH"); envState != "" {
		opts.sessionStatePath = envState
	}
	if envRet := os.Getenv("AIR_CURSOR_RETENTION"); envRet != "" {
		if d, err := time.ParseDuration(envRet); err == nil {
			opts.cursorRetention = d
		}
	}
	if envPrune := os.Getenv("AIR_CURSOR_PRUNE_INTERVAL"); envPrune != "" {
		if d, err := time.ParseDuration(envPrune); err == nil {
			opts.cursorPruneInterval = d
		}
	}
	if envFlush := os.Getenv("AIR_CURSOR_FLUSH_INTERVAL"); envFlush != "" {
		if d, err := time.ParseDuration(envFlush); err == nil {
			opts.cursorFlushInterval = d
		}
	}

	rootCmd := &cobra.Command{
		Use:   "air-observer",
		Short: "Observer agent for local harness session monitoring and telemetry shipping",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runObserver(*opts)
		},
	}

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	flags := rootCmd.Flags()
	flags.StringVar(&opts.ingesterURL, "ingester-url", opts.ingesterURL, "Ingester WebSocket URL (default ws://127.0.0.1:4501/ws/ingest)")
	flags.StringVar(&opts.token, "token", opts.token, "Bearer token for ingester authentication")
	flags.StringVar(&opts.envFlag, "env", "", "Environment label override")
	flags.DurationVar(&opts.scanInterval, "scan-interval", opts.scanInterval, "Interval between session scans (default 2s)")
	flags.StringVar(&opts.cursorPath, "cursor-path", "", "Custom path for persistent cursor file")
	flags.DurationVar(&opts.cursorRetention, "cursor-retention", opts.cursorRetention, "Retention duration for inactive cursor entries (default 24h)")
	flags.DurationVar(&opts.cursorPruneInterval, "cursor-prune-interval", opts.cursorPruneInterval, "Interval between periodic cursor pruning sweeps (default 1h)")
	flags.DurationVar(&opts.cursorFlushInterval, "cursor-flush-interval", opts.cursorFlushInterval, "Interval between periodic cursor store flushes (default 2s)")
	flags.StringVar(&opts.sessionStatePath, "session-state-path", "", "Custom path for persistent session state tracking file")
	flags.StringVar(&opts.claudeSessionsDir, "claude-sessions-dir", "", "Custom directory for Claude Code sessions")
	flags.StringVar(&opts.claudeProjectsDir, "claude-projects-dir", "", "Custom directory for Claude Code projects/transcripts")
	flags.StringVar(&opts.piSessionsDir, "pi-sessions-dir", "", "Custom directory for Pi sessions")
	flags.StringVar(&opts.codexSessionsDir, "codex-sessions-dir", "", "Custom directory for Codex sessions")
	flags.StringVar(&opts.opencodeDBPath, "opencode-db-path", "", "Custom path for OpenCode database")
	flags.StringVar(&opts.logLevel, "log-level", opts.logLevel, "Log level (debug, info, warn, error)")
	flags.StringVar(&opts.logDir, "log-dir", "", "Custom directory for rotating log files on disk (default: $XDG_STATE_HOME/agent-status/logs)")
	flags.StringVar(&opts.logFile, "log-file", "", "Custom path for rotating log file on disk")

	cobrahelptree.Setup(rootCmd)
	return rootCmd, opts
}

func main() {
	rootCmd, _ := newRootCmd()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runObserver(opts observerOptions) error {
	var fileCfg *logging.FileConfig
	if opts.logFile != "" || opts.logDir != "" {
		cfg := logging.LoadFileConfigFromEnv("observer")
		if opts.logFile != "" {
			cfg.Filename = opts.logFile
		}
		if opts.logDir != "" {
			cfg.Dir = opts.logDir
		}
		fileCfg = &cfg
	}
	logging.SetupWithOptions(logging.SetupOptions{
		Service:    "observer",
		Level:      opts.logLevel,
		FileConfig: fileCfg,
	})
	defer func() {
		_ = logging.Close()
	}()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runObserverContext(ctx, opts)
}

func runObserverContext(ctx context.Context, opts observerOptions) error {
	env, err := config.ResolveEnvironment(opts.envFlag)
	if err != nil {
		return fmt.Errorf("resolving environment: %w", err)
	}
	logging.Info("observer_starting", "env", env.Label, "envId", env.ID, "ingesterUrl", opts.ingesterURL)
	defer logging.Info("observer_shutting_down")
	ship := shipper.NewShipper(shipper.Config{
		IngesterURL: opts.ingesterURL, BearerToken: opts.token, Environment: *env,
	})
	if err := ship.Start(ctx); err != nil {
		return fmt.Errorf("starting shipper: %w", err)
	}
	defer ship.Stop()
	cfg := watcher.Config{
		ScanInterval: opts.scanInterval, ClaudeProjectsDir: opts.claudeProjectsDir,
		PiSessionsDir: opts.piSessionsDir, CodexSessionsDir: opts.codexSessionsDir,
		OpencodeDBPath: opts.opencodeDBPath, CursorPath: opts.cursorPath,
		SessionStatePath: opts.sessionStatePath, CursorRetention: opts.cursorRetention,
		CursorPruneInterval: opts.cursorPruneInterval, CursorFlushInterval: opts.cursorFlushInterval,
		ProcessSnapshotProvider: opts.procSnapshotProvider, Logger: logging.Default(),
	}
	if opts.claudeSessionsDir != "" {
		cfg.ClaudeSessionsDirs = []string{opts.claudeSessionsDir}
	} else if custom := os.Getenv("CLAUDE_SESSIONS_DIR"); custom != "" {
		cfg.ClaudeSessionsDirs = []string{custom}
	}
	if cfg.CursorPath == "" {
		cfg.CursorPath = config.ObserverCachePath("observer-cursors.json")
	}
	if cfg.SessionStatePath == "" {
		cfg.SessionStatePath = config.ObserverCachePath("observer-sessions.json")
	}
	resolver := git.NewResolver(30 * time.Second)
	err = watcher.Run(ctx, cfg, func(ctx context.Context, event watcher.Event, acknowledge func()) error {
		frame := telemetryFrame(ctx, *env, resolver, event)
		return ship.EnqueueWithAck(ctx, frame, shipper.AckCallback(acknowledge))
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func telemetryFrame(ctx context.Context, env types.IEnvironmentInfo, resolver *git.Resolver, event watcher.Event) types.ITelemetryFrame {
	project, workspace, _ := resolver.Resolve(ctx, event.CWD, event.GitBranch)
	return types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env,
		AgentID:   fmt.Sprintf("%s:%s:%s", env.ID, event.Harness, event.SessionID),
		SessionID: event.SessionID, SubagentID: event.SubagentID,
		PID: event.PID, Harness: types.HarnessType(event.Harness), HarnessVersion: event.HarnessVersion,
		Project: project, Workspace: workspace, Timestamp: event.Timestamp,
		EventType: event.EventType, Data: event.Data,
	}
}
