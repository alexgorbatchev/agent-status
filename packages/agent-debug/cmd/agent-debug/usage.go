package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/usage"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
	"github.com/spf13/cobra"
)

func newAgentUsageCmd() *cobra.Command {
	serverURL := agentServerURL()
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "usage <agent-id>",
		Short: "List observed skills, harness commands, and MCP tools from agent-server",
		Long: "Query agent-server for a session's observed skills, harness commands, and MCP tools.\n" +
			"Use the composite agent ID returned by the API, not the harness session ID.\n" +
			"Invocation attempts, skill loads, and slash requests are counted separately.\n" +
			"Coverage explains which activity is visible in the harness transcript. Shell commands are excluded.\n" +
			"Set AGENT_STATUS_TOKEN to the server's bearer token; --json includes the latest supporting event.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var report usage.Report
			if err := fetchServerJSON(cmd.Context(), serverURL, os.Getenv("AGENT_STATUS_TOKEN"),
				[]string{"api", "v1", "agents", url.PathEscape(args[0]), "usage"}, nil, "agent usage", &report); err != nil {
				return err
			}
			if report.AgentID != args[0] {
				return fmt.Errorf("invalid agent usage from agent-server: agent ID does not match")
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			return output.RenderUsage(cmd.OutOrStdout(), report, output.IsAgentMode())
		},
	}
	cmd.Flags().StringVar(&serverURL, "server-url", serverURL, "Agent-server HTTP base URL (or AGENT_SERVER_HTTP_URL)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output usage counts, latest evidence, and tracking coverage as JSON")
	return cmd
}
