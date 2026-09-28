package types

import (
	"encoding/json"

	parser "github.com/alexgorbatchev/agent-parser"
)

type LifecycleState string

const (
	LifecycleRunning LifecycleState = "RUNNING"
	LifecycleExited  LifecycleState = "EXITED"
)

type ActivityState string

const (
	ActivityBusy            ActivityState = "BUSY"
	ActivityWaitingQuestion ActivityState = "WAITING_QUESTION"
	ActivityError           ActivityState = "ERROR"
	ActivityRateLimited     ActivityState = "RATE_LIMITED"
	ActivityIdle            ActivityState = "IDLE"
)

type HarnessType string

const (
	HarnessClaudeCode HarnessType = "claude-code"
	HarnessPi         HarnessType = "pi"
	HarnessCodex      HarnessType = "codex"
	HarnessOpencode   HarnessType = "opencode"
)

type AgentPhase string

const (
	PhasePlanning      AgentPhase = "planning"
	PhaseEditing       AgentPhase = "editing"
	PhaseTesting       AgentPhase = "testing"
	PhaseDebugging     AgentPhase = "debugging"
	PhaseInvestigating AgentPhase = "investigating"
)

type IEnvironmentInfo struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Hostname        string `json:"hostname"`
	Platform        string `json:"platform"`
	User            string `json:"user"`
	IsStale         bool   `json:"isStale"`
	LastHeartbeatAt int64  `json:"lastHeartbeatAt"`
}

type IWorkspaceInfo struct {
	IsWorktree bool    `json:"isWorktree"`
	Branch     string  `json:"branch"`
	IsDetached bool    `json:"isDetached"`
	RelPath    *string `json:"relPath"`
	CWD        string  `json:"cwd"`
}

type IProjectInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CanonicalRoot string `json:"canonicalRoot"`
}

type TokenUsage = parser.TokenUsage
type ITokenUsage = parser.TokenUsage

type ICostInfo struct {
	Currency    string  `json:"currency"`
	Total       float64 `json:"total"`
	IsEstimated bool    `json:"isEstimated"`
}

type IRunInfo struct {
	ID             string `json:"id"`
	PID            int    `json:"pid"`
	HarnessVersion string `json:"harnessVersion"`
	StartedAt      int64  `json:"startedAt"`
	EndedAt        *int64 `json:"endedAt"`
}

type SubagentStatus string

const (
	SubagentRunning   SubagentStatus = "running"
	SubagentCompleted SubagentStatus = "completed"
	SubagentSettling  SubagentStatus = "settling"
)

type ISubagentSummary struct {
	ID          string         `json:"id"`
	Activity    ActivityState  `json:"activity"`
	ActiveTool  *string        `json:"activeTool,omitempty"`
	Action      string         `json:"action,omitempty"`
	LastError   *string        `json:"lastError,omitempty"`
	Status      SubagentStatus `json:"status,omitempty"`
	StartedAt   int64          `json:"startedAt,omitempty"`
	CompletedAt *int64         `json:"completedAt,omitempty"`
}

type IAgentActivity struct {
	Lifecycle           LifecycleState     `json:"lifecycle"`
	Activity            ActivityState      `json:"activity"`
	Title               string             `json:"title"`
	Action              string             `json:"action"`
	ActionStale         bool               `json:"actionStale"`
	Phase               *AgentPhase        `json:"phase"`
	ActiveTool          *string            `json:"activeTool"`
	ActiveTarget        *string            `json:"activeTarget"`
	ActiveToolStartedAt *int64             `json:"activeToolStartedAt,omitempty"`
	Question            *string            `json:"question"`
	LastThinkingExcerpt *string            `json:"lastThinkingExcerpt"`
	LastError           *string            `json:"lastError"`
	RateLimitResetAt    *int64             `json:"rateLimitResetAt,omitempty"`
	RetryAfterSeconds   *int64             `json:"retryAfterSeconds,omitempty"`
	Model               *string            `json:"model,omitempty"`
	ContextWindow       *int64             `json:"contextWindow,omitempty"`
	ContextFullness     *float64           `json:"contextFullness,omitempty"`
	Usage               ITokenUsage        `json:"usage"`
	Cost                *ICostInfo         `json:"cost"`
	SubagentCount       int                `json:"subagentCount"`
	ActiveSubagents     []ISubagentSummary `json:"activeSubagents,omitempty"`
	CurrentRun          *IRunInfo          `json:"currentRun"`
	FirstStartedAt      int64              `json:"firstStartedAt"`
	UpdatedAt           int64              `json:"updatedAt"`
	HasUnseenCompletion *bool              `json:"hasUnseenCompletion,omitempty"`
	CompletedAt         *int64             `json:"completedAt,omitempty"`
}

