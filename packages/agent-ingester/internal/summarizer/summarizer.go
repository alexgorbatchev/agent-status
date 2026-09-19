package summarizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/anthropic"
	"github.com/mozilla-ai/any-llm-go/providers/gemini"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
	"golang.org/x/time/rate"
)

const (
	DefaultDebounceWindow = 12 * time.Second
	DefaultTimeout        = 10 * time.Second
	MaxThinkingLength     = 1200 // ~300 tokens
	MaxPromptLength       = 1000
	MaxRecentTools        = 3
)

type ToolExecution struct {
	ToolName string `json:"toolName"`
	Target   string `json:"target"`
}

type SummaryInput struct {
	AgentID             string
	LastPrompt          string
	RecentTools         []ToolExecution
	LastThinkingExcerpt string
	Activity            types.ActivityState
	ActiveTool          *string
	ActiveTarget        *string
	LastError           *string
	Question            *string
}

type SummaryResult struct {
	Action       string            `json:"action"`
	Phase        *types.AgentPhase `json:"phase,omitempty"`
	ActiveTarget *string           `json:"activeTarget,omitempty"`
	IsFallback   bool              `json:"isFallback"`
}

type structuredSummary struct {
	Action       string `json:"action"`
	Phase        string `json:"phase"`
	ActiveTarget string `json:"activeTarget"`
}

type ProviderConfig struct {
	Name     string
	Provider anyllm.Provider
	Model    string
}

type Config struct {
	NoSummarize     bool
	DebounceWindow  time.Duration
	Timeout         time.Duration
	LiteLLMBaseURL  string
	LiteLLMAPIKey   string
	LiteLLMModel    string
	OpenAIBaseURL   string
	OpenAIAPIKey    string
	OpenAIModel     string
	GeminiAPIKey    string
	GeminiModel     string
	AnthropicAPIKey string
	AnthropicModel  string
	Providers       []ProviderConfig
}

type Summarizer struct {
	providers   []ProviderConfig
	limiters    map[string]*rate.Limiter
	mu          sync.Mutex
	debounce    time.Duration
	timeout     time.Duration
	noSummarize bool
}

