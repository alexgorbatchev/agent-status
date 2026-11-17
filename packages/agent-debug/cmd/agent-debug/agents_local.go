package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-watcher/scanner"
)

func discoverLocalAgents(ctx context.Context) ([]types.IAgentState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("resolving local hostname: %w", err)
	}
	provider := scanner.NewTickProcessSnapshotProvider(scanner.LiveProcessSnapshotProvider{})
	if _, err := provider.GetSnapshot(ctx); err != nil {
		return nil, fmt.Errorf("discovering local processes: %w", err)
	}
	claude := scanner.NewClaudeCodeScanner(scanner.DefaultClaudeSessionDirs())
	claude.SetProcessSnapshotProvider(provider)
	pi := scanner.NewPiScanner(scanner.DefaultPiSessionsDir(), nil)
	pi.SetProcessSnapshotProvider(provider)
	codex := scanner.NewCodexScanner(scanner.DefaultCodexSessionsDir(), nil)
	codex.SetProcessSnapshotProvider(provider)
	opencode := scanner.NewOpencodeScanner(scanner.DefaultOpencodeDBPath(), nil)
	opencode.SetProcessSnapshotProvider(provider)

	var agents []types.IAgentState
	add := func(harness types.HarnessType, desc scanner.SessionDescriptor) {
		if !scanner.VerifyProcessMatch(desc.PID, desc.StartedAt) {
			return
		}
		agents = append(agents, types.IAgentState{
			SessionID:   desc.SessionID,
			Harness:     harness,
			Environment: types.IEnvironmentInfo{Hostname: host},
			Workspace:   types.IWorkspaceInfo{CWD: desc.CWD},
			Activity: types.IAgentActivity{
				Title:      desc.Name,
				CurrentRun: &types.IRunInfo{PID: desc.PID, StartedAt: desc.StartedAt},
			},
		})
	}
	claudeSessions, err := claude.Scan()
	if err != nil {
		return nil, fmt.Errorf("discovering Claude Code sessions: %w", err)
	}
	for _, s := range claudeSessions {
		add(types.HarnessClaudeCode, s)
	}
	piSessions, err := pi.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovering Pi sessions: %w", err)
	}
	for _, s := range piSessions {
		if s.IsActive {
			add(types.HarnessPi, scanner.SessionDescriptor{PID: s.PID, SessionID: s.SessionID, CWD: s.CWD, StartedAt: s.StartedAt})
		}
	}
	codexSessions, err := codex.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovering Codex sessions: %w", err)
	}
	for _, s := range codexSessions {
		if s.IsActive {
			add(types.HarnessCodex, scanner.SessionDescriptor{PID: s.PID, SessionID: s.SessionID, CWD: s.CWD, StartedAt: s.StartedAt, Name: s.AgentNickname})
		}
	}
	opencodeSessions, err := opencode.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovering OpenCode sessions: %w", err)
	}
	for _, s := range opencodeSessions {
		if s.IsActive {
			add(types.HarnessOpencode, scanner.SessionDescriptor{PID: s.PID, SessionID: s.SessionID, CWD: s.CWD, StartedAt: s.StartedAt, Name: s.Title})
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}
