package main

import (
	"fmt"
	"os"

	"github.com/alexgorbatchev/agent-status/packages/print-session-cli/internal/session"
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"
)

var (
	version  = "dev"
	exitFunc = os.Exit
)

type printFlags struct {
	jsonOut     bool
	filePath    string
	summaryOnly bool
	handoff     bool
	lastTurn    bool
	errorsOnly  bool
	filesOnly   bool
	toolsOnly   bool
	promptsOnly bool
	tail        int
	limit       int
	all         bool
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "print-session",
		Short: "Print and inspect AI coding agent session logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	// Subject: claude
	claudeCmd := &cobra.Command{
		Use:   "claude",
		Short: "Inspect and print Claude Code session transcripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				// Shorthand invocation: print-session claude <session-id>
				flags := getPrintFlags(cmd)
				return runPrintClaude(cmd, args[0], flags)
			}
			return cmd.Help()
		},
	}

	// Verb: claude print <session-id>
	var pFlags printFlags
	printCmd := &cobra.Command{
		Use:   "print <session-id-or-path>",
		Short: "Parse and print Claude Code session events by ID or file path",
		Args: func(cmd *cobra.Command, args []string) error {
			if pFlags.filePath != "" {
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := pFlags.filePath
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			return runPrintClaude(cmd, target, pFlags)
		},
	}

	setupFlags(printCmd, &pFlags)
	setupFlags(claudeCmd, &pFlags)

	// Verb: claude summary <session-id>
	var sFlags printFlags
	summaryCmd := &cobra.Command{
		Use:   "summary <session-id-or-path>",
		Short: "Print a high-level token-efficient summary of a Claude Code session",
		Args: func(cmd *cobra.Command, args []string) error {
			if sFlags.filePath != "" {
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := sFlags.filePath
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			sFlags.summaryOnly = true
			return runPrintClaude(cmd, target, sFlags)
		},
	}
	summaryCmd.Flags().BoolVar(&sFlags.jsonOut, "json", false, "Output summary as formatted JSON")
	summaryCmd.Flags().StringVar(&sFlags.filePath, "path", "", "Direct path to Claude session .jsonl file")

	// Verb: claude handoff <session-id> (aliases: continue, resume)
	var hFlags printFlags
	handoffCmd := &cobra.Command{
		Use:     "handoff <session-id-or-path>",
		Aliases: []string{"continue", "resume"},
		Short:   "Generate structured continuation context for resuming work started in another session",
		Args: func(cmd *cobra.Command, args []string) error {
			if hFlags.filePath != "" {
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := hFlags.filePath
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			hFlags.handoff = true
			return runPrintClaude(cmd, target, hFlags)
		},
	}
	handoffCmd.Flags().BoolVar(&hFlags.jsonOut, "json", false, "Output handoff context as formatted JSON")
	handoffCmd.Flags().StringVar(&hFlags.filePath, "path", "", "Direct path to Claude session .jsonl file")

	claudeCmd.AddCommand(printCmd)
	claudeCmd.AddCommand(summaryCmd)
	claudeCmd.AddCommand(handoffCmd)
	rootCmd.AddCommand(claudeCmd)

	cobrahelptree.Setup(rootCmd)
	return rootCmd
}

func setupFlags(cmd *cobra.Command, flags *printFlags) {
	cmd.Flags().BoolVar(&flags.jsonOut, "json", false, "Output parsed events as formatted JSON")
	cmd.Flags().StringVar(&flags.filePath, "path", "", "Direct path to Claude session .jsonl file")
	cmd.Flags().BoolVar(&flags.summaryOnly, "summary", false, "Output high-level session summary only")
	cmd.Flags().BoolVar(&flags.handoff, "handoff", false, "Output continuation/handoff context for resuming work")
	cmd.Flags().BoolVar(&flags.lastTurn, "last-turn", false, "Output only the final conversational turn")
	cmd.Flags().BoolVar(&flags.errorsOnly, "errors", false, "Filter to errors and failed tool executions only")
	cmd.Flags().BoolVar(&flags.filesOnly, "files", false, "Filter to file modifications only")
	cmd.Flags().BoolVar(&flags.toolsOnly, "tools", false, "Filter to tool invocations and results only")
	cmd.Flags().BoolVar(&flags.promptsOnly, "prompts", false, "Filter to user prompts and turn boundaries only")
	cmd.Flags().IntVar(&flags.tail, "tail", 0, "Show only the last N events")
	cmd.Flags().IntVar(&flags.limit, "limit", 0, "Show only the first N events")
	cmd.Flags().BoolVar(&flags.all, "all", false, "Output full transcript without default agent-mode event limit")
}

func getPrintFlags(cmd *cobra.Command) printFlags {
	jsonVal, _ := cmd.Flags().GetBool("json")
	pathVal, _ := cmd.Flags().GetString("path")
	summaryVal, _ := cmd.Flags().GetBool("summary")
	handoffVal, _ := cmd.Flags().GetBool("handoff")
	lastTurnVal, _ := cmd.Flags().GetBool("last-turn")
	errorsVal, _ := cmd.Flags().GetBool("errors")
	filesVal, _ := cmd.Flags().GetBool("files")
	toolsVal, _ := cmd.Flags().GetBool("tools")
	promptsVal, _ := cmd.Flags().GetBool("prompts")
	tailVal, _ := cmd.Flags().GetInt("tail")
	limitVal, _ := cmd.Flags().GetInt("limit")
	allVal, _ := cmd.Flags().GetBool("all")

	return printFlags{
		jsonOut:     jsonVal,
		filePath:    pathVal,
		summaryOnly: summaryVal,
		handoff:     handoffVal,
		lastTurn:    lastTurnVal,
		errorsOnly:  errorsVal,
		filesOnly:   filesVal,
		toolsOnly:   toolsVal,
		promptsOnly: promptsVal,
		tail:        tailVal,
		limit:       limitVal,
		all:         allVal,
	}
}

func runPrintClaude(cmd *cobra.Command, target string, flags printFlags) error {
	resolvedPath, err := session.FindClaudeSession(target)
	if err != nil {
		return err
	}

	isAgent := os.Getenv("AGENT") == "1" || os.Getenv("AGENT") == "true" || os.Getenv("AGENT") == "yes"

	opts := session.PrintOptions{
		JSON:        flags.jsonOut,
		AgentMode:   isAgent,
		SummaryOnly: flags.summaryOnly,
		Handoff:     flags.handoff,
		LastTurn:    flags.lastTurn,
		ErrorsOnly:  flags.errorsOnly,
		FilesOnly:   flags.filesOnly,
		ToolsOnly:   flags.toolsOnly,
		PromptsOnly: flags.promptsOnly,
		Tail:        flags.tail,
		Limit:       flags.limit,
		All:         flags.all,
	}

	return session.PrintClaudeSession(cmd.OutOrStdout(), resolvedPath, opts)
}

func run(args []string) error {
	rootCmd := newRootCmd()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}
}