func New(cfg Config) (*Summarizer, error) {
	debounce := cfg.DebounceWindow
	if debounce <= 0 {
		debounce = DefaultDebounceWindow
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	s := &Summarizer{
		limiters:    make(map[string]*rate.Limiter),
		debounce:    debounce,
		timeout:     timeout,
		noSummarize: cfg.NoSummarize,
	}

	if len(cfg.Providers) > 0 {
		s.providers = cfg.Providers
		return s, nil
	}

	if cfg.NoSummarize {
		return s, nil
	}

	// 1. OpenAI / LiteLLM Proxy
	liteLLMBaseURL := cfg.LiteLLMBaseURL
	if liteLLMBaseURL == "" {
		liteLLMBaseURL = os.Getenv("LITELLM_BASE_URL")
	}
	if liteLLMBaseURL == "" {
		liteLLMBaseURL = cfg.OpenAIBaseURL
	}
	if liteLLMBaseURL == "" {
		liteLLMBaseURL = os.Getenv("OPENAI_BASE_URL")
	}

	liteLLMKey := cfg.LiteLLMAPIKey
	if liteLLMKey == "" {
		liteLLMKey = os.Getenv("LITELLM_API_KEY")
	}
	if liteLLMKey == "" {
		liteLLMKey = cfg.OpenAIAPIKey
	}
	if liteLLMKey == "" {
		liteLLMKey = os.Getenv("OPENAI_API_KEY")
	}

	if liteLLMBaseURL != "" || liteLLMKey != "" {
		opts := make([]anyllm.Option, 0, 2)
		if liteLLMBaseURL != "" {
			opts = append(opts, anyllm.WithBaseURL(liteLLMBaseURL))
		}
		if liteLLMKey != "" {
			opts = append(opts, anyllm.WithAPIKey(liteLLMKey))
		}
		p, err := openai.New(opts...)
		if err == nil && p != nil {
			model := cfg.LiteLLMModel
			if model == "" {
				model = cfg.OpenAIModel
			}
			if model == "" {
				model = os.Getenv("AIR_SUMMARIZER_MODEL")
			}
			if model == "" {
				model = "gpt-4o-mini"
			}
			s.providers = append(s.providers, ProviderConfig{
				Name:     "openai",
				Provider: p,
				Model:    model,
			})
		}
	}

	// 2. Google Gemini
	geminiKey := cfg.GeminiAPIKey
	if geminiKey == "" {
		geminiKey = os.Getenv("GEMINI_API_KEY")
	}
	if geminiKey != "" {
		p, err := gemini.New(anyllm.WithAPIKey(geminiKey))
		if err == nil && p != nil {
			model := cfg.GeminiModel
			if model == "" {
				model = "gemini-2.0-flash"
			}
			s.providers = append(s.providers, ProviderConfig{
				Name:     "gemini",
				Provider: p,
				Model:    model,
			})
		}
	}

	// 3. Anthropic Claude
	anthropicKey := cfg.AnthropicAPIKey
	if anthropicKey == "" {
		anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if anthropicKey != "" {
		p, err := anthropic.New(anyllm.WithAPIKey(anthropicKey))
		if err == nil && p != nil {
			model := cfg.AnthropicModel
			if model == "" {
				model = "claude-3-5-haiku-latest"
			}
			s.providers = append(s.providers, ProviderConfig{
				Name:     "anthropic",
				Provider: p,
				Model:    model,
			})
		}
	}

	return s, nil
}

// IsNoSummarize returns whether external LLM calls are disabled or unconfigured.
func (s *Summarizer) IsNoSummarize() bool {
	if s == nil {
		return true
	}
	return s.noSummarize || len(s.providers) == 0
}

// Allow checks the per-agent rate limiter for the 12-second debounce window.
func (s *Summarizer) Allow(agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	lim, exists := s.limiters[agentID]
	if !exists {
		lim = rate.NewLimiter(rate.Every(s.debounce), 1)
		s.limiters[agentID] = lim
	}

	return lim.Allow()
}

// Summarize generates a structured action summary using configured LLM providers with automatic fallback.
func (s *Summarizer) Summarize(ctx context.Context, input SummaryInput) SummaryResult {
	if s.noSummarize || len(s.providers) == 0 {
		return FallbackSummary(input)
	}

	parsed, err := s.callWithFailover(ctx, input)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Action) == "" {
		return FallbackSummary(input)
	}

	phase := normalizePhase(parsed.Phase)
	if phase == nil {
		fb := FallbackSummary(input)
		phase = fb.Phase
	}

	var activeTarget *string
	if strings.TrimSpace(parsed.ActiveTarget) != "" {
		t := strings.TrimSpace(parsed.ActiveTarget)
		activeTarget = &t
	} else if input.ActiveTarget != nil {
		activeTarget = input.ActiveTarget
	}

	return SummaryResult{
		Action:       strings.TrimSpace(parsed.Action),
		Phase:        phase,
		ActiveTarget: activeTarget,
		IsFallback:   false,
	}
}

func (s *Summarizer) callWithFailover(ctx context.Context, input SummaryInput) (*structuredSummary, error) {
	var lastErr error

	systemPrompt := `You are an AI activity summarizer for a software engineering agent dashboard.
Analyze the agent's prompt, tools, thoughts, and state, and respond with ONLY a single JSON object:
{"action":"<concise phrase in present continuous tense describing current task, max 100 chars, e.g. 'Fixing TypeScript compilation errors in build.ts'>","phase":"<one of: planning, editing, testing, debugging, investigating>","activeTarget":"<path or target resource, or empty string if none>"}
Do not output any reasoning, markdown formatting, or preamble.`

	userPrompt := buildUserPrompt(input)

	for i, pCfg := range s.providers {
		callCtx, cancel := context.WithTimeout(ctx, s.timeout)
		defer cancel()

		temp := 0.1
		maxTokens := 150
		params := anyllm.CompletionParams{
			Model: pCfg.Model,
			Messages: []anyllm.Message{
				{
					Role:    anyllm.RoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    anyllm.RoleUser,
					Content: userPrompt,
				},
			},
			Temperature: &temp,
			MaxTokens:   &maxTokens,
		}

		resp, err := pCfg.Provider.Completion(callCtx, params)
		if err == nil && resp != nil && len(resp.Choices) > 0 {
			content := resp.Choices[0].Message.ContentString()
			parsed, parseErr := parseStructuredOutput(content)
			if parseErr == nil && parsed != nil && strings.TrimSpace(parsed.Action) != "" {
				return parsed, nil
			}
			lastErr = fmt.Errorf("parsing output from provider %s: %w", pCfg.Name, parseErr)
		} else {
			lastErr = fmt.Errorf("provider %s completion failed: %w", pCfg.Name, err)
		}

		// Jittered backoff before next provider if not the last one
		if i < len(s.providers)-1 {
			jitter := time.Duration(10+rand.Intn(40)) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(jitter):
			}
		}
	}

	return nil, lastErr
}

