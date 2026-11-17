// Package environment exposes the local observer identity to telemetry consumers.
package environment

import "github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/config"

// ID returns the machine-and-user identity used by agent-observer.
func ID() (string, error) {
	return config.GetEnvironmentID()
}
