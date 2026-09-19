package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestOpen_Memory(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer st.Close()

	if st.DB() == nil {
		t.Errorf("expected DB() to be non-nil")
	}

	var version int
	if err := st.DB().QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("query user_version failed: %v", err)
	}
	if version != 1 {
		t.Errorf("got user_version = %d, want 1", version)
	}
}

func TestOpen_DiskPath(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sub", "test.db")

	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(diskPath) failed: %v", err)
	}
	defer st.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("db file not found: %v", err)
	}

	if err := Migrate(st.DB()); err != nil {
		t.Errorf("Migrate() second run failed: %v", err)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	tempFile, err := os.CreateTemp("", "db-file")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	invalidPath := filepath.Join(tempFile.Name(), "subdir", "test.db")
	_, err = Open(invalidPath)
	if err == nil {
		t.Errorf("expected error opening db with invalid dir path, got nil")
	}
}

func TestOpen_MigrationFailure(t *testing.T) {
	// Attempt to open /dev/null as a database which fails migration table creation
	_, err := Open("/dev/null")
	if err == nil {
		t.Errorf("expected error opening /dev/null, got nil")
	}
}

func TestMigrate_ErrorCases(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	db.Close()
	if err := Migrate(db); err == nil {
		t.Errorf("expected error on closed db, got nil")
	}
}

