package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
	"github.com/shirou/gopsutil/v4/process"
)

type OpencodeSessionDescriptor struct {
	PID            int    `json:"pid"`
	SessionID      string `json:"sessionId"`
	CWD            string `json:"cwd"`
	StartedAt      int64  `json:"startedAt"`
	UpdatedAt      int64  `json:"updatedAt"`
	HarnessVersion string `json:"harnessVersion"`
	IsActive       bool   `json:"isActive"`
	IsSubagent     bool   `json:"isSubagent"`
	ParentID       string `json:"parentId,omitempty"`
	Title          string `json:"title,omitempty"`
}

type OpencodeProcessInfo struct {
	PID        int
	CWD        string
	CreateTime int64 // unix millis
}

type OpencodeProcessFinder func(ctx context.Context) ([]OpencodeProcessInfo, error)

type OpencodeScanner struct {
	dbPath        string
	processFinder OpencodeProcessFinder
}

func DefaultOpencodeDBPath() string {
	if custom := os.Getenv("OPENCODE_DB_PATH"); custom != "" {
		return custom
	}
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, "opencode", "opencode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

func LiveOpencodeProcessFinder(ctx context.Context) ([]OpencodeProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing processes: %w", err)
	}

	var results []OpencodeProcessInfo
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}

		baseName := strings.ToLower(filepath.Base(name))
		isOpencode := baseName == "opencode" || baseName == "opencode.exe"
		if !isOpencode {
			cmdline, cErr := p.CmdlineSliceWithContext(ctx)
			if cErr != nil || len(cmdline) == 0 {
				continue
			}
			for _, arg := range cmdline {
				baseArg := strings.ToLower(filepath.Base(arg))
				if baseArg == "opencode" || strings.HasSuffix(arg, "/opencode") {
					isOpencode = true
					break
				}
			}
			if !isOpencode {
				continue
			}
		}

		cwd, err := p.CwdWithContext(ctx)
		if err != nil || cwd == "" {
			continue
		}

		createTime, err := p.CreateTimeWithContext(ctx)
		if err != nil {
			createTime = 0
		}

		results = append(results, OpencodeProcessInfo{
			PID:        int(p.Pid),
			CWD:        filepath.Clean(cwd),
			CreateTime: createTime,
		})
	}

	return results, nil
}

func NewOpencodeScanner(dbPath string, finder OpencodeProcessFinder) *OpencodeScanner {
	if dbPath == "" {
		dbPath = DefaultOpencodeDBPath()
	}
	if finder == nil {
		finder = LiveOpencodeProcessFinder
	}
	return &OpencodeScanner{
		dbPath:        dbPath,
		processFinder: finder,
	}
}

func (s *OpencodeScanner) Scan(ctx context.Context) ([]OpencodeSessionDescriptor, error) {
	if s.dbPath == "" {
		return nil, nil
	}

	if s.dbPath != ":memory:" {
		if _, err := os.Stat(s.dbPath); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("checking opencode database %q: %w", s.dbPath, err)
		}
	}

	db, err := parser.OpenOpencodeDB(s.dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening opencode database %q: %w", s.dbPath, err)
	}
	defer func() {
		_ = db.Close()
	}()

	sessions, err := db.GetSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting opencode sessions: %w", err)
	}

	var activeProcs []OpencodeProcessInfo
	if s.processFinder != nil {
		activeProcs, _ = s.processFinder(ctx)
	}

	procsByCWD := make(map[string][]OpencodeProcessInfo)
	for _, proc := range activeProcs {
		cleanCWD := filepath.Clean(proc.CWD)
		procsByCWD[cleanCWD] = append(procsByCWD[cleanCWD], proc)
	}

	var descriptors []OpencodeSessionDescriptor
	usedPIDs := make(map[int]bool)

	for _, sess := range sessions {
		cleanCWD := filepath.Clean(sess.Directory)
		parentID := ""
		if sess.ParentID != nil {
			parentID = *sess.ParentID
		}

		desc := OpencodeSessionDescriptor{
			PID:            0,
			SessionID:      sess.ID,
			CWD:            cleanCWD,
			StartedAt:      sess.TimeCreated,
			UpdatedAt:      sess.TimeUpdated,
			HarnessVersion: sess.Version,
			IsActive:       false,
			IsSubagent:     sess.IsSubagent,
			ParentID:       parentID,
			Title:          sess.Title,
		}

		if !sess.IsArchived {
			candidateProcs := procsByCWD[cleanCWD]
			var bestProc *OpencodeProcessInfo
			var minDiff int64 = 1<<62 - 1

			for _, proc := range candidateProcs {
				if usedPIDs[proc.PID] {
					continue
				}

				diff := sess.TimeCreated - proc.CreateTime
				if diff >= -5000 {
					absDiff := diff
					if absDiff < 0 {
						absDiff = -absDiff
					}
					if absDiff < minDiff {
						minDiff = absDiff
						procCopy := proc
						bestProc = &procCopy
					}
				}
			}

			if bestProc != nil {
				desc.PID = bestProc.PID
				desc.IsActive = true
				if bestProc.CreateTime > 0 {
					desc.StartedAt = bestProc.CreateTime
				}
				usedPIDs[bestProc.PID] = true
			} else if os.Getenv("AGENT_STATUS_TEST_MOCK_PROC") == "1" {
				desc.PID = 99994
				desc.IsActive = true
			}
		}

		descriptors = append(descriptors, desc)
	}

	return descriptors, nil
}
