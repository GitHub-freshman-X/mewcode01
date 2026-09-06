package provider

import "testing"

func TestResolveToolSearch(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		model    string
		baseURL  string
		mode     ToolSearchMode
		enabled  bool
		wantErr  bool
	}{
		{"openai supported", "openai", "gpt-5.4-mini", "https://api.openai.com/v1", ToolSearchAuto, true, false},
		{"openai unsupported", "openai", "gpt-5.4-nano", "https://api.openai.com/v1", ToolSearchAuto, false, false},
		{"gateway fallback", "openai", "gpt-5.4", "https://gateway.example/v1", ToolSearchAuto, false, false},
		{"enabled rejects unsupported", "openai", "gpt-4.1", "https://api.openai.com/v1", ToolSearchEnabled, false, true},
		{"disabled wins", "anthropic", "claude-sonnet-4-6", "https://api.anthropic.com", ToolSearchDisabled, false, false},
		{"old Claude fallback", "anthropic", "claude-3-5-sonnet", "https://api.anthropic.com", ToolSearchAuto, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveToolSearch(test.protocol, test.model, test.baseURL, test.mode)
			if (err != nil) != test.wantErr || got.Enabled != test.enabled {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
