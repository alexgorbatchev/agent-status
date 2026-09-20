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

type Tailer struct {
	filePath string
	store    *CursorStore
	onLine   LineHandler

	watcher *fsnotify.Watcher
	stopCh  chan struct{}
	once    sync.Once
	mu      sync.Mutex
	offset  int64
	key     string
}

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
		filePath: absPath,
		store:    store,
		onLine:   onLine,
		watcher:  watcher,
		stopCh:   make(chan struct{}),
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

func (t *Tailer) CurrentOffset() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.offset
}

func (t *Tailer) readAvailable() error {
	t.mu.Lock()
	defer t.mu.Unlock()

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
	t.key = key

	prefixHash, _ := ComputePrefixHash(f, fi.Size())

	offset := int64(0)
	if c, ok := t.store.Get(key); ok {
		if ShouldResetCursor(c, ino, fi.Size(), prefixHash) {
			offset = 0
		} else {
			offset = c.Offset
		}
	}

	if offset > fi.Size() {
		offset = 0
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seeking to offset %d: %w", offset, err)
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	bytesReadTotal := offset

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			// Only process complete lines (ending with '\n')
			if line[len(line)-1] == '\n' {
				bytesReadTotal += int64(len(line))
				cleanLine := bytes.TrimRight(line, "\r\n")
				if len(cleanLine) > 0 && t.onLine != nil {
					_ = t.onLine(cleanLine)
				}
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

	t.offset = bytesReadTotal
	t.store.Set(&Cursor{
		Key:        t.key,
		Path:       t.filePath,
		Offset:     bytesReadTotal,
		Size:       fi.Size(),
		Inode:      ino,
		Device:     dev,
		PrefixHash: prefixHash,
		UpdatedAt:  time.Now().UnixMilli(),
	})

	return nil
}
