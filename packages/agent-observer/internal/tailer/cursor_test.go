package tailer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCursorStore(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "observer-cursors.json")

	store, err := NewCursorStore(cursorPath)
	if err != nil {
		t.Fatalf("NewCursorStore() unexpected error = %v", err)
	}

	testKey := "123:456"
	c := &Cursor{
		Key:        testKey,
		Path:       "/path/to/session.jsonl",
		Offset:     1024,
		Size:       2048,
		Inode:      456,
		Device:     123,
		PrefixHash: "abcdef123456",
		UpdatedAt:  100000,
	}

	store.Set(c)

	if err := store.Save(); err != nil {
		t.Fatalf("store.Save() unexpected error = %v", err)
	}

	// Reload in a new store instance
	store2, err := NewCursorStore(cursorPath)
	if err != nil {
		t.Fatalf("NewCursorStore() reload error = %v", err)
	}

	got, ok := store2.Get(testKey)
	if !ok {
		t.Fatalf("store2.Get(%q) not found", testKey)
	}

	if got.Offset != 1024 || got.PrefixHash != "abcdef123456" || got.Inode != 456 {
		t.Errorf("got cursor %+v, mismatch", got)
	}

	// Non-existent key
	if _, ok := store2.Get("nonexistent-key"); ok {
		t.Errorf("expected not found for nonexistent key")
	}
}

func TestCursorStoreSaveError(t *testing.T) {
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "file_as_dir")
	_ = os.WriteFile(blockingFile, []byte("blocker"), 0600)

	badStore := &CursorStore{
		filePath: filepath.Join(blockingFile, "sub", "cursors.json"),
		cursors:  make(map[string]*Cursor),
	}

	if err := badStore.Save(); err == nil {
		t.Errorf("expected error when saving to invalid dir path")
	}

	// Target is an existing directory -> Rename fails
	badStoreDir := &CursorStore{
		filePath: tmpDir,
		cursors:  make(map[string]*Cursor),
	}
	if err := badStoreDir.Save(); err == nil {
		t.Errorf("expected error when saving over an existing directory")
	}
}

type dummyFileInfo struct {
	os.FileInfo
}

func (d dummyFileInfo) Sys() any {
	return "not-stat-t"
}

func TestGetFileInfoStat(t *testing.T) {
	_, _, err := getFileInfoStat(dummyFileInfo{})
	if err == nil {
		t.Errorf("expected error when Sys() cannot be cast to *syscall.Stat_t")
	}
}

func TestNewCursorStoreCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "corrupted-cursors.json")
	_ = os.WriteFile(badFile, []byte("{invalid-json"), 0600)

	_, err := NewCursorStore(badFile)
	if err == nil {
		t.Errorf("expected error for corrupted cursor store file")
	}
}

func TestCursorResetTriggers(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.jsonl")

	// Create content of >= 512 bytes
	content := make([]byte, 600)
	for i := range content {
		content[i] = 'A' + byte(i%26)
	}
	if err := os.WriteFile(filePath, content, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}

	dev, ino, err := getFileInfoStat(fi)
	if err != nil {
		t.Fatalf("getFileInfoStat error: %v", err)
	}

	sum := sha256.Sum256(content[:512])
	pHash := hex.EncodeToString(sum[:])

	tests := []struct {
		name        string
		cursor      *Cursor
		currentSize int64
		currentIno  uint64
		currentHash string
		wantReset   bool
	}{
		{
			name:        "nil cursor",
			cursor:      nil,
			currentSize: 100,
			currentIno:  ino,
			currentHash: pHash,
			wantReset:   false,
		},
		{
			name: "valid cursor - no reset",
			cursor: &Cursor{
				Offset:     10,
				Size:       int64(len(content)),
				Inode:      ino,
				Device:     dev,
				PrefixHash: pHash,
			},
			currentSize: int64(len(content)),
			currentIno:  ino,
			currentHash: pHash,
			wantReset:   false,
		},
		{
			name: "inode changed - trigger reset",
			cursor: &Cursor{
				Offset:     10,
				Size:       int64(len(content)),
				Inode:      ino + 999,
				Device:     dev,
				PrefixHash: pHash,
			},
			currentSize: int64(len(content)),
			currentIno:  ino,
			currentHash: pHash,
			wantReset:   true,
		},
		{
			name: "size shrunk below offset - trigger reset",
			cursor: &Cursor{
				Offset:     100,
				Size:       200,
				Inode:      ino,
				Device:     dev,
				PrefixHash: pHash,
			},
			currentSize: 50,
			currentIno:  ino,
			currentHash: pHash,
			wantReset:   true,
		},
		{
			name: "prefix hash mismatch - trigger reset",
			cursor: &Cursor{
				Offset:     10,
				Size:       int64(len(content)),
				Inode:      ino,
				Device:     dev,
				PrefixHash: "0000000000000000000000000000000000000000000000000000000000000000",
			},
			currentSize: int64(len(content)),
			currentIno:  ino,
			currentHash: pHash,
			wantReset:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset := ShouldResetCursor(tt.cursor, tt.currentIno, tt.currentSize, tt.currentHash)
			if reset != tt.wantReset {
				t.Errorf("ShouldResetCursor() = %v, want %v", reset, tt.wantReset)
			}
		})
	}
}

