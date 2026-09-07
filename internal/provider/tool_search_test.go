package provider

import "testing"

func TestResolveToolSearch(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		model    string
		baseURL  string
		mode     ToolSearchMode
		strategy ToolSearchStrategy
		wantErr  bool
	}{
		{"openai supported", "openai", "gpt-5.4-mini", "https://api.openai.com/v1", ToolSearchAuto, ToolSearchNativeOpenAI, false},
		{"openai unsupported", "openai", "gpt-5.4-nano", "https://api.openai.com/v1", ToolSearchAuto, ToolSearchLocal, false},
		{"gateway fallback", "openai", "gpt-5.4", "https://gateway.example/v1", ToolSearchAuto, ToolSearchLocal, false},
		{"enabled uses local", "openai", "gpt-4.1", "https://api.openai.com/v1", ToolSearchEnabled, ToolSearchLocal, false},
		{"disabled wins", "anthropic", "claude-sonnet-4-6", "https://api.anthropic.com", ToolSearchDisabled, ToolSearchFull, false},
		{"old Claude fallback", "anthropic", "claude-3-5-sonnet", "https://api.anthropic.com", ToolSearchAuto, ToolSearchLocal, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveToolSearch(test.protocol, test.model, test.baseURL, test.mode)
			if (err != nil) != test.wantErr || got.Mode != test.strategy {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
