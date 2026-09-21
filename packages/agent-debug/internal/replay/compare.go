package replay

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
)

type DiscrepancyField = output.DiscrepancyField
type ComparisonResult = output.ComparisonResult

// CompareSessionWithDB evaluates differences between replayed transcript state and persisted database state.
func CompareSessionWithDB(res *ReplayResult, record *db.SessionRecord) *ComparisonResult {
	if res == nil || record == nil || res.Agent == nil || record.Agent == nil {
		return nil
	}

	comp := &ComparisonResult{
		DBPath:    record.DBPath,
		SessionID: res.SessionID,
		AgentID:   res.AgentID,
		Fields:    make([]DiscrepancyField, 0),
		Warnings:  make([]string, 0),
	}

	// 1. Lifecycle
	repLifecycle := string(res.Agent.Activity.Lifecycle)
	dbLifecycle := string(record.Agent.Activity.Lifecycle)
	lifecycleDrift := repLifecycle != dbLifecycle
	lifecycleStatus := "MATCH"
	if lifecycleDrift {
		lifecycleStatus = "DRIFT"
	}
	comp.Fields = append(comp.Fields, DiscrepancyField{
		Name:           "Lifecycle",
		ReplayedValue:  repLifecycle,
		PersistedValue: dbLifecycle,
		Status:         lifecycleStatus,
		IsDrift:        lifecycleDrift,
	})

	// 2. Activity
	repActivity := string(res.Agent.Activity.Activity)
	dbActivity := string(record.Agent.Activity.Activity)
	activityDrift := repActivity != dbActivity
	activityStatus := "MATCH"
	if activityDrift {
		activityStatus = "DRIFT"
	}
	comp.Fields = append(comp.Fields, DiscrepancyField{
		Name:           "Activity",
		ReplayedValue:  repActivity,
		PersistedValue: dbActivity,
		Status:         activityStatus,
		IsDrift:        activityDrift,
	})

	// 3. Tokens Total
	repTokens := res.Agent.Activity.Usage.Total
	dbTokens := record.Agent.Activity.Usage.Total
	tokensDelta := dbTokens - repTokens
	tokensDrift := tokensDelta != 0
	tokensStatus := "MATCH"
	if tokensDrift {
		if tokensDelta > 0 {
			tokensStatus = fmt.Sprintf("DRIFT (+%s)", formatNumber(tokensDelta))
		} else {
			tokensStatus = fmt.Sprintf("DRIFT (-%s)", formatNumber(-tokensDelta))
		}
	}
	comp.Fields = append(comp.Fields, DiscrepancyField{
		Name:           "Tokens Total",
		ReplayedValue:  formatNumber(repTokens),
		PersistedValue: formatNumber(dbTokens),
		Status:         tokensStatus,
		IsDrift:        tokensDrift,
	})

	// 4. Cost USD
	repCost := 0.0
	if res.Agent.Activity.Cost != nil {
		repCost = res.Agent.Activity.Cost.Total
	}
	dbCost := 0.0
	if record.Agent.Activity.Cost != nil {
		dbCost = record.Agent.Activity.Cost.Total
	}
	costDelta := dbCost - repCost
	costDrift := math.Abs(costDelta) >= 0.0001
	costStatus := "MATCH"
	if costDrift {
		if costDelta > 0 {
			costStatus = fmt.Sprintf("DRIFT (+$%.4f)", costDelta)
		} else {
			costStatus = fmt.Sprintf("DRIFT (-$%.4f)", -costDelta)
		}
	}
	comp.Fields = append(comp.Fields, DiscrepancyField{
		Name:           "Cost USD",
		ReplayedValue:  fmt.Sprintf("$%.4f", repCost),
		PersistedValue: fmt.Sprintf("$%.4f", dbCost),
		Status:         costStatus,
		IsDrift:        costDrift,
	})

	// 5. Last Event Time
	repLastEvent := res.LastEventTimestamp
	if repLastEvent == 0 && res.Agent != nil {
		repLastEvent = res.Agent.Activity.UpdatedAt
	}
	dbLastEvent := int64(0)
	if record.LastEventTimestamp != nil {
		dbLastEvent = *record.LastEventTimestamp
	} else if record.Agent != nil {
		dbLastEvent = record.Agent.Activity.UpdatedAt
	}
	eventTimeDiff := math.Abs(float64(dbLastEvent - repLastEvent))
	eventTimeDrift := eventTimeDiff > 1000
	eventTimeStatus := "MATCH"
	if eventTimeDrift {
		eventTimeStatus = "DRIFT"
	}
	comp.Fields = append(comp.Fields, DiscrepancyField{
		Name:           "Last Event Time",
		ReplayedValue:  formatTimestampUTC(repLastEvent),
		PersistedValue: formatTimestampUTC(dbLastEvent),
		Status:         eventTimeStatus,
		IsDrift:        eventTimeDrift,
	})

	// 6. Updated At
	repUpdated := res.Agent.Activity.UpdatedAt
	dbUpdated := record.Agent.Activity.UpdatedAt
	updatedDelta := dbUpdated - repUpdated
	updatedDrift := math.Abs(float64(updatedDelta)) > 1000
	updatedStatus := "MATCH"
	if updatedDrift {
		if updatedDelta > 0 {
			updatedStatus = fmt.Sprintf("DRIFT (+%s)", formatDurationDelta(updatedDelta))
		} else {
			updatedStatus = fmt.Sprintf("DRIFT (-%s)", formatDurationDelta(-updatedDelta))
		}
	}
	comp.Fields = append(comp.Fields, DiscrepancyField{
		Name:           "Updated At",
		ReplayedValue:  formatTimestampUTC(repUpdated),
		PersistedValue: formatTimestampUTC(dbUpdated),
		Status:         updatedStatus,
		IsDrift:        updatedDrift,
	})

	// 7. Current Run ID
	repRunID := "NONE"
	if res.Agent.Activity.CurrentRun != nil {
		repRunID = res.Agent.Activity.CurrentRun.ID
	}
	dbRunID := "NONE"
	if record.Agent.Activity.CurrentRun != nil {
		dbRunID = record.Agent.Activity.CurrentRun.ID
	}
	runDrift := repRunID != dbRunID
	runStatus := "MATCH"
	if runDrift {
		runStatus = "DRIFT"
	}
	comp.Fields = append(comp.Fields, DiscrepancyField{
		Name:           "Current Run ID",
		ReplayedValue:  repRunID,
		PersistedValue: dbRunID,
		Status:         runStatus,
		IsDrift:        runDrift,
	})

	// Check overall drift
	for _, f := range comp.Fields {
		if f.IsDrift {
			comp.HasDrift = true
			break
		}
	}

	// Generate warnings
	if record.Agent.Activity.UpdatedAt > repLastEvent+60000 {
		diffMs := record.Agent.Activity.UpdatedAt - repLastEvent
		comp.Warnings = append(comp.Warnings, fmt.Sprintf("Database updated_at is %s newer than transcript last event time!", formatDurationDelta(diffMs)))
	}
	if record.Agent.Activity.Lifecycle == types.LifecycleRunning && res.Agent.Activity.Lifecycle == types.LifecycleExited {
		comp.Warnings = append(comp.Warnings, "Database agent is still marked RUNNING while transcript replay finished!")
	}
	if record.Agent.Activity.CurrentRun != nil && res.Agent.Activity.CurrentRun == nil {
		comp.Warnings = append(comp.Warnings, fmt.Sprintf("Database has active run %s while replayed transcript has no active run", record.Agent.Activity.CurrentRun.ID))
	}

	return comp
}

func formatTimestampUTC(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04:05 UTC")
}

func formatDurationDelta(ms int64) string {
	if ms < 0 {
		ms = -ms
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if hours > 0 {
		if mins > 0 {
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", mins)
}

func formatNumber(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	in := strconv.FormatInt(n, 10)
	out := make([]byte, 0, len(in)+len(in)/3)
	leading := len(in) % 3
	if leading > 0 {
		out = append(out, in[:leading]...)
		if len(in) > leading {
			out = append(out, ',')
		}
	}
	for i := leading; i < len(in); i += 3 {
		out = append(out, in[i:i+3]...)
		if i+3 < len(in) {
			out = append(out, ',')
		}
	}
	return sign + string(out)
}
