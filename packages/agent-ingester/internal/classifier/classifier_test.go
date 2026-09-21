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

	return New(s, p, sum), s
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

	// 6. tool_call -> AskUserQuestion -> WAITING_QUESTION
	q := "Should we proceed with migration?"
	toolName := "AskUserQuestion"
	f2 := baseFrame
	f2.Seq = 6
	f2.EventType = "tool_call"
	f2.Data = types.TelemetryFrameData{
		ToolName: &toolName,
		Question: &q,
	}
	agent, err = c.ProcessFrame(f2)
	if err != nil {
		t.Fatalf("ProcessFrame(tool_call) error = %v", err)
	}
	if agent.Activity.Activity != types.ActivityWaitingQuestion {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityWaitingQuestion)
	}
	if agent.Activity.Question == nil || *agent.Activity.Question != q {
		t.Errorf("got question %v, want %s", agent.Activity.Question, q)
	}
	if agent.Activity.Action != "Waiting for user input" {
		t.Errorf("got Action %q, want 'Waiting for user input'", agent.Activity.Action)
	}

	// 7. tool_result with isError = true -> ERROR
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
	if agent.Activity.Activity != types.ActivityError {
		t.Errorf("got activity %s, want %s", agent.Activity.Activity, types.ActivityError)
	}
	if agent.Activity.LastError == nil || *agent.Activity.LastError != errMsg {
		t.Errorf("got lastError %v, want %s", agent.Activity.LastError, errMsg)
	}

	// 8. tool_result with isError = false -> Busy
	isErrFalse := false
	okOut := "success"
	fToolResOk := baseFrame
	fToolResOk.Seq = 8
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

	// 12. run_exited -> EXITED
	f7 := baseFrame
	f7.Seq = 12
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
