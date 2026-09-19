package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverClaudeCodeSessions(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("mkdir sessions error: %v", err)
	}

	projectsDir := filepath.Join(tmpDir, "projects", "my-project")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatalf("mkdir projects error: %v", err)
	}

	sessionID := "sess-1234-5678"
	transcriptPath := filepath.Join(projectsDir, sessionID+".jsonl")
	if err := os.WriteFile(transcriptPath, []byte(`{"type":"user","message":"hi"}`+"\n"), 0600); err != nil {
		t.Fatalf("write transcript error: %v", err)
	}

	// Create subagents directory
	subagentsDir := filepath.Join(projectsDir, sessionID, "subagents")
	if err := os.MkdirAll(subagentsDir, 0755); err != nil {
		t.Fatalf("mkdir subagents error: %v", err)
	}
	subagentFile := filepath.Join(subagentsDir, "agent-1.jsonl")
	if err := os.WriteFile(subagentFile, []byte(`{"type":"user","message":"subagent hi"}`+"\n"), 0600); err != nil {
		t.Fatalf("write subagent transcript error: %v", err)
	}

	currentPID := os.Getpid()
	desc := SessionDescriptor{
		PID:            currentPID,
		SessionID:      sessionID,
		CWD:            tmpDir,
		Status:         "busy",
		TranscriptPath: transcriptPath,
		StartedAt:      0,
	}

	descBytes, _ := json.Marshal(desc)
	descFile := filepath.Join(sessionsDir, "current.json")
	if err := os.WriteFile(descFile, descBytes, 0600); err != nil {
		t.Fatalf("write desc file error: %v", err)
	}

	// Also write a .key file to verify it is ignored
	_ = os.WriteFile(filepath.Join(sessionsDir, "session.key"), []byte("secret"), 0600)

	scanner := NewClaudeCodeScanner([]string{sessionsDir})
	sessions, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scanner.Scan() unexpected error = %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.SessionID != sessionID {
		t.Errorf("s.SessionID = %q, want %q", s.SessionID, sessionID)
	}
	if s.PID != currentPID {
		t.Errorf("s.PID = %d, want %d", s.PID, currentPID)
	}
	if s.TranscriptPath != transcriptPath {
		t.Errorf("s.TranscriptPath = %q, want %q", s.TranscriptPath, transcriptPath)
	}

	subagents, err := scanner.DiscoverSubagents(s)
	if err != nil {
		t.Fatalf("DiscoverSubagents error: %v", err)
	}
	if len(subagents) != 1 {
		t.Fatalf("expected 1 subagent transcript, got %d", len(subagents))
	}
	if subagents[0].SubagentID != "agent-1" {
		t.Errorf("subagent ID = %q, want 'agent-1'", subagents[0].SubagentID)
	}
	if subagents[0].TranscriptPath != subagentFile {
		t.Errorf("subagent path = %q, want %q", subagents[0].TranscriptPath, subagentFile)
	}
}

func TestProcessLiveness(t *testing.T) {
	currentPID := os.Getpid()
	if !IsProcessAlive(currentPID) {
		t.Errorf("current PID %d should be alive", currentPID)
	}

	// PID 99999999 is highly unlikely to exist
	if IsProcessAlive(99999999) {
		t.Errorf("PID 99999999 should not be alive")
	}
}
