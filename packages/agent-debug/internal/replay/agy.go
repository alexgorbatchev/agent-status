package replay

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alexgorbatchev/agent-parser"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func loadAgySession(ctx context.Context, path, sessionID, cwd string, strict bool) (*LoadedSession, error) {
	agyParser := parser.NewAgyParser()
	items, _, err := readJSONLLines(path, agyParser.ParseLine)
	if err != nil {
		return nil, fmt.Errorf("reading agy transcript %s: %w", path, err)
	}
	logsDir := filepath.Dir(path)
	brainDir := filepath.Dir(filepath.Dir(filepath.Dir(logsDir)))
	nativeLayout := filepath.Base(logsDir) == "logs" && filepath.Base(filepath.Dir(logsDir)) == ".system_generated"
	if sessionID == "" {
		sessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if nativeLayout {
			sessionID = filepath.Base(filepath.Dir(filepath.Dir(logsDir)))
		}
	}
	var title string
	if nativeLayout {
		appDir := filepath.Dir(brainDir)
		summaries, _ := parser.ReadAgySummaries(ctx, filepath.Join(appDir, "conversation_summaries.db"))
		title = summaries[sessionID].Title
		workspaces := summaries[sessionID].WorkspacePaths
		if len(workspaces) == 0 {
			workspaces, _ = parser.ReadAgyConversationWorkspaces(ctx, filepath.Join(appDir, "conversations"), sessionID)
		}
		if cwd == "" && len(workspaces) > 0 {
			cwd = workspaces[0]
		}
	}
	if cwd != "" {
		cwd = filepath.Clean(cwd)
	}
	events := make([]ReplayEvent, 0, len(items))
	for _, item := range items {
		item.Event.CWD = cwd
		events = append(events, ReplayEvent{Event: item.Event, SourceLine: item.SourceLine})
	}
	var start, end int64
	if len(events) > 0 {
		start, end = events[0].Event.Timestamp, events[len(events)-1].Event.Timestamp
	}
	events = ensureSyntheticLifecycle(events, start, end, cwd, strict)
	if title != "" && len(events) > 0 {
		events[0].Event.Data.Title = &title
	}
	return &LoadedSession{Harness: types.HarnessAgy, SessionID: sessionID, CWD: cwd, Events: events}, nil
}
