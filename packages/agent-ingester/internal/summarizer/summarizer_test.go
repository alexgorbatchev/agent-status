package summarizer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
)

func ptrString(s string) *string {
	return &s
}

func TestNew_DefaultsAndProviders(t *testing.T) {
	// 1. Defaults when duration/timeout <= 0 and NoSummarize
	s, err := New(Config{
		NoSummarize: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if s.debounce != DefaultDebounceWindow {
		t.Errorf("got debounce %v, want %v", s.debounce, DefaultDebounceWindow)
	}
	if s.timeout != DefaultTimeout {
		t.Errorf("got timeout %v, want %v", s.timeout, DefaultTimeout)
	}
	if !s.IsNoSummarize() {
		t.Errorf("expected IsNoSummarize = true")
	}

	// 2. nil summarizer IsNoSummarize
	var nilSum *Summarizer
	if !nilSum.IsNoSummarize() {
		t.Errorf("expected nil Summarizer.IsNoSummarize = true")
	}

	// 3. Provider initialization from env vars and config
	t.Setenv("LITELLM_BASE_URL", "https://litellm.test/v1")
	t.Setenv("LITELLM_API_KEY", "litellm-test-key")
	t.Setenv("GEMINI_API_KEY", "gemini-test-key")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")
	t.Setenv("AIR_SUMMARIZER_MODEL", "custom-model")

	s2, err := New(Config{
		DebounceWindow: 5 * time.Second,
		Timeout:        3 * time.Second,
	})
	if err != nil {
		t.Fatalf("New() with env vars error = %v", err)
	}
	if len(s2.providers) < 3 {
		t.Errorf("expected at least 3 providers initialized, got %d", len(s2.providers))
	}
	if s2.IsNoSummarize() {
		t.Errorf("expected IsNoSummarize = false with configured providers")
	}
}

func TestStructuredOutputParsing(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantAction string
		wantPhase  *types.AgentPhase
		wantTarget string
		wantErr    bool
	}{
		{
			name:       "valid JSON from PRD spec",
			raw:        `{"action":"Fixing TypeScript compilation errors in build.ts","phase":"debugging","activeTarget":"packages/registry-cli/src/commands/buildCommand.ts"}`,
			wantAction: "Fixing TypeScript compilation errors in build.ts",
			wantPhase:  func() *types.AgentPhase { p := types.PhaseDebugging; return &p }(),
			wantTarget: "packages/registry-cli/src/commands/buildCommand.ts",
			wantErr:    false,
		},
		{
			name: "wrapped in markdown code block",
			raw: "```json\n" +
				`{"action":"Writing unit tests for authentication","phase":"testing","activeTarget":"internal/auth/auth_test.go"}` +
				"\n```",
			wantAction: "Writing unit tests for authentication",
			wantPhase:  func() *types.AgentPhase { p := types.PhaseTesting; return &p }(),
			wantTarget: "internal/auth/auth_test.go",
			wantErr:    false,
		},
		{
			name: "wrapped with extra commentary",
			raw: "Here is the summary:\n" +
				`{"action":"Investigating slow database queries","phase":"investigating","activeTarget":"db/schema.sql"}` +
				"\nDone.",
			wantAction: "Investigating slow database queries",
			wantPhase:  func() *types.AgentPhase { p := types.PhaseInvestigating; return &p }(),
			wantTarget: "db/schema.sql",
			wantErr:    false,
		},
		{
			name:       "phase aliases (plan, edit, test, debug, investigate)",
			raw:        `{"action":"Creating new project structure","phase":"plan","activeTarget":""}`,
			wantAction: "Creating new project structure",
			wantPhase:  func() *types.AgentPhase { p := types.PhasePlanning; return &p }(),
			wantTarget: "",
			wantErr:    false,
		},
		{
			name:       "phase aliases (writing, fixing, search)",
			raw:        `{"action":"Writing documentation","phase":"writing","activeTarget":"docs/README.md"}`,
			wantAction: "Writing documentation",
			wantPhase:  func() *types.AgentPhase { p := types.PhaseEditing; return &p }(),
			wantTarget: "docs/README.md",
			wantErr:    false,
		},
		{
			name:       "unknown phase normalizes to nil",
			raw:        `{"action":"Something custom","phase":"unknown_phase","activeTarget":""}`,
			wantAction: "Something custom",
			wantPhase:  nil,
			wantTarget: "",
			wantErr:    false,
		},
		{
			name:    "empty response errors out",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "no json object in response",
			raw:     "Just plain text without curly braces",
			wantErr: true,
		},
		{
			name:    "invalid JSON errors out",
			raw:     `{action: not a json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseStructuredOutput(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseStructuredOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if parsed.Action != tt.wantAction {
				t.Errorf("Action = %q, want %q", parsed.Action, tt.wantAction)
			}
			phase := normalizePhase(parsed.Phase)
			if (phase == nil) != (tt.wantPhase == nil) {
				t.Errorf("Phase = %v, want %v", phase, tt.wantPhase)
			} else if phase != nil && *phase != *tt.wantPhase {
				t.Errorf("Phase = %v, want %v", *phase, *tt.wantPhase)
			}
			if parsed.ActiveTarget != tt.wantTarget {
				t.Errorf("ActiveTarget = %q, want %q", parsed.ActiveTarget, tt.wantTarget)
			}
		})
	}
}

func TestDeterministicFallbackSummary(t *testing.T) {
	tests := []struct {
		name       string
		input      SummaryInput
		wantAction string
		wantPhase  types.AgentPhase
	}{
		{
			name: "edit tool with target",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("Edit"),
				ActiveTarget: ptrString("src/build.ts"),
			},
			wantAction: "Editing src/build.ts",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "edit tool without target",
			input: SummaryInput{
				Activity:   types.ActivityBusy,
				ActiveTool: ptrString("Edit"),
			},
			wantAction: "Editing code",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "notebook edit tool",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("NotebookEdit"),
				ActiveTarget: ptrString("analysis.ipynb"),
			},
			wantAction: "Editing analysis.ipynb",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "write tool with target",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("Write"),
				ActiveTarget: ptrString("config.json"),
			},
			wantAction: "Writing config.json",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "write tool without target",
			input: SummaryInput{
				Activity:   types.ActivityBusy,
				ActiveTool: ptrString("Write"),
			},
			wantAction: "Writing file",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "read tool with target",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("Read"),
				ActiveTarget: ptrString("README.md"),
			},
			wantAction: "Reading README.md",
			wantPhase:  types.PhaseInvestigating,
		},
		{
			name: "read tool without target",
			input: SummaryInput{
				Activity:   types.ActivityBusy,
				ActiveTool: ptrString("Read"),
			},
			wantAction: "Reading file",
			wantPhase:  types.PhaseInvestigating,
		},
		{
			name: "bash tool running tests",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("Bash"),
				ActiveTarget: ptrString("go test -v ./..."),
			},
			wantAction: "Running tests",
			wantPhase:  types.PhaseTesting,
		},
		{
			name: "bash tool running pytest",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("Bash"),
				ActiveTarget: ptrString("pytest tests/"),
			},
			wantAction: "Running tests",
			wantPhase:  types.PhaseTesting,
		},
		{
			name: "bash tool running general command",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("Bash"),
				ActiveTarget: ptrString("git status"),
			},
			wantAction: "Running git status",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "bash tool without target",
			input: SummaryInput{
				Activity:   types.ActivityBusy,
				ActiveTool: ptrString("Bash"),
			},
			wantAction: "Running shell command",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "ask user question tool",
			input: SummaryInput{
				Activity:   types.ActivityBusy,
				ActiveTool: ptrString("AskUserQuestion"),
			},
			wantAction: "Waiting for user input",
			wantPhase:  types.PhasePlanning,
		},
		{
			name: "investigating tools (WebSearch with target)",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("WebSearch"),
				ActiveTarget: ptrString("golang sqlite driver"),
			},
			wantAction: "Searching golang sqlite driver",
			wantPhase:  types.PhaseInvestigating,
		},
		{
			name: "investigating tools (ast-grep without target)",
			input: SummaryInput{
				Activity:   types.ActivityBusy,
				ActiveTool: ptrString("ast-grep"),
			},
			wantAction: "Investigating codebase",
			wantPhase:  types.PhaseInvestigating,
		},
		{
			name: "default tool with target",
			input: SummaryInput{
				Activity:     types.ActivityBusy,
				ActiveTool:   ptrString("CustomTool"),
				ActiveTarget: ptrString("target-item"),
			},
			wantAction: "Using CustomTool on target-item",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "default tool without target",
			input: SummaryInput{
				Activity:   types.ActivityBusy,
				ActiveTool: ptrString("CustomTool"),
			},
			wantAction: "Using CustomTool",
			wantPhase:  types.PhaseEditing,
		},
		{
			name: "waiting question activity without active tool",
			input: SummaryInput{
				Activity: types.ActivityWaitingQuestion,
			},
			wantAction: "Waiting for user input",
			wantPhase:  types.PhasePlanning,
		},
		{
			name: "error activity without active tool",
			input: SummaryInput{
				Activity: types.ActivityError,
			},
			wantAction: "Encountered error",
			wantPhase:  types.PhaseDebugging,
		},
		{
			name: "idle activity",
			input: SummaryInput{
				Activity: types.ActivityIdle,
			},
			wantAction: "Idle",
			wantPhase:  types.PhasePlanning, // nil phase in fallback gets no phase check or Idle
		},
		{
			name: "direct assistant response without tools",
			input: SummaryInput{
				Activity:              types.ActivityIdle,
				LastAssistantResponse: "I have explained the architecture.",
			},
			wantAction: "Responded to user",
			wantPhase:  types.PhasePlanning,
		},
		{
			name: "direct assistant response on busy activity without tools",
			input: SummaryInput{
				Activity:              types.ActivityBusy,
				LastAssistantResponse: "The requested changes are summarized below.",
			},
			wantAction: "Responded to user",
			wantPhase:  types.PhasePlanning,
		},
		{
			name: "thinking activity",
			input: SummaryInput{
				Activity:            types.ActivityBusy,
				LastThinkingExcerpt: "Considering how to restructure the database tables...",
			},
			wantAction: "Thinking",
			wantPhase:  types.PhasePlanning,
		},
		{
			name: "busy working without tool or thinking",
			input: SummaryInput{
				Activity: types.ActivityBusy,
			},
			wantAction: "Working",
			wantPhase:  types.PhasePlanning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := FallbackSummary(tt.input)
			if res.Action != tt.wantAction {
				t.Errorf("got Action %q, want %q", res.Action, tt.wantAction)
			}
			if !res.IsFallback {
				t.Errorf("expected IsFallback = true, got false")
			}
		})
	}
}

func TestBuildUserPrompt_TruncationAndDetails(t *testing.T) {
	longPrompt := strings.Repeat("A", 1500)
	longThinking := strings.Repeat("B", 1500)
	longError := strings.Repeat("C", 500)
	longResp := strings.Repeat("D", 1500)
	q := "Would you like to deploy now?"

	recentTools := []ToolExecution{
		{ToolName: "Tool1", Target: "target1"},
		{ToolName: "Tool2", Target: ""},
		{ToolName: "Tool3", Target: "target3"},
		{ToolName: "Tool4", Target: "target4"},
	}

	input := SummaryInput{
		AgentID:               "agent-trunc",
		LastPrompt:            longPrompt,
		LastAssistantResponse: longResp,
		RecentTools:           recentTools,
		LastThinkingExcerpt:   longThinking,
		Activity:              types.ActivityBusy,
		ActiveTool:            ptrString("Edit"),
		ActiveTarget:          ptrString("main.go"),
		LastError:             &longError,
		Question:              &q,
	}

	prompt := buildUserPrompt(input)
	if !strings.Contains(prompt, "User Prompt: ") || !strings.Contains(prompt, "...") {
		t.Errorf("expected truncated prompt")
	}
	if !strings.Contains(prompt, "Assistant Response: ") {
		t.Errorf("expected assistant response in prompt")
	}
	if !strings.Contains(prompt, "Active Tool: Edit (Target: main.go)") {
		t.Errorf("expected active tool with target in prompt, got %s", prompt)
	}
	if !strings.Contains(prompt, "- Tool4 on target4") || !strings.Contains(prompt, "- Tool2") {
		t.Errorf("expected recent tools in prompt, got %s", prompt)
	}
	if strings.Contains(prompt, "- Tool1") {
		t.Errorf("expected Tool1 to be truncated by MaxRecentTools=3")
	}
	if !strings.Contains(prompt, "Question Asked: Would you like to deploy now?") {
		t.Errorf("expected question in prompt")
	}
	if !strings.Contains(prompt, "Last Error: ") {
		t.Errorf("expected last error in prompt")
	}
}

func TestDebounceRateLimiting(t *testing.T) {
	s, err := New(Config{
		NoSummarize:    true,
		DebounceWindow: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	agentID := "agent-rate-limit-test"

	if !s.Allow(agentID) {
		t.Errorf("first Allow() should return true")
	}

	if s.Allow(agentID) {
		t.Errorf("immediate second Allow() should return false")
	}

	time.Sleep(60 * time.Millisecond)

	if !s.Allow(agentID) {
		t.Errorf("Allow() after debounce window should return true")
	}
}

func TestSummarizer_WithMockOpenAIServer(t *testing.T) {
	var requestCount int64

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-123",
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

	s, err := New(Config{
		Providers: []ProviderConfig{
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
		t.Fatalf("New() failed: %v", err)
	}

	input := SummaryInput{
		AgentID:    "agent-1",
		LastPrompt: "Fix TypeScript errors in build.ts",
		Activity:   types.ActivityBusy,
		RecentTools: []ToolExecution{
			{ToolName: "Edit", Target: "packages/registry-cli/src/commands/buildCommand.ts"},
		},
		ActiveTool:   ptrString("Edit"),
		ActiveTarget: ptrString("packages/registry-cli/src/commands/buildCommand.ts"),
	}

	res := s.Summarize(context.Background(), input)
	if res.IsFallback {
		t.Fatalf("expected non-fallback result, got fallback: %+v", res)
	}
	if res.Action != "Fixing TypeScript compilation errors in build.ts" {
		t.Errorf("got Action %q, want %q", res.Action, "Fixing TypeScript compilation errors in build.ts")
	}
	if res.Phase == nil || *res.Phase != types.PhaseDebugging {
		t.Errorf("got Phase %v, want %v", res.Phase, types.PhaseDebugging)
	}
	if res.ActiveTarget == nil || *res.ActiveTarget != "packages/registry-cli/src/commands/buildCommand.ts" {
		t.Errorf("got ActiveTarget %v, want packages/registry-cli/src/commands/buildCommand.ts", res.ActiveTarget)
	}
	if atomic.LoadInt64(&requestCount) != 1 {
		t.Errorf("expected 1 request to mock server, got %d", requestCount)
	}
}

func TestSummarizer_ProviderFailover(t *testing.T) {
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	t.Cleanup(primaryServer.Close)

	backupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":     "chatcmpl-456",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"action":"Refactoring auth middleware","phase":"editing","activeTarget":"internal/auth/middleware.go"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(backupServer.Close)

	pPrimary, err := openai.New(
		anyllm.WithBaseURL(primaryServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New(primary) failed: %v", err)
	}

	pBackup, err := openai.New(
		anyllm.WithBaseURL(backupServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New(backup) failed: %v", err)
	}

	s, err := New(Config{
		Providers: []ProviderConfig{
			{
				Name:     "primary-openai",
				Provider: pPrimary,
				Model:    "gpt-4o-mini",
			},
			{
				Name:     "backup-openai",
				Provider: pBackup,
				Model:    "gpt-4o-mini",
			},
		},
		DebounceWindow: 10 * time.Millisecond,
		Timeout:        2 * time.Second,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	input := SummaryInput{
		AgentID:  "agent-failover",
		Activity: types.ActivityBusy,
	}

	res := s.Summarize(context.Background(), input)
	if res.IsFallback {
		t.Fatalf("expected failover to succeed, got fallback: %+v", res)
	}
	if res.Action != "Refactoring auth middleware" {
		t.Errorf("got Action %q, want %q", res.Action, "Refactoring auth middleware")
	}
	if res.Phase == nil || *res.Phase != types.PhaseEditing {
		t.Errorf("got Phase %v, want %v", res.Phase, types.PhaseEditing)
	}
}

func TestSummarizer_ContextCanceledDuringFailover(t *testing.T) {
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	t.Cleanup(primaryServer.Close)

	pPrimary, _ := openai.New(
		anyllm.WithBaseURL(primaryServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)

	s, err := New(Config{
		Providers: []ProviderConfig{
			{Name: "p1", Provider: pPrimary, Model: "m1"},
			{Name: "p2", Provider: pPrimary, Model: "m2"},
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	input := SummaryInput{
		AgentID:  "agent-cancel",
		Activity: types.ActivityBusy,
	}

	res := s.Summarize(ctx, input)
	if !res.IsFallback {
		t.Errorf("expected fallback on canceled context")
	}
}

func TestSummarizer_NoSummarizeFlag(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("mock server should not be called when NoSummarize=true")
	}))
	t.Cleanup(mockServer.Close)

	p, err := openai.New(
		anyllm.WithBaseURL(mockServer.URL+"/v1"),
		anyllm.WithAPIKey("test-key"),
	)
	if err != nil {
		t.Fatalf("openai.New() failed: %v", err)
	}

	s, err := New(Config{
		NoSummarize: true,
		Providers: []ProviderConfig{
			{
				Name:     "mock-openai",
				Provider: p,
				Model:    "gpt-4o-mini",
			},
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	input := SummaryInput{
		AgentID:      "agent-no-sum",
		Activity:     types.ActivityBusy,
		ActiveTool:   ptrString("Edit"),
		ActiveTarget: ptrString("build.ts"),
	}

	res := s.Summarize(context.Background(), input)
	if !res.IsFallback {
		t.Errorf("expected IsFallback = true when NoSummarize is true")
	}
	if res.Action != "Editing build.ts" {
		t.Errorf("got Action %q, want %q", res.Action, "Editing build.ts")
	}
}
