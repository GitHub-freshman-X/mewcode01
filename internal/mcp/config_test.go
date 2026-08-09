package mcp

import (
	"errors"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/config"
)

func TestExpandServerExpandsAllStringValues(t *testing.T) {
	raw := config.MCPServerConfig{
		Type:    config.MCPTransportHTTP,
		URL:     "https://${HOST}/mcp",
		Args:    []string{"${ARG}"},
		Env:     map[string]string{"TOKEN": "${TOKEN}"},
		Headers: map[string]string{"Authorization": "Bearer ${TOKEN}"},
	}
	got, err := ExpandServer("issues", raw, func(key string) (string, bool) {
		return map[string]string{"HOST": "mcp.example.test", "ARG": "unused", "TOKEN": "secret-value"}[key], true
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://mcp.example.test/mcp" || got.Args[0] != "unused" || got.Env["TOKEN"] != "secret-value" || got.Headers["Authorization"] != "Bearer secret-value" {
		t.Fatalf("expanded=%+v", got)
	}
}

func TestExpandServerReportsMissingVariableWithoutValue(t *testing.T) {
	_, err := ExpandServer("issues", config.MCPServerConfig{Type: config.MCPTransportHTTP, URL: "https://${MISSING}/mcp", Headers: map[string]string{"Authorization": "Bearer ${SECRET}"}}, func(string) (string, bool) {
		return "", false
	})
	if !errors.Is(err, ErrMissingEnvironment) {
		t.Fatalf("error=%v", err)
	}
	if got := err.Error(); got == "" || contains(got, "SECRET") || contains(got, "Bearer") {
		t.Fatalf("unsafe error=%q", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
