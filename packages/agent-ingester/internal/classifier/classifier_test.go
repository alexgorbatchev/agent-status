package classifier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
)

func newTestClassifier(t *testing.T, sum *summarizer.Summarizer) (*Classifier, *store.Store) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	p, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("pricing.NewPricingEngine() failed: %v", err)
	}

	c := New(s, p, sum)
	c.SetIdleGracePeriod(0)
	return c, s
}

func TestClassifier_LifecycleAndActivityTransitions(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)

	env := types.IEnvironmentInfo{
		ID:              "env-1",
		Label:           "work-macbook",
		Hostname:        "macbook",
		Platform:        "darwin",
		User:            "alex",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{
		Branch: "feat/foo",
		CWD:    "/Users/alex/dev/repo",
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/Users/alex/dev/repo",
	}

	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-1",
		Environment:    env,
		AgentID:        "env-1:claude-code:session-1",
		SessionID:      "session-1",
		PID:            12345,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.1.277",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
	}

	// 1. run_started (with timestamp = 0 to test fallback to now)
	f1 := baseFrame
	f1.Timestamp = 0
	f1.EventType = "run_started"
	title := "Implementing Feature X"
	f1.Data = types.TelemetryFrameData{
		Title: &title,
	}
	agent, err := c.ProcessFrame(f1)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) error = %v", err)
	}
	if agent.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("got lifecycle %s, want %s", agent.Activity.Lifecycle, types.LifecycleRunning)
	}
	if agent.Activity.Activity != types.ActivityIdle {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityIdle)
	}
	if agent.Activity.Title != "Implementing Feature X" {
		t.Errorf("got title %q, want 'Implementing Feature X'", agent.Activity.Title)
	}
	if agent.Activity.CurrentRun == nil || agent.Activity.CurrentRun.PID != 12345 {
		t.Errorf("currentRun PID mismatch: %+v", agent.Activity.CurrentRun)
	}

	// 2. turn_start
	fTurnStart := baseFrame
	fTurnStart.Seq = 2
	fTurnStart.EventType = "turn_start"
	agent, err = c.ProcessFrame(fTurnStart)
	if err != nil {
		t.Fatalf("ProcessFrame(turn_start) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityBusy)
	}

	// 3. thinking
	thinkingContent := "Analyzing code structure and dependencies"
	fThinking := baseFrame
	fThinking.Seq = 3
	fThinking.EventType = "thinking"
	fThinking.Data = types.TelemetryFrameData{
		Content: &thinkingContent,
	}
	agent, err = c.ProcessFrame(fThinking)
	if err != nil {
		t.Fatalf("ProcessFrame(thinking) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityBusy)
	}
	if agent.Activity.LastThinkingExcerpt == nil || *agent.Activity.LastThinkingExcerpt != thinkingContent {
		t.Errorf("got thinking %v, want %s", agent.Activity.LastThinkingExcerpt, thinkingContent)
	}

	// 4. question_prompt
	qPrompt := "Confirm deployment?"
	fQPrompt := baseFrame
	fQPrompt.Seq = 4
	fQPrompt.EventType = "question_prompt"
	fQPrompt.Data = types.TelemetryFrameData{
		Question: &qPrompt,
	}
	agent, err = c.ProcessFrame(fQPrompt)
	if err != nil {
		t.Fatalf("ProcessFrame(question_prompt) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityWaitingQuestion)
	}
	if agent.Activity.Question == nil || *agent.Activity.Question != qPrompt {
		t.Errorf("got question %v, want %s", agent.Activity.Question, qPrompt)
	}

	// 5. error event with long content truncation
	longError := strings.Repeat("E", 3000)
	fErr := baseFrame
	fErr.Seq = 5
	fErr.EventType = "error"
	fErr.Data = types.TelemetryFrameData{
		Content: &longError,
	}
	agent, err = c.ProcessFrame(fErr)
	if err != nil {
		t.Fatalf("ProcessFrame(error) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityError {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityError)
	}
	if agent.Activity.LastError == nil || len(*agent.Activity.LastError) > 2000 {
		t.Errorf("expected LastError truncated to <= 2000 chars, got len %d", len(*agent.Activity.LastError))
	}

	// 6. tool_call -> Bash -> BUSY with ActiveTool and ActiveTarget
	cmd := "npm test"
	toolName := "Bash"
	f2 := baseFrame
	f2.Seq = 6
	f2.EventType = "tool_call"
	f2.Data = types.TelemetryFrameData{
		ToolName:  &toolName,
		ToolInput: map[string]interface{}{"command": cmd},
	}
	agent, err = c.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityBusy)
	}
	if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != toolName {
		t.Fatalf("expected ActiveTool %q, got %v", toolName, agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget == nil || *agent.Activity.ActiveTarget != cmd {
		t.Fatalf("expected ActiveTarget %q, got %v", cmd, agent.Activity.ActiveTarget)
	}

	// 7. tool_result with isError = true -> Remains Busy, clears ActiveTool and ActiveTarget
	errMsg := "permission denied: /etc/hosts"
	isErr := true
	f3 := baseFrame
	f3.Seq = 7
	f3.EventType = "tool_result"
	f3.Data = types.TelemetryFrameData{
		IsError:    &isErr,
		ToolOutput: &errMsg,
	}
	agent, err = c.ProcessFrame(f3)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_result error) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("got activity %s, want %s (non-zero tool exits must not flip activity to ERROR)", agent.Activity.Activity, types.ActivityBusy)
	}
	if agent.Activity.LastError != nil {
		t.Errorf("got lastError %v, want nil (tool error output must not be recorded as LastError)", *agent.Activity.LastError)
	}
	if agent.Activity.ActiveTool != nil {
		t.Errorf("expected ActiveTool nil after tool_result with error, got %q", *agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget != nil {
		t.Errorf("expected ActiveTarget nil after tool_result with error, got %q", *agent.Activity.ActiveTarget)
	}

	// 7b. tool_call -> Edit -> BUSY with ActiveTool and ActiveTarget
	editPath := "src/index.ts"
	editTool := "Edit"
	f2b := baseFrame
	f2b.Seq = 8
	f2b.EventType = "tool_call"
	f2b.Data = types.TelemetryFrameData{
		ToolName:  &editTool,
		ToolInput: map[string]interface{}{"path": editPath},
	}
	agent, err = c.ProcessFrame(f2b)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call edit) error = %v", err)
	}
	if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != editTool {
		t.Fatalf("expected ActiveTool %q, got %v", editTool, agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget == nil || *agent.Activity.ActiveTarget != editPath {
		t.Fatalf("expected ActiveTarget %q, got %v", editPath, agent.Activity.ActiveTarget)
	}

	// 8. tool_result with isError = false -> Busy, clears ActiveTool and ActiveTarget
	isErrFalse := false
	okOut := "success"
	fToolResOk := baseFrame
	fToolResOk.Seq = 9
	fToolResOk.EventType = "tool_result"
	fToolResOk.Data = types.TelemetryFrameData{
		IsError:    &isErrFalse,
		ToolOutput: &okOut,
	}
	agent, err = c.ProcessFrame(fToolResOk)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_result ok) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityBusy)
	}
	if agent.Activity.ActiveTool != nil {
		t.Errorf("expected ActiveTool nil after tool_result ok, got %q", *agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget != nil {
		t.Errorf("expected ActiveTarget nil after tool_result ok, got %q", *agent.Activity.ActiveTarget)
	}

	// 9. turn_end -> IDLE, clears question, active tool and target
	f4 := baseFrame
	f4.Seq = 9
	f4.EventType = "turn_end"
	agent, err = c.ProcessFrame(f4)
	if err != nil {
		t.Fatalf("ProcessFrame(turn_end) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityIdle {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityIdle)
	}
	if agent.Activity.Question != nil || agent.Activity.ActiveTool != nil || agent.Activity.ActiveTarget != nil {
		t.Errorf("expected tool and question fields cleared, got %+v", agent.Activity)
	}

	// 10. Usage & Cost accumulation
	model := "claude-opus-5"
	f5 := baseFrame
	f5.Seq = 10
	f5.EventType = "turn_end"
	f5.Data = types.TelemetryFrameData{
		Model: &model,
		Usage: &types.ITokenUsage{
			Input:      1000,
			Output:     200,
			CacheRead:  5000,
			CacheWrite: 1000,
			Reasoning:  100,
			Total:      7300,
		},
	}
	agent, err = c.ProcessFrame(f5)
	if err != nil {
		t.Fatalf("ProcessFrame(turn_end with usage) error = %v", err)
	}
	if agent.Activity.Usage.Input != 1000 || agent.Activity.Usage.Total != 7300 {
		t.Errorf("usage mismatch: %+v", agent.Activity.Usage)
	}
	if agent.Activity.Cost == nil || agent.Activity.Cost.Total <= 0 {
		t.Errorf("cost calculation failed: %+v", agent.Activity.Cost)
	}

	// 11. Subagent usage roll-up and count increment
	subagentID := "sub-1"
	f6 := baseFrame
	f6.Seq = 11
	f6.SubagentID = &subagentID
	f6.EventType = "turn_end"
	f6.Data = types.TelemetryFrameData{
		Model: &model,
		Usage: &types.ITokenUsage{
			Input:      500,
			Output:     100,
			CacheRead:  2500,
			CacheWrite: 500,
			Total:      3600,
		},
	}
	agent, err = c.ProcessFrame(f6)
	if err != nil {
		t.Fatalf("ProcessFrame(subagent usage) error = %v", err)
	}
	if agent.Activity.Usage.Input != 1500 {
		t.Errorf("expected rolled-up input usage 1500, got %d", agent.Activity.Usage.Input)
	}
	if agent.Activity.SubagentCount != 1 {
		t.Errorf("expected subagent_count = 1, got %d", agent.Activity.SubagentCount)
	}

	// Frame from the same subagent should NOT increment SubagentCount
	bashTool := "bash"
	f6b := baseFrame
	f6b.Seq = 12
	f6b.SubagentID = &subagentID
	f6b.EventType = "tool_call"
	f6b.Data = types.TelemetryFrameData{
		ToolName: &bashTool,
	}
	agent, err = c.ProcessFrame(f6b)
	if err != nil {
		t.Fatalf("ProcessFrame(subagent second frame) error = %v", err)
	}
	if agent.Activity.SubagentCount != 1 {
		t.Errorf("expected subagent_count = 1 after second frame from same subagent, got %d", agent.Activity.SubagentCount)
	}

	// Frame from a distinct subagent SHOULD increment SubagentCount to 2
	subagent2ID := "sub-2"
	f6c := baseFrame
	f6c.Seq = 13
	f6c.SubagentID = &subagent2ID
	f6c.EventType = "turn_start"
	agent, err = c.ProcessFrame(f6c)
	if err != nil {
		t.Fatalf("ProcessFrame(subagent-2 first frame) error = %v", err)
	}
	if agent.Activity.SubagentCount != 2 {
		t.Errorf("expected subagent_count = 2 after new subagent, got %d", agent.Activity.SubagentCount)
	}

	// Another frame from subagent 2 should NOT increment SubagentCount
	f6d := baseFrame
	f6d.Seq = 14
	f6d.SubagentID = &subagent2ID
	f6d.EventType = "turn_end"
	agent, err = c.ProcessFrame(f6d)
	if err != nil {
		t.Fatalf("ProcessFrame(subagent-2 second frame) error = %v", err)
	}
	if agent.Activity.SubagentCount != 2 {
		t.Errorf("expected subagent_count = 2 after second frame from subagent-2, got %d", agent.Activity.SubagentCount)
	}

	// 12. run_exited -> EXITED
	f7 := baseFrame
	f7.Seq = 15
	f7.EventType = "run_exited"
	agent, err = c.ProcessFrame(f7)
	if err != nil {
		t.Fatalf("ProcessFrame(run_exited) error = %v", err)
	}
	if agent.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("got lifecycle %s, want %s", agent.Activity.Lifecycle, types.LifecycleExited)
	}

	reloaded, err := s.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("s.GetAgent() error = %v", err)
	}
	if reloaded.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("reloaded lifecycle mismatch: %s", reloaded.Activity.Lifecycle)
	}
}

func TestExtractTarget(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		input      map[string]interface{}
		wantTarget string
	}{
		{
			name:       "nil input",
			tool:       "Edit",
			input:      nil,
			wantTarget: "",
		},
		{
			name: "file_path key",
			tool: "Edit",
			input: map[string]interface{}{
				"file_path": "/path/to/file.ts",
			},
			wantTarget: "/path/to/file.ts",
		},
		{
			name: "path key",
			tool: "Read",
			input: map[string]interface{}{
				"path": "docs/architecture.md",
			},
			wantTarget: "docs/architecture.md",
		},
		{
			name: "notebook_path key",
			tool: "NotebookEdit",
			input: map[string]interface{}{
				"notebook_path": "/notebooks/test.ipynb",
			},
			wantTarget: "/notebooks/test.ipynb",
		},
		{
			name: "description key",
			tool: "Bash",
			input: map[string]interface{}{
				"description": "Run integration tests",
			},
			wantTarget: "Run integration tests",
		},
		{
			name: "command key",
			tool: "Bash",
			input: map[string]interface{}{
				"command": "npm run build",
			},
			wantTarget: "npm run build",
		},
		{
			name: "url key",
			tool: "WebFetch",
			input: map[string]interface{}{
				"url": "https://example.com/api",
			},
			wantTarget: "https://example.com/api",
		},
		{
			name: "query key",
			tool: "WebSearch",
			input: map[string]interface{}{
				"query": "sqlite wal pragma",
			},
			wantTarget: "sqlite wal pragma",
		},
		{
			name: "skill key",
			tool: "Skill",
			input: map[string]interface{}{
				"skill": "golang",
			},
			wantTarget: "golang",
		},
		{
			name: "pattern key",
			tool: "grep",
			input: map[string]interface{}{
				"pattern": "func ProcessFrame",
			},
			wantTarget: "func ProcessFrame",
		},
		{
			name: "prompt key",
			tool: "claude-api",
			input: map[string]interface{}{
				"prompt": "explain this function",
			},
			wantTarget: "explain this function",
		},
		{
			name: "empty values ignored",
			tool: "Custom",
			input: map[string]interface{}{
				"file_path": "   ",
				"command":   "valid command",
			},
			wantTarget: "valid command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTarget(tt.tool, tt.input)
			if got != tt.wantTarget {
				t.Errorf("extractTarget() = %q, want %q", got, tt.wantTarget)
			}
		})
	}
}

func TestClassifier_WithLiveLLMSummarizer(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-sum-1",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Fixing TypeScript compilation errors in build.ts","phase":"debugging","activeTarget":"packages/registry-cli/src/commands/buildCommand.ts"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 10 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	var updatedAgent *types.IAgentState
	c.SetOnUpdate(func(a *types.IAgentState) {
		updatedAgent = a
	})

	env := types.IEnvironmentInfo{
		ID:              "env-test",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{
		Branch: "main",
		CWD:    "/workspace",
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}

	toolName := "Edit"
	prompt := "Please fix the build error in build.ts"
	f := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-test",
		Environment:    env,
		AgentID:        "env-test:claude-code:session-sum",
		SessionID:      "session-sum",
		PID:            9999,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.1.277",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      2000,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			LastPrompt: &prompt,
			ToolName:   &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "packages/registry-cli/src/commands/buildCommand.ts",
			},
		},
	}

	// 1. ProcessFrame returns immediately with non-blocking deterministic fallback summary
	agent, err := c.ProcessFrame(f)
	if err != nil {
		t.Fatalf("ProcessFrame() failed: %v", err)
	}
	if agent.Activity.Action != "Editing packages/registry-cli/src/commands/buildCommand.ts" {
		t.Errorf("got initial immediate Action %q, want 'Editing packages/registry-cli/src/commands/buildCommand.ts'", agent.Activity.Action)
	}

	// 2. Wait for async queue to finish
	c.Flush(agent.ID)

	// 3. Verify updated state in store and onUpdate notification
	reloaded, err := s.GetAgent(agent.ID)
	if err != nil || reloaded == nil {
		t.Fatalf("s.GetAgent() failed: %v", err)
	}

	if reloaded.Activity.Action != "Fixing TypeScript compilation errors in build.ts" {
		t.Errorf("got Action %q, want 'Fixing TypeScript compilation errors in build.ts'", reloaded.Activity.Action)
	}
	if reloaded.Activity.Phase == nil || *reloaded.Activity.Phase != types.PhaseDebugging {
		t.Errorf("got Phase %v, want debugging", reloaded.Activity.Phase)
	}
	if reloaded.Activity.ActiveTarget == nil || *reloaded.Activity.ActiveTarget != "packages/registry-cli/src/commands/buildCommand.ts" {
		t.Errorf("got ActiveTarget %v, want 'packages/registry-cli/src/commands/buildCommand.ts'", reloaded.Activity.ActiveTarget)
	}
	if reloaded.Activity.ActionStale {
		t.Errorf("expected ActionStale = false after successful summarization")
	}

	if updatedAgent == nil || updatedAgent.Activity.Action != "Fixing TypeScript compilation errors in build.ts" {
		t.Errorf("expected onUpdate callback to receive updated agent, got %+v", updatedAgent)
	}
}

func TestClassifier_ContinuousActivityDebounceAndStaleness(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-sum-2",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Refactoring database models","phase":"editing","activeTarget":"db/models.go"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 80 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{
		Branch: "main",
		CWD:    "/workspace",
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}

	toolName := "Edit"
	prompt := "Refactor models in db/models.go"

	f1 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-debounce",
		Environment:    env,
		AgentID:        "env-debounce:claude-code:session-deb",
		SessionID:      "session-deb",
		PID:            8888,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.1.277",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			LastPrompt: &prompt,
			ToolName:   &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "db/models.go",
			},
		},
	}
	agent, err := c.ProcessFrame(f1)
	if err != nil {
		t.Fatalf("ProcessFrame(f1) failed: %v", err)
	}
	if agent.Activity.Action != "Refactoring database models" {
		t.Errorf("got Action %q, want 'Refactoring database models'", agent.Activity.Action)
	}
	if agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = false after first summary")
	}

	// 2. Continuous tool call within debounce window -> rate limited, marked ActionStale = true
	f2 := f1
	f2.Seq = 2
	f2.Timestamp = 1010
	f2.Data.ToolInput = map[string]interface{}{
		"file_path": "db/schema.sql",
	}
	agent, err = c.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame(f2) failed: %v", err)
	}
	if !agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = true for tool call within debounce window")
	}

	// 3. Force trigger within debounce window (e.g. turn_end) -> uses fallback summary and resets ActionStale to false
	fEnd := f1
	fEnd.Seq = 3
	fEnd.Timestamp = 1020
	fEnd.EventType = "turn_end"
	fEnd.Data = types.TelemetryFrameData{}
	agent, err = c.ProcessFrame(fEnd)
	if err != nil {
		t.Fatalf("ProcessFrame(fEnd) failed: %v", err)
	}
	if agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = false after turn_end forced fallback")
	}

	// Wait for debounce window to elapse
	time.Sleep(100 * time.Millisecond)

	// Fresh tool call triggers new LLM summary
	f3 := f1
	f3.Seq = 4
	f3.Timestamp = 1200
	agent, err = c.ProcessFrame(f3)
	if err != nil {
		t.Fatalf("ProcessFrame(f3) failed: %v", err)
	}
	if agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = false after debounce window elapsed")
	}
}

