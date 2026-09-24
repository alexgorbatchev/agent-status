package scanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const tailScanLimit = 256 * 1024 // 256KB

var (
	primaryTimestampFields = []string{
		"timestamp",
		"created_at",
		"createdAt",
		"time",
		"date",
		"started_at",
		"startedAt",
	}

	containerFields = []string{
		"payload",
		"data",
		"message",
		"time",
	}

	nestedTimestampFields = []string{
		"timestamp",
		"created_at",
		"createdAt",
		"created",
		"time",
		"date",
		"started_at",
		"startedAt",
		"start",
		"completed",
	}
)

// ExtractLastActiveTimestamp reads the tail of a transcript file (up to 256KB),
// scanning backwards for valid JSON lines carrying a recognizable timestamp.
// If no valid timestamp can be extracted, it falls back to the file's ModTime().
func ExtractLastActiveTimestamp(filePath string) (int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("opening transcript file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	stat, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat transcript file: %w", err)
	}

	mtime := stat.ModTime().UnixMilli()
	fileSize := stat.Size()
	if fileSize == 0 {
		return mtime, nil
	}

	tailSize := int64(tailScanLimit)
	offset := int64(0)
	if fileSize > tailSize {
		offset = fileSize - tailSize
	}

	buf := make([]byte, fileSize-offset)
	n, err := file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return 0, fmt.Errorf("reading transcript tail: %w", err)
	}
	buf = buf[:n]

	lines := bytes.Split(buf, []byte("\n"))
	startIdx := 0
	if offset > 0 {
		// When seeking into the middle of a file, the first slice before the first \n
		// is truncated at the buffer boundary.
		startIdx = 1
	}

	for i := len(lines) - 1; i >= startIdx; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if ts, ok := extractTimestampFromMap(entry); ok {
			return ts, nil
		}
	}

	return mtime, nil
}

func extractTimestampFromMap(entry map[string]interface{}) (int64, bool) {
	for _, field := range primaryTimestampFields {
		if val, exists := entry[field]; exists {
			if ts, ok := parseTimestampValue(val); ok {
				return ts, true
			}
		}
	}

	for _, containerKey := range containerFields {
		if container, exists := entry[containerKey].(map[string]interface{}); exists {
			for _, field := range nestedTimestampFields {
				if val, exists := container[field]; exists {
					if ts, ok := parseTimestampValue(val); ok {
						return ts, true
					}
				}
			}
		}
	}

	return 0, false
}

func parseTimestampValue(v interface{}) (int64, bool) {
	if v == nil {
		return 0, false
	}

	switch val := v.(type) {
	case float64:
		return normalizeNumericTimestamp(val)
	case int64:
		return normalizeNumericTimestamp(float64(val))
	case int:
		return normalizeNumericTimestamp(float64(val))
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return normalizeNumericTimestamp(f)
		}
		return 0, false
	case string:
		val = strings.TrimSpace(val)
		if val == "" {
			return 0, false
		}

		if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
			if ts := t.UnixMilli(); ts > 0 {
				return ts, true
			}
		}
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			if ts := t.UnixMilli(); ts > 0 {
				return ts, true
			}
		}
		if t, err := time.Parse("2006-01-02T15:04:05.999999999", val); err == nil {
			if ts := t.UnixMilli(); ts > 0 {
				return ts, true
			}
		}
		if t, err := time.Parse("2006-01-02T15:04:05", val); err == nil {
			if ts := t.UnixMilli(); ts > 0 {
				return ts, true
			}
		}
		if t, err := time.Parse(time.DateTime, val); err == nil {
			if ts := t.UnixMilli(); ts > 0 {
				return ts, true
			}
		}

		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return normalizeNumericTimestamp(f)
		}
		return 0, false
	default:
		return 0, false
	}
}

func normalizeNumericTimestamp(v float64) (int64, bool) {
	if v <= 0 {
		return 0, false
	}
	if v >= 1e17 {
		return int64(v / 1e6), true
	}
	if v >= 1e14 {
		return int64(v / 1e3), true
	}
	if v >= 1e11 {
		return int64(v), true
	}
	if v >= 1e9 {
		return int64(v * 1000), true
	}
	return 0, false
}
