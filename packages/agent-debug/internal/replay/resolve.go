package replay

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
)

// ResolvedTarget holds the resolved file path, harness type, and optional session ID.
type ResolvedTarget struct {
	FilePath  string            `json:"filePath"`
	Harness   types.HarnessType `json:"harness"`
	SessionID string            `json:"sessionId,omitempty"`
}

// ResolveSessionTarget resolves an input argument that could be an explicit file path or a session ID.
func ResolveSessionTarget(ctx context.Context, target string, harnessHint types.HarnessType) (*ResolvedTarget, error) {
	if strings.TrimSpace(target) == "" {
		return nil, fmt.Errorf("target file path or session ID cannot be empty")
	}

	cleanTarget := filepath.Clean(target)

	// 1. Direct file or directory path match
	if info, err := os.Stat(cleanTarget); err == nil {
		harness := harnessHint
		if harness == "" {
			if detected, dErr := DetectHarness(cleanTarget); dErr == nil {
				harness = detected
			}
		}
		if info.IsDir() {
			// Find first .jsonl or .db inside directory
			var innerFile string
			_ = filepath.WalkDir(cleanTarget, func(path string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if strings.HasSuffix(d.Name(), ".jsonl") || strings.HasSuffix(d.Name(), ".db") {
						innerFile = path
						return fs.SkipAll
					}
				}
				return nil
			})
			if innerFile != "" {
				if harness == "" {
					harness, _ = DetectHarness(innerFile)
				}
				return &ResolvedTarget{
					FilePath: innerFile,
					Harness:  harness,
				}, nil
			}
		}
		return &ResolvedTarget{
			FilePath: cleanTarget,
			Harness:  harness,
		}, nil
	}

	// 2. Resolve as a session ID in standard harness directories
	home, _ := os.UserHomeDir()

	// A. Claude Code search
	if harnessHint == "" || harnessHint == types.HarnessClaudeCode {
		claudeDirs := []string{
			filepath.Join(home, ".claude", "projects"),
			filepath.Join(home, ".claude", "sessions"),
		}
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			claudeDirs = append(claudeDirs, filepath.Join(xdgData, "ai-registry", "claude-code", "sessions"))
		}
		for _, dir := range claudeDirs {
			if match := findTranscriptMatching(dir, target); match != "" {
				return &ResolvedTarget{
					FilePath:  match,
					Harness:   types.HarnessClaudeCode,
					SessionID: target,
				}, nil
			}
		}
	}

	// B. Pi search
	if harnessHint == "" || harnessHint == types.HarnessPi {
		piDir := filepath.Join(home, ".pi", "agent", "sessions")
		if custom := os.Getenv("PI_SESSIONS_DIR"); custom != "" {
			piDir = custom
		}
		if match := findTranscriptMatching(piDir, target); match != "" {
			return &ResolvedTarget{
				FilePath:  match,
				Harness:   types.HarnessPi,
				SessionID: target,
			}, nil
		}
	}

	// C. Codex search
	if harnessHint == "" || harnessHint == types.HarnessCodex {
		codexDir := filepath.Join(home, ".codex", "sessions")
		if custom := os.Getenv("CODEX_SESSIONS_DIR"); custom != "" {
			codexDir = custom
		}
		if match := findTranscriptMatching(codexDir, target); match != "" {
			return &ResolvedTarget{
				FilePath:  match,
				Harness:   types.HarnessCodex,
				SessionID: target,
			}, nil
		}
	}

	// D. OpenCode search
	if harnessHint == "" || harnessHint == types.HarnessOpencode {
		dbPath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
		if custom := os.Getenv("OPENCODE_DB_PATH"); custom != "" {
			dbPath = custom
		} else if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			dbPath = filepath.Join(xdgData, "opencode", "opencode.db")
		}
		if _, err := os.Stat(dbPath); err == nil {
			if db, err := parser.OpenOpencodeDB(dbPath); err == nil {
				defer func() { _ = db.Close() }()
				if sess, err := db.GetSessionByID(ctx, target); err == nil && sess != nil {
					return &ResolvedTarget{
						FilePath:  dbPath,
						Harness:   types.HarnessOpencode,
						SessionID: target,
					}, nil
				}
			}
		}
	}

	if harnessHint != "" {
		return nil, fmt.Errorf("session matching %q not found in filesystem or standard %s storage directories", target, harnessHint)
	}

	return nil, fmt.Errorf("session matching %q not found as a file or in any standard harness directory (~/.claude, ~/.pi, ~/.codex, ~/.local/share/opencode)", target)
}

func findTranscriptMatching(rootDir string, pattern string) string {
	if _, err := os.Stat(rootDir); err != nil {
		return ""
	}

	var bestMatch string
	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		name := d.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			return nil
		}

		baseNoExt := strings.TrimSuffix(name, ".jsonl")
		// Exact session ID match
		if baseNoExt == pattern {
			bestMatch = path
			return fs.SkipAll
		}
		// Substring match
		if strings.Contains(baseNoExt, pattern) {
			bestMatch = path
		}
		return nil
	})

	return bestMatch
}
