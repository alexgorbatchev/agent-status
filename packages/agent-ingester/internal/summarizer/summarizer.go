package summarizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/anthropic"
	"github.com/mozilla-ai/any-llm-go/providers/gemini"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
	"golang.org/x/time/rate"
)

const (
	DefaultDebounceWindow      = 10 * time.Second
	DefaultTimeout             = 10 * time.Second
	DefaultSimilarityThreshold = 0.60
	MaxThinkingLength          = 1200 // ~300 tokens
	MaxPromptLength            = 1000
	MaxToolArgsLength          = 300
	MaxErrorLength             = 300
	MaxRecentTools             = 3
)

type ToolExecution struct {
	ToolName string `json:"toolName"`
	Target   string `json:"target"`
}

type SummaryInput struct {
	AgentID               string
	CurrentAction         string
	LastPrompt            string
	LastAssistantResponse string
	RecentTools           []ToolExecution
	LastThinkingExcerpt   string
	Activity              types.ActivityState
	ActiveTool            *string
	ActiveTarget          *string
	ActiveToolArgs        *string
	ActiveToolInput       map[string]interface{}
	LastError             *string
	Question              *string
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
	Endpoint string
}

// ProviderInfo represents public metadata about an active LLM summarizer provider.
type ProviderInfo struct {
	Name     string `json:"name"`
	Model    string `json:"model"`
	Endpoint string `json:"endpoint,omitempty"`
}

type Config struct {
	NoSummarize       bool
	NoSummarizeByFlag bool
	NoSummarizeByEnv  bool
	NoSummarizeSource string
	DebounceWindow    time.Duration
	Timeout           time.Duration
	LiteLLMBaseURL    string
	LiteLLMAPIKey     string
	LiteLLMModel      string
	GeminiAPIKey      string
	GeminiModel       string
	AnthropicAPIKey   string
	AnthropicModel    string
	Providers         []ProviderConfig
	InitErrors        map[string]error
}

