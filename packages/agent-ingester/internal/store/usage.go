package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/usage"
)

const usageSchema = `CREATE TABLE IF NOT EXISTS usage_observations (
	agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
	observation_id TEXT NOT NULL,
	subagent_id TEXT NOT NULL,
	kind TEXT NOT NULL CHECK(kind IN ('skill','command','mcp')),
	name TEXT NOT NULL,
	server TEXT NOT NULL,
	operation TEXT NOT NULL CHECK(operation IN ('invoke','load','request')),
	observed_at INTEGER NOT NULL,
	source TEXT NOT NULL,
	event_type TEXT NOT NULL,
	tool_call_id TEXT NOT NULL,
	PRIMARY KEY(agent_id,observation_id)
);`

func recordUsage(tx *sql.Tx, frame types.ITelemetryFrame) error {
	for _, o := range usage.Extract(frame) {
		_, err := tx.Exec(`INSERT INTO usage_observations
			(agent_id,observation_id,subagent_id,kind,name,server,operation,observed_at,source,event_type,tool_call_id)
			VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(agent_id,observation_id) DO NOTHING`,
			frame.AgentID, o.ID, o.Evidence.SubagentID, o.Kind, o.Name, o.Server, o.Operation, o.Evidence.Timestamp, o.Evidence.Source, o.Evidence.EventType, o.Evidence.ToolCallID)
		if err != nil {
			return fmt.Errorf("recording usage observation: %w", err)
		}
	}
	return nil
}

func migrateV10(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration v10: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(usageSchema); err != nil {
		return fmt.Errorf("creating usage observations: %w", err)
	}
	if err := backfillUsage(tx); err != nil {
		return fmt.Errorf("backfilling usage observations: %w", err)
	}
	if _, err := tx.Exec("PRAGMA user_version=10"); err != nil {
		return fmt.Errorf("setting migration v10 version: %w", err)
	}
	return tx.Commit()
}

func backfillUsage(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT te.agent_id,te.subagent_id,a.harness,te.event_type,te.timestamp,te.data_json
		FROM telemetry_events te JOIN agents a ON a.id=te.agent_id
		WHERE te.event_type IN ('tool_call','tool_use','turn_start','user_prompt') ORDER BY te.id`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var f types.ITelemetryFrame
		var sub sql.NullString
		var data string
		if err := rows.Scan(&f.AgentID, &sub, &f.Harness, &f.EventType, &f.Timestamp, &data); err != nil {
			return err
		}
		if sub.Valid {
			f.SubagentID = &sub.String
		}
		if err := json.Unmarshal([]byte(data), &f.Data); err != nil {
			return err
		}
		if err := recordUsage(tx, f); err != nil {
			return err
		}
	}
	return rows.Err()
}
