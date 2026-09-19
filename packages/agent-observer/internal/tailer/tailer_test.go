package tailer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTailerReadAndAppend(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	store, err := NewCursorStore(cursorPath)
	if err != nil {
		t.Fatalf("NewCursorStore error: %v", err)
	}

	targetFile := filepath.Join(tmpDir, "session.jsonl")
	initialLines := []string{
		`{"type":"user","message":"hello"}`,
		`{"type":"assistant","message":"hi there"}`,
	}

	f, err := os.Create(targetFile)
	if err != nil {
		t.Fatalf("create file error: %v", err)
	}
	for _, l := range initialLines {
		_, _ = f.WriteString(l + "\n")
	}
	_ = f.Close()

	var received []string
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tail, err := NewTailer(targetFile, store, func(line []byte) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, string(line))
		return nil
	})
	if err != nil {
		t.Fatalf("NewTailer error: %v", err)
	}

	go func() {
		_ = tail.Start(ctx)
	}()

	// Wait for initial read
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	if len(received) != 2 {
		t.Fatalf("expected 2 lines initially, got %d", len(received))
	}
	if received[0] != initialLines[0] || received[1] != initialLines[1] {
		t.Errorf("initial lines mismatch: got %v", received)
	}
	mu.Unlock()

	// Append more lines
	fAppend, err := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("open append error: %v", err)
	}
	appendedLine := `{"type":"tool_call","tool":"Bash"}`
	_, _ = fAppend.WriteString(appendedLine + "\n")
	_ = fAppend.Sync()
	_ = fAppend.Close()

	// Wait for append
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	if len(received) != 3 {
		t.Fatalf("expected 3 lines after append, got %d", len(received))
	}
	if received[2] != appendedLine {
		t.Errorf("appended line mismatch: got %q, want %q", received[2], appendedLine)
	}
	mu.Unlock()

	tail.Stop()
}

func TestTailerLargeLine16MBBuffer(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	store, err := NewCursorStore(cursorPath)
	if err != nil {
		t.Fatalf("NewCursorStore error: %v", err)
	}

	targetFile := filepath.Join(tmpDir, "large.jsonl")

	// Create a line of 2MB (larger than default 64KB bufio.Scanner buffer)
	largeData := make([]byte, 2*1024*1024)
	for i := range largeData {
		largeData[i] = 'a'
	}
	largeLine := fmt.Sprintf(`{"type":"tool_result","content":"%s"}`, string(largeData))

	if err := os.WriteFile(targetFile, []byte(largeLine+"\n"), 0600); err != nil {
		t.Fatalf("write large file error: %v", err)
	}

	var received []string
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tail, err := NewTailer(targetFile, store, func(line []byte) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, string(line))
		return nil
	})
	if err != nil {
		t.Fatalf("NewTailer error: %v", err)
	}

	go func() {
		_ = tail.Start(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 large line, got %d", len(received))
	}
	if len(received[0]) != len(largeLine) {
		t.Errorf("received line length %d != expected %d", len(received[0]), len(largeLine))
	}

	tail.Stop()
}
