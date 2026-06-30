package provider

import "context"

type ThinkingOptions struct {
	Enabled      bool
	BudgetTokens int
}

type ChatRequest struct {
	Messages  []Message
	MaxTokens int
	Thinking  ThinkingOptions
	Tools     []ToolDefinition
}

type Provider interface {
	Stream(context.Context, ChatRequest) (<-chan StreamEvent, <-chan error)
}

type ToolDefinition struct {
	Name        string
	Description string
	Schema      map[string]any
}
