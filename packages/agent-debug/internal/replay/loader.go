package replay

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
)

// ReplayEvent annotates a parsed telemetry event with source lineage and subagent context.
type ReplayEvent struct {
	Event      parser.ParsedEvent
	SourceLine int
	SubagentID *string
}

// LoadedSession contains all normalized, timestamp-interleaved events for an offline replay.
type LoadedSession struct {
	Harness        types.HarnessType
	SessionID      string
	CWD            string
	HarnessVersion string
	Events         []ReplayEvent
	Subagents      []string
}

// LoadSession loads and prepares transcript events across all harnesses with full fidelity.
func LoadSession(ctx context.Context, filePath string, harness types.HarnessType, sessionID string, cwdOverride string) (*LoadedSession, error) {
	if harness == "" {
		detected, err := DetectHarness(filePath)
		if err != nil {
			return nil, fmt.Errorf("detecting harness: %w", err)
		}
		harness = detected
	}

	switch harness {
	case types.HarnessOpencode:
		return loadOpencodeSession(ctx, filePath, sessionID, cwdOverride)
	case types.HarnessClaudeCode:
		return loadClaudeCodeSession(filePath, sessionID, cwdOverride)
	case types.HarnessPi:
		return loadPiSession(filePath, sessionID, cwdOverride)
	case types.HarnessCodex:
		return loadCodexSession(filePath, sessionID, cwdOverride)
	default:
		return nil, fmt.Errorf("unsupported harness: %s", harness)
	}
}

func loadOpencodeSession(ctx context.Context, dbPath string, sessionID string, cwdOverride string) (*LoadedSession, error) {
	db, err := parser.OpenOpencodeDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening opencode database %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	sessions, err := db.GetSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting opencode sessions: %w", err)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found in opencode database %s", dbPath)
	}

	var target *parser.OpencodeSession
	if sessionID != "" {
		for i := range sessions {
			if sessions[i].ID == sessionID {
				target = &sessions[i]
				break
			}
		}
		if target == nil {
			return nil, fmt.Errorf("session ID %q not found in opencode database (found %d sessions)", sessionID, len(sessions))
		}
	} else {
		if len(sessions) > 1 {
			var ids []string
			for _, s := range sessions {
				ids = append(ids, s.ID)
			}
			return nil, fmt.Errorf("multiple sessions found (%d): %s; please specify --session-id <id>",
				len(sessions), strings.Join(ids, ", "))
		}
		target = &sessions[0]
	}

	parsedEvents, err := db.ReconstructSessionEvents(ctx, target.ID)
	if err != nil {
		return nil, fmt.Errorf("reconstructing session events for %s: %w", target.ID, err)
	}

	effectiveCWD := target.Directory
	if cwdOverride != "" {
		effectiveCWD = filepath.Clean(cwdOverride)
	}

	var replayEvents []ReplayEvent
	for i, ev := range parsedEvents {
		if cwdOverride != "" {
			ev.CWD = effectiveCWD
		} else if ev.CWD == "" {
			ev.CWD = effectiveCWD
		}
		replayEvents = append(replayEvents, ReplayEvent{
			Event:      ev,
			SourceLine: i + 1,
			SubagentID: nil,
		})
	}

	replayEvents = ensureSyntheticLifecycle(replayEvents, target.TimeCreated, target.TimeUpdated, effectiveCWD)

	return &LoadedSession{
		Harness:        types.HarnessOpencode,
		SessionID:      target.ID,
		CWD:            effectiveCWD,
		HarnessVersion: target.Version,
		Events:         replayEvents,
		Subagents:      nil,
	}, nil
}