func TestCursors(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	envID := "env-cursor-test"

	seq, err := st.GetLastSeq(envID)
	if err != nil {
		t.Fatalf("GetLastSeq() failed: %v", err)
	}
	if seq != 0 {
		t.Errorf("expected 0 for non-existent cursor, got %d", seq)
	}

	env := types.IEnvironmentInfo{
		ID:              envID,
		Label:           "test-env",
		Hostname:        "localhost",
		Platform:        "darwin",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	if err := st.UpsertEnvironment(env); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	if err := st.SaveCursor(envID, 42); err != nil {
		t.Fatalf("SaveCursor failed: %v", err)
	}

	seq, err = st.GetLastSeq(envID)
	if err != nil {
		t.Fatalf("GetLastSeq() after save failed: %v", err)
	}
	if seq != 42 {
		t.Errorf("got seq = %d, want 42", seq)
	}

	if err := st.SaveCursor(envID, 100); err != nil {
		t.Fatalf("SaveCursor update failed: %v", err)
	}

	seq, err = st.GetLastSeq(envID)
	if err != nil {
		t.Fatalf("GetLastSeq() after update failed: %v", err)
	}
	if seq != 100 {
		t.Errorf("got seq = %d, want 100", seq)
	}
}

func TestEnvironments(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	emptyEnvs, err := st.GetAllEnvironments()
	if err != nil {
		t.Fatalf("GetAllEnvironments on empty db failed: %v", err)
	}
	if len(emptyEnvs) != 0 {
		t.Errorf("expected 0 environments, got %d", len(emptyEnvs))
	}

	now := time.Now().UnixMilli()
	env1 := types.IEnvironmentInfo{
		ID:              "env-1",
		Label:           "laptop",
		Hostname:        "macbook.local",
		Platform:        "darwin",
		User:            "alex",
		LastHeartbeatAt: now,
	}

	if err := st.UpsertEnvironment(env1); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	if err := st.IncrementDroppedFrames("env-1", 5); err != nil {
		t.Fatalf("IncrementDroppedFrames failed: %v", err)
	}
	if err := st.IncrementDroppedFrames("env-1", 3); err != nil {
		t.Fatalf("IncrementDroppedFrames 2 failed: %v", err)
	}

	newHeartbeat := now + 1000
	if err := st.RecordHeartbeat("env-1", newHeartbeat); err != nil {
		t.Fatalf("RecordHeartbeat failed: %v", err)
	}

	env2 := types.IEnvironmentInfo{
		ID:              "env-2",
		Label:           "server",
		Hostname:        "linux-box",
		Platform:        "linux",
		User:            "root",
		LastHeartbeatAt: now - 60000,
	}
	if err := st.UpsertEnvironment(env2); err != nil {
		t.Fatalf("UpsertEnvironment env2 failed: %v", err)
	}

	envs, err := st.GetAllEnvironments()
	if err != nil {
		t.Fatalf("GetAllEnvironments failed: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("got %d environments, want 2", len(envs))
	}

	envMap := make(map[string]types.IEnvironmentInfo)
	for _, e := range envs {
		envMap[e.ID] = e
	}

	got1, ok := envMap["env-1"]
	if !ok {
		t.Fatalf("env-1 not found")
	}
	if got1.Label != "laptop" || got1.LastHeartbeatAt != newHeartbeat || got1.IsStale {
		t.Errorf("env-1 mismatch: %+v", got1)
	}

	got2, ok := envMap["env-2"]
	if !ok {
		t.Fatalf("env-2 not found")
	}
	if !got2.IsStale {
		t.Errorf("expected env-2 to be stale (heartbeat 60s ago)")
	}

	env1Updated := env1
	env1Updated.Label = "laptop-renamed"
	if err := st.UpsertEnvironment(env1Updated); err != nil {
		t.Fatalf("UpsertEnvironment update failed: %v", err)
	}
	envsAfter, _ := st.GetAllEnvironments()
	for _, e := range envsAfter {
		if e.ID == "env-1" && e.Label != "laptop-renamed" {
			t.Errorf("got label %q, want laptop-renamed", e.Label)
		}
	}
}

func TestAgentStateAndRuns(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	agentNil, err := st.GetAgent("non-existent")
	if err != nil {
		t.Fatalf("GetAgent non-existent error: %v", err)
	}
	if agentNil != nil {
		t.Errorf("expected nil agent, got %+v", agentNil)
	}

	now := time.Now().UnixMilli()
	phase := types.PhaseEditing
	tool := "Edit"
	target := "src/main.ts"
	question := "Proceed?"
	thinking := "Refactoring logic..."
	lastErr := "compilation error"
	relPath := "worktrees/feat"

	agent := types.IAgentState{
		ID:        "env-1:claude-code:sess-1",
		SessionID: "sess-1",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:              "env-1",
			Label:           "dev-mac",
			Hostname:        "mac",
			Platform:        "darwin",
			User:            "alex",
			LastHeartbeatAt: now - 50000, // stale heartbeat to test IsStale in GetAgent
		},
		Project: types.IProjectInfo{
			ID:            "github.com/alex/agent-status",
			Name:          "agent-status",
			CanonicalRoot: "/dev/agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     "feat/test",
			IsDetached: true,
			IsWorktree: true,
			RelPath:    &relPath,
			CWD:        "/dev/agent-status/worktrees/feat",
		},
		Activity: types.IAgentActivity{
			Lifecycle:   types.LifecycleRunning,
			Activity:    types.ActivityBusy,
			Title:       "Feature Dev",
			Action:      "Editing src/main.ts",
			ActionStale: true,
			CurrentRun: &types.IRunInfo{
				ID:             "run-101",
				PID:            1234,
				HarnessVersion: "2.1.277",
				StartedAt:      now,
			},
			Phase:               &phase,
			ActiveTool:          &tool,
			ActiveTarget:        &target,
			Question:            &question,
			LastThinkingExcerpt: &thinking,
			LastError:           &lastErr,
			Usage: types.ITokenUsage{
				Input:      1000,
				Output:     200,
				CacheRead:  5000,
				CacheWrite: 1000,
				Reasoning:  100,
				Total:      7300,
			},
			Cost: &types.ICostInfo{
				Currency: "USD",
				Total:    0.056,
			},
			SubagentCount:  2,
			FirstStartedAt: now - 10000,
			UpdatedAt:      now,
		},
	}

	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	got, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected agent to be found")
	}

	if !got.Environment.IsStale {
		t.Errorf("expected got.Environment.IsStale to be true")
	}

	if got.ID != agent.ID || got.SessionID != "sess-1" || got.Harness != types.HarnessClaudeCode {
		t.Errorf("identity mismatch: %+v", got)
	}
	if !got.Workspace.IsDetached || !got.Workspace.IsWorktree || got.Workspace.RelPath == nil || *got.Workspace.RelPath != relPath {
		t.Errorf("workspace mismatch: %+v", got.Workspace)
	}
	if got.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("got lifecycle %s, want %s", got.Activity.Lifecycle, types.LifecycleRunning)
	}
	if !got.Activity.ActionStale {
		t.Errorf("expected ActionStale = true")
	}
	if got.Activity.CurrentRun == nil || got.Activity.CurrentRun.ID != "run-101" || got.Activity.CurrentRun.PID != 1234 {
		t.Errorf("currentRun mismatch: %+v", got.Activity.CurrentRun)
	}
	if got.Activity.Phase == nil || *got.Activity.Phase != types.PhaseEditing {
		t.Errorf("phase mismatch: %v", got.Activity.Phase)
	}
	if got.Activity.ActiveTool == nil || *got.Activity.ActiveTool != tool {
		t.Errorf("activeTool mismatch: %v", got.Activity.ActiveTool)
	}
	if got.Activity.ActiveTarget == nil || *got.Activity.ActiveTarget != target {
		t.Errorf("activeTarget mismatch: %v", got.Activity.ActiveTarget)
	}
	if got.Activity.Question == nil || *got.Activity.Question != question {
		t.Errorf("question mismatch: %v", got.Activity.Question)
	}
	if got.Activity.LastThinkingExcerpt == nil || *got.Activity.LastThinkingExcerpt != thinking {
		t.Errorf("thinking mismatch: %v", got.Activity.LastThinkingExcerpt)
	}
	if got.Activity.LastError == nil || *got.Activity.LastError != lastErr {
		t.Errorf("lastError mismatch: %v", got.Activity.LastError)
	}
	if got.Activity.Cost == nil || got.Activity.Cost.Total != 0.056 {
		t.Errorf("cost mismatch: %+v", got.Activity.Cost)
	}
	if got.Activity.SubagentCount != 2 {
		t.Errorf("subagentCount = %d, want 2", got.Activity.SubagentCount)
	}

	// Create second agent to test multiple items in GetAllActiveAgents
	agent2 := agent
	agent2.ID = "env-1:claude-code:sess-2"
	agent2.SessionID = "sess-2"
	agent2.Activity.CurrentRun = nil
	agent2.Activity.UpdatedAt = now + 100
	if err := st.SaveAgentState(agent2); err != nil {
		t.Fatalf("SaveAgentState(agent2) failed: %v", err)
	}

	allAgents, err := st.GetAllActiveAgents()
	if err != nil {
		t.Fatalf("GetAllActiveAgents failed: %v", err)
	}
	if len(allAgents) != 2 {
		t.Errorf("GetAllActiveAgents count = %d, want 2", len(allAgents))
	}

	// Update existing agent state with new run and updated fields
	agentUpdated := *got
	agentUpdated.Activity.Title = "Updated Title"
	agentUpdated.Activity.ActionStale = false
	agentUpdated.Activity.CurrentRun = &types.IRunInfo{
		ID:             "run-102",
		PID:            5678,
		HarnessVersion: "2.1.278",
		StartedAt:      now + 100,
	}
	if err := st.SaveAgentState(agentUpdated); err != nil {
		t.Fatalf("SaveAgentState(agentUpdated) failed: %v", err)
	}

	gotUpdated, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent updated error: %v", err)
	}
	if gotUpdated.Activity.Title != "Updated Title" || gotUpdated.Activity.CurrentRun.ID != "run-102" || gotUpdated.Activity.ActionStale {
		t.Errorf("gotUpdated mismatch: %+v", gotUpdated)
	}

	subID := "sub-99"
	frame := types.ITelemetryFrame{
		AgentID:     agent.ID,
		SubagentID:  &subID,
		EventType:   "tool_call",
		Timestamp:   now,
		Data: types.TelemetryFrameData{
			ToolName: &tool,
		},
	}
	if err := st.RecordTelemetryEvent(frame); err != nil {
		t.Fatalf("RecordTelemetryEvent failed: %v", err)
	}

	endedAt := now + 5000
	if err := st.EndCurrentRun(agent.ID, endedAt); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}

	afterEnd, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after end failed: %v", err)
	}
	if afterEnd.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("got lifecycle %s, want %s", afterEnd.Activity.Lifecycle, types.LifecycleExited)
	}

	if err := st.EndCurrentRun(agent.ID, endedAt+1000); err != nil {
		t.Fatalf("EndCurrentRun when null failed: %v", err)
	}

	if err := st.EndCurrentRun("non-existent-agent-id", endedAt+2000); err != nil {
		t.Fatalf("EndCurrentRun for non-existent agent error: %v", err)
	}
}