func TestClassifier_BatchedFrames_TimestampDebounce(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-sum-batch",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Running tests","phase":"testing","activeTarget":"./..."}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 10 * time.Second,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		Label:    "devbox",
		Hostname: "devbox",
		Platform: "linux",
		User:     "testuser",
	}
	ws := types.IWorkspaceInfo{
		Branch: "main",
		CWD:    "/workspace",
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}

	editTool := "Edit"
	f1 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-batched",
		Environment:    env,
		AgentID:        "env-batched:claude-code:session-batched",
		SessionID:      "session-batched",
		PID:            7777,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.1.277",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &editTool,
			ToolInput: map[string]interface{}{
				"file_path": "main.go",
			},
		},
	}

	agent, err := c.ProcessFrame(f1)
	if err != nil {
		t.Fatalf("ProcessFrame(f1) failed: %v", err)
	}
	if agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = false for initial frame")
	}

	// Frame 2 arrives immediately on wall clock (<1ms), but timestamp is 20s later (>10s debounce)
	bashTool := "bash"
	f2 := f1
	f2.Seq = 2
	f2.Timestamp = 21000 // 20 seconds later
	f2.Data = types.TelemetryFrameData{
		ToolName: &bashTool,
		ToolInput: map[string]interface{}{
			"command": "go test ./...",
		},
	}

	agent, err = c.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame(f2) failed: %v", err)
	}
	// Without fix: Allow checks time.Now() (wall clock hasn't advanced), so ActionStale becomes true
	// With fix: AllowAt checks event timestamp (20s > 10s debounce), so ActionStale is false
	if agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = false for batched frame spaced 20s in event time")
	}

	// Frame 3 arrives immediately on wall clock, and timestamp is only 1s later (<10s debounce)
	grepTool := "grep"
	f3 := f1
	f3.Seq = 3
	f3.Timestamp = 22000 // 1 second later (within debounce)
	f3.Data = types.TelemetryFrameData{
		ToolName: &grepTool,
		ToolInput: map[string]interface{}{
			"pattern": "Test",
			"path":    "main.go",
		},
	}

	agent, err = c.ProcessFrame(f3)
	if err != nil {
		t.Fatalf("ProcessFrame(f3) failed: %v", err)
	}
	// ActionStale should be true because 1s < 10s debounce window
	if !agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = true for frame within debounce window")
	}
	// But Action MUST still be immediately updated with the deterministic fallback summary!
	// Without fix: Action is not updated when Action != "" and forceTrigger == false
	// With fix: Action is always updated to fb.Action
	expectedFallback := "Searching main.go"
	if agent.Activity.Action != expectedFallback {
		t.Errorf("expected Action to update immediately to fallback %q, got %q", expectedFallback, agent.Activity.Action)
	}
}

func TestClassifier_DatabaseErrorHandling(t *testing.T) {
	sum, _ := summarizer.New(summarizer.Config{NoSummarize: true})
	c, s := newTestClassifier(t, sum)

	// Close store to trigger database error in ProcessFrame
	_ = s.Close()

	frame := types.ITelemetryFrame{
		Version:   "1.0",
		Seq:       1,
		AgentID:   "env-err:claude-code:sess-err",
		SessionID: "sess-err",
		Harness:   types.HarnessClaudeCode,
		Timestamp: 1000,
		EventType: "run_started",
	}

	_, err := c.ProcessFrame(frame)
	if err == nil {
		t.Errorf("expected error from ProcessFrame when store is closed, got nil")
	}
}

func TestClassifier_UsageAccumulation_ReasoningTokens(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)

	env := types.IEnvironmentInfo{
		ID:              "env-usage",
		Label:           "work-macbook",
		Hostname:        "macbook",
		Platform:        "darwin",
		User:            "alex",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{
		Branch: "main",
		CWD:    "/Users/alex/dev/repo",
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/Users/alex/dev/repo",
	}

	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-usage",
		Environment:    env,
		AgentID:        "env-usage:claude-code:session-usage",
		SessionID:      "session-usage",
		PID:            12345,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.1.277",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "run_started",
	}

	// 1. Initial frame to start session
	if _, err := c.ProcessFrame(baseFrame); err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}

	// 2. Turn 1: Frame with explicit Total (u.Total > 0)
	model := "claude-3-7-sonnet"
	fTurn1 := baseFrame
	fTurn1.Seq = 2
	fTurn1.Timestamp = 2000
	fTurn1.EventType = "turn_end"
	fTurn1.Data = types.TelemetryFrameData{
		Model: &model,
		Usage: &types.ITokenUsage{
			Input:      1000,
			Output:     500, // already includes 300 reasoning tokens
			CacheRead:  2000,
			CacheWrite: 100,
			Reasoning:  300,
			Total:      3600, // 1000 + 500 + 2000 + 100
		},
	}

	agent, err := c.ProcessFrame(fTurn1)
	if err != nil {
		t.Fatalf("ProcessFrame(turn 1) failed: %v", err)
	}

	if agent.Activity.Usage.Input != 1000 {
		t.Errorf("got Input %d, want 1000", agent.Activity.Usage.Input)
	}
	if agent.Activity.Usage.Output != 500 {
		t.Errorf("got Output %d, want 500", agent.Activity.Usage.Output)
	}
	if agent.Activity.Usage.Reasoning != 300 {
		t.Errorf("got Reasoning %d, want 300", agent.Activity.Usage.Reasoning)
	}
	if agent.Activity.Usage.CacheRead != 2000 {
		t.Errorf("got CacheRead %d, want 2000", agent.Activity.Usage.CacheRead)
	}
	if agent.Activity.Usage.CacheWrite != 100 {
		t.Errorf("got CacheWrite %d, want 100", agent.Activity.Usage.CacheWrite)
	}
	// Total must be exactly 3600, NOT 3600 + 300 = 3900
	if agent.Activity.Usage.Total != 3600 {
		t.Errorf("got Total %d, want 3600 (reasoning tokens double-counted)", agent.Activity.Usage.Total)
	}

	// 3. Turn 2: Frame with Total = 0 (tests fallback without double-counting reasoning)
	fTurn2 := baseFrame
	fTurn2.Seq = 3
	fTurn2.Timestamp = 3000
	fTurn2.EventType = "turn_end"
	fTurn2.Data = types.TelemetryFrameData{
		Model: &model,
		Usage: &types.ITokenUsage{
			Input:      500,
			Output:     250, // already includes 150 reasoning tokens
			CacheRead:  1000,
			CacheWrite: 50,
			Reasoning:  150,
			Total:      0, // fallback to Input + Output + CacheRead + CacheWrite = 1800
		},
	}

	agent, err = c.ProcessFrame(fTurn2)
	if err != nil {
		t.Fatalf("ProcessFrame(turn 2) failed: %v", err)
	}

	if agent.Activity.Usage.Input != 1500 {
		t.Errorf("got Input %d, want 1500", agent.Activity.Usage.Input)
	}
	if agent.Activity.Usage.Output != 750 {
		t.Errorf("got Output %d, want 750", agent.Activity.Usage.Output)
	}
	if agent.Activity.Usage.Reasoning != 450 {
		t.Errorf("got Reasoning %d, want 450", agent.Activity.Usage.Reasoning)
	}
	if agent.Activity.Usage.CacheRead != 3000 {
		t.Errorf("got CacheRead %d, want 3000", agent.Activity.Usage.CacheRead)
	}
	if agent.Activity.Usage.CacheWrite != 150 {
		t.Errorf("got CacheWrite %d, want 150", agent.Activity.Usage.CacheWrite)
	}
	// Total must be 3600 + 1800 = 5400, NOT 5400 + 150 = 5550
	if agent.Activity.Usage.Total != 5400 {
		t.Errorf("got Total %d, want 5400 (reasoning tokens double-counted on fallback)", agent.Activity.Usage.Total)
	}
}

func TestClassifier_ConversationalAssistantResponses(t *testing.T) {
	t.Run("conversational non-tool turn with fallback summarizer", func(t *testing.T) {
		sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
		if err != nil {
			t.Fatalf("summarizer.New() failed: %v", err)
		}

		c, _ := newTestClassifier(t, sum)

		env := types.IEnvironmentInfo{
			ID:              "env-chat",
			Label:           "work-macbook",
			Hostname:        "macbook",
			Platform:        "darwin",
			User:            "alex",
			LastHeartbeatAt: time.Now().UnixMilli(),
		}
		ws := types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/workspace",
		}
		proj := types.IProjectInfo{
			ID:            "github.com/org/repo",
			Name:          "repo",
			CanonicalRoot: "/workspace",
		}

		baseFrame := types.ITelemetryFrame{
			Version:        "1.0",
			Seq:            1,
			EnvID:          "env-chat",
			Environment:    env,
			AgentID:        "env-chat:claude-code:session-chat",
			SessionID:      "session-chat",
			PID:            12345,
			Harness:        types.HarnessClaudeCode,
			HarnessVersion: "2.1.277",
			Project:        proj,
			Workspace:      ws,
			Timestamp:      1000,
			EventType:      "run_started",
		}

		// 1. Session start
		if _, err := c.ProcessFrame(baseFrame); err != nil {
			t.Fatalf("ProcessFrame(run_started) failed: %v", err)
		}

		// 2. Turn 1: user asks a conversational question
		prompt := "How does the classifier process telemetry frames?"
		fTurnStart := baseFrame
		fTurnStart.Seq = 2
		fTurnStart.EventType = "turn_start"
		fTurnStart.Data = types.TelemetryFrameData{
			LastPrompt: &prompt,
		}
		if _, err := c.ProcessFrame(fTurnStart); err != nil {
			t.Fatalf("ProcessFrame(turn_start) failed: %v", err)
		}

		// 3. Turn 1: turn_end with assistant response text and no tools called
		respContent := "The classifier processes frames into agent states and summarizes actions."
		fTurnEnd := baseFrame
		fTurnEnd.Seq = 3
		fTurnEnd.EventType = "turn_end"
		fTurnEnd.Data = types.TelemetryFrameData{
			Content: &respContent,
		}
		agent, err := c.ProcessFrame(fTurnEnd)
		if err != nil {
			t.Fatalf("ProcessFrame(turn_end) failed: %v", err)
		}

		if agent.Activity.Activity != types.ActivityIdle {
			t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityIdle)
		}
		if agent.Activity.Action != "Responded to user" {
			t.Errorf("got Action %q, want 'Responded to user'", agent.Activity.Action)
		}
	})

	t.Run("conversational non-tool turn after tool turn clears stale tool history", func(t *testing.T) {
		sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
		if err != nil {
			t.Fatalf("summarizer.New() failed: %v", err)
		}

		c, _ := newTestClassifier(t, sum)

		env := types.IEnvironmentInfo{
			ID:              "env-multi",
			Label:           "work-macbook",
			Hostname:        "macbook",
			Platform:        "darwin",
			User:            "alex",
			LastHeartbeatAt: time.Now().UnixMilli(),
		}
		ws := types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/workspace",
		}
		proj := types.IProjectInfo{
			ID:            "github.com/org/repo",
			Name:          "repo",
			CanonicalRoot: "/workspace",
		}

		baseFrame := types.ITelemetryFrame{
			Version:        "1.0",
			Seq:            1,
			EnvID:          "env-multi",
			Environment:    env,
			AgentID:        "env-multi:claude-code:session-multi",
			SessionID:      "session-multi",
			PID:            12345,
			Harness:        types.HarnessClaudeCode,
			HarnessVersion: "2.1.277",
			Project:        proj,
			Workspace:      ws,
			Timestamp:      1000,
			EventType:      "run_started",
		}

		// 1. Session start
		if _, err := c.ProcessFrame(baseFrame); err != nil {
			t.Fatalf("ProcessFrame(run_started) failed: %v", err)
		}

		// 2. Turn 1: Tool execution
		prompt1 := "Edit main.go"
		toolName := "Edit"
		fTool := baseFrame
		fTool.Seq = 2
		fTool.EventType = "tool_call"
		fTool.Data = types.TelemetryFrameData{
			LastPrompt: &prompt1,
			ToolName:   &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "main.go",
			},
		}
		agent, err := c.ProcessFrame(fTool)
		if err != nil {
			t.Fatalf("ProcessFrame(tool_call) failed: %v", err)
		}
		if agent.Activity.Action != "Editing main.go" {
			t.Errorf("got Action %q, want 'Editing main.go'", agent.Activity.Action)
		}

		// End of Turn 1
		stopReasonTool := "tool_use"
		fToolEnd := baseFrame
		fToolEnd.Seq = 3
		fToolEnd.EventType = "turn_end"
		fToolEnd.Data = types.TelemetryFrameData{
			StopReason: &stopReasonTool,
		}
		if _, err := c.ProcessFrame(fToolEnd); err != nil {
			t.Fatalf("ProcessFrame(turn_end) failed: %v", err)
		}

		// 3. Turn 2: User asks follow-up question
		prompt2 := "Can you explain what you changed?"
		fTurn2Start := baseFrame
		fTurn2Start.Seq = 4
		fTurn2Start.EventType = "turn_start"
		fTurn2Start.Data = types.TelemetryFrameData{
			LastPrompt: &prompt2,
		}
		if _, err := c.ProcessFrame(fTurn2Start); err != nil {
			t.Fatalf("ProcessFrame(turn_start) failed: %v", err)
		}

		// 4. Turn 2: Assistant responds directly without tools
		respContent := "I updated the error handling logic in main.go."
		fTurn2End := baseFrame
		fTurn2End.Seq = 5
		fTurn2End.EventType = "turn_end"
		fTurn2End.Data = types.TelemetryFrameData{
			Content: &respContent,
		}
		agent, err = c.ProcessFrame(fTurn2End)
		if err != nil {
			t.Fatalf("ProcessFrame(turn_end) failed: %v", err)
		}

		if agent.Activity.Action != "Responded to user" {
			t.Errorf("got Action %q, want 'Responded to user' (not stale tool action)", agent.Activity.Action)
		}
	})

	t.Run("conversational non-tool turn with LLM summarizer", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{
				"id":     "chatcmpl-sum-chat",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `{"action":"Explaining classifier telemetry architecture","phase":"investigating","activeTarget":""}`,
						},
						"finish_reason": "stop",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		t.Cleanup(mockServer.Close)

		p, err := openai.New(
			anyllm.WithBaseURL(mockServer.URL+"/v1"),
			anyllm.WithAPIKey("test-key"),
		)
		if err != nil {
			t.Fatalf("openai.New() failed: %v", err)
		}

		sum, err := summarizer.New(summarizer.Config{
			Providers: []summarizer.ProviderConfig{
				{
					Name:     "mock-openai",
					Provider: p,
					Model:    "gpt-4o-mini",
				},
			},
			DebounceWindow: 10 * time.Millisecond,
			Timeout:        2 * time.Second,
		})
		if err != nil {
			t.Fatalf("summarizer.New() failed: %v", err)
		}

		c, _ := newTestClassifier(t, sum)
		c.SetSyncMode(true)
		defer func() { _ = c.Close() }()

		env := types.IEnvironmentInfo{
			ID:              "env-llm-chat",
			Label:           "devbox",
			Hostname:        "devbox",
			Platform:        "linux",
			User:            "testuser",
			LastHeartbeatAt: time.Now().UnixMilli(),
		}
		ws := types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/workspace",
		}
		proj := types.IProjectInfo{
			ID:            "github.com/org/repo",
			Name:          "repo",
			CanonicalRoot: "/workspace",
		}

		prompt := "Describe the architecture of the system"
		respText := "The system is composed of an observer, ingester, and web dashboard."
		f := types.ITelemetryFrame{
			Version:        "1.0",
			Seq:            1,
			EnvID:          "env-llm-chat",
			Environment:    env,
			AgentID:        "env-llm-chat:claude-code:session-llm-chat",
			SessionID:      "session-llm-chat",
			PID:            12345,
			Harness:        types.HarnessClaudeCode,
			HarnessVersion: "2.1.277",
			Project:        proj,
			Workspace:      ws,
			Timestamp:      2000,
			EventType:      "turn_end",
			Data: types.TelemetryFrameData{
				LastPrompt: &prompt,
				Content:    &respText,
			},
		}

		agent, err := c.ProcessFrame(f)
		if err != nil {
			t.Fatalf("ProcessFrame() failed: %v", err)
		}

		if agent.Activity.Action != "Explaining classifier telemetry architecture" {
			t.Errorf("got Action %q, want 'Explaining classifier telemetry architecture'", agent.Activity.Action)
		}
		if agent.Activity.Phase == nil || *agent.Activity.Phase != types.PhaseInvestigating {
			t.Errorf("got Phase %v, want investigating", agent.Activity.Phase)
		}
	})
}

