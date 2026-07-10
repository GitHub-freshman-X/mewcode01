package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func TestBuildRequestForPathTargetUsesRelativePath(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry, sandbox := requestTestRegistry(t, root)
	tool, _ := registry.Get("read_file")
	req, err := BuildRequest(provider.ToolCall{
		ID:        "call-1",
		Name:      "read_file",
		Arguments: mustJSONBytes(map[string]any{"path": filepath.Join(root, "docs", "README.md")}),
	}, tool, sandbox)
	if err != nil {
		t.Fatalf("BuildRequest returned error: %v", err)
	}
	if req.CallID != "call-1" || req.Tool != "read_file" || req.MatchTarget != "docs/README.md" {
		t.Fatalf("unexpected request: %#v", req)
	}
	if len(req.Paths) != 1 || req.Paths[0].Relative != "docs/README.md" {
		t.Fatalf("unexpected path checks: %#v", req.Paths)
	}
}

func TestBuildRequestForCommandTargetNormalizesCommandText(t *testing.T) {
	registry, sandbox := requestTestRegistry(t, t.TempDir())
	tool, _ := registry.Get("run_command")
	req, err := BuildRequest(provider.ToolCall{
		ID:        "call-2",
		Name:      "run_command",
		Arguments: mustJSONBytes(map[string]any{"command": "git", "args": []any{"status", "--short"}}),
	}, tool, sandbox)
	if err != nil {
		t.Fatalf("BuildRequest returned error: %v", err)
	}
	if req.MatchTarget != "git status --short" {
		t.Fatalf("match target=%q", req.MatchTarget)
	}
}

func TestBuildRequestForPatternTarget(t *testing.T) {
	registry, sandbox := requestTestRegistry(t, t.TempDir())
	tool, _ := registry.Get("find_files")
	req, err := BuildRequest(provider.ToolCall{
		ID:        "call-3",
		Name:      "find_files",
		Arguments: mustJSONBytes(map[string]any{"pattern": "docs/**"}),
	}, tool, sandbox)
	if err != nil {
		t.Fatalf("BuildRequest returned error: %v", err)
	}
	if req.MatchTarget != "docs/**" {
		t.Fatalf("match target=%q", req.MatchTarget)
	}
}

func TestSuggestedRuleKey(t *testing.T) {
	req := Request{Tool: "run_command", MatchTarget: "git status"}
	if got := SuggestedRuleKey(req); got != "run_command(git status)" {
		t.Fatalf("SuggestedRuleKey=%q", got)
	}
}

func TestBuildRequestRejectsInvalidJSON(t *testing.T) {
	registry, sandbox := requestTestRegistry(t, t.TempDir())
	tool, _ := registry.Get("run_command")
	if _, err := BuildRequest(provider.ToolCall{Name: "run_command", Arguments: []byte("{")}, tool, sandbox); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func requestTestRegistry(t *testing.T, root string) (*tools.Registry, Sandbox) {
	t.Helper()
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := NewSandbox(root)
	if err != nil {
		t.Fatal(err)
	}
	return registry, sandbox
}

func mustJSONBytes(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