func TestAgentState_NilFieldsAndExitedRun(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	now := time.Now().UnixMilli()
	ended := now - 1000

	agent := types.IAgentState{
		ID:        "env-nil:pi:sess-nil",
		SessionID: "sess-nil",
		Harness:   types.HarnessPi,
		Environment: types.IEnvironmentInfo{
			ID:              "env-nil",
			Label:           "minimal",
			Hostname:        "host",
			Platform:        "linux",
			User:            "user",
			LastHeartbeatAt: now,
		},
		Project: types.IProjectInfo{
			ID:            "project-1",
			Name:          "proj",
			CanonicalRoot: "/root",
		},
		Workspace: types.IWorkspaceInfo{
			Branch:     "main",
			IsDetached: false,
			IsWorktree: false,
			RelPath:    nil,
			CWD:        "/root",
		},
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityIdle,
			CurrentRun: &types.IRunInfo{
				ID:             "run-ended",
				PID:            5555,
				HarnessVersion: "1.0.0",
				StartedAt:      now - 5000,
				EndedAt:        &ended,
			},
			Cost:           nil,
			FirstStartedAt: now - 5000,
			UpdatedAt:      now,
		},
	}

	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	got, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil {
		t.Fatalf("GetAgent returned nil")
	}

	if got.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("got lifecycle %s, want %s", got.Activity.Lifecycle, types.LifecycleExited)
	}
	if got.Activity.Cost != nil {
		t.Errorf("expected nil cost, got %+v", got.Activity.Cost)
	}
	if got.Activity.Phase != nil || got.Activity.ActiveTool != nil || got.Activity.ActiveTarget != nil || got.Activity.Question != nil || got.Activity.LastThinkingExcerpt != nil || got.Activity.LastError != nil {
		t.Errorf("expected all optional activity fields to be nil, got %+v", got.Activity)
	}

	agentNoRun := *got
	agentNoRun.Activity.CurrentRun = nil
	if err := st.SaveAgentState(agentNoRun); err != nil {
		t.Fatalf("SaveAgentState(agentNoRun) failed: %v", err)
	}

	gotNoRun, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after no run failed: %v", err)
	}
	if gotNoRun.Activity.CurrentRun != nil {
		t.Errorf("expected CurrentRun to be nil, got %+v", gotNoRun.Activity.CurrentRun)
	}
}

