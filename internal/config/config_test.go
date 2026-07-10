package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	if err != nil || !strings.HasSuffix(filepath.ToSlash(p), "mewcode/config.yaml") {
		t.Fatalf("path=%q err=%v", p, err)
	}
}

func TestLoadAndDefaults(t *testing.T) {
	for _, protocol := range []string{"anthropic", "openai"} {
		t.Run(protocol, func(t *testing.T) {
			p := writeConfig(t, "protocol: "+protocol+"\nmodel: test\nbase_url: http://localhost:1234\napi_key: fake\n")
			cfg, err := Load(p)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MaxTokens != DefaultMaxTokens {
				t.Fatalf("max_tokens=%d", cfg.MaxTokens)
			}
			if cfg.Agent.MaxIterations != DefaultMaxIterations {
				t.Fatalf("max_iterations=%d", cfg.Agent.MaxIterations)
			}
			if cfg.Permissions.Mode != PermissionModeDefault {
				t.Fatalf("permissions.mode=%q", cfg.Permissions.Mode)
			}
		})
	}
}

func TestPermissionModeConfig(t *testing.T) {
	for _, mode := range []PermissionMode{PermissionModeStrict, PermissionModeDefault, PermissionModeRelaxed} {
		t.Run(string(mode), func(t *testing.T) {
			cfg, err := Load(writeConfig(t, "protocol: openai\nmodel: test\nbase_url: http://localhost\napi_key: fake\npermissions:\n  mode: "+string(mode)+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Permissions.Mode != mode {
				t.Fatalf("mode=%q want %q", cfg.Permissions.Mode, mode)
			}
		})
	}
}

func TestInvalidConfig(t *testing.T) {
	canary := "canary-super-secret-key"
	cases := map[string]string{
		"unknown":         "extra: true\n",
		"multi":           "---\nprotocol: anthropic\n---\nprotocol: openai\n",
		"bad yaml":        "protocol: [\n",
		"bad protocol":    "protocol: other\nmodel: x\nbase_url: http://localhost\napi_key: " + canary + "\n",
		"bad url":         "protocol: openai\nmodel: x\nbase_url: nope\napi_key: " + canary + "\n",
		"zero tokens":     "protocol: openai\nmodel: x\nbase_url: http://localhost\napi_key: " + canary + "\nmax_tokens: 0\n",
		"negative agent":  "protocol: openai\nmodel: x\nbase_url: http://localhost\napi_key: " + canary + "\nagent:\n  max_iterations: -1\n",
		"bad permission":  "protocol: openai\nmodel: x\nbase_url: http://localhost\napi_key: " + canary + "\npermissions:\n  mode: wide-open\n",
		"thinking budget": "protocol: anthropic\nmodel: x\nbase_url: http://localhost\napi_key: " + canary + "\nmax_tokens: 1024\nthinking:\n  enabled: true\n  budget_tokens: 1024\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(body, "api_key:") && name != "multi" && name != "bad yaml" {
				body += "protocol: anthropic\nmodel: x\nbase_url: http://localhost\napi_key: " + canary + "\n"
			}
			_, err := Load(writeConfig(t, body))
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), canary) {
				t.Fatal("secret leaked")
			}
		})
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
