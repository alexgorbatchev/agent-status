//go:build darwin

package config

import (
	"bytes"
	"os/exec"
	"strings"
)

func getPlatformMachineID() (string, error) {
	// Try sysctl kern.uuid first
	out, err := exec.Command("sysctl", "-n", "kern.uuid").Output()
	if err == nil {
		id := strings.TrimSpace(string(out))
		if id != "" {
			return id, nil
		}
	}

	// Fallback to ioreg
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	out, err = cmd.Output()
	if err != nil {
		return "", err
	}

	for _, line := range bytes.Split(out, []byte("\n")) {
		lineStr := string(line)
		if strings.Contains(lineStr, "IOPlatformUUID") {
			parts := strings.Split(lineStr, "=")
			if len(parts) >= 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, "\"")
				return val, nil
			}
		}
	}

	return "", nil
}