func TestStore_ErrorHandlingOnClosedDB(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	_ = st.Close()

	if _, err := st.GetLastSeq("env"); err == nil {
		t.Errorf("expected error on closed db for GetLastSeq")
	}
	if err := st.SaveCursor("env", 1); err == nil {
		t.Errorf("expected error on closed db for SaveCursor")
	}
	if err := st.UpsertEnvironment(types.IEnvironmentInfo{ID: "e", Label: "l", Hostname: "h", Platform: "darwin", User: "u"}); err == nil {
		t.Errorf("expected error on closed db for UpsertEnvironment")
	}
	if err := st.RecordHeartbeat("e", 100); err == nil {
		t.Errorf("expected error on closed db for RecordHeartbeat")
	}
	if err := st.IncrementDroppedFrames("e", 1); err == nil {
		t.Errorf("expected error on closed db for IncrementDroppedFrames")
	}
	if err := st.RecordTelemetryEvent(types.ITelemetryFrame{AgentID: "a", Timestamp: 100}); err == nil {
		t.Errorf("expected error on closed db for RecordTelemetryEvent")
	}
	if err := st.SaveAgentState(types.IAgentState{ID: "a"}); err == nil {
		t.Errorf("expected error on closed db for SaveAgentState")
	}
	if _, err := st.GetAgent("a"); err == nil {
		t.Errorf("expected error on closed db for GetAgent")
	}
	if _, err := st.GetAllActiveAgents(); err == nil {
		t.Errorf("expected error on closed db for GetAllActiveAgents")
	}
	if _, err := st.GetAllEnvironments(); err == nil {
		t.Errorf("expected error on closed db for GetAllEnvironments")
	}
	if err := st.EndCurrentRun("a", 100); err == nil {
		t.Errorf("expected error on closed db for EndCurrentRun")
	}
	if err := st.PruneOldEventsAndExitedAgents(100, 100); err == nil {
		t.Errorf("expected error on closed db for PruneOldEventsAndExitedAgents")
	}
}

