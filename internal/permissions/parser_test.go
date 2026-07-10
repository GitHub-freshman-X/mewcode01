package permissions

import "testing"

func TestParseRuleAcceptsToolPatternAndEffect(t *testing.T) {
	rule, err := ParseRule("run_command(git *)", EffectAllow, ScopeProject, 3)
	if err != nil {
		t.Fatalf("ParseRule returned error: %v", err)
	}
	if rule.Key != "run_command(git *)" || rule.Tool != "run_command" || rule.Pattern != "git *" {
		t.Fatalf("unexpected rule: %#v", rule)
	}
	if rule.Effect != EffectAllow || rule.Scope != ScopeProject || rule.Index != 3 {
		t.Fatalf("unexpected metadata: %#v", rule)
	}
}

func TestParseRuleRejectsInvalidKeysAndEffects(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		effect Effect
	}{
		{name: "missing parens", key: "run_command", effect: EffectAllow},
		{name: "empty tool", key: "(git *)", effect: EffectAllow},
		{name: "empty pattern", key: "run_command()", effect: EffectAllow},
		{name: "invalid tool char", key: "run command(git *)", effect: EffectAllow},
		{name: "invalid effect", key: "run_command(git *)", effect: Effect("maybe")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseRule(tt.key, tt.effect, ScopeUser, 0); err == nil {
				t.Fatalf("ParseRule(%q, %q) returned nil error", tt.key, tt.effect)
			}
		})
	}
}

func TestMatchRuleUsesExactMatchWithoutGlobMeta(t *testing.T) {
	rule, err := ParseRule("read_file(docs/README.md)", EffectAllow, ScopeProject, 0)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := MatchRule(rule, Request{Tool: "read_file", MatchTarget: "docs/README.md"})
	if err != nil {
		t.Fatalf("MatchRule returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected exact target to match")
	}
	matched, err = MatchRule(rule, Request{Tool: "read_file", MatchTarget: "docs/README.md.bak"})
	if err != nil {
		t.Fatalf("MatchRule returned error: %v", err)
	}
	if matched {
		t.Fatal("exact rule matched a similar string")
	}
}

func TestMatchRuleUsesGlobMatchWithMeta(t *testing.T) {
	rule, err := ParseRule("run_command(git *)", EffectAllow, ScopeProject, 0)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := MatchRule(rule, Request{Tool: "run_command", MatchTarget: "git status"})
	if err != nil {
		t.Fatalf("MatchRule returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected glob target to match")
	}
	matched, err = MatchRule(rule, Request{Tool: "run_command", MatchTarget: "go test ./..."})
	if err != nil {
		t.Fatalf("MatchRule returned error: %v", err)
	}
	if matched {
		t.Fatal("glob rule matched unrelated command")
	}
}

func TestMatchRuleSupportsDoubleStarPaths(t *testing.T) {
	rule, err := ParseRule("read_file(docs/**)", EffectAllow, ScopeProject, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"docs/README.md", "docs/ch06/spec.md"} {
		matched, err := MatchRule(rule, Request{Tool: "read_file", MatchTarget: target})
		if err != nil {
			t.Fatalf("MatchRule returned error for %q: %v", target, err)
		}
		if !matched {
			t.Fatalf("expected %q to match docs/**", target)
		}
	}
	matched, err := MatchRule(rule, Request{Tool: "read_file", MatchTarget: "src/docs/file.md"})
	if err != nil {
		t.Fatalf("MatchRule returned error: %v", err)
	}
	if matched {
		t.Fatal("docs/** matched path outside docs")
	}
}
