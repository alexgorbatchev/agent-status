package store

import (
	"context"
	"errors"
	"testing"
)

func TestGetAgentUsage(t *testing.T) {
	s := setupTestDB(t)
	seedTestData(t, s)
	const id = "env1:claude-code:sess1"
	if _, err := s.db.Exec(`CREATE TABLE usage_observations (
		agent_id TEXT,observation_id TEXT,subagent_id TEXT,kind TEXT,name TEXT,server TEXT,operation TEXT,
		observed_at INTEGER,source TEXT,event_type TEXT,tool_call_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		key, kind, name, server, operation, sub string
		ts                                      int64
	}{
		{"a", "skill", "review", "", "invoke", "", 1000},
		{"b", "skill", "review", "", "invoke", "child", 2000},
		{"c", "skill", "review", "", "load", "", 3000},
		{"d", "command", "/review", "", "request", "", 4000},
		{"e", "mcp", "list_issues", "github", "invoke", "", 5000},
	} {
		if _, err := s.db.Exec(`INSERT INTO usage_observations VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, row.key, row.sub, row.kind, row.name, row.server, row.operation, row.ts, "tool_metadata", "tool_call", row.key); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.GetAgentUsage(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Skills) != 2 || len(r.Commands) != 1 || len(r.MCP) != 1 {
		t.Fatalf("wrong report: %+v", r)
	}
	invoke := r.Skills[0]
	if invoke.Operation != "invoke" || invoke.Count != 2 || invoke.FirstUsedAt != 1000 || invoke.LastUsedAt != 2000 || invoke.LatestEvidence.SubagentID != "child" || invoke.LatestEvidence.ToolCallID != "b" {
		t.Fatalf("invocations did not aggregate with evidence: %+v", invoke)
	}
	if r.Skills[1].Operation != "load" || r.Skills[1].Count != 1 || r.MCP[0].Server != "github" {
		t.Fatal("usage kinds or operations were conflated")
	}
	if _, err := s.db.Exec("DELETE FROM usage_observations"); err != nil {
		t.Fatal(err)
	}
	r, err = s.GetAgentUsage(context.Background(), id)
	if err != nil || len(r.Skills) != 0 || r.Coverage["skills"].Status != "partial" {
		t.Fatalf("empty usage lost coverage: %+v %v", r, err)
	}
	if _, err := s.GetAgentUsage(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing agent: %v", err)
	}
	if _, err := s.db.Exec("DROP TABLE usage_observations"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAgentUsage(context.Background(), id); err == nil {
		t.Fatal("missing migration must not return an empty list")
	}
}
