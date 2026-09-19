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

const maxScanTokenSize = 16 * 1024 * 1024 // 16MB buffer

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
	if err := t.readAvailable(); err != nil {
		// Log or return error if severe, but continue watching
	}

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
		case err, ok := <-t.watcher.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				// Continue on watcher errors
			}
		case <-ticker.C:
			_ = t.readAvailable()
		}
	}
}

func (t *Tailer) Stop() {
	t.once.Do(func() {
		close(t.stopCh)
		_ = t.watcher.Close()
	})
}

func (t *Tailer) readAvailable() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	f, err := os.Open(t.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

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
	currOffset := offset

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			currOffset += int64(len(line))
			cleanLine := bytes.TrimRight(line, "\r\n")
			if len(cleanLine) > 0 && t.onLine != nil {
				if err := t.onLine(cleanLine); err != nil {
					// handler error
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	t.offset = currOffset
	cursor := &Cursor{
		Key:        key,
		Path:       t.filePath,
		Offset:     currOffset,
		Size:       fi.Size(),
		Inode:      ino,
		Device:     dev,
		PrefixHash: prefixHash,
		UpdatedAt:  time.Now().UnixMilli(),
	}
	t.store.Set(cursor)
	_ = t.store.Save()

	return nil
}