func TestClassifier_AsyncQueueSerializedPerAgent(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	var processedActions []string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		idx := callCount
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     fmt.Sprintf("chatcmpl-%d", idx),
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": fmt.Sprintf(`{"action":"Step %d completed","phase":"editing","activeTarget":"file%d.go"}`, idx, idx),
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 1 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	c.SetOnUpdate(func(a *types.IAgentState) {
		mu.Lock()
		processedActions = append(processedActions, a.Activity.Action)
		mu.Unlock()
	})

	env := types.IEnvironmentInfo{
		ID:              "env-async-order",
		Label:           "testbox",
		Hostname:        "testbox",
		Platform:        "linux",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}

	// Dispatch 3 sequential tool calls for same agent
	for i := 1; i <= 3; i++ {
		toolName := "Edit"
		f := types.ITelemetryFrame{
			Version:        "1.0",
			Seq:            int64(i),
			EnvID:          "env-async-order",
			Environment:    env,
			AgentID:        "env-async-order:claude:sess-order",
			SessionID:      "sess-order",
			PID:            100,
			Harness:        types.HarnessClaudeCode,
			HarnessVersion: "2.0.0",
			Project:        proj,
			Workspace:      ws,
			Timestamp:      int64(1000 + i*100),
			EventType:      "tool_call",
			Data: types.TelemetryFrameData{
				ToolName: &toolName,
				ToolInput: map[string]interface{}{
					"file_path": fmt.Sprintf("file%d.go", i),
				},
			},
		}
		agent, err := c.ProcessFrame(f)
		if err != nil {
			t.Fatalf("ProcessFrame(%d) failed: %v", i, err)
		}
		if agent.Activity.Action == "" {
			t.Errorf("immediate Action should not be empty on frame %d", i)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Flush the queue
	c.Flush("env-async-order:claude:sess-order")

	// Verify the latest summary is applied
	agent, err := s.GetAgent("env-async-order:claude:sess-order")
	if err != nil || agent == nil {
		t.Fatalf("failed to retrieve agent: %v", err)
	}
	if !strings.HasPrefix(agent.Activity.Action, "Step") {
		t.Errorf("expected final Action to start with 'Step', got %q", agent.Activity.Action)
	}
	if agent.Activity.Phase == nil || *agent.Activity.Phase != types.PhaseEditing {
		t.Errorf("expected Phase editing, got %v", agent.Activity.Phase)
	}
}

func TestClassifier_SemanticDeduplication(t *testing.T) {
	var mu sync.Mutex
	updateCount := 0
	currentResponse := `{"action":"Refactoring JWT authentication flow","phase":"editing","activeTarget":"auth.go"}`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		respJSON := currentResponse
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-dedup",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": respJSON,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 1 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	c.SetOnUpdate(func(a *types.IAgentState) {
		mu.Lock()
		updateCount++
		mu.Unlock()
	})

	env := types.IEnvironmentInfo{
		ID:              "env-dedup",
		Label:           "testbox",
		Hostname:        "testbox",
		Platform:        "linux",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	toolName := "Edit"

	// Frame 1: Initial distinct summary -> "Refactoring JWT authentication flow"
	f1 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-dedup",
		Environment:    env,
		AgentID:        "env-dedup:claude:sess-dedup",
		SessionID:      "sess-dedup",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "auth.go",
			},
		},
	}
	_, err = c.ProcessFrame(f1)
	if err != nil {
		t.Fatalf("ProcessFrame 1 failed: %v", err)
	}
	c.Flush("env-dedup:claude:sess-dedup")

	mu.Lock()
	firstCount := updateCount
	mu.Unlock()
	if firstCount != 1 {
		t.Fatalf("expected updateCount=1 after first summary, got %d", firstCount)
	}

	// Frame 2: Near-duplicate summary -> "Refactoring JWT authentication flow in auth.go" (>= 60% overlap, same target & phase)
	mu.Lock()
	currentResponse = `{"action":"Refactoring JWT authentication flow in auth.go","phase":"editing","activeTarget":"auth.go"}`
	mu.Unlock()
	time.Sleep(5 * time.Millisecond)

	f2 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            2,
		EnvID:          "env-dedup",
		Environment:    env,
		AgentID:        "env-dedup:claude:sess-dedup",
		SessionID:      "sess-dedup",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1100,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "auth.go",
			},
		},
	}
	_, err = c.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame 2 failed: %v", err)
	}
	c.Flush("env-dedup:claude:sess-dedup")

	mu.Lock()
	secondCount := updateCount
	mu.Unlock()
	if secondCount != 1 {
		t.Errorf("expected near-duplicate summary to be deduplicated without triggering onUpdate (count=%d, want 1)", secondCount)
	}

	// Frame 3: Distinct action / target -> "Running JWT authentication unit tests" (target: "auth_test.go", phase: "testing")
	mu.Lock()
	currentResponse = `{"action":"Running JWT authentication unit tests","phase":"testing","activeTarget":"auth_test.go"}`
	mu.Unlock()
	time.Sleep(5 * time.Millisecond)

	f3 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            3,
		EnvID:          "env-dedup",
		Environment:    env,
		AgentID:        "env-dedup:claude:sess-dedup",
		SessionID:      "sess-dedup",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1200,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "auth_test.go",
			},
		},
	}
	_, err = c.ProcessFrame(f3)
	if err != nil {
		t.Fatalf("ProcessFrame 3 failed: %v", err)
	}
	c.Flush("env-dedup:claude:sess-dedup")

	mu.Lock()
	thirdCount := updateCount
	mu.Unlock()
	if thirdCount != 2 {
		t.Errorf("expected distinct summary to update onUpdate (count=%d, want 2)", thirdCount)
	}
}

func TestClassifier_CloseAndFlush(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)

	// Flush non-existent agent should return cleanly
	c.Flush("non-existent-agent")
	c.FlushAll()

	// Close cleanly
	if err := c.Close(); err != nil {
		t.Errorf("c.Close() returned error: %v", err)
	}

	// Second close should be idempotent
	if err := c.Close(); err != nil {
		t.Errorf("second c.Close() returned error: %v", err)
	}
}

func TestClassifier_WorkerCleanupOnRunExited(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-exit",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Testing exit","phase":"testing"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 1 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-exit",
		Label:           "testbox",
		Hostname:        "testbox",
		Platform:        "linux",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	agentID := "env-exit:claude:sess-exit"

	// 1. Tool call enqueues worker
	toolName := "Bash"
	f1 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-exit",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName,
		},
	}
	if _, err := c.ProcessFrame(f1); err != nil {
		t.Fatalf("ProcessFrame 1 failed: %v", err)
	}

	c.mu.Lock()
	_, workerExists := c.workers[agentID]
	_, ctxExists := c.agentCtx[agentID]
	c.mu.Unlock()

	if !workerExists {
		t.Errorf("expected worker to exist after tool call")
	}
	if !ctxExists {
		t.Errorf("expected agentCtx to exist after tool call")
	}

	// 2. run_exited terminates worker and cleans up maps
	fExit := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            2,
		EnvID:          "env-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-exit",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1100,
		EventType:      "run_exited",
	}
	if _, err := c.ProcessFrame(fExit); err != nil {
		t.Fatalf("ProcessFrame exit failed: %v", err)
	}

	c.mu.Lock()
	_, workerExistsAfter := c.workers[agentID]
	_, ctxExistsAfter := c.agentCtx[agentID]
	c.mu.Unlock()

	if workerExistsAfter {
		t.Errorf("expected worker to be removed on run_exited")
	}
	if ctxExistsAfter {
		t.Errorf("expected agentCtx to be removed on run_exited")
	}
}

func TestClassifier_DiscardStaleSummaryOnStateTransition(t *testing.T) {
	var mu sync.Mutex
	updateCount := 0
	serverHold := make(chan struct{})

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-serverHold // hold response until frame transition occurs
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-stale",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Old slow tool summary","phase":"editing","activeTarget":"old.go"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)
	defer close(serverHold)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 1 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	c.SetOnUpdate(func(a *types.IAgentState) {
		mu.Lock()
		updateCount++
		mu.Unlock()
	})

	env := types.IEnvironmentInfo{
		ID:              "env-stale",
		Label:           "testbox",
		Hostname:        "testbox",
		Platform:        "linux",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	agentID := "env-stale:claude:sess-stale"

	// 1. Tool call enqueues background job (held in mockServer)
	toolName := "Edit"
	f1 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-stale",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-stale",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "old.go",
			},
		},
	}
	if _, err := c.ProcessFrame(f1); err != nil {
		t.Fatalf("ProcessFrame 1 failed: %v", err)
	}

	// 2. Question prompt transitions agent to WAITING_QUESTION before background job completes
	q := "Do you want to apply this change?"
	fQ := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            2,
		EnvID:          "env-stale",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-stale",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1050,
		EventType:      "question_prompt",
		Data: types.TelemetryFrameData{
			Question: &q,
		},
	}
	if _, err := c.ProcessFrame(fQ); err != nil {
		t.Fatalf("ProcessFrame question failed: %v", err)
	}

	// Release mock server to allow background worker to process stale job
	serverHold <- struct{}{}

	// Flush queue
	c.Flush(agentID)

	// Verify agent remains in WAITING_QUESTION and was not overwritten by stale tool summary
	agent, err := s.GetAgent(agentID)
	if err != nil || agent == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("expected Activity to remain WAITING_QUESTION, got %v", agent.Activity.Activity)
	}
	if agent.Activity.Action != "Do you want to apply this change?" {
		t.Errorf("expected Action to remain question text, got %q", agent.Activity.Action)
	}
	if agent.Activity.ActiveTarget != nil {
		t.Errorf("expected ActiveTarget to be nil, got %v", *agent.Activity.ActiveTarget)
	}
}

func TestClassifier_DiscardInFlightSummaryOnRunExited(t *testing.T) {
	var mu sync.Mutex
	var updates []*types.IAgentState
	serverCalled := make(chan struct{})
	serverHold := make(chan struct{})
	serverDone := make(chan struct{})

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case serverCalled <- struct{}{}:
		default:
		}
		<-serverHold // hold response until agent exits
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-stale-exit",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"In-flight summary that should be discarded","phase":"editing","activeTarget":"file.go"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
		select {
		case serverDone <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(mockServer.Close)
	defer func() {
		// make sure serverHold is never left unclosed if test fails early
		select {
		case <-serverHold:
		default:
			close(serverHold)
		}
	}()

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 1 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	c.syncMode = false
	defer func() { _ = c.Close() }()

	c.SetOnUpdate(func(a *types.IAgentState) {
		mu.Lock()
		copied := *a
		updates = append(updates, &copied)
		mu.Unlock()
	})

	env := types.IEnvironmentInfo{
		ID:              "env-exit",
		Label:           "testbox",
		Hostname:        "testbox",
		Platform:        "linux",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	agentID := "env-exit:claude:sess-exit"

	// 1. Start run
	fStart := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-exit",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "run_started",
	}
	if _, err := c.ProcessFrame(fStart); err != nil {
		t.Fatalf("ProcessFrame run_started failed: %v", err)
	}

	// 2. Tool call enqueues background job which blocks on serverHold
	toolName := "Edit"
	fTool := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            2,
		EnvID:          "env-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-exit",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1001,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName,
			ToolInput: map[string]interface{}{
				"file_path": "file.go",
			},
		},
	}
	if _, err := c.ProcessFrame(fTool); err != nil {
		t.Fatalf("ProcessFrame tool_call failed: %v", err)
	}

	// Wait until mockServer is entered
	select {
	case <-serverCalled:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for summarizer to call mockServer")
	}

	// 3. Process run_exited while LLM summary is in flight
	fExit := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            3,
		EnvID:          "env-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-exit",
		PID:            101,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1002,
		EventType:      "run_exited",
	}
	if _, err := c.ProcessFrame(fExit); err != nil {
		t.Fatalf("ProcessFrame run_exited failed: %v", err)
	}

	// Verify agent has exited in store and capture state
	agentExited, err := s.GetAgent(agentID)
	if err != nil || agentExited == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agentExited.Activity.Lifecycle != types.LifecycleExited {
		t.Fatalf("expected Lifecycle to be EXITED, got %s", agentExited.Activity.Lifecycle)
	}
	expectedAction := agentExited.Activity.Action
	expectedUpdatedAt := agentExited.Activity.UpdatedAt

	mu.Lock()
	updatesCountAtExit := len(updates)
	mu.Unlock()

	// 4. Release mockServer so in-flight summary completes
	serverHold <- struct{}{}

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for mockServer to respond")
	}

	// Give background worker time to process (and discard) the completed summary
	time.Sleep(100 * time.Millisecond)

	// 5. Verify agent state in store was NOT modified by the in-flight summary
	agentFinal, err := s.GetAgent(agentID)
	if err != nil || agentFinal == nil {
		t.Fatalf("GetAgent final failed: %v", err)
	}
	if agentFinal.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected Lifecycle to remain EXITED, got %s", agentFinal.Activity.Lifecycle)
	}
	if agentFinal.Activity.Action != expectedAction {
		t.Errorf("expected Action to remain %q, got overwritten with %q", expectedAction, agentFinal.Activity.Action)
	}
	if agentFinal.Activity.UpdatedAt != expectedUpdatedAt {
		t.Errorf("expected UpdatedAt to remain %d, got %d", expectedUpdatedAt, agentFinal.Activity.UpdatedAt)
	}

	// 6. Verify onUpdate was NOT called with the discarded summary
	mu.Lock()
	defer mu.Unlock()
	if len(updates) > updatesCountAtExit {
		t.Errorf("expected no onUpdate calls after exit, got %d new update(s)", len(updates)-updatesCountAtExit)
		for i := updatesCountAtExit; i < len(updates); i++ {
			t.Logf("unexpected post-exit update: action=%q, phase=%v, updated_at=%d",
				updates[i].Activity.Action, updates[i].Activity.Phase, updates[i].Activity.UpdatedAt)
		}
	}
}

func TestClassifier_SyncFlushTrailingCandidate_DiscardOnRunExited(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-sync-exit",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Sync trailing summary that should be discarded","phase":"editing"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 10 * time.Second, // trailing debounce window
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	c.syncMode = true
	defer func() { _ = c.Close() }()

	var updates []*types.IAgentState
	var mu sync.Mutex
	c.SetOnUpdate(func(a *types.IAgentState) {
		mu.Lock()
		copied := *a
		updates = append(updates, &copied)
		mu.Unlock()
	})

	env := types.IEnvironmentInfo{
		ID:              "env-sync-exit",
		Label:           "testbox",
		Hostname:        "testbox",
		Platform:        "linux",
		User:            "tester",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	proj := types.IProjectInfo{
		ID:            "github.com/org/repo",
		Name:          "repo",
		CanonicalRoot: "/workspace",
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	agentID := "env-sync-exit:claude:sess-sync"

	// 1. Start run
	fStart := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-sync-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-sync",
		PID:            202,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
		EventType:      "run_started",
	}
	if _, err := c.ProcessFrame(fStart); err != nil {
		t.Fatalf("ProcessFrame run_started failed: %v", err)
	}

	// 2. First tool call to establish lastSummarizedToolKey
	toolName1 := "Read"
	fTool1 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            2,
		EnvID:          "env-sync-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-sync",
		PID:            202,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1001,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName1,
			ToolInput: map[string]interface{}{
				"file_path": "read.go",
			},
		},
	}
	if _, err := c.ProcessFrame(fTool1); err != nil {
		t.Fatalf("ProcessFrame tool_call 1 failed: %v", err)
	}

	// 3. Second tool call within 10s debounce window buffers as trailing candidate
	toolName2 := "Edit"
	fTool2 := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            3,
		EnvID:          "env-sync-exit",
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-sync",
		PID:            202,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "2.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1002,
		EventType:      "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName2,
			ToolInput: map[string]interface{}{
				"file_path": "edit.go",
			},
		},
	}
	if _, err := c.ProcessFrame(fTool2); err != nil {
		t.Fatalf("ProcessFrame tool_call 2 failed: %v", err)
	}

	// Now simulate the agent having exited in store while actx still has pendingCandidate
	now := int64(1003)
	if err := s.EndCurrentRun(agentID, now); err != nil {
		t.Fatalf("EndCurrentRun failed: %v", err)
	}
	agent, err := s.GetAgent(agentID)
	if err != nil || agent == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	agent.Activity.Lifecycle = types.LifecycleExited
	if agent.Activity.CurrentRun != nil {
		agent.Activity.CurrentRun.EndedAt = &now
	}
	if err := s.SaveAgentState(*agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	expectedAction := agent.Activity.Action
	mu.Lock()
	updatesCountBeforeFlush := len(updates)
	mu.Unlock()

	// Flush the candidate
	c.Flush(agentID)

	agentFinal, err := s.GetAgent(agentID)
	if err != nil || agentFinal == nil {
		t.Fatalf("GetAgent final failed: %v", err)
	}
	if agentFinal.Activity.Action != expectedAction {
		t.Errorf("expected Action to remain %q, got %q", expectedAction, agentFinal.Activity.Action)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) > updatesCountBeforeFlush {
		t.Errorf("expected no onUpdate calls on flush for exited agent, got %d updates", len(updates)-updatesCountBeforeFlush)
	}
}

func TestClassifier_CoverageEdgeCases(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	// 1. ensureAction with non-empty action returns nil
	st := &types.IAgentState{
		ID: "agent-ensure",
		Activity: types.IAgentActivity{
			Action: "Existing action",
		},
	}
	if job := c.ensureAction(st); job != nil {
		t.Errorf("expected ensureAction to return nil for non-empty action")
	}

	// 2. processSummaryJob with empty action
	c.processSummaryJob("non-existent", summaryJob{
		input: summarizer.SummaryInput{},
	})

	// 3. processSummaryJob with non-existent agent in store
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Some action","phase":"editing"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	mockSum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{Name: "mock", Provider: p, Model: "gpt-4o-mini"},
		},
		DebounceWindow: 1 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c2, s2 := newTestClassifier(t, mockSum)
	defer func() { _ = c2.Close() }()

	// Non-existent agent in store
	c2.processSummaryJob("non-existent-agent-id", summaryJob{
		input: summarizer.SummaryInput{CurrentAction: "Some action"},
	})

	// Near-duplicate with ActionStale == true
	agentID := "agent-dup-stale"
	agentState := types.IAgentState{
		ID:        agentID,
		SessionID: "sess-dup",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "linux",
		},
		Activity: types.IAgentActivity{
			Lifecycle:   types.LifecycleRunning,
			Activity:    types.ActivityBusy,
			Action:      "Some action",
			ActionStale: true,
			CurrentRun: &types.IRunInfo{
				ID:  "run-dup",
				PID: 1234,
			},
		},
	}
	if err := s2.SaveAgentState(agentState); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}
	c2.processSummaryJob(agentID, summaryJob{
		input: summarizer.SummaryInput{
			Activity:      types.ActivityBusy,
			CurrentAction: "Some action",
		},
	})
	reloaded, err := s2.GetAgent(agentID)
	if err != nil || reloaded == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if reloaded.Activity.ActionStale {
		t.Errorf("expected ActionStale to be cleared on near duplicate match")
	}

	// 4. flushTrailingCandidateLocked duplicate toolKey suppression
	c2.mu.Lock()
	c2.agentCtx[agentID] = &agentContext{
		lastSummarizedToolKey: "tool:Read:file.go",
		pendingCandidate: &pendingSummaryCandidate{
			toolKey:   "tool:Read:file.go",
			timestamp: time.Now().UnixMilli(),
		},
	}
	c2.flushTrailingCandidateLocked(agentID)
	c2.mu.Unlock()

	// 5. handleIdleTransition with non-running agent
	c.mu.Lock()
	c.agentCtx["agent-idle-exited"] = &agentContext{
		rootGeneration: 1,
		rootActivity:   types.ActivityBusy,
	}
	c.mu.Unlock()
	exitedAgent := types.IAgentState{
		ID:        "agent-idle-exited",
		SessionID: "sess-idle",
		Harness:   types.HarnessClaudeCode,
		Activity: types.IAgentActivity{
			Lifecycle: types.LifecycleExited,
			Activity:  types.ActivityBusy,
		},
	}
	_ = s.SaveAgentState(exitedAgent)
	c.handleIdleTransition("agent-idle-exited", 1)

	// handleIdleTransition with generation mismatch
	c.handleIdleTransition("agent-idle-exited", 999)
}

