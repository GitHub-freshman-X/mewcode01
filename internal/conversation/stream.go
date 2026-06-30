package conversation

import (
	"encoding/json"
	"errors"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func (c *Conversation) Apply(event provider.StreamEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return errors.New("no active turn")
	}
	switch event.Type {
	case provider.EventStarted:
		c.active.State = TurnConnecting
	case provider.EventThinkingDelta:
		block, err := blockAt(&c.active.AssistantMessage, event.BlockIndex, provider.BlockThinking)
		if err != nil {
			return err
		}
		block.Text += event.Delta
		c.active.State = TurnThinking
	case provider.EventSignatureDelta:
		block, err := blockAt(&c.active.AssistantMessage, event.BlockIndex, provider.BlockThinking)
		if err != nil {
			return err
		}
		block.Signature += event.Delta
	case provider.EventTextDelta:
		block, err := blockAt(&c.active.AssistantMessage, event.BlockIndex, provider.BlockText)
		if err != nil {
			return err
		}
		block.Text += event.Delta
		c.active.State = TurnGenerating
	case provider.EventToolCallStart:
		block, err := blockAt(&c.active.AssistantMessage, event.BlockIndex, provider.BlockToolCall)
		if err != nil {
			return err
		}
		if block.ToolCall == nil {
			block.ToolCall = &provider.ToolCall{}
		}
		if event.ToolCall != nil {
			block.ToolCall.ID = event.ToolCall.ID
			block.ToolCall.Name = event.ToolCall.Name
			if event.ToolCall.Arguments != "" {
				block.ToolCall.Arguments = []byte(event.ToolCall.Arguments)
			}
		}
		c.active.State = TurnToolRequested
	case provider.EventToolCallDelta:
		block, err := blockAt(&c.active.AssistantMessage, event.BlockIndex, provider.BlockToolCall)
		if err != nil {
			return err
		}
		if block.ToolCall == nil {
			block.ToolCall = &provider.ToolCall{}
		}
		if event.ToolCall != nil {
			if event.ToolCall.ID != "" {
				block.ToolCall.ID = event.ToolCall.ID
			}
			if event.ToolCall.Name != "" {
				block.ToolCall.Name = event.ToolCall.Name
			}
			if event.ToolCall.Arguments != "" {
				block.ToolCall.Arguments = []byte(event.ToolCall.Arguments)
			}
			if event.ToolCall.ArgumentsDelta != "" {
				block.ToolCall.Arguments = append(block.ToolCall.Arguments, []byte(event.ToolCall.ArgumentsDelta)...)
			}
		}
		c.active.State = TurnToolRequested
	case provider.EventToolCallDone:
		block, err := blockAt(&c.active.AssistantMessage, event.BlockIndex, provider.BlockToolCall)
		if err != nil {
			return err
		}
		if block.ToolCall == nil {
			block.ToolCall = &provider.ToolCall{}
		}
		if event.ToolCall != nil {
			if event.ToolCall.ID != "" {
				block.ToolCall.ID = event.ToolCall.ID
			}
			if event.ToolCall.Name != "" {
				block.ToolCall.Name = event.ToolCall.Name
			}
			if event.ToolCall.Arguments != "" {
				block.ToolCall.Arguments = []byte(event.ToolCall.Arguments)
			}
		}
		if block.ToolCall.Name == "" || block.ToolCall.ID == "" {
			return errors.New("tool call is missing id or name")
		}
		if len(block.ToolCall.Arguments) == 0 {
			block.ToolCall.Arguments = []byte("{}")
		}
		if !json.Valid(block.ToolCall.Arguments) {
			return errors.New("tool call arguments are not valid JSON")
		}
		c.active.State = TurnToolRequested
	case provider.EventCompleted:
		for _, block := range c.active.AssistantMessage.Blocks {
			if block.Type != provider.BlockToolCall || block.ToolCall == nil {
				continue
			}
			if block.ToolCall.Name == "" || block.ToolCall.ID == "" {
				return errors.New("tool call is missing id or name")
			}
			if len(block.ToolCall.Arguments) == 0 {
				block.ToolCall.Arguments = []byte("{}")
			}
			if !json.Valid(block.ToolCall.Arguments) {
				return errors.New("tool call arguments are not valid JSON")
			}
		}
		c.active.State = TurnCompleted
	default:
		return errors.New("unknown stream event")
	}
	return nil
}

func blockAt(message *provider.Message, index int, kind provider.BlockType) (*provider.ContentBlock, error) {
	if index < 0 {
		return nil, errors.New("negative block index")
	}
	for len(message.Blocks) <= index {
		message.Blocks = append(message.Blocks, provider.ContentBlock{})
	}
	block := &message.Blocks[index]
	if block.Type == "" {
		block.Type = kind
	}
	if block.Type != kind {
		return nil, errors.New("content block type conflict")
	}
	return block, nil
}
