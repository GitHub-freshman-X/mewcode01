package config

import (
	"fmt"
	"net/url"
	"strings"
)

func Validate(cfg Config) error {
	required := []struct{ name, value string }{
		{"protocol", string(cfg.Protocol)}, {"model", cfg.Model},
		{"base_url", cfg.BaseURL}, {"api_key", cfg.APIKey},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("config: field %q is required", field.name)
		}
	}
	if cfg.Protocol != ProtocolAnthropic && cfg.Protocol != ProtocolOpenAI {
		return fmt.Errorf("config: field %q must be anthropic or openai", "protocol")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("config: field %q must be an absolute HTTP(S) URL", "base_url")
	}
	if cfg.MaxTokens <= 0 {
		return fmt.Errorf("config: field %q must be greater than zero", "max_tokens")
	}
	if cfg.Agent.MaxIterations < 0 {
		return fmt.Errorf("config: field %q must not be negative", "agent.max_iterations")
	}
	if cfg.Worktree.RetentionHours < 0 {
		return fmt.Errorf("config: field %q must not be negative", "worktree.retention_hours")
	}
	if err := validateContextConfig(cfg.Agent.Context); err != nil {
		return err
	}
	switch cfg.Permissions.Mode {
	case "", PermissionModeStrict, PermissionModeDefault, PermissionModeRelaxed:
	default:
		return fmt.Errorf("config: field %q must be strict, default, or relaxed", "permissions.mode")
	}
	if cfg.Protocol == ProtocolOpenAI && (cfg.Thinking.Enabled || cfg.Thinking.BudgetTokens != 0) {
		return fmt.Errorf("config: field %q is only supported by anthropic", "thinking")
	}
	if !cfg.Thinking.Enabled && cfg.Thinking.BudgetTokens != 0 {
		return fmt.Errorf("config: field %q must be zero when thinking is disabled", "thinking.budget_tokens")
	}
	if cfg.Thinking.Enabled {
		if cfg.Thinking.BudgetTokens < 1024 {
			return fmt.Errorf("config: field %q must be at least 1024", "thinking.budget_tokens")
		}
		if cfg.Thinking.BudgetTokens >= cfg.MaxTokens {
			return fmt.Errorf("config: field %q must be less than max_tokens", "thinking.budget_tokens")
		}
	}
	for name, server := range cfg.MCPServers {
		if err := ValidateMCPServer(name, server); err != nil {
			return err
		}
	}
	return nil
}

func validateContextConfig(cfg ContextConfig) error {
	for _, field := range []struct {
		name  string
		value int
	}{
		{"agent.context.window_tokens", cfg.WindowTokens}, {"agent.context.summary_output_tokens", cfg.SummaryOutputTokens},
		{"agent.context.auto_safety_tokens", cfg.AutoSafetyTokens}, {"agent.context.manual_safety_tokens", cfg.ManualSafetyTokens},
		{"agent.context.single_result_chars", cfg.SingleResultChars}, {"agent.context.message_result_chars", cfg.MessageResultChars},
		{"agent.context.preview_chars", cfg.PreviewChars}, {"agent.context.recent_tokens", cfg.RecentTokens},
		{"agent.context.recent_message_minimum", cfg.RecentMessageMinimum},
	} {
		if field.value < 0 {
			return fmt.Errorf("config: field %q must not be negative", field.name)
		}
	}
	if cfg.SingleResultChars == 0 || cfg.MessageResultChars == 0 || cfg.PreviewChars == 0 || cfg.RecentTokens == 0 || cfg.RecentMessageMinimum == 0 {
		return fmt.Errorf("config: first-layer and recent context budgets must be greater than zero")
	}
	if cfg.WindowTokens <= cfg.SummaryOutputTokens+cfg.AutoSafetyTokens || cfg.WindowTokens <= cfg.SummaryOutputTokens+cfg.ManualSafetyTokens {
		return fmt.Errorf("config: field %q must exceed summary output and safety budgets", "agent.context.window_tokens")
	}
	return nil
}

func ValidateMCPServer(name string, server MCPServerConfig) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("config: mcp server name must not be empty")
	}
	switch server.Type {
	case MCPTransportStdio:
		if strings.TrimSpace(server.Command) == "" {
			return fmt.Errorf("config: mcp server %q requires command for stdio transport", name)
		}
	case MCPTransportHTTP:
		u, err := url.Parse(server.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("config: mcp server %q requires an absolute HTTP(S) url", name)
		}
	default:
		return fmt.Errorf("config: mcp server %q has unsupported transport %q", name, server.Type)
	}
	return nil
}
