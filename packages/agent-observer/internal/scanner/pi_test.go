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
	t.Run("with PI_SESSIONS_DIR set", func(t *testing.T) {
		t.Setenv("PI_SESSIONS_DIR", "/direct/sessions/dir")
		got := DefaultPiSessionsDir()
		if got != "/direct/sessions/dir" {
			t.Errorf("DefaultPiSessionsDir() = %q, want %q", got, "/direct/sessions/dir")
		}
	})

	t.Run("with PI_CODING_AGENT_DIR set", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", "/custom/pi/dir")
		got := DefaultPiSessionsDir()
		want := filepath.Join("/custom/pi/dir", "sessions")
		if got != want {
			t.Errorf("DefaultPiSessionsDir() = %q, want %q", got, want)
		}
	})

	t.Run("with PI_CODING_AGENT_DIR unset and valid HOME", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", "")
		t.Setenv("PI_SESSIONS_DIR", "")
		t.Setenv("HOME", "/custom/user/home")
		got := DefaultPiSessionsDir()
		want := filepath.Join("/custom/user/home", ".pi", "agent", "sessions")
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

	t.Run("valid header with numeric timestamp", func(t *testing.T) {
		validFileFloat := filepath.Join(tmpDir, "valid_float.jsonl")
		_ = os.WriteFile(validFileFloat, []byte(`{"type":"session","version":3,"id":"s-float","timestamp":1700000000000.0,"cwd":"/path"}`+"\n"), 0600)
		h, err := ReadPiSessionHeader(validFileFloat)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h.ID != "s-float" {
			t.Errorf("expected s-float, got %s", h.ID)
		}

		validFileInt := filepath.Join(tmpDir, "valid_int.jsonl")
		_ = os.WriteFile(validFileInt, []byte(`{"type":"session","version":3,"id":"s-int","timestamp":1700000000000,"cwd":"/path"}`+"\n"), 0600)
		h2, err := ReadPiSessionHeader(validFileInt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h2.ID != "s-int" {
			t.Errorf("expected s-int, got %s", h2.ID)
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

	// 3. Session with numeric timestamp (int64/float64)
	sessionID3 := "03c0d86d-test-session-3"
	sessionFilePath3 := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", now.UnixMilli()+2000, sessionID3))
	_ = os.WriteFile(sessionFilePath3, []byte(fmt.Sprintf(`{"type":"session","version":1,"id":%q,"timestamp":%d,"cwd":%q}`+"\n", sessionID3, now.UnixMilli(), testCWD)), 0644)

	// 4. Session with float64 timestamp
	sessionID4 := "04d0e97e-test-session-4"
	sessionFilePath4 := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", now.UnixMilli()+3000, sessionID4))
	_ = os.WriteFile(sessionFilePath4, []byte(fmt.Sprintf(`{"type":"session","version":1,"id":%q,"timestamp":%f,"cwd":%q}`+"\n", sessionID4, float64(now.UnixMilli()), testCWD)), 0644)

	// 5. Session with RFC3339 timestamp (without nano)
	sessionID5 := "05e0f08f-test-session-5"
	sessionFilePath5 := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", now.UnixMilli()+4000, sessionID5))
	_ = os.WriteFile(sessionFilePath5, []byte(fmt.Sprintf(`{"type":"session","version":1,"id":%q,"timestamp":%q,"cwd":%q}`+"\n", sessionID5, now.Format(time.RFC3339), testCWD)), 0644)

	// 6. Files to be ignored: subdirectories, non-jsonl, invalid headers
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

	if len(sessions) != 5 {
		t.Fatalf("expected 5 sessions, got %d", len(sessions))
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

	// Test Scan when sessionsDir is a regular file (triggers read error)
	tmpFile := filepath.Join(tmpDir, "file_as_dir")
	_ = os.WriteFile(tmpFile, []byte("not a dir"), 0644)
	errScanner := NewPiScanner(tmpFile, nil)
	_, scanErr := errScanner.Scan(context.Background())
	if scanErr == nil {
		t.Errorf("expected error when sessionsDir is a file")
	}
}

func TestPiScanner_MultipleSessionsAndResumed(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	testCWD := "/Users/test/projects/multi-resumed-app"
	encodedDir := EncodePiCWD(testCWD)
	projectSessionsDir := filepath.Join(sessionsDir, encodedDir)
	if err := os.MkdirAll(projectSessionsDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	baseTime := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	threeDaysAgo := baseTime.Add(-72 * time.Hour)

	// 1. Older session in testCWD (lexicographically first filename: 1000_old)
	oldID := "01-older-session"
	oldHeader := PiHeader{
		Type:      "session",
		Version:   3,
		ID:        oldID,
		Timestamp: baseTime.Add(-10 * time.Minute).Format(time.RFC3339Nano),
		CWD:       testCWD,
	}
	oldBytes, _ := json.Marshal(oldHeader)
	oldFile := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", baseTime.Add(-10*time.Minute).UnixMilli(), oldID))
	_ = os.WriteFile(oldFile, append(oldBytes, '\n'), 0644)
	_ = os.Chtimes(oldFile, baseTime.Add(-10*time.Minute), baseTime.Add(-10*time.Minute))

	// 2. Newer active session in same testCWD (lexicographically second filename: 2000_new)
	newID := "02-newer-session"
	newHeader := PiHeader{
		Type:      "session",
		Version:   3,
		ID:        newID,
		Timestamp: baseTime.Add(-1 * time.Minute).Format(time.RFC3339Nano),
		CWD:       testCWD,
	}
	newBytes, _ := json.Marshal(newHeader)
	newFile := filepath.Join(projectSessionsDir, fmt.Sprintf("%d_%s.jsonl", baseTime.Add(-1*time.Minute).UnixMilli(), newID))
	_ = os.WriteFile(newFile, append(newBytes, '\n'), 0644)
	_ = os.Chtimes(newFile, baseTime, baseTime)

	// 3. Resumed session in another CWD: header from 3 days ago, but file updated recently (now)
	resumedCWD := "/Users/test/projects/resumed-app"
	resumedEncodedDir := EncodePiCWD(resumedCWD)
	resumedProjectDir := filepath.Join(sessionsDir, resumedEncodedDir)
	_ = os.MkdirAll(resumedProjectDir, 0755)

	resumedID := "03-resumed-session"
	resumedHeader := PiHeader{
		Type:      "session",
		Version:   3,
		ID:        resumedID,
		Timestamp: threeDaysAgo.Format(time.RFC3339Nano),
		CWD:       resumedCWD,
	}
	resumedBytes, _ := json.Marshal(resumedHeader)
	resumedFile := filepath.Join(resumedProjectDir, fmt.Sprintf("%d_%s.jsonl", threeDaysAgo.UnixMilli(), resumedID))
	resumedMsg := fmt.Sprintf(`{"type":"message","timestamp":%q}`+"\n", baseTime.Format(time.RFC3339Nano))
	_ = os.WriteFile(resumedFile, append(append(resumedBytes, '\n'), []byte(resumedMsg)...), 0644)
	_ = os.Chtimes(resumedFile, baseTime, baseTime)

	// Mock process finder:
	// Process 5001 running in testCWD, started at baseTime - 10 minutes
	// Process 5002 running in resumedCWD, started at baseTime
	mockFinder := func(ctx context.Context) ([]PiProcessInfo, error) {
		return []PiProcessInfo{
			{
				PID:        5001,
				CWD:        testCWD,
				CreateTime: baseTime.Add(-10 * time.Minute).UnixMilli(),
			},
			{
				PID:        5002,
				CWD:        resumedCWD,
				CreateTime: baseTime.UnixMilli(),
			},
		}, nil
	}

	scanner := NewPiScanner(sessionsDir, mockFinder)
	sessions, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scanner.Scan error: %v", err)
	}

	sessionByID := make(map[string]PiSessionDescriptor)
	for _, s := range sessions {
		sessionByID[s.SessionID] = s
	}

	// In testCWD, the newer session (02-newer-session) must be paired with PID 5001, NOT the older one
	newDesc, ok := sessionByID[newID]
	if !ok {
		t.Fatalf("session %s not found", newID)
	}
	if !newDesc.IsActive || newDesc.PID != 5001 {
		t.Errorf("expected newer session %s to be active with PID 5001, got active=%v PID=%d", newID, newDesc.IsActive, newDesc.PID)
	}

	oldDesc, ok := sessionByID[oldID]
	if !ok {
		t.Fatalf("session %s not found", oldID)
	}
	if oldDesc.IsActive {
		t.Errorf("expected older session %s to be inactive, got active=%v PID=%d", oldID, oldDesc.IsActive, oldDesc.PID)
	}

	// Resumed session (03-resumed-session) must be paired with PID 5002 despite header being 3 days old
	resumedDesc, ok := sessionByID[resumedID]
	if !ok {
		t.Fatalf("session %s not found", resumedID)
	}
	if !resumedDesc.IsActive || resumedDesc.PID != 5002 {
		t.Errorf("expected resumed session %s to be active with PID 5002, got active=%v PID=%d", resumedID, resumedDesc.IsActive, resumedDesc.PID)
	}
}
