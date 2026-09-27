package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	observerscanner "github.com/alexgorbatchev/agent-status/packages/agent-observer/scanner"
	"github.com/alexgorbatchev/agent-parser"
	"github.com/shirou/gopsutil/v4/process"
)

type PairingStatus string

const (
	StatusPaired        PairingStatus = "PAIRED"
	StatusOrphanProcess PairingStatus = "ORPHAN_PROCESS"
	StatusOrphanSession PairingStatus = "ORPHAN_SESSION"
)

type ProcessInfo struct {
	PID        int     `json:"pid"`
	CreatedAt  int64   `json:"createdAt"` // unix millis
	CWD        string  `json:"cwd"`
	Cmdline    string  `json:"cmdline,omitempty"`
	MemoryMB   float64 `json:"memoryMb,omitempty"`
	CPUPercent float64 `json:"cpuPercent,omitempty"`
}

type SessionInfo struct {
	Harness        string `json:"harness"`
	SessionID      string `json:"sessionId"`
	CWD            string `json:"cwd"`
	FilePath       string `json:"filePath"`
	HeaderTime     int64  `json:"headerTime"` // unix millis
	HarnessVersion string `json:"harnessVersion,omitempty"`
	PID            int    `json:"pid,omitempty"` // if session descriptor recorded PID (e.g. Claude Code)
}

type AuditRecord struct {
	Harness       string        `json:"harness"`
	PID           int           `json:"pid"`
	CreatedAt     int64         `json:"createdAt"` // unix millis
	CWD           string        `json:"cwd"`
	Cmdline       string        `json:"cmdline,omitempty"`
	MemoryMB      float64       `json:"memoryMb,omitempty"`
	CPUPercent    float64       `json:"cpuPercent,omitempty"`
	SessionID     string        `json:"sessionId,omitempty"`
	SessionFile   string        `json:"sessionFile,omitempty"`
	HeaderTime    int64         `json:"headerTime,omitempty"` // unix millis
	DeltaMs       *int64        `json:"deltaMs,omitempty"`    // headerTime - proc.CreateTime
	Status        PairingStatus `json:"status"`
	StatusDetails string        `json:"statusDetails,omitempty"`
}

type AuditFilter struct {
	Harness string // filter by harness e.g. "pi", "claude", "codex", "opencode"
	CWD     string // filter by working directory
}

type Auditor struct {
	NowFunc func() time.Time

	PiProcsFinder    func(ctx context.Context) ([]ProcessInfo, error)
	PiSessionsFinder func(ctx context.Context) ([]SessionInfo, error)

	ClaudeProcsFinder    func(ctx context.Context) ([]ProcessInfo, error)
	ClaudeSessionsFinder func(ctx context.Context) ([]SessionInfo, error)

	CodexProcsFinder    func(ctx context.Context) ([]ProcessInfo, error)
	CodexSessionsFinder func(ctx context.Context) ([]SessionInfo, error)

	OpencodeProcsFinder    func(ctx context.Context) ([]ProcessInfo, error)
	OpencodeSessionsFinder func(ctx context.Context) ([]SessionInfo, error)
}

func NewAuditor() *Auditor {
	return &Auditor{
		NowFunc:              time.Now,
		PiProcsFinder:        livePiProcs,
		PiSessionsFinder:        LivePiSessions,
		ClaudeProcsFinder:       liveClaudeProcs,
		ClaudeSessionsFinder:    LiveClaudeSessions,
		CodexProcsFinder:        liveCodexProcs,
		CodexSessionsFinder:     LiveCodexSessions,
		OpencodeProcsFinder:     liveOpencodeProcs,
		OpencodeSessionsFinder:  LiveOpencodeSessions,
	}
}

func (a *Auditor) now() time.Time {
	if a.NowFunc != nil {
		return a.NowFunc()
	}
	return time.Now()
}

