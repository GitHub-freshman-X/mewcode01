package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuleSetTreatsMissingFilesAsEmpty(t *testing.T) {
	paths := FilePaths{
		User:    filepath.Join(t.TempDir(), "user.yaml"),
		Project: filepath.Join(t.TempDir(), "project.yaml"),
	}
	rules, err := LoadRuleSet(paths)
	if err != nil {
		t.Fatalf("LoadRuleSet returned error: %v", err)
	}
	if len(rules.User) != 0 || len(rules.Project) != 0 {
		t.Fatalf("expected empty ruleset, got %#v", rules)
	}
}

func TestDefaultFilePathsUsesUserConfigAndProjectRoot(t *testing.T) {
	workspace := t.TempDir()
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := DefaultFilePaths(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(configDir, "mewcode", "permissions.yaml"); paths.User != want {
		t.Fatalf("user path = %q, want %q", paths.User, want)
	}
	if want := filepath.Join(workspace, ".mewcode", "permissions.yaml"); paths.Project != want {
		t.Fatalf("project path = %q, want %q", paths.Project, want)
	}
}

func TestLoadRuleSetLoadsUserAndProjectScopes(t *testing.T) {
	dir := t.TempDir()
	paths := FilePaths{
		User:    filepath.Join(dir, "user.yaml"),
		Project: filepath.Join(dir, "project.yaml"),
	}
	writeFile(t, paths.User, "rules:\n  run_command(git *): allow\n")
	writeFile(t, paths.Project, "rules:\n  run_command(git *): deny\n")

	rules, err := LoadRuleSet(paths)
	if err != nil {
		t.Fatalf("LoadRuleSet returned error: %v", err)
	}
	if rules.User[0].Scope != ScopeUser || rules.Project[0].Effect != EffectDeny {
		t.Fatalf("unexpected loaded scopes: %#v", rules)
	}
}

func TestRuleStoreFindsRulesByPriority(t *testing.T) {
	store := NewRuleStore(RuleSet{
		User:    []Rule{mustRule(t, "run_command(git *)", EffectAllow, ScopeUser, 0)},
		Project: []Rule{mustRule(t, "run_command(git *)", EffectDeny, ScopeProject, 0)},
	})
	store.AddSessionRule(mustRule(t, "run_command(git *)", EffectDeny, ScopeSession, 0))
	rule, ok, err := store.Find(Request{Tool: "run_command", MatchTarget: "git status"})
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if !ok || rule.Scope != ScopeSession || rule.Effect != EffectDeny {
		t.Fatalf("expected session deny, got %#v ok=%v", rule, ok)
	}

	store = NewRuleStore(RuleSet{
		User:    []Rule{mustRule(t, "run_command(git *)", EffectAllow, ScopeUser, 0)},
		Project: []Rule{mustRule(t, "run_command(git *)", EffectDeny, ScopeProject, 0)},
	})
	rule, ok, err = store.Find(Request{Tool: "run_command", MatchTarget: "git status"})
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if !ok || rule.Scope != ScopeProject || rule.Effect != EffectDeny {
		t.Fatalf("expected project deny, got %#v ok=%v", rule, ok)
	}
}

func TestLoadRuleSetRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid yaml", body: "rules: [\n"},
		{name: "unknown field", body: "rules: {}\nextra: true\n"},
		{name: "invalid effect", body: "rules:\n  run_command(git *): maybe\n"},
		{name: "invalid rule key", body: "rules:\n  run_command: allow\n"},
		{name: "invalid glob", body: "rules:\n  read_file([): allow\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			paths := FilePaths{User: filepath.Join(dir, "bad.yaml")}
			writeFile(t, paths.User, tt.body)
			if _, err := LoadRuleSet(paths); err == nil {
				t.Fatal("expected invalid config error")
			}
		})
	}
}

func TestAppendProjectAllowWritesReloadableRule(t *testing.T) {
	dir := t.TempDir()
	paths := FilePaths{Project: filepath.Join(dir, ".mewcode", "permissions.yaml")}
	rule := mustRule(t, "write_file(tmp/out.txt)", EffectAllow, ScopeProject, 0)
	if err := AppendProjectAllow(paths, rule); err != nil {
		t.Fatalf("AppendProjectAllow returned error: %v", err)
	}
	rules, err := LoadRuleSet(paths)
	if err != nil {
		t.Fatalf("LoadRuleSet returned error: %v", err)
	}
	if len(rules.Project) != 1 || rules.Project[0].Key != rule.Key || rules.Project[0].Effect != EffectAllow {
		t.Fatalf("unexpected project rules: %#v", rules.Project)
	}
}

func mustRule(t *testing.T, key string, effect Effect, scope Scope, index int) Rule {
	t.Helper()
	rule, err := ParseRule(key, effect, scope, index)
	if err != nil {
		t.Fatalf("ParseRule(%q) returned error: %v", key, err)
	}
	return rule
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
