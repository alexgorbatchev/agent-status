package tailer

import (
	"context"
	"errors"
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

func TestTailerFileTruncationAndHandlerError(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	targetFile := filepath.Join(tmpDir, "truncate.jsonl")

	// Initial long file
	_ = os.WriteFile(targetFile, []byte("line 1 long long long long\nline 2 long long long long\n"), 0600)

	var mu sync.Mutex
	var received []string

	tail, err := NewTailer(targetFile, store, func(line []byte) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, string(line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// First read
	if err := tail.readAvailable(); err != nil {
		t.Fatalf("readAvailable error: %v", err)
	}

	mu.Lock()
	if len(received) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(received))
	}
	mu.Unlock()

	// Truncate file to shorter size
	_ = os.WriteFile(targetFile, []byte("short\n"), 0600)

	// Second read: should detect shrink and reset offset
	if err := tail.readAvailable(); err != nil {
		t.Fatalf("readAvailable after truncate error: %v", err)
	}

	mu.Lock()
	if len(received) != 3 || received[2] != "short" {
		t.Errorf("expected 3 lines with 'short', got %v", received)
	}
	mu.Unlock()

	// Read on non-existent file
	missingTail, _ := NewTailer(filepath.Join(tmpDir, "missing.jsonl"), store, nil)
	if err := missingTail.readAvailable(); err == nil {
		t.Errorf("expected error for missing file in readAvailable")
	}
}

func TestTailerStartStopMethod(t *testing.T) {
	origInterval := tailerPollInterval
	tailerPollInterval = 20 * time.Millisecond
	defer func() { tailerPollInterval = origInterval }()

	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	targetFile := filepath.Join(tmpDir, "stop.jsonl")
	_ = os.WriteFile(targetFile, []byte("initial\n"), 0600)

	var mu sync.Mutex
	var count int
	tail, err := NewTailer(targetFile, store, func(line []byte) error {
		mu.Lock()
		defer mu.Unlock()
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- tail.Start(context.Background())
	}()

	// Wait for initial read and ticker poll
	time.Sleep(50 * time.Millisecond)

	// Append via ticker check
	f, _ := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("second\n")
	_ = f.Close()

	time.Sleep(60 * time.Millisecond)

	tail.Stop()

	select {
	case err := <-doneCh:
		if err != nil {
			t.Errorf("expected nil error on tail.Stop(), got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for tail.Start to exit after tail.Stop()")
	}

	mu.Lock()
	if count != 2 {
		t.Errorf("expected 2 lines read, got %d", count)
	}
	mu.Unlock()
}

func TestTailerEmptyLinesAndNilHandler(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	targetFile := filepath.Join(tmpDir, "empty_lines.jsonl")

	// File containing empty lines and whitespace lines
	_ = os.WriteFile(targetFile, []byte("\n\nvalid line\n\n\n"), 0600)

	var received []string
	tail, err := NewTailer(targetFile, store, func(line []byte) error {
		received = append(received, string(line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := tail.readAvailable(); err != nil {
		t.Fatalf("readAvailable error: %v", err)
	}

	if len(received) != 1 || received[0] != "valid line" {
		t.Errorf("expected 1 'valid line', got %v", received)
	}

	// Test with nil onLine handler
	tailNilHandler, err := NewTailer(targetFile, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tailNilHandler.readAvailable(); err != nil {
		t.Fatalf("readAvailable with nil handler error: %v", err)
	}
}

func TestTailerResumeFromExistingCursor(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	targetFile := filepath.Join(tmpDir, "resume.jsonl")

	line1 := "first line 123456789\n"
	line2 := "second line 987654321\n"
	_ = os.WriteFile(targetFile, []byte(line1+line2), 0600)

	// Tailer 1 reads both lines and saves cursor
	tail1, err := NewTailer(targetFile, store, func(line []byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := tail1.readAvailable(); err != nil {
		t.Fatal(err)
	}

	// Tailer 2 opens same file and should read 0 lines because cursor is up to date
	var read2 []string
	tail2, err := NewTailer(targetFile, store, func(line []byte) error {
		read2 = append(read2, string(line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tail2.readAvailable(); err != nil {
		t.Fatal(err)
	}

	if len(read2) != 0 {
		t.Errorf("expected 0 lines read on resume with up-to-date cursor, got %d", len(read2))
	}

	// Append a 3rd line
	line3 := "third line after resume\n"
	f, _ := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString(line3)
	_ = f.Close()

	if err := tail2.readAvailable(); err != nil {
		t.Fatal(err)
	}

	if len(read2) != 1 || read2[0] != "third line after resume" {
		t.Errorf("expected 1 line 'third line after resume', got %v", read2)
	}
}

func TestTailerCloseWatcherEventsDirectly(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	targetFile := filepath.Join(tmpDir, "close_watcher.jsonl")
	_ = os.WriteFile(targetFile, []byte("line\n"), 0600)

	tail, err := NewTailer(targetFile, store, func(line []byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- tail.Start(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)

	// Closing watcher directly causes events channel to close and Start to return nil
	_ = tail.watcher.Close()

	select {
	case err := <-doneCh:
		if err != nil {
			t.Errorf("expected nil on watcher close, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for tailer to exit on watcher close")
	}
}

func TestTailerStartInvalidWatchDir(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	invalidFile := filepath.Join(tmpDir, "deeply", "nested", "nonexistent", "file.jsonl")
	tail, err := NewTailer(invalidFile, store, func(line []byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = tail.Start(ctx)
	if err == nil {
		t.Errorf("expected error when starting tailer on nonexistent directory")
	}
}

func TestTailerEventsChannel(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	targetFile := filepath.Join(tmpDir, "events.jsonl")
	_ = os.WriteFile(targetFile, []byte("line 1\n"), 0600)

	var mu sync.Mutex
	var lines []string
	tail, err := NewTailer(targetFile, store, func(l []byte) error {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, string(l))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = tail.Start(ctx)
	}()

	time.Sleep(30 * time.Millisecond)

	// Append data and trigger write event
	f, _ := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("line 2\n")
	_ = f.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := len(lines)
		mu.Unlock()
		if c >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines, got %d", len(lines))
	}
	mu.Unlock()

	tail.Stop()
}

func TestTailerBackpressureHandlerErrorPausesReads(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewCursorStore(filepath.Join(tmpDir, "cursors.json"))
	targetFile := filepath.Join(tmpDir, "backpressure.jsonl")

	line1 := "line 1 payload\n"
	line2 := "line 2 payload\n"
	line3 := "line 3 payload\n"
	_ = os.WriteFile(targetFile, []byte(line1+line2+line3), 0600)

	var mu sync.Mutex
	var received []string
	shouldFailLine2 := true

	tail, err := NewTailer(targetFile, store, func(line []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if string(line) == "line 2 payload" && shouldFailLine2 {
			return errors.New("simulated backpressure / shipper full")
		}
		received = append(received, string(line))
		return nil
	})
	if err != nil {
		t.Fatalf("NewTailer error: %v", err)
	}

	// 1. First readAvailable pass: Line 1 succeeds, Line 2 fails with backpressure
	if err := tail.readAvailable(); err != nil {
		t.Fatalf("readAvailable error: %v", err)
	}

	mu.Lock()
	if len(received) != 1 || received[0] != "line 1 payload" {
		t.Fatalf("expected only line 1 to be received on backpressure, got %v", received)
	}
	mu.Unlock()

	// Offset must pause at line 1 and not skip line 2
	expectedOffset1 := int64(len(line1))
	if tail.CurrentOffset() != expectedOffset1 {
		t.Errorf("CurrentOffset() = %d, want %d", tail.CurrentOffset(), expectedOffset1)
	}

	// 2. Clear backpressure condition and read again
	mu.Lock()
	shouldFailLine2 = false
	mu.Unlock()

	if err := tail.readAvailable(); err != nil {
		t.Fatalf("second readAvailable error: %v", err)
	}

	mu.Lock()
	if len(received) != 3 {
		t.Fatalf("expected all 3 lines to be received after unblocking, got %d: %v", len(received), received)
	}
	if received[0] != "line 1 payload" || received[1] != "line 2 payload" || received[2] != "line 3 payload" {
		t.Errorf("unexpected received lines: %v", received)
	}
	mu.Unlock()

	totalBytes := int64(len(line1 + line2 + line3))
	if tail.CurrentOffset() != totalBytes {
		t.Errorf("final CurrentOffset() = %d, want %d", tail.CurrentOffset(), totalBytes)
	}
}

func TestTailerCursorCommittedDelivery(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursors.json")
	store, _ := NewCursorStore(cursorPath)
	targetFile := filepath.Join(tmpDir, "committed.jsonl")

	line1 := "line 1 committed\n"
	line2 := "line 2 committed\n"
	_ = os.WriteFile(targetFile, []byte(line1+line2), 0600)

	var mu sync.Mutex
	var received []string
	var offsets []int64

	tail, err := NewTailerWithOffsetHandler(targetFile, store, func(line []byte, offset int64) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, string(line))
		offsets = append(offsets, offset)
		return nil
	})
	if err != nil {
		t.Fatalf("NewTailerWithOffsetHandler error: %v", err)
	}

	// Read from disk
	if err := tail.readAvailable(); err != nil {
		t.Fatalf("readAvailable error: %v", err)
	}

	mu.Lock()
	if len(received) != 2 {
		t.Fatalf("expected 2 lines received, got %d", len(received))
	}
	mu.Unlock()

	// In committed delivery mode, CurrentOffset is advanced in memory but CommittedOffset remains 0 until ACK
	totalBytes := int64(len(line1 + line2))
	if tail.CurrentOffset() != totalBytes {
		t.Errorf("CurrentOffset() = %d, want %d", tail.CurrentOffset(), totalBytes)
	}
	if tail.CommittedOffset() != 0 {
		t.Errorf("CommittedOffset() = %d, want 0 before commit", tail.CommittedOffset())
	}

	// Cursor store should not have committed offset yet
	if c, ok := store.Get(tail.key); ok && c.Offset != 0 {
		t.Errorf("store cursor offset = %d, want 0", c.Offset)
	}

	// Now simulate ACK for line 1
	line1Offset := int64(len(line1))
	if err := tail.Commit(line1Offset); err != nil {
		t.Fatalf("tail.Commit error: %v", err)
	}
	if tail.CommittedOffset() != line1Offset {
		t.Errorf("CommittedOffset() = %d, want %d", tail.CommittedOffset(), line1Offset)
	}

	c, ok := store.Get(tail.key)
	if !ok || c.Offset != line1Offset {
		t.Errorf("store.Get(%q) = %v (ok: %v), want offset %d", tail.key, c, ok, line1Offset)
	}

	// Re-reading from disk without new file writes should be a no-op
	if err := tail.readAvailable(); err != nil {
		t.Fatalf("readAvailable no-op error: %v", err)
	}
	if tail.CurrentOffset() != totalBytes {
		t.Errorf("CurrentOffset() = %d, want %d", tail.CurrentOffset(), totalBytes)
	}

	// Now simulate ACK for line 2
	if err := tail.Commit(totalBytes); err != nil {
		t.Fatalf("tail.Commit line 2 error: %v", err)
	}
	if tail.CommittedOffset() != totalBytes {
		t.Errorf("CommittedOffset() = %d, want %d", tail.CommittedOffset(), totalBytes)
	}

	// Lower offset commit should be ignored (monotonic)
	if err := tail.Commit(line1Offset); err != nil {
		t.Fatalf("tail.Commit lower offset error: %v", err)
	}
	if tail.CommittedOffset() != totalBytes {
		t.Errorf("CommittedOffset() after lower commit = %d, want %d", tail.CommittedOffset(), totalBytes)
	}
}

