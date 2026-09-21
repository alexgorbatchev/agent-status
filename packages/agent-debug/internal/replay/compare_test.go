package replay

import (
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/db"
)

func TestCompareSessionWithDB_Match(t *testing.T) {
	now := time.Now().UnixMilli()
	runID := "run-1"

	agentReplay := &types.IAgentState{
		ID:        "env-test:claude-code:ses-1",
		SessionID: "ses-1",
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			Usage: types.ITokenUsage{
				Total: 1000,
			},
			Cost: &types.ICostInfo{
				Total: 0.05,
			},
			UpdatedAt: now,
		},
	}

	res := &ReplayResult{
		Harness:            types.HarnessClaudeCode,
		SessionID:          "ses-1",
		AgentID:            "env-test:claude-code:ses-1",
		Agent:              agentReplay,
		LastEventTimestamp: now,
	}

	agentDB := &types.IAgentState{
		ID:        "env-test:claude-code:ses-1",
		SessionID: "ses-1",
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			Usage: types.ITokenUsage{
				Total: 1000,
			},
			Cost: &types.ICostInfo{
				Total: 0.05,
			},
			UpdatedAt: now,
		},
	}

	lastEvt := now
	rec := &db.SessionRecord{
		DBPath:             "/path/to/agent-status.db",
		Agent:              agentDB,
		Runs:               []db.RunRecord{{ID: runID, PID: 1234, StartedAt: now - 5000, EndedAt: &now}},
		EventCount:         5,
		LastEventTimestamp: &lastEvt,
	}

	comp := CompareSessionWithDB(res, rec)
	if comp == nil {
		t.Fatal("expected non-nil ComparisonResult")
	}

	if comp.HasDrift {
		t.Errorf("expected no drift, got HasDrift = true")
	}
	if len(comp.Warnings) > 0 {
		t.Errorf("expected 0 warnings, got %v", comp.Warnings)
	}

	// Verify all fields are MATCH
	for _, f := range comp.Fields {
		if f.Status != "MATCH" {
			t.Errorf("expected field %s to be MATCH, got %s", f.Name, f.Status)
		}
		if f.IsDrift {
			t.Errorf("expected field %s IsDrift = false", f.Name)
		}
	}
}

func TestCompareSessionWithDB_Drift(t *testing.T) {
	t0 := time.Now().UnixMilli() - 3600000*31 // 31 hours ago
	tNow := time.Now().UnixMilli()
	runID := "run-live"

	agentReplay := &types.IAgentState{
		ID:        "env-test:claude-code:ses-1",
		SessionID: "ses-1",
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			Usage: types.ITokenUsage{
				Total: 34852280,
			},
			Cost: &types.ICostInfo{
				Total: 1.2069,
			},
			UpdatedAt: t0,
		},
	}

	res := &ReplayResult{
		Harness:            types.HarnessClaudeCode,
		SessionID:          "ses-1",
		AgentID:            "env-test:claude-code:ses-1",
		Agent:              agentReplay,
		LastEventTimestamp: t0,
	}

	agentDB := &types.IAgentState{
		ID:        "env-test:claude-code:ses-1",
		SessionID: "ses-1",
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleRunning,
			Activity:  types.ActivityIdle,
			CurrentRun: &types.IRunInfo{
				ID: runID,
			},
			Usage: types.ITokenUsage{
				Total: 36442473,
			},
			Cost: &types.ICostInfo{
				Total: 1.2685,
			},
			UpdatedAt: tNow,
		},
	}

	rec := &db.SessionRecord{
		DBPath:             "/path/to/agent-status.db",
		Agent:              agentDB,
		Runs:               []db.RunRecord{{ID: runID, PID: 27680, StartedAt: t0, EndedAt: nil}},
		EventCount:         42,
		LastEventTimestamp: &t0,
	}

	comp := CompareSessionWithDB(res, rec)
	if comp == nil {
		t.Fatal("expected non-nil ComparisonResult")
	}

	if !comp.HasDrift {
		t.Errorf("expected HasDrift = true, got false")
	}

	// Verify warnings
	if len(comp.Warnings) == 0 {
		t.Error("expected warnings about updated_at drift and running lifecycle, got none")
	}

	fieldMap := make(map[string]DiscrepancyField)
	for _, f := range comp.Fields {
		fieldMap[f.Name] = f
	}

	// Check Lifecycle drift
	if f, ok := fieldMap["Lifecycle"]; !ok || !f.IsDrift || f.Status != "DRIFT" {
		t.Errorf("expected Lifecycle DRIFT, got %+v", f)
	}

	// Check Tokens drift
	if f, ok := fieldMap["Tokens Total"]; !ok || !f.IsDrift {
		t.Errorf("expected Tokens Total DRIFT, got %+v", f)
	}

	// Check Cost drift
	if f, ok := fieldMap["Cost USD"]; !ok || !f.IsDrift {
		t.Errorf("expected Cost USD DRIFT, got %+v", f)
	}

	// Check Updated At drift
	if f, ok := fieldMap["Updated At"]; !ok || !f.IsDrift {
		t.Errorf("expected Updated At DRIFT, got %+v", f)
	}
}
