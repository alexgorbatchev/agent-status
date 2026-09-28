package scanner_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/scanner"
	observerscanner "github.com/alexgorbatchev/agent-status/packages/agent-observer/scanner"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestScannerAudit_PiPairing(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1758400000000)

	auditor := &scanner.Auditor{
		NowFunc: func() time.Time { return now },
		PiProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{
					PID:        27680,
					CreatedAt:  1758347252149,
					CWD:        "/Users/dev/agent-status",
					Cmdline:    "pi",
					MemoryMB:   45.2,
					CPUPercent: 0.5,
				},
			}, nil
		},
		PiSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{
					Harness:    "pi",
					SessionID:  "01a0bd5b-0024-7222-9709-194e2fa6a892",
					CWD:        "/Users/dev/agent-status",
					FilePath:   "/path/to/session.jsonl",
					HeaderTime: 1758347252000,
				},
			}, nil
		},
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "pi"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	r := records[0]
	if r.Harness != "pi" {
		t.Errorf("expected harness 'pi', got %q", r.Harness)
	}
	if r.PID != 27680 {
		t.Errorf("expected PID 27680, got %d", r.PID)
	}
	if r.Status != scanner.StatusPaired {
		t.Errorf("expected status %s, got %s", scanner.StatusPaired, r.Status)
	}
	if r.DeltaMs == nil || *r.DeltaMs != -149 {
		t.Errorf("expected delta -149ms, got %v", r.DeltaMs)
	}
	if r.SessionID != "01a0bd5b-0024-7222-9709-194e2fa6a892" {
		t.Errorf("expected session ID 01a0bd5b-0024-7222-9709-194e2fa6a892, got %q", r.SessionID)
	}
	if r.StatusDetails == "" {
		t.Errorf("expected status details to be computed")
	}
}

func TestScannerAudit_ClaudePairing(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1758400000000)

	auditor := &scanner.Auditor{
		NowFunc: func() time.Time { return now },
		ClaudeProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{
					PID:       4444,
					CreatedAt: 1758399900000,
					CWD:       "/Users/dev/claude-paired",
					Cmdline:   "claude",
				},
			}, nil
		},
		ClaudeSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{
					Harness:    "claude-code",
					SessionID:  "claude-paired-sess",
					CWD:        "/Users/dev/claude-paired",
					PID:        4444,
					HeaderTime: 1758399900020,
					FilePath:   "/path/to/claude.json",
				},
			}, nil
		},
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "claude-code"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != scanner.StatusPaired {
		t.Errorf("expected StatusPaired, got %s", records[0].Status)
	}
	if records[0].DeltaMs == nil || *records[0].DeltaMs != 20 {
		t.Errorf("expected delta 20ms, got %v", records[0].DeltaMs)
	}
}

func TestScannerAudit_CodexPairing(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1758400000000)

	auditor := &scanner.Auditor{
		NowFunc: func() time.Time { return now },
		CodexProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{PID: 1234, CreatedAt: 1758399990000, CWD: "/repo/codex"},
			}, nil
		},
		CodexSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{Harness: "codex", SessionID: "codex-1", CWD: "/repo/codex", HeaderTime: 1758399990050, FilePath: "/f.jsonl"},
			}, nil
		},
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "codex"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != scanner.StatusPaired {
		t.Errorf("expected PAIRED, got %s", records[0].Status)
	}
	if records[0].DeltaMs == nil || *records[0].DeltaMs != 50 {
		t.Errorf("expected delta 50ms, got %v", records[0].DeltaMs)
	}
}

func TestScannerAudit_OpencodePairing(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1758400000000)

	auditor := &scanner.Auditor{
		NowFunc: func() time.Time { return now },
		OpencodeProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{PID: 5678, CreatedAt: 1758399900000, CWD: "/repo/opencode"},
			}, nil
		},
		OpencodeSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{Harness: "opencode", SessionID: "opencode-1", CWD: "/repo/opencode", HeaderTime: 1758399900010, FilePath: "/db.db"},
			}, nil
		},
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "opencode"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != scanner.StatusPaired {
		t.Errorf("expected PAIRED, got %s", records[0].Status)
	}
}

