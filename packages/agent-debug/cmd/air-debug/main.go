package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
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
	envFlag     string
	harnessFlag string
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
				ExportDB:    rFlags.exportDB,
				Summarize:   rFlags.summarize,
				AgentMode:   output.IsAgentMode(),
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

			if rFlags.jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}

			output.RenderReplaySummary(cmd.OutOrStdout(), res.Summary(), output.IsAgentMode())
			if res.Verification != nil && !res.Verification.Passed {
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
				StopOnError: false,
				AgentMode:   output.IsAgentMode(),
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
				lifecycleDetails := "Lifecycle=EXITED"
				if res.Agent == nil {
					lifecycleDetails = "nil agent state"
					lifecyclePassed = false
					report.Passed = false
				} else if res.Agent.Activity.Lifecycle != types.LifecycleExited {
					lifecycleDetails = fmt.Sprintf("Lifecycle=%s (did not finalize cleanly)", res.Agent.Activity.Lifecycle)
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
			}

			if vFlags.jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if encErr := enc.Encode(report); encErr != nil {
					return encErr
				}
			} else {
				output.RenderValidationReport(cmd.OutOrStdout(), report, output.IsAgentMode())
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

	sessionCmd.AddCommand(listCmd)
	sessionCmd.AddCommand(replayCmd)
	sessionCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(sessionCmd)

	cobrahelptree.Setup(rootCmd)
	return rootCmd
}

func run(args []string) error {
	rootCmd := newRootCmd()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
