package provider

import "testing"

func TestPromptBundleCarriesStableDynamicAndCachePolicy(t *testing.T) {
	bundle := PromptBundle{
		StableSystem: "stable rules",
		DynamicSystem: []SystemMessage{
			{Tag: "mew.environment", Content: "workspace=/tmp/project"},
		},
		CachePolicy: CachePolicy{Enable: true, StableSystem: true, StableTools: true},
	}

	req := ChatRequest{
		Prompt: bundle,
		Tools: []ToolDefinition{
			{Name: "read_file", Description: "Read a file", Cacheable: true},
		},
	}

	if req.Prompt.StableSystem != "stable rules" {
		t.Fatalf("stable system not carried: %q", req.Prompt.StableSystem)
	}
	if got := req.Prompt.DynamicSystem[0].Tag; got != "mew.environment" {
		t.Fatalf("dynamic tag = %q, want mew.environment", got)
	}
	if !req.Prompt.CachePolicy.Enable || !req.Prompt.CachePolicy.StableSystem || !req.Prompt.CachePolicy.StableTools {
		t.Fatalf("cache policy not carried: %+v", req.Prompt.CachePolicy)
	}
	if !req.Tools[0].Cacheable {
		t.Fatalf("tool cacheable flag not carried")
	}
}

func TestUsageAddIncludesCacheFields(t *testing.T) {
	total := Usage{InputTokens: 10, OutputTokens: 2, CacheReadInputTokens: 5}

	total.Add(Usage{
		InputTokens:              3,
		OutputTokens:             4,
		CacheReadInputTokens:     7,
		CacheCreationInputTokens: 11,
		CacheUnavailable:         true,
	})

	if total.InputTokens != 13 || total.OutputTokens != 6 {
		t.Fatalf("normal tokens = (%d,%d), want (13,6)", total.InputTokens, total.OutputTokens)
	}
	if total.CacheReadInputTokens != 12 {
		t.Fatalf("cache read tokens = %d, want 12", total.CacheReadInputTokens)
	}
	if total.CacheCreationInputTokens != 11 {
		t.Fatalf("cache creation tokens = %d, want 11", total.CacheCreationInputTokens)
	}
	if !total.CacheUnavailable {
		t.Fatalf("cache unavailable should be preserved when either side reports it")
	}
}

func TestUsageCacheHitRate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage Usage
		rate  int
		ok    bool
	}{
		{name: "claude", usage: Usage{InputTokens: 10, CacheReadInputTokens: 20, CacheCreationInputTokens: 5}, rate: 57, ok: true},
		{name: "openai", usage: Usage{InputTokens: 6222, CacheReadInputTokens: 4118, CacheTokensIncludedInInput: 4118}, rate: 66, ok: true},
		{name: "zero", usage: Usage{}, ok: false},
		{name: "unknown", usage: Usage{CacheReadInputTokens: 20, CacheUnavailable: true}, ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rate, ok := tc.usage.CacheHitRate()
			if rate != tc.rate || ok != tc.ok {
				t.Fatalf("CacheHitRate()=(%d,%t), want (%d,%t)", rate, ok, tc.rate, tc.ok)
			}
		})
	}
}
