package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

var ErrSessionNotFound = errors.New("session not found in database")

type RunRecord struct {
	ID             string `json:"id"`
	AgentID        string `json:"agentId"`
	PID            int    `json:"pid"`
	HarnessVersion string `json:"harnessVersion"`
	StartedAt      int64  `json:"startedAt"`
	EndedAt        *int64 `json:"endedAt,omitempty"`
}

type SessionRecord struct {
	DBPath              string             `json:"dbPath"`
	Agent               *types.IAgentState `json:"agent"`
	Runs                []RunRecord        `json:"runs"`
	EventCount          int                `json:"eventCount"`
	FirstEventTimestamp *int64             `json:"firstEventTimestamp,omitempty"`
	LastEventTimestamp  *int64             `json:"lastEventTimestamp,omitempty"`
}

// FindDefaultDBPath searches standard locations for agent-status.db.
func FindDefaultDBPath() string {
	if p := os.Getenv("AIR_DB_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		return ""
	}

	xdgData := os.Getenv("XDG_DATA_HOME")
	if xdgData == "" {
		if home, err := os.UserHomeDir(); err == nil {
			xdgData = filepath.Join(home, ".local", "share")
		}
	}
	if xdgData != "" {
		p := filepath.Join(xdgData, "agent-status", "agent-status.db")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	devStackPath := filepath.Join(".tmp", "dev-stack", "data", "agent-status", "agent-status.db")
	if _, err := os.Stat(devStackPath); err == nil {
		return devStackPath
	}

	if home, err := os.UserHomeDir(); err == nil {
		statePath := filepath.Join(home, ".local", "state", "agent-status", "agent-status.db")
		if _, err := os.Stat(statePath); err == nil {
			return statePath
		}
	}

	return ""
}

// ResolveDBPath resolves a user-supplied flag or auto-locates agent-status.db.
func ResolveDBPath(flagVal string) (string, error) {
	if flagVal == "" || flagVal == "auto" {
		found := FindDefaultDBPath()
		if found == "" {
			return "", fmt.Errorf("agent-status SQLite database not found; specify --db <path>")
		}
		return found, nil
	}

	path := flagVal
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("sqlite database not found at %q: %w", path, err)
	}
	return path, nil
}

const selectAgentQuery = `
	SELECT
		a.id, a.session_id, a.environment_id, a.harness,
		a.project_id, a.project_name, a.canonical_root,
		a.branch, a.is_detached, a.is_worktree, a.worktree_rel_path, a.cwd,
		a.current_run_id, a.activity, a.title, a.action, a.action_stale,
		a.phase, a.active_tool, a.active_target, a.question,
		a.last_thinking_excerpt, a.last_error,
		a.tokens_input, a.tokens_output, a.tokens_cache_read, a.tokens_cache_write,
		a.tokens_reasoning, a.tokens_total,
		a.cost_usd, a.subagent_count, a.first_started_at, a.updated_at,
		COALESCE(e.label, ''), COALESCE(e.hostname, ''), COALESCE(e.platform, ''), COALESCE(e.user, ''), COALESCE(e.last_heartbeat_at, 0),
		r.pid, r.harness_version, r.started_at, r.ended_at
	FROM agents a
	LEFT JOIN environments e ON a.environment_id = e.id
	LEFT JOIN runs r ON a.current_run_id = r.id
	WHERE a.session_id = ? OR a.id = ?
	ORDER BY a.updated_at DESC
	LIMIT 1
`

