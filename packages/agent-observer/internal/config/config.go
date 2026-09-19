package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

type telemetryConfig struct {
	Environment string `json:"environment"`
	Env         string `json:"env"`
	Label       string `json:"label"`
}

func getDefaultConfigPath() string {
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		return filepath.Join(configHome, "ai-registry", "telemetry.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ai-registry", "telemetry.json")
}

func resolveLabel(flagVal string, configFilePath string) string {
	// 1. --env flag
	if strings.TrimSpace(flagVal) != "" {
		return strings.TrimSpace(flagVal)
	}

	// 2. AI_ENV env var
	if envVar := strings.TrimSpace(os.Getenv("AI_ENV")); envVar != "" {
		return envVar
	}

	// 3. ~/.config/ai-registry/telemetry.json
	if configFilePath == "" {
		configFilePath = getDefaultConfigPath()
	}

	if configFilePath != "" {
		if data, err := os.ReadFile(configFilePath); err == nil {
			var cfg telemetryConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				if cfg.Environment != "" {
					return cfg.Environment
				}
				if cfg.Env != "" {
					return cfg.Env
				}
				if cfg.Label != "" {
					return cfg.Label
				}
			}
		}
	}

	// 4. os.Hostname()
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		return hostname
	}

	return "unknown-host"
}

func GetEnvironmentID() (string, error) {
	machineID, err := getPlatformMachineID()
	if err != nil || machineID == "" {
		hostname, hErr := os.Hostname()
		if hErr != nil {
			machineID = "fallback-machine"
		} else {
			machineID = hostname
		}
	}

	uid := "0"
	if u, err := user.Current(); err == nil {
		uid = u.Uid
	} else if osUID := os.Getenv("UID"); osUID != "" {
		uid = osUID
	}

	raw := machineID + ":" + uid
	hash := sha256.Sum256([]byte(raw))
	hexHash := hex.EncodeToString(hash[:])
	if len(hexHash) > 16 {
		return hexHash[:16], nil
	}
	return hexHash, nil
}

func ResolveEnvironment(envFlag string) (*types.IEnvironmentInfo, error) {
	envID, err := GetEnvironmentID()
	if err != nil {
		return nil, fmt.Errorf("resolving environment id: %w", err)
	}

	label := resolveLabel(envFlag, "")
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	username := "unknown"
	if u, err := user.Current(); err == nil {
		username = u.Username
	} else if uEnv := os.Getenv("USER"); uEnv != "" {
		username = uEnv
	}

	platform := runtime.GOOS
	if platform != "darwin" && platform != "linux" {
		platform = "linux"
	}

	return &types.IEnvironmentInfo{
		ID:              envID,
		Label:           label,
		Hostname:        hostname,
		Platform:        platform,
		User:            username,
		IsStale:         false,
		LastHeartbeatAt: time.Now().UnixMilli(),
	}, nil
}