func TestClassifier_IdleDebounceDuringAPIPauses(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	// Enable async worker and set a 100ms idle grace period for testing
	c.syncMode = false
	c.SetIdleGracePeriod(100 * time.Millisecond)
	defer func() { _ = c.Close() }()

	proj := types.IProjectInfo{
		ID:            "proj-1",
		Name:          "repo",
		CanonicalRoot: "/repo",
	}
	ws := types.IWorkspaceInfo{
		Branch:     "main",
		IsDetached: false,
		IsWorktree: false,
		CWD:        "/repo",
	}
	env := types.IEnvironmentInfo{
		ID:       "env-1",
		Hostname: "my-macbook",
		Label:    "my-macbook",
		Platform: "darwin",
	}

	agentID := "env-1:pi:sess-debounce"
	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          env.ID,
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-debounce",
		PID:            200,
		Harness:        types.HarnessPi,
		HarnessVersion: "1.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
	}

	// 1. Session start -> Idle
	fStart := baseFrame
	fStart.Seq = 1
	fStart.EventType = "run_started"
	agent, err := c.ProcessFrame(fStart)
	if err != nil {
		t.Fatalf("run_started failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityIdle {
		t.Fatalf("expected initial Activity to be IDLE, got %v", agent.Activity.Activity)
	}

	// 2. User prompt -> Busy
	promptText := "Refactor auth middleware"
	fPrompt := baseFrame
	fPrompt.Seq = 2
	fPrompt.Timestamp = 1010
	fPrompt.EventType = "turn_start"
	fPrompt.Data = types.TelemetryFrameData{
		Title:   &promptText,
		Content: &promptText,
	}
	agent, err = c.ProcessFrame(fPrompt)
	if err != nil {
		t.Fatalf("turn_start failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected Activity to be BUSY after user prompt, got %v", agent.Activity.Activity)
	}

	// 3. Model intermediate explanation without tool call -> turn_end with stopReason "stop"
	// Under idle grace period, the agent must REMAIN BUSY to absorb the API pause
	stopReason := "stop"
	explanation := "I will first search for auth middleware files."
	fEnd1 := baseFrame
	fEnd1.Seq = 3
	fEnd1.Timestamp = 1050
	fEnd1.EventType = "turn_end"
	fEnd1.Data = types.TelemetryFrameData{
		StopReason: &stopReason,
		Content:    &explanation,
	}
	agent, err = c.ProcessFrame(fEnd1)
	if err != nil {
		t.Fatalf("turn_end 1 failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected Activity to REMAIN BUSY during API pause within grace period, got %v", agent.Activity.Activity)
	}

	// 4. Before grace period expires (at 40ms into 100ms grace period), next API call returns with tool_call
	time.Sleep(40 * time.Millisecond)
	toolName := "Read"
	fTool := baseFrame
	fTool.Seq = 4
	fTool.Timestamp = 1090
	fTool.EventType = "tool_call"
	fTool.Data = types.TelemetryFrameData{
		ToolName: &toolName,
	}
	agent, err = c.ProcessFrame(fTool)
	if err != nil {
		t.Fatalf("tool_call failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected Activity to be BUSY on tool_call, got %v", agent.Activity.Activity)
	}

	// 5. Tool execution finishes with non-zero exit code -> must REMAIN BUSY
	isErr := true
	toolOutput := "exit status 1: pattern not found"
	fToolRes := baseFrame
	fToolRes.Seq = 5
	fToolRes.Timestamp = 1120
	fToolRes.EventType = "tool_result"
	fToolRes.Data = types.TelemetryFrameData{
		IsError:    &isErr,
		ToolOutput: &toolOutput,
	}
	agent, err = c.ProcessFrame(fToolRes)
	if err != nil {
		t.Fatalf("tool_result failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected Activity to remain BUSY on non-zero tool exit, got %v", agent.Activity.Activity)
	}
	if agent.Activity.LastError != nil {
		t.Errorf("expected LastError to be nil on non-zero tool exit, got %v", *agent.Activity.LastError)
	}

	// 6. Final turn_end arrives (agent completes answer)
	finalAnswer := "All refactoring complete."
	fEnd2 := baseFrame
	fEnd2.Seq = 6
	fEnd2.Timestamp = 1200
	fEnd2.EventType = "turn_end"
	fEnd2.Data = types.TelemetryFrameData{
		StopReason: &stopReason,
		Content:    &finalAnswer,
	}
	agent, err = c.ProcessFrame(fEnd2)
	if err != nil {
		t.Fatalf("turn_end 2 failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected Activity to remain BUSY initially on final turn_end, got %v", agent.Activity.Activity)
	}

	// 7. Wait for idle grace period (100ms) to expire with no further events
	time.Sleep(150 * time.Millisecond)

	// Verify agent has now smoothly transitioned to IDLE in store
	reloaded, err := s.GetAgent(agentID)
	if err != nil || reloaded == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if reloaded.Activity.Activity != types.ActivityIdle {
		t.Errorf("expected Activity to transition to IDLE after grace period elapsed, got %v", reloaded.Activity.Activity)
	}
}

func TestClassifier_QuestionPromptPreservedAcrossTurnEnd(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	proj := types.IProjectInfo{
		ID:            "proj-1",
		Name:          "repo",
		CanonicalRoot: "/repo",
	}
	ws := types.IWorkspaceInfo{
		Branch:     "main",
		IsDetached: false,
		IsWorktree: false,
		CWD:        "/repo",
	}
	env := types.IEnvironmentInfo{
		ID:       "env-1",
		Hostname: "my-macbook",
		Label:    "my-macbook",
		Platform: "darwin",
	}

	agentID := "env-1:pi:sess-question"
	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          env.ID,
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-question",
		PID:            300,
		Harness:        types.HarnessPi,
		HarnessVersion: "1.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
	}

	// 1. Session start
	fStart := baseFrame
	fStart.Seq = 1
	fStart.EventType = "run_started"
	_, _ = c.ProcessFrame(fStart)

	// 2. Question prompt arrives
	qText := "Should we proceed with database migration?"
	fQ := baseFrame
	fQ.Seq = 2
	fQ.EventType = "question_prompt"
	fQ.Data = types.TelemetryFrameData{
		Question: &qText,
	}
	agent, err := c.ProcessFrame(fQ)
	if err != nil {
		t.Fatalf("question_prompt failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Fatalf("expected Activity to be WAITING_QUESTION, got %v", agent.Activity.Activity)
	}
	if agent.Activity.Question == nil || *agent.Activity.Question != qText {
		t.Fatalf("expected Question to be %q, got %v", qText, agent.Activity.Question)
	}

	// 3. Subsequent turn_end arrives (without tool use, e.g. stopReason "stop" or nil)
	fEnd := baseFrame
	fEnd.Seq = 3
	fEnd.EventType = "turn_end"
	agent, err = c.ProcessFrame(fEnd)
	if err != nil {
		t.Fatalf("turn_end failed: %v", err)
	}
	// Must NOT be overwritten to IDLE or have Question wiped out!
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("turn_end must NOT overwrite WAITING_QUESTION, got %v", agent.Activity.Activity)
	}
	if agent.Activity.Question == nil || *agent.Activity.Question != qText {
		t.Errorf("turn_end must NOT erase Question prompt, got %v", agent.Activity.Question)
	}
}

func TestClassifier_AutoUnhideOnActivity(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	cls, st := newTestClassifier(t, sum)
	cls.SetSyncMode(true)

	t0 := int64(10000)
	baseFrame := types.ITelemetryFrame{
		Version: "1.0",
		Seq:     1,
		EnvID:   "env-1",
		Environment: types.IEnvironmentInfo{
			ID:              "env-1",
			Label:           "local",
			Hostname:        "dev",
			Platform:        "linux",
			User:            "user",
			LastHeartbeatAt: t0,
		},
		SessionID: "sess-hide-test",
		AgentID:   "env-1:claude-code:sess-hide-test",
		Harness:   types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:            "proj",
			Name:          "proj",
			CanonicalRoot: "/work/proj",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/work/proj",
		},
		Timestamp: t0,
		EventType: "run_started",
	}

	// 1. Initial run_started
	agent, err := cls.ProcessFrame(baseFrame)
	if err != nil {
		t.Fatalf("ProcessFrame failed: %v", err)
	}
	if agent.IsHidden {
		t.Fatalf("expected initial agent not to be hidden")
	}

	// 2. Hide the agent at t0 + 1000
	tHide := t0 + 1000
	_, err = st.SetAgentVisibility(agent.ID, true, &tHide)
	if err != nil {
		t.Fatalf("SetAgentVisibility failed: %v", err)
	}

	// 3. Stale frame arriving with timestamp <= tHide should NOT unhide
	toolName := "read_file"
	staleFrame := baseFrame
	staleFrame.Seq = 2
	staleFrame.Timestamp = tHide - 500
	staleFrame.EventType = "tool_call"
	staleFrame.Data = types.TelemetryFrameData{
		ToolName: &toolName,
	}
	agent, err = cls.ProcessFrame(staleFrame)
	if err != nil {
		t.Fatalf("ProcessFrame stale failed: %v", err)
	}
	if !agent.IsHidden {
		t.Fatalf("expected agent to stay hidden on stale frame before hiddenAt")
	}

	// 4. run_exited frame should NOT unhide
	exitCode := 0
	exitFrame := baseFrame
	exitFrame.Seq = 3
	exitFrame.Timestamp = tHide + 500
	exitFrame.EventType = "run_exited"
	exitFrame.Data = types.TelemetryFrameData{
		ExitCode: &exitCode,
	}
	agent, err = cls.ProcessFrame(exitFrame)
	if err != nil {
		t.Fatalf("ProcessFrame exit failed: %v", err)
	}
	if !agent.IsHidden {
		t.Fatalf("expected agent to stay hidden on run_exited")
	}

	// 5. New activity frame after tHide (e.g. tool_call) MUST unhide automatically
	newTool := "grep"
	activityFrame := baseFrame
	activityFrame.Seq = 4
	activityFrame.Timestamp = tHide + 1000
	activityFrame.EventType = "tool_call"
	activityFrame.Data = types.TelemetryFrameData{
		ToolName: &newTool,
	}
	agent, err = cls.ProcessFrame(activityFrame)
	if err != nil {
		t.Fatalf("ProcessFrame activity failed: %v", err)
	}
	if agent.IsHidden {
		t.Fatalf("expected agent to auto-unhide on new activity after hiddenAt, but it is still hidden")
	}
	if agent.HiddenAt != nil {
		t.Fatalf("expected agent.HiddenAt to be nil after unhide, got %v", agent.HiddenAt)
	}

	// Verify persistence in DB
	persisted, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if persisted.IsHidden {
		t.Fatalf("expected persisted agent to be unhidden in DB")
	}
	if persisted.HiddenAt != nil {
		t.Fatalf("expected persisted agent.HiddenAt to be nil in DB")
	}
}

func TestClassifier_DistinctSubagentCount(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	cls, st := newTestClassifier(t, sum)

	agentID := "agent-distinct-subagents"
	baseFrame := types.ITelemetryFrame{
		Version:   "1.0",
		EnvID:     "env-1",
		AgentID:   agentID,
		SessionID: "sess-distinct",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Label:    "test-mac",
			Platform: "darwin",
		},
		Timestamp: 1000,
		EventType: "run_started",
	}

	// 1. Initialize root session
	agent, err := cls.ProcessFrame(baseFrame)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}
	if agent.Activity.SubagentCount != 0 {
		t.Fatalf("expected initial subagent_count = 0, got %d", agent.Activity.SubagentCount)
	}

	// 2. Subagent 1 emits multiple frames
	sub1 := "sub-1"
	sub1Events := []string{"turn_start", "tool_call", "tool_result", "thinking", "turn_end"}
	for i, evType := range sub1Events {
		frame := baseFrame
		frame.Seq = int64(i + 2)
		frame.Timestamp = int64(1100 + i*10)
		frame.SubagentID = &sub1
		frame.EventType = evType
		agent, err = cls.ProcessFrame(frame)
		if err != nil {
			t.Fatalf("ProcessFrame(%s for sub-1) failed: %v", evType, err)
		}
		if agent.Activity.SubagentCount != 1 {
			t.Fatalf("expected subagent_count = 1 after sub-1 event %s (step %d), got %d", evType, i+1, agent.Activity.SubagentCount)
		}
	}

	// 3. Subagent 2 emits multiple frames
	sub2 := "sub-2"
	sub2Events := []string{"turn_start", "tool_call", "turn_end"}
	for i, evType := range sub2Events {
		frame := baseFrame
		frame.Seq = int64(len(sub1Events) + i + 2)
		frame.Timestamp = int64(1200 + i*10)
		frame.SubagentID = &sub2
		frame.EventType = evType
		agent, err = cls.ProcessFrame(frame)
		if err != nil {
			t.Fatalf("ProcessFrame(%s for sub-2) failed: %v", evType, err)
		}
		if agent.Activity.SubagentCount != 2 {
			t.Fatalf("expected subagent_count = 2 after sub-2 event %s (step %d), got %d", evType, i+1, agent.Activity.SubagentCount)
		}
	}

	// 4. Subagent 1 emits another frame -> should stay 2
	fSub1Again := baseFrame
	fSub1Again.Seq = 20
	fSub1Again.Timestamp = 1300
	fSub1Again.SubagentID = &sub1
	fSub1Again.EventType = "tool_call"
	agent, err = cls.ProcessFrame(fSub1Again)
	if err != nil {
		t.Fatalf("ProcessFrame(sub-1 again) failed: %v", err)
	}
	if agent.Activity.SubagentCount != 2 {
		t.Fatalf("expected subagent_count to remain 2 on re-encountering sub-1, got %d", agent.Activity.SubagentCount)
	}

	// 5. Subagent 3 emits a frame -> increments to 3
	sub3 := "sub-3"
	fSub3 := baseFrame
	fSub3.Seq = 21
	fSub3.Timestamp = 1400
	fSub3.SubagentID = &sub3
	fSub3.EventType = "tool_call"
	agent, err = cls.ProcessFrame(fSub3)
	if err != nil {
		t.Fatalf("ProcessFrame(sub-3) failed: %v", err)
	}
	if agent.Activity.SubagentCount != 3 {
		t.Fatalf("expected subagent_count = 3 after sub-3, got %d", agent.Activity.SubagentCount)
	}

	// Verify DB persistence
	persisted, err := st.GetAgent(agentID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if persisted.Activity.SubagentCount != 3 {
		t.Fatalf("expected persisted subagent_count = 3, got %d", persisted.Activity.SubagentCount)
	}
}

func TestClassifier_DistinctSubagentCount_RestartRecovery(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	cls, st := newTestClassifier(t, sum)

	agentID := "agent-restart-subagents"
	baseFrame := types.ITelemetryFrame{
		Version:   "1.0",
		EnvID:     "env-1",
		AgentID:   agentID,
		SessionID: "sess-restart",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Label:    "test-mac",
			Platform: "darwin",
		},
		Timestamp: 1000,
		EventType: "run_started",
	}

	// 1. Process root run_started
	if _, err := cls.ProcessFrame(baseFrame); err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}

	// 2. Subagent 1 emits frames and persists events in store
	sub1 := "sub-alpha"
	f1 := baseFrame
	f1.Seq = 2
	f1.Timestamp = 1100
	f1.SubagentID = &sub1
	f1.EventType = "tool_call"
	if _, err := cls.ProcessFrame(f1); err != nil {
		t.Fatalf("ProcessFrame(sub-alpha) failed: %v", err)
	}
	if err := st.RecordTelemetryEvent(f1); err != nil {
		t.Fatalf("RecordTelemetryEvent(f1) failed: %v", err)
	}

	// 3. Create a brand new Classifier instance pointing to the same store (simulating daemon restart)
	p, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("pricing.NewPricingEngine failed: %v", err)
	}
	cls2 := New(st, p, sum)
	cls2.SetIdleGracePeriod(0)

	// 4. Send another frame from sub-alpha to the new classifier instance -> SubagentCount should remain 1
	f2 := baseFrame
	f2.Seq = 3
	f2.Timestamp = 1200
	f2.SubagentID = &sub1
	f2.EventType = "tool_result"
	agent, err := cls2.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame on cls2 failed: %v", err)
	}
	if agent.Activity.SubagentCount != 1 {
		t.Fatalf("expected subagent_count = 1 after restart for already-seen subagent, got %d", agent.Activity.SubagentCount)
	}

	// 5. Send frame from a new subagent beta to the new classifier instance -> SubagentCount increments to 2
	sub2 := "sub-beta"
	f3 := baseFrame
	f3.Seq = 4
	f3.Timestamp = 1300
	f3.SubagentID = &sub2
	f3.EventType = "tool_call"
	agent, err = cls2.ProcessFrame(f3)
	if err != nil {
		t.Fatalf("ProcessFrame(sub-beta) on cls2 failed: %v", err)
	}
	if agent.Activity.SubagentCount != 2 {
		t.Fatalf("expected subagent_count = 2 for new subagent on cls2, got %d", agent.Activity.SubagentCount)
	}
}

func TestClassifier_RunExited_PreservesUpdatedAt(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	p, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("NewPricingEngine failed: %v", err)
	}
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New failed: %v", err)
	}
	c := New(st, p, sum)
	c.SetIdleGracePeriod(0)

	now := int64(1000000)
	baseFrame := types.ITelemetryFrame{
		Version: "1.0",
		EnvID:   "env-1",
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		AgentID:   "agent-preserve-updated-at",
		SessionID: "sess-preserve",
		Harness:   types.HarnessPi,
		Project: types.IProjectInfo{
			ID:   "proj-1",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/workspace",
		},
		PID:       1234,
		Timestamp: now,
	}

	// 1. run_started at T0
	f1 := baseFrame
	f1.Seq = 1
	f1.EventType = "run_started"
	agent, err := c.ProcessFrame(f1)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}
	if agent.Activity.UpdatedAt != now {
		t.Fatalf("expected UpdatedAt %d, got %d", now, agent.Activity.UpdatedAt)
	}

	// 2. tool_call at T0 + 2000
	toolTime := now + 2000
	tool := "bash"
	f2 := baseFrame
	f2.Seq = 2
	f2.EventType = "tool_call"
	f2.Timestamp = toolTime
	f2.Data.ToolName = &tool
	agent, err = c.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call) failed: %v", err)
	}
	if agent.Activity.UpdatedAt != toolTime {
		t.Fatalf("expected UpdatedAt %d, got %d", toolTime, agent.Activity.UpdatedAt)
	}

	// 3. run_exited 24 hours later (T0 + 86400000)
	exitTime := now + 86400000
	fExit := baseFrame
	fExit.Seq = 3
	fExit.EventType = "run_exited"
	fExit.Timestamp = exitTime
	agent, err = c.ProcessFrame(fExit)
	if err != nil {
		t.Fatalf("ProcessFrame(run_exited) failed: %v", err)
	}

	if agent.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected Lifecycle %s, got %s", types.LifecycleExited, agent.Activity.Lifecycle)
	}

	// The in-memory state returned must preserve the last actual active timestamp (toolTime)
	if agent.Activity.UpdatedAt != toolTime {
		t.Errorf("agent.Activity.UpdatedAt was overwritten: expected %d, got %d (diff: %d ms)", toolTime, agent.Activity.UpdatedAt, agent.Activity.UpdatedAt-toolTime)
	}

	// Persisted state in SQLite must also preserve toolTime
	reloaded, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("st.GetAgent failed: %v", err)
	}
	if reloaded.Activity.UpdatedAt != toolTime {
		t.Errorf("persisted reloaded.Activity.UpdatedAt was overwritten: expected %d, got %d (diff: %d ms)", toolTime, reloaded.Activity.UpdatedAt, reloaded.Activity.UpdatedAt-toolTime)
	}
}

