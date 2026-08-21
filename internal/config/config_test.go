package config

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestEnableVerificationAgentConfig(t *testing.T) {
	base := "protocol: openai\nmodel: test\nbase_url: http://localhost\napi_key: fake\n"
	defaults, err := Load(writeConfig(t, base))
	if err != nil || defaults.Agent.EnableVerificationAgent {
		t.Fatalf("default config=%+v err=%v", defaults.Agent, err)
	}
	enabled, err := Load(writeConfig(t, base+"agent:\n  enable_verification_agent: true\n"))
	if err != nil || !enabled.Agent.EnableVerificationAgent {
		t.Fatalf("enabled config=%+v err=%v", enabled.Agent, err)
	}
}

func TestContextConfigDefaultsAndOverrides(t *testing.T) {
	base := "protocol: openai\nmodel: test\nbase_url: http://localhost\napi_key: fake\n"
	defaults, err := Load(writeConfig(t, base))
	if err != nil {
		t.Fatal(err)
	}
	assertContextConfig(t, defaults, map[string]int{
		"WindowTokens": 200000, "SummaryOutputTokens": 20000,
		"AutoSafetyTokens": 13000, "ManualSafetyTokens": 3000,
		"SingleResultChars": 50000, "MessageResultChars": 200000,
		"PreviewChars": 2000, "RecentTokens": 10000, "RecentMessageMinimum": 5,
	})

	overridden, err := Load(writeConfig(t, base+`agent:
  context:
    window_tokens: 300000
    summary_output_tokens: 25000
    auto_safety_tokens: 11000
    manual_safety_tokens: 4000
    single_result_chars: 60000
    message_result_chars: 250000
    preview_chars: 3000
    recent_tokens: 12000
    recent_message_minimum: 6
`))
	if err != nil {
		t.Fatal(err)
	}
	assertContextConfig(t, overridden, map[string]int{
		"WindowTokens": 300000, "SummaryOutputTokens": 25000,
		"AutoSafetyTokens": 11000, "ManualSafetyTokens": 4000,
		"SingleResultChars": 60000, "MessageResultChars": 250000,
		"PreviewChars": 3000, "RecentTokens": 12000, "RecentMessageMinimum": 6,
	})
}

func TestInvalidContextConfig(t *testing.T) {
	base := "protocol: openai\nmodel: test\nbase_url: http://localhost\napi_key: fake\nagent:\n  context:\n"
	for name, body := range map[string]string{
		"negative":         "    auto_safety_tokens: -1\n",
		"zero first layer": "    single_result_chars: 0\n",
		"window too small": "    window_tokens: 23000\n    summary_output_tokens: 20000\n    manual_safety_tokens: 3000\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, base+body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func assertContextConfig(t *testing.T, cfg Config, want map[string]int) {
	t.Helper()
	contextValue := reflect.ValueOf(cfg.Agent).FieldByName("Context")
	if !contextValue.IsValid() {
		t.Fatal("agent context configuration is missing")
	}
	for field, expected := range want {
		got := contextValue.FieldByName(field)
		if !got.IsValid() || int(got.Int()) != expected {
			t.Fatalf("context.%s=%v want %d", field, got, expected)
		}
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

func TestMCPServerConfig(t *testing.T) {
	cfg, err := Load(writeConfig(t, `protocol: openai
model: test
base_url: http://localhost
api_key: fake
mcp_servers:
  filesystem:
    type: stdio
    command: npx
    args: ["-y", "filesystem-server"]
    env:
      ACCESS_TOKEN: ${FILESYSTEM_TOKEN}
  issues:
    type: http
    url: https://mcp.example.test/mcp
    headers:
      Authorization: Bearer ${ISSUES_TOKEN}
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.MCPServers["filesystem"]; got.Type != MCPTransportStdio || got.Command != "npx" || len(got.Args) != 2 || got.Env["ACCESS_TOKEN"] != "${FILESYSTEM_TOKEN}" {
		t.Fatalf("stdio server=%+v", got)
	}
	if got := cfg.MCPServers["issues"]; got.Type != MCPTransportHTTP || got.URL != "https://mcp.example.test/mcp" || got.Headers["Authorization"] != "Bearer ${ISSUES_TOKEN}" {
		t.Fatalf("http server=%+v", got)
	}
}

func TestLoadAcceptsHooksForSeparateHookLoader(t *testing.T) {
	_, err := Load(writeConfig(t, `protocol: openai
model: test
base_url: http://localhost
api_key: fake
hooks:
  - event: session_start
    action:
      type: prompt
      message: hello
`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestInvalidMCPServerConfig(t *testing.T) {
	base := "protocol: openai\nmodel: test\nbase_url: http://localhost\napi_key: fake\nmcp_servers:\n  server:\n"
	for name, suffix := range map[string]string{
		"unknown transport":     "    type: websocket\n",
		"stdio without command": "    type: stdio\n",
		"http without url":      "    type: http\n",
		"http bad url":          "    type: http\n    url: relative/path\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, base+suffix)); err == nil {
				t.Fatal("expected error")
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