type Summarizer struct {
	providers         []ProviderConfig
	initErrors        map[string]error
	limiters          map[string]*rate.Limiter
	mu                sync.Mutex
	debounce          time.Duration
	timeout           time.Duration
	noSummarize       bool
	noSummarizeSource string
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

	noSummarizeSource := cfg.NoSummarizeSource
	if noSummarizeSource == "" {
		if cfg.NoSummarizeByEnv {
			noSummarizeSource = "env"
		} else if cfg.NoSummarizeByFlag {
			noSummarizeSource = "flag"
		} else if cfg.NoSummarize {
			noSummarizeSource = "flag"
		}
	}

	s := &Summarizer{
		initErrors:        make(map[string]error),
		limiters:          make(map[string]*rate.Limiter),
		debounce:          debounce,
		timeout:           timeout,
		noSummarize:       cfg.NoSummarize,
		noSummarizeSource: noSummarizeSource,
	}

	for k, v := range cfg.InitErrors {
		s.initErrors[k] = v
	}

	if len(cfg.Providers) > 0 {
		s.providers = make([]ProviderConfig, len(cfg.Providers))
		for i, p := range cfg.Providers {
			s.providers[i] = ProviderConfig{
				Name:     p.Name,
				Provider: p.Provider,
				Model:    p.Model,
				Endpoint: SanitizeEndpoint(p.Endpoint),
			}
		}
		return s, nil
	}

	if cfg.NoSummarize {
		return s, nil
	}

	// 1. OpenAI / LiteLLM Proxy
	liteLLMBaseURL := cfg.LiteLLMBaseURL
	liteLLMKey := cfg.LiteLLMAPIKey
	if liteLLMBaseURL != "" || liteLLMKey != "" {
		opts := make([]anyllm.Option, 0, 2)
		if liteLLMBaseURL != "" {
			opts = append(opts, anyllm.WithBaseURL(liteLLMBaseURL))
		}
		if liteLLMKey != "" {
			opts = append(opts, anyllm.WithAPIKey(liteLLMKey))
		}
		p, err := openai.New(opts...)
		if err != nil {
			s.initErrors["openai"] = fmt.Errorf("initializing openai provider: %w", err)
		} else if p != nil {
			model := cfg.LiteLLMModel
			if model == "" {
				model = "gpt-4o-mini"
			}
			s.providers = append(s.providers, ProviderConfig{
				Name:     "openai",
				Provider: p,
				Model:    model,
				Endpoint: SanitizeEndpoint(liteLLMBaseURL),
			})
		}
	}

	// 2. Google Gemini
	geminiKey := cfg.GeminiAPIKey
	if geminiKey != "" {
		p, err := gemini.New(anyllm.WithAPIKey(geminiKey))
		if err != nil {
			s.initErrors["gemini"] = fmt.Errorf("initializing gemini provider: %w", err)
		} else if p != nil {
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
	if anthropicKey != "" {
		p, err := anthropic.New(anyllm.WithAPIKey(anthropicKey))
		if err != nil {
			s.initErrors["anthropic"] = fmt.Errorf("initializing anthropic provider: %w", err)
		} else if p != nil {
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

// SanitizeEndpoint parses rawURL and strips userinfo (credentials), query strings, and fragments,
// returning only scheme, host, and path. If rawURL is empty or fails to parse, it returns an empty string.
func SanitizeEndpoint(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	hasScheme := strings.Contains(rawURL, "://")
	parseTarget := rawURL
	if !hasScheme {
		parseTarget = "http://" + rawURL
	}

	u, err := url.Parse(parseTarget)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""

	if !hasScheme {
		return strings.TrimPrefix(u.String(), "http://")
	}
	return u.String()
}

// ActiveProviders returns metadata for all configured providers.
func (s *Summarizer) ActiveProviders() []ProviderInfo {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]ProviderInfo, len(s.providers))
	for i, p := range s.providers {
		res[i] = ProviderInfo{
			Name:     p.Name,
			Model:    p.Model,
			Endpoint: SanitizeEndpoint(p.Endpoint),
		}
	}
	return res
}

// DisabledByFlag returns true if summarization was explicitly disabled by flag/config.
func (s *Summarizer) DisabledByFlag() bool {
	if s == nil {
		return false
	}
	return s.noSummarize && s.noSummarizeSource != "env"
}

// DisabledByEnv returns true if summarization was disabled by environment variable.
func (s *Summarizer) DisabledByEnv() bool {
	if s == nil {
		return false
	}
	return s.noSummarize && s.noSummarizeSource == "env"
}

// NoSummarizeSource returns the source that disabled summarization ("flag", "env", or "").
func (s *Summarizer) NoSummarizeSource() string {
	if s == nil || !s.noSummarize {
		return ""
	}
	return s.noSummarizeSource
}

// ProviderInitErrors returns a map of provider name to constructor error for any providers that failed initialization.
func (s *Summarizer) ProviderInitErrors() map[string]error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.initErrors) == 0 {
		return nil
	}
	res := make(map[string]error, len(s.initErrors))
	for k, v := range s.initErrors {
		res[k] = v
	}
	return res
}

// HasInitErrors returns true if at least one provider constructor failed during initialization.
func (s *Summarizer) HasInitErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.initErrors) > 0
}

// HasProviders returns true if at least one active LLM provider is configured.
func (s *Summarizer) HasProviders() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.providers) > 0
}

// IsNoSummarize returns whether external LLM calls are disabled or unconfigured.
func (s *Summarizer) IsNoSummarize() bool {
	if s == nil {
		return true
	}
	return s.noSummarize || len(s.providers) == 0
}

// DebounceWindow returns the configured debounce window.
func (s *Summarizer) DebounceWindow() time.Duration {
	if s == nil {
		return 0
	}
	return s.debounce
}
func (s *Summarizer) Allow(agentID string) bool {
	return s.AllowAt(agentID, time.Now())
}

// AllowAt checks the per-agent rate limiter for the debounce window at a specific event time.
func (s *Summarizer) AllowAt(agentID string, eventTime time.Time) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	lim, exists := s.limiters[agentID]
	if !exists {
		lim = rate.NewLimiter(rate.Every(s.debounce), 1)
		s.limiters[agentID] = lim
	}

	return lim.AllowN(eventTime, 1)
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

		callCtx, cancel := context.WithTimeout(ctx, s.timeout)
		resp, err := pCfg.Provider.Completion(callCtx, params)
		cancel()

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

