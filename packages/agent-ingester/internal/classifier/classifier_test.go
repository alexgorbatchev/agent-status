package classifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	t.Cleanup(func() { s.Close() })

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

	c, _ := newTestClassifier(t, sum)

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

	agent, err := c.ProcessFrame(f)
	if err != nil {
		t.Fatalf("ProcessFrame() failed: %v", err)
	}

	if agent.Activity.Action != "Fixing TypeScript compilation errors in build.ts" {
		t.Errorf("got Action %q, want 'Fixing TypeScript compilation errors in build.ts'", agent.Activity.Action)
	}
	if agent.Activity.Phase == nil || *agent.Activity.Phase != types.PhaseDebugging {
		t.Errorf("got Phase %v, want debugging", agent.Activity.Phase)
	}
	if agent.Activity.ActiveTarget == nil || *agent.Activity.ActiveTarget != "packages/registry-cli/src/commands/buildCommand.ts" {
		t.Errorf("got ActiveTarget %v, want 'packages/registry-cli/src/commands/buildCommand.ts'", agent.Activity.ActiveTarget)
	}
	if agent.Activity.ActionStale {
		t.Errorf("expected ActionStale = false after successful summarization")
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

	env := types.IEnvironmentInfo{
		ID:              "env-debounce",
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
	s.Close()

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
	agent, err := c.ProcessFrame(baseFrame)
	if err != nil {
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

	agent, err = c.ProcessFrame(fTurn1)
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
