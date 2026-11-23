package scanner

import (
	"context"

	observerscanner "github.com/alexgorbatchev/agent-watcher/scanner"
)

func liveAgyProcs(ctx context.Context) ([]ProcessInfo, error) {
	return LiveAgyProcsWithFinder(ctx, observerscanner.LiveAgyProcessFinder)
}

// LiveAgyProcsWithFinder discovers native Antigravity processes and enriches them.
func LiveAgyProcsWithFinder(ctx context.Context, finder observerscanner.AgyProcessFinder) ([]ProcessInfo, error) {
	procs, err := finder(ctx)
	if err != nil {
		return nil, err
	}
	var result []ProcessInfo
	for _, proc := range procs {
		info := ProcessInfo{PID: proc.PID, CreatedAt: proc.CreateTime, CWD: proc.CWD}
		EnrichProcessInfo(ctx, proc.PID, &info)
		result = append(result, info)
	}
	return result, nil
}

// LiveAgySessions discovers transcripts and native workspace metadata without
// scanning processes; the auditor pairs its independently captured process list.
func LiveAgySessions(ctx context.Context) ([]SessionInfo, error) {
	return liveAgySessionsForProcs(ctx, nil)
}

func liveAgySessionsForProcs(ctx context.Context, procs []ProcessInfo) ([]SessionInfo, error) {
	scanner := observerscanner.NewAgyScanner("", func(context.Context) ([]observerscanner.AgyProcessInfo, error) {
		var native []observerscanner.AgyProcessInfo
		for _, proc := range procs {
			native = append(native, observerscanner.AgyProcessInfo{PID: proc.PID, CWD: proc.CWD, CreateTime: proc.CreatedAt})
		}
		return native, nil
	})
	sessions, err := scanner.Scan(ctx)
	if err != nil {
		return nil, err
	}
	var result []SessionInfo
	for _, session := range sessions {
		result = append(result, SessionInfo{
			Harness: "agy", SessionID: session.SessionID, CWD: session.CWD,
			FilePath: session.TranscriptPath, HeaderTime: session.StartedAt,
			HarnessVersion: session.HarnessVersion,
			PID:            session.PID,
			IsSubagent:     session.IsSubagent,
		})
	}
	return result, nil
}
