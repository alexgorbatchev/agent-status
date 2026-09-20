package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
	"github.com/shirou/gopsutil/v4/process"
)

type CodexSessionDescriptor struct {
	PID            int    `json:"pid"`
	SessionID      string `json:"sessionId"`
	CWD            string `json:"cwd"`
	TranscriptPath string `json:"transcriptPath"`
	StartedAt      int64  `json:"startedAt"`
	HarnessVersion string `json:"harnessVersion"`
	IsActive       bool   `json:"isActive"`
	ParentThreadID string `json:"parentThreadId,omitempty"`
	AgentNickname  string `json:"agentNickname,omitempty"`
	AgentRole      string `json:"agentRole,omitempty"`
	IsSubagent     bool   `json:"isSubagent"`
}

type CodexProcessInfo struct {
	PID        int
	CWD        string
	CreateTime int64 // unix millis
}

type rawCodexHeaderPayload struct {
	ID             string      `json:"id"`
	Timestamp      string      `json:"timestamp"`
	CWD            string      `json:"cwd"`
	CLIVersion     string      `json:"cli_version"`
	ParentThreadID string      `json:"parent_thread_id"`
	AgentNickname  string      `json:"agent_nickname"`
	AgentRole      string      `json:"agent_role"`
	Source         interface{} `json:"source"`
}

type rawCodexHeader struct {
	Type      string                `json:"type"`
	Timestamp interface{}           `json:"timestamp"`
	Payload   rawCodexHeaderPayload `json:"payload"`
}

type CodexSessionHeader struct {
	ID             string
	Timestamp      int64
	CWD            string
	CLIVersion     string
	ParentThreadID string
	AgentNickname  string
	AgentRole      string
	IsSubagent     bool
}

type CodexProcessFinder func(ctx context.Context) ([]CodexProcessInfo, error)

type CodexScanner struct {
	sessionsDir   string
	processFinder CodexProcessFinder
}

func DefaultCodexSessionsDir() string {
	if custom := os.Getenv("CODEX_SESSIONS_DIR"); custom != "" {
		return custom
	}
	if homeDir := os.Getenv("CODEX_HOME"); homeDir != "" {
		return filepath.Join(homeDir, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

func LiveCodexProcessFinder(ctx context.Context) ([]CodexProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing processes: %w", err)
	}

	var results []CodexProcessInfo
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}

		baseName := strings.ToLower(filepath.Base(name))
		isCodex := baseName == "codex" || baseName == "codex.exe" || baseName == "codex-tui" || baseName == "codex-cli"
		if !isCodex {
			cmdline, cErr := p.CmdlineSliceWithContext(ctx)
			if cErr != nil || len(cmdline) == 0 {
				continue
			}
			for _, arg := range cmdline {
				baseArg := strings.ToLower(filepath.Base(arg))
				if baseArg == "codex" || baseArg == "codex-cli" || strings.HasSuffix(arg, "/codex") {
					isCodex = true
					break
				}
			}
			if !isCodex {
				continue
			}
		}

		cwd, err := p.CwdWithContext(ctx)
		if err != nil || cwd == "" {
			continue
		}

		createTime, err := p.CreateTimeWithContext(ctx)
		if err != nil {
			createTime = 0
		}

		results = append(results, CodexProcessInfo{
			PID:        int(p.Pid),
			CWD:        filepath.Clean(cwd),
			CreateTime: createTime,
		})
	}

	return results, nil
}

func NewCodexScanner(sessionsDir string, finder CodexProcessFinder) *CodexScanner {
	if sessionsDir == "" {
		sessionsDir = DefaultCodexSessionsDir()
	}
	if finder == nil {
		finder = LiveCodexProcessFinder
	}
	return &CodexScanner{
		sessionsDir:   sessionsDir,
		processFinder: finder,
	}
}

func parseRFC3339Time(s string) int64 {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	return 0
}