func loadClaudeCodeSession(filePath string, sessionIDOverride string, cwdOverride string) (*LoadedSession, error) {
	sessID := sessionIDOverride
	if sessID == "" {
		base := filepath.Base(filePath)
		sessID = strings.TrimSuffix(base, ".jsonl")
	}

	// 1. Read main transcript
	mainEvents, inferredCWD, err := readJSONLLines(filePath, func(line []byte) ([]parser.ParsedEvent, error) {
		return parser.ParseClaudeCodeLine(line)
	})
	if err != nil {
		return nil, fmt.Errorf("reading claude transcript %s: %w", filePath, err)
	}

	effectiveCWD := inferredCWD
	if cwdOverride != "" {
		effectiveCWD = filepath.Clean(cwdOverride)
	}

	var allReplayEvents []ReplayEvent
	for _, me := range mainEvents {
		ev := me.Event
		if effectiveCWD != "" {
			ev.CWD = effectiveCWD
		}
		allReplayEvents = append(allReplayEvents, ReplayEvent{
			Event:      ev,
			SourceLine: me.SourceLine,
			SubagentID: nil,
		})
	}

	// 2. Discover and parse sibling subagents
	subagentsDirCandidates := []string{
		filepath.Join(filepath.Dir(filePath), sessID, "subagents"),
		filepath.Join(filepath.Dir(filePath), "subagents"),
	}

	var subagentIDs []string
	seenSubagents := make(map[string]bool)

	for _, subDir := range subagentsDirCandidates {
		entries, rErr := os.ReadDir(subDir)
		if rErr != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			subID := strings.TrimSuffix(entry.Name(), ".jsonl")
			if seenSubagents[subID] {
				continue
			}
			seenSubagents[subID] = true
			subagentIDs = append(subagentIDs, subID)

			subPath := filepath.Join(subDir, entry.Name())
			subEvents, _, sErr := readJSONLLines(subPath, func(line []byte) ([]parser.ParsedEvent, error) {
				return parser.ParseClaudeCodeLine(line)
			})
			if sErr != nil {
				continue
			}

			subIDCopy := subID
			for _, se := range subEvents {
				ev := se.Event
				if effectiveCWD != "" {
					ev.CWD = effectiveCWD
				}
				allReplayEvents = append(allReplayEvents, ReplayEvent{
					Event:      ev,
					SourceLine: se.SourceLine,
					SubagentID: &subIDCopy,
				})
			}
		}
	}

	// 3. Interleave all events in chronological order
	sort.SliceStable(allReplayEvents, func(i, j int) bool {
		return allReplayEvents[i].Event.Timestamp < allReplayEvents[j].Event.Timestamp
	})

	// 4. Ensure synthetic lifecycle
	var startTS, endTS int64
	if len(allReplayEvents) > 0 {
		startTS = allReplayEvents[0].Event.Timestamp
		endTS = allReplayEvents[len(allReplayEvents)-1].Event.Timestamp
	}
	allReplayEvents = ensureSyntheticLifecycle(allReplayEvents, startTS, endTS, effectiveCWD)

	return &LoadedSession{
		Harness:        types.HarnessClaudeCode,
		SessionID:      sessID,
		CWD:            effectiveCWD,
		HarnessVersion: "",
		Events:         allReplayEvents,
		Subagents:      subagentIDs,
	}, nil
}

func loadPiSession(filePath string, sessionIDOverride string, cwdOverride string) (*LoadedSession, error) {
	sessID := sessionIDOverride
	if sessID == "" {
		base := filepath.Base(filePath)
		sessID = strings.TrimSuffix(base, ".jsonl")
	}

	effectiveCWD := cwdOverride

	mainEvents, inferredCWD, err := readJSONLLines(filePath, func(line []byte) ([]parser.ParsedEvent, error) {
		return parser.ParsePiLine(line, effectiveCWD)
	})
	if err != nil {
		return nil, fmt.Errorf("reading pi transcript %s: %w", filePath, err)
	}

	if effectiveCWD == "" {
		effectiveCWD = inferredCWD
	}

	var allReplayEvents []ReplayEvent
	for _, me := range mainEvents {
		ev := me.Event
		if effectiveCWD != "" {
			ev.CWD = effectiveCWD
		}
		allReplayEvents = append(allReplayEvents, ReplayEvent{
			Event:      ev,
			SourceLine: me.SourceLine,
			SubagentID: nil,
		})
	}

	var startTS, endTS int64
	if len(allReplayEvents) > 0 {
		startTS = allReplayEvents[0].Event.Timestamp
		endTS = allReplayEvents[len(allReplayEvents)-1].Event.Timestamp
	}
	allReplayEvents = ensureSyntheticLifecycle(allReplayEvents, startTS, endTS, effectiveCWD)

	return &LoadedSession{
		Harness:        types.HarnessPi,
		SessionID:      sessID,
		CWD:            effectiveCWD,
		HarnessVersion: "",
		Events:         allReplayEvents,
		Subagents:      nil,
	}, nil
}

