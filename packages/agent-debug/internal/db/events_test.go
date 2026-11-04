package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "modernc.org/sqlite"
)

func strPtr(s string) *string {
	return &s
}

func createTestAgentStatusDB(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "agent-status.db")
	db, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
		CREATE TABLE environments (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			hostname TEXT NOT NULL,
			platform TEXT NOT NULL,
			user TEXT NOT NULL,
			dropped_frames INTEGER NOT NULL DEFAULT 0,
			last_heartbeat_at INTEGER NOT NULL
		);
		CREATE TABLE runs (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			pid INTEGER NOT NULL,
			harness_version TEXT NOT NULL,
			started_at INTEGER NOT NULL,
			ended_at INTEGER
		);
		CREATE TABLE agents (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
			harness TEXT NOT NULL,
			project_id TEXT NOT NULL,
			project_name TEXT NOT NULL,
			canonical_root TEXT NOT NULL,
			branch TEXT NOT NULL,
			cwd TEXT NOT NULL,
			current_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
			activity TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL
		);
		CREATE TABLE telemetry_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
			subagent_id TEXT,
			event_type TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			data_json TEXT NOT NULL
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}

	// Insert environments
	_, _ = db.Exec(`INSERT INTO environments (id, label, hostname, platform, user, last_heartbeat_at)
		VALUES ('env-1', 'local-dev', 'host1', 'darwin', 'user1', 1000)`)

	// Insert runs
	_, _ = db.Exec(`INSERT INTO runs (id, agent_id, pid, harness_version, started_at)
		VALUES ('run-1', 'env-1:pi:ses-1', 27680, '1.0.0', 1000)`)

	// Insert agents
	_, _ = db.Exec(`INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, current_run_id, activity, updated_at)
		VALUES ('env-1:pi:ses-1', 'ses-1', 'env-1', 'pi', 'p1', 'proj1', '/root', 'main', '/cwd', 'run-1', 'IDLE', 6000)`)
	_, _ = db.Exec(`INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, activity, updated_at)
		VALUES ('env-1:claude-code:ses-2', 'ses-2', 'env-1', 'claude-code', 'p1', 'proj1', '/root', 'main', '/cwd', 'IDLE', 2000)`)

	// Insert events for ses-1
	d1, _ := json.Marshal(types.TelemetryFrameData{Title: strPtr("Initial session")})
	_, _ = db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (1, 'env-1:pi:ses-1', NULL, 'run_started', 1000, ?)`, string(d1))

	d2, _ := json.Marshal(types.TelemetryFrameData{
		ToolName:  strPtr("bash"),
		ToolInput: map[string]interface{}{"command": "npm test"},
	})
	_, _ = db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (2, 'env-1:pi:ses-1', NULL, 'tool_call', 2000, ?)`, string(d2))

	isErr := false
	exitCode := 0
	d3, _ := json.Marshal(types.TelemetryFrameData{
		ToolName: strPtr("bash"),
		IsError:  &isErr,
		ExitCode: &exitCode,
	})
	_, _ = db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (3, 'env-1:pi:ses-1', NULL, 'tool_result', 3000, ?)`, string(d3))

	d4, _ := json.Marshal(types.TelemetryFrameData{
		ToolName: strPtr("read"),
	})
	_, _ = db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (4, 'env-1:pi:ses-1', 'sub-1', 'tool_call', 4000, ?)`, string(d4))

	d5, _ := json.Marshal(types.TelemetryFrameData{
		Usage: &types.ITokenUsage{Total: 242260, Input: 200000, Output: 42260},
	})
	_, _ = db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (5, 'env-1:pi:ses-1', NULL, 'turn_end', 5000, ?)`, string(d5))

	d6, _ := json.Marshal(types.TelemetryFrameData{})
	_, _ = db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (6, 'env-1:pi:ses-1', NULL, 'run_exited', 6000, ?)`, string(d6))

	// Insert event for ses-2
	_, _ = db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (7, 'env-1:claude-code:ses-2', NULL, 'run_started', 1500, ?)`, string(d1))

	return p
}

func TestGetTelemetryEvents(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := createTestAgentStatusDB(t, tmpDir)
	ctx := context.Background()

	t.Run("query by session_id desc default", func(t *testing.T) {
		events, err := GetTelemetryEvents(ctx, dbPath, "ses-1", EventQueryOptions{Desc: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 6 {
			t.Fatalf("expected 6 events, got %d", len(events))
		}
		if events[0].ID != 6 || events[5].ID != 1 {
			t.Errorf("expected IDs in descending order [6 ... 1], got [%d ... %d]", events[0].ID, events[5].ID)
		}
		if events[0].PID != 27680 {
			t.Errorf("expected PID 27680 from runs table, got %d", events[0].PID)
		}
		if events[0].Harness != "pi" {
			t.Errorf("expected Harness 'pi', got %q", events[0].Harness)
		}
	})

	t.Run("query with asc order", func(t *testing.T) {
		events, err := GetTelemetryEvents(ctx, dbPath, "ses-1", EventQueryOptions{Desc: false})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 6 {
			t.Fatalf("expected 6 events, got %d", len(events))
		}
		if events[0].ID != 1 || events[5].ID != 6 {
			t.Errorf("expected IDs in ascending order [1 ... 6], got [%d ... %d]", events[0].ID, events[5].ID)
		}
	})

	t.Run("query with limit", func(t *testing.T) {
		events, err := GetTelemetryEvents(ctx, dbPath, "ses-1", EventQueryOptions{Limit: 2, Desc: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
		if events[0].ID != 6 || events[1].ID != 5 {
			t.Errorf("expected IDs [6, 5], got [%d, %d]", events[0].ID, events[1].ID)
		}
	})

	t.Run("query with event_type filter", func(t *testing.T) {
		events, err := GetTelemetryEvents(ctx, dbPath, "ses-1", EventQueryOptions{
			EventType: "run_started,run_exited",
			Desc:      false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
		if events[0].EventType != "run_started" || events[1].EventType != "run_exited" {
			t.Errorf("unexpected event types: %s, %s", events[0].EventType, events[1].EventType)
		}
	})

	t.Run("query with subagent_id filter", func(t *testing.T) {
		events, err := GetTelemetryEvents(ctx, dbPath, "ses-1", EventQueryOptions{
			SubagentID: "sub-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].ID != 4 {
			t.Errorf("expected ID 4, got %d", events[0].ID)
		}
	})

	t.Run("query with include_subagents false", func(t *testing.T) {
		inc := false
		events, err := GetTelemetryEvents(ctx, dbPath, "ses-1", EventQueryOptions{
			IncludeSubagents: &inc,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 5 {
			t.Fatalf("expected 5 root events, got %d", len(events))
		}
		for _, e := range events {
			if e.SubagentID != nil && *e.SubagentID != "" {
				t.Errorf("unexpected subagent event in results: ID %d", e.ID)
			}
		}
	})

	t.Run("query by event_id", func(t *testing.T) {
		events, err := GetTelemetryEvents(ctx, dbPath, "", EventQueryOptions{
			EventID: 3,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].ID != 3 {
			t.Errorf("expected event ID 3, got %d", events[0].ID)
		}
		if events[0].Data.ToolName == nil || *events[0].Data.ToolName != "bash" {
			t.Errorf("expected toolName 'bash', got %v", events[0].Data.ToolName)
		}
	})

	t.Run("query orphaned event without agent row", func(t *testing.T) {
		db, err := sql.Open("sqlite", "file:"+dbPath)
		if err != nil {
			t.Fatalf("opening sqlite: %v", err)
		}
		defer func() { _ = db.Close() }()

		_, _ = db.Exec(`INSERT INTO agents (id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, activity, updated_at)
			VALUES ('env-x:custom-harness:orphan-ses-99', '', 'env-1', '', 'p1', 'proj1', '/root', 'main', '/cwd', 'IDLE', 7000)`)

		if _, err := db.Exec(`INSERT INTO telemetry_events (id, agent_id, subagent_id, event_type, timestamp, data_json)
			VALUES (100, 'env-x:custom-harness:orphan-ses-99', NULL, 'run_started', 7000, '{"pid": 9999, "harness": "custom-harness"}')`); err != nil {
			t.Fatalf("inserting orphan event: %v", err)
		}

		events, err := GetTelemetryEvents(ctx, dbPath, "orphan-ses-99", EventQueryOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].PID != 9999 {
			t.Errorf("expected PID 9999 from data_json, got %d", events[0].PID)
		}
		if events[0].Harness != "custom-harness" {
			t.Errorf("expected Harness 'custom-harness', got %q", events[0].Harness)
		}
		if events[0].SessionID != "orphan-ses-99" {
			t.Errorf("expected SessionID 'orphan-ses-99', got %q", events[0].SessionID)
		}
	})

	t.Run("query non-existent database file", func(t *testing.T) {
		_, err := GetTelemetryEvents(ctx, filepath.Join(tmpDir, "not-here.db"), "ses-1", EventQueryOptions{})
		if err == nil {
			t.Fatalf("expected error for non-existent database file")
		}
	})
}
