package config

const DefaultMaxTokens = 4096
const DefaultMaxIterations = 20

const (
	DefaultContextWindowTokens         = 200000
	DefaultContextSummaryOutputTokens  = 20000
	DefaultContextAutoSafetyTokens     = 13000
	DefaultContextManualSafetyTokens   = 3000
	DefaultContextSingleResultChars    = 50000
	DefaultContextMessageResultChars   = 200000
	DefaultContextPreviewChars         = 2000
	DefaultContextRecentTokens         = 10000
	DefaultContextRecentMessageMinimum = 5
)

type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolOpenAI    Protocol = "openai"
)

type Config struct {
	Protocol    Protocol                   `yaml:"protocol"`
	Model       string                     `yaml:"model"`
	BaseURL     string                     `yaml:"base_url"`
	APIKey      string                     `yaml:"api_key"`
	MaxTokens   int                        `yaml:"max_tokens,omitempty"`
	Thinking    ThinkingConfig             `yaml:"thinking,omitempty"`
	Agent       AgentConfig                `yaml:"agent,omitempty"`
	Permissions PermissionConfig           `yaml:"permissions,omitempty"`
	MCPServers  map[string]MCPServerConfig `yaml:"mcp_servers,omitempty"`
	Worktree    WorktreeConfig             `yaml:"worktree,omitempty"`
}

type MCPTransportType string

const (
	MCPTransportStdio MCPTransportType = "stdio"
	MCPTransportHTTP  MCPTransportType = "http"
)

type MCPServerConfig struct {
	Type    MCPTransportType  `yaml:"type"`
	Command string            `yaml:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	URL     string            `yaml:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type AgentConfig struct {
	MaxIterations           int           `yaml:"max_iterations,omitempty"`
	EnableVerificationAgent bool          `yaml:"enable_verification_agent,omitempty"`
	Context                 ContextConfig `yaml:"context,omitempty"`
}

type ContextConfig struct {
	WindowTokens         int `yaml:"window_tokens,omitempty"`
	SummaryOutputTokens  int `yaml:"summary_output_tokens,omitempty"`
	AutoSafetyTokens     int `yaml:"auto_safety_tokens,omitempty"`
	ManualSafetyTokens   int `yaml:"manual_safety_tokens,omitempty"`
	SingleResultChars    int `yaml:"single_result_chars,omitempty"`
	MessageResultChars   int `yaml:"message_result_chars,omitempty"`
	PreviewChars         int `yaml:"preview_chars,omitempty"`
	RecentTokens         int `yaml:"recent_tokens,omitempty"`
	RecentMessageMinimum int `yaml:"recent_message_minimum,omitempty"`
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

type WorktreeConfig struct {
	LocalFiles         []string `yaml:"local_files,omitempty"`
	SymlinkDirectories []string `yaml:"symlink_directories,omitempty"`
	RetentionHours     int      `yaml:"retention_hours,omitempty"`
}

func (c *Config) applyDefaults() {
	if c.MaxTokens == 0 {
		c.MaxTokens = DefaultMaxTokens
	}
	if c.Agent.MaxIterations == 0 {
		c.Agent.MaxIterations = DefaultMaxIterations
	}
	c.Agent.Context.applyDefaults()
	if c.Permissions.Mode == "" {
		c.Permissions.Mode = PermissionModeDefault
	}
}

func (c *ContextConfig) applyDefaults() {
	if c.WindowTokens == 0 {
		c.WindowTokens = DefaultContextWindowTokens
	}
	if c.SummaryOutputTokens == 0 {
		c.SummaryOutputTokens = DefaultContextSummaryOutputTokens
	}
	if c.AutoSafetyTokens == 0 {
		c.AutoSafetyTokens = DefaultContextAutoSafetyTokens
	}
	if c.ManualSafetyTokens == 0 {
		c.ManualSafetyTokens = DefaultContextManualSafetyTokens
	}
	if c.SingleResultChars == 0 {
		c.SingleResultChars = DefaultContextSingleResultChars
	}
	if c.MessageResultChars == 0 {
		c.MessageResultChars = DefaultContextMessageResultChars
	}
	if c.PreviewChars == 0 {
		c.PreviewChars = DefaultContextPreviewChars
	}
	if c.RecentTokens == 0 {
		c.RecentTokens = DefaultContextRecentTokens
	}
	if c.RecentMessageMinimum == 0 {
		c.RecentMessageMinimum = DefaultContextRecentMessageMinimum
	}
}
