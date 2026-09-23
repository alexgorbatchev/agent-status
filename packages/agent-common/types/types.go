package types

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

type ITokenUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Reasoning  int64 `json:"reasoning"`
	Total      int64 `json:"total"`
}

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

type ISubagentSummary struct {
	ID         string        `json:"id"`
	Activity   ActivityState `json:"activity"`
	ActiveTool *string       `json:"activeTool,omitempty"`
	Action     string        `json:"action,omitempty"`
	LastError  *string       `json:"lastError,omitempty"`
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

type DiffLine struct {
	Type          string `json:"type"` // "add" | "del" | "ctx"
	OldLineNumber *int   `json:"oldLineNumber,omitempty"`
	NewLineNumber *int   `json:"newLineNumber,omitempty"`
	Content       string `json:"content"`
}

type DiffHunk struct {
	Header string     `json:"header,omitempty"`
	Lines  []DiffLine `json:"lines"`
}

type FileModification struct {
	Action   string     `json:"action"` // "EDIT" | "WRITE"
	FilePath string     `json:"filePath"`
	Hunks    []DiffHunk `json:"hunks,omitempty"`
	Content  *string    `json:"content,omitempty"`
}

type TelemetryFrameData struct {
	Role             *string                `json:"role,omitempty"`
	Content          *string                `json:"content,omitempty"`
	Title            *string                `json:"title,omitempty"`
	LastPrompt       *string                `json:"lastPrompt,omitempty"`
	Model            *string                `json:"model,omitempty"`
	ModelContextWindow *int64               `json:"modelContextWindow,omitempty"`
	ContextFullness    *float64             `json:"contextFullness,omitempty"`
	ToolName         *string                `json:"toolName,omitempty"`
	ToolCallID       *string                `json:"toolCallId,omitempty"`
	ToolInput        map[string]interface{} `json:"toolInput,omitempty"`
	FileModification *FileModification      `json:"fileModification,omitempty"`
	ToolOutput       *string                `json:"toolOutput,omitempty"`
	IsError          *bool                  `json:"isError,omitempty"`
	ExitCode         *int                   `json:"exitCode,omitempty"`
	Question         *string                `json:"question,omitempty"`
	StopReason       *string                `json:"stopReason,omitempty"`
	Usage            *ITokenUsage           `json:"usage,omitempty"`
	RawHarnessCost   *float64               `json:"rawHarnessCost,omitempty"`
}

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
	Type         string              `json:"type"` // "snapshot" | "agent_upsert" | "agent_removed" | "environment_upsert" | "log_append"
	At           int64               `json:"at"`
	Agent        *IAgentState        `json:"agent,omitempty"`
	Agents       []IAgentState       `json:"agents,omitempty"`
	AgentID      string              `json:"agentId,omitempty"`
	Environment  *IEnvironmentInfo   `json:"environment,omitempty"`
	Environments []IEnvironmentInfo  `json:"environments,omitempty"`
	Event        *ITelemetryFrame    `json:"event,omitempty"`
}
