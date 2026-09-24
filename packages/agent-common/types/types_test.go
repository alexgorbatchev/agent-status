package types

import (
	"encoding/json"
	"testing"
)

func TestEnumConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		// LifecycleState
		{"LifecycleRunning", string(LifecycleRunning), "RUNNING"},
		{"LifecycleExited", string(LifecycleExited), "EXITED"},

		// ActivityState
		{"ActivityBusy", string(ActivityBusy), "BUSY"},
		{"ActivityWaitingQuestion", string(ActivityWaitingQuestion), "WAITING_QUESTION"},
		{"ActivityError", string(ActivityError), "ERROR"},
		{"ActivityRateLimited", string(ActivityRateLimited), "RATE_LIMITED"},
		{"ActivityIdle", string(ActivityIdle), "IDLE"},

		// HarnessType
		{"HarnessClaudeCode", string(HarnessClaudeCode), "claude-code"},
		{"HarnessPi", string(HarnessPi), "pi"},
		{"HarnessCodex", string(HarnessCodex), "codex"},
		{"HarnessOpencode", string(HarnessOpencode), "opencode"},

		// AgentPhase
		{"PhasePlanning", string(PhasePlanning), "planning"},
		{"PhaseEditing", string(PhaseEditing), "editing"},
		{"PhaseTesting", string(PhaseTesting), "testing"},
		{"PhaseDebugging", string(PhaseDebugging), "debugging"},
		{"PhaseInvestigating", string(PhaseInvestigating), "investigating"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestEnvironmentInfoJSON(t *testing.T) {
	env := IEnvironmentInfo{
		ID:              "env-1",
		Label:           "local-dev",
		Hostname:        "devbox",
		Platform:        "darwin",
		User:            "testuser",
		IsStale:         false,
		LastHeartbeatAt: 1700000000000,
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded IEnvironmentInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded != env {
		t.Fatalf("decoded %+v != original %+v", decoded, env)
	}
}

func TestWorkspaceInfoJSON(t *testing.T) {
	relPath := "packages/agent-server"
	ws := IWorkspaceInfo{
		IsWorktree: true,
		Branch:     "feat/test",
		IsDetached: false,
		RelPath:    &relPath,
		CWD:        "/workspace/agent-status",
	}

	data, err := json.Marshal(ws)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded IWorkspaceInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.IsWorktree != ws.IsWorktree || decoded.Branch != ws.Branch || decoded.CWD != ws.CWD {
		t.Fatalf("mismatched decoded workspace fields: %+v", decoded)
	}
	if decoded.RelPath == nil || *decoded.RelPath != relPath {
		t.Fatalf("mismatched RelPath: got %v, want %q", decoded.RelPath, relPath)
	}

	// Test nil RelPath
	wsNilRel := IWorkspaceInfo{
		IsWorktree: false,
		Branch:     "main",
		IsDetached: true,
		RelPath:    nil,
		CWD:        "/workspace/agent-status",
	}
	nilData, err := json.Marshal(wsNilRel)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decodedNil IWorkspaceInfo
	if err := json.Unmarshal(nilData, &decodedNil); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decodedNil.RelPath != nil {
		t.Fatalf("expected nil RelPath, got %v", *decodedNil.RelPath)
	}
}

func TestProjectInfoJSON(t *testing.T) {
	proj := IProjectInfo{
		ID:            "proj-123",
		Name:          "agent-status",
		CanonicalRoot: "/projects/agent-status",
	}

	data, err := json.Marshal(proj)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded IProjectInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded != proj {
		t.Fatalf("decoded %+v != original %+v", decoded, proj)
	}
}

func TestTokenUsageJSON(t *testing.T) {
	usage := ITokenUsage{
		Input:      1500,
		Output:     300,
		CacheRead:  500,
		CacheWrite: 200,
		Reasoning:  100,
		Total:      2600,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ITokenUsage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded != usage {
		t.Fatalf("decoded %+v != original %+v", decoded, usage)
	}
}

func TestCostInfoJSON(t *testing.T) {
	cost := ICostInfo{
		Currency:    "USD",
		Total:       0.045,
		IsEstimated: true,
	}

	data, err := json.Marshal(cost)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ICostInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded != cost {
		t.Fatalf("decoded %+v != original %+v", decoded, cost)
	}
}

func TestRunInfoJSON(t *testing.T) {
	endedAt := int64(1700000050000)
	run := IRunInfo{
		ID:             "run-1",
		PID:            12345,
		HarnessVersion: "1.0.0",
		StartedAt:      1700000000000,
		EndedAt:        &endedAt,
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded IRunInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.ID != run.ID || decoded.PID != run.PID || decoded.HarnessVersion != run.HarnessVersion || decoded.StartedAt != run.StartedAt {
		t.Fatalf("decoded run info mismatch: %+v", decoded)
	}
	if decoded.EndedAt == nil || *decoded.EndedAt != endedAt {
		t.Fatalf("EndedAt mismatch: got %v, want %d", decoded.EndedAt, endedAt)
	}

	// Test nil EndedAt
	runNil := IRunInfo{
		ID:             "run-2",
		PID:            12346,
		HarnessVersion: "1.0.0",
		StartedAt:      1700000000000,
		EndedAt:        nil,
	}
	nilData, err := json.Marshal(runNil)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decodedNil IRunInfo
	if err := json.Unmarshal(nilData, &decodedNil); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decodedNil.EndedAt != nil {
		t.Fatalf("expected nil EndedAt, got %v", *decodedNil.EndedAt)
	}
}

func TestAgentActivityJSON(t *testing.T) {
	phase := PhaseEditing
	tool := "file_edit"
	target := "main.go"
	question := "Proceed with edit?"
	thinking := "Analyzing dependency impact..."
	lastErr := "file not found"

	cost := ICostInfo{
		Currency:    "USD",
		Total:       0.12,
		IsEstimated: false,
	}
	run := IRunInfo{
		ID:             "run-xyz",
		PID:            999,
		HarnessVersion: "0.2.0",
		StartedAt:      1700000000000,
	}

	completed := int64(1700000008000)
	activity := IAgentActivity{
		Lifecycle:           LifecycleRunning,
		Activity:            ActivityBusy,
		Title:               "Editing files",
		Action:              "Applying patch to main.go",
		ActionStale:         false,
		Phase:               &phase,
		ActiveTool:          &tool,
		ActiveTarget:        &target,
		Question:            &question,
		LastThinkingExcerpt: &thinking,
		LastError:           &lastErr,
		Usage: ITokenUsage{
			Input: 1000,
			Total: 1000,
		},
		ActiveSubagents: []ISubagentSummary{
			{
				ID:          "sub-1",
				Activity:    ActivityIdle,
				Status:      SubagentCompleted,
				StartedAt:   1700000002000,
				CompletedAt: &completed,
			},
		},
		Cost:           &cost,
		SubagentCount:  2,
		CurrentRun:     &run,
		FirstStartedAt: 1700000000000,
		UpdatedAt:      1700000010000,
	}

	data, err := json.Marshal(activity)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded IAgentActivity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(decoded.ActiveSubagents) != 1 {
		t.Fatalf("expected 1 ActiveSubagent, got %d", len(decoded.ActiveSubagents))
	}
	if decoded.ActiveSubagents[0].Status != SubagentCompleted {
		t.Fatalf("expected SubagentCompleted, got %v", decoded.ActiveSubagents[0].Status)
	}
	if decoded.ActiveSubagents[0].StartedAt != 1700000002000 {
		t.Fatalf("expected StartedAt 1700000002000, got %d", decoded.ActiveSubagents[0].StartedAt)
	}
	if decoded.ActiveSubagents[0].CompletedAt == nil || *decoded.ActiveSubagents[0].CompletedAt != completed {
		t.Fatalf("expected CompletedAt %d, got %v", completed, decoded.ActiveSubagents[0].CompletedAt)
	}

	if decoded.Lifecycle != LifecycleRunning || decoded.Activity != ActivityBusy {
		t.Fatalf("lifecycle/activity mismatch: %s / %s", decoded.Lifecycle, decoded.Activity)
	}
	if decoded.Phase == nil || *decoded.Phase != PhaseEditing {
		t.Fatalf("phase mismatch: got %v, want %s", decoded.Phase, PhaseEditing)
	}
	if decoded.ActiveTool == nil || *decoded.ActiveTool != tool {
		t.Fatalf("ActiveTool mismatch: got %v, want %s", decoded.ActiveTool, tool)
	}
	if decoded.ActiveTarget == nil || *decoded.ActiveTarget != target {
		t.Fatalf("ActiveTarget mismatch: got %v, want %s", decoded.ActiveTarget, target)
	}
	if decoded.Question == nil || *decoded.Question != question {
		t.Fatalf("Question mismatch: got %v, want %s", decoded.Question, question)
	}
	if decoded.LastThinkingExcerpt == nil || *decoded.LastThinkingExcerpt != thinking {
		t.Fatalf("LastThinkingExcerpt mismatch: got %v, want %s", decoded.LastThinkingExcerpt, thinking)
	}
	if decoded.LastError == nil || *decoded.LastError != lastErr {
		t.Fatalf("LastError mismatch: got %v, want %s", decoded.LastError, lastErr)
	}
	if decoded.Cost == nil || decoded.Cost.Total != cost.Total {
		t.Fatalf("Cost mismatch: got %v, want %+v", decoded.Cost, cost)
	}
	if decoded.CurrentRun == nil || decoded.CurrentRun.ID != run.ID {
		t.Fatalf("CurrentRun mismatch: got %v, want %+v", decoded.CurrentRun, run)
	}

	// Test zero values / nil optional pointers
	minimalActivity := IAgentActivity{
		Lifecycle:      LifecycleExited,
		Activity:       ActivityIdle,
		Title:          "Idle agent",
		FirstStartedAt: 1700000000000,
		UpdatedAt:      1700000000000,
	}
	minimalData, err := json.Marshal(minimalActivity)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decodedMinimal IAgentActivity
	if err := json.Unmarshal(minimalData, &decodedMinimal); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decodedMinimal.Phase != nil || decodedMinimal.ActiveTool != nil || decodedMinimal.Question != nil || decodedMinimal.LastError != nil || decodedMinimal.Cost != nil || decodedMinimal.CurrentRun != nil {
		t.Fatalf("expected nil for all optional fields, got %+v", decodedMinimal)
	}
}

func TestAgentStateJSON(t *testing.T) {
	hiddenAt := int64(1700000005000)
	agent := IAgentState{
		ID:        "env-1:claude-code:session-1",
		SessionID: "session-1",
		Harness:   HarnessClaudeCode,
		Environment: IEnvironmentInfo{
			ID:              "env-1",
			Label:           "local-dev",
			Hostname:        "dev-box",
			Platform:        "darwin",
			User:            "alice",
			IsStale:         false,
			LastHeartbeatAt: 1700000000000,
		},
		Project: IProjectInfo{
			ID:            "github.com/org/repo",
			Name:          "repo",
			CanonicalRoot: "/work/repo",
		},
		Workspace: IWorkspaceInfo{
			IsWorktree: false,
			Branch:     "main",
			IsDetached: false,
			CWD:        "/work/repo",
		},
		Activity: IAgentActivity{
			Lifecycle:      LifecycleRunning,
			Activity:       ActivityBusy,
			Title:          "Refactoring",
			FirstStartedAt: 1700000000000,
			UpdatedAt:      1700000005000,
		},
		IsHidden: true,
		HiddenAt: &hiddenAt,
	}

	data, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded IAgentState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !decoded.IsHidden {
		t.Fatalf("IsHidden = false, want true")
	}
	if decoded.HiddenAt == nil || *decoded.HiddenAt != hiddenAt {
		t.Fatalf("HiddenAt = %v, want %d", decoded.HiddenAt, hiddenAt)
	}
}

func TestTelemetryFrameJSON(t *testing.T) {
	subagentID := "sub-1"
	role := "assistant"
	content := "Running tests"
	isError := false
	exitCode := 0

	frame := ITelemetryFrame{
		Version: "1.0",
		Seq:     42,
		EnvID:   "env-1",
		Environment: IEnvironmentInfo{
			ID:       "env-1",
			Hostname: "host1",
		},
		AgentID:        "env-1:claude-code:sess-1",
		SessionID:      "sess-1",
		SubagentID:     &subagentID,
		PID:            1001,
		Harness:        HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project: IProjectInfo{
			ID:   "p1",
			Name: "my-project",
		},
		Workspace: IWorkspaceInfo{
			Branch: "main",
			CWD:    "/path/to/project",
		},
		Timestamp: 1700000000000,
		EventType: "tool_use",
		Data: TelemetryFrameData{
			Role:     &role,
			Content:  &content,
			IsError:  &isError,
			ExitCode: &exitCode,
			ToolInput: map[string]interface{}{
				"command": "just test-all",
			},
		},
	}

	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ITelemetryFrame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Seq != frame.Seq || decoded.AgentID != frame.AgentID || decoded.Harness != HarnessClaudeCode {
		t.Fatalf("decoded frame mismatch: %+v", decoded)
	}
	if decoded.SubagentID == nil || *decoded.SubagentID != subagentID {
		t.Fatalf("SubagentID mismatch: got %v, want %s", decoded.SubagentID, subagentID)
	}
	if decoded.Data.Role == nil || *decoded.Data.Role != role {
		t.Fatalf("Role mismatch: got %v, want %s", decoded.Data.Role, role)
	}
	if decoded.Data.ToolInput["command"] != "just test-all" {
		t.Fatalf("ToolInput mismatch: got %v", decoded.Data.ToolInput)
	}

	// Verify omitempty behavior on TelemetryFrameData
	emptyData := TelemetryFrameData{}
	marshaledEmpty, err := json.Marshal(emptyData)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if string(marshaledEmpty) != "{}" {
		t.Fatalf("expected empty JSON object for TelemetryFrameData zero value, got %s", string(marshaledEmpty))
	}
}

func TestMessagesAndEventsJSON(t *testing.T) {
	t.Run("IObserverMessage", func(t *testing.T) {
		msg := IObserverMessage{
			Type:  "hello",
			EnvID: "env-local",
			Environment: &IEnvironmentInfo{
				ID:       "env-local",
				Hostname: "box",
			},
			Since:   100,
			Dropped: 0,
			Frames:  []ITelemetryFrame{},
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decoded IObserverMessage
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded.Type != "hello" || decoded.EnvID != "env-local" || decoded.Environment == nil {
			t.Fatalf("decoded message mismatch: %+v", decoded)
		}
	})

	t.Run("IIngesterMessage", func(t *testing.T) {
		msg := IIngesterMessage{
			Type:    "ack",
			UpTo:    42,
			Code:    "OK",
			Message: "success",
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decoded IIngesterMessage
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded.Type != "ack" || decoded.UpTo != 42 {
			t.Fatalf("decoded ingester message mismatch: %+v", decoded)
		}
	})

	t.Run("IServerEvent", func(t *testing.T) {
		evt := IServerEvent{
			Type:    "agent_upsert",
			At:      1700000000000,
			AgentID: "agent-1",
			Agent: &IAgentState{
				ID:        "agent-1",
				SessionID: "sess-1",
				Harness:   HarnessPi,
			},
		}
		data, err := json.Marshal(evt)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decoded IServerEvent
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded.Type != "agent_upsert" || decoded.Agent == nil || decoded.Agent.Harness != HarnessPi {
			t.Fatalf("decoded server event mismatch: %+v", decoded)
		}
	})

	t.Run("StatusResponse and ProjectGroup", func(t *testing.T) {
		status := StatusResponse{
			Status:                 "healthy",
			Environments:           []IEnvironmentInfo{{ID: "env-1"}},
			ActiveAgentCount:       3,
			RequiresAttentionCount: 1,
			TotalFleetSpend:        12.34,
		}
		data, err := json.Marshal(status)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decodedStatus StatusResponse
		if err := json.Unmarshal(data, &decodedStatus); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decodedStatus.ActiveAgentCount != 3 || decodedStatus.TotalFleetSpend != 12.34 {
			t.Fatalf("decoded status response mismatch: %+v", decodedStatus)
		}

		group := ProjectGroup{
			Project:          IProjectInfo{ID: "p1", Name: "agent-status"},
			Agents:           []IAgentState{},
			TotalCost:        5.67,
			ActiveAgentCount: 2,
		}
		groupData, err := json.Marshal(group)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decodedGroup ProjectGroup
		if err := json.Unmarshal(groupData, &decodedGroup); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decodedGroup.Project.Name != "agent-status" || decodedGroup.TotalCost != 5.67 {
			t.Fatalf("decoded project group mismatch: %+v", decodedGroup)
		}
	})

	t.Run("ContextWindow and ContextFullness serialization", func(t *testing.T) {
		model := "claude-3-7-sonnet"
		cw := int64(200000)
		fullness := 0.35
		activity := IAgentActivity{
			Lifecycle:       LifecycleRunning,
			Activity:        ActivityBusy,
			Model:           &model,
			ContextWindow:   &cw,
			ContextFullness: &fullness,
		}
		data, err := json.Marshal(activity)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decodedAct IAgentActivity
		if err := json.Unmarshal(data, &decodedAct); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decodedAct.Model == nil || *decodedAct.Model != model {
			t.Errorf("decoded model = %v, want %q", decodedAct.Model, model)
		}
		if decodedAct.ContextWindow == nil || *decodedAct.ContextWindow != cw {
			t.Errorf("decoded ContextWindow = %v, want %d", decodedAct.ContextWindow, cw)
		}
		if decodedAct.ContextFullness == nil || *decodedAct.ContextFullness != fullness {
			t.Errorf("decoded ContextFullness = %v, want %f", decodedAct.ContextFullness, fullness)
		}

		frameData := TelemetryFrameData{
			Model:              &model,
			ModelContextWindow: &cw,
			ContextFullness:    &fullness,
		}
		frameBytes, err := json.Marshal(frameData)
		if err != nil {
			t.Fatalf("json.Marshal frameData failed: %v", err)
		}
		var decodedFrame TelemetryFrameData
		if err := json.Unmarshal(frameBytes, &decodedFrame); err != nil {
			t.Fatalf("json.Unmarshal frameData failed: %v", err)
		}
		if decodedFrame.ModelContextWindow == nil || *decodedFrame.ModelContextWindow != cw {
			t.Errorf("decoded ModelContextWindow = %v, want %d", decodedFrame.ModelContextWindow, cw)
		}
		if decodedFrame.ContextFullness == nil || *decodedFrame.ContextFullness != fullness {
			t.Errorf("decoded ContextFullness = %v, want %f", decodedFrame.ContextFullness, fullness)
		}
	})

	t.Run("HasUnseenCompletion and CompletedAt serialization and unmarshaling", func(t *testing.T) {
		tTrue := true
		compAt := int64(1700000050000)
		act := IAgentActivity{
			Lifecycle:           LifecycleRunning,
			Activity:            ActivityIdle,
			HasUnseenCompletion: &tTrue,
			CompletedAt:         &compAt,
		}

		data, err := json.Marshal(act)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decoded IAgentActivity
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded.HasUnseenCompletion == nil || !*decoded.HasUnseenCompletion {
			t.Errorf("expected HasUnseenCompletion true, got %v", decoded.HasUnseenCompletion)
		}
		if decoded.CompletedAt == nil || *decoded.CompletedAt != compAt {
			t.Errorf("expected CompletedAt %d, got %v", compAt, decoded.CompletedAt)
		}

		// Also verify snake_case deserialization
		snakeJSON := `{"lifecycle":"RUNNING","activity":"IDLE","has_unseen_completion":true,"completed_at":1700000050000}`
		var decodedSnake IAgentActivity
		if err := json.Unmarshal([]byte(snakeJSON), &decodedSnake); err != nil {
			t.Fatalf("json.Unmarshal snake_case failed: %v", err)
		}
		if decodedSnake.HasUnseenCompletion == nil || !*decodedSnake.HasUnseenCompletion {
			t.Errorf("expected HasUnseenCompletion true from snake_case, got %v", decodedSnake.HasUnseenCompletion)
		}
		if decodedSnake.CompletedAt == nil || *decodedSnake.CompletedAt != compAt {
			t.Errorf("expected CompletedAt %d from snake_case, got %v", compAt, decodedSnake.CompletedAt)
		}

		// When both camelCase and snake_case are provided, camelCase takes precedence
		bothJSON := `{"lifecycle":"RUNNING","activity":"IDLE","hasUnseenCompletion":false,"has_unseen_completion":true,"completedAt":111,"completed_at":222}`
		var decodedBoth IAgentActivity
		if err := json.Unmarshal([]byte(bothJSON), &decodedBoth); err != nil {
			t.Fatalf("json.Unmarshal both failed: %v", err)
		}
		if decodedBoth.HasUnseenCompletion == nil || *decodedBoth.HasUnseenCompletion != false {
			t.Errorf("expected camelCase false to win, got %v", decodedBoth.HasUnseenCompletion)
		}
		if decodedBoth.CompletedAt == nil || *decodedBoth.CompletedAt != 111 {
			t.Errorf("expected camelCase 111 to win, got %v", decodedBoth.CompletedAt)
		}

		// Verify RateLimitResetAt and RetryAfterSeconds serialization and snake_case deserialization
		resetAt := int64(1700000060000)
		retrySec := int64(30)
		actRL := IAgentActivity{
			Lifecycle:         LifecycleRunning,
			Activity:          ActivityRateLimited,
			RateLimitResetAt:  &resetAt,
			RetryAfterSeconds: &retrySec,
		}
		dataRL, err := json.Marshal(actRL)
		if err != nil {
			t.Fatalf("json.Marshal rate limit failed: %v", err)
		}
		var decodedRL IAgentActivity
		if err := json.Unmarshal(dataRL, &decodedRL); err != nil {
			t.Fatalf("json.Unmarshal rate limit failed: %v", err)
		}
		if decodedRL.RateLimitResetAt == nil || *decodedRL.RateLimitResetAt != resetAt {
			t.Errorf("expected RateLimitResetAt %d, got %v", resetAt, decodedRL.RateLimitResetAt)
		}
		if decodedRL.RetryAfterSeconds == nil || *decodedRL.RetryAfterSeconds != retrySec {
			t.Errorf("expected RetryAfterSeconds %d, got %v", retrySec, decodedRL.RetryAfterSeconds)
		}

		snakeRLJSON := `{"lifecycle":"RUNNING","activity":"RATE_LIMITED","rate_limit_reset_at":1700000060000,"retry_after_seconds":30}`
		var decodedSnakeRL IAgentActivity
		if err := json.Unmarshal([]byte(snakeRLJSON), &decodedSnakeRL); err != nil {
			t.Fatalf("json.Unmarshal snake_case rate limit failed: %v", err)
		}
		if decodedSnakeRL.RateLimitResetAt == nil || *decodedSnakeRL.RateLimitResetAt != resetAt {
			t.Errorf("expected RateLimitResetAt %d from snake_case, got %v", resetAt, decodedSnakeRL.RateLimitResetAt)
		}
		if decodedSnakeRL.RetryAfterSeconds == nil || *decodedSnakeRL.RetryAfterSeconds != retrySec {
			t.Errorf("expected RetryAfterSeconds %d from snake_case, got %v", retrySec, decodedSnakeRL.RetryAfterSeconds)
		}

		// When both camelCase and snake_case are provided, camelCase takes precedence
		bothRLJSON := `{"lifecycle":"RUNNING","activity":"RATE_LIMITED","rateLimitResetAt":100,"rate_limit_reset_at":200,"retryAfterSeconds":10,"retry_after_seconds":20}`
		var decodedBothRL IAgentActivity
		if err := json.Unmarshal([]byte(bothRLJSON), &decodedBothRL); err != nil {
			t.Fatalf("json.Unmarshal both RL failed: %v", err)
		}
		if decodedBothRL.RateLimitResetAt == nil || *decodedBothRL.RateLimitResetAt != 100 {
			t.Errorf("expected camelCase 100 to win, got %v", decodedBothRL.RateLimitResetAt)
		}
		if decodedBothRL.RetryAfterSeconds == nil || *decodedBothRL.RetryAfterSeconds != 10 {
			t.Errorf("expected camelCase 10 to win, got %v", decodedBothRL.RetryAfterSeconds)
		}

		var invalid IAgentActivity
		if err := json.Unmarshal([]byte(`{"lifecycle": 123}`), &invalid); err == nil {
			t.Errorf("expected error unmarshaling invalid json")
		}
	})
}