func buildUserPrompt(input SummaryInput) string {
	var sb strings.Builder

	if input.LastPrompt != "" {
		p := input.LastPrompt
		if len(p) > MaxPromptLength {
			p = p[:MaxPromptLength] + "..."
		}
		sb.WriteString("User Prompt: ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}

	sb.WriteString("Current Activity State: ")
	sb.WriteString(string(input.Activity))
	sb.WriteString("\n")

	if input.ActiveTool != nil && *input.ActiveTool != "" {
		sb.WriteString("Active Tool: ")
		sb.WriteString(*input.ActiveTool)
		if input.ActiveTarget != nil && *input.ActiveTarget != "" {
			sb.WriteString(" (Target: ")
			sb.WriteString(*input.ActiveTarget)
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}

	if len(input.RecentTools) > 0 {
		sb.WriteString("Recent Tool Calls:\n")
		start := 0
		if len(input.RecentTools) > MaxRecentTools {
			start = len(input.RecentTools) - MaxRecentTools
		}
		for _, t := range input.RecentTools[start:] {
			if t.Target != "" {
				sb.WriteString(fmt.Sprintf("- %s on %s\n", t.ToolName, t.Target))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", t.ToolName))
			}
		}
	}

	if input.LastThinkingExcerpt != "" {
		th := input.LastThinkingExcerpt
		if len(th) > MaxThinkingLength {
			th = th[:MaxThinkingLength] + "..."
		}
		sb.WriteString("Thinking: ")
		sb.WriteString(th)
		sb.WriteString("\n")
	}

	if input.Question != nil && *input.Question != "" {
		sb.WriteString("Question Asked: ")
		sb.WriteString(*input.Question)
		sb.WriteString("\n")
	}

	if input.LastError != nil && *input.LastError != "" {
		lErr := *input.LastError
		if len(lErr) > 300 {
			lErr = lErr[:300] + "..."
		}
		sb.WriteString("Last Error: ")
		sb.WriteString(lErr)
		sb.WriteString("\n")
	}

	return sb.String()
}

func parseStructuredOutput(raw string) (*structuredSummary, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("empty response")
	}

	// Strip markdown code fences if present
	if strings.Contains(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		var inside bool
		var jsonLines []string
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inside = !inside
				continue
			}
			if inside {
				jsonLines = append(jsonLines, line)
			}
		}
		if len(jsonLines) > 0 {
			trimmed = strings.Join(jsonLines, "\n")
		}
	}

	// Locate outer JSON object
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, errors.New("no json object found in response")
	}

	jsonStr := trimmed[start : end+1]

	var res structuredSummary
	if err := json.Unmarshal([]byte(jsonStr), &res); err != nil {
		return nil, fmt.Errorf("unmarshaling json: %w", err)
	}

	return &res, nil
}

