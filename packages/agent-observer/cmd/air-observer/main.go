package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/config"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/git"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/scanner"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/shipper"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/tailer"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"
)

var version = "dev"

type observerOptions struct {
	ingesterURL       string
	token             string
	envFlag           string
	scanInterval      time.Duration
	cursorPath        string
	claudeSessionsDir string
	claudeProjectsDir string
	piSessionsDir     string
	logLevel          string
}

func newRootCmd() (*cobra.Command, *observerOptions) {
	opts := &observerOptions{
		ingesterURL:  "ws://127.0.0.1:4501/ws/ingest",
		scanInterval: 2 * time.Second,
		logLevel:     "info",
	}

	if envURL := os.Getenv("AGENT_INGESTER_URL"); envURL != "" {
		opts.ingesterURL = envURL
	}
	if envToken := os.Getenv("AGENT_STATUS_TOKEN"); envToken != "" {
		opts.token = envToken
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
	flags.StringVar(&opts.claudeSessionsDir, "claude-sessions-dir", "", "Custom directory for Claude Code sessions")
	flags.StringVar(&opts.claudeProjectsDir, "claude-projects-dir", "", "Custom directory for Claude Code projects/transcripts")
	flags.StringVar(&opts.piSessionsDir, "pi-sessions-dir", "", "Custom directory for Pi sessions")
	flags.StringVar(&opts.logLevel, "log-level", opts.logLevel, "Log level (debug, info, warn, error)")

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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logging.Setup("observer", opts.logLevel)

	if opts.claudeProjectsDir != "" {
		_ = os.Setenv("CLAUDE_PROJECTS_DIR", opts.claudeProjectsDir)
	}
	if opts.claudeSessionsDir != "" {
		_ = os.Setenv("CLAUDE_SESSIONS_DIR", opts.claudeSessionsDir)
	}
	if opts.piSessionsDir != "" {
		_ = os.Setenv("PI_SESSIONS_DIR", opts.piSessionsDir)
	}

	env, err := config.ResolveEnvironment(opts.envFlag)
	if err != nil {
		return fmt.Errorf("resolving environment: %w", err)
	}

	cursorStore, err := tailer.NewCursorStore(opts.cursorPath)
	if err != nil {
		return fmt.Errorf("initializing cursor store: %w", err)
	}

	ship := shipper.NewShipper(shipper.Config{
		IngesterURL: opts.ingesterURL,
		BearerToken: opts.token,
		Environment: *env,
	})

	if err := ship.Start(ctx); err != nil {
		return fmt.Errorf("starting shipper: %w", err)
	}
	defer ship.Stop()

	gitResolver := git.NewResolver(30 * time.Second)

	var ccDirs []string
	if opts.claudeSessionsDir != "" {
		ccDirs = []string{opts.claudeSessionsDir}
	} else if custom := os.Getenv("CLAUDE_SESSIONS_DIR"); custom != "" {
		ccDirs = []string{custom}
	}
	if opts.claudeProjectsDir != "" {
		_ = os.Setenv("CLAUDE_PROJECTS_DIR", opts.claudeProjectsDir)
	}
	ccScanner := scanner.NewClaudeCodeScanner(ccDirs)
	var piDir string
	if opts.piSessionsDir != "" {
		piDir = opts.piSessionsDir
	} else if custom := os.Getenv("PI_SESSIONS_DIR"); custom != "" {
		piDir = custom
	}
	piScanner := scanner.NewPiScanner(piDir, nil)

	type activeTail struct {
		tailer *tailer.Tailer
		cancel context.CancelFunc
	}

	var mu sync.Mutex
	activeTails := make(map[string]*activeTail)
	activeClaudeSessions := make(map[string]bool)
	activePiSessions := make(map[string]scanner.PiSessionDescriptor)

	attachTailer := func(
		path string,
		harness types.HarnessType,
		harnessVer string,
		sessionID string,
		cwd string,
		pid int,
		subagentID *string,
	) {
		mu.Lock()
		if _, exists := activeTails[path]; exists {
			mu.Unlock()
			return
		}

		tailCtx, tailCancel := context.WithCancel(ctx)
		t, err := tailer.NewTailer(path, cursorStore, func(line []byte) error {
			var events []parser.ParsedEvent
			var pErr error

			if harness == types.HarnessClaudeCode {
				events, pErr = parser.ParseClaudeCodeLine(line)
			} else {
				events, pErr = parser.ParsePiLine(line, cwd)
			}
			if pErr != nil {
				return pErr
			}

			for _, ev := range events {
				eventCWD := ev.CWD
				if eventCWD == "" {
					eventCWD = cwd
				}

				proj, ws, _ := gitResolver.Resolve(ctx, eventCWD, ev.GitBranch)

				agentID := fmt.Sprintf("%s:%s:%s", env.ID, harness, sessionID)

				frame := types.ITelemetryFrame{
					Version:        "1.0",
					EnvID:          env.ID,
					Environment:    *env,
					AgentID:        agentID,
					SessionID:      sessionID,
					SubagentID:     subagentID,
					PID:            pid,
					Harness:        harness,
					HarnessVersion: harnessVer,
					Project:        proj,
					Workspace:      ws,
					Timestamp:      ev.Timestamp,
					EventType:      ev.EventType,
					Data:           ev.Data,
				}
				ship.Enqueue(frame)
			}
			return nil
		})

		if err != nil {
			tailCancel()
			mu.Unlock()
			return
		}

		activeTails[path] = &activeTail{
			tailer: t,
			cancel: tailCancel,
		}
		mu.Unlock()

		go func() {
			_ = t.Start(tailCtx)
		}()
	}

	scanAll := func() {
		// 1. Scan Claude Code
		ccSessions, err := ccScanner.Scan()
		newCCMap := make(map[string]bool)
		if err == nil {
			for _, s := range ccSessions {
				newCCMap[s.SessionID] = true

				// If newly discovered active session, emit run_started so it registers as RUNNING
				mu.Lock()
				isNew := !activeClaudeSessions[s.SessionID]
				if isNew {
					activeClaudeSessions[s.SessionID] = true
				}
				mu.Unlock()

				proj, ws, _ := gitResolver.Resolve(ctx, s.CWD, "")
				if isNew {
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessClaudeCode, s.SessionID)
					now := time.Now().UnixMilli()
					if s.StartedAt > 0 {
						now = s.StartedAt
					}
					ship.Enqueue(types.ITelemetryFrame{
						Version:        "1.0",
						EnvID:          env.ID,
						Environment:    *env,
						AgentID:        agentID,
						SessionID:      s.SessionID,
						PID:            s.PID,
						Harness:        types.HarnessClaudeCode,
						HarnessVersion: s.HarnessVersion,
						Project:        proj,
						Workspace:      ws,
						Timestamp:      now,
						EventType:      "run_started",
						Data:           types.TelemetryFrameData{},
					})
				}

				if s.TranscriptPath != "" {
					attachTailer(s.TranscriptPath, types.HarnessClaudeCode, s.HarnessVersion, s.SessionID, s.CWD, s.PID, nil)
				}
				if subagents, sErr := ccScanner.DiscoverSubagents(s); sErr == nil {
					for _, sub := range subagents {
						subID := sub.SubagentID
						attachTailer(sub.TranscriptPath, types.HarnessClaudeCode, s.HarnessVersion, s.SessionID, s.CWD, s.PID, &subID)
					}
				}
			}
		}

		// Detect exited Claude Code sessions
		mu.Lock()
		for sessID := range activeClaudeSessions {
			if !newCCMap[sessID] {
				// Claude Code session exited
				agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessClaudeCode, sessID)
				ship.Enqueue(types.ITelemetryFrame{
					Version:        "1.0",
					EnvID:          env.ID,
					Environment:    *env,
					AgentID:        agentID,
					SessionID:      sessID,
					Harness:        types.HarnessClaudeCode,
					Timestamp:      time.Now().UnixMilli(),
					EventType:      "run_exited",
					Data:           types.TelemetryFrameData{},
				})
			}
		}
		activeClaudeSessions = newCCMap
		mu.Unlock()

		// 2. Scan Pi
		piSessions, err := piScanner.Scan(ctx)
		newPiMap := make(map[string]scanner.PiSessionDescriptor)
		if err == nil {
			for _, s := range piSessions {
				newPiMap[s.SessionID] = s

				mu.Lock()
				isNew := !activePiSessions[s.SessionID].IsActive
				if s.IsActive && isNew {
					activePiSessions[s.SessionID] = s
				}
				mu.Unlock()

				if s.IsActive && isNew {
					proj, ws, _ := gitResolver.Resolve(ctx, s.CWD, "")
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessPi, s.SessionID)
					now := time.Now().UnixMilli()
					if s.StartedAt > 0 {
						now = s.StartedAt
					}
					ship.Enqueue(types.ITelemetryFrame{
						Version:        "1.0",
						EnvID:          env.ID,
						Environment:    *env,
						AgentID:        agentID,
						SessionID:      s.SessionID,
						PID:            s.PID,
						Harness:        types.HarnessPi,
						HarnessVersion: s.HarnessVersion,
						Project:        proj,
						Workspace:      ws,
						Timestamp:      now,
						EventType:      "run_started",
						Data:           types.TelemetryFrameData{},
					})
				}
				if s.TranscriptPath != "" {
					attachTailer(s.TranscriptPath, types.HarnessPi, s.HarnessVersion, s.SessionID, s.CWD, s.PID, nil)
				}
			}
		}

		// Detect exited Pi sessions
		mu.Lock()
		for sessID, oldDesc := range activePiSessions {
			newDesc, exists := newPiMap[sessID]
			if oldDesc.IsActive && (!exists || !newDesc.IsActive) {
				// Pi process terminated or session ended
				agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessPi, sessID)
				proj, ws, _ := gitResolver.Resolve(ctx, oldDesc.CWD, "")
				ship.Enqueue(types.ITelemetryFrame{
					Version:        "1.0",
					EnvID:          env.ID,
					Environment:    *env,
					AgentID:        agentID,
					SessionID:      sessID,
					PID:            oldDesc.PID,
					Harness:        types.HarnessPi,
					HarnessVersion: oldDesc.HarnessVersion,
					Project:        proj,
					Workspace:      ws,
					Timestamp:      time.Now().UnixMilli(),
					EventType:      "run_exited",
					Data:           types.TelemetryFrameData{},
				})
			}
		}
		activePiSessions = newPiMap
		mu.Unlock()
	}

	// Perform initial scan
	scanAll()

	scanTicker := time.NewTicker(opts.scanInterval)
	defer scanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, at := range activeTails {
				at.cancel()
				at.tailer.Stop()
			}
			mu.Unlock()
			return nil
		case <-scanTicker.C:
			scanAll()
		}
	}
}
