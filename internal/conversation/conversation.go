package conversation

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type ChatOptions struct {
	MaxTokens int
	Thinking  provider.ThinkingOptions
	Tools     []provider.ToolDefinition
	Executor  *tools.Executor
}
type TurnState string

const (
	TurnIdle          TurnState = "idle"
	TurnConnecting    TurnState = "connecting"
	TurnThinking      TurnState = "thinking"
	TurnGenerating    TurnState = "generating"
	TurnToolRequested TurnState = "tool_requested"
	TurnToolRunning   TurnState = "tool_running"
	TurnToolCompleted TurnState = "tool_completed"
	TurnCompleted     TurnState = "completed"
	TurnCancelled     TurnState = "cancelled"
	TurnFailed        TurnState = "failed"
)

type Turn struct {
	UserMessage      provider.Message
	AssistantMessage provider.Message
	State            TurnState
	Err              error
}
type Conversation struct {
	mu       sync.Mutex
	provider provider.Provider
	options  ChatOptions
	history  []provider.Message
	active   *Turn
	cancel   context.CancelFunc
}

func NewConversation(p provider.Provider, options ChatOptions) *Conversation {
	return &Conversation{provider: p, options: options}
}

func (c *Conversation) Start(ctx context.Context, input string) (<-chan provider.StreamEvent, <-chan error, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil, errors.New("input is empty")
	}
	if c.active != nil && (c.active.State == TurnConnecting || c.active.State == TurnThinking || c.active.State == TurnGenerating) {
		return nil, nil, errors.New("a request is already active")
	}
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: input}}}
	c.active = &Turn{UserMessage: user, AssistantMessage: provider.Message{Role: provider.RoleAssistant}, State: TurnConnecting}
	messages := append(provider.CloneMessages(c.history), provider.CloneMessage(user))
	events, done := c.provider.Stream(ctx, provider.ChatRequest{Messages: messages, MaxTokens: c.options.MaxTokens, Thinking: c.options.Thinking, Tools: c.options.Tools})
	return events, done, nil
}

func (c *Conversation) History() []provider.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return provider.CloneMessages(c.history)
}
func (c *Conversation) ActiveTurn() *Turn {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return nil
	}
	t := *c.active
	t.UserMessage = provider.CloneMessage(t.UserMessage)
	t.AssistantMessage = provider.CloneMessage(t.AssistantMessage)
	return &t
}

func (c *Conversation) Complete() error {
	c.mu.Lock()
	if c.active == nil {
		c.mu.Unlock()
		return errors.New("no active turn")
	}
	if c.active.State != TurnCompleted {
		c.mu.Unlock()
		return errors.New("turn has not received a completion event")
	}
	toolCalls := toolCallsFrom(c.active.AssistantMessage)
	c.history = append(c.history, provider.CloneMessage(c.active.UserMessage), provider.CloneMessage(c.active.AssistantMessage))
	if len(toolCalls) == 0 {
		c.cancel = nil
		c.mu.Unlock()
		return nil
	}
	if c.options.Executor == nil {
		c.mu.Unlock()
		return errors.New("tool executor is not configured")
	}
	c.active.State = TurnToolRunning
	executor := c.options.Executor
	c.mu.Unlock()

	blocks := make([]provider.ContentBlock, 0, len(toolCalls))
	for _, call := range toolCalls {
		result := executor.Execute(context.Background(), call)
		blocks = append(blocks, provider.ContentBlock{Type: provider.BlockToolResult, ToolResult: &result})
	}

	c.mu.Lock()
	if c.active != nil {
		c.active.State = TurnToolCompleted
	}
	c.history = append(c.history, provider.Message{Role: provider.RoleUser, Blocks: blocks})
	c.cancel = nil
	c.mu.Unlock()
	return nil
}
func (c *Conversation) Fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil {
		c.active.State = TurnFailed
		c.active.Err = err
	}
	c.cancel = nil
}
func (c *Conversation) Cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	if c.active != nil {
		c.active.State = TurnCancelled
	}
	c.cancel = nil
}
func (c *Conversation) IsBusy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active != nil && (c.active.State == TurnConnecting || c.active.State == TurnThinking || c.active.State == TurnGenerating || c.active.State == TurnToolRequested || c.active.State == TurnToolRunning)
}

func toolCallsFrom(message provider.Message) []provider.ToolCall {
	var calls []provider.ToolCall
	for _, block := range message.Blocks {
		if block.Type == provider.BlockToolCall && block.ToolCall != nil {
			calls = append(calls, *block.ToolCall)
		}
	}
	return calls
}
