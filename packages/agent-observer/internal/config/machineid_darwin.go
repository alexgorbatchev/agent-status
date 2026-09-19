//go:build darwin

package config

import (
	"bytes"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

func parseIOPlatformUUID(out []byte) string {
	for _, line := range bytes.Split(out, []byte("\n")) {
		lineStr := string(line)
		if strings.Contains(lineStr, "IOPlatformUUID") {
			parts := strings.Split(lineStr, "=")
			if len(parts) >= 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, "\"")
				return val
			}
		}
	}
	return ""
}

func getPlatformMachineID() (string, error) {
	// Try sysctl kern.uuid first
	out, err := execCommand("sysctl", "-n", "kern.uuid").Output()
	if err == nil {
		id := strings.TrimSpace(string(out))
		if id != "" {
			return id, nil
		}
	}

	// Fallback to ioreg
	cmd := execCommand("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	out, err = cmd.Output()
	if err != nil {
		return "", err
	}

	return parseIOPlatformUUID(out), nil
}