func TestScannerAudit_Orphans(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1758400000000)

	auditor := &scanner.Auditor{
		NowFunc: func() time.Time { return now },
		ClaudeProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{
					PID:        91201,
					CreatedAt:  1758347000000,
					CWD:        "/Users/dev/envoy",
					Cmdline:    "claude",
					MemoryMB:   80.0,
					CPUPercent: 1.2,
				},
			}, nil
		},
		ClaudeSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{
					Harness:    "claude-code",
					SessionID:  "claude-orphan-session",
					CWD:        "/Users/dev/other",
					FilePath:   "/path/to/claude.json",
					HeaderTime: 1758346000000,
					PID:        99999, // dead PID
				},
			}, nil
		},
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "claude"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	var orphanProc, orphanSess *scanner.AuditRecord
	for i := range records {
		switch records[i].Status {
		case scanner.StatusOrphanProcess:
			orphanProc = &records[i]
		case scanner.StatusOrphanSession:
			orphanSess = &records[i]
		}
	}

	if orphanProc == nil {
		t.Fatalf("expected to find ORPHAN_PROCESS record")
	}
	if orphanProc.PID != 91201 {
		t.Errorf("expected orphan proc PID 91201, got %d", orphanProc.PID)
	}

	if orphanSess == nil {
		t.Fatalf("expected to find ORPHAN_SESSION record")
	}
	if orphanSess.SessionID != "claude-orphan-session" {
		t.Errorf("expected orphan sess ID 'claude-orphan-session', got %q", orphanSess.SessionID)
	}
}

func TestScannerAudit_Filtering(t *testing.T) {
	ctx := context.Background()
	auditor := &scanner.Auditor{
		PiProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{PID: 101, CreatedAt: 1000, CWD: "/repo/one"},
			}, nil
		},
		PiSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{Harness: "pi", SessionID: "s1", CWD: "/repo/one", HeaderTime: 1050},
			}, nil
		},
		CodexProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{PID: 202, CreatedAt: 2000, CWD: "/repo/two"},
			}, nil
		},
		CodexSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{Harness: "codex", SessionID: "s2", CWD: "/repo/two", HeaderTime: 2020},
			}, nil
		},
	}

	// Filter by harness
	recPi, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "pi"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}
	if len(recPi) != 1 || recPi[0].Harness != "pi" {
		t.Errorf("expected 1 pi record, got %d", len(recPi))
	}

	// Filter by CWD
	recCWD, err := auditor.Audit(ctx, scanner.AuditFilter{CWD: "/repo/two"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}
	if len(recCWD) != 1 || recCWD[0].Harness != "codex" {
		t.Errorf("expected 1 codex record for /repo/two, got %d", len(recCWD))
	}
}

