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

func TestDefaultPiSessionsDir(t *testing.T) {
	t.Run("with PI_CODING_AGENT_DIR set", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", "/custom/pi/dir")
		got := DefaultPiSessionsDir()
		want := filepath.Join("/custom/pi/dir", "sessions")
		if got != want {
			t.Errorf("DefaultPiSessionsDir() = %q, want %q", got, want)
		}
	})

	t.Run("with PI_CODING_AGENT_DIR unset", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot get user home dir")
		}
		got := DefaultPiSessionsDir()
		want := filepath.Join(home, ".pi", "agent", "sessions")
		if got != want {
			t.Errorf("DefaultPiSessionsDir() = %q, want %q", got, want)
		}
	})
}

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

func TestReadPiSessionHeader(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ReadPiSessionHeader(filepath.Join(tmpDir, "nonexistent.jsonl"))
		if err == nil {
			t.Errorf("expected error for nonexistent file")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		emptyFile := filepath.Join(tmpDir, "empty.jsonl")
		_ = os.WriteFile(emptyFile, []byte(""), 0600)
		_, err := ReadPiSessionHeader(emptyFile)
		if err == nil {
			t.Errorf("expected error for empty file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		invalidFile := filepath.Join(tmpDir, "invalid.jsonl")
		_ = os.WriteFile(invalidFile, []byte("{invalid-json\n"), 0600)
		_, err := ReadPiSessionHeader(invalidFile)
		if err == nil {
			t.Errorf("expected error for invalid json")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		wrongTypeFile := filepath.Join(tmpDir, "wrong_type.jsonl")
		_ = os.WriteFile(wrongTypeFile, []byte(`{"type":"message","id":"m1"}`+"\n"), 0600)
		_, err := ReadPiSessionHeader(wrongTypeFile)
		if err == nil {
			t.Errorf("expected error for wrong header type")
		}
	})

	t.Run("empty ID", func(t *testing.T) {
		emptyIDFile := filepath.Join(tmpDir, "empty_id.jsonl")
		_ = os.WriteFile(emptyIDFile, []byte(`{"type":"session","id":""}`+"\n"), 0600)
		_, err := ReadPiSessionHeader(emptyIDFile)
		if err == nil {
			t.Errorf("expected error for empty session id")
		}
	})

	t.Run("valid header", func(t *testing.T) {
		validFile := filepath.Join(tmpDir, "valid.jsonl")
		_ = os.WriteFile(validFile, []byte(`{"type":"session","version":3,"id":"s123","timestamp":"2026-09-19T10:00:00Z","cwd":"/path"}`+"\n"), 0600)
		h, err := ReadPiSessionHeader(validFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h.ID != "s123" || h.Version != 3 || h.CWD != "/path" {
			t.Errorf("header mismatch: %+v", h)
		}
	})
}

func TestLivePiProcessFinder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	procs, err := LivePiProcessFinder(ctx)
	if err != nil {
		t.Logf("LivePiProcessFinder returned error (acceptable if no permission): %v", err)
	}
	t.Logf("LivePiProcessFinder found %d processes", len(procs))
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

	// 1. Session with RFC3339Nano timestamp
	now := time.Now().UTC()
	sessionID1 := "01a0b64b-test-session-1"
	sessionHeader1 := PiHeader{
		Type:      "session",
		Version:   3,
		ID:        sessionID1,
		Timestamp: now.Format(time.RFC3339Nano),
		CWD:       testCWD,
	}
	h1Bytes, _ := json.Marshal(sessionHeader1)
	sessionFilePath1 := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", now.UnixMilli(), sessionID1))
	_ = os.WriteFile(sessionFilePath1, []byte(string(h1Bytes)+"\n"), 0644)

	// 2. Session with standard RFC3339 timestamp (without nanos)
	sessionID2 := "02b0c75c-test-session-2"
	sessionHeader2 := PiHeader{
		Type:      "session",
		Version:   2,
		ID:        sessionID2,
		Timestamp: now.Format(time.RFC3339),
		CWD:       testCWD,
	}
	h2Bytes, _ := json.Marshal(sessionHeader2)
	sessionFilePath2 := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", now.UnixMilli()+1000, sessionID2))
	_ = os.WriteFile(sessionFilePath2, []byte(string(h2Bytes)+"\n"), 0644)

	// 3. Session with empty timestamp (falls back to file modtime)
	sessionID3 := "03c0d86d-test-session-3"
	sessionHeader3 := PiHeader{
		Type:    "session",
		Version: 1,
		ID:      sessionID3,
		CWD:     testCWD,
	}
	h3Bytes, _ := json.Marshal(sessionHeader3)
	sessionFilePath3 := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", now.UnixMilli()+2000, sessionID3))
	_ = os.WriteFile(sessionFilePath3, []byte(string(h3Bytes)+"\n"), 0644)

	// 4. Files to be ignored: subdirectories, non-jsonl, invalid headers
	_ = os.MkdirAll(filepath.Join(projectSessionsDir, "ignored_subdir"), 0755)
	_ = os.WriteFile(filepath.Join(projectSessionsDir, "notes.txt"), []byte("text"), 0644)
	_ = os.WriteFile(filepath.Join(projectSessionsDir, "corrupt.jsonl"), []byte("{bad\n"), 0644)
	_ = os.WriteFile(filepath.Join(sessionsDir, "file_in_root.txt"), []byte("ignored"), 0644)

	// Mock process finder with 2 candidate processes for testCWD
	mockFinder := func(ctx context.Context) ([]PiProcessInfo, error) {
		return []PiProcessInfo{
			{
				PID:        1001,
				CWD:        testCWD,
				CreateTime: now.UnixMilli() - 50, // very close to session 1
			},
			{
				PID:        1002,
				CWD:        testCWD,
				CreateTime: now.UnixMilli() + 950, // very close to session 2
			},
			{
				PID:        1003,
				CWD:        "/other/unrelated/cwd",
				CreateTime: now.UnixMilli(),
			},
			{
				PID:        1004,
				CWD:        testCWD,
				CreateTime: now.UnixMilli() + 100000, // far in future (>5s), should not be paired
			},
		}, nil
	}

	scanner := NewPiScanner(sessionsDir, mockFinder)
	sessions, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scanner.Scan error: %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Verify pairing
	activeCount := 0
	for _, s := range sessions {
		if s.IsActive {
			activeCount++
			if s.PID != 1001 && s.PID != 1002 {
				t.Errorf("unexpected active PID: %d", s.PID)
			}
		}
	}
	if activeCount != 2 {
		t.Errorf("expected 2 active paired sessions, got %d", activeCount)
	}

	// Test NewPiScanner with empty sessionsDir & nil finder
	defaultPi := NewPiScanner("", nil)
	if defaultPi.processFinder == nil {
		t.Errorf("expected default process finder to be set")
	}

	// Test Scan with empty sessionsDir
	emptyScanner := &PiScanner{sessionsDir: ""}
	emptySessions, err := emptyScanner.Scan(context.Background())
	if err != nil || len(emptySessions) != 0 {
		t.Errorf("expected nil, nil for empty sessionsDir, got %v, %v", emptySessions, err)
	}

	// Test Scan with nonexistent sessionsDir
	nonexistentScanner := NewPiScanner("/nonexistent/pi/dir", nil)
	nonexistentSessions, err := nonexistentScanner.Scan(context.Background())
	if err != nil || len(nonexistentSessions) != 0 {
		t.Errorf("expected nil, nil for nonexistent dir, got %v, %v", nonexistentSessions, err)
	}
}
