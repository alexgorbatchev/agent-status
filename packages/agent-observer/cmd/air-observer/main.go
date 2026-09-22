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
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/spf13/cobra"
)

var version = "dev"

type observerOptions struct {
	ingesterURL       string
	token             string
	envFlag           string
	scanInterval      time.Duration
	cursorPath        string
	sessionStatePath  string
	claudeSessionsDir string
	claudeProjectsDir string
	piSessionsDir     string
	codexSessionsDir  string
	opencodeDBPath    string
	logLevel          string
	logDir            string
	logFile           string
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
	if envState := os.Getenv("AIR_SESSION_STATE_PATH"); envState != "" {
		opts.sessionStatePath = envState
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

func parseLineForHarness(harness types.HarnessType, line []byte, cwd string) ([]parser.ParsedEvent, error) {
	switch harness {
	case types.HarnessClaudeCode:
		return parser.ParseClaudeCodeLine(line)
	case types.HarnessPi:
		return parser.ParsePiLine(line, cwd)
	case types.HarnessCodex:
		return parser.ParseCodexLine(line)
	default:
		return nil, fmt.Errorf("unsupported harness type: %q", harness)
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
	if opts.claudeProjectsDir != "" {
		_ = os.Setenv("CLAUDE_PROJECTS_DIR", opts.claudeProjectsDir)
	}
	if opts.claudeSessionsDir != "" {
		_ = os.Setenv("CLAUDE_SESSIONS_DIR", opts.claudeSessionsDir)
	}
	if opts.piSessionsDir != "" {
		_ = os.Setenv("PI_SESSIONS_DIR", opts.piSessionsDir)
	}
	if opts.codexSessionsDir != "" {
		_ = os.Setenv("CODEX_SESSIONS_DIR", opts.codexSessionsDir)
	}
	if opts.opencodeDBPath != "" {
		_ = os.Setenv("OPENCODE_DB_PATH", opts.opencodeDBPath)
	}

	env, err := config.ResolveEnvironment(opts.envFlag)
	if err != nil {
		return fmt.Errorf("resolving environment: %w", err)
	}

	logging.Info("observer_starting", "env", env.Label, "envId", env.ID, "ingesterUrl", opts.ingesterURL)

	cursorStore, err := tailer.NewCursorStore(opts.cursorPath)
	if err != nil {
		return fmt.Errorf("initializing cursor store: %w", err)
	}
	defer func() {
		if err := cursorStore.Save(); err != nil {
			logging.Warn("cursor_store_save_error", "err", err)
		}
	}()

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
	var codexDir string
	if opts.codexSessionsDir != "" {
		codexDir = opts.codexSessionsDir
	} else if custom := os.Getenv("CODEX_SESSIONS_DIR"); custom != "" {
		codexDir = custom
	}
	codexScanner := scanner.NewCodexScanner(codexDir, nil)
	var opencodePath string
	if opts.opencodeDBPath != "" {
		opencodePath = opts.opencodeDBPath
	} else if custom := os.Getenv("OPENCODE_DB_PATH"); custom != "" {
		opencodePath = custom
	}
	opencodeScanner := scanner.NewOpencodeScanner(opencodePath, nil)

	type activeTail struct {
		tailer *tailer.Tailer
		cancel context.CancelFunc
	}

	sessionTracker, err := scanner.NewSessionTracker(opts.sessionStatePath)
	if err != nil {
		logging.Warn("session_tracker_init_error", "err", err)
	}
	defer func() {
		if sessionTracker != nil {
			if sErr := sessionTracker.Save(); sErr != nil {
				logging.Warn("session_tracker_save_error", "err", sErr)
			}
		}
	}()

	type claudeSessionInfo struct {
		SessionID      string
		PID            int
		CWD            string
		HarnessVersion string
		StartedAt      int64
	}

	var mu sync.Mutex
	activeTails := make(map[string]*activeTail)
	sessionTails := make(map[string][]string)
	activeClaudeSessions := make(map[string]claudeSessionInfo)
	activeClaudeSubagents := make(map[string]map[string]bool)
	activePiSessions := make(map[string]scanner.PiSessionDescriptor)
	activeCodexSessions := make(map[string]scanner.CodexSessionDescriptor)
	activeOpencodeSessions := make(map[string]scanner.OpencodeSessionDescriptor)

	if sessionTracker != nil {
		for sessID, s := range sessionTracker.GetAll() {
			if !s.IsActive {
				continue
			}
			switch types.HarnessType(s.Harness) {
			case types.HarnessClaudeCode:
				activeClaudeSessions[sessID] = claudeSessionInfo{
					SessionID:      sessID,
					PID:            s.PID,
					CWD:            s.CWD,
					HarnessVersion: s.HarnessVersion,
					StartedAt:      s.StartedAt,
				}
			case types.HarnessPi:
				activePiSessions[sessID] = scanner.PiSessionDescriptor{
					SessionID:      sessID,
					PID:            s.PID,
					CWD:            s.CWD,
					HarnessVersion: s.HarnessVersion,
					StartedAt:      s.StartedAt,
					IsActive:       true,
				}
			case types.HarnessCodex:
				activeCodexSessions[sessID] = scanner.CodexSessionDescriptor{
					SessionID:      sessID,
					PID:            s.PID,
					CWD:            s.CWD,
					HarnessVersion: s.HarnessVersion,
					StartedAt:      s.StartedAt,
					IsActive:       true,
				}
			case types.HarnessOpencode:
				activeOpencodeSessions[sessID] = scanner.OpencodeSessionDescriptor{
					SessionID:      sessID,
					PID:            s.PID,
					CWD:            s.CWD,
					HarnessVersion: s.HarnessVersion,
					StartedAt:      s.StartedAt,
					IsActive:       true,
				}
			}
		}
	}

	cleanupSessionTailsLocked := func(sessionID string) {
		for _, p := range sessionTails[sessionID] {
			if at, exists := activeTails[p]; exists {
				at.cancel()
				at.tailer.Stop()
				delete(activeTails, p)
			}
		}
		delete(sessionTails, sessionID)
	}

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
		var t *tailer.Tailer
		t, err := tailer.NewTailerWithOffsetHandler(path, cursorStore, func(line []byte, offset int64) error {
			events, pErr := parseLineForHarness(harness, line, cwd)
			if pErr != nil {
				logging.Warn("transcript_line_parse_failed", "harness", harness, "path", path, "err", pErr)
				_ = t.Commit(offset)
				return nil
			}

			if len(events) == 0 {
				_ = t.Commit(offset)
				return nil
			}

			for i, ev := range events {
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
				var onAck shipper.AckCallback
				if i == len(events)-1 {
					onAck = func() {
						_ = t.Commit(offset)
					}
				}
				err := ship.EnqueueWithAck(tailCtx, frame, onAck)
				if err != nil {
					return err
				}
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
		sessionTails[sessionID] = append(sessionTails[sessionID], path)
		mu.Unlock()

		logging.Info("session_attached", "harness", harness, "sessionId", sessionID, "pid", pid, "cwd", cwd)

		go func() {
			_ = t.Start(tailCtx)
		}()
	}

	scanAll := func() {
		// 1. Scan Claude Code
		ccSessions, err := ccScanner.Scan()
		newCCMap := make(map[string]claudeSessionInfo)
		if err != nil {
			logging.Warn("scan_failed", "harness", types.HarnessClaudeCode, "err", err)
		} else {
			for _, s := range ccSessions {
				info := claudeSessionInfo{
					SessionID:      s.SessionID,
					PID:            s.PID,
					CWD:            s.CWD,
					HarnessVersion: s.HarnessVersion,
					StartedAt:      s.StartedAt,
				}
				newCCMap[s.SessionID] = info

				if sessionTracker != nil {
					sessionTracker.Set(s.SessionID, scanner.TrackedSession{
						SessionID:      s.SessionID,
						Harness:        string(types.HarnessClaudeCode),
						PID:            s.PID,
						CWD:            s.CWD,
						HarnessVersion: s.HarnessVersion,
						StartedAt:      s.StartedAt,
						IsActive:       true,
					})
				}

				// If newly discovered active session, emit run_started so it registers as RUNNING
				mu.Lock()
				_, alreadyActive := activeClaudeSessions[s.SessionID]
				isNew := !alreadyActive
				if isNew {
					activeClaudeSessions[s.SessionID] = info
				}
				mu.Unlock()

				proj, ws, _ := gitResolver.Resolve(ctx, s.CWD, "")
				if isNew {
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessClaudeCode, s.SessionID)
					now := time.Now().UnixMilli()
					if s.StartedAt > 0 {
						now = s.StartedAt
					}
					var titlePtr *string
					if s.NameSource == "custom" && s.Name != "" {
						titlePtr = &s.Name
					}
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
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
						Data: types.TelemetryFrameData{
							Title: titlePtr,
						},
					})
				}

				if s.TranscriptPath != "" {
					attachTailer(s.TranscriptPath, types.HarnessClaudeCode, s.HarnessVersion, s.SessionID, s.CWD, s.PID, nil)
				}
				if subagents, sErr := ccScanner.DiscoverSubagents(s); sErr == nil {
					for _, sub := range subagents {
						subID := sub.SubagentID

						mu.Lock()
						sessionSubs := activeClaudeSubagents[s.SessionID]
						if sessionSubs == nil {
							sessionSubs = make(map[string]bool)
							activeClaudeSubagents[s.SessionID] = sessionSubs
						}
						isNewSub := !sessionSubs[subID]
						if isNewSub {
							sessionSubs[subID] = true
						}
						mu.Unlock()

						if isNewSub {
							agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessClaudeCode, s.SessionID)
							subNow := time.Now().UnixMilli()
							if fi, statErr := os.Stat(sub.TranscriptPath); statErr == nil {
								mod := fi.ModTime().UnixMilli()
								if mod > 0 && mod < subNow {
									subNow = mod
								}
							}
							if s.StartedAt > 0 && subNow < s.StartedAt {
								subNow = s.StartedAt
							}
							_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
								Version:        "1.0",
								EnvID:          env.ID,
								Environment:    *env,
								AgentID:        agentID,
								SessionID:      s.SessionID,
								SubagentID:     &subID,
								PID:            s.PID,
								Harness:        types.HarnessClaudeCode,
								HarnessVersion: s.HarnessVersion,
								Project:        proj,
								Workspace:      ws,
								Timestamp:      subNow,
								EventType:      "run_started",
								Data:           types.TelemetryFrameData{},
							})
						}

						attachTailer(sub.TranscriptPath, types.HarnessClaudeCode, s.HarnessVersion, s.SessionID, s.CWD, s.PID, &subID)
					}
				}
			}

			// Detect exited Claude Code sessions
			mu.Lock()
			for sessID, oldInfo := range activeClaudeSessions {
				if _, exists := newCCMap[sessID]; !exists {
					// Claude Code session exited
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessClaudeCode, sessID)
					proj, ws, _ := gitResolver.Resolve(ctx, oldInfo.CWD, "")
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
						Version:        "1.0",
						EnvID:          env.ID,
						Environment:    *env,
						AgentID:        agentID,
						SessionID:      sessID,
						PID:            oldInfo.PID,
						Harness:        types.HarnessClaudeCode,
						HarnessVersion: oldInfo.HarnessVersion,
						Project:        proj,
						Workspace:      ws,
						Timestamp:      time.Now().UnixMilli(),
						EventType:      "run_exited",
						Data:           types.TelemetryFrameData{},
					})
					cleanupSessionTailsLocked(sessID)
					delete(activeClaudeSubagents, sessID)
					if sessionTracker != nil {
						sessionTracker.Delete(sessID)
					}
				}
			}
			activeClaudeSessions = newCCMap
			mu.Unlock()
		}

		// 2. Scan Pi
		piSessions, err := piScanner.Scan(ctx)
		newPiMap := make(map[string]scanner.PiSessionDescriptor)
		if err != nil {
			logging.Warn("scan_failed", "harness", types.HarnessPi, "err", err)
		} else {
			for _, s := range piSessions {
				newPiMap[s.SessionID] = s

				mu.Lock()
				isNew := !activePiSessions[s.SessionID].IsActive
				if s.IsActive && isNew {
					activePiSessions[s.SessionID] = s
				}
				mu.Unlock()

				if s.IsActive && sessionTracker != nil {
					sessionTracker.Set(s.SessionID, scanner.TrackedSession{
						SessionID:      s.SessionID,
						Harness:        string(types.HarnessPi),
						PID:            s.PID,
						CWD:            s.CWD,
						HarnessVersion: s.HarnessVersion,
						StartedAt:      s.StartedAt,
						IsActive:       true,
					})
				}

				if s.IsActive && isNew {
					proj, ws, _ := gitResolver.Resolve(ctx, s.CWD, "")
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessPi, s.SessionID)
					now := time.Now().UnixMilli()
					if s.StartedAt > 0 {
						now = s.StartedAt
					}
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
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
				if s.IsActive && s.TranscriptPath != "" {
					attachTailer(s.TranscriptPath, types.HarnessPi, s.HarnessVersion, s.SessionID, s.CWD, s.PID, nil)
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
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
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
					cleanupSessionTailsLocked(sessID)
					if sessionTracker != nil {
						sessionTracker.Delete(sessID)
					}
				}
			}
			activePiSessions = newPiMap
			mu.Unlock()
		}

		// 3. Scan Codex
		codexSessions, err := codexScanner.Scan(ctx)
		newCodexMap := make(map[string]scanner.CodexSessionDescriptor)
		if err != nil {
			logging.Warn("scan_failed", "harness", types.HarnessCodex, "err", err)
		} else {
			for _, s := range codexSessions {
				newCodexMap[s.SessionID] = s

				mu.Lock()
				isNew := !activeCodexSessions[s.SessionID].IsActive
				if s.IsActive && isNew {
					activeCodexSessions[s.SessionID] = s
				}
				mu.Unlock()

				if s.IsActive && sessionTracker != nil {
					sessionTracker.Set(s.SessionID, scanner.TrackedSession{
						SessionID:      s.SessionID,
						Harness:        string(types.HarnessCodex),
						PID:            s.PID,
						CWD:            s.CWD,
						HarnessVersion: s.HarnessVersion,
						StartedAt:      s.StartedAt,
						IsActive:       true,
					})
				}

				if s.IsActive && isNew {
					proj, ws, _ := gitResolver.Resolve(ctx, s.CWD, "")
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessCodex, s.SessionID)
					now := time.Now().UnixMilli()
					if s.StartedAt > 0 {
						now = s.StartedAt
					}
					var subID *string
					if s.IsSubagent && s.ParentThreadID != "" {
						subID = &s.SessionID
					}
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
						Version:        "1.0",
						EnvID:          env.ID,
						Environment:    *env,
						AgentID:        agentID,
						SessionID:      s.SessionID,
						SubagentID:     subID,
						PID:            s.PID,
						Harness:        types.HarnessCodex,
						HarnessVersion: s.HarnessVersion,
						Project:        proj,
						Workspace:      ws,
						Timestamp:      now,
						EventType:      "run_started",
						Data:           types.TelemetryFrameData{},
					})
				}
				if s.IsActive && s.TranscriptPath != "" {
					var subID *string
					if s.IsSubagent && s.ParentThreadID != "" {
						subID = &s.SessionID
					}
					attachTailer(s.TranscriptPath, types.HarnessCodex, s.HarnessVersion, s.SessionID, s.CWD, s.PID, subID)
				}
			}

			// Detect exited Codex sessions
			mu.Lock()
			for sessID, oldDesc := range activeCodexSessions {
				newDesc, exists := newCodexMap[sessID]
				if oldDesc.IsActive && (!exists || !newDesc.IsActive) {
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessCodex, sessID)
					proj, ws, _ := gitResolver.Resolve(ctx, oldDesc.CWD, "")
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
						Version:        "1.0",
						EnvID:          env.ID,
						Environment:    *env,
						AgentID:        agentID,
						SessionID:      sessID,
						PID:            oldDesc.PID,
						Harness:        types.HarnessCodex,
						HarnessVersion: oldDesc.HarnessVersion,
						Project:        proj,
						Workspace:      ws,
						Timestamp:      time.Now().UnixMilli(),
						EventType:      "run_exited",
						Data:           types.TelemetryFrameData{},
					})
					cleanupSessionTailsLocked(sessID)
					if sessionTracker != nil {
						sessionTracker.Delete(sessID)
					}
				}
			}
			activeCodexSessions = newCodexMap
			mu.Unlock()
		}

		// 4. Scan OpenCode
		opencodeSessions, err := opencodeScanner.Scan(ctx)
		newOpencodeMap := make(map[string]scanner.OpencodeSessionDescriptor)
		if err != nil {
			logging.Warn("scan_failed", "harness", types.HarnessOpencode, "err", err)
		} else {
			for _, s := range opencodeSessions {
				newOpencodeMap[s.SessionID] = s

				mu.Lock()
				isNew := !activeOpencodeSessions[s.SessionID].IsActive
				if s.IsActive && isNew {
					activeOpencodeSessions[s.SessionID] = s
				}
				mu.Unlock()

				if s.IsActive && sessionTracker != nil {
					sessionTracker.Set(s.SessionID, scanner.TrackedSession{
						SessionID:      s.SessionID,
						Harness:        string(types.HarnessOpencode),
						PID:            s.PID,
						CWD:            s.CWD,
						HarnessVersion: s.HarnessVersion,
						StartedAt:      s.StartedAt,
						IsActive:       true,
					})
				}

				if s.IsActive && isNew {
					proj, ws, _ := gitResolver.Resolve(ctx, s.CWD, "")
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessOpencode, s.SessionID)
					now := time.Now().UnixMilli()
					if s.StartedAt > 0 {
						now = s.StartedAt
					}
					var subID *string
					if s.IsSubagent && s.ParentID != "" {
						subID = &s.SessionID
					}
					var titlePtr *string
					if s.Title != "" {
						titlePtr = &s.Title
					}
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
						Version:        "1.0",
						EnvID:          env.ID,
						Environment:    *env,
						AgentID:        agentID,
						SessionID:      s.SessionID,
						SubagentID:     subID,
						PID:            s.PID,
						Harness:        types.HarnessOpencode,
						HarnessVersion: s.HarnessVersion,
						Project:        proj,
						Workspace:      ws,
						Timestamp:      now,
						EventType:      "run_started",
						Data: types.TelemetryFrameData{
							Title: titlePtr,
						},
					})
				}
			}

			// Detect exited OpenCode sessions
			mu.Lock()
			for sessID, oldDesc := range activeOpencodeSessions {
				newDesc, exists := newOpencodeMap[sessID]
				if oldDesc.IsActive && (!exists || !newDesc.IsActive) {
					agentID := fmt.Sprintf("%s:%s:%s", env.ID, types.HarnessOpencode, sessID)
					proj, ws, _ := gitResolver.Resolve(ctx, oldDesc.CWD, "")
					_ = ship.EnqueueContext(ctx, types.ITelemetryFrame{
						Version:        "1.0",
						EnvID:          env.ID,
						Environment:    *env,
						AgentID:        agentID,
						SessionID:      sessID,
						PID:            oldDesc.PID,
						Harness:        types.HarnessOpencode,
						HarnessVersion: oldDesc.HarnessVersion,
						Project:        proj,
						Workspace:      ws,
						Timestamp:      time.Now().UnixMilli(),
						EventType:      "run_exited",
						Data:           types.TelemetryFrameData{},
					})
					cleanupSessionTailsLocked(sessID)
					if sessionTracker != nil {
						sessionTracker.Delete(sessID)
					}
				}
			}
			activeOpencodeSessions = newOpencodeMap
			mu.Unlock()
		}

		if sessionTracker != nil {
			_ = sessionTracker.Save()
		}
	}

	// Perform initial scan
	scanAll()

	scanInterval := opts.scanInterval
	if scanInterval <= 0 {
		scanInterval = 2 * time.Second
	}
	scanTicker := time.NewTicker(scanInterval)
	defer scanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			logging.Info("observer_shutting_down")
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
