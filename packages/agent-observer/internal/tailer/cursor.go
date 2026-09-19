package tailer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Cursor struct {
	Key        string `json:"key"` // e.g. "device:inode"
	Path       string `json:"path"`
	Offset     int64  `json:"offset"`
	Size       int64  `json:"size"`
	Inode      uint64 `json:"inode"`
	Device     uint64 `json:"device"`
	PrefixHash string `json:"prefixHash"`
	UpdatedAt  int64  `json:"updatedAt"`
}

type CursorStore struct {
	mu       sync.RWMutex
	filePath string
	cursors  map[string]*Cursor
}

var userHomeDirFunc = os.UserHomeDir

func DefaultCursorPath() string {
	if cacheHome := os.Getenv("XDG_CACHE_HOME"); cacheHome != "" {
		return filepath.Join(cacheHome, "agent-status", "observer-cursors.json")
	}
	home, err := userHomeDirFunc()
	if err != nil {
		return filepath.Join(os.TempDir(), "agent-status", "observer-cursors.json")
	}
	return filepath.Join(home, ".cache", "agent-status", "observer-cursors.json")
}

func NewCursorStore(filePath string) (*CursorStore, error) {
	if filePath == "" {
		filePath = DefaultCursorPath()
	}

	cs := &CursorStore{
		filePath: filePath,
		cursors:  make(map[string]*Cursor),
	}

	if err := cs.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading cursor store: %w", err)
		}
	}

	return cs, nil
}

func (cs *CursorStore) load() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		return err
	}

	var cursors map[string]*Cursor
	if err := json.Unmarshal(data, &cursors); err != nil {
		return fmt.Errorf("parsing cursor store json: %w", err)
	}

	if cursors != nil {
		cs.cursors = cursors
	}
	return nil
}

func (cs *CursorStore) Get(key string) (*Cursor, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	c, ok := cs.cursors[key]
	if !ok {
		return nil, false
	}
	// return copy
	cCopy := *c
	return &cCopy, true
}

func (cs *CursorStore) Set(c *Cursor) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cCopy := *c
	cCopy.UpdatedAt = time.Now().UnixMilli()
	cs.cursors[c.Key] = &cCopy
}

func (cs *CursorStore) Save() error {
	cs.mu.RLock()
	data, err := json.MarshalIndent(cs.cursors, "", "  ")
	cs.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshaling cursors: %w", err)
	}

	dir := filepath.Dir(cs.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating cursor directory %q: %w", dir, err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", cs.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("writing cursor temp file: %w", err)
	}

	if err := os.Rename(tmpFile, cs.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("renaming cursor file: %w", err)
	}

	return nil
}

func ShouldResetCursor(c *Cursor, currentIno uint64, currentSize int64, currentHash string) bool {
	if c == nil {
		return false
	}
	// Trigger 1: Inode change
	if currentIno != 0 && c.Inode != 0 && c.Inode != currentIno {
		return true
	}
	// Trigger 2: Size shrunk below recorded offset
	if currentSize < c.Offset {
		return true
	}
	// Trigger 3: Prefix SHA-256 mismatch (when size >= 512)
	if c.PrefixHash != "" && currentHash != "" && c.PrefixHash != currentHash {
		return true
	}
	return false
}

func ComputePrefixHash(file io.ReaderAt, size int64) (string, error) {
	if size < 512 {
		return "", nil
	}
	buf := make([]byte, 512)
	n, err := file.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n < 512 {
		return "", nil
	}
	sum := sha256.Sum256(buf[:n])
	return hex.EncodeToString(sum[:]), nil
}
