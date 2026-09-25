package session

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FindClaudeSession locates a Claude Code transcript on disk given an exact file path,
// a session ID, or a partial session ID.
func FindClaudeSession(target string) (string, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", fmt.Errorf("session ID or file path cannot be empty")
	}

	clean := filepath.Clean(trimmed)
	if fi, err := os.Stat(clean); err == nil && !fi.IsDir() {
		return clean, nil
	}

	searchDirs := getClaudeSearchDirs()
	for _, dir := range searchDirs {
		if match := findTranscriptInDir(dir, trimmed); match != "" {
			return match, nil
		}
	}

	return "", fmt.Errorf("claude session %q not found as a file or in search directories (~/.claude/projects, ~/.claude/sessions)", target)
}

func getClaudeSearchDirs() []string {
	var dirs []string

	if custom := os.Getenv("CLAUDE_PROJECTS_DIR"); custom != "" {
		dirs = append(dirs, custom)
	}
	if custom := os.Getenv("CLAUDE_SESSIONS_DIR"); custom != "" {
		dirs = append(dirs, custom)
	}

	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		dirs = append(dirs,
			filepath.Join(xdgData, "ai-registry", "claude-code", "projects"),
			filepath.Join(xdgData, "ai-registry", "claude-code", "sessions"),
		)
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".claude", "projects"),
			filepath.Join(home, ".claude", "sessions"),
			filepath.Join(home, ".local", "share", "ai-registry", "claude-code", "projects"),
			filepath.Join(home, ".local", "share", "ai-registry", "claude-code", "sessions"),
		)
	}

	return dirs
}

func findTranscriptInDir(rootDir string, pattern string) string {
	if _, err := os.Stat(rootDir); err != nil {
		return ""
	}

	var bestMatch string
	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		name := d.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			return nil
		}

		baseNoExt := strings.TrimSuffix(name, ".jsonl")
		// Exact session ID match has highest priority
		if baseNoExt == pattern {
			bestMatch = path
			return fs.SkipAll
		}

		// Prefix match has second priority
		if strings.HasPrefix(baseNoExt, pattern) && bestMatch == "" {
			bestMatch = path
			return nil
		}

		// Substring match
		if strings.Contains(baseNoExt, pattern) && bestMatch == "" {
			bestMatch = path
		}

		return nil
	})

	return bestMatch
}
