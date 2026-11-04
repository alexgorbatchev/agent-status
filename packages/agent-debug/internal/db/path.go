package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// devStackDBPath is where the dev stack's ingester writes, relative to the repository root.
var devStackDBPath = filepath.Join(".tmp", "dev-stack", "data", "agent-status", "agent-status.db")

// ResolveDBPath returns the agent-status database every agent-debug command reads. An
// explicit path (with a leading ~/ expanded) must exist. "" or "auto" searches, in order:
// AGENT_STATUS_DB_PATH; where the installed ingester writes,
// $XDG_DATA_HOME/agent-status/agent-status.db or ~/.local/share/agent-status/agent-status.db;
// and the dev stack's database under the current directory.
func ResolveDBPath(flagVal string) (string, error) {
	if flagVal != "" && flagVal != "auto" {
		path := expandHome(flagVal)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("agent-status database not found at %q: %w", path, err)
		}
		return path, nil
	}
	if override := os.Getenv("AGENT_STATUS_DB_PATH"); override != "" {
		path := expandHome(override)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("agent-status database from AGENT_STATUS_DB_PATH not found at %q: %w", path, err)
		}
		return path, nil
	}
	var candidates []string
	if installed := ingesterDBPath(); installed != "" {
		candidates = append(candidates, installed)
	}
	candidates = append(candidates, devStackDBPath)
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("agent-status database not found in %s; specify --db <path>", strings.Join(candidates, " or "))
}

// ingesterDBPath is where the installed ingester writes by default.
func ingesterDBPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "agent-status", "agent-status.db")
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return filepath.Clean(path)
}
