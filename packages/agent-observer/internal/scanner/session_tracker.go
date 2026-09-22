package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type TrackedSession struct {
	SessionID      string `json:"sessionId"`
	Harness        string `json:"harness"`
	PID            int    `json:"pid"`
	CWD            string `json:"cwd"`
	HarnessVersion string `json:"harnessVersion"`
	StartedAt      int64  `json:"startedAt"`
	IsActive       bool   `json:"isActive"`
}

type SessionTracker struct {
	mu       sync.Mutex
	filePath string
	sessions map[string]TrackedSession
}

func DefaultSessionStatePath() string {
	if cacheHome := os.Getenv("XDG_CACHE_HOME"); cacheHome != "" {
		return filepath.Join(cacheHome, "agent-status", "observer-sessions.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agent-status", "observer-sessions.json")
	}
	return filepath.Join(home, ".cache", "agent-status", "observer-sessions.json")
}

func NewSessionTracker(filePath string) (*SessionTracker, error) {
	if filePath == "" {
		filePath = DefaultSessionStatePath()
	}

	st := &SessionTracker{
		filePath: filePath,
		sessions: make(map[string]TrackedSession),
	}

	data, err := os.ReadFile(filePath)
	if err == nil {
		_ = json.Unmarshal(data, &st.sessions)
	}

	return st, nil
}

func (st *SessionTracker) GetAll() map[string]TrackedSession {
	st.mu.Lock()
	defer st.mu.Unlock()

	copied := make(map[string]TrackedSession, len(st.sessions))
	for k, v := range st.sessions {
		copied[k] = v
	}
	return copied
}

func (st *SessionTracker) Set(sessionID string, s TrackedSession) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[sessionID] = s
}

func (st *SessionTracker) Delete(sessionID string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, sessionID)
}

func (st *SessionTracker) Save() error {
	st.mu.Lock()
	defer st.mu.Unlock()

	dir := filepath.Dir(st.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(st.sessions, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := st.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, st.filePath)
}