func TestClassifier_ToolResult_ClearsActiveToolAndTarget(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		_ = st.Close()
	}()

	p, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("NewPricingEngine failed: %v", err)
	}
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New failed: %v", err)
	}
	c := New(st, p, sum)
	c.SetIdleGracePeriod(0)

	now := int64(1000000)
	baseFrame := types.ITelemetryFrame{
		Version: "1.0",
		EnvID:   "env-1",
		Environment: types.IEnvironmentInfo{
			ID:       "env-1",
			Platform: "darwin",
		},
		AgentID:   "agent-tool-result-clear",
		SessionID: "sess-clear",
		Harness:   types.HarnessClaudeCode,
		Project: types.IProjectInfo{
			ID:   "proj-1",
			Name: "agent-status",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/workspace",
		},
		PID:       4321,
		Timestamp: now,
	}

	// 1. run_started
	f1 := baseFrame
	f1.Seq = 1
	f1.EventType = "run_started"
	agent, err := c.ProcessFrame(f1)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}
	if agent.Activity.ActiveTool != nil || agent.Activity.ActiveTarget != nil {
		t.Fatalf("expected ActiveTool and ActiveTarget nil on start")
	}

	// 2. tool_call (Bash running tests)
	toolBash := "Bash"
	cmd := "go test -race ./..."
	f2 := baseFrame
	f2.Seq = 2
	f2.Timestamp = now + 1000
	f2.EventType = "tool_call"
	f2.Data = types.TelemetryFrameData{
		ToolName:  &toolBash,
		ToolInput: map[string]interface{}{"command": cmd},
	}
	agent, err = c.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call Bash) failed: %v", err)
	}
	if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != toolBash {
		t.Fatalf("expected ActiveTool %q, got %v", toolBash, agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget == nil || *agent.Activity.ActiveTarget != cmd {
		t.Fatalf("expected ActiveTarget %q, got %v", cmd, agent.Activity.ActiveTarget)
	}

	// 3. tool_result (success) -> MUST clear ActiveTool and ActiveTarget
	f3 := baseFrame
	f3.Seq = 3
	f3.Timestamp = now + 5000
	f3.EventType = "tool_result"
	outOk := "PASS\nok\n"
	isErrFalse := false
	f3.Data = types.TelemetryFrameData{
		ToolOutput: &outOk,
		IsError:    &isErrFalse,
	}
	agent, err = c.ProcessFrame(f3)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_result ok) failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected ActivityBusy, got %s", agent.Activity.Activity)
	}
	if agent.Activity.ActiveTool != nil {
		t.Errorf("expected ActiveTool nil after tool_result, got %q", *agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget != nil {
		t.Errorf("expected ActiveTarget nil after tool_result, got %q", *agent.Activity.ActiveTarget)
	}

	// Verify in SQLite store
	persisted, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("st.GetAgent failed: %v", err)
	}
	if persisted.Activity.ActiveTool != nil {
		t.Errorf("persisted ActiveTool must be nil after tool_result, got %q", *persisted.Activity.ActiveTool)
	}
	if persisted.Activity.ActiveTarget != nil {
		t.Errorf("persisted ActiveTarget must be nil after tool_result, got %q", *persisted.Activity.ActiveTarget)
	}

	// 4. tool_call (Edit)
	toolEdit := "Edit"
	filePath := "internal/auth/jwt.go"
	f4 := baseFrame
	f4.Seq = 4
	f4.Timestamp = now + 6000
	f4.EventType = "tool_call"
	f4.Data = types.TelemetryFrameData{
		ToolName:  &toolEdit,
		ToolInput: map[string]interface{}{"path": filePath},
	}
	agent, err = c.ProcessFrame(f4)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call Edit) failed: %v", err)
	}
	if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != toolEdit {
		t.Fatalf("expected ActiveTool %q, got %v", toolEdit, agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget == nil || *agent.Activity.ActiveTarget != filePath {
		t.Fatalf("expected ActiveTarget %q, got %v", filePath, agent.Activity.ActiveTarget)
	}

	// 5. tool_result (error) -> MUST clear ActiveTool and ActiveTarget
	f5 := baseFrame
	f5.Seq = 5
	f5.Timestamp = now + 7000
	f5.EventType = "tool_result"
	outErr := "syntax error: unexpected token"
	isErrTrue := true
	f5.Data = types.TelemetryFrameData{
		ToolOutput: &outErr,
		IsError:    &isErrTrue,
	}
	agent, err = c.ProcessFrame(f5)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_result error) failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected ActivityBusy on tool error exit, got %s", agent.Activity.Activity)
	}
	if agent.Activity.ActiveTool != nil {
		t.Errorf("expected ActiveTool nil after error tool_result, got %q", *agent.Activity.ActiveTool)
	}
	if agent.Activity.ActiveTarget != nil {
		t.Errorf("expected ActiveTarget nil after error tool_result, got %q", *agent.Activity.ActiveTarget)
	}

	// Verify in SQLite store
	persisted2, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("st.GetAgent failed: %v", err)
	}
	if persisted2.Activity.ActiveTool != nil {
		t.Errorf("persisted ActiveTool must be nil after error tool_result, got %q", *persisted2.Activity.ActiveTool)
	}
	if persisted2.Activity.ActiveTarget != nil {
		t.Errorf("persisted ActiveTarget must be nil after error tool_result, got %q", *persisted2.Activity.ActiveTarget)
	}
}

func TestClassifier_FlushAll_And_Coverage(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	p, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("NewPricingEngine failed: %v", err)
	}
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New failed: %v", err)
	}
	c := New(st, p, sum)
	defer func() { _ = c.Close() }()

	// Test FlushAll with no workers
	c.FlushAll()

	// Test extractTarget coverage
	if tgt := extractTarget("custom", nil); tgt != "" {
		t.Errorf("expected empty string for nil input, got %q", tgt)
	}
	if tgt := extractTarget("custom", map[string]interface{}{"path": 123}); tgt != "" {
		t.Errorf("expected empty string for non-string, got %q", tgt)
	}
	if tgt := extractTarget("custom", map[string]interface{}{"path": "   "}); tgt != "" {
		t.Errorf("expected empty string for whitespace string, got %q", tgt)
	}
	for _, key := range []string{"file_path", "notebook_path", "description", "url", "query", "skill", "pattern", "prompt"} {
		input := map[string]interface{}{key: "target-val"}
		if tgt := extractTarget("custom", input); tgt != "target-val" {
			t.Errorf("expected target-val for key %s, got %q", key, tgt)
		}
	}

	// Test ensureAction when action is empty
	agent := &types.IAgentState{
		ID: "agent-ensure",
		Activity: types.IAgentActivity{
			Activity: types.ActivityIdle,
		},
	}
	c.ensureAction(agent)
	if agent.Activity.Action == "" {
		t.Errorf("expected ensureAction to populate Action")
	}

	// Test FlushAll with active workers and trailing timers
	f := types.ITelemetryFrame{
		AgentID:   "agent-flush",
		SessionID: "sess-flush",
		EventType: "run_started",
		Timestamp: 1000,
	}
	_, _ = c.ProcessFrame(f)

	c.mu.Lock()
	c.scheduleTrailingTimerLocked("agent-flush", 0)
	c.scheduleTrailingTimerLocked("agent-flush-2", 10*time.Millisecond)
	c.mu.Unlock()

	c.FlushAll()

	// Test syncMode trailing candidate flush
	c.SetSyncMode(true)
	c.SetOnUpdate(func(a *types.IAgentState) {})
	toolEdit := "Edit"
	c.mu.Lock()
	actx := c.agentCtx["agent-flush"]
	if actx == nil {
		actx = &agentContext{seenSubagents: make(map[string]struct{})}
		c.agentCtx["agent-flush"] = actx
	}
	actx.pendingCandidate = &pendingSummaryCandidate{
		input: summarizer.SummaryInput{
			AgentID:    "agent-flush",
			Activity:   types.ActivityBusy,
			ActiveTool: &toolEdit,
		},
		timestamp: 1005,
		toolKey:   "Edit|file.go|",
	}
	c.mu.Unlock()

	c.Flush("agent-flush")

	// Test saturated worker queue in enqueueJobLocked
	c.SetSyncMode(false)
	c.mu.Lock()
	for i := 0; i < 15; i++ {
		c.enqueueJobLocked("agent-flush", summarizer.SummaryInput{
			AgentID:  "agent-flush",
			Activity: types.ActivityBusy,
		}, int64(2000+i), false)
	}
	c.mu.Unlock()
	c.Flush("agent-flush")

	// Test ensureAction on error and waiting_question
	agent.Activity.Activity = types.ActivityError
	agent.Activity.Action = ""
	c.ensureAction(agent)
	if agent.Activity.Action == "" {
		t.Errorf("expected ensureAction to populate Action for error")
	}

	agent.Activity.Activity = types.ActivityWaitingQuestion
	agent.Activity.Action = ""
	c.ensureAction(agent)
	if agent.Activity.Action == "" {
		t.Errorf("expected ensureAction to populate Action for question")
	}
}

func TestClassifier_ToolResultAndThinking_DoNotRevertActionToThinking(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New failed: %v", err)
	}

	c, st := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		Label:    "local-dev",
		Hostname: "test-host",
		Platform: "darwin",
		User:     "dev",
	}
	ws := types.IWorkspaceInfo{
		Branch: "main",
		CWD:    "/workspace/test",
	}
	proj := types.IProjectInfo{
		ID:            "test-proj",
		Name:          "test-proj",
		CanonicalRoot: "/workspace/test",
	}

	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          "env-1",
		Environment:    env,
		AgentID:        "env-1:claude-code:sess-no-revert",
		SessionID:      "sess-no-revert",
		PID:            4242,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project:        proj,
		Workspace:      ws,
	}

	now := int64(100000)

	var agent *types.IAgentState

	// 1. run_started
	fStart := baseFrame
	fStart.Seq = 1
	fStart.Timestamp = now
	fStart.EventType = "run_started"
	_, err = c.ProcessFrame(fStart)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}

	// 2. turn_start
	fTurnStart := baseFrame
	fTurnStart.Seq = 2
	fTurnStart.Timestamp = now + 1000
	fTurnStart.EventType = "turn_start"
	_, err = c.ProcessFrame(fTurnStart)
	if err != nil {
		t.Fatalf("ProcessFrame(turn_start) failed: %v", err)
	}

	// 3. thinking frame at beginning of turn before any tools run -> Action should be "Thinking"
	thinkingContent1 := "I should edit the auth file to fix token expiration"
	fThinking1 := baseFrame
	fThinking1.Seq = 3
	fThinking1.Timestamp = now + 2000
	fThinking1.EventType = "thinking"
	fThinking1.Data = types.TelemetryFrameData{
		Content: &thinkingContent1,
	}
	agent, err = c.ProcessFrame(fThinking1)
	if err != nil {
		t.Fatalf("ProcessFrame(thinking1) failed: %v", err)
	}
	if agent.Activity.Action != "Thinking" {
		t.Errorf("expected Action 'Thinking' at start of turn, got %q", agent.Activity.Action)
	}

	// 4. tool_call (Edit)
	toolEdit := "Edit"
	filePath := "internal/auth/jwt.go"
	fToolCall := baseFrame
	fToolCall.Seq = 4
	fToolCall.Timestamp = now + 3000
	fToolCall.EventType = "tool_call"
	fToolCall.Data = types.TelemetryFrameData{
		ToolName:  &toolEdit,
		ToolInput: map[string]interface{}{"path": filePath},
	}
	agent, err = c.ProcessFrame(fToolCall)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call) failed: %v", err)
	}
	if agent.Activity.Action != "Editing internal/auth/jwt.go" {
		t.Errorf("expected Action 'Editing internal/auth/jwt.go', got %q", agent.Activity.Action)
	}
	if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != "Edit" {
		t.Errorf("expected ActiveTool 'Edit', got %v", agent.Activity.ActiveTool)
	}

	// 5. tool_result (success) -> MUST NOT revert Action to "Thinking"!
	outOk := "File updated successfully"
	isErrFalse := false
	fToolRes := baseFrame
	fToolRes.Seq = 5
	fToolRes.Timestamp = now + 4000
	fToolRes.EventType = "tool_result"
	fToolRes.Data = types.TelemetryFrameData{
		ToolOutput: &outOk,
		IsError:    &isErrFalse,
	}
	agent, err = c.ProcessFrame(fToolRes)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_result) failed: %v", err)
	}
	if agent.Activity.ActiveTool != nil {
		t.Errorf("expected ActiveTool nil after tool_result, got %q", *agent.Activity.ActiveTool)
	}
	if agent.Activity.Action == "Thinking" {
		t.Errorf("Action was reverted to 'Thinking' after tool_result! Expected semantic action to be preserved")
	}
	if agent.Activity.Action != "Editing internal/auth/jwt.go" {
		t.Errorf("expected Action 'Editing internal/auth/jwt.go', got %q", agent.Activity.Action)
	}

	// Verify persisted state in SQLite as well
	persisted, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("st.GetAgent failed: %v", err)
	}
	if persisted.Activity.Action == "Thinking" {
		t.Errorf("persisted Action was reverted to 'Thinking' after tool_result")
	}
	if persisted.Activity.Action != "Editing internal/auth/jwt.go" {
		t.Errorf("expected persisted Action 'Editing internal/auth/jwt.go', got %q", persisted.Activity.Action)
	}

	// 6. thinking frame arriving while busy after a tool has executed -> MUST NOT overwrite Action with "Thinking"
	thinkingContent2 := "Now examining the test results and deciding what to do next"
	fThinking2 := baseFrame
	fThinking2.Seq = 6
	fThinking2.Timestamp = now + 5000
	fThinking2.EventType = "thinking"
	fThinking2.Data = types.TelemetryFrameData{
		Content: &thinkingContent2,
	}
	agent, err = c.ProcessFrame(fThinking2)
	if err != nil {
		t.Fatalf("ProcessFrame(thinking2) failed: %v", err)
	}
	if agent.Activity.Action == "Thinking" {
		t.Errorf("Action was overwritten with 'Thinking' by mid-turn thinking frame!")
	}
	if agent.Activity.Action != "Editing internal/auth/jwt.go" {
		t.Errorf("expected Action 'Editing internal/auth/jwt.go', got %q", agent.Activity.Action)
	}

	// 7. second tool_call (Bash)
	toolBash := "Bash"
	cmdStr := "go test ./..."
	fToolCall2 := baseFrame
	fToolCall2.Seq = 7
	fToolCall2.Timestamp = now + 6000
	fToolCall2.EventType = "tool_call"
	fToolCall2.Data = types.TelemetryFrameData{
		ToolName:  &toolBash,
		ToolInput: map[string]interface{}{"command": cmdStr},
	}
	agent, err = c.ProcessFrame(fToolCall2)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call 2) failed: %v", err)
	}
	if agent.Activity.Action != "Running tests" {
		t.Errorf("expected Action 'Running tests', got %q", agent.Activity.Action)
	}

	// 8. second tool_result -> MUST NOT revert to "Thinking"
	fToolRes2 := baseFrame
	fToolRes2.Seq = 8
	fToolRes2.Timestamp = now + 7000
	fToolRes2.EventType = "tool_result"
	fToolRes2.Data = types.TelemetryFrameData{
		ToolOutput: &outOk,
		IsError:    &isErrFalse,
	}
	agent, err = c.ProcessFrame(fToolRes2)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_result 2) failed: %v", err)
	}
	if agent.Activity.Action == "Thinking" {
		t.Errorf("Action was reverted to 'Thinking' after second tool_result")
	}
	if agent.Activity.Action != "Running tests" {
		t.Errorf("expected Action 'Running tests', got %q", agent.Activity.Action)
	}
}

