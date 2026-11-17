package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

var (
	identifier    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]*$`)
	slashCommand  = regexp.MustCompile(`^(/[A-Za-z][A-Za-z0-9_.:-]*)(?:\s|$)`)
	claudeCommand = regexp.MustCompile(`<command-name>\s*(/?[A-Za-z][A-Za-z0-9_.:-]*)\s*</command-name>`)
	piSkill       = regexp.MustCompile(`^<skill name="([^"]+)" location="([^"]+)">\n`)
	codexSkill    = regexp.MustCompile(`^<skill>\s*<name>([^<]+)</name>\s*<path>([^<]+)</path>`)
)

// Extract uses explicit metadata only. It never inspects shell commands, skill
// catalogs, conversational mentions, tool results, or harness configuration files.
func Extract(f types.ITelemetryFrame) []Observation {
	if f.Timestamp <= 0 {
		return nil
	}
	var observations []Observation
	add := func(kind, name, server, operation, source string) {
		if name == "" || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return
		}
		e := Evidence{Source: source, EventType: f.EventType, Timestamp: f.Timestamp}
		if f.SubagentID != nil {
			e.SubagentID = *f.SubagentID
		}
		if f.Data.ToolCallID != nil && (f.EventType == "tool_call" || f.EventType == "tool_use") {
			e.ToolCallID = *f.Data.ToolCallID
		}
		identity := []any{f.AgentID, e.SubagentID, kind, name, server, operation}
		if e.ToolCallID != "" {
			identity = append(identity, "call", e.ToolCallID)
		} else {
			identity = append(identity, "timestamp", f.Timestamp, source)
		}
		encoded, _ := json.Marshal(identity)
		hash := sha256.Sum256(encoded)
		observations = append(observations, Observation{ID: hex.EncodeToString(hash[:]), Kind: kind, Name: name, Server: server, Operation: operation, Evidence: e})
	}
	if f.EventType == "tool_call" || f.EventType == "tool_use" {
		tool := ""
		if f.Data.ToolName != nil {
			tool = *f.Data.ToolName
		}
		input := func(key string) string { value, _ := f.Data.ToolInput[key].(string); return value }
		if (f.Harness == types.HarnessClaudeCode && tool == "Skill") || (f.Harness == types.HarnessOpencode && tool == "skill") {
			key := "skill"
			if f.Harness == types.HarnessOpencode {
				key = "name"
			}
			if name := input(key); identifier.MatchString(name) {
				add("skill", name, "", "invoke", "skill_tool")
			}
		}
		key := ""
		switch {
		case f.Harness == types.HarnessClaudeCode && tool == "Read":
			key = "file_path"
		case f.Harness == types.HarnessPi && tool == "read":
			key = "path"
		case f.Harness == types.HarnessOpencode && tool == "read":
			key = "filePath"
		case f.Harness == types.HarnessCodex && tool == "read_file":
			key = "path"
		}
		if key != "" {
			if file := input(key); path.Base(file) == "SKILL.md" {
				add("skill", file, "", "load", "skill_file_read")
			}
		}
		if suffix, ok := strings.CutPrefix(tool, "mcp__"); ok {
			server, name, ok := strings.Cut(suffix, "__")
			if ok && identifier.MatchString(server) && identifier.MatchString(name) {
				add("mcp", name, server, "invoke", "mcp_tool")
			}
		}
		return observations
	}
	if (f.EventType != "turn_start" && f.EventType != "user_prompt") || f.Data.Content == nil {
		return nil
	}
	content := strings.TrimSpace(*f.Data.Content)
	if f.Harness == types.HarnessClaudeCode && (strings.HasPrefix(content, "<command-") || strings.HasPrefix(content, "<local-command-caveat>")) {
		if match := claudeCommand.FindStringSubmatch(content); match != nil {
			add("command", "/"+strings.TrimPrefix(match[1], "/"), "", "invoke", "command_metadata")
			return observations
		}
	}
	if f.Harness == types.HarnessPi {
		if match := piSkill.FindStringSubmatch(content); match != nil && identifier.MatchString(match[1]) && strings.Contains(content, "\n</skill>") {
			add("skill", match[1], "", "load", "skill_expansion")
			add("command", "/skill:"+match[1], "", "invoke", "skill_expansion")
			return observations
		}
	}
	if f.Harness == types.HarnessCodex {
		if match := codexSkill.FindStringSubmatch(content); match != nil && identifier.MatchString(match[1]) && strings.Contains(content, "</skill>") {
			add("skill", match[1], "", "load", "skill_injection")
			return observations
		}
	}
	if match := slashCommand.FindStringSubmatch(content); match != nil {
		add("command", match[1], "", "request", "slash_prompt")
	}
	return observations
}
