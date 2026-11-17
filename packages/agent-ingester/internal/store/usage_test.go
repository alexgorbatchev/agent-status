package store

import (
	"encoding/json"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func usageFixture(t *testing.T) (*Store, types.ITelemetryFrame) {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := types.IAgentState{ID: "usage-agent", SessionID: "usage-session", Harness: types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{ID: "env", Platform: "linux"}, Activity: types.IAgentActivity{Activity: types.ActivityIdle, UpdatedAt: 10000}}
	if err := s.SaveAgentState(a); err != nil {
		t.Fatal(err)
	}
	name, id := "Skill", "call-1"
	f := types.ITelemetryFrame{AgentID: a.ID, Harness: a.Harness, EventType: "tool_call", Timestamp: 1000,
		Data: types.TelemetryFrameData{ToolName: &name, ToolCallID: &id, ToolInput: map[string]any{"skill": "review", "args": "private arguments"}}}
	return s, f
}

func TestUsagePersistenceAndRetention(t *testing.T) {
	s, f := usageFixture(t)
	for i := range 3 {
		f.Seq = int64(i)
		if err := s.RecordTelemetryEvent(f); err != nil {
			t.Fatal(err)
		}
	}
	child := "child"
	f.SubagentID = &child
	if err := s.RecordTelemetryEvent(f); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM usage_observations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("got %d observations; replay must deduplicate and subagents must remain distinct", count)
	}
	if err := s.PruneOldEventsAndExitedAgents(2000, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM usage_observations").Scan(&count); err != nil || count != 2 {
		t.Fatalf("usage lost after telemetry pruning: %d %v", count, err)
	}
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("telemetry not pruned: %d %v", count, err)
	}
	if err := s.RecordTelemetryEvent(f); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM usage_observations").Scan(&count); err != nil || count != 2 {
		t.Fatalf("replay after pruning duplicated observations: %d %v", count, err)
	}
	if _, err := s.DB().Exec("DELETE FROM agents WHERE id=?", f.AgentID); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM usage_observations").Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned observations: %d %v", count, err)
	}
}

func TestUsagePersistenceIsAtomic(t *testing.T) {
	s, f := usageFixture(t)
	if _, err := s.DB().Exec(`CREATE TRIGGER fail_usage BEFORE INSERT ON usage_observations BEGIN SELECT RAISE(FAIL,'usage storage failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordTelemetryEvent(f); err == nil {
		t.Fatal("expected usage persistence failure")
	}
	var count int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partially committed telemetry: %d %v", count, err)
	}
}

func TestUsageMigrationBackfillsAndRollsBack(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "backfill", true: "rollback"}[invalid], func(t *testing.T) {
			s, f := usageFixture(t)
			if _, err := s.DB().Exec("DROP TABLE IF EXISTS usage_observations; PRAGMA user_version=9"); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(f.Data)
			if err != nil {
				t.Fatal(err)
			}
			if invalid {
				data = []byte("invalid JSON")
			}
			for range 2 {
				if _, err := s.DB().Exec("INSERT INTO telemetry_events(agent_id,event_type,timestamp,data_json) VALUES(?,?,?,?)", f.AgentID, f.EventType, f.Timestamp, string(data)); err != nil {
					t.Fatal(err)
				}
			}
			err = Migrate(s.DB())
			if invalid {
				if err == nil {
					t.Fatal("expected malformed historical telemetry to abort migration")
				}
				var version int
				if err := s.DB().QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 9 {
					t.Fatalf("migration did not roll back: %d %v", version, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := Migrate(s.DB()); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := s.DB().QueryRow("SELECT COUNT(*) FROM usage_observations").Scan(&count); err != nil || count != 1 {
				t.Fatalf("backfill not deduplicated: %d %v", count, err)
			}
		})
	}
}

func TestUsageMigrationStorageFailures(t *testing.T) {
	t.Run("closed database", func(t *testing.T) {
		s, _ := usageFixture(t)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if err := migrateV10(s.DB()); err == nil {
			t.Fatal("migration accepted a closed database")
		}
	})
	for _, tc := range []struct {
		name, setup string
	}{
		{"schema conflict", "DROP TABLE usage_observations; CREATE VIEW usage_observations AS SELECT 1"},
		{"missing telemetry", "DROP TABLE telemetry_events"},
		{"observation write failure", `CREATE TRIGGER fail_usage BEFORE INSERT ON usage_observations BEGIN SELECT RAISE(FAIL,'usage storage failed'); END`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f := usageFixture(t)
			if err := s.RecordTelemetryEvent(f); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB().Exec("DELETE FROM usage_observations; PRAGMA user_version=9"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB().Exec(tc.setup); err != nil {
				t.Fatal(err)
			}
			if err := migrateV10(s.DB()); err == nil {
				t.Fatal("migration hid storage failure")
			}
			var version int
			if err := s.DB().QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 9 {
				t.Fatalf("failed migration advanced version: %d %v", version, err)
			}
		})
	}
}
