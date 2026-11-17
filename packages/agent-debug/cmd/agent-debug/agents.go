package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/environment"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/store"
	"github.com/alexgorbatchev/agent-watcher/scanner"
	"github.com/spf13/cobra"
)

func newAgentCmd() *cobra.Command {
	group := &cobra.Command{
		Use:   "agent",
		Short: "Select running agents for shell integration",
	}
	var all bool
	var dbPath string
	serverURL := agentServerURL()
	list := &cobra.Command{
		Use:   "list",
		Short: "Print live local agents as headerless TSV for fzf",
		Long: "Discover running local harness sessions directly, including idle and waiting agents.\n" +
			"The default works without a dashboard, daemon, or agent-status database.\n" +
			"Each row contains: PID, harness, session ID, hostname, working directory, title.\n" +
			"Fields are tab-separated; control characters in text are sanitized.\n" +
			"Local PID and start time are verified. --db selects monitoring data instead.\n" +
			"--all queries agent-server over HTTP and includes remote running agents with fresh\n" +
			"observer heartbeats; remote PIDs cannot be verified locally.\n" +
			"Set AGENT_SERVER_HTTP_URL or --server-url and AGENT_STATUS_TOKEN for authentication.\n" +
			"--db selects offline local monitoring data and cannot be combined with --all.\n" +
			"Output has no header or colors in either AGENT mode. An empty list writes nothing.",
		Example:       "  agent-debug agent list | fzf\n  agent-debug agent list --all --server-url https://agents.example.com | fzf",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all && cmd.Flags().Changed("server-url") {
				return fmt.Errorf("--server-url requires --all")
			}
			if !all && !cmd.Flags().Changed("db") {
				agents, err := discoverLocalAgents(cmd.Context())
				if err != nil {
					return err
				}
				for _, a := range agents {
					if err := writeAgentRow(cmd.OutOrStdout(), a); err != nil {
						return err
					}
				}
				return nil
			}
			localID, err := environment.ID()
			if err != nil {
				return fmt.Errorf("resolving local environment: %w", err)
			}
			var agents []types.IAgentState
			if all {
				agents, err = fetchServerAgents(cmd.Context(), serverURL, os.Getenv("AGENT_STATUS_TOKEN"))
			} else {
				path, err := db.ResolveDBPath(dbPath)
				if err != nil {
					return err
				}
				s, err := store.OpenReadOnly(path)
				if err != nil {
					return err
				}
				defer func() { _ = s.Close() }() // Best-effort read-only connection cleanup.
				agents, err = s.GetAgents(cmd.Context(), store.AgentFilter{
					Lifecycle: types.LifecycleRunning, Environment: localID,
				})
				if err != nil {
					return err
				}
			}
			if err != nil {
				return err
			}
			for _, a := range agents {
				run := a.Activity.CurrentRun
				if a.Activity.Lifecycle != types.LifecycleRunning || run == nil || run.PID <= 0 || run.EndedAt != nil {
					continue
				}
				if a.Environment.ID == localID {
					if !scanner.VerifyProcessMatch(run.PID, run.StartedAt) {
						continue
					}
				} else if a.Environment.IsStale {
					continue
				}
				if err := writeAgentRow(cmd.OutOrStdout(), a); err != nil {
					return err
				}
			}
			return nil
		},
	}
	list.Flags().BoolVar(&all, "all", false, "Include running agents on all monitored machines with fresh heartbeats")
	list.Flags().StringVar(&dbPath, "db", "auto", "Read a local monitoring database offline instead of scanning harnesses (incompatible with --all)")
	list.Flags().StringVar(&serverURL, "server-url", serverURL, "Agent-server HTTP base URL (or AGENT_SERVER_HTTP_URL); requires --all")
	list.MarkFlagsMutuallyExclusive("all", "db")
	group.AddCommand(list)
	group.AddCommand(newAgentUsageCmd())
	return group
}

func writeAgentRow(w io.Writer, a types.IAgentState) error {
	clean := func(s string) string {
		return strings.NewReplacer("\t", " ", "\n", " ").Replace(output.SanitizeTranscriptText(s))
	}
	_, err := fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", a.Activity.CurrentRun.PID,
		clean(string(a.Harness)), clean(a.SessionID), clean(a.Environment.Hostname),
		clean(a.Workspace.CWD), clean(a.Activity.Title))
	if err != nil {
		return fmt.Errorf("writing agent list: %w", err)
	}
	return nil
}
