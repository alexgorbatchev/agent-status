package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

var ErrNotFound = errors.New("not found")

const (
	EnvironmentStaleThresholdMs int64 = 30000
	SessionSilenceThresholdMs   int64 = 120000
	ToolStallThresholdMs        int64 = 180000
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func OpenReadOnly(dbPath string) (*Store, error) {
	dsn := "file:" + dbPath + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening read-only sqlite db %q: %w", dbPath, err)
	}
	return New(db), nil
}

func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}

type AgentFilter struct {
	Project     string
	Environment string
	Lifecycle   types.LifecycleState
	Activity    types.ActivityState
	Hidden      *bool
}

func (s *Store) GetEnvironments(ctx context.Context) ([]types.IEnvironmentInfo, error) {
	return s.GetEnvironmentsAt(ctx, time.Now().UnixMilli())
}

func (s *Store) GetEnvironmentsAt(ctx context.Context, nowMs int64) ([]types.IEnvironmentInfo, error) {
	query := `
		SELECT id, label, hostname, platform, user, last_heartbeat_at
		FROM environments
		ORDER BY label ASC, id ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying environments: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	envs := make([]types.IEnvironmentInfo, 0)
	for rows.Next() {
		var env types.IEnvironmentInfo
		var lastHeartbeat int64
		if err := rows.Scan(&env.ID, &env.Label, &env.Hostname, &env.Platform, &env.User, &lastHeartbeat); err != nil {
			return nil, fmt.Errorf("scanning environment row: %w", err)
		}
		env.LastHeartbeatAt = lastHeartbeat
		// Stale if last heartbeat was non-positive or more than 30 seconds (30,000 ms) ago
		env.IsStale = lastHeartbeat <= 0 || (nowMs-lastHeartbeat) > EnvironmentStaleThresholdMs
		envs = append(envs, env)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating environment rows: %w", err)
	}
	return envs, nil
}

var statusQuery = fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN a.current_run_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS active_count,
			COALESCE(SUM(CASE 
				WHEN a.current_run_id IS NOT NULL AND e.last_heartbeat_at > 0 AND (? - e.last_heartbeat_at) <= %d AND (
					a.activity IN ('WAITING_QUESTION', 'ERROR') OR
					(a.activity = 'BUSY' AND a.active_tool IS NOT NULL AND a.active_tool != '' AND a.active_tool_started_at > 0 AND (? - a.active_tool_started_at) >= %d) OR
					(a.activity = 'BUSY' AND a.updated_at > 0 AND (? - a.updated_at) >= %d) OR
					(a.activity = 'RATE_LIMITED' AND a.updated_at > 0 AND (? - a.updated_at) >= %d AND (
						(a.rate_limit_reset_at IS NOT NULL AND a.rate_limit_reset_at <= ?) OR
						(a.rate_limit_reset_at IS NULL AND (a.retry_after_seconds IS NULL OR (? - a.updated_at) >= (a.retry_after_seconds * 1000)))
					))
				) THEN 1 ELSE 0 END), 0) AS attention_count,
			SUM(a.cost_usd) AS total_spend
		FROM agents a
		LEFT JOIN environments e ON a.environment_id = e.id
	`, EnvironmentStaleThresholdMs, ToolStallThresholdMs, SessionSilenceThresholdMs, SessionSilenceThresholdMs)

func (s *Store) GetStatus(ctx context.Context) (*types.StatusResponse, error) {
	return s.GetStatusAt(ctx, time.Now().UnixMilli())
}

func (s *Store) GetStatusAt(ctx context.Context, nowMs int64) (*types.StatusResponse, error) {
	envs, err := s.GetEnvironmentsAt(ctx, nowMs)
	if err != nil {
		return nil, fmt.Errorf("getting environments for status: %w", err)
	}

	var activeAgentCount int
	var requiresAttentionCount int
	var totalSpend sql.NullFloat64

	err = s.db.QueryRowContext(ctx, statusQuery, nowMs, nowMs, nowMs, nowMs, nowMs, nowMs).Scan(&activeAgentCount, &requiresAttentionCount, &totalSpend)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("querying status aggregates: %w", err)
	}

	resp := &types.StatusResponse{
		Status:                 "ok",
		Environments:           envs,
		ActiveAgentCount:       activeAgentCount,
		RequiresAttentionCount: requiresAttentionCount,
		TotalFleetSpend:        0.0,
	}
	if totalSpend.Valid {
		resp.TotalFleetSpend = totalSpend.Float64
	}

	return resp, nil
}