func (a *Auditor) Audit(ctx context.Context, filter AuditFilter) ([]AuditRecord, error) {
	now := a.now()
	harnessTarget := strings.ToLower(strings.TrimSpace(filter.Harness))
	filterCWD := ""
	if filter.CWD != "" {
		filterCWD = filepath.Clean(filter.CWD)
	}

	allRecords := []AuditRecord{}

	checkHarness := func(name string, aliases ...string) bool {
		if harnessTarget == "" {
			return true
		}
		if harnessTarget == name {
			return true
		}
		for _, alias := range aliases {
			if harnessTarget == alias {
				return true
			}
		}
		return false
	}

	// 1. Pi
	if checkHarness("pi") && a.PiProcsFinder != nil && a.PiSessionsFinder != nil {
		procs, _ := a.PiProcsFinder(ctx)
		sessions, _ := a.PiSessionsFinder(ctx)
		records := auditCWDMatchedHarness("pi", procs, sessions, filterCWD, now)
		allRecords = append(allRecords, records...)
	}

	// 2. Claude Code
	if checkHarness("claude-code", "claude") && a.ClaudeProcsFinder != nil && a.ClaudeSessionsFinder != nil {
		procs, _ := a.ClaudeProcsFinder(ctx)
		sessions, _ := a.ClaudeSessionsFinder(ctx)
		records := auditClaudeHarness(procs, sessions, filterCWD, now)
		allRecords = append(allRecords, records...)
	}

	// 3. Codex
	if checkHarness("codex") && a.CodexProcsFinder != nil && a.CodexSessionsFinder != nil {
		procs, _ := a.CodexProcsFinder(ctx)
		sessions, _ := a.CodexSessionsFinder(ctx)
		records := auditCWDMatchedHarness("codex", procs, sessions, filterCWD, now)
		allRecords = append(allRecords, records...)
	}

	// 4. OpenCode
	if checkHarness("opencode") && a.OpencodeProcsFinder != nil && a.OpencodeSessionsFinder != nil {
		procs, _ := a.OpencodeProcsFinder(ctx)
		sessions, _ := a.OpencodeSessionsFinder(ctx)
		records := auditCWDMatchedHarness("opencode", procs, sessions, filterCWD, now)
		allRecords = append(allRecords, records...)
	}

	// Sort records
	sort.SliceStable(allRecords, func(i, j int) bool {
		r1, r2 := allRecords[i], allRecords[j]
		prio := func(s PairingStatus) int {
			switch s {
			case StatusPaired:
				return 0
			case StatusOrphanProcess:
				return 1
			case StatusOrphanSession:
				return 2
			default:
				return 3
			}
		}
		p1, p2 := prio(r1.Status), prio(r2.Status)
		if p1 != p2 {
			return p1 < p2
		}
		return r1.CreatedAt > r2.CreatedAt
	})

	return allRecords, nil
}

func matchCWD(target, filter string) bool {
	if filter == "" {
		return true
	}
	cleanTarget := filepath.Clean(target)
	cleanFilter := filepath.Clean(filter)
	return cleanTarget == cleanFilter || strings.HasPrefix(cleanTarget, cleanFilter+string(filepath.Separator))
}