func loadCodexSession(filePath string, sessionIDOverride string, cwdOverride string) (*LoadedSession, error) {
	sessID := sessionIDOverride
	if sessID == "" {
		if rolloutInfo, err := parser.ParseCodexRolloutPath(filePath); err == nil && rolloutInfo.SessionID != "" {
			sessID = rolloutInfo.SessionID
		} else {
			base := filepath.Base(filePath)
			sessID = strings.TrimSuffix(base, ".jsonl")
		}
	}

	effectiveCWD := cwdOverride

	mainEvents, inferredCWD, err := readJSONLLines(filePath, func(line []byte) ([]parser.ParsedEvent, error) {
		return parser.ParseCodexLine(line)
	})
	if err != nil {
		return nil, fmt.Errorf("reading codex transcript %s: %w", filePath, err)
	}

	if effectiveCWD == "" {
		effectiveCWD = inferredCWD
	}

	var allReplayEvents []ReplayEvent
	for _, me := range mainEvents {
		ev := me.Event
		if effectiveCWD != "" {
			ev.CWD = effectiveCWD
		}
		allReplayEvents = append(allReplayEvents, ReplayEvent{
			Event:      ev,
			SourceLine: me.SourceLine,
			SubagentID: nil,
		})
	}

	var startTS, endTS int64
	if len(allReplayEvents) > 0 {
		startTS = allReplayEvents[0].Event.Timestamp
		endTS = allReplayEvents[len(allReplayEvents)-1].Event.Timestamp
	}
	allReplayEvents = ensureSyntheticLifecycle(allReplayEvents, startTS, endTS, effectiveCWD)

	return &LoadedSession{
		Harness:        types.HarnessCodex,
		SessionID:      sessID,
		CWD:            effectiveCWD,
		HarnessVersion: "",
		Events:         allReplayEvents,
		Subagents:      nil,
	}, nil
}

type lineParseFunc func(line []byte) ([]parser.ParsedEvent, error)

type parsedLineItem struct {
	Event      parser.ParsedEvent
	SourceLine int
}

func readJSONLLines(filePath string, parseFn lineParseFunc) ([]parsedLineItem, string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 20*1024*1024) // Support up to 20MB lines for massive transcripts

	var items []parsedLineItem
	var inferredCWD string
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}

		evs, pErr := parseFn(raw)
		if pErr != nil {
			// If JSON parse fails on an individual line, skip or report
			continue
		}

		for _, ev := range evs {
			if inferredCWD == "" && ev.CWD != "" {
				inferredCWD = ev.CWD
			}
			items = append(items, parsedLineItem{
				Event:      ev,
				SourceLine: lineNum,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", filePath, err)
	}

	return items, inferredCWD, nil
}

func ensureSyntheticLifecycle(events []ReplayEvent, fallbackStartTS, fallbackEndTS int64, cwd string) []ReplayEvent {
	if len(events) == 0 {
		return events
	}

	hasStart := false
	hasEnd := false

	for _, e := range events {
		if e.SubagentID != nil && *e.SubagentID != "" {
			continue
		}
		if e.Event.EventType == "run_started" {
			hasStart = true
		}
		if e.Event.EventType == "run_exited" {
			hasEnd = true
		}
	}

	var result []ReplayEvent

	if !hasStart {
		ts := fallbackStartTS
		if ts == 0 {
			ts = events[0].Event.Timestamp
		}
		result = append(result, ReplayEvent{
			Event: parser.ParsedEvent{
				EventType: "run_started",
				Timestamp: ts,
				CWD:       cwd,
				Data:      types.TelemetryFrameData{},
			},
			SourceLine: 0,
			SubagentID: nil,
		})
	}

	result = append(result, events...)

	if !hasEnd {
		ts := fallbackEndTS
		if ts == 0 {
			ts = events[len(events)-1].Event.Timestamp + 1
		} else if ts <= events[len(events)-1].Event.Timestamp {
			ts = events[len(events)-1].Event.Timestamp + 1
		}
		result = append(result, ReplayEvent{
			Event: parser.ParsedEvent{
				EventType: "run_exited",
				Timestamp: ts,
				CWD:       cwd,
				Data:      types.TelemetryFrameData{},
			},
			SourceLine: 0,
			SubagentID: nil,
		})
	}

	return result
}
