package store

import (
	"database/sql"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"testing"
)

func TestMigrateV9PreservesAgentsAndSummaryRoundTrip(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	agent := types.IAgentState{ID: "native", SessionID: "native", Harness: types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{ID: "env", Platform: "linux"},
		Activity:    types.IAgentActivity{Activity: types.ActivityIdle, Action: "Finished"}}
	if err := s.SaveAgentState(agent); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`ALTER TABLE agents DROP COLUMN summary; ALTER TABLE agents DROP COLUMN summary_updated_at; PRAGMA user_version=8;`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(s.DB()); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(s.DB()); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	stored, err := s.GetAgent("native")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Activity.Action != "Finished" || stored.Activity.Summary != nil || stored.Activity.SummaryUpdatedAt != nil {
		t.Fatalf("existing state changed: %+v", stored.Activity)
	}
	for _, text := range []string{"First recap", "Latest recap"} {
		agent.Activity.Summary = &text
		agent.Activity.SummaryUpdatedAt = new(int64(2000))
		if err := s.SaveAgentState(agent); err != nil {
			t.Fatal(err)
		}
		stored, err = s.GetAgent("native")
		if err != nil {
			t.Fatal(err)
		}
		if stored.Activity.Summary == nil || *stored.Activity.Summary != text || *stored.Activity.SummaryUpdatedAt != 2000 {
			t.Fatalf("recap did not round trip: %+v", stored.Activity)
		}
	}
}

func TestMigrateV9RollsBackOnFailure(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.Exec("CREATE TABLE agents (id TEXT, summary_updated_at INTEGER); PRAGMA user_version=8;"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err == nil {
		t.Fatal("expected migration error")
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 8 {
		t.Fatalf("failed migration advanced version to %d", version)
	}
	if _, err := db.Exec("SELECT summary FROM agents"); err == nil {
		t.Fatal("partial migration was not rolled back")
	}
}

func TestMigrateV9ClosedDatabase(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := migrateV9(db); err == nil {
		t.Fatal("expected error on closed database")
	}
}