func auditCWDMatchedHarness(harness string, procs []ProcessInfo, sessions []SessionInfo, filterCWD string, now time.Time) []AuditRecord {
	var filteredProcs []ProcessInfo
	for _, p := range procs {
		if matchCWD(p.CWD, filterCWD) {
			filteredProcs = append(filteredProcs, p)
		}
	}

	var filteredSessions []SessionInfo
	for _, s := range sessions {
		if matchCWD(s.CWD, filterCWD) {
			filteredSessions = append(filteredSessions, s)
		}
	}

	sessionKey := func(s *SessionInfo) string {
		if s.SessionID != "" {
			return s.FilePath + ":" + s.SessionID
		}
		return s.FilePath
	}

	usedSessions := make(map[string]bool)
	var records []AuditRecord

	for _, proc := range filteredProcs {
		cleanProcCWD := filepath.Clean(proc.CWD)
		var bestSession *SessionInfo
		var minDiff int64 = math.MaxInt64
		var bestDelta int64

		for i := range filteredSessions {
			sess := &filteredSessions[i]
			if usedSessions[sessionKey(sess)] {
				continue
			}
			if filepath.Clean(sess.CWD) != cleanProcCWD {
				continue
			}

			diff := sess.HeaderTime - proc.CreatedAt
			if diff >= -5000 {
				absDiff := diff
				if absDiff < 0 {
					absDiff = -absDiff
				}
				if absDiff < minDiff {
					minDiff = absDiff
					bestSession = sess
					bestDelta = diff
				}
			}
		}

		if bestSession != nil {
			usedSessions[sessionKey(bestSession)] = true
			delta := bestDelta
			statusDetails := ComputeStatusDetails(bestSession.HeaderTime, now)
			records = append(records, AuditRecord{
				Harness:       harness,
				PID:           proc.PID,
				CreatedAt:     proc.CreatedAt,
				CWD:           proc.CWD,
				Cmdline:       proc.Cmdline,
				MemoryMB:      proc.MemoryMB,
				CPUPercent:    proc.CPUPercent,
				SessionID:     bestSession.SessionID,
				SessionFile:   bestSession.FilePath,
				HeaderTime:    bestSession.HeaderTime,
				DeltaMs:       &delta,
				Status:        StatusPaired,
				StatusDetails: statusDetails,
			})
		} else {
			records = append(records, AuditRecord{
				Harness:    harness,
				PID:        proc.PID,
				CreatedAt:  proc.CreatedAt,
				CWD:        proc.CWD,
				Cmdline:    proc.Cmdline,
				MemoryMB:   proc.MemoryMB,
				CPUPercent: proc.CPUPercent,
				Status:     StatusOrphanProcess,
			})
		}
	}

	for _, sess := range filteredSessions {
		if !usedSessions[sessionKey(&sess)] {
			records = append(records, AuditRecord{
				Harness:     harness,
				PID:         sess.PID,
				CreatedAt:   sess.HeaderTime,
				CWD:         sess.CWD,
				SessionID:   sess.SessionID,
				SessionFile: sess.FilePath,
				HeaderTime:  sess.HeaderTime,
				Status:      StatusOrphanSession,
			})
		}
	}

	return records
}

func auditClaudeHarness(procs []ProcessInfo, sessions []SessionInfo, filterCWD string, now time.Time) []AuditRecord {
	var filteredProcs []ProcessInfo
	procsByPID := make(map[int]ProcessInfo)
	for _, p := range procs {
		if matchCWD(p.CWD, filterCWD) {
			filteredProcs = append(filteredProcs, p)
			procsByPID[p.PID] = p
		}
	}

	var filteredSessions []SessionInfo
	for _, s := range sessions {
		if matchCWD(s.CWD, filterCWD) {
			filteredSessions = append(filteredSessions, s)
		}
	}

	sort.SliceStable(filteredSessions, func(i, j int) bool {
		return filteredSessions[i].HeaderTime > filteredSessions[j].HeaderTime
	})

	usedPIDs := make(map[int]bool)
	var records []AuditRecord

	for _, sess := range filteredSessions {
		if sess.PID > 0 && !usedPIDs[sess.PID] {
			if proc, alive := procsByPID[sess.PID]; alive {
				usedPIDs[sess.PID] = true
				delta := sess.HeaderTime - proc.CreatedAt
				statusDetails := ComputeStatusDetails(sess.HeaderTime, now)
				records = append(records, AuditRecord{
					Harness:       "claude-code",
					PID:           sess.PID,
					CreatedAt:     proc.CreatedAt,
					CWD:           proc.CWD,
					Cmdline:       proc.Cmdline,
					MemoryMB:      proc.MemoryMB,
					CPUPercent:    proc.CPUPercent,
					SessionID:     sess.SessionID,
					SessionFile:   sess.FilePath,
					HeaderTime:    sess.HeaderTime,
					DeltaMs:       &delta,
					Status:        StatusPaired,
					StatusDetails: statusDetails,
				})
				continue
			}
		}

		records = append(records, AuditRecord{
			Harness:     "claude-code",
			PID:         sess.PID,
			CreatedAt:   sess.HeaderTime,
			CWD:         sess.CWD,
			SessionID:   sess.SessionID,
			SessionFile: sess.FilePath,
			HeaderTime:  sess.HeaderTime,
			Status:      StatusOrphanSession,
		})
	}

	for _, proc := range filteredProcs {
		if !usedPIDs[proc.PID] {
			records = append(records, AuditRecord{
				Harness:    "claude-code",
				PID:        proc.PID,
				CreatedAt:  proc.CreatedAt,
				CWD:        proc.CWD,
				Cmdline:    proc.Cmdline,
				MemoryMB:   proc.MemoryMB,
				CPUPercent: proc.CPUPercent,
				Status:     StatusOrphanProcess,
			})
		}
	}

	return records
}

