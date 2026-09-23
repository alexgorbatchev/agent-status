package store_test

import (
	"math"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
)

func TestStore_ContextWindowAndFullness_Persistence(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	env := &types.IEnvironmentInfo{
		ID:              "env-cw-1",
		Label:           "Local Env",
		Hostname:        "host",
		Platform:        "linux",
		User:            "alex",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	if err := st.UpsertEnvironment(*env); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	model := "claude-3-7-sonnet-20250219"
	cw := int64(200000)
	fullness := 0.275

	agent := &types.IAgentState{
		ID:        "env-cw-1:claude-code:sess-cw-1",
		SessionID: "sess-cw-1",
		Harness:   types.HarnessClaudeCode,
		Environment: *env,
		Project: types.IProjectInfo{
			ID:            "proj-1",
			Name:          "repo",
			CanonicalRoot: "/repo",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/repo",
		},
		Activity: types.IAgentActivity{
			Lifecycle:       types.LifecycleRunning,
			Activity:        types.ActivityIdle,
			Title:           "Context Window Test",
			Action:          "Testing",
			Model:           &model,
			ContextWindow:   &cw,
			ContextFullness: &fullness,
			Usage: types.ITokenUsage{
				Input:      5000,
				Output:     1000,
				CacheRead:  40000,
				CacheWrite: 9000,
				Total:      55000,
			},
			FirstStartedAt: time.Now().UnixMilli(),
			UpdatedAt:      time.Now().UnixMilli(),
		},
	}

	if err := st.SaveAgentState(*agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	loaded, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}

	if loaded.Activity.Model == nil || *loaded.Activity.Model != model {
		t.Fatalf("expected Model %q, got %v", model, loaded.Activity.Model)
	}
	if loaded.Activity.ContextWindow == nil || *loaded.Activity.ContextWindow != cw {
		t.Fatalf("expected ContextWindow %d, got %v", cw, loaded.Activity.ContextWindow)
	}
	if loaded.Activity.ContextFullness == nil || math.Abs(*loaded.Activity.ContextFullness-fullness) > 1e-6 {
		t.Fatalf("expected ContextFullness %f, got %v", fullness, loaded.Activity.ContextFullness)
	}
}
