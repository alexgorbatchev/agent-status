package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

// EventRecord captures an ingested telemetry frame persisted in telemetry_events.
type EventRecord struct {
	ID         int64                    `json:"id"`
	AgentID    string                   `json:"agentId"`
	SessionID  string                   `json:"sessionId"`
	Harness    string                   `json:"harness,omitempty"`
	SubagentID *string                  `json:"subagentId"`
	EventType  string                   `json:"eventType"`
	Timestamp  int64                    `json:"timestamp"`
	DataJSON   string                   `json:"dataJson"`
	Data       types.TelemetryFrameData `json:"data"`
	PID        int                      `json:"pid,omitempty"`
}

// EventQueryOptions controls filtering and pagination for telemetry events.
type EventQueryOptions struct {
	Limit            int
	EventType        string // comma-separated event types, e.g. "run_started,run_exited,error"
	SubagentID       string // specific subagent ID filter
	IncludeSubagents *bool  // if false, only subagent_id IS NULL or empty
	Desc             bool   // if true, ORDER BY te.id DESC; if false, ASC
	EventID          int64  // if > 0, filter by te.id = ?
}

// ResolveDatabasePath finds agent-status.db based on explicit path, env vars, or defaults.
func ResolveDatabasePath(customPath string) (string, error) {
	if customPath != "" && customPath != "auto" {
		if _, err := os.Stat(customPath); err != nil {
			return "", fmt.Errorf("database file %s not found: %w", customPath, err)
		}
		return customPath, nil
	}

	// Candidates in order
	var candidates []string

	// Local dev-stack data
	candidates = append(candidates,
		filepath.Join(".tmp", "dev-stack", "data", "agent-status", "agent-status.db"),
		filepath.Join(".tmp", "dev-stack", "data", "agent-status.db"),
	)

	// Environment variable AIR_DATA_DIR
	if envDir := os.Getenv("AIR_DATA_DIR"); envDir != "" {
		candidates = append(candidates,
			filepath.Join(envDir, "agent-status.db"),
			filepath.Join(envDir, "agent-status", "agent-status.db"),
		)
	}

	// Environment variable XDG_DATA_HOME
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		candidates = append(candidates,
			filepath.Join(xdg, "agent-status", "agent-status.db"),
			filepath.Join(xdg, "agent-status.db"),
		)
	}

	// User home directory
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "share", "agent-status", "agent-status.db"),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		defaultPath := filepath.Join(home, ".local", "share", "agent-status", "agent-status.db")
		return "", fmt.Errorf("agent-status database not found in default paths (%s, .tmp/dev-stack/data/agent-status/agent-status.db); specify path using --db", defaultPath)
	}

	return "", fmt.Errorf("agent-status database not found; specify path using --db")
}

// GetTelemetryEvents queries telemetry_events from SQLite database in read-only mode.
func GetTelemetryEvents(ctx context.Context, dbPath string, sessionID string, opts EventQueryOptions) ([]EventRecord, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("opening sqlite database %s: %w", dbPath, err)
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connecting to sqlite database %s: %w", dbPath, err)
	}

	baseQuery := `
		SELECT te.id, te.agent_id, te.subagent_id, te.event_type, te.timestamp, te.data_json,
		       COALESCE(a.session_id, ''), COALESCE(a.harness, ''),
		       COALESCE(r.pid, 0)
		FROM telemetry_events te
		LEFT JOIN agents a ON te.agent_id = a.id
		LEFT JOIN runs r ON a.current_run_id = r.id
	`

	var where []string
	var args []interface{}

	if opts.EventID > 0 {
		where = append(where, "te.id = ?")
		args = append(args, opts.EventID)
	}

	if sessionID != "" {
		where = append(where, "(a.session_id = ? OR a.session_id LIKE ? OR a.id = ? OR a.id LIKE ? OR te.agent_id = ? OR te.agent_id LIKE ?)")
		likePattern := "%" + sessionID + "%"
		args = append(args, sessionID, likePattern, sessionID, likePattern, sessionID, likePattern)
	}

	if opts.EventType != "" {
		parts := strings.Split(opts.EventType, ",")
		var placeholders []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				placeholders = append(placeholders, "?")
				args = append(args, p)
			}
		}
		if len(placeholders) > 0 {
			where = append(where, "te.event_type IN ("+strings.Join(placeholders, ",")+")")
		}
	}

	if opts.SubagentID != "" {
		where = append(where, "te.subagent_id = ?")
		args = append(args, opts.SubagentID)
	}

	if opts.IncludeSubagents != nil && !*opts.IncludeSubagents {
		where = append(where, "(te.subagent_id IS NULL OR te.subagent_id = '')")
	}

	query := baseQuery
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	if opts.Desc {
		query += " ORDER BY te.id DESC"
	} else {
		query += " ORDER BY te.id ASC"
	}

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying telemetry events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []EventRecord
	for rows.Next() {
		var (
			id, ts                    int64
			agentID, eventType        string
			subID                     sql.NullString
			dataJSON                  string
			sessID, harness           string
			pid                       sql.NullInt64
		)

		if err := rows.Scan(&id, &agentID, &subID, &eventType, &ts, &dataJSON, &sessID, &harness, &pid); err != nil {
			return nil, fmt.Errorf("scanning telemetry event row: %w", err)
		}

		var data types.TelemetryFrameData
		if dataJSON != "" {
			_ = json.Unmarshal([]byte(dataJSON), &data)
		}

		var rawMap map[string]interface{}
		if dataJSON != "" {
			_ = json.Unmarshal([]byte(dataJSON), &rawMap)
		}

		finalPID := int(pid.Int64)
		if finalPID == 0 && rawMap != nil {
			if p, ok := rawMap["pid"].(float64); ok && p > 0 {
				finalPID = int(p)
			}
		}

		finalHarness := harness
		if finalHarness == "" && rawMap != nil {
			if h, ok := rawMap["harness"].(string); ok {
				finalHarness = h
			}
		}

		if sessID == "" {
			parts := strings.Split(agentID, ":")
			if len(parts) >= 3 {
				sessID = parts[2]
				if finalHarness == "" {
					finalHarness = parts[1]
				}
			}
		}

		var subagentPtr *string
		if subID.Valid && subID.String != "" {
			subagentPtr = &subID.String
		}

		results = append(results, EventRecord{
			ID:         id,
			AgentID:    agentID,
			SessionID:  sessID,
			Harness:    finalHarness,
			SubagentID: subagentPtr,
			EventType:  eventType,
			Timestamp:  ts,
			DataJSON:   dataJSON,
			Data:       data,
			PID:        finalPID,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating telemetry event rows: %w", err)
	}

	return results, nil
}
