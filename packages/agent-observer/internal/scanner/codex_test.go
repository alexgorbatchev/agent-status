package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultCodexSessionsDir(t *testing.T) {
	t.Run("with CODEX_SESSIONS_DIR set", func(t *testing.T) {
		t.Setenv("CODEX_SESSIONS_DIR", "/custom/codex/sessions")
		got := DefaultCodexSessionsDir()
		if got != "/custom/codex/sessions" {
			t.Errorf("DefaultCodexSessionsDir() = %q, want %q", got, "/custom/codex/sessions")
		}
	})

	t.Run("with CODEX_HOME set", func(t *testing.T) {
		t.Setenv("CODEX_SESSIONS_DIR", "")
		t.Setenv("CODEX_HOME", "/custom/codex/home")
		got := DefaultCodexSessionsDir()
		want := filepath.Join("/custom/codex/home", "sessions")
		if got != want {
			t.Errorf("DefaultCodexSessionsDir() = %q, want %q", got, want)
		}
	})

	t.Run("with env unset and valid HOME", func(t *testing.T) {
		t.Setenv("CODEX_SESSIONS_DIR", "")
		t.Setenv("CODEX_HOME", "")
		t.Setenv("HOME", "/custom/user/home")
		got := DefaultCodexSessionsDir()
		want := filepath.Join("/custom/user/home", ".codex", "sessions")
		if got != want {
			t.Errorf("DefaultCodexSessionsDir() = %q, want %q", got, want)
		}
	})
}