const selectAgentBaseQuery = `
	SELECT
		a.id, a.session_id, a.environment_id, a.harness,
		a.project_id, a.project_name, a.canonical_root,
		a.branch, a.is_detached, a.is_worktree, a.worktree_rel_path, a.cwd,
		a.current_run_id, a.activity, a.title, a.action, a.action_stale,
		a.phase, a.active_tool, a.active_target, a.active_tool_started_at, a.question,
		a.last_thinking_excerpt, a.last_error,
		a.tokens_input, a.tokens_output, a.tokens_cache_read, a.tokens_cache_write,
		a.tokens_reasoning, a.tokens_total,
		a.cost_usd, a.subagent_count, a.active_subagents, a.first_started_at, a.updated_at,
		e.label, e.hostname, e.platform, e.user, e.last_heartbeat_at,
		r.pid, r.harness_version, r.started_at, r.ended_at,
		a.is_hidden, a.hidden_at,
		a.model, a.context_window, a.context_fullness,
		a.rate_limit_reset_at, a.retry_after_seconds,
		a.has_unseen_completion, a.completed_at
	FROM agents a
	INNER JOIN environments e ON a.environment_id = e.id
	LEFT JOIN runs r ON a.current_run_id = r.id
`

type agentRowScanner interface {
	Scan(dest ...any) error
}

