package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/replay"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"
)

var version = "dev"

type replayFlags struct {
	sessionID   string
	cwd         string
	diff        bool
	untilLine   int
	untilEvent  string
	stopOnError bool
	jsonOut     bool
	exportDB    string
	summarize   bool
	envFlag         string
	harnessFlag     string
	strictLifecycle bool
	dbPath          string
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "air-debug",
		Short: "Agent Status Offline Replay & Ingestion Debugger",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "Inspect, replay, and validate agent session transcripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// 1. session list
	listCmd := &cobra.Command{
		Use:   "list <file-or-database>",
		Short: "List sessions inside an OpenCode SQLite database or session file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			resolved, err := replay.ResolveSessionTarget(ctx, args[0], types.HarnessOpencode)
			if err != nil {
				return err
			}
			filePath := resolved.FilePath

			db, err := parser.OpenOpencodeDB(filePath)
			if err != nil {
				return fmt.Errorf("opening opencode database %s: %w", filePath, err)
			}
			defer func() { _ = db.Close() }()

			sessions, err := db.GetSessions(ctx)
			if err != nil {
				return fmt.Errorf("reading sessions: %w", err)
			}

			var summaries []output.SessionSummary
			for _, s := range sessions {
				totTokens := s.Tokens.Total
				if totTokens == 0 {
					totTokens = s.Tokens.Input + s.Tokens.Output + s.Tokens.Reasoning + s.Tokens.Cache.Read + s.Tokens.Cache.Write
				}
				summaries = append(summaries, output.SessionSummary{
					ID:        s.ID,
					Title:     s.Title,
					Directory: s.Directory,
					CreatedAt: time.UnixMilli(s.TimeCreated),
					CostUSD:   s.Cost,
					Tokens:    totTokens,
				})
			}

			output.RenderSessionList(cmd.OutOrStdout(), summaries, output.IsAgentMode())
			return nil
		},
	}

	// 2. session replay
	rFlags := replayFlags{}
	replayCmd := &cobra.Command{
		Use:   "replay <file-or-session-id>",
		Short: "Replay session transcript through the ingestion pipeline offline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			resolved, err := replay.ResolveSessionTarget(ctx, args[0], types.HarnessType(rFlags.harnessFlag))
			if err != nil {
				return err
			}

			sessID := rFlags.sessionID
			if sessID == "" {
				sessID = resolved.SessionID
			}

			opts := replay.ReplayOptions{
				FilePath:    resolved.FilePath,
				Harness:     resolved.Harness,
				SessionID:   sessID,
				CWD:         rFlags.cwd,
				EnvLabel:    rFlags.envFlag,
				Diff:        rFlags.diff,
				UntilLine:   rFlags.untilLine,
				UntilEvent:  rFlags.untilEvent,
				StopOnError: rFlags.stopOnError,
				ExportDB:        rFlags.exportDB,
				Summarize:       rFlags.summarize,
				AgentMode:       output.IsAgentMode(),
				StrictLifecycle: rFlags.strictLifecycle,
			}

			if rFlags.diff && !rFlags.jsonOut {
				opts.OnDiff = func(diff output.StateDiff) {
					output.RenderDiff(cmd.OutOrStdout(), diff, output.IsAgentMode())
				}
			}

			res, err := replay.RunReplay(ctx, opts)
			if err != nil {
				return err
			}

			if rFlags.dbPath != "" {
				resolvedDB, err := db.ResolveDBPath(rFlags.dbPath)
				if err != nil {
					return fmt.Errorf("resolving database for comparison: %w", err)
				}
				compareSessID := rFlags.sessionID
				if compareSessID == "" && res != nil {
					compareSessID = res.SessionID
				}
				record, err := db.InspectSession(ctx, resolvedDB, compareSessID)
				if err != nil {
					return fmt.Errorf("inspecting database session %s: %w", compareSessID, err)
				}
				res.Comparison = replay.CompareSessionWithDB(res, record)
			}

			if rFlags.jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}

			output.RenderReplaySummary(cmd.OutOrStdout(), res.Summary(), output.IsAgentMode())
			if res.Comparison != nil {
				output.RenderComparison(cmd.OutOrStdout(), res.Comparison, output.IsAgentMode())
			}
			if res.Verification != nil && !res.Verification.Passed && !strings.Contains(res.Verification.LastError, "transcript is open-ended") {
				return fmt.Errorf("read-side verification failed: %s", res.Verification.LastError)
			}
			return nil
		},
	}

	flags := replayCmd.Flags()
	flags.StringVar(&rFlags.sessionID, "session-id", "", "OpenCode session ID to replay")
	flags.StringVar(&rFlags.cwd, "cwd", "", "Override transcript working directory for local Git resolution")
	flags.BoolVar(&rFlags.diff, "diff", false, "Display turn-by-turn state mutations during replay")
	flags.IntVar(&rFlags.untilLine, "until-line", 0, "Halt replay after processing source line N")
	flags.StringVar(&rFlags.untilEvent, "until-event", "", "Halt replay after encountering event type")
	flags.BoolVar(&rFlags.stopOnError, "stop-on-error", false, "Halt immediately on first parse or state error")
	flags.BoolVar(&rFlags.jsonOut, "json", false, "Output final state and verification results as JSON")
	flags.StringVar(&rFlags.exportDB, "export-db", "", "Path to save populated SQLite database on disk")
	flags.BoolVar(&rFlags.summarize, "summarize", false, "Enable live LLM summarizer provider verification")
	flags.StringVar(&rFlags.envFlag, "env", "debug", "Environment label override")
	flags.StringVar(&rFlags.harnessFlag, "harness", "", "Explicit harness override (claude-code, pi, codex, opencode)")
	flags.BoolVar(&rFlags.strictLifecycle, "strict-lifecycle", false, "Disable automatic synthesis of run_started and run_exited boundary frames")
	flags.BoolVar(&rFlags.strictLifecycle, "no-synthetic-lifecycle", false, "Disable automatic synthesis of run_started and run_exited boundary frames (alias)")
	flags.StringVar(&rFlags.dbPath, "db", "", "Path to agent-status.db for discrepancy comparison (or 'auto' for auto-detection)")

	// 3. session validate
	vFlags := replayFlags{}
	validateCmd := &cobra.Command{
		Use:   "validate <file-or-session-id>",
		Short: "Validate session transcript against parser, classifier, pricing, and query assertions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			resolved, err := replay.ResolveSessionTarget(ctx, args[0], types.HarnessType(vFlags.harnessFlag))
			if err != nil {
				return err
			}

			sessID := vFlags.sessionID
			if sessID == "" {
				sessID = resolved.SessionID
			}

			opts := replay.ReplayOptions{
				FilePath:    resolved.FilePath,
				Harness:     resolved.Harness,
				SessionID:   sessID,
				CWD:         vFlags.cwd,
				EnvLabel:    vFlags.envFlag,
				StopOnError:     false,
				AgentMode:       output.IsAgentMode(),
				StrictLifecycle: vFlags.strictLifecycle,
			}

			res, err := replay.RunReplay(ctx, opts)
			report := output.ValidationReport{
				Target: resolved.FilePath,
				Passed: true,
			}

			if err != nil {
				report.Passed = false
				report.Checks = append(report.Checks, output.ValidationCheck{
					Name:    "Replay Pipeline Execution",
					Passed:  false,
					Details: err.Error(),
				})
			} else {
				report.Harness = res.Harness
				// Check 1: Harness detection
				report.Checks = append(report.Checks, output.ValidationCheck{
					Name:    "Harness Detection",
					Passed:  true,
					Details: string(res.Harness),
				})

				// Check 2: Parse & framing
				parsePassed := res.FramesProcessed > 0 && len(res.Errors) == 0
				parseDetails := fmt.Sprintf("%d frames processed", res.FramesProcessed)
				if len(res.Errors) > 0 {
					parseDetails += fmt.Sprintf(" (%d errors)", len(res.Errors))
					report.Passed = false
				}
				report.Checks = append(report.Checks, output.ValidationCheck{
					Name:    "Telemetry Event Framing",
					Passed:  parsePassed,
					Details: parseDetails,
				})

				// Check 3: State machine finalization
				lifecyclePassed := res.Agent != nil && res.Agent.Activity.Lifecycle == types.LifecycleExited
				var lifecycleDetails string
				if res.Agent == nil {
					lifecycleDetails = "nil agent state"
					lifecyclePassed = false
					report.Passed = false
				} else if res.HasSyntheticEnd {
					lifecycleDetails = "Lifecycle=EXITED (synthetic)"
				} else if res.HasNativeEnd {
					lifecycleDetails = "Lifecycle=EXITED (native)"
				} else {
					if vFlags.strictLifecycle {
						lifecycleDetails = fmt.Sprintf("Lifecycle=%s (open-ended transcript, no explicit harness exit event)", res.Agent.Activity.Lifecycle)
					} else {
						lifecycleDetails = fmt.Sprintf("Lifecycle=%s (did not finalize cleanly)", res.Agent.Activity.Lifecycle)
					}
					lifecyclePassed = false
					report.Passed = false
				}
				report.Checks = append(report.Checks, output.ValidationCheck{
					Name:    "Lifecycle State Invariant",
					Passed:  lifecyclePassed,
					Details: lifecycleDetails,
				})

				// Check 4: Pricing and Usage
				cost := 0.0
				if res.Agent != nil && res.Agent.Activity.Cost != nil {
					cost = res.Agent.Activity.Cost.Total
				}
				totTokens := int64(0)
				if res.Agent != nil {
					totTokens = res.Agent.Activity.Usage.Total
				}
				pricingPassed := cost >= 0 && totTokens >= 0
				report.Checks = append(report.Checks, output.ValidationCheck{
					Name:    "Usage & Pricing Invariants",
					Passed:  pricingPassed,
					Details: fmt.Sprintf("%d total tokens, $%.4f USD", totTokens, cost),
				})
				if !pricingPassed {
					report.Passed = false
				}

				// Check 5: Server-side Query Verification
				verPassed := res.Verification != nil && res.Verification.Passed
				verDetails := "GetAgentByID & GetAgentLogs scanned cleanly"
				if !verPassed {
					if res.Verification != nil && res.Verification.LastError != "" {
						verDetails = res.Verification.LastError
					} else {
						verDetails = "query verification failed"
					}
					report.Passed = false
				}
				report.Checks = append(report.Checks, output.ValidationCheck{
					Name:    "Server Read Verification",
					Passed:  verPassed,
					Details: verDetails,
				})

				// Check 6: Live Database Discrepancy Check
				if vFlags.dbPath != "" {
					resolvedDB, err := db.ResolveDBPath(vFlags.dbPath)
					if err != nil {
						report.Passed = false
						report.Checks = append(report.Checks, output.ValidationCheck{
							Name:    "Database Discrepancy Check",
							Passed:  false,
							Details: fmt.Sprintf("resolving database failed: %v", err),
						})
					} else {
						compareSessID := vFlags.sessionID
						if compareSessID == "" && res != nil {
							compareSessID = res.SessionID
						}
						record, err := db.InspectSession(ctx, resolvedDB, compareSessID)
						if err != nil {
							report.Passed = false
							report.Checks = append(report.Checks, output.ValidationCheck{
								Name:    "Database Discrepancy Check",
								Passed:  false,
								Details: fmt.Sprintf("session not found in database: %v", err),
							})
						} else {
							comp := replay.CompareSessionWithDB(res, record)
							res.Comparison = comp
							driftCount := 0
							for _, f := range comp.Fields {
								if f.IsDrift {
									driftCount++
								}
							}
							details := "persisted database state matches replayed transcript"
							if comp.HasDrift {
								details = fmt.Sprintf("drift detected across %d field(s)", driftCount)
								report.Passed = false
							}
							report.Checks = append(report.Checks, output.ValidationCheck{
								Name:    "Database Discrepancy Check",
								Passed:  !comp.HasDrift,
								Details: details,
							})
						}
					}
				}
			}

			if vFlags.jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if encErr := enc.Encode(report); encErr != nil {
					return encErr
				}
			} else {
				output.RenderValidationReport(cmd.OutOrStdout(), report, output.IsAgentMode())
				if res != nil && res.Comparison != nil {
					output.RenderComparison(cmd.OutOrStdout(), res.Comparison, output.IsAgentMode())
				}
			}

			if !report.Passed {
				return fmt.Errorf("session validation failed for %s", resolved.FilePath)
			}
			return nil
		},
	}

	vflags := validateCmd.Flags()
	vflags.StringVar(&vFlags.sessionID, "session-id", "", "OpenCode session ID to validate")
	vflags.StringVar(&vFlags.cwd, "cwd", "", "Override transcript working directory for local Git resolution")
	vflags.BoolVar(&vFlags.jsonOut, "json", false, "Output validation report as JSON")
	vflags.StringVar(&vFlags.envFlag, "env", "debug", "Environment label override")
	vflags.StringVar(&vFlags.harnessFlag, "harness", "", "Explicit harness override (claude-code, pi, codex, opencode)")
	vflags.BoolVar(&vFlags.strictLifecycle, "strict-lifecycle", false, "Disable automatic synthesis of run_started and run_exited boundary frames")
	vflags.BoolVar(&vFlags.strictLifecycle, "no-synthetic-lifecycle", false, "Disable automatic synthesis of run_started and run_exited boundary frames (alias)")
	vflags.StringVar(&vFlags.dbPath, "db", "", "Path to agent-status.db for discrepancy comparison (or 'auto' for auto-detection)")

	// 4. session view
	viewFlags := replayFlags{}
	viewCmd := &cobra.Command{
		Use:   "view <file-or-session-id>",
		Short: "Render terminal UI card representation of agent session state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			resolved, err := replay.ResolveSessionTarget(ctx, args[0], types.HarnessType(viewFlags.harnessFlag))
			if err != nil {
				return err
			}

			sessID := viewFlags.sessionID
			if sessID == "" {
				sessID = resolved.SessionID
			}

			opts := replay.ReplayOptions{
				FilePath:    resolved.FilePath,
				Harness:     resolved.Harness,
				SessionID:   sessID,
				CWD:         viewFlags.cwd,
				EnvLabel:    viewFlags.envFlag,
				UntilLine:   viewFlags.untilLine,
				UntilEvent:  viewFlags.untilEvent,
				StopOnError: false,
				AgentMode:   output.IsAgentMode(),
			}

			res, err := replay.RunReplay(ctx, opts)
			if err != nil {
				return err
			}

			if res.Agent == nil {
				return fmt.Errorf("no agent state produced from session")
			}

			if viewFlags.jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res.Agent)
			}

			output.RenderAgentView(cmd.OutOrStdout(), *res.Agent, output.IsAgentMode())
			return nil
		},
	}

	viewCmdFlags := viewCmd.Flags()
	viewCmdFlags.StringVar(&viewFlags.sessionID, "session-id", "", "OpenCode session ID to replay")
	viewCmdFlags.StringVar(&viewFlags.cwd, "cwd", "", "Override transcript working directory for local Git resolution")
	viewCmdFlags.StringVar(&viewFlags.envFlag, "env", "debug", "Environment label override")
	viewCmdFlags.StringVar(&viewFlags.harnessFlag, "harness", "", "Explicit harness override (claude-code, pi, codex, opencode)")
	viewCmdFlags.IntVar(&viewFlags.untilLine, "until-line", 0, "Halt replay after processing source line N")
	viewCmdFlags.StringVar(&viewFlags.untilEvent, "until-event", "", "Halt replay after encountering event type")
	viewCmdFlags.BoolVar(&viewFlags.jsonOut, "json", false, "Output final agent state as JSON")

	// 5. events command
	type eventsFlags struct {
		dbPath           string
		limit            int
		eventType        string
		subagentID       string
		includeSubagents bool
		jsonOut          bool
		desc             bool
		asc              bool
		eventID          int64
	}

	eFlags := eventsFlags{}
	eventsCmd := &cobra.Command{
		Use:   "events [session-id]",
		Short: "Inspect the raw stream of ingested telemetry events from SQLite database",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			var sessionID string
			if len(args) > 0 {
				sessionID = args[0]
			}

			if sessionID == "" && eFlags.eventID == 0 {
				return fmt.Errorf("session ID argument or --id flag is required")
			}

			dbPath, err := db.ResolveDatabasePath(eFlags.dbPath)
			if err != nil {
				return err
			}

			desc := true
			if cmd.Flags().Changed("asc") && eFlags.asc {
				desc = false
			} else if cmd.Flags().Changed("desc") {
				desc = eFlags.desc
			}

			includeSubs := eFlags.includeSubagents
			opts := db.EventQueryOptions{
				Limit:            eFlags.limit,
				EventType:        eFlags.eventType,
				SubagentID:       eFlags.subagentID,
				IncludeSubagents: &includeSubs,
				Desc:             desc,
				EventID:          eFlags.eventID,
			}

			events, err := db.GetTelemetryEvents(ctx, dbPath, sessionID, opts)
			if err != nil {
				return err
			}

			if eFlags.eventID > 0 {
				if len(events) == 0 {
					return fmt.Errorf("telemetry event with ID %d not found", eFlags.eventID)
				}
				if eFlags.jsonOut {
					return output.RenderEventJSON(cmd.OutOrStdout(), events[0])
				}
				return output.RenderEventDetail(cmd.OutOrStdout(), events[0], output.IsAgentMode())
			}

			if eFlags.jsonOut {
				return output.RenderEventsJSON(cmd.OutOrStdout(), events)
			}

			return output.RenderEventsTable(cmd.OutOrStdout(), events, output.IsAgentMode())
		},
	}

	eflags := eventsCmd.Flags()
	eflags.StringVar(&eFlags.dbPath, "db", "auto", "Path to agent-status.db (or auto-detect)")
	eflags.IntVar(&eFlags.limit, "limit", 20, "Maximum number of telemetry events to retrieve")
	eflags.StringVar(&eFlags.eventType, "type", "", "Filter by comma-separated event types (e.g. run_started,run_exited)")
	eflags.StringVar(&eFlags.subagentID, "subagent-id", "", "Filter by specific subagent ID")
	eflags.BoolVar(&eFlags.includeSubagents, "include-subagents", true, "Include events from subagents")
	eflags.BoolVar(&eFlags.jsonOut, "json", false, "Output events as JSON")
	eflags.BoolVar(&eFlags.desc, "desc", true, "Order events by ID descending (newest first)")
	eflags.BoolVar(&eFlags.asc, "asc", false, "Order events by ID ascending (oldest first)")
	eflags.Int64Var(&eFlags.eventID, "id", 0, "Inspect specific telemetry event by ID")

	sessionCmd.AddCommand(listCmd)
	sessionCmd.AddCommand(replayCmd)
	sessionCmd.AddCommand(validateCmd)
	sessionCmd.AddCommand(viewCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(eventsCmd)

	// 4. db
	dbCmd := &cobra.Command{
		Use:   "db",
		Short: "Inspect persisted agent-status SQLite database records",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	var dbInspectPath string
	var dbInspectJSON bool
	inspectCmd := &cobra.Command{
		Use:   "inspect <session-id>",
		Short: "Inspect persisted agent state and runs from SQLite database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			resolvedDB, err := db.ResolveDBPath(dbInspectPath)
			if err != nil {
				return err
			}
			record, err := db.InspectSession(ctx, resolvedDB, args[0])
			if err != nil {
				return err
			}
			if dbInspectJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(record)
			}
			output.RenderSessionInspect(cmd.OutOrStdout(), record, output.IsAgentMode())
			return nil
		},
	}
	inspectFlags := inspectCmd.Flags()
	inspectFlags.StringVar(&dbInspectPath, "db", "auto", "Path to agent-status.db (default: auto-detect)")
	inspectFlags.BoolVar(&dbInspectJSON, "json", false, "Output inspection result as JSON")
	dbCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(dbCmd)

	cobrahelptree.Setup(rootCmd)
	return rootCmd
}

func run(args []string) error {
	rootCmd := newRootCmd()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

var exitFunc = os.Exit

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitFunc(1)
	}
}