func TestPruneOldEventsAndExitedAgents(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	now := time.Now().UnixMilli()
	oldTime := now - (50 * 3600 * 1000) // 50 hours ago
	recentTime := now - (10 * 1000)

	env := types.IEnvironmentInfo{
		ID:              "env-prune",
		Label:           "prune-env",
		Hostname:        "localhost",
		Platform:        "darwin",
		User:            "tester",
		LastHeartbeatAt: now,
	}
	if err := st.UpsertEnvironment(env); err != nil {
		t.Fatalf("UpsertEnvironment failed: %v", err)
	}

	agent1 := types.IAgentState{
		ID:          "env-prune:claude-code:sess-old",
		SessionID:   "sess-old",
		Harness:     types.HarnessClaudeCode,
		Environment: env,
		Project: types.IProjectInfo{
			ID:            "github.com/test/repo",
			Name:          "repo",
			CanonicalRoot: "/repo",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/repo",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleExited,
			Activity:       types.ActivityIdle,
			FirstStartedAt: now - (45 * 24 * 3600 * 1000),
			UpdatedAt:      now - (40 * 24 * 3600 * 1000),
		},
	}
	if err := st.SaveAgentState(agent1); err != nil {
		t.Fatalf("SaveAgentState(agent1) failed: %v", err)
	}

	agent2 := types.IAgentState{
		ID:          "env-prune:claude-code:sess-active",
		SessionID:   "sess-active",
		Harness:     types.HarnessClaudeCode,
		Environment: env,
		Project: types.IProjectInfo{
			ID:            "github.com/test/repo",
			Name:          "repo",
			CanonicalRoot: "/repo",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/repo",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			FirstStartedAt: recentTime,
			UpdatedAt:      recentTime,
		},
	}
	if err := st.SaveAgentState(agent2); err != nil {
		t.Fatalf("SaveAgentState(agent2) failed: %v", err)
	}

	oldFrame := types.ITelemetryFrame{
		AgentID:   agent2.ID,
		EventType: "tool_call",
		Timestamp: oldTime,
		Data:      types.TelemetryFrameData{},
	}
	if err := st.RecordTelemetryEvent(oldFrame); err != nil {
		t.Fatalf("RecordTelemetryEvent(old) failed: %v", err)
	}

	recentFrame := types.ITelemetryFrame{
		AgentID:   agent2.ID,
		EventType: "tool_call",
		Timestamp: recentTime,
		Data:      types.TelemetryFrameData{},
	}
	if err := st.RecordTelemetryEvent(recentFrame); err != nil {
		t.Fatalf("RecordTelemetryEvent(recent) failed: %v", err)
	}

	eventCutoff := now - (48 * 3600 * 1000)
	exitedAgentCutoff := now - (30 * 24 * 3600 * 1000)

	if err := st.PruneOldEventsAndExitedAgents(eventCutoff, exitedAgentCutoff); err != nil {
		t.Fatalf("PruneOldEventsAndExitedAgents failed: %v", err)
	}

	prunedAgent, err := st.GetAgent(agent1.ID)
	if err != nil {
		t.Fatalf("GetAgent(agent1) error: %v", err)
	}
	if prunedAgent != nil {
		t.Errorf("expected old exited agent to be pruned, but found %+v", prunedAgent)
	}

	activeAgent, err := st.GetAgent(agent2.ID)
	if err != nil {
		t.Fatalf("GetAgent(agent2) error: %v", err)
	}
	if activeAgent == nil {
		t.Errorf("expected active agent to remain")
	}

	var eventCount int
	if err := st.DB().QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&eventCount); err != nil {
		t.Fatalf("query telemetry_events count failed: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("got %d telemetry events, want 1", eventCount)
	}
}
