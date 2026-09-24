package output

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
	"github.com/olekukonko/tablewriter"
)

type DiscrepancyField struct {
	Name           string `json:"name"`
	ReplayedValue  string `json:"replayedValue"`
	PersistedValue string `json:"persistedValue"`
	Status         string `json:"status"`
	IsDrift        bool   `json:"isDrift"`
}

type ComparisonResult struct {
	DBPath    string             `json:"dbPath"`
	SessionID string             `json:"sessionId"`
	AgentID   string             `json:"agentId"`
	Fields    []DiscrepancyField `json:"fields"`
	Warnings  []string           `json:"warnings"`
	HasDrift  bool               `json:"hasDrift"`
}

// RenderComparison renders the live database comparison table or agent key-values.
func RenderComparison(w io.Writer, comp *ComparisonResult, agentMode bool) {
	if comp == nil {
		return
	}

	if agentMode {
		driftStr := "MATCH"
		if comp.HasDrift {
			driftStr = "DRIFT"
		}
		_, _ = fmt.Fprintf(w, "db_comparison status:%s db_path:%s session_id:%s agent_id:%s\n",
			driftStr, comp.DBPath, comp.SessionID, comp.AgentID)
		for _, f := range comp.Fields {
			_, _ = fmt.Fprintf(w, "  db_field name:%s replayed:%q db:%q status:%q drift:%t\n",
				f.Name, f.ReplayedValue, f.PersistedValue, f.Status, f.IsDrift)
		}
		for _, warn := range comp.Warnings {
			_, _ = fmt.Fprintf(w, "  db_warning %q\n", warn)
		}
		return
	}

	RenderDivider(w, '-')
	_, _ = fmt.Fprintf(w, "Live Database Comparison (%s)\n", filepath.Base(comp.DBPath))
	RenderDivider(w, '-')

	table := tablewriter.NewTable(w, tablewriter.WithHeader([]string{"Field", "Replayed (Transcript)", "Persisted (Database)", "Status"}))
	for _, f := range comp.Fields {
		_ = table.Append([]string{
			f.Name,
			f.ReplayedValue,
			f.PersistedValue,
			f.Status,
		})
	}
	_ = table.Render()

	if len(comp.Warnings) > 0 {
		RenderDivider(w, '-')
		for _, warn := range comp.Warnings {
			_, _ = fmt.Fprintf(w, "%s %s\n", GlyphWarn, warn)
		}
	}
	RenderDivider(w, '-')
}

// RenderSessionInspect writes persisted session information from SQLite.
func RenderSessionInspect(w io.Writer, rec *db.SessionRecord, agentMode bool) {
	if rec == nil || rec.Agent == nil {
		return
	}

	agent := rec.Agent
	if agentMode {
		costVal := 0.0
		if agent.Activity.Cost != nil {
			costVal = agent.Activity.Cost.Total
		}
		_, _ = fmt.Fprintf(w, "db_agent id:%s session_id:%s harness:%s env:%s lifecycle:%s activity:%s title:%q action:%q tokens:%d cost_usd:%.4f updated_at:%d runs:%d events:%d\n",
			agent.ID, agent.SessionID, agent.Harness, agent.Environment.Label,
			agent.Activity.Lifecycle, agent.Activity.Activity, agent.Activity.Title, agent.Activity.Action,
			agent.Activity.Usage.Total, costVal, agent.Activity.UpdatedAt, len(rec.Runs), rec.EventCount)
		for _, r := range rec.Runs {
			endedStr := "running"
			if r.EndedAt != nil {
				endedStr = fmt.Sprintf("%d", *r.EndedAt)
			}
			_, _ = fmt.Fprintf(w, "  db_run id:%s pid:%d harness_version:%s started_at:%d ended_at:%s\n",
				r.ID, r.PID, r.HarnessVersion, r.StartedAt, endedStr)
		}
		return
	}

	RenderDivider(w, '-')
	_, _ = fmt.Fprintf(w, "Persisted Agent Record (%s)\n", agent.SessionID)
	RenderDivider(w, '-')
	_, _ = fmt.Fprintf(w, "  Agent ID:        %s\n", agent.ID)
	_, _ = fmt.Fprintf(w, "  Session ID:      %s\n", agent.SessionID)
	_, _ = fmt.Fprintf(w, "  Harness:         %s\n", agent.Harness)
	_, _ = fmt.Fprintf(w, "  Environment:     %s (%s)\n", agent.Environment.Label, agent.Environment.Hostname)
	_, _ = fmt.Fprintf(w, "  Project:         %s (branch: %s)\n", agent.Project.Name, agent.Workspace.Branch)
	_, _ = fmt.Fprintf(w, "  Directory:       %s\n", agent.Workspace.CWD)
	if agent.Activity.Title != "" {
		_, _ = fmt.Fprintf(w, "  Title:           %s\n", agent.Activity.Title)
	}
	if agent.Activity.Action != "" {
		_, _ = fmt.Fprintf(w, "  Action:          %s\n", agent.Activity.Action)
	}
	_, _ = fmt.Fprintf(w, "  Lifecycle:       %s\n", agent.Activity.Lifecycle)
	_, _ = fmt.Fprintf(w, "  Activity:        %s\n", agent.Activity.Activity)
	_, _ = fmt.Fprintf(w, "  Token Usage:     Input: %d | Output: %d | Total: %d\n",
		agent.Activity.Usage.Input, agent.Activity.Usage.Output, agent.Activity.Usage.Total)
	if agent.Activity.Cost != nil {
		_, _ = fmt.Fprintf(w, "  Total Cost:      $%.4f USD\n", agent.Activity.Cost.Total)
	}
	if agent.Activity.CurrentRun != nil {
		_, _ = fmt.Fprintf(w, "  Current Run:     %s (PID: %d)\n", agent.Activity.CurrentRun.ID, agent.Activity.CurrentRun.PID)
	}
	_, _ = fmt.Fprintf(w, "  Historical Runs: %d\n", len(rec.Runs))
	for _, r := range rec.Runs {
		endedDesc := "RUNNING"
		if r.EndedAt != nil {
			endedDesc = time.UnixMilli(*r.EndedAt).UTC().Format("2006-01-02 15:04:05 UTC")
		}
		_, _ = fmt.Fprintf(w, "    - Run %s: PID %d, started %s, ended %s\n",
			r.ID, r.PID, time.UnixMilli(r.StartedAt).UTC().Format("2006-01-02 15:04:05 UTC"), endedDesc)
	}
	_, _ = fmt.Fprintf(w, "  Telemetry Events: %d recorded\n", rec.EventCount)
	if rec.FirstEventTimestamp != nil && rec.LastEventTimestamp != nil {
		_, _ = fmt.Fprintf(w, "  Event Window:    %s to %s\n",
			time.UnixMilli(*rec.FirstEventTimestamp).UTC().Format("2006-01-02 15:04:05 UTC"),
			time.UnixMilli(*rec.LastEventTimestamp).UTC().Format("2006-01-02 15:04:05 UTC"))
	}
	_, _ = fmt.Fprintf(w, "  Last Updated:    %s\n", time.UnixMilli(agent.Activity.UpdatedAt).UTC().Format("2006-01-02 15:04:05 UTC"))
	RenderDivider(w, '-')
}