func ReadCodexSessionHeader(path string) (*CodexSessionHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening codex session file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scanning session file: %w", err)
		}
		return nil, fmt.Errorf("empty session file")
	}

	var raw rawCodexHeader
	if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling session header: %w", err)
	}

	if raw.Type != "session_meta" {
		return nil, fmt.Errorf("invalid header type: %q, expected 'session_meta'", raw.Type)
	}

	headerTime := int64(0)
	if raw.Payload.Timestamp != "" {
		headerTime = parseRFC3339Time(raw.Payload.Timestamp)
	}
	if headerTime == 0 && raw.Timestamp != nil {
		switch ts := raw.Timestamp.(type) {
		case string:
			headerTime = parseRFC3339Time(ts)
		case float64:
			headerTime = int64(ts)
		case int64:
			headerTime = ts
		}
	}

	isSubagent := raw.Payload.ParentThreadID != "" || raw.Payload.AgentNickname != "" || raw.Payload.AgentRole != ""
	if !isSubagent && raw.Payload.Source != nil {
		if sm, ok := raw.Payload.Source.(map[string]interface{}); ok {
			if subVal, exists := sm["subagent"]; exists && subVal != nil {
				isSubagent = true
			}
		} else if subStr, ok := raw.Payload.Source.(string); ok && subStr != "cli" && subStr != "" {
			isSubagent = true
		}
	}

	sessionID := raw.Payload.ID
	if sessionID == "" {
		if info, pErr := parser.ParseCodexRolloutPath(path); pErr == nil && info.SessionID != "" {
			sessionID = info.SessionID
		}
	}

	return &CodexSessionHeader{
		ID:             sessionID,
		Timestamp:      headerTime,
		CWD:            raw.Payload.CWD,
		CLIVersion:     raw.Payload.CLIVersion,
		ParentThreadID: raw.Payload.ParentThreadID,
		AgentNickname:  raw.Payload.AgentNickname,
		AgentRole:      raw.Payload.AgentRole,
		IsSubagent:     isSubagent,
	}, nil
}

func (s *CodexScanner) Scan(ctx context.Context) ([]CodexSessionDescriptor, error) {
	if s.sessionsDir == "" {
		return nil, nil
	}

	resolvedDir, err := filepath.EvalSymlinks(s.sessionsDir)
	if err != nil {
		resolvedDir = s.sessionsDir
	}

	if _, err := os.Stat(resolvedDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("accessing codex sessions dir: %w", err)
	}

	var activeProcs []CodexProcessInfo
	if s.processFinder != nil {
		activeProcs, _ = s.processFinder(ctx)
	}

	procsByCWD := make(map[string][]CodexProcessInfo)
	for _, proc := range activeProcs {
		cleanCWD := filepath.Clean(proc.CWD)
		procsByCWD[cleanCWD] = append(procsByCWD[cleanCWD], proc)
	}

	var descriptors []CodexSessionDescriptor
	usedPIDs := make(map[int]bool)

	err = filepath.WalkDir(resolvedDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		header, err := ReadCodexSessionHeader(path)
		if err != nil {
			return nil
		}

		headerTime := header.Timestamp
		if headerTime == 0 {
			if info, sErr := d.Info(); sErr == nil {
				headerTime = info.ModTime().UnixMilli()
			}
		}

		cleanCWD := filepath.Clean(header.CWD)
		desc := CodexSessionDescriptor{
			PID:            0,
			SessionID:      header.ID,
			CWD:            cleanCWD,
			TranscriptPath: path,
			StartedAt:      headerTime,
			HarnessVersion: header.CLIVersion,
			IsActive:       false,
			ParentThreadID: header.ParentThreadID,
			AgentNickname:  header.AgentNickname,
			AgentRole:      header.AgentRole,
			IsSubagent:     header.IsSubagent,
		}

		// Pair with active process if available
		candidateProcs := procsByCWD[cleanCWD]
		var bestProc *CodexProcessInfo
		var minDiff int64 = 1<<62 - 1

		for _, proc := range candidateProcs {
			if usedPIDs[proc.PID] {
				continue
			}

			diff := headerTime - proc.CreateTime
			if diff >= -5000 {
				absDiff := diff
				if absDiff < 0 {
					absDiff = -absDiff
				}
				if absDiff < minDiff {
					minDiff = absDiff
					procCopy := proc
					bestProc = &procCopy
				}
			}
		}

		if bestProc != nil {
			desc.PID = bestProc.PID
			desc.IsActive = true
			if bestProc.CreateTime > 0 {
				desc.StartedAt = bestProc.CreateTime
			}
			usedPIDs[bestProc.PID] = true
		} else if os.Getenv("AGENT_STATUS_TEST_MOCK_PROC") == "1" {
			desc.PID = 99993
			desc.IsActive = true
		}

		descriptors = append(descriptors, desc)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking codex sessions dir: %w", err)
	}

	return descriptors, nil
}