// ComputeStatusDetails returns "ACTIVE" or "IDLE Xh" based on session age.
func ComputeStatusDetails(headerTime int64, now time.Time) string {
	if headerTime <= 0 {
		return "ACTIVE"
	}
	diff := now.Sub(time.UnixMilli(headerTime))
	if diff >= time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("IDLE %dh", hours)
	}
	return "ACTIVE"
}

// Live process & session finders using agent-observer scanner logic

func livePiProcs(ctx context.Context) ([]ProcessInfo, error) {
	return LivePiProcsWithFinder(ctx, observerscanner.LivePiProcessFinder)
}

// LivePiProcsWithFinder uses a provided finder function to discover Pi processes and enrich metadata.
func LivePiProcsWithFinder(ctx context.Context, finder func(ctx context.Context) ([]observerscanner.PiProcessInfo, error)) ([]ProcessInfo, error) {
	procs, err := finder(ctx)
	if err != nil {
		return nil, err
	}
	var res []ProcessInfo
	for _, p := range procs {
		info := ProcessInfo{
			PID:       p.PID,
			CreatedAt: p.CreateTime,
			CWD:       p.CWD,
		}
		EnrichProcessInfo(ctx, p.PID, &info)
		res = append(res, info)
	}
	return res, nil
}

// LivePiSessions discovers session files under Pi sessions dir.
func LivePiSessions(ctx context.Context) ([]SessionInfo, error) {
	dir := observerscanner.DefaultPiSessionsDir()
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var res []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, entry.Name())
		files, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			filePath := filepath.Join(subDir, f.Name())
			header, err := observerscanner.ReadPiSessionHeader(filePath)
			if err != nil {
				continue
			}
			headerTime := int64(0)
			if header.Timestamp != nil {
				switch ts := header.Timestamp.(type) {
				case float64:
					headerTime = int64(ts)
				case int64:
					headerTime = ts
				case string:
					if t, perr := time.Parse(time.RFC3339Nano, ts); perr == nil {
						headerTime = t.UnixMilli()
					} else if t, perr := time.Parse(time.RFC3339, ts); perr == nil {
						headerTime = t.UnixMilli()
					}
				}
			}
			if headerTime == 0 {
				if fi, ferr := f.Info(); ferr == nil {
					headerTime = fi.ModTime().UnixMilli()
				}
			}
			res = append(res, SessionInfo{
				Harness:    "pi",
				SessionID:  header.ID,
				CWD:        header.CWD,
				FilePath:   filePath,
				HeaderTime: headerTime,
			})
		}
	}
	return res, nil
}

func liveClaudeProcs(ctx context.Context) ([]ProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	var res []ProcessInfo
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		baseName := strings.ToLower(filepath.Base(name))
		isClaude := baseName == "claude" || baseName == "claude.exe"
		if !isClaude {
			cmdline, cErr := p.CmdlineSliceWithContext(ctx)
			if cErr == nil {
				for _, arg := range cmdline {
					if filepath.Base(arg) == "claude" || strings.HasSuffix(arg, "/claude") {
						isClaude = true
						break
					}
				}
			}
		}
		if !isClaude {
			continue
		}
		cwd, err := p.CwdWithContext(ctx)
		if err != nil || cwd == "" {
			continue
		}
		createTime, err := p.CreateTimeWithContext(ctx)
		if err != nil {
			createTime = 0
		}
		info := ProcessInfo{
			PID:       int(p.Pid),
			CreatedAt: createTime,
			CWD:       filepath.Clean(cwd),
		}
		EnrichProcessInfo(ctx, int(p.Pid), &info)
		res = append(res, info)
	}
	return res, nil
}

// LiveClaudeSessions discovers session descriptors under Claude Code session dirs.
func LiveClaudeSessions(ctx context.Context) ([]SessionInfo, error) {
	dirs := observerscanner.DefaultClaudeSessionDirs()
	var res []SessionInfo
	seen := make(map[string]bool)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".key") {
				continue
			}
			filePath := filepath.Join(dir, entry.Name())
			if seen[filePath] {
				continue
			}
			seen[filePath] = true
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			var desc observerscanner.SessionDescriptor
			if err := json.Unmarshal(data, &desc); err != nil {
				continue
			}
			res = append(res, SessionInfo{
				Harness:        "claude-code",
				SessionID:      desc.SessionID,
				CWD:            desc.CWD,
				FilePath:       filePath,
				HeaderTime:     desc.StartedAt,
				HarnessVersion: desc.HarnessVersion,
				PID:            desc.PID,
			})
		}
	}
	return res, nil
}

