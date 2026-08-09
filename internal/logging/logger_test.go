package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewWritesJSONL(t *testing.T) {
	root := t.TempDir()
	logger, err := New(root, func() time.Time { return time.Date(2026, 8, 6, 10, 0, 0, 123, time.UTC) }, 42)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("mcp", "tool_registered", "MCP tool registered", Fields{"server": "filesystem", "tool": "filesystem__read_text_file"})
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*", "mewcode-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("log files=%v", files)
	}
	contents, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var event Event
	if err := json.Unmarshal(contents, &event); err != nil {
		t.Fatal(err)
	}
	if event.Time.IsZero() || event.Level != "info" || event.Component != "mcp" || event.Event != "tool_registered" || event.Message != "MCP tool registered" {
		t.Fatalf("event=%+v", event)
	}
	if event.Fields["server"] != "filesystem" || event.Fields["tool"] != "filesystem__read_text_file" {
		t.Fatalf("fields=%v", event.Fields)
	}
	if !strings.Contains(event.Source, "logger_test.go:") || filepath.IsAbs(event.Source) {
		t.Fatalf("source=%q", event.Source)
	}
}

func TestNewUsesDateDirectory(t *testing.T) {
	root := t.TempDir()
	now := func() time.Time { return time.Date(2026, 8, 6, 10, 0, 0, 0, time.Local) }
	logger, err := New(root, now, 42)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "logs", "2026", "08", "06", "mewcode-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%v", files)
	}
}

func TestLoggerConcurrentDerivedWrites(t *testing.T) {
	root := t.TempDir()
	logger, err := New(root, time.Now, 42)
	if err != nil {
		t.Fatal(err)
	}
	const writers = 20
	const writesPerWriter = 10
	var group sync.WaitGroup
	for i := 0; i < writers; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			child := logger.WithFields(Fields{"writer": i})
			for j := 0; j < writesPerWriter; j++ {
				child.Info("mcp", "tool_registered", "MCP tool registered", nil)
			}
		}(i)
	}
	group.Wait()
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) != writers*writesPerWriter {
		t.Fatalf("line count=%d", len(lines))
	}
	for _, line := range lines {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("invalid line %q: %v", line, err)
		}
	}
}

func TestNop(t *testing.T) {
	logger := Nop().WithFields(Fields{"server": "filesystem"})
	logger.Info("mcp", "tool_registered", "MCP tool registered", nil)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
}