func TestClassifier_DebounceCadence_TrailingEdgeFlush(t *testing.T) {
	var mu sync.Mutex
	requestCount := 0
	lastRequestPrompt := ""

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if msgs, ok := body["messages"].([]interface{}); ok && len(msgs) > 0 {
			if m, ok := msgs[len(msgs)-1].(map[string]interface{}); ok {
				if content, ok := m["content"].(string); ok {
					mu.Lock()
					lastRequestPrompt = content
					mu.Unlock()
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Summarized Tool Action","phase":"editing"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 50 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	c.SetDebounceWindow(50 * time.Millisecond)

	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          "env-1",
		AgentID:        "env-1:claude:sess-cadence",
		SessionID:      "sess-cadence",
		PID:            100,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project:        types.IProjectInfo{ID: "p1", Name: "p1", CanonicalRoot: "/ws"},
		Workspace:      types.IWorkspaceInfo{Branch: "main", CWD: "/ws"},
		Environment:    types.IEnvironmentInfo{Label: "local", Platform: "darwin"},
	}

	// 1. Start session & turn
	fStart := baseFrame
	fStart.EventType = "run_started"
	fStart.Timestamp = 1000
	_, _ = c.ProcessFrame(fStart)

	fTurn := baseFrame
	fTurn.EventType = "turn_start"
	fTurn.Timestamp = 1001
	_, _ = c.ProcessFrame(fTurn)

	// 2. Tool call 1 (Bash) -> should trigger immediate summary
	tBash := "Bash"
	cmd1 := "npm install"
	fTool1 := baseFrame
	fTool1.EventType = "tool_call"
	fTool1.Timestamp = 1002
	fTool1.Data = types.TelemetryFrameData{
		ToolName:  &tBash,
		ToolInput: map[string]interface{}{"command": cmd1},
	}
	_, err = c.ProcessFrame(fTool1)
	if err != nil {
		t.Fatalf("ProcessFrame(fTool1) failed: %v", err)
	}

	c.Flush("env-1:claude:sess-cadence")

	mu.Lock()
	countAfter1 := requestCount
	mu.Unlock()
	if countAfter1 != 1 {
		t.Fatalf("expected 1 summary after tool 1, got %d", countAfter1)
	}

	// 3. Tool call 2 (Grep) within 50ms window -> should be buffered
	tGrep := "grep"
	pat2 := "authToken"
	fTool2 := baseFrame
	fTool2.EventType = "tool_call"
	fTool2.Timestamp = 1010
	fTool2.Data = types.TelemetryFrameData{
		ToolName:  &tGrep,
		ToolInput: map[string]interface{}{"pattern": pat2},
	}
	_, err = c.ProcessFrame(fTool2)
	if err != nil {
		t.Fatalf("ProcessFrame(fTool2) failed: %v", err)
	}

	// 4. Tool call 3 (Edit) within 50ms window -> should replace candidate
	tEdit := "Edit"
	file3 := "src/token.ts"
	fTool3 := baseFrame
	fTool3.EventType = "tool_call"
	fTool3.Timestamp = 1020
	fTool3.Data = types.TelemetryFrameData{
		ToolName:  &tEdit,
		ToolInput: map[string]interface{}{"path": file3},
	}
	_, err = c.ProcessFrame(fTool3)
	if err != nil {
		t.Fatalf("ProcessFrame(fTool3) failed: %v", err)
	}

	// Wait for trailing edge timer to fire (> 50ms)
	time.Sleep(120 * time.Millisecond)
	c.Flush("env-1:claude:sess-cadence")

	mu.Lock()
	countAfterFlush := requestCount
	prompt := lastRequestPrompt
	mu.Unlock()

	// Should have dispatched exactly 2 summaries total: tool 1 immediately, and tool 3 on trailing edge
	if countAfterFlush != 2 {
		t.Fatalf("expected 2 summaries total (immediate + trailing), got %d", countAfterFlush)
	}
	if !strings.Contains(prompt, "src/token.ts") && !strings.Contains(prompt, "Edit") {
		t.Errorf("expected trailing summary to include src/token.ts / Edit, got prompt:\n%s", prompt)
	}
}

func TestClassifier_DuplicateToolSuppression(t *testing.T) {
	var mu sync.Mutex
	requestCount := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Testing duplicate suppression"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 50 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	c.SetDebounceWindow(50 * time.Millisecond)

	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          "env-1",
		AgentID:        "env-1:claude:sess-dup",
		SessionID:      "sess-dup",
		PID:            200,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project:        types.IProjectInfo{ID: "p1", Name: "p1", CanonicalRoot: "/ws"},
		Workspace:      types.IWorkspaceInfo{Branch: "main", CWD: "/ws"},
		Environment:    types.IEnvironmentInfo{Label: "local", Platform: "darwin"},
	}

	fStart := baseFrame
	fStart.EventType = "run_started"
	fStart.Timestamp = 1000
	_, _ = c.ProcessFrame(fStart)

	// Tool call 1
	tBash := "Bash"
	cmd := "go test ./..."
	fTool1 := baseFrame
	fTool1.EventType = "tool_call"
	fTool1.Timestamp = 1002
	fTool1.Data = types.TelemetryFrameData{
		ToolName:  &tBash,
		ToolInput: map[string]interface{}{"command": cmd},
	}
	_, _ = c.ProcessFrame(fTool1)
	c.Flush("env-1:claude:sess-dup")

	mu.Lock()
	count1 := requestCount
	mu.Unlock()
	if count1 != 1 {
		t.Fatalf("expected 1 summary after tool 1, got %d", count1)
	}

	// Advance time beyond debounce window (> 50ms)
	time.Sleep(70 * time.Millisecond)

	// Tool call 2 with identical tool and command
	fTool2 := baseFrame
	fTool2.EventType = "tool_call"
	fTool2.Timestamp = 2000
	fTool2.Data = types.TelemetryFrameData{
		ToolName:  &tBash,
		ToolInput: map[string]interface{}{"command": cmd},
	}
	_, _ = c.ProcessFrame(fTool2)
	c.Flush("env-1:claude:sess-dup")

	mu.Lock()
	count2 := requestCount
	mu.Unlock()

	// Should NOT trigger a second summary because tool call is unchanged
	if count2 != 1 {
		t.Errorf("expected duplicate tool call to be suppressed (count = 1), got %d", count2)
	}
}

func TestClassifier_DuplicateRunStarted_Idempotent(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          "env-test",
		AgentID:        "env-test:claude-code:sess-run-idempotent",
		SessionID:      "sess-run-idempotent",
		PID:            42424,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project:        types.IProjectInfo{ID: "proj-1", Name: "proj-1", CanonicalRoot: "/repo"},
		Workspace:      types.IWorkspaceInfo{Branch: "main", CWD: "/repo"},
		Environment:    types.IEnvironmentInfo{Label: "local", Platform: "darwin"},
	}

	// 1. Initial run_started (e.g. from observer process scanner) at T0 = 1000
	fStart1 := baseFrame
	fStart1.EventType = "run_started"
	fStart1.Timestamp = 1000
	agent, err := c.ProcessFrame(fStart1)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}
	if agent.Activity.Lifecycle != types.LifecycleRunning {
		t.Fatalf("expected Lifecycle %s, got %s", types.LifecycleRunning, agent.Activity.Lifecycle)
	}
	if agent.Activity.CurrentRun == nil {
		t.Fatalf("expected non-nil CurrentRun")
	}

	initialRunID := agent.Activity.CurrentRun.ID
	initialStartedAt := agent.Activity.CurrentRun.StartedAt
	if initialRunID == "" {
		t.Fatalf("expected non-empty initialRunID")
	}
	if initialStartedAt != 1000 {
		t.Fatalf("expected initialStartedAt = 1000, got %d", initialStartedAt)
	}

	// 2. Activity occurs (turn_start or tool_call) making agent BUSY at T1 = 1500
	tBash := "bash"
	fTool := baseFrame
	fTool.EventType = "tool_call"
	fTool.Timestamp = 1500
	fTool.Data = types.TelemetryFrameData{
		ToolName: &tBash,
	}
	agent, err = c.ProcessFrame(fTool)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call) failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected Activity %s, got %s", types.ActivityBusy, agent.Activity.Activity)
	}

	// 3. Duplicate run_started arrives (e.g. tailer reads line 1 from transcript) at T2 = 2500 with same PID
	fStart2 := baseFrame
	fStart2.EventType = "run_started"
	fStart2.Timestamp = 2500
	agent, err = c.ProcessFrame(fStart2)
	if err != nil {
		t.Fatalf("ProcessFrame(duplicate run_started) failed: %v", err)
	}

	// Assertions for idempotency:
	// - Run ID must NOT be overwritten
	if agent.Activity.CurrentRun.ID != initialRunID {
		t.Errorf("CurrentRun.ID was overwritten: expected %q, got %q", initialRunID, agent.Activity.CurrentRun.ID)
	}
	// - StartedAt must NOT be clobbered
	if agent.Activity.CurrentRun.StartedAt != initialStartedAt {
		t.Errorf("CurrentRun.StartedAt was clobbered: expected %d, got %d", initialStartedAt, agent.Activity.CurrentRun.StartedAt)
	}
	// - Lifecycle must remain RUNNING
	if agent.Activity.Lifecycle != types.LifecycleRunning {
		t.Errorf("Lifecycle changed: expected %s, got %s", types.LifecycleRunning, agent.Activity.Lifecycle)
	}
	// - Ongoing Activity (BUSY) must NOT be reset to IDLE by duplicate run_started
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("Activity was reset: expected %s, got %s", types.ActivityBusy, agent.Activity.Activity)
	}
}

func TestClassifier_ActiveToolStartedAt_Lifecycle(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, s := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	now := int64(10000)
	agentID := "agent-active-tool-stall-test"
	sessID := "sess-stall-lifecycle"
	env := types.IEnvironmentInfo{ID: "env-stall", Platform: "darwin"}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "proj-1", Name: "repo", CanonicalRoot: "/repo"}

	baseFrame := types.ITelemetryFrame{
		AgentID:     agentID,
		SessionID:   sessID,
		Harness:     types.HarnessClaudeCode,
		Environment: env,
		Workspace:   ws,
		Project:     proj,
		Timestamp:   now,
	}

	// 1. run_started
	fStart := baseFrame
	fStart.Seq = 1
	fStart.EventType = "run_started"
	fStart.PID = 5432
	agent, err := c.ProcessFrame(fStart)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}
	if agent.Activity.ActiveToolStartedAt != nil {
		t.Errorf("expected ActiveToolStartedAt nil at run_started, got %v", *agent.Activity.ActiveToolStartedAt)
	}

	// 2. tool_call: bash at t = 10500 -> MUST set ActiveToolStartedAt = 10500
	toolTime := int64(10500)
	toolBash := "bash"
	fCall := baseFrame
	fCall.Seq = 2
	fCall.EventType = "tool_call"
	fCall.Timestamp = toolTime
	fCall.Data = types.TelemetryFrameData{
		ToolName:  &toolBash,
		ToolInput: map[string]interface{}{"command": "git rebase main"},
	}
	agent, err = c.ProcessFrame(fCall)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call) failed: %v", err)
	}
	if agent.Activity.ActiveToolStartedAt == nil {
		t.Fatalf("expected ActiveToolStartedAt non-nil after tool_call, got nil")
	}
	if *agent.Activity.ActiveToolStartedAt != toolTime {
		t.Errorf("expected ActiveToolStartedAt = %d, got %d", toolTime, *agent.Activity.ActiveToolStartedAt)
	}
	persisted, err := s.GetAgent(agentID)
	if err != nil || persisted == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if persisted.Activity.ActiveToolStartedAt == nil || *persisted.Activity.ActiveToolStartedAt != toolTime {
		t.Errorf("persisted ActiveToolStartedAt mismatch: got %v", persisted.Activity.ActiveToolStartedAt)
	}

	// 3. tool_result at t = 12000 -> MUST clear ActiveToolStartedAt, ActiveTool, ActiveTarget
	resultTime := int64(12000)
	out := "rebase complete"
	fResult := baseFrame
	fResult.Seq = 3
	fResult.EventType = "tool_result"
	fResult.Timestamp = resultTime
	fResult.Data = types.TelemetryFrameData{
		ToolOutput: &out,
	}
	agent, err = c.ProcessFrame(fResult)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_result) failed: %v", err)
	}
	if agent.Activity.ActiveToolStartedAt != nil {
		t.Errorf("expected ActiveToolStartedAt nil after tool_result, got %d", *agent.Activity.ActiveToolStartedAt)
	}

	// 4. Another tool_call followed by turn_end -> MUST clear ActiveToolStartedAt
	toolTime2 := int64(13000)
	fCall2 := baseFrame
	fCall2.Seq = 4
	fCall2.EventType = "tool_call"
	fCall2.Timestamp = toolTime2
	fCall2.Data = types.TelemetryFrameData{
		ToolName:  &toolBash,
		ToolInput: map[string]interface{}{"command": "git status"},
	}
	agent, err = c.ProcessFrame(fCall2)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call 2) failed: %v", err)
	}
	if agent.Activity.ActiveToolStartedAt == nil || *agent.Activity.ActiveToolStartedAt != toolTime2 {
		t.Fatalf("expected ActiveToolStartedAt = %d, got %v", toolTime2, agent.Activity.ActiveToolStartedAt)
	}

	stopEndTurn := "end_turn"
	fTurnEnd := baseFrame
	fTurnEnd.Seq = 5
	fTurnEnd.EventType = "turn_end"
	fTurnEnd.Timestamp = int64(14000)
	fTurnEnd.Data = types.TelemetryFrameData{
		StopReason: &stopEndTurn,
	}
	agent, err = c.ProcessFrame(fTurnEnd)
	if err != nil {
		t.Fatalf("ProcessFrame(turn_end) failed: %v", err)
	}
	if agent.Activity.ActiveToolStartedAt != nil {
		t.Errorf("expected ActiveToolStartedAt nil after turn_end, got %d", *agent.Activity.ActiveToolStartedAt)
	}
}

func TestClassifier_FileModification_SetsActiveTarget(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	defer func() { _ = c.Close() }()

	now := int64(1000)
	baseFrame := types.ITelemetryFrame{
		Version:   "1.0",
		EnvID:     "env-filemod",
		Environment: types.IEnvironmentInfo{
			ID:       "env-filemod",
			Platform: "darwin",
		},
		AgentID:   "agent-filemod",
		SessionID: "sess-filemod",
		Harness:   types.HarnessClaudeCode,
		Timestamp: now,
		EventType: "run_started",
	}

	_, err = c.ProcessFrame(baseFrame)
	if err != nil {
		t.Fatalf("ProcessFrame(run_started) failed: %v", err)
	}

	toolEdit := "Edit"
	filePath := "packages/agent-parser/normalized.go"
	fCall := baseFrame
	fCall.Seq = 2
	fCall.EventType = "tool_call"
	fCall.Timestamp = now + 1000
	fCall.Data = types.TelemetryFrameData{
		ToolName: &toolEdit,
		FileModification: &types.FileModification{
			Action:   "EDIT",
			FilePath: filePath,
		},
		ToolInput: map[string]interface{}{
			// Deliberately different or empty path to verify FileModification takes precedence
			"file_path": "legacy_path.go",
		},
	}

	agent, err := c.ProcessFrame(fCall)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call) failed: %v", err)
	}

	if agent.Activity.ActiveTarget == nil {
		t.Fatalf("expected ActiveTarget non-nil, got nil")
	}
	if *agent.Activity.ActiveTarget != filePath {
		t.Errorf("expected ActiveTarget = %q, got %q", filePath, *agent.Activity.ActiveTarget)
	}
}

func TestClassifier_SubagentActivity_Rollup_ErrorPrecedence(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-sub-err",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}

	// 1. Root agent starts run and goes idle
	agentID := "env-sub-err:claude-code:sess-root-1"
	fStart := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SessionID:   "sess-root-1",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   1000,
		EventType:   "run_started",
	}
	agent, err := c.ProcessFrame(fStart)
	if err != nil {
		t.Fatalf("run_started failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityIdle {
		t.Fatalf("expected root to be IDLE, got %s", agent.Activity.Activity)
	}

	// 2. Subagent encounters an error
	subID := "worker-1"
	isErr := true
	errMsg := "npm test failed: 2 tests crashed"
	fSubErr := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SubagentID:  &subID,
		SessionID:   "sess-root-1",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   2000,
		EventType:   "tool_result",
		Data: types.TelemetryFrameData{
			IsError:    &isErr,
			ToolOutput: &errMsg,
		},
	}
	agent, err = c.ProcessFrame(fSubErr)
	if err != nil {
		t.Fatalf("subagent error frame failed: %v", err)
	}

	// Composite parent activity must reflect ERROR
	if agent.Activity.Activity != types.ActivityError {
		t.Errorf("expected composite Activity to be ERROR, got %s", agent.Activity.Activity)
	}
	if agent.Activity.LastError == nil || !strings.Contains(*agent.Activity.LastError, errMsg) {
		t.Errorf("expected composite LastError to contain %q, got %v", errMsg, agent.Activity.LastError)
	}
}

func TestClassifier_SubagentActivity_Rollup_QuestionPrecedence(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-sub-q",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}

	agentID := "env-sub-q:claude-code:sess-root-2"
	fStart := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SessionID:   "sess-root-2",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   1000,
		EventType:   "run_started",
	}
	_, err = c.ProcessFrame(fStart)
	if err != nil {
		t.Fatalf("run_started failed: %v", err)
	}

	// Subagent requests user intervention
	subID := "scout-1"
	qText := "Do you want to delete the file?"
	fSubQ := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SubagentID:  &subID,
		SessionID:   "sess-root-2",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   2000,
		EventType:   "question_prompt",
		Data: types.TelemetryFrameData{
			Content: &qText,
		},
	}
	agent, err := c.ProcessFrame(fSubQ)
	if err != nil {
		t.Fatalf("subagent question frame failed: %v", err)
	}

	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("expected composite Activity to be WAITING_QUESTION, got %s", agent.Activity.Activity)
	}
	if agent.Activity.Question == nil || !strings.Contains(*agent.Activity.Question, qText) {
		t.Errorf("expected composite Question to contain %q, got %v", qText, agent.Activity.Question)
	}
}

func TestClassifier_SubagentActivity_Rollup_BusyPrecedence(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-sub-busy",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}

	agentID := "env-sub-busy:claude-code:sess-root-3"
	fStart := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SessionID:   "sess-root-3",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   1000,
		EventType:   "run_started",
	}
	_, err = c.ProcessFrame(fStart)
	if err != nil {
		t.Fatalf("run_started failed: %v", err)
	}

	// Subagent executes a tool
	subID := "worker-2"
	toolName := "bash"
	fSubTool := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SubagentID:  &subID,
		SessionID:   "sess-root-3",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   2000,
		EventType:   "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolName,
		},
	}
	agent, err := c.ProcessFrame(fSubTool)
	if err != nil {
		t.Fatalf("subagent tool frame failed: %v", err)
	}

	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected composite Activity to be BUSY when subagent is busy, got %s", agent.Activity.Activity)
	}
	if !strings.Contains(agent.Activity.Action, subID) {
		t.Errorf("expected composite Action to indicate subagent %q execution, got %q", subID, agent.Activity.Action)
	}
}

func TestClassifier_SubagentActivity_Rollup_TurnEndIsolation(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-sub-turn",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}

	agentID := "env-sub-turn:claude-code:sess-root-4"
	fStart := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SessionID:   "sess-root-4",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   1000,
		EventType:   "run_started",
	}
	_, _ = c.ProcessFrame(fStart)

	// Two subagents execute tools
	sub1 := "sub-1"
	sub2 := "sub-2"
	tBash := "bash"
	tGrep := "grep"

	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-root-4", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 2000, EventType: "tool_call", Data: types.TelemetryFrameData{ToolName: &tBash},
	})

	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub2,
		SessionID: "sess-root-4", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 2100, EventType: "tool_call", Data: types.TelemetryFrameData{ToolName: &tGrep},
	})

	// sub-1 turns ends
	agent, err := c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-root-4", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 3000, EventType: "turn_end",
	})
	if err != nil {
		t.Fatalf("sub-1 turn_end failed: %v", err)
	}

	// Since sub-2 is still busy, parent MUST remain BUSY
	if agent.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected parent to remain BUSY while sub-2 is running, got %s", agent.Activity.Activity)
	}

	// Now sub-2 turn ends
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub2,
		SessionID: "sess-root-4", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 4000, EventType: "turn_end",
	})
	if err != nil {
		t.Fatalf("sub-2 turn_end failed: %v", err)
	}

	// All subagents and parent are done -> parent transitions to IDLE
	if agent.Activity.Activity != types.ActivityIdle {
		t.Errorf("expected parent to be IDLE when all subagents finished, got %s", agent.Activity.Activity)
	}
}

func TestClassifier_SubagentActivity_Rollup_ActiveSubagentsList(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-sub-list",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}

	agentID := "env-sub-list:claude-code:sess-root-5"
	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-root-5", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1000, EventType: "run_started",
	})

	sub1 := "sub-alpha"
	tBash := "bash"
	agent, err := c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-root-5", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 2000, EventType: "tool_call", Data: types.TelemetryFrameData{ToolName: &tBash},
	})
	if err != nil {
		t.Fatalf("tool_call failed: %v", err)
	}

	if len(agent.Activity.ActiveSubagents) != 1 {
		t.Fatalf("expected 1 ActiveSubagent, got %d", len(agent.Activity.ActiveSubagents))
	}
	if agent.Activity.ActiveSubagents[0].ID != "sub-alpha" {
		t.Errorf("expected subagent ID 'sub-alpha', got %q", agent.Activity.ActiveSubagents[0].ID)
	}
	if agent.Activity.ActiveSubagents[0].Activity != types.ActivityBusy {
		t.Errorf("expected subagent Activity BUSY, got %s", agent.Activity.ActiveSubagents[0].Activity)
	}
}

