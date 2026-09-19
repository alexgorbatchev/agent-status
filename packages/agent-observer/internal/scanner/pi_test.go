package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncodePiCWD(t *testing.T) {
	tests := []struct {
		cwd  string
		want string
	}{
		{
			cwd:  "/Users/agorbatchev/development/agent-status",
			want: "--Users-agorbatchev-development-agent-status--",
		},
		{
			cwd:  `C:\Users\dev\project`,
			want: "--C--Users-dev-project--",
		},
		{
			cwd:  "/home/user/workspace/repo",
			want: "--home-user-workspace-repo--",
		},
	}

	for _, tt := range tests {
		t.Run(tt.cwd, func(t *testing.T) {
			got := EncodePiCWD(tt.cwd)
			if got != tt.want {
				t.Fatalf("EncodePiCWD(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestPiScannerDiscoveryAndPairing(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	testCWD := "/Users/test/projects/my-app"
	encodedDir := EncodePiCWD(testCWD)
	projectSessionsDir := filepath.Join(sessionsDir, encodedDir)
	if err := os.MkdirAll(projectSessionsDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	now := time.Now().UTC()
	sessionID := "01a0b64b-test-session"
	sessionHeader := PiHeader{
		Type:      "session",
		Version:   3,
		ID:        sessionID,
		Timestamp: now.Format(time.RFC3339Nano),
		CWD:       testCWD,
	}
	headerBytes, _ := json.Marshal(sessionHeader)
	sessionFilePath := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", now.UnixMilli(), sessionID))

	content := string(headerBytes) + "\n" + `{"type":"message","id":"msg1","message":{"role":"user","content":"hello"}}` + "\n"
	if err := os.WriteFile(sessionFilePath, []byte(content), 0644); err != nil {
		t.Fatalf("write session file error: %v", err)
	}

	// 1. Scan with mock process finder matching the process
	mockFinder := func(ctx context.Context) ([]PiProcessInfo, error) {
		return []PiProcessInfo{
			{
				PID:        4567,
				CWD:        testCWD,
				CreateTime: now.UnixMilli() - 100, // started 100ms before session
			},
		}, nil
	}

	scanner := NewPiScanner(sessionsDir, mockFinder)
	sessions, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scanner.Scan error: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.SessionID != sessionID {
		t.Errorf("s.SessionID = %q, want %q", s.SessionID, sessionID)
	}
	if !s.IsActive {
		t.Errorf("s.IsActive = false, want true")
	}
	if s.PID != 4567 {
		t.Errorf("s.PID = %d, want 4567", s.PID)
	}
	if s.CWD != testCWD {
		t.Errorf("s.CWD = %q, want %q", s.CWD, testCWD)
	}

	// 2. Scan with no matching process (terminated process)
	emptyFinder := func(ctx context.Context) ([]PiProcessInfo, error) {
		return nil, nil
	}
	inactiveScanner := NewPiScanner(sessionsDir, emptyFinder)
	inactiveSessions, err := inactiveScanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("inactiveScanner.Scan error: %v", err)
	}
	if len(inactiveSessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(inactiveSessions))
	}
	if inactiveSessions[0].IsActive {
		t.Errorf("inactive session IsActive = true, want false")
	}
	if inactiveSessions[0].PID != 0 {
		t.Errorf("inactive session PID = %d, want 0", inactiveSessions[0].PID)
	}
}
