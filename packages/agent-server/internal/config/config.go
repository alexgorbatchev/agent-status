package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Addr          string
	DBPath        string
	SocketPath    string
	IngesterURL   string
	AuthToken     string
	CORSOrigins   []string
	LogLevel      string
	LogDir        string
	LogFile       string
}

func DefaultDBPath() string {
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, "agent-status", "agent-status.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "agent-status", "agent-status.db")
	}
	return filepath.Join(home, ".local", "share", "agent-status", "agent-status.db")
}

func DefaultSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/tmp"
	}
	return filepath.Join(runtimeDir, "agent-status", "notify.sock")
}

func DefaultConfig() Config {
	token := os.Getenv("AGENT_STATUS_TOKEN")
	ingesterURL := os.Getenv("AIR_INGESTER_URL")
	if ingesterURL == "" {
		ingesterURL = "http://127.0.0.1:4501"
	}
	return Config{
		Addr:        ":4502",
		DBPath:      DefaultDBPath(),
		SocketPath:  DefaultSocketPath(),
		IngesterURL: ingesterURL,
		AuthToken:   token,
		CORSOrigins: []string{"http://localhost:3000"},
		LogLevel:    "info",
		LogDir:      os.Getenv("AIR_LOG_DIR"),
		LogFile:     os.Getenv("AIR_LOG_FILE"),
	}
}