func TestClassifier_SubagentActivity_Rollup_BranchesAndCoverage(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{ID: "env-branch", Label: "devbox", Hostname: "devbox", Platform: "linux", User: "testuser"}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-branch:claude-code:sess-branch-1"

	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-branch-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1000, EventType: "run_started",
	})

	sub1 := "sub-b1"
	// Subagent turn_start
	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-branch-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1500, EventType: "turn_start",
	})

	// Subagent error via ToolOutput
	errOut := "fatal: crash in subprocess"
	agent, _ := c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-branch-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1600, EventType: "error", Data: types.TelemetryFrameData{ToolOutput: &errOut},
	})
	if agent.Activity.Activity != types.ActivityError {
		t.Errorf("expected ActivityError, got %s", agent.Activity.Activity)
	}

	// Subagent recovers by turn_end
	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-branch-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1650, EventType: "turn_end",
	})

	// Root agent enters WAITING_QUESTION
	q := "Root question?"
	agent, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-branch-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1700, EventType: "question_prompt", Data: types.TelemetryFrameData{Content: &q},
	})
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("expected WAITING_QUESTION, got %s", agent.Activity.Activity)
	}

	// Root agent enters ERROR
	rootErr := "Root crash"
	agent, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-branch-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1800, EventType: "error", Data: types.TelemetryFrameData{Content: &rootErr},
	})
	if agent.Activity.Activity != types.ActivityError {
		t.Errorf("expected ActivityError, got %s", agent.Activity.Activity)
	}
}

func TestClassifier_Subagent_AskUserQuestionAndQuestionPrompt(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)

	env := types.IEnvironmentInfo{ID: "env-ask", Label: "devbox", Hostname: "devbox", Platform: "linux", User: "testuser"}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-ask:claude-code:sess-ask-1"

	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-ask-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1000, EventType: "run_started",
	})

	sub1 := "sub-asker"
	toolAsk := "AskUserQuestion"
	qText := "Do you want to proceed with subtask?"

	// 1. tool_call with AskUserQuestion
	agent, err := c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-ask-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1200, EventType: "tool_call",
		Data: types.TelemetryFrameData{
			ToolName: &toolAsk,
			Question: &qText,
		},
	})
	if err != nil {
		t.Fatalf("ProcessFrame tool_call AskUserQuestion failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("expected composite Activity WAITING_QUESTION, got %s", agent.Activity.Activity)
	}
	if agent.Activity.Question == nil || !strings.Contains(*agent.Activity.Question, qText) {
		t.Errorf("expected composite Question to contain %q, got %v", qText, agent.Activity.Question)
	}

	// 2. question_prompt with Question field
	qPrompt := "Confirm deletion?"
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-ask-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1300, EventType: "question_prompt",
		Data: types.TelemetryFrameData{
			Question: &qPrompt,
		},
	})
	if err != nil {
		t.Fatalf("ProcessFrame question_prompt failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("expected composite Activity WAITING_QUESTION, got %s", agent.Activity.Activity)
	}
	if agent.Activity.Question == nil || !strings.Contains(*agent.Activity.Question, qPrompt) {
		t.Errorf("expected composite Question to contain %q, got %v", qPrompt, agent.Activity.Question)
	}
}

func TestClassifier_Subagent_RestartRecoveryFromStore(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c1, st := newTestClassifier(t, sum)

	env := types.IEnvironmentInfo{ID: "env-rec", Label: "devbox", Hostname: "devbox", Platform: "linux", User: "testuser"}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-rec:claude-code:sess-rec-1"

	_, _ = c1.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-rec-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1000, EventType: "run_started",
	})

	sub1 := "sub-saved"
	tBash := "bash"
	_, _ = c1.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-rec-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1100, EventType: "tool_call", Data: types.TelemetryFrameData{ToolName: &tBash},
	})

	// Now simulate ingester restart by creating a new Classifier with same store
	p, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("pricing.NewPricingEngine() failed: %v", err)
	}
	c2 := New(st, p, sum)
	c2.SetIdleGracePeriod(0)
	defer func() { _ = c2.Close() }()

	// Process another subagent frame on c2
	sub2 := "sub-saved-2"
	tGrep := "grep"
	agent2, err := c2.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub2,
		SessionID: "sess-rec-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1200, EventType: "tool_call", Data: types.TelemetryFrameData{ToolName: &tGrep},
	})
	if err != nil {
		t.Fatalf("c2 ProcessFrame failed: %v", err)
	}

	// Should have both sub-saved and sub-saved-2 in ActiveSubagents
	if len(agent2.Activity.ActiveSubagents) != 2 {
		t.Fatalf("expected 2 ActiveSubagents after restart, got %d", len(agent2.Activity.ActiveSubagents))
	}
}

func TestClassifier_Subagent_IdleTransitionWithActiveSubagent(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)

	env := types.IEnvironmentInfo{ID: "env-idle", Label: "devbox", Hostname: "devbox", Platform: "linux", User: "testuser"}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-idle:claude-code:sess-idle-1"

	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-idle-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1000, EventType: "run_started",
	})

	prompt := "run tests"
	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-idle-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1050, EventType: "turn_start", Data: types.TelemetryFrameData{Content: &prompt},
	})

	// Subagent starts and runs tool
	sub1 := "sub-runner"
	tTest := "npm test"
	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-idle-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1100, EventType: "tool_call", Data: types.TelemetryFrameData{ToolName: &tTest},
	})

	c.mu.Lock()
	gen := c.agentCtx[agentID].rootGeneration
	c.mu.Unlock()

	// Trigger handleIdleTransition directly
	c.handleIdleTransition(agentID, gen)

	loaded, err := c.store.GetAgent(agentID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	// Even though root transitioned to idle, composite activity must remain BUSY because subagent is running!
	if loaded.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected composite Activity BUSY during active subagent, got %s", loaded.Activity.Activity)
	}
	if !strings.Contains(loaded.Activity.Action, "sub-runner") {
		t.Errorf("expected Action to mention sub-runner, got %q", loaded.Activity.Action)
	}
}

func TestClassifier_Subagent_ErrorAndQuestionClearing(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)

	env := types.IEnvironmentInfo{ID: "env-clear", Label: "devbox", Hostname: "devbox", Platform: "linux", User: "testuser"}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-clear:claude-code:sess-clear-1"

	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-clear-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1000, EventType: "run_started",
	})

	sub1 := "sub-failing"
	errMsg := "Compilation failed"

	// Subagent error
	agent, _ := c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-clear-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1100, EventType: "error", Data: types.TelemetryFrameData{Content: &errMsg},
	})
	if agent.Activity.Activity != types.ActivityError {
		t.Fatalf("expected ActivityError, got %s", agent.Activity.Activity)
	}
	if agent.Activity.LastError == nil {
		t.Fatalf("expected LastError populated, got nil")
	}

	// Subagent turns end and resolves
	agent, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-clear-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1200, EventType: "turn_end",
	})
	if agent.Activity.Activity == types.ActivityError {
		t.Errorf("expected Activity to not be ERROR after turn_end, got %s", agent.Activity.Activity)
	}
	if agent.Activity.LastError != nil {
		t.Errorf("expected LastError to be cleared to nil, got %v", *agent.Activity.LastError)
	}
}

func TestClassifier_Subagent_RunExitedClearsSubagents(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)

	env := types.IEnvironmentInfo{ID: "env-exit", Label: "devbox", Hostname: "devbox", Platform: "linux", User: "testuser"}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-exit:claude-code:sess-exit-1"

	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-exit-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1000, EventType: "run_started",
	})

	sub1 := "sub-running"
	tBash := "bash"
	_, _ = c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID, SubagentID: &sub1,
		SessionID: "sess-exit-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 1100, EventType: "tool_call", Data: types.TelemetryFrameData{ToolName: &tBash},
	})

	// Process run_exited
	agent, err := c.ProcessFrame(types.ITelemetryFrame{
		Version: "1.0", EnvID: env.ID, Environment: env, AgentID: agentID,
		SessionID: "sess-exit-1", Harness: types.HarnessClaudeCode, Project: proj, Workspace: ws,
		Timestamp: 2000, EventType: "run_exited",
	})
	if err != nil {
		t.Fatalf("run_exited failed: %v", err)
	}
	if agent.Activity.Lifecycle != types.LifecycleExited {
		t.Errorf("expected Lifecycle EXITED, got %s", agent.Activity.Lifecycle)
	}
	if agent.Activity.ActiveSubagents != nil {
		t.Errorf("expected ActiveSubagents nil on run_exited, got %+v", agent.Activity.ActiveSubagents)
	}
}

func TestClassifier_SubagentDoesNotCancelRootIdleTimer(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, s := newTestClassifier(t, sum)
	c.syncMode = false
	c.SetIdleGracePeriod(100 * time.Millisecond)
	defer func() { _ = c.Close() }()

	proj := types.IProjectInfo{
		ID:            "proj-1",
		Name:          "repo",
		CanonicalRoot: "/repo",
	}
	ws := types.IWorkspaceInfo{
		Branch:     "main",
		IsDetached: false,
		IsWorktree: false,
		CWD:        "/repo",
	}
	env := types.IEnvironmentInfo{
		ID:       "env-1",
		Hostname: "my-macbook",
		Label:    "my-macbook",
		Platform: "darwin",
	}

	agentID := "env-1:claude-code:sess-subagent-idle"
	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          env.ID,
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-subagent-idle",
		PID:            500,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
	}

	// 1. Session start -> Idle
	fStart := baseFrame
	fStart.Seq = 1
	fStart.EventType = "run_started"
	_, err = c.ProcessFrame(fStart)
	if err != nil {
		t.Fatalf("run_started failed: %v", err)
	}

	// 2. User prompt -> Busy
	promptText := "Review code"
	fPrompt := baseFrame
	fPrompt.Seq = 2
	fPrompt.Timestamp = 1010
	fPrompt.EventType = "turn_start"
	fPrompt.Data = types.TelemetryFrameData{
		Title:   &promptText,
		Content: &promptText,
	}
	agent, err := c.ProcessFrame(fPrompt)
	if err != nil {
		t.Fatalf("turn_start failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected Activity to be BUSY after user prompt, got %v", agent.Activity.Activity)
	}

	// 3. Root turn_end with stopReason "end_turn" (no tool use)
	stopReason := "end_turn"
	respText := "Done reviewing"
	fEnd := baseFrame
	fEnd.Seq = 3
	fEnd.Timestamp = 1050
	fEnd.EventType = "turn_end"
	fEnd.Data = types.TelemetryFrameData{
		StopReason: &stopReason,
		Content:    &respText,
	}
	agent, err = c.ProcessFrame(fEnd)
	if err != nil {
		t.Fatalf("turn_end failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected Activity to remain BUSY during idle grace period, got %v", agent.Activity.Activity)
	}

	// 4. Within grace period (e.g. 30ms into 100ms grace period), subagent sends tool_call frame
	time.Sleep(30 * time.Millisecond)
	subID := "agent-sub-1"
	toolName := "bash"
	fSubTool := baseFrame
	fSubTool.Seq = 4
	fSubTool.SubagentID = &subID
	fSubTool.Timestamp = 1080
	fSubTool.EventType = "tool_call"
	fSubTool.Data = types.TelemetryFrameData{
		ToolName: &toolName,
	}
	_, err = c.ProcessFrame(fSubTool)
	if err != nil {
		t.Fatalf("subagent tool_call failed: %v", err)
	}

	// 5. Wait past root grace period (150ms total)
	time.Sleep(150 * time.Millisecond)

	// Subagent is still busy, but root should have transitioned to IDLE.
	// Rolled up activity must show BUSY with "Running subagent..." action.
	reloaded, err := s.GetAgent(agentID)
	if err != nil || reloaded == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if reloaded.Activity.Activity != types.ActivityBusy {
		t.Errorf("expected rolled up Activity to be BUSY while subagent is active, got %v", reloaded.Activity.Activity)
	}
	if !strings.Contains(reloaded.Activity.Action, "Running subagent agent-sub-1") {
		t.Errorf("expected Action to indicate subagent execution, got %q", reloaded.Activity.Action)
	}

	// Also verify that the root activity within classifier context transitioned to IDLE
	c.mu.Lock()
	actx := c.agentCtx[agentID]
	rootAct := actx.rootActivity
	c.mu.Unlock()
	if rootAct != types.ActivityIdle {
		t.Errorf("expected rootActivity in classifier context to transition to IDLE after grace period, got %v", rootAct)
	}

	// 6. Subagent finishes with turn_end
	fSubEnd := baseFrame
	fSubEnd.Seq = 5
	fSubEnd.SubagentID = &subID
	fSubEnd.Timestamp = 1300
	fSubEnd.EventType = "turn_end"
	_, err = c.ProcessFrame(fSubEnd)
	if err != nil {
		t.Fatalf("subagent turn_end failed: %v", err)
	}

	// Since root is already IDLE and subagent is now IDLE, rolled up activity must transition to IDLE!
	reloaded, err = s.GetAgent(agentID)
	if err != nil || reloaded == nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if reloaded.Activity.Activity != types.ActivityIdle {
		t.Errorf("expected final Activity to be IDLE after subagent finished, got %v", reloaded.Activity.Activity)
	}
}

func TestClassifier_RootActivityNotOverwrittenBySubagentRollup(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-sub-pin",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-sub-pin:claude-code:sess-root-pin"

	// 1. Root agent starts and is IDLE
	agent, err := c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SessionID:   "sess-root-pin",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   1000,
		EventType:   "run_started",
	})
	if err != nil {
		t.Fatalf("run_started failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityIdle {
		t.Fatalf("expected root agent to start IDLE, got %s", agent.Activity.Activity)
	}

	// 2. Background subagent executes a tool and becomes BUSY
	subID := "sub-worker-1"
	tBash := "bash"
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SubagentID:  &subID,
		SessionID:   "sess-root-pin",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   2000,
		EventType:   "tool_call",
		Data:        types.TelemetryFrameData{ToolName: &tBash},
	})
	if err != nil {
		t.Fatalf("subagent tool_call failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected rolled-up activity to be BUSY while subagent is running, got %s", agent.Activity.Activity)
	}

	// 3. Root receives a title_change event (does not change activity)
	title := "Updated Session Title"
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SessionID:   "sess-root-pin",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   2500,
		EventType:   "title_change",
		Data:        types.TelemetryFrameData{Title: &title},
	})
	if err != nil {
		t.Fatalf("root title_change failed: %v", err)
	}
	// Still BUSY because subagent is still active
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected rolled-up activity to remain BUSY while subagent is active, got %s", agent.Activity.Activity)
	}

	// 4. Subagent emits turn_end (subagent finishes and goes IDLE)
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SubagentID:  &subID,
		SessionID:   "sess-root-pin",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   3000,
		EventType:   "turn_end",
	})
	if err != nil {
		t.Fatalf("subagent turn_end failed: %v", err)
	}

	// 5. Root activity must transition back to IDLE, not remain pinned to BUSY!
	if agent.Activity.Activity != types.ActivityIdle {
		t.Errorf("expected agent to transition to IDLE after subagent turn_end, got %s (rootActivity pinned to BUSY)", agent.Activity.Activity)
	}
}

func TestClassifier_RootActivityNotOverwrittenByIdempotentRunStarted(t *testing.T) {
	sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}
	c, _ := newTestClassifier(t, sum)
	c.SetSyncMode(true)
	defer func() { _ = c.Close() }()

	env := types.IEnvironmentInfo{
		ID:              "env-sub-pin2",
		Label:           "devbox",
		Hostname:        "devbox",
		Platform:        "linux",
		User:            "testuser",
		LastHeartbeatAt: time.Now().UnixMilli(),
	}
	ws := types.IWorkspaceInfo{Branch: "main", CWD: "/workspace"}
	proj := types.IProjectInfo{ID: "repo", Name: "repo", CanonicalRoot: "/workspace"}
	agentID := "env-sub-pin2:claude-code:sess-root-pin2"

	// 1. Root agent starts and is IDLE
	agent, err := c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		PID:         12345,
		SessionID:   "sess-root-pin2",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   1000,
		EventType:   "run_started",
	})
	if err != nil {
		t.Fatalf("run_started failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityIdle {
		t.Fatalf("expected root agent to start IDLE, got %s", agent.Activity.Activity)
	}

	// 2. Background subagent executes a tool and becomes BUSY
	subID := "sub-worker-2"
	tBash := "bash"
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SubagentID:  &subID,
		SessionID:   "sess-root-pin2",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   2000,
		EventType:   "tool_call",
		Data:        types.TelemetryFrameData{ToolName: &tBash},
	})
	if err != nil {
		t.Fatalf("subagent tool_call failed: %v", err)
	}
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected rolled-up activity to be BUSY while subagent is running, got %s", agent.Activity.Activity)
	}

	// 3. Root receives an idempotent run_started event
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		PID:         12345,
		SessionID:   "sess-root-pin2",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   2500,
		EventType:   "run_started",
	})
	if err != nil {
		t.Fatalf("idempotent run_started failed: %v", err)
	}
	// Still BUSY because subagent is still active
	if agent.Activity.Activity != types.ActivityBusy {
		t.Fatalf("expected rolled-up activity to remain BUSY while subagent is active, got %s", agent.Activity.Activity)
	}

	// 4. Subagent emits turn_end
	agent, err = c.ProcessFrame(types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     agentID,
		SubagentID:  &subID,
		SessionID:   "sess-root-pin2",
		Harness:     types.HarnessClaudeCode,
		Project:     proj,
		Workspace:   ws,
		Timestamp:   3000,
		EventType:   "turn_end",
	})
	if err != nil {
		t.Fatalf("subagent turn_end failed: %v", err)
	}

	// 5. Root activity must transition back to IDLE
	if agent.Activity.Activity != types.ActivityIdle {
		t.Errorf("expected agent to transition to IDLE after subagent turn_end, got %s (rootActivity pinned to BUSY)", agent.Activity.Activity)
	}
}

func TestClassifier_EqualPtr(t *testing.T) {
	str1 := "test"
	str2 := "test"
	str3 := "other"

	if !equalPtr[string](nil, nil) {
		t.Errorf("equalPtr(nil, nil) want true, got false")
	}
	if equalPtr(&str1, nil) {
		t.Errorf("equalPtr(&str1, nil) want false, got true")
	}
	if equalPtr(nil, &str1) {
		t.Errorf("equalPtr(nil, &str1) want false, got true")
	}
	if !equalPtr(&str1, &str2) {
		t.Errorf("equalPtr(&str1, &str2) with same value want true, got false")
	}
	if equalPtr(&str1, &str3) {
		t.Errorf("equalPtr(&str1, &str3) with diff value want false, got true")
	}

	int1 := 42
	int2 := 42
	int3 := 99
	if !equalPtr(&int1, &int2) {
		t.Errorf("equalPtr(&int1, &int2) with same int want true, got false")
	}
	if equalPtr(&int1, &int3) {
		t.Errorf("equalPtr(&int1, &int3) with diff int want false, got true")
	}
}

