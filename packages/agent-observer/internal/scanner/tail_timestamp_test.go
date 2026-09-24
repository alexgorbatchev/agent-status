package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractLastActiveTimestamp(t *testing.T) {
	t.Run("non-existent file returns error", func(t *testing.T) {
		_, err := ExtractLastActiveTimestamp("/path/does/not/exist/transcript.jsonl")
		if err == nil {
			t.Fatal("expected error for non-existent file, got nil")
		}
	})

	t.Run("empty file falls back to mtime", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "empty.jsonl")
		if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
			t.Fatalf("writing empty file: %v", err)
		}

		fi, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat empty file: %v", err)
		}
		expectedMtime := fi.ModTime().UnixMilli()

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != expectedMtime {
			t.Fatalf("ExtractLastActiveTimestamp(empty) = %d, want mtime %d", got, expectedMtime)
		}
	})

	t.Run("non-JSON file falls back to mtime", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "plain.log")
		content := "This is a plain text file without any JSON\nSecond line of plain text\n"
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("writing non-JSON file: %v", err)
		}

		fi, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat non-JSON file: %v", err)
		}
		expectedMtime := fi.ModTime().UnixMilli()

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != expectedMtime {
			t.Fatalf("ExtractLastActiveTimestamp(plain) = %d, want mtime %d", got, expectedMtime)
		}
	})

	t.Run("file with invalid timestamps falls back to mtime", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "invalid_ts.jsonl")
		content := strings.Join([]string{
			`{"timestamp":"not-a-valid-date"}`,
			`{"timestamp":null}`,
			`{"timestamp":-100}`,
			`{"type":"last-prompt","content":"no timestamp field"}`,
		}, "\n") + "\n"
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("writing invalid timestamp file: %v", err)
		}

		fi, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat invalid timestamp file: %v", err)
		}
		expectedMtime := fi.ModTime().UnixMilli()

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != expectedMtime {
			t.Fatalf("ExtractLastActiveTimestamp(invalid) = %d, want mtime %d", got, expectedMtime)
		}
	})

	t.Run("multi-line JSONL extracts latest tail entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "multiline.jsonl")

		t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
		t2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC).UnixMilli()
		t3 := time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC).UnixMilli()

		content := fmt.Sprintf(
			"{\"timestamp\":%d}\n{\"timestamp\":%d}\n{\"timestamp\":%d}\n",
			t1, t2, t3,
		)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("writing multiline file: %v", err)
		}

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != t3 {
			t.Fatalf("ExtractLastActiveTimestamp(multiline) = %d, want latest %d", got, t3)
		}
	})

	t.Run("multi-line JSONL with timestamp-less last line extracts preceding valid line", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "multiline_last_prompt.jsonl")

		t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
		t2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC).UnixMilli()

		content := fmt.Sprintf(
			"{\"timestamp\":%d}\n{\"timestamp\":%d}\n{\"type\":\"last-prompt\",\"content\":\"nav pointer\"}\n",
			t1, t2,
		)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("writing file: %v", err)
		}

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != t2 {
			t.Fatalf("ExtractLastActiveTimestamp = %d, want %d", got, t2)
		}
	})

	t.Run("truncated last line falls back to preceding valid line", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "crashed_write.jsonl")

		expectedTS := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixMilli()
		content := fmt.Sprintf("{\"timestamp\":%d}\n{\"type\":\"assistant\",\"message\":{\"role\":\"assist", expectedTS)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("writing crashed write file: %v", err)
		}

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != expectedTS {
			t.Fatalf("ExtractLastActiveTimestamp = %d, want %d", got, expectedTS)
		}
	})

	t.Run("ISO-8601 timestamps parsed correctly", func(t *testing.T) {
		tests := []struct {
			name      string
			tsString  string
			wantMilli int64
		}{
			{
				name:      "RFC3339Nano UTC",
				tsString:  "2026-09-18T20:53:30.007Z",
				wantMilli: time.Date(2026, 9, 18, 20, 53, 30, 7000000, time.UTC).UnixMilli(),
			},
			{
				name:      "RFC3339 with offset",
				tsString:  "2026-09-18T22:53:30.007+02:00",
				wantMilli: time.Date(2026, 9, 18, 20, 53, 30, 7000000, time.UTC).UnixMilli(),
			},
			{
				name:      "RFC3339 standard Z",
				tsString:  "2026-09-18T20:53:30Z",
				wantMilli: time.Date(2026, 9, 18, 20, 53, 30, 0, time.UTC).UnixMilli(),
			},
			{
				name:      "ISO format without offset with millis",
				tsString:  "2026-09-18T20:53:30.007",
				wantMilli: time.Date(2026, 9, 18, 20, 53, 30, 7000000, time.UTC).UnixMilli(),
			},
			{
				name:      "ISO format without offset",
				tsString:  "2026-09-18T20:53:30",
				wantMilli: time.Date(2026, 9, 18, 20, 53, 30, 0, time.UTC).UnixMilli(),
			},
			{
				name:      "standard DateTime",
				tsString:  "2026-09-18 20:53:30",
				wantMilli: time.Date(2026, 9, 18, 20, 53, 30, 0, time.UTC).UnixMilli(),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "iso.jsonl")
				content := fmt.Sprintf("{\"timestamp\":%q}\n", tt.tsString)
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					t.Fatalf("writing file: %v", err)
				}

				got, err := ExtractLastActiveTimestamp(filePath)
				if err != nil {
					t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
				}
				if got != tt.wantMilli {
					t.Fatalf("ExtractLastActiveTimestamp(%s) = %d, want %d", tt.tsString, got, tt.wantMilli)
				}
			})
		}
	})

	t.Run("unix epoch milliseconds and seconds", func(t *testing.T) {
		tests := []struct {
			name      string
			line      string
			wantMilli int64
		}{
			{
				name:      "milliseconds numeric",
				line:      `{"timestamp":1789851210007}`,
				wantMilli: 1789851210007,
			},
			{
				name:      "seconds numeric",
				line:      `{"timestamp":1789851210}`,
				wantMilli: 1789851210000,
			},
			{
				name:      "milliseconds string",
				line:      `{"timestamp":"1789851210007"}`,
				wantMilli: 1789851210007,
			},
			{
				name:      "seconds string",
				line:      `{"timestamp":"1789851210"}`,
				wantMilli: 1789851210000,
			},
			{
				name:      "created_at field",
				line:      `{"created_at":1789851210007}`,
				wantMilli: 1789851210007,
			},
			{
				name:      "createdAt field",
				line:      `{"createdAt":"2026-09-18T20:53:30.007Z"}`,
				wantMilli: time.Date(2026, 9, 18, 20, 53, 30, 7000000, time.UTC).UnixMilli(),
			},
			{
				name:      "time field numeric",
				line:      `{"time":1789851210007}`,
				wantMilli: 1789851210007,
			},
			{
				name:      "nested payload timestamp",
				line:      `{"type":"session_meta","payload":{"timestamp":"2026-06-12T16:08:36.123Z"}}`,
				wantMilli: time.Date(2026, 6, 12, 16, 8, 36, 123000000, time.UTC).UnixMilli(),
			},
			{
				name:      "nested time object created",
				line:      `{"type":"message","time":{"created":1789851210007}}`,
				wantMilli: 1789851210007,
			},
			{
				name:      "microseconds numeric",
				line:      `{"timestamp":1789851210007000}`,
				wantMilli: 1789851210007,
			},
			{
				name:      "nanoseconds numeric",
				line:      `{"timestamp":1789851210007000000}`,
				wantMilli: 1789851210007,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "epoch.jsonl")
				if err := os.WriteFile(filePath, []byte(tt.line+"\n"), 0644); err != nil {
					t.Fatalf("writing file: %v", err)
				}

				got, err := ExtractLastActiveTimestamp(filePath)
				if err != nil {
					t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
				}
				if got != tt.wantMilli {
					t.Fatalf("ExtractLastActiveTimestamp = %d, want %d", got, tt.wantMilli)
				}
			})
		}
	})

	t.Run("truncated lines at 256KB buffer boundary", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "large_boundary.jsonl")

		// Construct a file > 256KB where a line crosses the 256KB tail boundary.
		// Total size around 300KB.
		// 256KB = 262144 bytes.
		// Head: 40KB of padding lines.
		// Crossing line: starts before byte 40000, spans across byte 40000 to 50000.
		// Followed by valid lines in the tail.
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("creating file: %v", err)
		}

		// Write 50KB of prefix lines with an early timestamp
		earlyTS := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		prefixLine := fmt.Sprintf("{\"timestamp\":%d,\"padding\":\"%s\"}\n", earlyTS, strings.Repeat("A", 1000))
		for i := 0; i < 50; i++ {
			if _, err := f.WriteString(prefixLine); err != nil {
				t.Fatalf("writing prefix: %v", err)
			}
		}

		// Write padding bytes to position exactly near 256KB boundary from end
		// Write 300KB of lines
		midTS := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		midLine := fmt.Sprintf("{\"timestamp\":%d,\"padding\":\"%s\"}\n", midTS, strings.Repeat("B", 2000))
		for i := 0; i < 150; i++ {
			if _, err := f.WriteString(midLine); err != nil {
				t.Fatalf("writing mid: %v", err)
			}
		}

		// Now add the final line with the latest timestamp
		expectedLatest := time.Date(2026, 3, 1, 15, 30, 0, 0, time.UTC).UnixMilli()
		finalLine := fmt.Sprintf("{\"timestamp\":%d,\"msg\":\"final event\"}\n", expectedLatest)
		if _, err := f.WriteString(finalLine); err != nil {
			t.Fatalf("writing final line: %v", err)
		}
		_ = f.Close()

		fi, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if fi.Size() <= 256*1024 {
			t.Fatalf("file size %d <= 256KB, need > 256KB for boundary test", fi.Size())
		}

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != expectedLatest {
			t.Fatalf("ExtractLastActiveTimestamp(boundary) = %d, want latest %d", got, expectedLatest)
		}
	})

	t.Run("large file with single truncated line in tail buffer falls back to mtime", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "single_huge_line.jsonl")

		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("creating file: %v", err)
		}

		// Write a 300KB file consisting of a single line without any newlines
		// The 256KB tail buffer will therefore only see a truncated segment without newlines.
		chunk := strings.Repeat("X", 300*1024)
		if _, err := f.WriteString(chunk); err != nil {
			t.Fatalf("writing single huge line: %v", err)
		}
		_ = f.Close()

		fi, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		expectedMtime := fi.ModTime().UnixMilli()

		got, err := ExtractLastActiveTimestamp(filePath)
		if err != nil {
			t.Fatalf("ExtractLastActiveTimestamp failed: %v", err)
		}
		if got != expectedMtime {
			t.Fatalf("ExtractLastActiveTimestamp(huge line) = %d, want mtime %d", got, expectedMtime)
		}
	})

	t.Run("parseTimestampValue unit variations", func(t *testing.T) {
		if ts, ok := parseTimestampValue(nil); ok || ts != 0 {
			t.Errorf("expected nil to return 0, false; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(int64(1789851210007)); !ok || ts != 1789851210007 {
			t.Errorf("expected int64 to return 1789851210007, true; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(int64(-50)); ok || ts != 0 {
			t.Errorf("expected negative int64 to return 0, false; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(int(1789851210)); !ok || ts != 1789851210000 {
			t.Errorf("expected int to return 1789851210000, true; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(json.Number("1789851210007")); !ok || ts != 1789851210007 {
			t.Errorf("expected json.Number to return 1789851210007, true; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(json.Number("invalid")); ok || ts != 0 {
			t.Errorf("expected invalid json.Number to return 0, false; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(true); ok || ts != 0 {
			t.Errorf("expected bool to return 0, false; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(""); ok || ts != 0 {
			t.Errorf("expected empty string to return 0, false; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(float64(50)); ok || ts != 0 {
			t.Errorf("expected small float to return 0, false; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue(float64(-100)); ok || ts != 0 {
			t.Errorf("expected negative float to return 0, false; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue("  2026-09-18T20:53:30Z  "); !ok || ts != 1789764810000 {
			t.Errorf("expected trimmed RFC3339 string to return 1789764810000, true; got %d, %v", ts, ok)
		}
		if ts, ok := parseTimestampValue("1789851210007.5"); !ok || ts != 1789851210007 {
			t.Errorf("expected float string to return 1789851210007, true; got %d, %v", ts, ok)
		}
	})
}
