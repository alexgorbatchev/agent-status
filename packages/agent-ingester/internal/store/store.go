package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	_ "github.com/ncruces/go-sqlite3/driver"
)

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating database directory %s: %w", dir, err)
		}
	}

	dsn := "file:" + dbPath + "?" +
		"_pragma=journal_mode(wal)&" +
		"_pragma=synchronous(normal)&" +
		"_pragma=busy_timeout(5000)&" +
		"_pragma=foreign_keys(on)&" +
		"_pragma=auto_vacuum(incremental)"
	if dbPath == ":memory:" {
		dsn += "&cache=shared"
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	maxConns := max(4, runtime.GOMAXPROCS(0))
	if dbPath == ":memory:" {
		maxConns = 1
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) GetLastSeq(envID string) (int64, error) {
	var lastSeq int64
	err := s.db.QueryRow("SELECT last_seq FROM observer_cursors WHERE environment_id = ?", envID).Scan(&lastSeq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("querying last_seq: %w", err)
	}
	return lastSeq, nil
}

func (s *Store) SaveCursor(envID string, seq int64) error {
	_, err := s.db.Exec(`
		INSERT INTO observer_cursors (environment_id, last_seq)
		VALUES (?, ?)
		ON CONFLICT(environment_id) DO UPDATE SET last_seq = excluded.last_seq
	`, envID, seq)
	return err
}

func (s *Store) UpsertEnvironment(env types.IEnvironmentInfo) error {
	_, err := s.db.Exec(`
		INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
		VALUES (?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT(id) DO UPDATE SET
			label = excluded.label,
			hostname = excluded.hostname,
			platform = excluded.platform,
			user = excluded.user,
			last_heartbeat_at = excluded.last_heartbeat_at
	`, env.ID, env.Label, env.Hostname, env.Platform, env.User, env.LastHeartbeatAt)
	return err
}

func (s *Store) RecordHeartbeat(envID string, ts int64) error {
	_, err := s.db.Exec("UPDATE environments SET last_heartbeat_at = ? WHERE id = ?", ts, envID)
	return err
}

func (s *Store) IncrementDroppedFrames(envID string, count int64) error {
	_, err := s.db.Exec("UPDATE environments SET dropped_frames = dropped_frames + ? WHERE id = ?", count, envID)
	return err
}

func (s *Store) RecordTelemetryEvent(frame types.ITelemetryFrame) error {
	dataBytes, err := json.Marshal(frame.Data)
	if err != nil {
		return fmt.Errorf("marshaling frame data: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO telemetry_events (agent_id, subagent_id, event_type, timestamp, data_json)
		VALUES (?, ?, ?, ?, ?)
	`, frame.AgentID, frame.SubagentID, frame.EventType, frame.Timestamp, string(dataBytes))
	return err
}

func (s *Store) SaveAgentState(agent types.IAgentState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Ensure environment exists
	_, err = tx.Exec(`
		INSERT INTO environments (id, label, hostname, platform, user, dropped_frames, last_heartbeat_at)
		VALUES (?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT(id) DO UPDATE SET
			label = excluded.label,
			hostname = excluded.hostname,
			last_heartbeat_at = excluded.last_heartbeat_at
	`, agent.Environment.ID, agent.Environment.Label, agent.Environment.Hostname, agent.Environment.Platform, agent.Environment.User, agent.Environment.LastHeartbeatAt)
	if err != nil {
		return fmt.Errorf("upserting environment in agent save: %w", err)
	}

	// Save or update run if present
	var currentRunID *string
	if agent.Activity.CurrentRun != nil {
		r := agent.Activity.CurrentRun
		currentRunID = &r.ID
		_, err = tx.Exec(`
			INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				pid = excluded.pid,
				harness_version = excluded.harness_version,
				started_at = excluded.started_at,
				ended_at = excluded.ended_at
		`, r.ID, agent.ID, r.PID, r.HarnessVersion, r.StartedAt, r.EndedAt)
		if err != nil {
			return fmt.Errorf("upserting run: %w", err)
		}
	}

	var costUSD *float64
	if agent.Activity.Cost != nil {
		costUSD = &agent.Activity.Cost.Total
	}

	var phaseStr *string
	if agent.Activity.Phase != nil {
		p := string(*agent.Activity.Phase)
		phaseStr = &p
	}

	isDetached := 0
	if agent.Workspace.IsDetached {
		isDetached = 1
	}
	isWorktree := 0
	if agent.Workspace.IsWorktree {
		isWorktree = 1
	}
	actionStale := 0
	if agent.Activity.ActionStale {
		actionStale = 1
	}
	isHidden := 0
	if agent.IsHidden {
		isHidden = 1
	}
	hiddenAt := agent.HiddenAt

	_, err = tx.Exec(`
		INSERT INTO agents (
			id, session_id, environment_id, harness,
			project_id, project_name, canonical_root,
			branch, is_detached, is_worktree, worktree_rel_path, cwd,
			current_run_id, activity, title, action, action_stale,
			phase, active_tool, active_target, question, last_thinking_excerpt, last_error,
			tokens_input, tokens_output, tokens_cache_read, tokens_cache_write, tokens_reasoning, tokens_total,
			cost_usd, subagent_count, first_started_at, updated_at,
			is_hidden, hidden_at
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?
		)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			project_name = excluded.project_name,
			canonical_root = excluded.canonical_root,
			branch = excluded.branch,
			is_detached = excluded.is_detached,
			is_worktree = excluded.is_worktree,
			worktree_rel_path = excluded.worktree_rel_path,
			cwd = excluded.cwd,
			current_run_id = excluded.current_run_id,
			activity = excluded.activity,
			title = excluded.title,
			action = excluded.action,
			action_stale = excluded.action_stale,
			phase = excluded.phase,
			active_tool = excluded.active_tool,
			active_target = excluded.active_target,
			question = excluded.question,
			last_thinking_excerpt = excluded.last_thinking_excerpt,
			last_error = excluded.last_error,
			tokens_input = excluded.tokens_input,
			tokens_output = excluded.tokens_output,
			tokens_cache_read = excluded.tokens_cache_read,
			tokens_cache_write = excluded.tokens_cache_write,
			tokens_reasoning = excluded.tokens_reasoning,
			tokens_total = excluded.tokens_total,
			cost_usd = excluded.cost_usd,
			subagent_count = excluded.subagent_count,
			updated_at = excluded.updated_at,
			is_hidden = excluded.is_hidden,
			hidden_at = excluded.hidden_at
	`,
		agent.ID, agent.SessionID, agent.Environment.ID, string(agent.Harness),
		agent.Project.ID, agent.Project.Name, agent.Project.CanonicalRoot,
		agent.Workspace.Branch, isDetached, isWorktree, agent.Workspace.RelPath, agent.Workspace.CWD,
		currentRunID, string(agent.Activity.Activity), agent.Activity.Title, agent.Activity.Action, actionStale,
		phaseStr, agent.Activity.ActiveTool, agent.Activity.ActiveTarget, agent.Activity.Question, agent.Activity.LastThinkingExcerpt, agent.Activity.LastError,
		agent.Activity.Usage.Input, agent.Activity.Usage.Output, agent.Activity.Usage.CacheRead, agent.Activity.Usage.CacheWrite, agent.Activity.Usage.Reasoning, agent.Activity.Usage.Total,
		costUSD, agent.Activity.SubagentCount, agent.Activity.FirstStartedAt, agent.Activity.UpdatedAt,
		isHidden, hiddenAt,
	)
	if err != nil {
		return fmt.Errorf("upserting agent: %w", err)
	}

	return tx.Commit()
}

func (s *Store) GetAgent(id string) (*types.IAgentState, error) {
	row := s.db.QueryRow(`
		SELECT
			a.id, a.session_id, a.harness, a.project_id, a.project_name, a.canonical_root,
			a.branch, a.is_detached, a.is_worktree, a.worktree_rel_path, a.cwd,
			a.current_run_id, a.activity, a.title, a.action, a.action_stale,
			a.phase, a.active_tool, a.active_target, a.question, a.last_thinking_excerpt, a.last_error,
			a.tokens_input, a.tokens_output, a.tokens_cache_read, a.tokens_cache_write, a.tokens_reasoning, a.tokens_total,
			a.cost_usd, a.subagent_count, a.first_started_at, a.updated_at,
			e.id, e.label, e.hostname, e.platform, e.user, e.last_heartbeat_at,
			r.id, r.pid, r.harness_version, r.started_at, r.ended_at,
			a.is_hidden, a.hidden_at
		FROM agents a
		JOIN environments e ON a.environment_id = e.id
		LEFT JOIN runs r ON a.current_run_id = r.id
		WHERE a.id = ?
	`, id)

	var (
		agent                                              types.IAgentState
		harnessStr, activityStr                            string
		isDetached, isWorktree, actionStale                int
		isHidden                                           int
		hiddenAt                                           sql.NullInt64
		phaseStr                                           sql.NullString
		activeTool, activeTarget, question, thinking, lErr sql.NullString
		worktreeRel                                        sql.NullString
		costUSD                                            sql.NullFloat64
		currentRunID                                       sql.NullString
		envPlatform                                        string
		runID                                              sql.NullString
		runPID                                             sql.NullInt64
		runHarnessVer                                      sql.NullString
		runStart, runEnd                                   sql.NullInt64
	)

	err := row.Scan(
		&agent.ID, &agent.SessionID, &harnessStr, &agent.Project.ID, &agent.Project.Name, &agent.Project.CanonicalRoot,
		&agent.Workspace.Branch, &isDetached, &isWorktree, &worktreeRel, &agent.Workspace.CWD,
		&currentRunID, &activityStr, &agent.Activity.Title, &agent.Activity.Action, &actionStale,
		&phaseStr, &activeTool, &activeTarget, &question, &thinking, &lErr,
		&agent.Activity.Usage.Input, &agent.Activity.Usage.Output, &agent.Activity.Usage.CacheRead, &agent.Activity.Usage.CacheWrite, &agent.Activity.Usage.Reasoning, &agent.Activity.Usage.Total,
		&costUSD, &agent.Activity.SubagentCount, &agent.Activity.FirstStartedAt, &agent.Activity.UpdatedAt,
		&agent.Environment.ID, &agent.Environment.Label, &agent.Environment.Hostname, &envPlatform, &agent.Environment.User, &agent.Environment.LastHeartbeatAt,
		&runID, &runPID, &runHarnessVer, &runStart, &runEnd,
		&isHidden, &hiddenAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning agent: %w", err)
	}

	agent.Harness = types.HarnessType(harnessStr)
	agent.Activity.Activity = types.ActivityState(activityStr)
	agent.Environment.Platform = envPlatform
	agent.Workspace.IsDetached = isDetached == 1
	agent.Workspace.IsWorktree = isWorktree == 1
	agent.Activity.ActionStale = actionStale == 1
	agent.IsHidden = isHidden == 1
	if hiddenAt.Valid {
		v := hiddenAt.Int64
		agent.HiddenAt = &v
	}

	if worktreeRel.Valid {
		agent.Workspace.RelPath = &worktreeRel.String
	}
	if phaseStr.Valid {
		p := types.AgentPhase(phaseStr.String)
		agent.Activity.Phase = &p
	}
	if activeTool.Valid {
		agent.Activity.ActiveTool = &activeTool.String
	}
	if activeTarget.Valid {
		agent.Activity.ActiveTarget = &activeTarget.String
	}
	if question.Valid {
		agent.Activity.Question = &question.String
	}
	if thinking.Valid {
		agent.Activity.LastThinkingExcerpt = &thinking.String
	}
	if lErr.Valid {
		agent.Activity.LastError = &lErr.String
	}
	if costUSD.Valid {
		agent.Activity.Cost = &types.ICostInfo{
			Currency:    "USD",
			Total:       costUSD.Float64,
			IsEstimated: false,
		}
	}

	// Lifecycle derivation based on current run
	if currentRunID.Valid && runID.Valid && !runEnd.Valid {
		agent.Activity.Lifecycle = types.LifecycleRunning
		agent.Activity.CurrentRun = &types.IRunInfo{
			ID:             runID.String,
			PID:            int(runPID.Int64),
			HarnessVersion: runHarnessVer.String,
			StartedAt:      runStart.Int64,
			EndedAt:        nil,
		}
	} else {
		agent.Activity.Lifecycle = types.LifecycleExited
		if runID.Valid {
			var endedAt *int64
			if runEnd.Valid {
				v := runEnd.Int64
				endedAt = &v
			}
			agent.Activity.CurrentRun = &types.IRunInfo{
				ID:             runID.String,
				PID:            int(runPID.Int64),
				HarnessVersion: runHarnessVer.String,
				StartedAt:      runStart.Int64,
				EndedAt:        endedAt,
			}
		}
	}

	// Staleness calculation (heartbeat > 30s)
	now := time.Now().UnixMilli()
	agent.Environment.IsStale = (now - agent.Environment.LastHeartbeatAt) > 30000

	return &agent, nil
}

func (s *Store) SetAgentVisibility(id string, isHidden bool, hiddenAt *int64) (*types.IAgentState, error) {
	val := 0
	if isHidden {
		val = 1
	}
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`
		UPDATE agents
		SET is_hidden = ?, hidden_at = ?, updated_at = ?
		WHERE id = ?
	`, val, hiddenAt, now, id)
	if err != nil {
		return nil, fmt.Errorf("updating agent visibility %s: %w", id, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking rows affected for %s: %w", id, err)
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("agent %s not found", id)
	}
	return s.GetAgent(id)
}

func (s *Store) GetAllActiveAgents() ([]types.IAgentState, error) {
	rows, err := s.db.Query("SELECT id FROM agents ORDER BY updated_at DESC")
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()

	var agents []types.IAgentState
	for _, id := range ids {
		a, err := s.GetAgent(id)
		if err != nil {
			return nil, err
		}
		if a != nil {
			agents = append(agents, *a)
		}
	}
	return agents, nil
}

func (s *Store) GetAllEnvironments() ([]types.IEnvironmentInfo, error) {
	rows, err := s.db.Query("SELECT id, label, hostname, platform, user, last_heartbeat_at FROM environments")
	if err != nil {
		return nil, fmt.Errorf("querying environments: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	now := time.Now().UnixMilli()
	var envs []types.IEnvironmentInfo
	for rows.Next() {
		var e types.IEnvironmentInfo
		var plat string
		if err := rows.Scan(&e.ID, &e.Label, &e.Hostname, &plat, &e.User, &e.LastHeartbeatAt); err != nil {
			return nil, err
		}
		e.Platform = plat
		e.IsStale = (now - e.LastHeartbeatAt) > 30000
		envs = append(envs, e)
	}
	return envs, nil
}

func (s *Store) EndCurrentRun(agentID string, endedAt int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var currentRunID sql.NullString
	err = tx.QueryRow("SELECT current_run_id FROM agents WHERE id = ?", agentID).Scan(&currentRunID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if currentRunID.Valid {
		_, err = tx.Exec("UPDATE runs SET ended_at = ? WHERE id = ?", endedAt, currentRunID.String)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec("UPDATE agents SET current_run_id = NULL WHERE id = ?", agentID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetDistinctSubagentIDs returns all distinct non-empty subagent IDs recorded in telemetry_events for an agent.
func (s *Store) GetDistinctSubagentIDs(agentID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT subagent_id
		FROM telemetry_events
		WHERE agent_id = ? AND subagent_id IS NOT NULL AND subagent_id != ''
		ORDER BY subagent_id ASC
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("querying distinct subagent IDs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning subagent ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subagent IDs: %w", err)
	}
	return ids, nil
}

