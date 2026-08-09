package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
)

func TestStdioTransportSendsAndReceivesJSONLines(t *testing.T) {
	transport := NewStdioTransport("cat", nil, nil)
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer transport.Close(context.Background())
	if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case inbound := <-transport.Receive():
		if inbound.Err != nil || string(inbound.Message) != `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` {
			t.Fatalf("inbound=%+v", inbound)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stdout line")
	}
}

func TestStdioTransportLogsStderr(t *testing.T) {
	root := t.TempDir()
	logger, err := logging.New(root, time.Now, 42)
	if err != nil {
		t.Fatal(err)
	}
	transport := NewStdioTransport("sh", []string{"-c", "printf 'startup-failure\\nAuthorization: Bearer SECRET_VALUE\\nstack-line\\n' >&2; cat"}, nil, logger)
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := transport.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
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
	if !strings.Contains(string(contents), "startup-failure") || !strings.Contains(string(contents), "stack-line") {
		t.Fatalf("logs=%s", contents)
	}
	if strings.Count(string(contents), `"message":"MCP server stderr"`) != 1 {
		t.Fatalf("expected one stderr event, logs=%s", contents)
	}
	if strings.Contains(string(contents), "SECRET_VALUE") {
		t.Fatalf("stderr secret leaked into logs=%s", contents)
	}
}
