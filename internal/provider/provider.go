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
	Enabled bool
}
