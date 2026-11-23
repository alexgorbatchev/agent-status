package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/testenv"
	observerscanner "github.com/alexgorbatchev/agent-watcher/scanner"
)

func TestAgyAuditAndDiscovery(t *testing.T) {
	testenv.IsolateHarnessHome(t)
	dir := filepath.Join(t.TempDir(), "brain")
	t.Setenv("AGY_SESSIONS_DIR", dir)
	path := filepath.Join(dir, "session", ".system_generated", "logs", "transcript_full.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"type\":\"USER_INPUT\",\"content\":\"Hello\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sessions, err := LiveAgySessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].SessionID != "session" || sessions[0].FilePath != path {
		t.Fatalf("sessions=%+v err=%v", sessions, err)
	}
	finder := func(context.Context) ([]observerscanner.AgyProcessInfo, error) {
		return []observerscanner.AgyProcessInfo{{PID: 2147483647, CWD: "/repo", CreateTime: 1000}}, nil
	}
	procs, err := LiveAgyProcsWithFinder(context.Background(), finder)
	if err != nil || len(procs) != 1 || procs[0].CWD != "/repo" {
		t.Fatalf("processes=%+v err=%v", procs, err)
	}
	auditor := NewAuditor()
	auditor.NowFunc = func() time.Time { return time.UnixMilli(2000) }
	auditor.AgyProcsFinder = func(context.Context) ([]ProcessInfo, error) { return procs, nil }
	auditor.AgySessionsFinder = func(context.Context, []ProcessInfo) ([]SessionInfo, error) {
		return []SessionInfo{{SessionID: "session", FilePath: path, CWD: "/repo", HeaderTime: 1000, PID: 2147483647}}, nil
	}
	for _, alias := range []string{"agy", "antigravity"} {
		records, err := auditor.Audit(context.Background(), AuditFilter{Harness: alias})
		if err != nil || len(records) != 1 || records[0].Harness != "agy" || records[0].Status != StatusPaired {
			t.Fatalf("records=%+v err=%v", records, err)
		}
	}
	_, err = LiveAgyProcsWithFinder(context.Background(), func(context.Context) ([]observerscanner.AgyProcessInfo, error) {
		return nil, errors.New("process lookup failed")
	})
	if err == nil {
		t.Fatal("finder error lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LiveAgySessions(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestAgyAuditUsesNativeRecentSessionPairing(t *testing.T) {
	testenv.IsolateHarnessHome(t)
	root := t.TempDir()
	brain := filepath.Join(root, "brain")
	t.Setenv("AGY_SESSIONS_DIR", brain)
	conversations := filepath.Join(root, "conversations")
	if err := os.MkdirAll(conversations, 0700); err != nil {
		t.Fatal(err)
	}
	for _, session := range []struct{ id, timestamp string }{
		{"older", "2026-11-21T00:00:01Z"}, {"current", "2026-11-21T00:10:00Z"}, {"child", "2026-11-21T00:20:00Z"},
	} {
		logs := filepath.Join(brain, session.id, ".system_generated", "logs")
		if err := os.MkdirAll(logs, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(logs, "transcript_full.jsonl"), []byte(`{"type":"USER_INPUT","created_at":"`+session.timestamp+`","content":"Hello"}`+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(conversations, session.id+".meta"), []byte(`{"cwd":"/repo"}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	subagents := filepath.Join(brain, "current", ".system_generated", "subagents")
	if err := os.MkdirAll(subagents, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subagents, "child.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	auditor := NewAuditor()
	auditor.AgyProcsFinder = func(context.Context) ([]ProcessInfo, error) {
		return []ProcessInfo{{PID: 42, CWD: "/repo", CreatedAt: 100}}, nil
	}
	records, err := auditor.Audit(context.Background(), AuditFilter{Harness: "agy"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records=%+v", records)
	}
	for _, record := range records {
		if record.SessionID == "current" && (record.Status != StatusPaired || record.PID != 42) {
			t.Fatalf("current session unpaired: %+v", record)
		}
		if record.SessionID == "child" && (record.Status != StatusPaired || record.PID != 42) {
			t.Fatalf("child session unpaired: %+v", record)
		}
		if record.SessionID == "older" && record.Status != StatusOrphanSession {
			t.Fatalf("older session paired: %+v", record)
		}
	}
}