func TestScannerAudit_LiveFindersWithTempDirs(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// 1. Pi sessions
	piDir := filepath.Join(tmpDir, "pi_sessions")
	piSubDir := filepath.Join(piDir, "--Users-test--")
	_ = os.MkdirAll(piSubDir, 0755)
	// Non-matching file and directory
	_ = os.MkdirAll(filepath.Join(piSubDir, "nested_dir"), 0755)
	_ = os.WriteFile(filepath.Join(piSubDir, "ignore.txt"), []byte("ignore"), 0644)
	_ = os.WriteFile(filepath.Join(piSubDir, "corrupt.jsonl"), []byte("{invalid json}\n"), 0644)

	piFile1 := filepath.Join(piSubDir, "pi_test1.jsonl")
	_ = os.WriteFile(piFile1, []byte(`{"type":"session","id":"pi-sess-1","timestamp":"2026-09-20T05:47:32Z","cwd":"/Users/test"}`+"\n"), 0644)
	piFile2 := filepath.Join(piSubDir, "pi_test2.jsonl")
	_ = os.WriteFile(piFile2, []byte(`{"type":"session","id":"pi-sess-2","timestamp":1758400000000,"cwd":"/Users/test"}`+"\n"), 0644)
	piFile3 := filepath.Join(piSubDir, "pi_test3.jsonl")
	_ = os.WriteFile(piFile3, []byte(`{"type":"session","id":"pi-sess-3","timestamp":1758400000000.5,"cwd":"/Users/test"}`+"\n"), 0644)
	piFile4 := filepath.Join(piSubDir, "pi_test4.jsonl")
	_ = os.WriteFile(piFile4, []byte(`{"type":"session","id":"pi-sess-4","timestamp":"2026-09-20T05:47:32.123456789Z","cwd":"/Users/test"}`+"\n"), 0644)
	piFile5 := filepath.Join(piSubDir, "pi_test5.jsonl")
	_ = os.WriteFile(piFile5, []byte(`{"type":"session","id":"pi-sess-5","cwd":"/Users/test"}`+"\n"), 0644)
	t.Setenv("PI_SESSIONS_DIR", piDir)

	// 2. Claude session
	claudeDir := filepath.Join(tmpDir, "claude_sessions")
	_ = os.MkdirAll(claudeDir, 0755)
	_ = os.MkdirAll(filepath.Join(claudeDir, "nested"), 0755)
	_ = os.WriteFile(filepath.Join(claudeDir, "sess.key"), []byte("key"), 0644)
	_ = os.WriteFile(filepath.Join(claudeDir, "not_json.txt"), []byte("txt"), 0644)
	_ = os.WriteFile(filepath.Join(claudeDir, "corrupt.json"), []byte("{bad"), 0644)
	claudeFile := filepath.Join(claudeDir, "claude_test.json")
	_ = os.WriteFile(claudeFile, []byte(`{"sessionId":"claude-sess-1","pid":12345,"startedAt":1758400000000,"cwd":"/Users/claude"}`), 0644)
	t.Setenv("CLAUDE_SESSIONS_DIR", claudeDir)

	// 3. Codex session
	codexDir := filepath.Join(tmpDir, "codex_sessions", "2026", "09", "21")
	_ = os.MkdirAll(codexDir, 0755)
	_ = os.WriteFile(filepath.Join(codexDir, "not_jsonl.txt"), []byte("txt"), 0644)
	_ = os.WriteFile(filepath.Join(codexDir, "bad.jsonl"), []byte("{bad}\n"), 0644)
	codexFile := filepath.Join(codexDir, "codex_test.jsonl")
	_ = os.WriteFile(codexFile, []byte(`{"type":"session_meta","payload":{"id":"codex-sess-1","timestamp":"2026-09-21T10:00:00Z","cwd":"/Users/codex"}}`+"\n"), 0644)
	t.Setenv("CODEX_SESSIONS_DIR", filepath.Join(tmpDir, "codex_sessions"))

	// 4. OpenCode real SQLite DB
	dbPath := filepath.Join(tmpDir, "opencode.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE session (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			parent_id TEXT,
			slug TEXT,
			directory TEXT,
			title TEXT,
			version TEXT,
			agent TEXT,
			model TEXT,
			cost REAL,
			tokens_input INTEGER,
			tokens_output INTEGER,
			tokens_reasoning INTEGER,
			tokens_cache_read INTEGER,
			tokens_cache_write INTEGER,
			time_created INTEGER,
			time_updated INTEGER,
			time_archived INTEGER,
			metadata TEXT
		);
		INSERT INTO session (id, directory, time_created, time_updated, tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write)
		VALUES ('opencode-sess-1', '/Users/opencode', 1758400000000, 1758400000000, 0, 0, 0, 0, 0);
		INSERT INTO session (id, directory, time_created, time_updated, time_archived, tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write)
		VALUES ('opencode-archived', '/Users/opencode', 1758400000000, 1758400000000, 1758400010000, 0, 0, 0, 0, 0);
	`)
	_ = db.Close()
	if err != nil {
		t.Fatalf("failed to insert opencode session: %v", err)
	}
	t.Setenv("OPENCODE_DB_PATH", dbPath)

	auditor := scanner.NewAuditor()
	if auditor == nil {
		t.Fatalf("expected NewAuditor to return non-nil")
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	foundPi := false
	foundClaude := false
	foundCodex := false
	foundOpencode := false
	for _, r := range records {
		if r.SessionID == "pi-sess-1" {
			foundPi = true
		}
		if r.SessionID == "claude-sess-1" {
			foundClaude = true
		}
		if r.SessionID == "codex-sess-1" {
			foundCodex = true
		}
		if r.SessionID == "opencode-sess-1" {
			foundOpencode = true
		}
		if r.SessionID == "opencode-archived" {
			t.Errorf("expected archived opencode session to be skipped, but found in records")
		}
	}

	if !foundPi {
		t.Errorf("expected to find pi-sess-1 in live session audit")
	}
	if !foundClaude {
		t.Errorf("expected to find claude-sess-1 in live session audit")
	}
	if !foundCodex {
		t.Errorf("expected to find codex-sess-1 in live session audit")
	}
	if !foundOpencode {
		t.Errorf("expected to find opencode-sess-1 in live session audit")
	}
}

func TestScannerAudit_EmptyDirectories(t *testing.T) {
	ctx := context.Background()
	emptyDir := t.TempDir()
	t.Setenv("PI_SESSIONS_DIR", emptyDir)
	t.Setenv("CODEX_SESSIONS_DIR", emptyDir)
	t.Setenv("CLAUDE_SESSIONS_DIR", emptyDir)
	t.Setenv("OPENCODE_DB_PATH", filepath.Join(emptyDir, "nonexistent.db"))

	auditor := scanner.NewAuditor()
	auditor.ClaudeSessionsFinder = func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil }
	auditor.ClaudeProcsFinder = func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil }
	auditor.PiProcsFinder = func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil }
	auditor.CodexProcsFinder = func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil }
	auditor.OpencodeProcsFinder = func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil }

	records, err := auditor.Audit(ctx, scanner.AuditFilter{})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records for empty dirs, got %d", len(records))
	}
}

func TestComputeStatusDetails(t *testing.T) {
	now := time.Now()
	if s := scanner.ComputeStatusDetails(0, now); s != "ACTIVE" {
		t.Errorf("expected ACTIVE for 0 headerTime, got %s", s)
	}
	if s := scanner.ComputeStatusDetails(now.UnixMilli()-5000, now); s != "ACTIVE" {
		t.Errorf("expected ACTIVE for recent headerTime, got %s", s)
	}
	if s := scanner.ComputeStatusDetails(now.UnixMilli()-7200000, now); s != "IDLE 2h" {
		t.Errorf("expected 'IDLE 2h' for 2h old headerTime, got %s", s)
	}
}

func TestScannerAudit_LiveProcessFinders(t *testing.T) {
	ctx := context.Background()
	emptyDir := t.TempDir()
	// Test calling the live process finders on the current system
	auditor := scanner.NewAuditor()
	auditor.ClaudeSessionsFinder = func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil }
	auditor.PiSessionsFinder = func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil }
	auditor.CodexSessionsFinder = func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil }
	auditor.OpencodeSessionsFinder = func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil }

	_, err := auditor.Audit(ctx, scanner.AuditFilter{})
	if err != nil {
		t.Fatalf("Audit with live procs failed: %v", err)
	}

	// Test mock finders through WithFinder helpers
	piProcs, err := scanner.LivePiProcsWithFinder(ctx, func(ctx context.Context) ([]observerscanner.PiProcessInfo, error) {
		return []observerscanner.PiProcessInfo{{PID: os.Getpid(), CreateTime: 1000, CWD: "/test/pi"}}, nil
	})
	if err != nil || len(piProcs) != 1 {
		t.Errorf("LivePiProcsWithFinder failed: %v", err)
	}

	codexProcs, err := scanner.LiveCodexProcsWithFinder(ctx, func(ctx context.Context) ([]observerscanner.CodexProcessInfo, error) {
		return []observerscanner.CodexProcessInfo{{PID: os.Getpid(), CreateTime: 2000, CWD: "/test/codex"}}, nil
	})
	if err != nil || len(codexProcs) != 1 {
		t.Errorf("LiveCodexProcsWithFinder failed: %v", err)
	}

	opencodeProcs, err := scanner.LiveOpencodeProcsWithFinder(ctx, func(ctx context.Context) ([]observerscanner.OpencodeProcessInfo, error) {
		return []observerscanner.OpencodeProcessInfo{{PID: os.Getpid(), CreateTime: 3000, CWD: "/test/opencode"}}, nil
	})
	if err != nil || len(opencodeProcs) != 1 {
		t.Errorf("LiveOpencodeProcsWithFinder failed: %v", err)
	}

	// Test error cases for WithFinder helpers
	_, _ = scanner.LivePiProcsWithFinder(ctx, func(ctx context.Context) ([]observerscanner.PiProcessInfo, error) {
		return nil, os.ErrNotExist
	})
	_, _ = scanner.LiveCodexProcsWithFinder(ctx, func(ctx context.Context) ([]observerscanner.CodexProcessInfo, error) {
		return nil, os.ErrNotExist
	})
	_, _ = scanner.LiveOpencodeProcsWithFinder(ctx, func(ctx context.Context) ([]observerscanner.OpencodeProcessInfo, error) {
		return nil, os.ErrNotExist
	})

	// Test direct LiveSession finders with error conditions
	t.Setenv("OPENCODE_DB_PATH", filepath.Join(emptyDir, "does-not-exist.db"))
	sOpen, err := scanner.LiveOpencodeSessions(ctx)
	if err != nil || len(sOpen) != 0 {
		t.Errorf("expected empty sessions for missing db, got %v, err %v", sOpen, err)
	}

	t.Setenv("PI_SESSIONS_DIR", filepath.Join(emptyDir, "does-not-exist"))
	sPi, err := scanner.LivePiSessions(ctx)
	if err == nil {
		t.Logf("LivePiSessions returned %v", sPi)
	}

	// Test EnrichProcessInfo edge cases
	var info scanner.ProcessInfo
	scanner.EnrichProcessInfo(ctx, 0, &info)
	scanner.EnrichProcessInfo(ctx, -1, &info)
	scanner.EnrichProcessInfo(ctx, 99999999, &info)
	scanner.EnrichProcessInfo(ctx, os.Getpid(), &info)
}

func TestScannerAudit_MultipleOpencodeSessionsSameDB(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1758400000000)

	auditor := &scanner.Auditor{
		NowFunc: func() time.Time { return now },
		OpencodeProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{PID: 101, CreatedAt: 1758399900000, CWD: "/repo/opencode-1"},
				{PID: 102, CreatedAt: 1758399905000, CWD: "/repo/opencode-2"},
			}, nil
		},
		OpencodeSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{
					Harness:    "opencode",
					SessionID:  "opencode-sess-1",
					CWD:        "/repo/opencode-1",
					HeaderTime: 1758399900010,
					FilePath:   "/shared/opencode.db",
				},
				{
					Harness:    "opencode",
					SessionID:  "opencode-sess-2",
					CWD:        "/repo/opencode-2",
					HeaderTime: 1758399905010,
					FilePath:   "/shared/opencode.db", // Same db file!
				},
			}, nil
		},
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "opencode"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	for _, r := range records {
		if r.Status != scanner.StatusPaired {
			t.Errorf("expected both sessions to be PAIRED, got PID %d status %s (sessionId: %s)", r.PID, r.Status, r.SessionID)
		}
	}
}

func TestScannerAudit_MultipleClaudeSessionsSamePID(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1758400000000)

	auditor := &scanner.Auditor{
		NowFunc: func() time.Time { return now },
		ClaudeProcsFinder: func(ctx context.Context) ([]scanner.ProcessInfo, error) {
			return []scanner.ProcessInfo{
				{PID: 5555, CreatedAt: 1758399900000, CWD: "/repo/claude", Cmdline: "claude"},
			}, nil
		},
		ClaudeSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) {
			return []scanner.SessionInfo{
				{
					Harness:    "claude-code",
					SessionID:  "claude-new",
					CWD:        "/repo/claude",
					PID:        5555,
					HeaderTime: 1758399950000, // Newer session
					FilePath:   "/path/to/claude-new.json",
				},
				{
					Harness:    "claude-code",
					SessionID:  "claude-old",
					CWD:        "/repo/claude",
					PID:        5555,
					HeaderTime: 1758399910000, // Older session referencing same PID
					FilePath:   "/path/to/claude-old.json",
				},
			}, nil
		},
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{Harness: "claude-code"})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	var pairedCount, orphanSessionCount int
	for _, r := range records {
		switch r.Status {
		case scanner.StatusPaired:
			pairedCount++
			if r.SessionID != "claude-new" {
				t.Errorf("expected newest session claude-new to be paired, got %s", r.SessionID)
			}
		case scanner.StatusOrphanSession:
			orphanSessionCount++
			if r.SessionID != "claude-old" {
				t.Errorf("expected older session claude-old to be orphan session, got %s", r.SessionID)
			}
		default:
			t.Errorf("unexpected status %s for record %v", r.Status, r)
		}
	}

	if pairedCount != 1 {
		t.Errorf("expected exactly 1 PAIRED record, got %d", pairedCount)
	}
	if orphanSessionCount != 1 {
		t.Errorf("expected exactly 1 ORPHAN_SESSION record, got %d", orphanSessionCount)
	}
}

func TestScannerAudit_EmptyJSONOutput(t *testing.T) {
	ctx := context.Background()
	auditor := &scanner.Auditor{
		PiProcsFinder:          func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil },
		PiSessionsFinder:       func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil },
		ClaudeProcsFinder:      func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil },
		ClaudeSessionsFinder:   func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil },
		CodexProcsFinder:       func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil },
		CodexSessionsFinder:    func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil },
		OpencodeProcsFinder:    func(ctx context.Context) ([]scanner.ProcessInfo, error) { return nil, nil },
		OpencodeSessionsFinder: func(ctx context.Context) ([]scanner.SessionInfo, error) { return nil, nil },
	}

	records, err := auditor.Audit(ctx, scanner.AuditFilter{})
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	if records == nil {
		t.Errorf("expected non-nil slice from Audit, got nil")
	}

	data, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("expected JSON '[]', got %q", string(data))
	}
}