func TestComputePrefixHash(t *testing.T) {
	t.Run("size < 512", func(t *testing.T) {
		r := bytes.NewReader([]byte("short content"))
		h, err := ComputePrefixHash(r, int64(len("short content")))
		if err != nil {
			t.Fatal(err)
		}
		if h != "" {
			t.Errorf("expected empty hash for size < 512, got %q", h)
		}
	})

	t.Run("size >= 512 with short read", func(t *testing.T) {
		r := bytes.NewReader([]byte("short")) // len 5 < 512
		h, err := ComputePrefixHash(r, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if h != "" {
			t.Errorf("expected empty hash for short read, got %q", h)
		}
	})

	t.Run("size >= 512", func(t *testing.T) {
		data := make([]byte, 1000)
		for i := range data {
			data[i] = byte(i % 256)
		}
		r := bytes.NewReader(data)
		h, err := ComputePrefixHash(r, 1000)
		if err != nil {
			t.Fatal(err)
		}
		expectedSum := sha256.Sum256(data[:512])
		expectedHex := hex.EncodeToString(expectedSum[:])
		if h != expectedHex {
			t.Errorf("ComputePrefixHash() = %q, want %q", h, expectedHex)
		}
	})
}

func TestDefaultCursorPath(t *testing.T) {
	t.Run("with XDG_CACHE_HOME set", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "/custom/cache")
		path := DefaultCursorPath()
		if path != "/custom/cache/agent-status/observer-cursors.json" {
			t.Errorf("got DefaultCursorPath = %q, want %q", path, "/custom/cache/agent-status/observer-cursors.json")
		}
	})

	t.Run("with XDG_CACHE_HOME unset", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot get user home dir")
		}
		path := DefaultCursorPath()
		want := filepath.Join(home, ".cache", "agent-status", "observer-cursors.json")
		if path != want {
			t.Errorf("DefaultCursorPath() = %q, want %q", path, want)
		}
	})

	t.Run("with XDG_CACHE_HOME unset and UserHomeDir error", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "")
		origHome := userHomeDirFunc
		defer func() { userHomeDirFunc = origHome }()
		userHomeDirFunc = func() (string, error) {
			return "", os.ErrNotExist
		}

		path := DefaultCursorPath()
		want := filepath.Join(os.TempDir(), "agent-status", "observer-cursors.json")
		if path != want {
			t.Errorf("DefaultCursorPath() fallback = %q, want %q", path, want)
		}
	})

	t.Run("NewCursorStore with empty string uses default", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		store, err := NewCursorStore("")
		if err != nil {
			t.Fatalf("NewCursorStore(\"\") error: %v", err)
		}
		if store.filePath == "" {
			t.Errorf("expected store.filePath to be set")
		}
	})
}

func TestCursorStoreConcurrentSave(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "observer-cursors.json")
	store, err := NewCursorStore(cursorPath)
	if err != nil {
		t.Fatalf("NewCursorStore error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.Set(&Cursor{
				Key:       fmt.Sprintf("key-%d", idx),
				Path:      fmt.Sprintf("/path/%d.jsonl", idx),
				Offset:    int64(idx * 100),
				UpdatedAt: time.Now().UnixMilli(),
			})
			_ = store.Save()
		}(i)
	}
	wg.Wait()

	storeReloaded, err := NewCursorStore(cursorPath)
	if err != nil {
		t.Fatalf("reloading cursor store: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, ok := storeReloaded.Get(fmt.Sprintf("key-%d", i)); !ok {
			t.Errorf("missing key-%d in reloaded cursor store", i)
		}
	}
}

