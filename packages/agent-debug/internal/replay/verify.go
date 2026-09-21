package replay

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
	serverstore "github.com/alexgorbatchev/agent-status/packages/agent-server/store"
)

// VerifyOptions configures read-side verification checks.
type VerifyOptions struct {
	StrictLifecycle bool
	HasExplicitExit bool
}

// VerificationReport captures all read-side API assertions.
type VerificationReport struct {
	AgentID   string                   `json:"agentId"`
	Passed    bool                     `json:"passed"`
	Agent     *types.IAgentState       `json:"agent,omitempty"`
	LogCount  int                      `json:"logCount"`
	Checks    []output.ValidationCheck `json:"checks"`
	LastError string                   `json:"lastError,omitempty"`
}

// VerifyAgentState executes agent-server queries against the populated SQLite database.
func VerifyAgentState(ctx context.Context, db *sql.DB, agentID string, opts ...VerifyOptions) (*VerificationReport, error) {
	report := &VerificationReport{
		AgentID: agentID,
		Passed:  true,
	}

	addCheck := func(name string, passed bool, details string) {
		report.Checks = append(report.Checks, output.ValidationCheck{
			Name:    name,
			Passed:  passed,
			Details: details,
		})
		if !passed {
			report.Passed = false
			if report.LastError == "" {
				report.LastError = fmt.Sprintf("%s: %s", name, details)
			}
		}
	}

	sStore := serverstore.New(db)

	// 1. Verify GetAgentByID
	agent, err := sStore.GetAgentByID(ctx, agentID)
	if err != nil {
		addCheck("GetAgentByID deserialization", false, fmt.Sprintf("query error: %v", err))
		return report, nil
	}
	report.Agent = agent
	addCheck("GetAgentByID deserialization", true, fmt.Sprintf("loaded agent %s", agent.ID))

	// 2. Verify Environment & Workspace Scan
	if agent.Environment.ID != "" && agent.Environment.Hostname != "" {
		addCheck("Environment scan", true, fmt.Sprintf("env %s (%s)", agent.Environment.ID, agent.Environment.Platform))
	} else {
		addCheck("Environment scan", false, "empty environment metadata")
	}

	if agent.Project.Name != "" && agent.Workspace.CWD != "" {
		addCheck("Project & Workspace scan", true, fmt.Sprintf("project %s on %s", agent.Project.Name, agent.Workspace.Branch))
	} else {
		addCheck("Project & Workspace scan", false, "empty project or workspace cwd")
	}

	// 3. Verify Lifecycle and Activity invariants
	if agent.Activity.Lifecycle == types.LifecycleRunning || agent.Activity.Lifecycle == types.LifecycleExited {
		addCheck("Lifecycle invariant", true, string(agent.Activity.Lifecycle))
	} else {
		addCheck("Lifecycle invariant", false, fmt.Sprintf("invalid lifecycle: %q", agent.Activity.Lifecycle))
	}

	if len(opts) > 0 && opts[0].StrictLifecycle {
		if opts[0].HasExplicitExit {
			addCheck("Transcript lifecycle termination", true, "terminated with explicit harness exit event")
		} else {
			addCheck("Transcript lifecycle termination", false, fmt.Sprintf("transcript is open-ended (lifecycle=%s, no explicit harness exit event)", agent.Activity.Lifecycle))
		}
	}

	switch agent.Activity.Activity {
	case types.ActivityIdle, types.ActivityBusy, types.ActivityWaitingQuestion, types.ActivityError:
		addCheck("Activity invariant", true, string(agent.Activity.Activity))
	default:
		addCheck("Activity invariant", false, fmt.Sprintf("invalid activity: %q", agent.Activity.Activity))
	}

	// 4. Verify Usage and Cost non-negative
	u := agent.Activity.Usage
	if u.Input >= 0 && u.Output >= 0 && u.Total >= 0 {
		addCheck("Token usage invariants", true, fmt.Sprintf("in:%d out:%d total:%d", u.Input, u.Output, u.Total))
	} else {
		addCheck("Token usage invariants", false, fmt.Sprintf("negative token counts: %+v", u))
	}

	if agent.Activity.Cost != nil {
		if agent.Activity.Cost.Total >= 0 {
			addCheck("Pricing calculation", true, fmt.Sprintf("$%.4f %s", agent.Activity.Cost.Total, agent.Activity.Cost.Currency))
		} else {
			addCheck("Pricing calculation", false, fmt.Sprintf("negative cost: %f", agent.Activity.Cost.Total))
		}
	} else {
		addCheck("Pricing calculation", true, "cost is nil (unpriced model or zero usage)")
	}

	// 5. Verify GetAgentLogs deserialization
	logs, err := sStore.GetAgentLogs(ctx, agentID, 1000, 0, "")
	if err != nil {
		addCheck("GetAgentLogs deserialization", false, fmt.Sprintf("query error: %v", err))
		return report, nil
	}

	report.LogCount = len(logs)
	if len(logs) == 0 {
		addCheck("GetAgentLogs deserialization", false, "no telemetry log records found")
		return report, nil
	}

	// Verify all frames deserialized cleanly
	invalidFrames := 0
	for _, f := range logs {
		if f.Timestamp <= 0 || f.EventType == "" {
			invalidFrames++
		}
	}
	if invalidFrames > 0 {
		addCheck("GetAgentLogs framing", false, fmt.Sprintf("%d frames have invalid timestamp or empty event_type", invalidFrames))
	} else {
		addCheck("GetAgentLogs framing", true, fmt.Sprintf("%d log frames deserialized cleanly", len(logs)))
	}

	return report, nil
}
