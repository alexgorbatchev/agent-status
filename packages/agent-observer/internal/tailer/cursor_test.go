package tailer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
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
