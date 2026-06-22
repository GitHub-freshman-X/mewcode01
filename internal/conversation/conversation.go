package conversation

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type ChatOptions struct {
	MaxTokens int
	Thinking  provider.ThinkingOptions
}
type TurnState string

const (
	TurnIdle       TurnState = "idle"
	TurnConnecting TurnState = "connecting"
	TurnThinking   TurnState = "thinking"
	TurnGenerating TurnState = "generating"
	TurnCompleted  TurnState = "completed"
	TurnCancelled  TurnState = "cancelled"
	TurnFailed     TurnState = "failed"
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
	events, done := c.provider.Stream(ctx, provider.ChatRequest{Messages: messages, MaxTokens: c.options.MaxTokens, Thinking: c.options.Thinking})
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
	defer c.mu.Unlock()
	if c.active == nil {
		return errors.New("no active turn")
	}
	if c.active.State != TurnCompleted {
		return errors.New("turn has not received a completion event")
	}
	c.history = append(c.history, provider.CloneMessage(c.active.UserMessage), provider.CloneMessage(c.active.AssistantMessage))
	c.cancel = nil
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
	return c.active != nil && (c.active.State == TurnConnecting || c.active.State == TurnThinking || c.active.State == TurnGenerating)
}