func liveCodexProcs(ctx context.Context) ([]ProcessInfo, error) {
	return LiveCodexProcsWithFinder(ctx, observerscanner.LiveCodexProcessFinder)
}

// LiveCodexProcsWithFinder uses a provided finder to discover Codex processes and enrich metadata.
func LiveCodexProcsWithFinder(ctx context.Context, finder func(ctx context.Context) ([]observerscanner.CodexProcessInfo, error)) ([]ProcessInfo, error) {
	procs, err := finder(ctx)
	if err != nil {
		return nil, err
	}
	var res []ProcessInfo
	for _, p := range procs {
		info := ProcessInfo{
			PID:       p.PID,
			CreatedAt: p.CreateTime,
			CWD:       p.CWD,
		}
		EnrichProcessInfo(ctx, p.PID, &info)
		res = append(res, info)
	}
	return res, nil
}

// LiveCodexSessions discovers session files under Codex sessions dir.
func LiveCodexSessions(ctx context.Context) ([]SessionInfo, error) {
	dir := observerscanner.DefaultCodexSessionsDir()
	if dir == "" {
		return nil, nil
	}
	var res []SessionInfo
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		header, err := observerscanner.ReadCodexSessionHeader(path)
		if err != nil || header == nil {
			return nil
		}
		res = append(res, SessionInfo{
			Harness:        "codex",
			SessionID:      header.ID,
			CWD:            header.CWD,
			FilePath:       path,
			HeaderTime:     header.Timestamp,
			HarnessVersion: header.CLIVersion,
		})
		return nil
	})
	return res, nil
}

func liveOpencodeProcs(ctx context.Context) ([]ProcessInfo, error) {
	return LiveOpencodeProcsWithFinder(ctx, observerscanner.LiveOpencodeProcessFinder)
}

// LiveOpencodeProcsWithFinder uses a provided finder to discover OpenCode processes and enrich metadata.
func LiveOpencodeProcsWithFinder(ctx context.Context, finder func(ctx context.Context) ([]observerscanner.OpencodeProcessInfo, error)) ([]ProcessInfo, error) {
	procs, err := finder(ctx)
	if err != nil {
		return nil, err
	}
	var res []ProcessInfo
	for _, p := range procs {
		info := ProcessInfo{
			PID:       p.PID,
			CreatedAt: p.CreateTime,
			CWD:       p.CWD,
		}
		EnrichProcessInfo(ctx, p.PID, &info)
		res = append(res, info)
	}
	return res, nil
}

// LiveOpencodeSessions queries sessions from OpenCode SQLite database.
func LiveOpencodeSessions(ctx context.Context) ([]SessionInfo, error) {
	dbPath := observerscanner.DefaultOpencodeDBPath()
	if dbPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil
	}
	db, err := parser.OpenOpencodeDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	sessions, err := db.GetSessions(ctx)
	if err != nil {
		return nil, err
	}
	var res []SessionInfo
	for _, s := range sessions {
		if s.TimeArchived != nil && *s.TimeArchived > 0 {
			continue
		}
		res = append(res, SessionInfo{
			Harness:    "opencode",
			SessionID:  s.ID,
			CWD:        s.Directory,
			FilePath:   dbPath,
			HeaderTime: s.TimeCreated,
		})
	}
	return res, nil
}

// EnrichProcessInfo inspects process details (cmdline, RSS memory, CPU) using gopsutil.
func EnrichProcessInfo(ctx context.Context, pid int, info *ProcessInfo) {
	if pid <= 0 {
		return
	}
	p, err := process.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return
	}
	if cmdline, err := p.CmdlineWithContext(ctx); err == nil {
		info.Cmdline = cmdline
	}
	if mem, err := p.MemoryInfoWithContext(ctx); err == nil && mem != nil {
		info.MemoryMB = float64(mem.RSS) / (1024 * 1024)
	}
	if cpu, err := p.CPUPercentWithContext(ctx); err == nil {
		info.CPUPercent = cpu
	}
}


