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
		Protocol    Protocol         `yaml:"protocol"`
		Model       string           `yaml:"model"`
		BaseURL     string           `yaml:"base_url"`
		APIKey      string           `yaml:"api_key"`
		MaxTokens   *int             `yaml:"max_tokens,omitempty"`
		Thinking    ThinkingConfig   `yaml:"thinking,omitempty"`
		Agent       AgentConfig      `yaml:"agent,omitempty"`
		Permissions PermissionConfig `yaml:"permissions,omitempty"`
	}
	var raw rawConfig
	if err := loader.Load(&raw); err != nil {
		return Config{}, fmt.Errorf("config: parse YAML: %w", err)
	}
	cfg := Config{Protocol: raw.Protocol, Model: raw.Model, BaseURL: raw.BaseURL, APIKey: raw.APIKey, Thinking: raw.Thinking, Agent: raw.Agent, Permissions: raw.Permissions, MaxTokens: DefaultMaxTokens}
	cfg.applyDefaults()
	if raw.MaxTokens != nil {
		cfg.MaxTokens = *raw.MaxTokens
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