func TestClassifier_ShouldApplySummary(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	phasePtr := func(p types.AgentPhase) *types.AgentPhase { return &p }

	phaseA := phasePtr(types.PhaseEditing)
	phaseB := phasePtr(types.PhaseTesting)
	phaseSame := phasePtr(types.PhaseEditing)

	targetA := strPtr("main.go")
	targetB := strPtr("other.go")
	targetSame := strPtr("main.go")

	tests := []struct {
		name      string
		current   *types.IAgentActivity
		res       summarizer.SummaryResult
		wantApply bool
	}{
		{
			name:      "nil current activity should apply",
			current:   nil,
			res:       summarizer.SummaryResult{Action: "Some action"},
			wantApply: true,
		},
		{
			name: "distinct new action: applies even if phase and target are identical",
			current: &types.IAgentActivity{
				Action:       "Running tests on server",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing handler endpoints",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			wantApply: true,
		},
		{
			name: "near duplicate action with identical non-nil phase and target: suppressed (should NOT apply)",
			current: &types.IAgentActivity{
				Action:       "Editing packages/agent-ingester/internal/classifier/classifier.go",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing packages/agent-ingester/internal/classifier/classifier.go",
				Phase:        phaseSame,
				ActiveTarget: targetSame,
			},
			wantApply: false,
		},
		{
			name: "near duplicate action with identical nil phase and nil target: suppressed (should NOT apply)",
			current: &types.IAgentActivity{
				Action:       "Running test suite",
				Phase:        nil,
				ActiveTarget: nil,
			},
			res: summarizer.SummaryResult{
				Action:       "Running test suite",
				Phase:        nil,
				ActiveTarget: nil,
			},
			wantApply: false,
		},
		{
			name: "near duplicate action but phase differs (both non-nil): applies",
			current: &types.IAgentActivity{
				Action:       "Editing classifier.go implementation",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing classifier.go implementation",
				Phase:        phaseB,
				ActiveTarget: targetSame,
			},
			wantApply: true,
		},
		{
			name: "near duplicate action but phase differs (nil vs non-nil): applies",
			current: &types.IAgentActivity{
				Action:       "Editing classifier.go implementation",
				Phase:        nil,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing classifier.go implementation",
				Phase:        phaseA,
				ActiveTarget: targetSame,
			},
			wantApply: true,
		},
		{
			name: "near duplicate action but phase differs (non-nil vs nil): applies",
			current: &types.IAgentActivity{
				Action:       "Editing classifier.go implementation",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing classifier.go implementation",
				Phase:        nil,
				ActiveTarget: targetSame,
			},
			wantApply: true,
		},
		{
			name: "near duplicate action but target differs (both non-nil): applies",
			current: &types.IAgentActivity{
				Action:       "Editing packages/agent-ingester file",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing packages/agent-ingester file",
				Phase:        phaseSame,
				ActiveTarget: targetB,
			},
			wantApply: true,
		},
		{
			name: "near duplicate action but target differs (nil vs non-nil): applies",
			current: &types.IAgentActivity{
				Action:       "Editing packages/agent-ingester file",
				Phase:        phaseA,
				ActiveTarget: nil,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing packages/agent-ingester file",
				Phase:        phaseSame,
				ActiveTarget: targetA,
			},
			wantApply: true,
		},
		{
			name: "near duplicate action but target differs (non-nil vs nil): applies",
			current: &types.IAgentActivity{
				Action:       "Editing packages/agent-ingester file",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Editing packages/agent-ingester file",
				Phase:        phaseSame,
				ActiveTarget: nil,
			},
			wantApply: true,
		},
		{
			name: "near duplicate action with matching phase and matching target: suppressed",
			current: &types.IAgentActivity{
				Action:       "Processing telemetry frame batch for session",
				Phase:        phaseA,
				ActiveTarget: targetA,
			},
			res: summarizer.SummaryResult{
				Action:       "Processing telemetry frame batch for session",
				Phase:        phaseSame,
				ActiveTarget: targetSame,
			},
			wantApply: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldApplySummary(tt.current, tt.res)
			if got != tt.wantApply {
				t.Errorf("shouldApplySummary() = %v, want %v", got, tt.wantApply)
			}
		})
	}
}

func TestClassifier_ApplySummary(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	phasePtr := func(p types.AgentPhase) *types.AgentPhase { return &p }

	t.Run("updates action and phase, and updates activeTarget when activeTool is non-nil", func(t *testing.T) {
		act := types.IAgentActivity{
			Action:       "Old action",
			Phase:        phasePtr(types.PhasePlanning),
			ActiveTool:   strPtr("edit"),
			ActiveTarget: strPtr("old.go"),
		}
		res := summarizer.SummaryResult{
			Action:       "New action",
			Phase:        phasePtr(types.PhaseEditing),
			ActiveTarget: strPtr("new.go"),
		}

		applySummary(&act, res)

		if act.Action != "New action" {
			t.Errorf("Action = %q, want %q", act.Action, "New action")
		}
		if act.Phase == nil || *act.Phase != types.PhaseEditing {
			t.Errorf("Phase = %v, want %v", act.Phase, types.PhaseEditing)
		}
		if act.ActiveTarget == nil || *act.ActiveTarget != "new.go" {
			t.Errorf("ActiveTarget = %v, want new.go", act.ActiveTarget)
		}
	})

	t.Run("does not update activeTarget when activeTool is nil", func(t *testing.T) {
		act := types.IAgentActivity{
			Action:       "Old action",
			Phase:        phasePtr(types.PhasePlanning),
			ActiveTool:   nil,
			ActiveTarget: strPtr("old.go"),
		}
		res := summarizer.SummaryResult{
			Action:       "New action",
			Phase:        phasePtr(types.PhaseEditing),
			ActiveTarget: strPtr("new.go"),
		}

		applySummary(&act, res)

		if act.Action != "New action" {
			t.Errorf("Action = %q, want %q", act.Action, "New action")
		}
		if act.Phase == nil || *act.Phase != types.PhaseEditing {
			t.Errorf("Phase = %v, want %v", act.Phase, types.PhaseEditing)
		}
		if act.ActiveTarget == nil || *act.ActiveTarget != "old.go" {
			t.Errorf("ActiveTarget = %v, want old.go (unchanged when ActiveTool is nil)", act.ActiveTarget)
		}
	})

	t.Run("does not update activeTarget when res.ActiveTarget is empty or nil", func(t *testing.T) {
		act := types.IAgentActivity{
			Action:       "Old action",
			Phase:        phasePtr(types.PhasePlanning),
			ActiveTool:   strPtr("bash"),
			ActiveTarget: strPtr("original.go"),
		}
		emptyTarget := ""
		res := summarizer.SummaryResult{
			Action:       "New action",
			Phase:        phasePtr(types.PhaseEditing),
			ActiveTarget: &emptyTarget,
		}

		applySummary(&act, res)

		if act.ActiveTarget == nil || *act.ActiveTarget != "original.go" {
			t.Errorf("ActiveTarget = %v, want original.go (unchanged when res.ActiveTarget is empty)", act.ActiveTarget)
		}

		resNil := summarizer.SummaryResult{
			Action:       "New action 2",
			Phase:        phasePtr(types.PhaseTesting),
			ActiveTarget: nil,
		}
		applySummary(&act, resNil)
		if act.ActiveTarget == nil || *act.ActiveTarget != "original.go" {
			t.Errorf("ActiveTarget = %v, want original.go (unchanged when res.ActiveTarget is nil)", act.ActiveTarget)
		}
	})
}

func TestClassifier_HandleIdleTransition_AbortedGenerationDoesNotDropPendingSummary(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Working on task","phase":"editing"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	sum, err := summarizer.New(summarizer.Config{
		Providers: []summarizer.ProviderConfig{
			{Name: "mock", Provider: p, Model: "gpt-4o-mini"},
		},
		DebounceWindow: 10 * time.Second,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("summarizer.New() failed: %v", err)
	}

	c, _ := newTestClassifier(t, sum)
	c.syncMode = false
	c.SetDebounceWindow(10 * time.Second)
	c.SetIdleGracePeriod(100 * time.Millisecond)
	defer func() { _ = c.Close() }()

	proj := types.IProjectInfo{
		ID:            "proj-race",
		Name:          "repo",
		CanonicalRoot: "/repo",
	}
	ws := types.IWorkspaceInfo{
		Branch:     "main",
		IsDetached: false,
		IsWorktree: false,
		CWD:        "/repo",
	}
	env := types.IEnvironmentInfo{
		ID:       "env-race",
		Hostname: "workstation",
		Label:    "workstation",
		Platform: "linux",
	}

	agentID := "env-race:claude-code:sess-race"
	baseFrame := types.ITelemetryFrame{
		Version:        "1.0",
		EnvID:          env.ID,
		Environment:    env,
		AgentID:        agentID,
		SessionID:      "sess-race",
		PID:            100,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project:        proj,
		Workspace:      ws,
		Timestamp:      1000,
	}

	// 1. Session starts
	fStart := baseFrame
	fStart.Seq = 1
	fStart.EventType = "session_start"
	if _, err := c.ProcessFrame(fStart); err != nil {
		t.Fatalf("session_start failed: %v", err)
	}

	// 2. User prompt starts a turn -> agent becomes BUSY
	prompt := "Write tests"
	fPrompt := baseFrame
	fPrompt.Seq = 2
	fPrompt.Timestamp = 1010
	fPrompt.EventType = "turn_start"
	fPrompt.Data = types.TelemetryFrameData{
		Title:   &prompt,
		Content: &prompt,
	}
	if _, err := c.ProcessFrame(fPrompt); err != nil {
		t.Fatalf("turn_start failed: %v", err)
	}

	// 3. Root turn_end arrives (no tool use). Agent enters idle grace period.
	stopReason := "end_turn"
	respText := "I will write tests"
	fEnd := baseFrame
	fEnd.Seq = 3
	fEnd.Timestamp = 1020
	fEnd.EventType = "turn_end"
	fEnd.Data = types.TelemetryFrameData{
		StopReason: &stopReason,
		Content:    &respText,
	}
	if _, err := c.ProcessFrame(fEnd); err != nil {
		t.Fatalf("turn_end failed: %v", err)
	}

	// Capture the scheduled generation from agent context
	c.mu.Lock()
	actx := c.agentCtx[agentID]
	if actx == nil {
		c.mu.Unlock()
		t.Fatalf("expected agentContext to exist")
	}
	scheduledGen := actx.rootGeneration
	// Set lastSummaryDispatchedAt to now so subsequent tool_call falls within the 10s debounce window
	actx.lastSummaryDispatchedAt = time.Now().UnixMilli()
	c.mu.Unlock()

	// 4. Set hook: between GetAgent and second check, inject a new root frame (tool_call within debounce window)
	hookCalled := false
	c.beforeIdleSummarizeHook = func(id string) {
		hookCalled = true
		toolName := "Edit"
		toolPath := "file.go"
		fTool := baseFrame
		fTool.Seq = 4
		fTool.Timestamp = time.Now().UnixMilli()
		fTool.EventType = "tool_call"
		fTool.Data = types.TelemetryFrameData{
			ToolName: &toolName,
			ToolInput: map[string]interface{}{
				"file_path": toolPath,
			},
		}
		if _, err := c.ProcessFrame(fTool); err != nil {
			t.Fatalf("ProcessFrame inside hook failed: %v", err)
		}

		c.mu.Lock()
		inHookCtx := c.agentCtx[agentID]
		if inHookCtx.pendingCandidate == nil {
			c.mu.Unlock()
			t.Fatalf("expected tool_call to buffer pendingCandidate during hook")
		}
		if c.trailingTimers[agentID] == nil {
			c.mu.Unlock()
			t.Fatalf("expected tool_call to schedule trailing timer during hook")
		}
		c.mu.Unlock()
	}

	// 5. Trigger handleIdleTransition with the scheduled generation
	c.handleIdleTransition(agentID, scheduledGen)

	if !hookCalled {
		t.Fatalf("expected beforeIdleSummarizeHook to be called")
	}

	// 6. Verify that the aborted idle transition did NOT destroy the newer frame's pendingCandidate or cancel its trailing timer!
	c.mu.Lock()
	defer c.mu.Unlock()
	finalCtx := c.agentCtx[agentID]
	if finalCtx == nil {
		t.Fatalf("expected agentContext to exist")
	}

	if finalCtx.pendingCandidate == nil {
		t.Errorf("FAIL: aborted idle transition destroyed pendingCandidate of newer frame!")
	}
	if c.trailingTimers[agentID] == nil {
		t.Errorf("FAIL: aborted idle transition canceled trailing timer of newer frame!")
	}
	if finalCtx.rootGeneration <= scheduledGen {
		t.Errorf("expected rootGeneration > scheduledGen, got %d <= %d", finalCtx.rootGeneration, scheduledGen)
	}
}

func TestClassifier_PrepareSummaryInput_NoSideEffects(t *testing.T) {
	c, _ := newTestClassifier(t, nil)
	defer func() { _ = c.Close() }()

	agentID := "env-1:claude-code:sess-prepare-test"
	prompt := "What is the capital of France?"
	ans := "Paris"
	toolName := "Grep"
	toolArg := "test-arg"

	c.mu.Lock()
	actx := &agentContext{
		seenSubagents:           make(map[string]struct{}),
		lastPrompt:              prompt,
		lastAssistantResponse:   ans,
		lastToolArgs:            toolArg,
		lastSummaryDispatchedAt: 12345678,
		rootGeneration:          5,
		pendingCandidate: &pendingSummaryCandidate{
			timestamp: 12345678,
			toolKey:   "Grep|target|test-arg",
		},
	}
	c.agentCtx[agentID] = actx
	c.mu.Unlock()

	agent := &types.IAgentState{
		ID: agentID,
		Activity: types.IAgentActivity{
			Activity:   types.ActivityBusy,
			Action:     "Old Action",
			ActiveTool: &toolName,
		},
	}

	input, toolKey := c.prepareSummaryInput(agent)

	// Verify input fields
	if input.AgentID != agentID {
		t.Errorf("input.AgentID = %q, want %q", input.AgentID, agentID)
	}
	if input.LastPrompt != prompt {
		t.Errorf("input.LastPrompt = %q, want %q", input.LastPrompt, prompt)
	}
	if input.LastAssistantResponse != ans {
		t.Errorf("input.LastAssistantResponse = %q, want %q", input.LastAssistantResponse, ans)
	}
	if toolKey != "Grep||test-arg" {
		t.Errorf("toolKey = %q, want Grep||test-arg", toolKey)
	}

	// Verify agent activity got fallback summary applied
	if agent.Activity.Action == "Old Action" {
		t.Errorf("expected agent.Activity.Action to be updated by fallback, got %q", agent.Activity.Action)
	}

	// Verify actx side-effects: NONE!
	c.mu.Lock()
	defer c.mu.Unlock()
	if actx.pendingCandidate == nil {
		t.Errorf("prepareSummaryInput must not clear pendingCandidate")
	}
	if actx.lastSummaryDispatchedAt != 12345678 {
		t.Errorf("prepareSummaryInput must not alter lastSummaryDispatchedAt: got %d, want 12345678", actx.lastSummaryDispatchedAt)
	}
	if actx.rootGeneration != 5 {
		t.Errorf("prepareSummaryInput must not alter rootGeneration: got %d, want 5", actx.rootGeneration)
	}
}

func TestIsToolUseStopReason(t *testing.T) {
	tests := []struct {
		reason string
		want   bool
	}{
		{"tool_use", true},
		{"toolUse", true},
		{"tool_calls", true},
		{"tool_call", true},
		{"end_turn", false},
		{"stop", false},
		{"max_tokens", false},
		{"stop_sequence", false},
		{"refusal", false},
		{"", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			got := isToolUseStopReason(tt.reason)
			if got != tt.want {
				t.Errorf("isToolUseStopReason(%q) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func TestClassifier_TurnEndToolUseStopReasons(t *testing.T) {
	toolStopReasons := []string{"tool_use", "toolUse", "tool_calls", "tool_call"}

	for _, stopReason := range toolStopReasons {
		t.Run("preserves active tool and busy state for "+stopReason, func(t *testing.T) {
			sum, err := summarizer.New(summarizer.Config{NoSummarize: true})
			if err != nil {
				t.Fatalf("summarizer.New() failed: %v", err)
			}
			c, _ := newTestClassifier(t, sum)

			env := types.IEnvironmentInfo{
				ID:       "env-1",
				Hostname: "host",
				Platform: "linux",
			}
			ws := types.IWorkspaceInfo{CWD: "/workspace"}
			proj := types.IProjectInfo{ID: "proj-1", Name: "repo"}

			agentID := fmt.Sprintf("env-1:pi:sess-%s", stopReason)
			baseFrame := types.ITelemetryFrame{
				Version:     "1.0",
				EnvID:       "env-1",
				Environment: env,
				AgentID:     agentID,
				SessionID:   "sess-" + stopReason,
				PID:         1000,
				Harness:     types.HarnessPi,
				Project:     proj,
				Workspace:   ws,
				Timestamp:   1000,
				EventType:   "run_started",
			}

			if _, err := c.ProcessFrame(baseFrame); err != nil {
				t.Fatalf("ProcessFrame(run_started) failed: %v", err)
			}

			// Send tool_call to set active tool
			toolName := "Bash"
			fTool := baseFrame
			fTool.Seq = 2
			fTool.Timestamp = 1100
			fTool.EventType = "tool_call"
			fTool.Data = types.TelemetryFrameData{
				ToolName: &toolName,
			}
			agent, err := c.ProcessFrame(fTool)
			if err != nil {
				t.Fatalf("ProcessFrame(tool_call) failed: %v", err)
			}
			if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != "Bash" {
				t.Fatalf("expected ActiveTool to be Bash, got %v", agent.Activity.ActiveTool)
			}

			// Send turn_end with tool stop reason
			sr := stopReason
			fEnd := baseFrame
			fEnd.Seq = 3
			fEnd.Timestamp = 1200
			fEnd.EventType = "turn_end"
			fEnd.Data = types.TelemetryFrameData{
				StopReason: &sr,
			}

			agent, err = c.ProcessFrame(fEnd)
			if err != nil {
				t.Fatalf("ProcessFrame(turn_end) failed: %v", err)
			}

			if agent.Activity.Activity != types.ActivityBusy {
				t.Errorf("expected Activity to be BUSY for stopReason %q, got %s", stopReason, agent.Activity.Activity)
			}
			if agent.Activity.ActiveTool == nil || *agent.Activity.ActiveTool != "Bash" {
				t.Errorf("expected ActiveTool to be preserved as 'Bash' for stopReason %q, got %v", stopReason, agent.Activity.ActiveTool)
			}

			c.mu.Lock()
			_, hasIdleTimer := c.idleTimers[agentID]
			c.mu.Unlock()
			if hasIdleTimer {
				t.Errorf("expected no idle timer scheduled for tool stopReason %q, but idle timer was active", stopReason)
			}
		})
	}
}

