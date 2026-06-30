package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestDefaultRegistryAndCoreTools(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "code.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(registry.List()); got != 6 {
		t.Fatalf("tool count=%d", got)
	}
	for _, name := range []string{"read_file", "write_file", "edit_file", "run_command", "find_files", "search_code"} {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("missing tool %s", name)
		}
	}

	runTool(t, registry, "read_file", map[string]any{"path": "hello.txt"}, true)
	runTool(t, registry, "write_file", map[string]any{"path": "new.txt", "content": "new content"}, true)
	if got, err := os.ReadFile(filepath.Join(root, "new.txt")); err != nil || string(got) != "new content" {
		t.Fatalf("written content=%q err=%v", got, err)
	}
	runTool(t, registry, "write_file", map[string]any{"path": "test/test1/test2/test3.txt", "content": "Hello, world!"}, true)
	if got, err := os.ReadFile(filepath.Join(root, "test", "test1", "test2", "test3.txt")); err != nil || string(got) != "Hello, world!" {
		t.Fatalf("nested written content=%q err=%v", got, err)
	}
	runTool(t, registry, "edit_file", map[string]any{"path": "hello.txt", "old_text": "world", "new_text": "agent"}, true)
	if got, _ := os.ReadFile(filepath.Join(root, "hello.txt")); string(got) != "hello agent\n" {
		t.Fatalf("edited content=%q", got)
	}
	runTool(t, registry, "find_files", map[string]any{"pattern": "**/*.go", "limit": 10}, true)
	runTool(t, registry, "search_code", map[string]any{"pattern": "func main", "limit": 10}, true)

	command, args := commandForEcho()
	runTool(t, registry, "run_command", map[string]any{"command": command, "args": args}, true)
}

func TestToolValidationAndWorkspaceBoundaries(t *testing.T) {
	root := t.TempDir()
	registry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if result := runTool(t, registry, "read_file", map[string]any{}, false); result.Error.Type != ErrorValidation {
		t.Fatalf("error=%+v", result.Error)
	}
	if result := runTool(t, registry, "write_file", map[string]any{"path": "../escape.txt", "content": "x"}, false); result.Error.Type != ErrorPermission {
		t.Fatalf("error=%+v", result.Error)
	}
	if _, err := os.Stat(filepath.Join(root, "..", "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape file exists or stat err=%v", err)
	}
}

func TestEditFileRequiresUniqueMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dup.txt")
	if err := os.WriteFile(path, []byte("same same"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	result := runTool(t, registry, "edit_file", map[string]any{"path": "dup.txt", "old_text": "same", "new_text": "once"}, false)
	if result.Error.Type != ErrorConflict {
		t.Fatalf("error=%+v", result.Error)
	}
	if got, _ := os.ReadFile(path); string(got) != "same same" {
		t.Fatalf("file changed: %q", got)
	}
}

func TestExecutorUnknownAndTimeout(t *testing.T) {
	registry := NewRegistry()
	executor := NewExecutor(registry, 10*time.Millisecond)
	result := executor.Execute(context.Background(), provider.ToolCall{ID: "call-1", Name: "missing", Arguments: []byte("{}")})
	if !result.IsError || !strings.Contains(result.Content, string(ErrorNotFound)) {
		t.Fatalf("result=%+v", result)
	}

	root := t.TempDir()
	defaultRegistry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	executor = NewExecutor(defaultRegistry, 20*time.Millisecond)
	command, args := commandForSleep()
	result = executor.Execute(context.Background(), provider.ToolCall{ID: "call-2", Name: "run_command", Arguments: mustJSON(map[string]any{"command": command, "args": args, "timeout_ms": 10})})
	if !strings.Contains(result.Content, `"timed_out":true`) {
		t.Fatalf("result=%+v", result)
	}
}

func runTool(t *testing.T, registry *Registry, name string, args map[string]any, wantSuccess bool) Result {
	t.Helper()
	tool, ok := registry.Get(name)
	if !ok {
		t.Fatalf("missing tool %s", name)
	}
	result := tool.Execute(context.Background(), mustJSON(args))
	if result.Success != wantSuccess {
		t.Fatalf("%s success=%v error=%+v data=%+v", name, result.Success, result.Error, result.Data)
	}
	return result
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func commandForEcho() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "echo ok"}
	}
	return "printf", []string{"ok"}
}

func commandForSleep() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "ping -n 3 127.0.0.1 >NUL"}
	}
	return "sleep", []string{"1"}
}