// SanitizeToolInput produces a bounded single-line preview of tool arguments (max 300 chars),
// filtering out large payload bodies like full file contents, replacement blocks, or diffs.
func SanitizeToolInput(input map[string]interface{}, maxLen int) string {
	if len(input) == 0 {
		return ""
	}
	if maxLen <= 0 {
		maxLen = MaxToolArgsLength
	}

	// Filter out large payload fields and sensitive credential parameters
	skipKeys := map[string]bool{
		"content":       true,
		"file_text":     true,
		"replacement":   true,
		"patch":         true,
		"old_str":       true,
		"new_str":       true,
		"old_text":      true,
		"new_text":      true,
		"text":          true,
		"code":          true,
		"body":          true,
		"edits":         true,
		"raw":           true,
		"bytes":         true,
		"items":         true,
		"output":        true,
		"instructions":  true,
		"api_key":       true,
		"apikey":        true,
		"token":         true,
		"password":      true,
		"secret":        true,
		"auth":          true,
		"authorization": true,
		"access_token":  true,
		"refresh_token": true,
		"credential":    true,
		"credentials":   true,
		"private_key":   true,
		"key":           true,
	}

	// Extract keys in sorted order for deterministic preview
	keys := make([]string, 0, len(input))
	for k := range input {
		if !skipKeys[strings.ToLower(k)] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		val := input[k]
		if val == nil {
			continue
		}
		strVal := fmt.Sprintf("%v", val)
		strVal = strings.ReplaceAll(strVal, "\n", " ")
		strVal = strings.TrimSpace(strVal)
		if strVal == "" {
			continue
		}
		if len(strVal) > 100 {
			strVal = strVal[:100] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s: %s", k, strVal))
	}

	if len(parts) == 0 {
		return ""
	}

	preview := strings.Join(parts, ", ")
	if len(preview) > maxLen {
		preview = preview[:maxLen] + "..."
	}
	return preview
}

// Tokenize normalizes text and splits it into lowercase alphanumeric word tokens.
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var cur strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// JaccardSimilarity calculates the token-level Jaccard similarity index between two strings.
// Returns a value between 0.0 (no overlap) and 1.0 (identical token sets).
func JaccardSimilarity(s1, s2 string) float64 {
	t1 := Tokenize(s1)
	t2 := Tokenize(s2)

	if len(t1) == 0 && len(t2) == 0 {
		if strings.EqualFold(strings.TrimSpace(s1), strings.TrimSpace(s2)) {
			return 1.0
		}
		return 0.0
	}
	if len(t1) == 0 || len(t2) == 0 {
		return 0.0
	}

	set1 := make(map[string]struct{}, len(t1))
	for _, t := range t1 {
		set1[t] = struct{}{}
	}

	set2 := make(map[string]struct{}, len(t2))
	for _, t := range t2 {
		set2[t] = struct{}{}
	}

	intersection := 0
	for t := range set1 {
		if _, exists := set2[t]; exists {
			intersection++
		}
	}

	union := len(set1)
	for t := range set2 {
		if _, exists := set1[t]; !exists {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// WordOverlapSimilarity calculates the ratio of shared tokens relative to the smaller token set.
func WordOverlapSimilarity(s1, s2 string) float64 {
	t1 := Tokenize(s1)
	t2 := Tokenize(s2)

	if len(t1) == 0 && len(t2) == 0 {
		if strings.EqualFold(strings.TrimSpace(s1), strings.TrimSpace(s2)) {
			return 1.0
		}
		return 0.0
	}
	if len(t1) == 0 || len(t2) == 0 {
		return 0.0
	}

	set1 := make(map[string]struct{}, len(t1))
	for _, t := range t1 {
		set1[t] = struct{}{}
	}

	set2 := make(map[string]struct{}, len(t2))
	for _, t := range t2 {
		set2[t] = struct{}{}
	}

	intersection := 0
	for t := range set1 {
		if _, exists := set2[t]; exists {
			intersection++
		}
	}

	minSize := len(set1)
	if len(set2) < minSize {
		minSize = len(set2)
	}

	if minSize == 0 {
		return 0.0
	}

	return float64(intersection) / float64(minSize)
}

// IsNearDuplicate checks whether newSummary has >= threshold word overlap (Jaccard or token overlap similarity) with lastSummary.
// Token overlap similarity is only applied when both strings have at least 3 tokens, preventing 1-2 word fallback states
// (like "Working" or "Thinking") from falsely matching detailed summaries.
func IsNearDuplicate(lastSummary, newSummary string, threshold float64) bool {
	if threshold <= 0 {
		threshold = DefaultSimilarityThreshold
	}
	s1 := strings.TrimSpace(lastSummary)
	s2 := strings.TrimSpace(newSummary)

	if s1 == "" || s2 == "" {
		return s1 == s2
	}
	if strings.EqualFold(s1, s2) {
		return true
	}

	if JaccardSimilarity(s1, s2) >= threshold {
		return true
	}

	t1 := Tokenize(s1)
	t2 := Tokenize(s2)
	minTokens := min(len(t1), len(t2))
	if minTokens >= 3 && WordOverlapSimilarity(s1, s2) >= threshold {
		return true
	}

	return false
}

func buildUserPrompt(input SummaryInput) string {
	var sb strings.Builder

	hasThinking := strings.TrimSpace(input.LastThinkingExcerpt) != ""
	hasTool := input.ActiveTool != nil && strings.TrimSpace(*input.ActiveTool) != ""
	hasPrompt := strings.TrimSpace(input.LastPrompt) != ""

	formatPrompt := func() {
		if hasPrompt {
			p := strings.TrimSpace(input.LastPrompt)
			if len(p) > MaxPromptLength {
				p = p[:MaxPromptLength] + "..."
			}
			sb.WriteString("User Prompt: ")
			sb.WriteString(p)
			sb.WriteString("\n")
		}
	}

	formatThinking := func() {
		if hasThinking {
			th := strings.TrimSpace(input.LastThinkingExcerpt)
			if len(th) > MaxThinkingLength {
				th = th[:MaxThinkingLength] + "..."
			}
			sb.WriteString("Thinking: ")
			sb.WriteString(th)
			sb.WriteString("\n")
		}
	}

	formatTool := func() {
		if hasTool {
			tool := strings.TrimSpace(*input.ActiveTool)
			sb.WriteString("Active Tool: ")
			sb.WriteString(tool)
			if input.ActiveTarget != nil && strings.TrimSpace(*input.ActiveTarget) != "" {
				sb.WriteString(" (Target: ")
				sb.WriteString(strings.TrimSpace(*input.ActiveTarget))
				sb.WriteString(")")
			}
			sb.WriteString("\n")

			args := ""
			if input.ActiveToolArgs != nil && strings.TrimSpace(*input.ActiveToolArgs) != "" {
				args = strings.TrimSpace(*input.ActiveToolArgs)
			} else if input.ActiveToolInput != nil {
				args = SanitizeToolInput(input.ActiveToolInput, MaxToolArgsLength)
			}
			if args != "" {
				if len(args) > MaxToolArgsLength {
					args = args[:MaxToolArgsLength] + "..."
				}
				sb.WriteString("Tool Arguments: ")
				sb.WriteString(args)
				sb.WriteString("\n")
			}
		}
	}

	formatRecentTools := func() {
		if len(input.RecentTools) > 0 {
			sb.WriteString("Recent Tool Calls:\n")
			start := 0
			if len(input.RecentTools) > MaxRecentTools {
				start = len(input.RecentTools) - MaxRecentTools
			}
			for _, t := range input.RecentTools[start:] {
				if t.Target != "" {
					fmt.Fprintf(&sb, "- %s on %s\n", t.ToolName, t.Target)
				} else {
					fmt.Fprintf(&sb, "- %s\n", t.ToolName)
				}
			}
		}
	}

	// Specialized prompt template selection based on active context
	switch {
	case hasThinking && hasTool:
		// Strategy 1: Combined thinking + tool execution
		formatPrompt()
		formatThinking()
		formatTool()

	case hasThinking && !hasTool:
		// Strategy 2: Thinking only
		formatPrompt()
		formatThinking()

	case !hasThinking && hasTool:
		// Strategy 3: Tool execution only
		formatPrompt()
		formatTool()
		formatRecentTools()

	default:
		// Strategy 4: Conversational / Error / Idle status context
		formatPrompt()
		if input.LastAssistantResponse != "" {
			r := strings.TrimSpace(input.LastAssistantResponse)
			if len(r) > MaxPromptLength {
				r = r[:MaxPromptLength] + "..."
			}
			sb.WriteString("Assistant Response: ")
			sb.WriteString(r)
			sb.WriteString("\n")
		}
		sb.WriteString("Current Activity State: ")
		sb.WriteString(string(input.Activity))
		sb.WriteString("\n")
		formatRecentTools()
		if input.Question != nil && strings.TrimSpace(*input.Question) != "" {
			sb.WriteString("Question Asked: ")
			sb.WriteString(strings.TrimSpace(*input.Question))
			sb.WriteString("\n")
		}
		if input.LastError != nil && strings.TrimSpace(*input.LastError) != "" {
			lErr := strings.TrimSpace(*input.LastError)
			if len(lErr) > MaxErrorLength {
				lErr = lErr[:MaxErrorLength] + "..."
			}
			sb.WriteString("Last Error: ")
			sb.WriteString(lErr)
			sb.WriteString("\n")
		}
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

func resolveToolSummary(tool, target string) (string, *types.AgentPhase) {
	normalized := strings.ToLower(strings.TrimSpace(tool))
	var phase *types.AgentPhase
	var action string

	switch normalized {
	case "edit", "notebookedit", "notebook_edit", "multiedit", "multi_edit":
		p := types.PhaseEditing
		phase = &p
		if target != "" {
			action = fmt.Sprintf("Editing %s", target)
		} else {
			action = "Editing code"
		}

	case "write", "writefile", "write_file":
		p := types.PhaseEditing
		phase = &p
		if target != "" {
			action = fmt.Sprintf("Writing %s", target)
		} else {
			action = "Writing file"
		}

	case "read", "readfile", "read_file":
		p := types.PhaseInvestigating
		phase = &p
		if target != "" {
			action = fmt.Sprintf("Reading %s", target)
		} else {
			action = "Reading file"
		}

	case "bash", "sh", "zsh", "exec", "exec_command":
		lowerTarget := strings.ToLower(target)
		if strings.Contains(lowerTarget, "test") || strings.Contains(lowerTarget, "jest") || strings.Contains(lowerTarget, "pytest") {
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

	case "askuserquestion", "ask_user_question":
		p := types.PhasePlanning
		phase = &p
		action = "Waiting for user input"

	case "websearch", "web_search", "webfetch", "web_fetch", "ast-grep", "codegraph", "grep", "find", "glob":
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

	return action, phase
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
		action, phase = resolveToolSummary(tool, target)
	} else {
		switch input.Activity {
		case types.ActivityWaitingQuestion:
			p := types.PhasePlanning
			phase = &p
			if input.Question != nil && strings.TrimSpace(*input.Question) != "" {
				action = strings.TrimSpace(*input.Question)
			} else {
				action = "Waiting for user input"
			}

		case types.ActivityError:
			p := types.PhaseDebugging
			phase = &p
			if input.LastError != nil && strings.TrimSpace(*input.LastError) != "" {
				action = strings.TrimSpace(*input.LastError)
			} else {
				action = "Encountered error"
			}

		case types.ActivityRateLimited:
			p := types.PhasePlanning
			phase = &p
			action = "Rate limit reached — backoff/retry in progress"

		case types.ActivityIdle:
			if input.LastAssistantResponse != "" && len(input.RecentTools) == 0 {
				action = "Responded to user"
			} else {
				action = "Idle"
			}

		default:
			if input.LastAssistantResponse != "" && len(input.RecentTools) == 0 {
				action = "Responded to user"
			} else if input.CurrentAction != "" && input.CurrentAction != "Thinking" {
				action = input.CurrentAction
			} else if len(input.RecentTools) > 0 {
				last := input.RecentTools[len(input.RecentTools)-1]
				action, phase = resolveToolSummary(last.ToolName, last.Target)
			} else if input.LastThinkingExcerpt != "" || input.CurrentAction == "Thinking" {
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
