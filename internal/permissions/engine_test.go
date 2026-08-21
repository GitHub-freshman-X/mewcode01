package permissions

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func TestEngineDecideBlacklistCannotBeBypassed(t *testing.T) {
	engine, registry := testEngine(t, ModeRelaxed, RuleSet{
		Session: []Rule{mustRule(t, "run_command(*)", EffectAllow, ScopeSession, 0)},
	})
	tool, _ := registry.Get("run_command")
	decision, err := engine.Decide(context.Background(), provider.ToolCall{
		ID:        "danger",
		Name:      "run_command",
		Arguments: mustJSONBytes(map[string]any{"command": "rm", "args": []any{"-rf", "/"}}),
	}, tool)
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Action != ActionDeny || decision.Stage != StageBlacklist {
		t.Fatalf("expected blacklist deny, got %#v", decision)
	}
}

func TestEngineDecideUsesRulePriorityBeforeMode(t *testing.T) {
	engine, registry := testEngine(t, ModeStrict, RuleSet{
		User:    []Rule{mustRule(t, "run_command(git *)", EffectAllow, ScopeUser, 0)},
		Project: []Rule{mustRule(t, "run_command(git *)", EffectDeny, ScopeProject, 0)},
		Session: []Rule{mustRule(t, "run_command(git *)", EffectDeny, ScopeSession, 0)},
	})
	tool, _ := registry.Get("run_command")
	decision, err := engine.Decide(context.Background(), provider.ToolCall{
		ID:        "git",
		Name:      "run_command",
		Arguments: mustJSONBytes(map[string]any{"command": "git", "args": []any{"status"}}),
	}, tool)
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Action != ActionDeny || decision.Rule == nil || decision.Rule.Scope != ScopeSession {
		t.Fatalf("expected session deny, got %#v", decision)
	}
}

func TestEngineModesDefaultDecisions(t *testing.T) {
	root := t.TempDir()
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	readTool, _ := registry.Get("read_file")
	writeTool, _ := registry.Get("write_file")
	readCall := provider.ToolCall{Name: "read_file", Arguments: mustJSONBytes(map[string]any{"path": "README.md"})}
	writeCall := provider.ToolCall{Name: "write_file", Arguments: mustJSONBytes(map[string]any{"path": "README.md", "content": "x"})}

	tests := []struct {
		mode      Mode
		tool      tools.Tool
		call      provider.ToolCall
		want      Action
		wantStage Stage
	}{
		{mode: ModeStrict, tool: readTool, call: readCall, want: ActionAsk, wantStage: StageMode},
		{mode: ModeDefault, tool: readTool, call: readCall, want: ActionAllow, wantStage: StageMode},
		{mode: ModeDefault, tool: writeTool, call: writeCall, want: ActionAsk, wantStage: StageMode},
		{mode: ModeRelaxed, tool: writeTool, call: writeCall, want: ActionAllow, wantStage: StageMode},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode)+"_"+tt.call.Name, func(t *testing.T) {
			sandbox, err := NewSandbox(root)
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{Mode: tt.mode, Rules: NewRuleStore(RuleSet{}), Sandbox: sandbox}
			decision, err := engine.Decide(context.Background(), tt.call, tt.tool)
			if err != nil {
				t.Fatalf("Decide returned error: %v", err)
			}
			if decision.Action != tt.want || decision.Stage != tt.wantStage {
				t.Fatalf("decision=%#v want action=%s stage=%s", decision, tt.want, tt.wantStage)
			}
		})
	}
}

func TestEngineApplyConfirmationAddsSessionAndPermanentAllow(t *testing.T) {
	dir := t.TempDir()
	paths := FilePaths{Project: filepath.Join(dir, ".mewcode", "permissions.yaml")}
	engine := &Engine{Rules: NewRuleStore(RuleSet{}), Paths: paths}
	decision := Decision{Request: Request{Tool: "write_file", MatchTarget: "tmp/out.txt"}, SuggestedKey: "write_file(tmp/out.txt)"}

	if err := engine.ApplyConfirmation(Confirmation{Decision: decision, Choice: ChoiceAllowSession}); err != nil {
		t.Fatalf("ApplyConfirmation session returned error: %v", err)
	}
	rule, ok, err := engine.Rules.Find(Request{Tool: "write_file", MatchTarget: "tmp/out.txt"})
	if err != nil || !ok || rule.Scope != ScopeSession || rule.Effect != EffectAllow {
		t.Fatalf("session rule=%#v ok=%v err=%v", rule, ok, err)
	}

	if err := engine.ApplyConfirmation(Confirmation{Decision: decision, Choice: ChoiceAllowPermanent}); err != nil {
		t.Fatalf("ApplyConfirmation permanent returned error: %v", err)
	}
	rules, err := LoadRuleSet(paths)
	if err != nil {
		t.Fatalf("LoadRuleSet returned error: %v", err)
	}
	if len(rules.Project) != 1 || rules.Project[0].Effect != EffectAllow {
		t.Fatalf("unexpected project rules: %#v", rules.Project)
	}
}

func testEngine(t *testing.T, mode Mode, rules RuleSet) (*Engine, *tools.Registry) {
	t.Helper()
	root := t.TempDir()
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := NewSandbox(root)
	if err != nil {
		t.Fatal(err)
	}
	return &Engine{Mode: mode, Rules: NewRuleStore(rules), Sandbox: sandbox}, registry
}
