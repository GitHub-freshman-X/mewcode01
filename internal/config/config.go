package config

const DefaultMaxTokens = 4096

type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolOpenAI    Protocol = "openai"
)

type Config struct {
	Protocol  Protocol       `yaml:"protocol"`
	Model     string         `yaml:"model"`
	BaseURL   string         `yaml:"base_url"`
	APIKey    string         `yaml:"api_key"`
	MaxTokens int            `yaml:"max_tokens,omitempty"`
	Thinking  ThinkingConfig `yaml:"thinking,omitempty"`
}

type ThinkingConfig struct {
	Enabled      bool `yaml:"enabled"`
	BudgetTokens int  `yaml:"budget_tokens,omitempty"`
}

func (c *Config) applyDefaults() {
	if c.MaxTokens == 0 {
		c.MaxTokens = DefaultMaxTokens
	}
}
