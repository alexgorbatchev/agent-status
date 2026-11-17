package store

import (
	"context"
	"fmt"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/usage"
)

func (s *Store) GetAgentUsage(ctx context.Context, agentID string) (*usage.Report, error) {
	a, err := s.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	r := usage.NewReport(a.ID, a.Harness)
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
		SELECT *, ROW_NUMBER() OVER w AS rank,
		COUNT(*) OVER p AS count, MIN(observed_at) OVER p AS first_used_at, MAX(observed_at) OVER p AS last_used_at
		FROM usage_observations WHERE agent_id=?
		WINDOW p AS (PARTITION BY kind,server,name,operation),
		w AS (p ORDER BY observed_at DESC,observation_id DESC)
	) SELECT kind,name,server,operation,count,first_used_at,last_used_at,
		source,event_type,observed_at,subagent_id,tool_call_id FROM ranked WHERE rank=1
		ORDER BY kind,server,name,operation`, agentID)
	if err != nil {
		return nil, fmt.Errorf("querying agent usage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind string
		var e usage.Entry
		if err := rows.Scan(&kind, &e.Name, &e.Server, &e.Operation, &e.Count, &e.FirstUsedAt, &e.LastUsedAt,
			&e.LatestEvidence.Source, &e.LatestEvidence.EventType, &e.LatestEvidence.Timestamp, &e.LatestEvidence.SubagentID, &e.LatestEvidence.ToolCallID); err != nil {
			return nil, fmt.Errorf("scanning agent usage: %w", err)
		}
		switch kind {
		case "skill":
			r.Skills = append(r.Skills, e)
		case "command":
			r.Commands = append(r.Commands, e)
		case "mcp":
			r.MCP = append(r.MCP, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading agent usage: %w", err)
	}
	return r, nil
}