func normalizePhase(raw string) *types.AgentPhase {
	p := strings.ToLower(strings.TrimSpace(raw))
	switch p {
	case "planning", "plan":
		phase := types.PhasePlanning
		return &phase
	case "editing", "edit", "writing", "write":
		phase := types.PhaseEditing
		return &phase
	case "testing", "test":
		phase := types.PhaseTesting
		return &phase
	case "debugging", "debug", "fix", "fixing":
		phase := types.PhaseDebugging
		return &phase
	case "investigating", "investigate", "reading", "search", "searching":
		phase := types.PhaseInvestigating
		return &phase
	default:
		return nil
	}
}

// FallbackSummary produces deterministic tool and activity descriptions.
func FallbackSummary(input SummaryInput) SummaryResult {
	var (
		action       string
		phase        *types.AgentPhase
		activeTarget *string
	)

	if input.ActiveTarget != nil && *input.ActiveTarget != "" {
		activeTarget = input.ActiveTarget
	}

	if input.ActiveTool != nil && *input.ActiveTool != "" {
		tool := *input.ActiveTool
		target := ""
		if activeTarget != nil {
			target = *activeTarget
		}

		switch tool {
		case "Edit", "NotebookEdit":
			p := types.PhaseEditing
			phase = &p
			if target != "" {
				action = fmt.Sprintf("Editing %s", target)
			} else {
				action = "Editing code"
			}

		case "Write":
			p := types.PhaseEditing
			phase = &p
			if target != "" {
				action = fmt.Sprintf("Writing %s", target)
			} else {
				action = "Writing file"
			}

		case "Read":
			p := types.PhaseInvestigating
			phase = &p
			if target != "" {
				action = fmt.Sprintf("Reading %s", target)
			} else {
				action = "Reading file"
			}

		case "Bash":
			if strings.Contains(strings.ToLower(target), "test") || strings.Contains(strings.ToLower(target), "jest") || strings.Contains(strings.ToLower(target), "pytest") {
				p := types.PhaseTesting
				phase = &p
				action = "Running tests"
			} else if target != "" {
				p := types.PhaseEditing
				phase = &p
				action = fmt.Sprintf("Running %s", target)
			} else {
				p := types.PhaseEditing
				phase = &p
				action = "Running shell command"
			}

		case "AskUserQuestion":
			p := types.PhasePlanning
			phase = &p
			action = "Waiting for user input"

		case "WebSearch", "WebFetch", "ast-grep", "codegraph", "grep", "find":
			p := types.PhaseInvestigating
			phase = &p
			if target != "" {
				action = fmt.Sprintf("Searching %s", target)
			} else {
				action = "Investigating codebase"
			}

		default:
			p := types.PhaseEditing
			phase = &p
			if target != "" {
				action = fmt.Sprintf("Using %s on %s", tool, target)
			} else {
				action = fmt.Sprintf("Using %s", tool)
			}
		}
	} else {
		switch input.Activity {
		case types.ActivityWaitingQuestion:
			p := types.PhasePlanning
			phase = &p
			action = "Waiting for user input"

		case types.ActivityError:
			p := types.PhaseDebugging
			phase = &p
			action = "Encountered error"

		case types.ActivityIdle:
			action = "Idle"

		default:
			if input.LastThinkingExcerpt != "" {
				p := types.PhasePlanning
				phase = &p
				action = "Thinking"
			} else {
				action = "Working"
			}
		}
	}

	return SummaryResult{
		Action:       action,
		Phase:        phase,
		ActiveTarget: activeTarget,
		IsFallback:   true,
	}
}
