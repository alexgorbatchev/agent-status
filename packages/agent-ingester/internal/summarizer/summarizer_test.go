package summarizer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/types"
	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
)

func ptrString(s string) *string {
	return &s
}

func TestStructuredOutputParsing(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantAction   string
		wantPhase    *types.AgentPhase
		wantTarget   string
		wantErr      bool
	}{
		{
			name: "valid JSON from PRD spec",
			raw:  `{"action":"Fixing TypeScript compilation errors in build.ts","phase":"debugging","activeTarget":"packages/registry-cli/src/commands/buildCommand.ts"}`,
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
			name: "phase aliases (plan, edit, test, debug, investigate)",
			raw:  `{"action":"Creating new project structure","phase":"plan","activeTarget":""}`,
			wantAction: "Creating new project structure",
			wantPhase:  func() *types.AgentPhase { p := types.PhasePlanning; return &p }(),
			wantTarget: "",
			wantErr:    false,
		},
		{
			name:       "empty response errors out",
			raw:        "",
			wantErr:    true,
		},
		{
			name:       "invalid JSON errors out",
			raw:        `{action: not a json}`,
			wantErr:    true,
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
			name: "waiting question",
			input: SummaryInput{
				Activity: types.ActivityWaitingQuestion,
			},
			wantAction: "Waiting for user input",
			wantPhase:  types.PhasePlanning,
		},
		{
			name: "error activity",
			input: SummaryInput{
				Activity: types.ActivityError,
			},
			wantAction: "Encountered error",
			wantPhase:  types.PhaseDebugging,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := FallbackSummary(tt.input)
			if res.Action != tt.wantAction {
				t.Errorf("got Action %q, want %q", res.Action, tt.wantAction)
			}
			if res.Phase == nil || *res.Phase != tt.wantPhase {
				t.Errorf("got Phase %v, want %v", res.Phase, tt.wantPhase)
			}
			if !res.IsFallback {
				t.Errorf("expected IsFallback = true, got false")
			}
		})
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

	// First call should be allowed
	if !s.Allow(agentID) {
		t.Errorf("first Allow() should return true")
	}

	// Immediate second call should be denied (debounced)
	if s.Allow(agentID) {
		t.Errorf("immediate second Allow() should return false")
	}

	// Wait for debounce window to elapse
	time.Sleep(60 * time.Millisecond)

	// Should now be allowed again
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
	// Primary server fails with 500 error
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	t.Cleanup(primaryServer.Close)

	// Backup server succeeds
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
