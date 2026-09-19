//go:build !darwin && !linux

package config

import "os"

func getPlatformMachineID() (string, error) {
	return os.Hostname()
}
