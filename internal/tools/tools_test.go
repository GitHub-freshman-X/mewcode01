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

func TestRegistryRebindWorkspaceIsolatesLocalTools(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "same.txt"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "same.txt"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := NewDefaultRegistry(first)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := NewWorkspace(second)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = registry.RebindWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	result := runTool(t, registry, "read_file", map[string]any{"path": "same.txt"}, true)
	if result.Data["content"] != "second" {
		t.Fatalf("content=%q", result.Data["content"])
	}
}

func TestFindFilesAcceptsAbsolutePatternsAndReturnsDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".mewcode"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	result := runTool(t, registry, "find_files", map[string]any{"pattern": filepath.Join(root, ".mewcode", "**")}, true)
	matches, ok := result.Data["matches"].([]string)
	if !ok || len(matches) != 1 || matches[0] != ".mewcode" {
		t.Fatalf("matches=%#v", result.Data["matches"])
	}
}

func TestSafetyAndFilterBySafety(t *testing.T) {
	registry, err := NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	readOnly := registry.FilterBySafety(SafetyReadOnly)
	if len(readOnly.List()) != 3 || len(registry.List()) != 6 {
		t.Fatalf("read=%d all=%d", len(readOnly.List()), len(registry.List()))
	}
	for _, name := range []string{"read_file", "find_files", "search_code"} {
		tool, ok := readOnly.Get(name)
		if !ok || NormalizeSafety(tool.Metadata().Safety) != SafetyReadOnly {
			t.Fatalf("missing read-only %s", name)
		}
	}
	for _, name := range []string{"write_file", "edit_file", "run_command"} {
		if _, ok := readOnly.Get(name); ok {
			t.Fatalf("side effect leaked: %s", name)
		}
	}
	if NormalizeSafety("") != SafetySideEffect || NormalizeSafety("other") != SafetySideEffect {
		t.Fatal("unknown safety is not conservative")
	}
}

func TestCoreToolsDeclarePermissionMetadata(t *testing.T) {
	registry, err := NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		target PermissionTarget
		paths  []string
	}{
		"read_file":   {target: PermissionTargetPath, paths: []string{"path"}},
		"write_file":  {target: PermissionTargetPath, paths: []string{"path"}},
		"edit_file":   {target: PermissionTargetPath, paths: []string{"path"}},
		"run_command": {target: PermissionTargetCommand},
		"find_files":  {target: PermissionTargetPattern},
		"search_code": {target: PermissionTargetPattern},
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			tool, ok := registry.Get(name)
			if !ok {
				t.Fatalf("missing tool %s", name)
			}
			got := tool.Metadata().Permission
			if got.Target != want.target {
				t.Fatalf("target=%q want %q", got.Target, want.target)
			}
			if strings.Join(got.PathParams, ",") != strings.Join(want.paths, ",") {
				t.Fatalf("path params=%v want %v", got.PathParams, want.paths)
			}
		})
	}
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
	executor := NewExecutor(10 * time.Millisecond)
	result := executor.Execute(context.Background(), registry, provider.ToolCall{ID: "call-1", Name: "missing", Arguments: []byte("{}")})
	if !result.IsError || !strings.Contains(result.Content, string(ErrorNotFound)) {
		t.Fatalf("result=%+v", result)
	}

	root := t.TempDir()
	defaultRegistry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	executor = NewExecutor(20 * time.Millisecond)
	command, args := commandForSleep()
	result = executor.Execute(context.Background(), defaultRegistry, provider.ToolCall{ID: "call-2", Name: "run_command", Arguments: mustJSON(map[string]any{"command": command, "args": args, "timeout_ms": 10})})
	if !strings.Contains(result.Content, `"timed_out":true`) {
		t.Fatalf("result=%+v", result)
	}
}

type panicTool struct{}

func (panicTool) Metadata() Metadata {
	return Metadata{Name: "panic", Description: "panic test", Schema: Schema{"type": "object"}}
}
func (panicTool) Execute(context.Context, json.RawMessage) Result { panic("boom") }

type delayedAgentTool struct{}

func (delayedAgentTool) Metadata() Metadata {
	return Metadata{Name: "agent", Description: "agent timeout exemption test", Schema: Schema{"type": "object"}}
}
func (delayedAgentTool) Execute(context.Context, json.RawMessage) Result {
	time.Sleep(30 * time.Millisecond)
	return Success("agent", nil)
}

func TestExecutorDoesNotApplyGeneralTimeoutToAgent(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(delayedAgentTool{}); err != nil {
		t.Fatal(err)
	}
	result := NewExecutor(5*time.Millisecond).Execute(context.Background(), registry, provider.ToolCall{ID: "agent-call", Name: "agent", Arguments: []byte(`{}`)})
	if result.IsError {
		t.Fatalf("agent inherited general executor timeout: %+v", result)
	}
}

func TestRunCommandTimeoutSchema(t *testing.T) {
	root := t.TempDir()
	registry, err := NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := registry.Get("run_command")
	if !ok {
		t.Fatal("missing run_command tool")
	}
	props, ok := tool.Metadata().Schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %+v", tool.Metadata().Schema)
	}
	timeoutSchema, ok := props["timeout_ms"].(map[string]any)
	if !ok {
		t.Fatalf("timeout_ms schema = %+v", props)
	}
	if got := timeoutSchema["maximum"]; got != float64(600_000) {
		t.Fatalf("timeout_ms maximum = %v, want 600000", got)
	}
	longOK := map[string]any{"command": "true", "timeout_ms": 300_000}
	if _, verr := Validate(tool.Metadata().Schema, mustJSON(longOK)); verr != nil {
		t.Fatalf("300000ms should be valid: %+v", verr)
	}
	overCap := map[string]any{"command": "true", "timeout_ms": 600_001}
	if _, verr := Validate(tool.Metadata().Schema, mustJSON(overCap)); verr == nil {
		t.Fatal("600001ms should be rejected")
	}
}

func TestExecutorPanic(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(panicTool{}); err != nil {
		t.Fatal(err)
	}
	result := NewExecutor(time.Second).Execute(context.Background(), registry, provider.ToolCall{ID: "1", Name: "panic", Arguments: []byte(`{}`)})
	if !result.IsError || !strings.Contains(result.Content, string(ErrorInternal)) {
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
