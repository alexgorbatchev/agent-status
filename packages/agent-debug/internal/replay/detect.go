package replay

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-parser"
)

var (
	ErrUnknownHarness = errors.New("unable to auto-detect harness type from file content or extension")
	sqliteHeaderMagic = []byte("SQLite format 3\x00")
)

// DetectHarness identifies the agent harness from a given transcript file, database, or directory.
func DetectHarness(filePath string) (types.HarnessType, error) {
	cleanPath := filepath.Clean(filePath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", cleanPath, err)
	}

	if info.IsDir() {
		// Directory detection: could be a session directory with transcript files
		// Check for subagents/ or specific files
		entries, rErr := os.ReadDir(cleanPath)
		if rErr == nil {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".jsonl") {
					return DetectHarness(filepath.Join(cleanPath, e.Name()))
				}
				if strings.HasSuffix(e.Name(), ".db") {
					return types.HarnessOpencode, nil
				}
			}
		}
		return "", fmt.Errorf("%w: directory %s does not contain recognized session files", ErrUnknownHarness, cleanPath)
	}

	f, err := os.Open(cleanPath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", cleanPath, err)
	}
	defer func() { _ = f.Close() }()

	// 1. Check for SQLite database magic header (OpenCode)
	header := make([]byte, 16)
	n, _ := io.ReadFull(f, header)
	if n == 16 && bytes.Equal(header, sqliteHeaderMagic) {
		return types.HarnessOpencode, nil
	}
	if strings.HasSuffix(strings.ToLower(cleanPath), ".db") {
		return types.HarnessOpencode, nil
	}

	// 2. Rewind and inspect text / JSONL content
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek %s: %w", cleanPath, err)
	}

	scanner := bufio.NewScanner(f)
	// Buffer up to 10MB per line to handle large message dumps
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var sampleLines [][]byte
	for scanner.Scan() && len(sampleLines) < 20 {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) > 0 {
			lineCopy := make([]byte, len(line))
			copy(lineCopy, line)
			sampleLines = append(sampleLines, lineCopy)
		}
	}

	if len(sampleLines) == 0 {
		return "", fmt.Errorf("%w: file is empty", ErrUnknownHarness)
	}

	// Filename hints
	baseName := strings.ToLower(filepath.Base(cleanPath))
	if strings.HasPrefix(baseName, "rollout-") && strings.HasSuffix(baseName, ".jsonl") {
		return types.HarnessCodex, nil
	}

	// 3. Inspect JSON signatures
	for _, line := range sampleLines {
		s := string(line)

		// Codex signatures
		if strings.Contains(s, `"type":"session_meta"`) || strings.Contains(s, `"event_type":"session_meta"`) {
			return types.HarnessCodex, nil
		}

		// Pi signatures
		if strings.Contains(s, `"type":"session"`) && (strings.Contains(s, `"cwd"`) || strings.Contains(s, `"version"`) || strings.Contains(s, `"parentId"`)) {
			return types.HarnessPi, nil
		}
		if strings.Contains(s, `"type":"session_info"`) || strings.Contains(s, `"type":"model_change"`) || strings.Contains(s, `"type":"thinking_level_change"`) {
			return types.HarnessPi, nil
		}

		// Claude Code signatures
		if strings.Contains(s, `"type":"ai-title"`) || strings.Contains(s, `"type":"progress"`) || strings.Contains(s, `"type":"user"`) || strings.Contains(s, `"type":"assistant"`) {
			return types.HarnessClaudeCode, nil
		}
		if strings.Contains(s, `"costUSD"`) || strings.Contains(s, `"toolsUse"`) || strings.Contains(s, `"toolUse"`) {
			return types.HarnessClaudeCode, nil
		}
	}

	// 4. Fallback parser trial
	// Test parse first line with each parser
	firstLine := sampleLines[0]
	if evs, err := parser.NewClaudeCodeParser().ParseLine(firstLine); err == nil && len(evs) > 0 {
		return types.HarnessClaudeCode, nil
	}
	if evs, err := parser.ParsePiLine(firstLine, ""); err == nil && len(evs) > 0 {
		return types.HarnessPi, nil
	}
	if evs, err := parser.ParseCodexLine(firstLine); err == nil && len(evs) > 0 {
		return types.HarnessCodex, nil
	}

	return "", fmt.Errorf("%w for file %s", ErrUnknownHarness, cleanPath)
}
