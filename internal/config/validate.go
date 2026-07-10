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
	return nil
}
