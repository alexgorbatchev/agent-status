// Package usage models observed skill, harness command, and MCP activity.
package usage

import "github.com/alexgorbatchev/agent-status/packages/agent-common/types"

type Evidence struct {
	Source     string `json:"source"`
	EventType  string `json:"eventType"`
	Timestamp  int64  `json:"timestamp"`
	SubagentID string `json:"subagentId,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
}

type Observation struct {
	ID        string
	Kind      string
	Name      string
	Server    string
	Operation string
	Evidence  Evidence
}

type Entry struct {
	Name           string   `json:"name"`
	Server         string   `json:"server,omitempty"`
	Operation      string   `json:"operation"`
	Count          int64    `json:"count"`
	FirstUsedAt    int64    `json:"firstUsedAt"`
	LastUsedAt     int64    `json:"lastUsedAt"`
	LatestEvidence Evidence `json:"latestEvidence"`
}

type Coverage struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Report struct {
	AgentID  string              `json:"agentId"`
	Harness  types.HarnessType   `json:"harness"`
	Skills   []Entry             `json:"skills"`
	Commands []Entry             `json:"commands"`
	MCP      []Entry             `json:"mcp"`
	Coverage map[string]Coverage `json:"coverage"`
	History  string              `json:"history"`
}

func NewReport(agentID string, harness types.HarnessType) *Report {
	details := map[string]string{}
	switch harness {
	case types.HarnessClaudeCode:
		details["skills"] = "Skill tool invocation attempts and direct SKILL.md read attempts; direct slash expansion may not identify the skill separately."
		details["commands"] = "Native command-name markers and recorded slash requests; commands omitted from transcripts are unknown."
	case types.HarnessPi:
		details["skills"] = "Expanded skill blocks and direct SKILL.md read attempts."
		details["commands"] = "Expanded /skill:name commands and recorded slash requests; extension commands and prompt-template names may not be persisted."
	case types.HarnessCodex:
		details["skills"] = "Injected skill blocks and explicit read_file calls; skill reads through shell tools are excluded."
		details["commands"] = "Recorded slash requests only; client-side commands may not be persisted."
	case types.HarnessOpencode:
		details["skills"] = "Skill tool invocation attempts and direct SKILL.md read attempts."
		details["commands"] = "Recorded slash requests only; expanded commands may omit their original names."
	}
	coverage := make(map[string]Coverage)
	for _, kind := range []string{"skills", "commands", "mcp"} {
		detail, known := details[kind]
		if kind == "mcp" && len(details) > 0 {
			detail = "Explicit mcp__server__tool calls only; unqualified names and calls hidden inside other tools are unknown."
			known = true
		}
		status := "partial"
		if !known {
			status = "unavailable"
			detail = "No extractor is defined for this harness."
		}
		coverage[kind] = Coverage{Status: status, Detail: detail}
	}
	return &Report{AgentID: agentID, Harness: harness, Skills: []Entry{}, Commands: []Entry{}, MCP: []Entry{}, Coverage: coverage,
		History: "Includes retained telemetry backfilled at migration and subsequent observations. Older pruned history is unknown. Missing native call IDs use timestamp-based deduplication and may undercount identical observations at the same timestamp."}
}
