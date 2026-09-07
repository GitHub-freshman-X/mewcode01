package provider

import "context"

type ThinkingOptions struct {
	Enabled      bool
	BudgetTokens int
}

type ChatRequest struct {
	Model      string
	Prompt     PromptBundle
	Messages   []Message
	MaxTokens  int
	Thinking   ThinkingOptions
	Tools      []ToolDefinition
	ToolSearch ToolSearchConfig
}

type Provider interface {
	Stream(context.Context, ChatRequest) (<-chan StreamEvent, <-chan error)
}

type ToolDefinition struct {
	Name        string
	Description string
	Schema      map[string]any
	Cacheable   bool
	MCPServer   string
	RemoteName  string
}

type ToolSearchConfig struct {
	Mode ToolSearchStrategy
	// Enabled preserves compatibility for callers constructing native requests directly.
	Enabled bool
}

type ToolSearchStrategy string

const (
	ToolSearchFull            ToolSearchStrategy = "full"
	ToolSearchNativeAnthropic ToolSearchStrategy = "native_anthropic"
	ToolSearchNativeOpenAI    ToolSearchStrategy = "native_openai"
	ToolSearchLocal           ToolSearchStrategy = "local"
)

func (c ToolSearchConfig) NativeAnthropic() bool {
	return c.Enabled || c.Mode == ToolSearchNativeAnthropic
}
func (c ToolSearchConfig) NativeOpenAI() bool { return c.Enabled || c.Mode == ToolSearchNativeOpenAI }
func (c ToolSearchConfig) Local() bool        { return c.Mode == ToolSearchLocal }