func TestParseRFC3339Time(t *testing.T) {
	t.Run("RFC3339Nano", func(t *testing.T) {
		ts := "2026-06-12T16:08:36.123456789Z"
		res := parseRFC3339Time(ts)
		if res <= 0 {
			t.Errorf("parseRFC3339Time(%q) = %d, want > 0", ts, res)
		}
	})

	t.Run("RFC3339", func(t *testing.T) {
		ts := "2026-06-12T16:08:36Z"
		res := parseRFC3339Time(ts)
		if res <= 0 {
			t.Errorf("parseRFC3339Time(%q) = %d, want > 0", ts, res)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		res := parseRFC3339Time("invalid-date-string")
		if res != 0 {
			t.Errorf("parseRFC3339Time(invalid) = %d, want 0", res)
		}
	})
}

func TestReadCodexSessionHeader(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ReadCodexSessionHeader(filepath.Join(tmpDir, "nonexistent.jsonl"))
		if err == nil {
			t.Errorf("expected error for nonexistent file")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		emptyFile := filepath.Join(tmpDir, "empty.jsonl")
		_ = os.WriteFile(emptyFile, []byte(""), 0600)
		_, err := ReadCodexSessionHeader(emptyFile)
		if err == nil {
			t.Errorf("expected error for empty file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		invalidFile := filepath.Join(tmpDir, "invalid.jsonl")
		_ = os.WriteFile(invalidFile, []byte("{not-json\n"), 0600)
		_, err := ReadCodexSessionHeader(invalidFile)
		if err == nil {
			t.Errorf("expected error for invalid json")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		wrongTypeFile := filepath.Join(tmpDir, "wrong_type.jsonl")
		_ = os.WriteFile(wrongTypeFile, []byte(`{"type":"turn_context","payload":{}}`+"\n"), 0600)
		_, err := ReadCodexSessionHeader(wrongTypeFile)
		if err == nil {
			t.Errorf("expected error for wrong header type")
		}
	})

	t.Run("valid session meta header", func(t *testing.T) {
		validFile := filepath.Join(tmpDir, "valid.jsonl")
		raw := map[string]interface{}{
			"type": "session_meta",
			"payload": map[string]interface{}{
				"id":          "codex-sess-123",
				"timestamp":   "2026-06-12T16:08:36.123Z",
				"cwd":         "/Users/dev/project",
				"cli_version": "0.121.0",
			},
		}
		data, _ := json.Marshal(raw)
		_ = os.WriteFile(validFile, append(data, '\n'), 0600)

		header, err := ReadCodexSessionHeader(validFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if header.ID != "codex-sess-123" {
			t.Errorf("header.ID = %q, want 'codex-sess-123'", header.ID)
		}
		if header.CWD != "/Users/dev/project" {
			t.Errorf("header.CWD = %q, want '/Users/dev/project'", header.CWD)
		}
		if header.CLIVersion != "0.121.0" {
			t.Errorf("header.CLIVersion = %q, want '0.121.0'", header.CLIVersion)
		}
		if header.IsSubagent {
			t.Errorf("expected IsSubagent to be false")
		}
	})

	t.Run("valid header with outer timestamp variants", func(t *testing.T) {
		// String outer timestamp
		f1 := filepath.Join(tmpDir, "outer_str.jsonl")
		raw1 := map[string]interface{}{
			"type":      "session_meta",
			"timestamp": "2026-06-12T16:08:36Z",
			"payload": map[string]interface{}{
				"id":  "codex-str-ts",
				"cwd": "/Users/dev/p1",
			},
		}
		d1, _ := json.Marshal(raw1)
		_ = os.WriteFile(f1, append(d1, '\n'), 0600)
		h1, err := ReadCodexSessionHeader(f1)
		if err != nil || h1.Timestamp == 0 {
			t.Errorf("expected parsed timestamp from outer string, got %v, err %v", h1, err)
		}

		// Float64 outer timestamp
		f2 := filepath.Join(tmpDir, "outer_float.jsonl")
		raw2 := map[string]interface{}{
			"type":      "session_meta",
			"timestamp": float64(1750000000000),
			"payload": map[string]interface{}{
				"id":  "codex-float-ts",
				"cwd": "/Users/dev/p2",
			},
		}
		d2, _ := json.Marshal(raw2)
		_ = os.WriteFile(f2, append(d2, '\n'), 0600)
		h2, err := ReadCodexSessionHeader(f2)
		if err != nil || h2.Timestamp != 1750000000000 {
			t.Errorf("expected parsed timestamp from outer float64, got %v, err %v", h2, err)
		}

		// Int64 outer timestamp
		f3 := filepath.Join(tmpDir, "outer_int.jsonl")
		raw3 := map[string]interface{}{
			"type":      "session_meta",
			"timestamp": int64(1750000000100),
			"payload": map[string]interface{}{
				"id":  "codex-int-ts",
				"cwd": "/Users/dev/p3",
			},
		}
		d3, _ := json.Marshal(raw3)
		_ = os.WriteFile(f3, append(d3, '\n'), 0600)
		h3, err := ReadCodexSessionHeader(f3)
		if err != nil || h3.Timestamp != 1750000000100 {
			t.Errorf("expected parsed timestamp from outer int64, got %v, err %v", h3, err)
		}
	})

	t.Run("valid subagent header with parent_thread_id", func(t *testing.T) {
		subFile := filepath.Join(tmpDir, "subagent.jsonl")
		raw := map[string]interface{}{
			"type": "session_meta",
			"payload": map[string]interface{}{
				"id":               "sub-sess-456",
				"timestamp":        "2026-06-12T16:10:00Z",
				"cwd":              "/Users/dev/project",
				"cli_version":      "0.121.0",
				"parent_thread_id": "codex-sess-123",
				"agent_nickname":   "Explorer",
				"agent_role":       "explorer",
			},
		}
		data, _ := json.Marshal(raw)
		_ = os.WriteFile(subFile, append(data, '\n'), 0600)

		header, err := ReadCodexSessionHeader(subFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !header.IsSubagent {
			t.Errorf("expected IsSubagent to be true")
		}
		if header.ParentThreadID != "codex-sess-123" {
			t.Errorf("header.ParentThreadID = %q, want 'codex-sess-123'", header.ParentThreadID)
		}
		if header.AgentNickname != "Explorer" {
			t.Errorf("header.AgentNickname = %q, want 'Explorer'", header.AgentNickname)
		}
	})

	t.Run("valid subagent header with source map", func(t *testing.T) {
		subSourceFile := filepath.Join(tmpDir, "sub_source.jsonl")
		raw := map[string]interface{}{
			"type": "session_meta",
			"payload": map[string]interface{}{
				"id":          "sub-sess-789",
				"timestamp":   "2026-06-12T16:12:00Z",
				"cwd":         "/Users/dev/project",
				"cli_version": "0.121.0",
				"source": map[string]interface{}{
					"subagent": map[string]interface{}{
						"role": "reviewer",
					},
				},
			},
		}
		data, _ := json.Marshal(raw)
		_ = os.WriteFile(subSourceFile, append(data, '\n'), 0600)

		header, err := ReadCodexSessionHeader(subSourceFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !header.IsSubagent {
			t.Errorf("expected IsSubagent to be true for source.subagent")
		}
	})

	t.Run("valid subagent header with source string", func(t *testing.T) {
		subStrFile := filepath.Join(tmpDir, "sub_str.jsonl")
		raw := map[string]interface{}{
			"type": "session_meta",
			"payload": map[string]interface{}{
				"id":          "sub-sess-str",
				"timestamp":   "2026-06-12T16:12:00Z",
				"cwd":         "/Users/dev/project",
				"cli_version": "0.121.0",
				"source":      "reviewer-worker",
			},
		}
		data, _ := json.Marshal(raw)
		_ = os.WriteFile(subStrFile, append(data, '\n'), 0600)

		header, err := ReadCodexSessionHeader(subStrFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !header.IsSubagent {
			t.Errorf("expected IsSubagent to be true for non-cli source string")
		}
	})

	t.Run("fallback to rollout filename session ID", func(t *testing.T) {
		rolloutFile := filepath.Join(tmpDir, "rollout-2026-06-12T16-08-36-my-custom-uuid.jsonl")
		raw := map[string]interface{}{
			"type": "session_meta",
			"payload": map[string]interface{}{
				"cwd":         "/Users/dev/project",
				"cli_version": "0.121.0",
			},
		}
		data, _ := json.Marshal(raw)
		_ = os.WriteFile(rolloutFile, append(data, '\n'), 0600)

		header, err := ReadCodexSessionHeader(rolloutFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if header.ID != "my-custom-uuid" {
			t.Errorf("header.ID = %q, want 'my-custom-uuid'", header.ID)
		}
	})
}

func TestLiveCodexProcessFinder(t *testing.T) {
	tmpDir := t.TempDir()
	codexBin := filepath.Join(tmpDir, "codex")
	if sleepData, err := os.ReadFile("/bin/sleep"); err == nil {
		if err := os.WriteFile(codexBin, sleepData, 0755); err == nil {
			cmd := exec.Command(codexBin, "5")
			if err := cmd.Start(); err == nil {
				defer func() {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}()
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	procs, err := LiveCodexProcessFinder(ctx)
	if err != nil {
		t.Logf("LiveCodexProcessFinder returned error (acceptable if no permission): %v", err)
	}
	t.Logf("LiveCodexProcessFinder found %d processes", len(procs))
}

func TestCodexScannerDiscoveryAndPairing(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	testCWD := "/Users/test/projects/my-codex-app"

	// 1. Date-partitioned session: sessions/2026/06/12/rollout-2026-06-12T16-08-36-session-1.jsonl
	dateDir := filepath.Join(sessionsDir, "2026", "06", "12")
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	now := time.Now().UTC()
	sess1ID := "sess-codex-1"
	sess1File := filepath.Join(dateDir, fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), sess1ID))
	meta1 := map[string]interface{}{
		"type": "session_meta",
		"payload": map[string]interface{}{
			"id":          sess1ID,
			"timestamp":   now.Format(time.RFC3339Nano),
			"cwd":         testCWD,
			"cli_version": "0.121.0",
		},
	}
	m1Bytes, _ := json.Marshal(meta1)
	_ = os.WriteFile(sess1File, append(m1Bytes, '\n'), 0644)

	// 2. Flat rollout session in root of sessionsDir
	sess2ID := "sess-codex-2"
	sess2File := filepath.Join(sessionsDir, fmt.Sprintf("rollout-%s-%s.jsonl", now.Add(time.Second).Format("2006-01-02T15-04-05"), sess2ID))
	meta2 := map[string]interface{}{
		"type": "session_meta",
		"payload": map[string]interface{}{
			"id":          sess2ID,
			"timestamp":   now.Add(time.Second).Format(time.RFC3339),
			"cwd":         testCWD,
			"cli_version": "0.121.0",
		},
	}
	m2Bytes, _ := json.Marshal(meta2)
	_ = os.WriteFile(sess2File, append(m2Bytes, '\n'), 0644)

	// 3. Ignored files: text file, invalid jsonl, subagent directory
	_ = os.WriteFile(filepath.Join(dateDir, "notes.txt"), []byte("ignored"), 0644)
	_ = os.WriteFile(filepath.Join(dateDir, "corrupt.jsonl"), []byte("{bad\n"), 0644)
	_ = os.MkdirAll(filepath.Join(dateDir, "nested_dir"), 0755)

	mockFinder := func(ctx context.Context) ([]CodexProcessInfo, error) {
		return []CodexProcessInfo{
			{
				PID:        2001,
				CWD:        testCWD,
				CreateTime: now.UnixMilli() - 50,
			},
			{
				PID:        2002,
				CWD:        "/other/cwd",
				CreateTime: now.UnixMilli(),
			},
		}, nil
	}

	scanner := NewCodexScanner(sessionsDir, mockFinder)
	sessions, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scanner.Scan error: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	activeCount := 0
	for _, s := range sessions {
		if s.IsActive {
			activeCount++
			if s.PID != 2001 {
				t.Errorf("unexpected active PID: %d, want 2001", s.PID)
			}
		}
	}
	if activeCount != 1 {
		t.Errorf("expected 1 active paired session, got %d", activeCount)
	}

	// Test with AGENT_STATUS_TEST_MOCK_PROC = "1"
	t.Run("with AGENT_STATUS_TEST_MOCK_PROC", func(t *testing.T) {
		t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "1")
		mockEmptyFinder := func(ctx context.Context) ([]CodexProcessInfo, error) {
			return nil, nil
		}
		mockScanner := NewCodexScanner(sessionsDir, mockEmptyFinder)
		mockSessions, err := mockScanner.Scan(context.Background())
		if err != nil {
			t.Fatalf("mockScanner.Scan error: %v", err)
		}
		for _, s := range mockSessions {
			if !s.IsActive || s.PID != 99993 {
				t.Errorf("expected mock active session PID 99993, got %+v", s)
			}
		}
	})

	// Test NewCodexScanner with empty sessionsDir
	defaultCodex := NewCodexScanner("", nil)
	if defaultCodex.processFinder == nil {
		t.Errorf("expected default process finder to be set")
	}

	// Test Scan with empty sessionsDir
	emptyScanner := &CodexScanner{sessionsDir: ""}
	emptySessions, err := emptyScanner.Scan(context.Background())
	if err != nil || len(emptySessions) != 0 {
		t.Errorf("expected nil, nil for empty sessionsDir, got %v, %v", emptySessions, err)
	}

	// Test Scan with nonexistent sessionsDir
	nonexistentScanner := NewCodexScanner("/nonexistent/codex/dir", nil)
	nonexistentSessions, err := nonexistentScanner.Scan(context.Background())
	if err != nil || len(nonexistentSessions) != 0 {
		t.Errorf("expected nil, nil for nonexistent dir, got %v, %v", nonexistentSessions, err)
	}
}

func TestCodexScanner_MultipleSessionsAndResumed(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	testCWD := "/Users/test/projects/my-codex-multi"
	dateDir := filepath.Join(sessionsDir, "2026", "06", "12")
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	baseTime := time.Date(2026, 6, 12, 16, 0, 0, 0, time.UTC)
	threeDaysAgo := baseTime.Add(-72 * time.Hour)

	// 1. Older session in testCWD (lexicographically first filename: rollout-2026-06-12T15-50-00)
	oldID := "sess-codex-old"
	oldFile := filepath.Join(dateDir, fmt.Sprintf("rollout-2026-06-12T15-50-00-%s.jsonl", oldID))
	metaOld := map[string]interface{}{
		"type": "session_meta",
		"payload": map[string]interface{}{
			"id":          oldID,
			"timestamp":   baseTime.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			"cwd":         testCWD,
			"cli_version": "0.121.0",
		},
	}
	mOldBytes, _ := json.Marshal(metaOld)
	_ = os.WriteFile(oldFile, append(mOldBytes, '\n'), 0644)
	_ = os.Chtimes(oldFile, baseTime.Add(-10*time.Minute), baseTime.Add(-10*time.Minute))

	// 2. Newer active session in same testCWD (lexicographically second filename: rollout-2026-06-12T15-59-00)
	newID := "sess-codex-new"
	newFile := filepath.Join(dateDir, fmt.Sprintf("rollout-2026-06-12T15-59-00-%s.jsonl", newID))
	metaNew := map[string]interface{}{
		"type": "session_meta",
		"payload": map[string]interface{}{
			"id":          newID,
			"timestamp":   baseTime.Add(-1 * time.Minute).Format(time.RFC3339Nano),
			"cwd":         testCWD,
			"cli_version": "0.121.0",
		},
	}
	mNewBytes, _ := json.Marshal(metaNew)
	_ = os.WriteFile(newFile, append(mNewBytes, '\n'), 0644)
	_ = os.Chtimes(newFile, baseTime, baseTime)

	// 3. Resumed session in another CWD: header from 3 days ago, but file updated recently (now)
	resumedCWD := "/Users/test/projects/codex-resumed"
	resumedDir := filepath.Join(sessionsDir, "2026", "06", "09")
	_ = os.MkdirAll(resumedDir, 0755)

	resumedID := "sess-codex-resumed"
	resumedFile := filepath.Join(resumedDir, fmt.Sprintf("rollout-2026-06-09T16-00-00-%s.jsonl", resumedID))
	metaResumed := map[string]interface{}{
		"type": "session_meta",
		"payload": map[string]interface{}{
			"id":          resumedID,
			"timestamp":   threeDaysAgo.Format(time.RFC3339Nano),
			"cwd":         resumedCWD,
			"cli_version": "0.121.0",
		},
	}
	mResumedBytes, _ := json.Marshal(metaResumed)
	resumedMsg := fmt.Sprintf(`{"type":"event_msg","timestamp":%q}`+"\n", baseTime.Format(time.RFC3339Nano))
	_ = os.WriteFile(resumedFile, append(append(mResumedBytes, '\n'), []byte(resumedMsg)...), 0644)
	_ = os.Chtimes(resumedFile, baseTime, baseTime)

	// Mock process finder:
	// Process 6001 in testCWD started at baseTime - 10 minutes
	// Process 6002 in resumedCWD started at baseTime
	mockFinder := func(ctx context.Context) ([]CodexProcessInfo, error) {
		return []CodexProcessInfo{
			{
				PID:        6001,
				CWD:        testCWD,
				CreateTime: baseTime.Add(-10 * time.Minute).UnixMilli(),
			},
			{
				PID:        6002,
				CWD:        resumedCWD,
				CreateTime: baseTime.UnixMilli(),
			},
		}, nil
	}

	scanner := NewCodexScanner(sessionsDir, mockFinder)
	sessions, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scanner.Scan error: %v", err)
	}

	sessionByID := make(map[string]CodexSessionDescriptor)
	for _, s := range sessions {
		sessionByID[s.SessionID] = s
	}

	// In testCWD, the newer session must be paired with PID 6001
	newDesc, ok := sessionByID[newID]
	if !ok {
		t.Fatalf("session %s not found", newID)
	}
	if !newDesc.IsActive || newDesc.PID != 6001 {
		t.Errorf("expected newer session %s to be active with PID 6001, got active=%v PID=%d", newID, newDesc.IsActive, newDesc.PID)
	}

	oldDesc, ok := sessionByID[oldID]
	if !ok {
		t.Fatalf("session %s not found", oldID)
	}
	if oldDesc.IsActive {
		t.Errorf("expected older session %s to be inactive, got active=%v PID=%d", oldID, oldDesc.IsActive, oldDesc.PID)
	}

	// Resumed session must be paired with PID 6002 despite 3-day old header
	resumedDesc, ok := sessionByID[resumedID]
	if !ok {
		t.Fatalf("session %s not found", resumedID)
	}
	if !resumedDesc.IsActive || resumedDesc.PID != 6002 {
		t.Errorf("expected resumed session %s to be active with PID 6002, got active=%v PID=%d", resumedID, resumedDesc.IsActive, resumedDesc.PID)
	}
}

func TestCodexScanner_SkipsProcessFinderWhenNoCandidateSessions(t *testing.T) {
	tmpDir := t.TempDir()
	emptySessionsDir := filepath.Join(tmpDir, "empty_sessions")
	if err := os.MkdirAll(emptySessionsDir, 0755); err != nil {
		t.Fatalf("failed to create empty sessions dir: %v", err)
	}

	finderCalled := 0
	mockFinder := func(ctx context.Context) ([]CodexProcessInfo, error) {
		finderCalled++
		return nil, nil
	}

	scanner := NewCodexScanner(emptySessionsDir, mockFinder)
	sessions, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error from Scan: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
	if finderCalled != 0 {
		t.Errorf("expected process finder NOT to be called when sessions dir has 0 candidate files, but it was called %d times", finderCalled)
	}
}

func TestCodexScanner_SetProcessFinderAndProvider(t *testing.T) {
	scanner := NewCodexScanner("/dummy", nil)

	customFinder := func(ctx context.Context) ([]CodexProcessInfo, error) {
		return []CodexProcessInfo{{PID: 234}}, nil
	}
	scanner.SetProcessFinder(customFinder)
	procs, err := scanner.processFinder(context.Background())
	if err != nil || len(procs) != 1 || procs[0].PID != 234 {
		t.Errorf("SetProcessFinder failed: %v, %v", procs, err)
	}

	snapProvider := ProcessSnapshotProviderFunc(func(ctx context.Context) (ProcessSnapshot, error) {
		return &testDummySnapshot{alivePID: 102}, nil
	})
	scanner.SetProcessSnapshotProvider(snapProvider)
	procs, err = scanner.processFinder(context.Background())
	if err != nil || len(procs) != 1 || procs[0].PID != 102 {
		t.Errorf("SetProcessSnapshotProvider failed: %v, %v", procs, err)
	}
}