func scanAgent(s agentRowScanner, nowMs int64) (*types.IAgentState, error) {
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
		activeToolStartedAt                                          sql.NullInt64
		lastThinkingExcerpt, lastError                               sql.NullString
		tokensInput, tokensOutput, tokensCacheRead, tokensCacheWrite int64
		tokensReasoning, tokensTotal                                 int64
		costUSD                                                      sql.NullFloat64
		subagentCount                                                int
		activeSubagents                                              sql.NullString
		firstStartedAt, updatedAt                                    int64
		envLabel, envHostname, envPlatform, envUser                  string
		envLastHeartbeat                                             int64
		runPID                                                       sql.NullInt64
		runHarnessVersion                                            sql.NullString
		runStartedAt, runEndedAt                                     sql.NullInt64
		isHidden                                                     int
		hiddenAt                                                     sql.NullInt64
		modelStr                                                     sql.NullString
		contextWindow                                                sql.NullInt64
		contextFullness                                              sql.NullFloat64
		rateLimitResetAt, retryAfterSeconds                          sql.NullInt64
		hasUnseenCompletion                                          int
		completedAt                                                  sql.NullInt64
	)

	err := s.Scan(
		&id, &sessionID, &envID, &harness,
		&projectID, &projectName, &canonicalRoot,
		&branch, &isDetached, &isWorktree, &worktreeRelPath, &cwd,
		&currentRunID, &activity, &title, &action, &actionStale,
		&phase, &activeTool, &activeTarget, &activeToolStartedAt, &question,
		&lastThinkingExcerpt, &lastError,
		&tokensInput, &tokensOutput, &tokensCacheRead, &tokensCacheWrite,
		&tokensReasoning, &tokensTotal,
		&costUSD, &subagentCount, &activeSubagents, &firstStartedAt, &updatedAt,
		&envLabel, &envHostname, &envPlatform, &envUser, &envLastHeartbeat,
		&runPID, &runHarnessVersion, &runStartedAt, &runEndedAt,
		&isHidden, &hiddenAt,
		&modelStr, &contextWindow, &contextFullness,
		&rateLimitResetAt, &retryAfterSeconds,
		&hasUnseenCompletion, &completedAt,
	)
	if err != nil {
		return nil, err
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
			Currency:    "USD",
			Total:       costUSD.Float64,
			IsEstimated: false,
		}
	}

	var relPathPtr *string
	if worktreeRelPath.Valid {
		relPathPtr = &worktreeRelPath.String
	}

	var phasePtr *types.AgentPhase
	if phase.Valid && phase.String != "" {
		p := types.AgentPhase(phase.String)
		phasePtr = &p
	}

	var activeToolPtr *string
	if activeTool.Valid && activeTool.String != "" {
		activeToolPtr = &activeTool.String
	}

	var activeTargetPtr *string
	if activeTarget.Valid && activeTarget.String != "" {
		activeTargetPtr = &activeTarget.String
	}

	var activeToolStartedAtPtr *int64
	if activeToolStartedAt.Valid {
		v := activeToolStartedAt.Int64
		activeToolStartedAtPtr = &v
	}

	var questionPtr *string
	if question.Valid && question.String != "" {
		questionPtr = &question.String
	}

	var lastThinkingExcerptPtr *string
	if lastThinkingExcerpt.Valid && lastThinkingExcerpt.String != "" {
		lastThinkingExcerptPtr = &lastThinkingExcerpt.String
	}

	var lastErrorPtr *string
	if lastError.Valid && lastError.String != "" {
		lastErrorPtr = &lastError.String
	}

	var hiddenAtPtr *int64
	if hiddenAt.Valid {
		v := hiddenAt.Int64
		hiddenAtPtr = &v
	}

	agent := &types.IAgentState{
		ID:        id,
		SessionID: sessionID,
		Harness:   types.HarnessType(harness),
		IsHidden:  isHidden == 1,
		HiddenAt:  hiddenAtPtr,
		Environment: types.IEnvironmentInfo{
			ID:              envID,
			Label:           envLabel,
			Hostname:        envHostname,
			Platform:        envPlatform,
			User:            envUser,
			IsStale:         envLastHeartbeat <= 0 || (nowMs-envLastHeartbeat) > EnvironmentStaleThresholdMs,
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
			RelPath:    relPathPtr,
			CWD:        cwd,
		},
		Activity: types.IAgentActivity{
			Lifecycle:           lifecycle,
			Activity:            types.ActivityState(activity),
			Title:               title,
			Action:              action,
			ActionStale:         actionStale == 1,
			Phase:               phasePtr,
			ActiveTool:          activeToolPtr,
			ActiveTarget:        activeTargetPtr,
			ActiveToolStartedAt: activeToolStartedAtPtr,
			Question:            questionPtr,
			LastThinkingExcerpt: lastThinkingExcerptPtr,
			LastError:           lastErrorPtr,
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

	if modelStr.Valid && modelStr.String != "" {
		agent.Activity.Model = &modelStr.String
	}
	if contextWindow.Valid {
		v := contextWindow.Int64
		agent.Activity.ContextWindow = &v
	}
	if contextFullness.Valid {
		v := contextFullness.Float64
		agent.Activity.ContextFullness = &v
	}
	if rateLimitResetAt.Valid {
		v := rateLimitResetAt.Int64
		agent.Activity.RateLimitResetAt = &v
	}
	if retryAfterSeconds.Valid {
		v := retryAfterSeconds.Int64
		agent.Activity.RetryAfterSeconds = &v
	}
	if hasUnseenCompletion == 1 {
		b := true
		agent.Activity.HasUnseenCompletion = &b
	}
	if completedAt.Valid {
		v := completedAt.Int64
		agent.Activity.CompletedAt = &v
	}

	if activeSubagents.Valid && strings.TrimSpace(activeSubagents.String) != "" {
		_ = json.Unmarshal([]byte(activeSubagents.String), &agent.Activity.ActiveSubagents)
	}

	return agent, nil
}

func (s *Store) GetAgents(ctx context.Context, filter AgentFilter) ([]types.IAgentState, error) {
	var whereClauses []string
	var args []any

	if filter.Project != "" {
		whereClauses = append(whereClauses, "a.project_id = ?")
		args = append(args, filter.Project)
	}
	if filter.Environment != "" {
		whereClauses = append(whereClauses, "a.environment_id = ?")
		args = append(args, filter.Environment)
	}
	if filter.Activity != "" {
		whereClauses = append(whereClauses, "a.activity = ?")
		args = append(args, string(filter.Activity))
	}
	switch filter.Lifecycle {
	case types.LifecycleRunning:
		whereClauses = append(whereClauses, "a.current_run_id IS NOT NULL")
	case types.LifecycleExited:
		whereClauses = append(whereClauses, "a.current_run_id IS NULL")
	}
	if filter.Hidden != nil {
		if *filter.Hidden {
			whereClauses = append(whereClauses, "a.is_hidden = 1")
		} else {
			whereClauses = append(whereClauses, "a.is_hidden = 0")
		}
	}

	query := selectAgentBaseQuery
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	query += " ORDER BY a.updated_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	nowMs := time.Now().UnixMilli()
	agents := make([]types.IAgentState, 0)
	for rows.Next() {
		agent, err := scanAgent(rows, nowMs)
		if err != nil {
			return nil, fmt.Errorf("scanning agent row: %w", err)
		}
		agents = append(agents, *agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating agent rows: %w", err)
	}

	return agents, nil
}

func (s *Store) GetAgentByID(ctx context.Context, id string) (*types.IAgentState, error) {
	query := selectAgentBaseQuery + " WHERE a.id = ? LIMIT 1"
	row := s.db.QueryRowContext(ctx, query, id)

	nowMs := time.Now().UnixMilli()
	agent, err := scanAgent(row, nowMs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning agent by id %q: %w", id, err)
	}

	return agent, nil
}

func (s *Store) GetProjects(ctx context.Context) ([]types.ProjectGroup, error) {
	agents, err := s.GetAgents(ctx, AgentFilter{})
	if err != nil {
		return nil, fmt.Errorf("getting agents for projects: %w", err)
	}

	groupsMap := make(map[string]*types.ProjectGroup)
	groupOrder := make([]string, 0)

	for _, agent := range agents {
		projID := agent.Project.ID
		group, exists := groupsMap[projID]
		if !exists {
			group = &types.ProjectGroup{
				Project:          agent.Project,
				Agents:           make([]types.IAgentState, 0),
				TotalCost:        0.0,
				ActiveAgentCount: 0,
			}
			groupsMap[projID] = group
			groupOrder = append(groupOrder, projID)
		}
		group.Agents = append(group.Agents, agent)
		if agent.Activity.Cost != nil {
			group.TotalCost += agent.Activity.Cost.Total
		}
		if agent.Activity.Lifecycle == types.LifecycleRunning {
			group.ActiveAgentCount++
		}
	}

	result := make([]types.ProjectGroup, 0, len(groupOrder))
	for _, id := range groupOrder {
		result = append(result, *groupsMap[id])
	}
	return result, nil
}

func (s *Store) GetAgentLogs(ctx context.Context, agentID string, limit int, beforeID int64, subagentID string) ([]types.ITelemetryFrame, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var whereClauses []string
	var args []any

	whereClauses = append(whereClauses, "te.agent_id = ?")
	args = append(args, agentID)

	if subagentID != "" {
		whereClauses = append(whereClauses, "te.subagent_id = ?")
		args = append(args, subagentID)
	}
	if beforeID > 0 {
		whereClauses = append(whereClauses, "te.id < ?")
		args = append(args, beforeID)
	}

	query := `
		SELECT
			te.id, te.agent_id, te.subagent_id, te.event_type, te.timestamp, te.data_json,
			a.session_id, a.harness,
			a.project_id, a.project_name, a.canonical_root,
			a.branch, a.is_detached, a.is_worktree, a.worktree_rel_path, a.cwd,
			e.id, e.label, e.hostname, e.platform, e.user, e.last_heartbeat_at,
			r.pid, r.harness_version
		FROM telemetry_events te
		INNER JOIN agents a ON te.agent_id = a.id
		INNER JOIN environments e ON a.environment_id = e.id
		LEFT JOIN runs r ON a.current_run_id = r.id
		WHERE ` + strings.Join(whereClauses, " AND ") + `
		ORDER BY te.id DESC
		LIMIT ?
	`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying telemetry events for agent %q: %w", agentID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	nowMs := time.Now().UnixMilli()
	frames := make([]types.ITelemetryFrame, 0)
	for rows.Next() {
		var (
			seqID, ts                                 int64
			agID, eventType, dataJSON                 string
			subID                                     sql.NullString
			sessionID, harness                        string
			projectID, projectName, canonicalRoot     string
			branch                                    string
			isDetached, isWorktree                    int
			worktreeRelPath                           sql.NullString
			cwd                                       string
			envID, envLabel, envHostname, envPlatform string
			envUser                                   string
			envLastHeartbeat                          int64
			runPID                                    sql.NullInt64
			runHarnessVersion                         sql.NullString
		)

		err := rows.Scan(
			&seqID, &agID, &subID, &eventType, &ts, &dataJSON,
			&sessionID, &harness,
			&projectID, &projectName, &canonicalRoot,
			&branch, &isDetached, &isWorktree, &worktreeRelPath, &cwd,
			&envID, &envLabel, &envHostname, &envPlatform, &envUser, &envLastHeartbeat,
			&runPID, &runHarnessVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning telemetry event row: %w", err)
		}

		var data types.TelemetryFrameData
		if dataJSON != "" {
			if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
				// Fallback if dataJSON is invalid
				data = types.TelemetryFrameData{}
			}
		}

		var subagentIDPtr *string
		if subID.Valid && subID.String != "" {
			subagentIDPtr = &subID.String
		}

		var relPathPtr *string
		if worktreeRelPath.Valid {
			relPathPtr = &worktreeRelPath.String
		}

		frame := types.ITelemetryFrame{
			Version: "1.0",
			Seq:     seqID,
			EnvID:   envID,
			Environment: types.IEnvironmentInfo{
				ID:              envID,
				Label:           envLabel,
				Hostname:        envHostname,
				Platform:        envPlatform,
				User:            envUser,
				IsStale:         envLastHeartbeat <= 0 || (nowMs-envLastHeartbeat) > EnvironmentStaleThresholdMs,
				LastHeartbeatAt: envLastHeartbeat,
			},
			AgentID:        agID,
			SessionID:      sessionID,
			SubagentID:     subagentIDPtr,
			PID:            int(runPID.Int64),
			Harness:        types.HarnessType(harness),
			HarnessVersion: runHarnessVersion.String,
			Project: types.IProjectInfo{
				ID:            projectID,
				Name:          projectName,
				CanonicalRoot: canonicalRoot,
			},
			Workspace: types.IWorkspaceInfo{
				IsWorktree: isWorktree == 1,
				Branch:     branch,
				IsDetached: isDetached == 1,
				RelPath:    relPathPtr,
				CWD:        cwd,
			},
			Timestamp: ts,
			EventType: eventType,
			Data:      data,
		}

		frames = append(frames, frame)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating telemetry event rows: %w", err)
	}

	return frames, nil
}
