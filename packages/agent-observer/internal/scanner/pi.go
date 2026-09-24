package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

type PiSessionDescriptor struct {
	PID            int    `json:"pid"`
	SessionID      string `json:"sessionId"`
	CWD            string `json:"cwd"`
	TranscriptPath string `json:"transcriptPath"`
	StartedAt      int64  `json:"startedAt"`
	HarnessVersion string `json:"harnessVersion"`
	IsActive       bool   `json:"isActive"`
}

type PiProcessInfo struct {
	PID        int
	CWD        string
	CreateTime int64 // unix millis
}

type PiHeader struct {
	Type      string      `json:"type"`
	Version   int         `json:"version"`
	ID        string      `json:"id"`
	Timestamp interface{} `json:"timestamp"`
	CWD       string      `json:"cwd"`
}

type PiProcessFinder func(ctx context.Context) ([]PiProcessInfo, error)

type PiScanner struct {
	sessionsDir   string
	processFinder PiProcessFinder
}

func DefaultPiSessionsDir() string {
	if custom := os.Getenv("PI_SESSIONS_DIR"); custom != "" {
		return custom
	}
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return filepath.Join(dir, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

// EncodePiCWD implements Pi's directory naming rule:
// --${cwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--
func EncodePiCWD(cwd string) string {
	clean := filepath.Clean(cwd)
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.TrimPrefix(clean, "\\")
	r := strings.NewReplacer("/", "-", "\\", "-", ":", "-")
	return "--" + r.Replace(clean) + "--"
}

// LivePiProcessFinder uses gopsutil/v4/process to discover running pi processes.
func LivePiProcessFinder(ctx context.Context) ([]PiProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing processes: %w", err)
	}

	var results []PiProcessInfo
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}

		baseName := strings.ToLower(filepath.Base(name))
		if baseName != "pi" && baseName != "pi.exe" {
			// Also check cmdline in case it was run via node / npx / wrapper
			cmdline, cErr := p.CmdlineSliceWithContext(ctx)
			if cErr != nil || len(cmdline) == 0 {
				continue
			}
			isPi := false
			for _, arg := range cmdline {
				if filepath.Base(arg) == "pi" || strings.HasSuffix(arg, "/pi") {
					isPi = true
					break
				}
			}
			if !isPi {
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

		results = append(results, PiProcessInfo{
			PID:        int(p.Pid),
			CWD:        filepath.Clean(cwd),
			CreateTime: createTime,
		})
	}

	return results, nil
}

func NewPiScanner(sessionsDir string, finder PiProcessFinder) *PiScanner {
	if sessionsDir == "" {
		sessionsDir = DefaultPiSessionsDir()
	}
	if finder == nil {
		finder = LivePiProcessFinder
	}
	return &PiScanner{
		sessionsDir:   sessionsDir,
		processFinder: finder,
	}
}

// ReadPiSessionHeader reads the first JSON line of a Pi session file.
func ReadPiSessionHeader(path string) (*PiHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("empty session file")
	}

	var header PiHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("unmarshaling session header: %w", err)
	}

	if header.Type != "session" || header.ID == "" {
		return nil, fmt.Errorf("invalid session header type: %s", header.Type)
	}

	return &header, nil
}

// Scan enumerates Pi sessions under sessionsDir, finds running Pi processes,
// and pairs processes to their corresponding session files.
func (s *PiScanner) Scan(ctx context.Context) ([]PiSessionDescriptor, error) {
	if s.sessionsDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading pi sessions dir: %w", err)
	}

	var activeProcs []PiProcessInfo
	if s.processFinder != nil {
		activeProcs, _ = s.processFinder(ctx)
	}

	// Map procs by CWD
	procsByCWD := make(map[string][]PiProcessInfo)
	for _, proc := range activeProcs {
		cleanCWD := filepath.Clean(proc.CWD)
		procsByCWD[cleanCWD] = append(procsByCWD[cleanCWD], proc)
	}

	type piCandidate struct {
		desc       PiSessionDescriptor
		headerTime int64
		modTime    int64
		cleanCWD   string
	}
	var candidates []piCandidate

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		subDir := filepath.Join(s.sessionsDir, entry.Name())
		files, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}

			filePath := filepath.Join(subDir, f.Name())
			header, err := ReadPiSessionHeader(filePath)
			if err != nil {
				continue
			}

			modTime, sErr := ExtractLastActiveTimestamp(filePath)
			if sErr != nil {
				if info, infoErr := f.Info(); infoErr == nil {
					modTime = info.ModTime().UnixMilli()
				}
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
				headerTime = modTime
			}
			if modTime == 0 {
				modTime = headerTime
			}

			cleanCWD := filepath.Clean(header.CWD)
			desc := PiSessionDescriptor{
				PID:            0,
				SessionID:      header.ID,
				CWD:            cleanCWD,
				TranscriptPath: filePath,
				StartedAt:      headerTime,
				HarnessVersion: fmt.Sprintf("v%d", header.Version),
				IsActive:       false,
			}

			candidates = append(candidates, piCandidate{
				desc:       desc,
				headerTime: headerTime,
				modTime:    modTime,
				cleanCWD:   cleanCWD,
			})
		}
	}

	// Sort candidates by modification time descending so newest and resumed sessions take precedence.
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].modTime != candidates[j].modTime {
			return candidates[i].modTime > candidates[j].modTime
		}
		return candidates[i].headerTime > candidates[j].headerTime
	})

	var descriptors []PiSessionDescriptor
	usedPIDs := make(map[int]bool)

	for i := range candidates {
		cand := &candidates[i]
		candidateProcs := procsByCWD[cand.cleanCWD]
		var bestProc *PiProcessInfo
		var minDiff int64 = 1<<62 - 1

		for _, proc := range candidateProcs {
			if usedPIDs[proc.PID] {
				continue
			}

			// Check if process started at or before the session was created or modified,
			// allowing resumed sessions (where headerTime is in the past, but modTime reflects recent activity).
			diff := cand.headerTime - proc.CreateTime
			if diff < -5000 && cand.modTime-proc.CreateTime >= -5000 {
				diff = cand.modTime - proc.CreateTime
			}
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
			cand.desc.PID = bestProc.PID
			cand.desc.IsActive = true
			if bestProc.CreateTime > 0 {
				cand.desc.StartedAt = bestProc.CreateTime
			}
			usedPIDs[bestProc.PID] = true
		} else if os.Getenv("AGENT_STATUS_TEST_MOCK_PROC") == "1" {
			cand.desc.PID = 99992
			cand.desc.IsActive = true
		}

		descriptors = append(descriptors, cand.desc)
	}

	return descriptors, nil
}
