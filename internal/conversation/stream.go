package conversation

import (
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
	case provider.EventCompleted:
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
