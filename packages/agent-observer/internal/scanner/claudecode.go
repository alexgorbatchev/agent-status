package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SessionDescriptor struct {
	PID            int    `json:"pid"`
	SessionID      string `json:"sessionId"`
	CWD            string `json:"cwd"`
	Status         string `json:"status"`
	TranscriptPath string `json:"transcriptPath"`
	StartedAt      int64  `json:"startedAt"`
	HarnessVersion string `json:"version"`
}

type SubagentTranscript struct {
	SubagentID     string `json:"subagentId"`
	TranscriptPath string `json:"transcriptPath"`
}

type ClaudeCodeScanner struct {
	sessionDirs []string
}

func DefaultClaudeSessionDirs() []string {
	var dirs []string
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		dirs = append(dirs, filepath.Join(dataHome, "ai-registry", "claude-code", "sessions"))
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "ai-registry", "claude-code", "sessions"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".claude", "sessions"))
	}

	return dirs
}

func NewClaudeCodeScanner(dirs []string) *ClaudeCodeScanner {
	if len(dirs) == 0 {
		dirs = DefaultClaudeSessionDirs()
	}
	return &ClaudeCodeScanner{sessionDirs: dirs}
}

func (s *ClaudeCodeScanner) Scan() ([]SessionDescriptor, error) {
	var results []SessionDescriptor
	seenPIDs := make(map[int]bool)

	for _, rawDir := range s.sessionDirs {
		dir, err := filepath.EvalSymlinks(rawDir)
		if err != nil {
			dir = rawDir
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Skip missing directories
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".key") || !strings.HasSuffix(name, ".json") {
				continue
			}

			filePath := filepath.Join(dir, name)
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var desc SessionDescriptor
			if err := json.Unmarshal(data, &desc); err != nil {
				continue
			}

			if desc.PID <= 0 || seenPIDs[desc.PID] {
				continue
			}

			if !IsProcessAlive(desc.PID) {
				continue
			}

			if desc.StartedAt > 0 && !VerifyProcessMatch(desc.PID, desc.StartedAt) {
				continue
			}

			if desc.TranscriptPath == "" && desc.SessionID != "" {
				desc.TranscriptPath = locateTranscript(desc.SessionID, desc.CWD)
			}

			seenPIDs[desc.PID] = true
			results = append(results, desc)
		}
	}

	return results, nil
}

func (s *ClaudeCodeScanner) DiscoverSubagents(desc SessionDescriptor) ([]SubagentTranscript, error) {
	if desc.TranscriptPath == "" {
		return nil, nil
	}

	dir := filepath.Dir(desc.TranscriptPath)
	candidates := []string{
		filepath.Join(dir, desc.SessionID, "subagents"),
		filepath.Join(dir, "subagents"),
	}

	var results []SubagentTranscript
	for _, subagentsDir := range candidates {
		entries, err := os.ReadDir(subagentsDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}

			subID := strings.TrimSuffix(entry.Name(), ".jsonl")
			results = append(results, SubagentTranscript{
				SubagentID:     subID,
				TranscriptPath: filepath.Join(subagentsDir, entry.Name()),
			})
		}
	}

	return results, nil
}

func locateTranscript(sessionID, cwd string) string {
	var projectsDirs []string

	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		projectsDirs = append(projectsDirs, filepath.Join(dataHome, "ai-registry", "claude-code", "projects"))
	} else if home, err := os.UserHomeDir(); err == nil {
		projectsDirs = append(projectsDirs, filepath.Join(home, ".local", "share", "ai-registry", "claude-code", "projects"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		projectsDirs = append(projectsDirs, filepath.Join(home, ".claude", "projects"))
	}

	targetFileName := fmt.Sprintf("%s.jsonl", sessionID)

	// 1. If cwd is provided, check the exact slug folder first
	if cwd != "" {
		slug := strings.ReplaceAll(cwd, "/", "-")
		slug = strings.ReplaceAll(slug, ".", "-")
		if !strings.HasPrefix(slug, "-") {
			slug = "-" + slug
		}

		for _, pDir := range projectsDirs {
			resolvedDir, err := filepath.EvalSymlinks(pDir)
			if err != nil {
				resolvedDir = pDir
			}
			exactPath := filepath.Join(resolvedDir, slug, targetFileName)
			if _, err := os.Stat(exactPath); err == nil {
				return exactPath
			}
		}
	}

	// 2. Scan project folders across all candidate directories
	for _, pDir := range projectsDirs {
		resolvedDir, err := filepath.EvalSymlinks(pDir)
		if err != nil {
			resolvedDir = pDir
		}

		entries, err := os.ReadDir(resolvedDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				candidate := filepath.Join(resolvedDir, e.Name(), targetFileName)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}

	return ""
}
