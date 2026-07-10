package config

const DefaultMaxTokens = 4096
const DefaultMaxIterations = 20

type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolOpenAI    Protocol = "openai"
)

type Config struct {
	Protocol    Protocol         `yaml:"protocol"`
	Model       string           `yaml:"model"`
	BaseURL     string           `yaml:"base_url"`
	APIKey      string           `yaml:"api_key"`
	MaxTokens   int              `yaml:"max_tokens,omitempty"`
	Thinking    ThinkingConfig   `yaml:"thinking,omitempty"`
	Agent       AgentConfig      `yaml:"agent,omitempty"`
	Permissions PermissionConfig `yaml:"permissions,omitempty"`
}

type AgentConfig struct {
	MaxIterations int `yaml:"max_iterations,omitempty"`
}

type ThinkingConfig struct {
	Enabled      bool `yaml:"enabled"`
	BudgetTokens int  `yaml:"budget_tokens,omitempty"`
}

type PermissionMode string

const (
	PermissionModeStrict  PermissionMode = "strict"
	PermissionModeDefault PermissionMode = "default"
	PermissionModeRelaxed PermissionMode = "relaxed"
)

type PermissionConfig struct {
	Mode PermissionMode `yaml:"mode,omitempty"`
}

func (c *Config) applyDefaults() {
	if c.MaxTokens == 0 {
		c.MaxTokens = DefaultMaxTokens
	}
	if c.Agent.MaxIterations == 0 {
		c.Agent.MaxIterations = DefaultMaxIterations
	}
	if c.Permissions.Mode == "" {
		c.Permissions.Mode = PermissionModeDefault
	}
}
