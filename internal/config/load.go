package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: locate user configuration directory: %w", err)
	}
	return filepath.Join(dir, "mewcode", "config.yaml"), nil
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	loader, err := yaml.NewLoader(f, yaml.WithKnownFields(), yaml.WithSingleDocument())
	if err != nil {
		return Config{}, fmt.Errorf("config: initialize YAML loader: %w", err)
	}
	type rawConfig struct {
		Protocol    Protocol                   `yaml:"protocol"`
		Model       string                     `yaml:"model"`
		BaseURL     string                     `yaml:"base_url"`
		APIKey      string                     `yaml:"api_key"`
		MaxTokens   *int                       `yaml:"max_tokens,omitempty"`
		Thinking    ThinkingConfig             `yaml:"thinking,omitempty"`
		Agent       rawAgentConfig             `yaml:"agent,omitempty"`
		Permissions PermissionConfig           `yaml:"permissions,omitempty"`
		MCPServers  map[string]MCPServerConfig `yaml:"mcp_servers,omitempty"`
		Hooks       any                        `yaml:"hooks,omitempty"`
	}
	var raw rawConfig
	if err := loader.Load(&raw); err != nil {
		return Config{}, fmt.Errorf("config: parse YAML: %w", err)
	}
	cfg := Config{Protocol: raw.Protocol, Model: raw.Model, BaseURL: raw.BaseURL, APIKey: raw.APIKey, Thinking: raw.Thinking, Agent: AgentConfig{MaxIterations: raw.Agent.MaxIterations, EnableVerificationAgent: raw.Agent.EnableVerificationAgent}, Permissions: raw.Permissions, MCPServers: raw.MCPServers, MaxTokens: DefaultMaxTokens}
	cfg.applyDefaults()
	raw.Agent.Context.apply(&cfg.Agent.Context)
	if raw.MaxTokens != nil {
		cfg.MaxTokens = *raw.MaxTokens
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

type rawAgentConfig struct {
	MaxIterations           int              `yaml:"max_iterations,omitempty"`
	EnableVerificationAgent bool             `yaml:"enable_verification_agent,omitempty"`
	Context                 rawContextConfig `yaml:"context,omitempty"`
}

type rawContextConfig struct {
	WindowTokens         *int `yaml:"window_tokens,omitempty"`
	SummaryOutputTokens  *int `yaml:"summary_output_tokens,omitempty"`
	AutoSafetyTokens     *int `yaml:"auto_safety_tokens,omitempty"`
	ManualSafetyTokens   *int `yaml:"manual_safety_tokens,omitempty"`
	SingleResultChars    *int `yaml:"single_result_chars,omitempty"`
	MessageResultChars   *int `yaml:"message_result_chars,omitempty"`
	PreviewChars         *int `yaml:"preview_chars,omitempty"`
	RecentTokens         *int `yaml:"recent_tokens,omitempty"`
	RecentMessageMinimum *int `yaml:"recent_message_minimum,omitempty"`
}

func (r rawContextConfig) apply(cfg *ContextConfig) {
	for _, field := range []struct {
		source *int
		target *int
	}{
		{r.WindowTokens, &cfg.WindowTokens}, {r.SummaryOutputTokens, &cfg.SummaryOutputTokens},
		{r.AutoSafetyTokens, &cfg.AutoSafetyTokens}, {r.ManualSafetyTokens, &cfg.ManualSafetyTokens},
		{r.SingleResultChars, &cfg.SingleResultChars}, {r.MessageResultChars, &cfg.MessageResultChars},
		{r.PreviewChars, &cfg.PreviewChars}, {r.RecentTokens, &cfg.RecentTokens},
		{r.RecentMessageMinimum, &cfg.RecentMessageMinimum},
	} {
		if field.source != nil {
			*field.target = *field.source
		}
	}
}