func (a *IAgentActivity) UnmarshalJSON(data []byte) error {
	type Alias IAgentActivity
	aux := struct {
		*Alias
		SnakeHasUnseenCompletion *bool  `json:"has_unseen_completion"`
		SnakeCompletedAt         *int64 `json:"completed_at"`
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if a.HasUnseenCompletion == nil && aux.SnakeHasUnseenCompletion != nil {
		a.HasUnseenCompletion = aux.SnakeHasUnseenCompletion
	}
	if a.CompletedAt == nil && aux.SnakeCompletedAt != nil {
		a.CompletedAt = aux.SnakeCompletedAt
	}
	return nil
}

type IAgentState struct {
	ID          string           `json:"id"`
	SessionID   string           `json:"sessionId"`
	Harness     HarnessType      `json:"harness"`
	Environment IEnvironmentInfo `json:"environment"`
	Project     IProjectInfo     `json:"project"`
	Workspace   IWorkspaceInfo   `json:"workspace"`
	Activity    IAgentActivity   `json:"activity"`
	IsHidden    bool             `json:"isHidden"`
	HiddenAt    *int64           `json:"hiddenAt,omitempty"`
}

type StatusResponse struct {
	Status                 string             `json:"status"`
	Environments           []IEnvironmentInfo `json:"environments"`
	ActiveAgentCount       int                `json:"activeAgentCount"`
	RequiresAttentionCount int                `json:"requiresAttentionCount"`
	TotalFleetSpend        float64            `json:"totalFleetSpend"`
}

type ProjectGroup struct {
	Project          IProjectInfo  `json:"project"`
	Agents           []IAgentState `json:"agents"`
	TotalCost        float64       `json:"totalCost"`
	ActiveAgentCount int           `json:"activeAgentCount"`
}

type DiffLine = parser.DiffLine
type DiffHunk = parser.DiffHunk
type FileModification = parser.FileModification
type TelemetryFrameData = parser.EventData

type ITelemetryFrame struct {
	Version        string             `json:"version"`
	Seq            int64              `json:"seq"`
	EnvID          string             `json:"envId"`
	Environment    IEnvironmentInfo   `json:"environment"`
	AgentID        string             `json:"agentId"`
	SessionID      string             `json:"sessionId"`
	SubagentID     *string            `json:"subagentId"`
	PID            int                `json:"pid"`
	Harness        HarnessType        `json:"harness"`
	HarnessVersion string             `json:"harnessVersion"`
	Project        IProjectInfo       `json:"project"`
	Workspace      IWorkspaceInfo     `json:"workspace"`
	Timestamp      int64              `json:"timestamp"`
	EventType      string             `json:"eventType"`
	Data           TelemetryFrameData `json:"data"`
}

type IObserverMessage struct {
	Type        string            `json:"type"` // "hello" | "frames"
	EnvID       string            `json:"envId,omitempty"`
	Environment *IEnvironmentInfo `json:"environment,omitempty"`
	Since       int64             `json:"since,omitempty"`
	Dropped     int64             `json:"dropped,omitempty"`
	Frames      []ITelemetryFrame `json:"frames,omitempty"`
}

type IIngesterMessage struct {
	Type    string `json:"type"` // "ack" | "error"
	UpTo    int64  `json:"upTo,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type IServerEvent struct {
	Type         string             `json:"type"` // "snapshot" | "agent_upsert" | "agent_removed" | "environment_upsert" | "log_append" | "hello" (internal transport handshake event)
	At           int64              `json:"at"`
	Agent        *IAgentState       `json:"agent,omitempty"`
	Agents       []IAgentState      `json:"agents,omitempty"`
	AgentID      string             `json:"agentId,omitempty"`
	Environment  *IEnvironmentInfo  `json:"environment,omitempty"`
	Environments []IEnvironmentInfo `json:"environments,omitempty"`
	Event        *ITelemetryFrame   `json:"event,omitempty"`
}

func (e IServerEvent) MarshalJSON() ([]byte, error) {
	if e.Type == "snapshot" {
		agents := e.Agents
		if agents == nil {
			agents = []IAgentState{}
		}
		envs := e.Environments
		if envs == nil {
			envs = []IEnvironmentInfo{}
		}
		type Alias IServerEvent
		return json.Marshal(&struct {
			Alias
			Agents       []IAgentState      `json:"agents"`
			Environments []IEnvironmentInfo `json:"environments"`
		}{
			Alias:        Alias(e),
			Agents:       agents,
			Environments: envs,
		})
	}
	type Alias IServerEvent
	return json.Marshal(Alias(e))
}

func (e *IServerEvent) UnmarshalJSON(data []byte) error {
	type Alias IServerEvent
	var aux Alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*e = IServerEvent(aux)
	if e.Type == "snapshot" {
		if e.Agents == nil {
			e.Agents = []IAgentState{}
		}
		if e.Environments == nil {
			e.Environments = []IEnvironmentInfo{}
		}
	}
	return nil
}