// InspectSession opens agent-status.db read-only and loads agent, runs, and event metrics.
func InspectSession(ctx context.Context, dbPath string, sessionIDOrAgentID string) (*SessionRecord, error) {
	dsn := "file:" + dbPath + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening read-only sqlite database %q: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	var (
		id, sessionID, envID, harness                                string
		projectID, projectName, canonicalRoot                        string
		branch                                                       string
		isDetached, isWorktree                                       int
		worktreeRelPath                                              sql.NullString
		cwd                                                          string
		currentRunID                                                 sql.NullString
		activity                                                     string
		title, action                                                string
		actionStale                                                  int
		phase, activeTool, activeTarget, question                    sql.NullString
		lastThinkingExcerpt, lastError                               sql.NullString
		tokensInput, tokensOutput, tokensCacheRead, tokensCacheWrite int64
		tokensReasoning, tokensTotal                                 int64
		costUSD                                                      sql.NullFloat64
		subagentCount                                                int
		firstStartedAt, updatedAt                                    int64
		envLabel, envHostname, envPlatform, envUser                  string
		envLastHeartbeat                                             int64
		runPID                                                       sql.NullInt64
		runHarnessVersion                                            sql.NullString
		runStartedAt, runEndedAt                                     sql.NullInt64
	)

	row := db.QueryRowContext(ctx, selectAgentQuery, sessionIDOrAgentID, sessionIDOrAgentID)
	err = row.Scan(
		&id, &sessionID, &envID, &harness,
		&projectID, &projectName, &canonicalRoot,
		&branch, &isDetached, &isWorktree, &worktreeRelPath, &cwd,
		&currentRunID, &activity, &title, &action, &actionStale,
		&phase, &activeTool, &activeTarget, &question,
		&lastThinkingExcerpt, &lastError,
		&tokensInput, &tokensOutput, &tokensCacheRead, &tokensCacheWrite,
		&tokensReasoning, &tokensTotal,
		&costUSD, &subagentCount, &firstStartedAt, &updatedAt,
		&envLabel, &envHostname, &envPlatform, &envUser, &envLastHeartbeat,
		&runPID, &runHarnessVersion, &runStartedAt, &runEndedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionIDOrAgentID)
		}
		return nil, fmt.Errorf("querying agent record: %w", err)
	}

	lifecycle := types.LifecycleExited
	var currentRun *types.IRunInfo
	if currentRunID.Valid && currentRunID.String != "" {
		lifecycle = types.LifecycleRunning
		var endedAtPtr *int64
		if runEndedAt.Valid {
			v := runEndedAt.Int64
			endedAtPtr = &v
		}
		currentRun = &types.IRunInfo{
			ID:             currentRunID.String,
			PID:            int(runPID.Int64),
			HarnessVersion: runHarnessVersion.String,
			StartedAt:      runStartedAt.Int64,
			EndedAt:        endedAtPtr,
		}
	}

	var costInfo *types.ICostInfo
	if costUSD.Valid {
		costInfo = &types.ICostInfo{
			Currency: "USD",
			Total:    costUSD.Float64,
		}
	}

	var phasePtr *types.AgentPhase
	if phase.Valid && phase.String != "" {
		p := types.AgentPhase(phase.String)
		phasePtr = &p
	}

	agent := &types.IAgentState{
		ID:        id,
		SessionID: sessionID,
		Harness:   types.HarnessType(harness),
		Environment: types.IEnvironmentInfo{
			ID:              envID,
			Label:           envLabel,
			Hostname:        envHostname,
			Platform:        envPlatform,
			User:            envUser,
			LastHeartbeatAt: envLastHeartbeat,
		},
		Project: types.IProjectInfo{
			ID:            projectID,
			Name:          projectName,
			CanonicalRoot: canonicalRoot,
		},
		Workspace: types.IWorkspaceInfo{
			IsWorktree: isWorktree == 1,
			Branch:     branch,
			IsDetached: isDetached == 1,
			CWD:        cwd,
		},
		Activity: types.IAgentActivity{
			Lifecycle:      lifecycle,
			Activity:       types.ActivityState(activity),
			Title:          title,
			Action:         action,
			ActionStale:    actionStale == 1,
			Phase:          phasePtr,
			ActiveTool:     nullStringPtr(activeTool),
			ActiveTarget:   nullStringPtr(activeTarget),
			Question:       nullStringPtr(question),
			LastError:      nullStringPtr(lastError),
			Usage: types.ITokenUsage{
				Input:      tokensInput,
				Output:     tokensOutput,
				CacheRead:  tokensCacheRead,
				CacheWrite: tokensCacheWrite,
				Reasoning:  tokensReasoning,
				Total:      tokensTotal,
			},
			Cost:           costInfo,
			SubagentCount:  subagentCount,
			CurrentRun:     currentRun,
			FirstStartedAt: firstStartedAt,
			UpdatedAt:      updatedAt,
		},
	}
	if worktreeRelPath.Valid {
		agent.Workspace.RelPath = &worktreeRelPath.String
	}
	if lastThinkingExcerpt.Valid {
		agent.Activity.LastThinkingExcerpt = &lastThinkingExcerpt.String
	}

	// Fetch all runs for this agent
	runsQuery := `
		SELECT id, agent_id, pid, harness_version, started_at, ended_at
		FROM runs
		WHERE agent_id = ?
		ORDER BY started_at ASC
	`
	runRows, err := db.QueryContext(ctx, runsQuery, agent.ID)
	if err != nil {
		return nil, fmt.Errorf("querying agent runs: %w", err)
	}
	defer func() { _ = runRows.Close() }()

	var runs []RunRecord
	for runRows.Next() {
		var r RunRecord
		var endedAt sql.NullInt64
		if err := runRows.Scan(&r.ID, &r.AgentID, &r.PID, &r.HarnessVersion, &r.StartedAt, &endedAt); err != nil {
			return nil, fmt.Errorf("scanning run row: %w", err)
		}
		if endedAt.Valid {
			v := endedAt.Int64
			r.EndedAt = &v
		}
		runs = append(runs, r)
	}
	if err := runRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating run rows: %w", err)
	}

	// Fetch telemetry event counts and timestamp bounds
	eventsQuery := `
		SELECT COUNT(*), MIN(timestamp), MAX(timestamp)
		FROM telemetry_events
		WHERE agent_id = ?
	`
	var eventCount int
	var minTS, maxTS sql.NullInt64
	err = db.QueryRowContext(ctx, eventsQuery, agent.ID).Scan(&eventCount, &minTS, &maxTS)
	if err != nil {
		return nil, fmt.Errorf("querying telemetry events: %w", err)
	}

	record := &SessionRecord{
		DBPath:     dbPath,
		Agent:      agent,
		Runs:       runs,
		EventCount: eventCount,
	}
	if minTS.Valid {
		v := minTS.Int64
		record.FirstEventTimestamp = &v
	}
	if maxTS.Valid {
		v := maxTS.Int64
		record.LastEventTimestamp = &v
	}

	return record, nil
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid && ns.String != "" {
		return &ns.String
	}
	return nil
}
