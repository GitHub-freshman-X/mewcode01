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
}

type Provider interface {
	Stream(context.Context, ChatRequest) (<-chan StreamEvent, <-chan error)
}
