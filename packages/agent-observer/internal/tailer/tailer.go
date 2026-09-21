package tailer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var tailerPollInterval = 1 * time.Second

type LineHandler func(line []byte) error

type LineOffsetHandler func(line []byte, offset int64) error

type Tailer struct {
	filePath string
	store    *CursorStore
	onLine   LineHandler
	onOffset LineOffsetHandler

	watcher         *fsnotify.Watcher
	stopCh          chan struct{}
	once            sync.Once
	mu              sync.Mutex
	readMu          sync.Mutex
	offset          int64
	committedOffset int64
	autoCommit      bool
	key             string
	prefixHash      string
	fileSize        int64
	inode           uint64
	device          uint64
}

// NewTailer creates a tailer using the classic LineHandler (auto-commits read offsets to cursor store).
func NewTailer(filePath string, store *CursorStore, onLine LineHandler) (*Tailer, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path for %q: %w", filePath, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Tailer{
		filePath:   absPath,
		store:      store,
		onLine:     onLine,
		watcher:    watcher,
		stopCh:     make(chan struct{}),
		autoCommit: true,
	}, nil
}

// NewTailerWithOffsetHandler creates a tailer using LineOffsetHandler with explicit ACK-committed delivery.
func NewTailerWithOffsetHandler(filePath string, store *CursorStore, onOffset LineOffsetHandler) (*Tailer, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path for %q: %w", filePath, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Tailer{
		filePath:   absPath,
		store:      store,
		onOffset:   onOffset,
		watcher:    watcher,
		stopCh:     make(chan struct{}),
		autoCommit: false,
	}, nil
}

func (t *Tailer) Start(ctx context.Context) error {
	defer func() {
		_ = t.watcher.Close()
	}()

	dir := filepath.Dir(t.filePath)
	if err := t.watcher.Add(dir); err != nil {
		// If watching dir fails, try watching file directly
		if fErr := t.watcher.Add(t.filePath); fErr != nil {
			return fmt.Errorf("watching file path: %w", fErr)
		}
	}

	// Initial read
	_ = t.readAvailable()

	ticker := time.NewTicker(tailerPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.stopCh:
			return nil
		case event, ok := <-t.watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				cleanEventPath := filepath.Clean(event.Name)
				if cleanEventPath == t.filePath || filepath.Base(cleanEventPath) == filepath.Base(t.filePath) {
					_ = t.readAvailable()
				}
			}
		case _, ok := <-t.watcher.Errors:
			if !ok {
				return nil
			}
		case <-ticker.C:
			_ = t.readAvailable()
		}
	}
}

func (t *Tailer) Stop() {
	t.once.Do(func() {
		close(t.stopCh)
	})
}

// CurrentOffset returns the in-memory read position of the tailer.
func (t *Tailer) CurrentOffset() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.offset
}

// CommittedOffset returns the highest confirmed/ACKed offset persisted to CursorStore.
func (t *Tailer) CommittedOffset() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.committedOffset
}

// Commit updates the cursor store to the confirmed offset and persists it to disk.
func (t *Tailer) Commit(offset int64) error {
	t.mu.Lock()
	if offset <= t.committedOffset {
		t.mu.Unlock()
		return nil
	}
	t.committedOffset = offset
	key := t.key
	prefixHash := t.prefixHash
	size := t.fileSize
	ino := t.inode
	dev := t.device
	store := t.store
	filePath := t.filePath
	t.mu.Unlock()

	if store != nil && key != "" {
		if size == 0 && ino == 0 {
			if fi, err := os.Stat(filePath); err == nil {
				size = fi.Size()
				dev, ino, _ = getFileInfoStat(fi)
			}
		}
		store.Set(&Cursor{
			Key:        key,
			Path:       filePath,
			Offset:     offset,
			Size:       size,
			Inode:      ino,
			Device:     dev,
			PrefixHash: prefixHash,
			UpdatedAt:  time.Now().UnixMilli(),
		})
		return store.Save()
	}
	return nil
}

func (t *Tailer) readAvailable() error {
	t.readMu.Lock()
	defer t.readMu.Unlock()

	f, err := os.Open(t.filePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	dev, ino, _ := getFileInfoStat(fi)
	key := fmt.Sprintf("%d:%d", dev, ino)
	if dev == 0 && ino == 0 {
		key = t.filePath
	}

	prefixHash, _ := ComputePrefixHash(f, fi.Size())

	t.mu.Lock()
	if t.key != "" && t.key != key {
		t.offset = 0
		t.committedOffset = 0
	}
	t.key = key
	t.prefixHash = prefixHash
	t.fileSize = fi.Size()
	t.device = dev
	t.inode = ino

	// Initialize offset from cursor store on first run
	if t.offset == 0 && t.committedOffset == 0 {
		if c, ok := t.store.Get(key); ok {
			if ShouldResetCursor(c, ino, fi.Size(), prefixHash) {
				t.offset = 0
				t.committedOffset = 0
			} else {
				t.offset = c.Offset
				t.committedOffset = c.Offset
			}
		}
	}

	if t.offset > fi.Size() {
		t.offset = 0
		t.committedOffset = 0
	}

	startOffset := t.offset
	t.mu.Unlock()

	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		return fmt.Errorf("seeking to offset %d: %w", startOffset, err)
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	bytesReadTotal := startOffset

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				lineBytes := int64(len(line))
				cleanLine := bytes.TrimRight(line, "\r\n")
				if len(cleanLine) > 0 {
					newLineOffset := bytesReadTotal + lineBytes
					var hErr error
					if t.onOffset != nil {
						hErr = t.onOffset(cleanLine, newLineOffset)
					} else if t.onLine != nil {
						hErr = t.onLine(cleanLine)
					}

					if hErr != nil {
						// Handler signaled backpressure or error; stop reading immediately
						// without advancing read offset past this unhandled line.
						break
					}
				}
				bytesReadTotal += lineBytes
				t.mu.Lock()
				t.offset = bytesReadTotal
				t.mu.Unlock()
			} else {
				// Partial line, stop reading for now, wait for newline
				break
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	// In autoCommit mode, update CursorStore upon reading
	if t.autoCommit {
		t.mu.Lock()
		shouldSave := bytesReadTotal > t.committedOffset
		if shouldSave {
			t.committedOffset = bytesReadTotal
		}
		key := t.key
		store := t.store
		filePath := t.filePath
		t.mu.Unlock()

		if shouldSave && store != nil && key != "" {
			store.Set(&Cursor{
				Key:        key,
				Path:       filePath,
				Offset:     bytesReadTotal,
				Size:       fi.Size(),
				Inode:      ino,
				Device:     dev,
				PrefixHash: prefixHash,
				UpdatedAt:  time.Now().UnixMilli(),
			})
			_ = store.Save()
		}
	}

	return nil
}
