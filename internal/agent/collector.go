package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type roundResult struct {
	Assistant provider.Message
	ToolCalls []provider.ToolCall
	Usage     provider.Usage
}

func collectRound(ctx context.Context, iteration int, events <-chan provider.StreamEvent, done <-chan error, emit func(Event) bool) (roundResult, error) {
	result := roundResult{Assistant: provider.Message{Role: provider.RoleAssistant}}
	completed := false
	var streamErr error
	for events != nil || done != nil {
		select {
		case <-ctx.Done():
			return roundResult{}, ctx.Err()
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if err := applyProviderEvent(&result, event, iteration, emit, &completed); err != nil {
				return roundResult{}, err
			}
		case err, ok := <-done:
			if !ok {
				done = nil
				continue
			}
			if err != nil {
				streamErr = err
			}
			done = nil
		}
	}
	if streamErr != nil {
		return roundResult{}, streamErr
	}
	if !completed {
		return roundResult{}, errors.New("provider stream ended before completion")
	}
	for i := range result.Assistant.Blocks {
		block := &result.Assistant.Blocks[i]
		if block.Type != provider.BlockToolCall || block.ToolCall == nil {
			continue
		}
		call := block.ToolCall
		if call.ID == "" || call.Name == "" {
			return roundResult{}, errors.New("tool call is missing id or name")
		}
		if len(call.Arguments) == 0 {
			call.Arguments = []byte("{}")
		}
		if !json.Valid(call.Arguments) {
			return roundResult{}, fmt.Errorf("tool call %q arguments are not valid JSON", call.Name)
		}
		result.ToolCalls = append(result.ToolCalls, *call)
	}
	return result, nil
}

func applyProviderEvent(result *roundResult, event provider.StreamEvent, iteration int, emit func(Event) bool, completed *bool) error {
	switch event.Type {
	case provider.EventStarted:
		return nil
	case provider.EventThinkingDelta:
		block, err := roundBlockAt(&result.Assistant, event.BlockIndex, provider.BlockThinking)
		if err != nil {
			return err
		}
		block.Text += event.Delta
	case provider.EventSignatureDelta:
		block, err := roundBlockAt(&result.Assistant, event.BlockIndex, provider.BlockThinking)
		if err != nil {
			return err
		}
		block.Signature += event.Delta
	case provider.EventTextDelta:
		block, err := roundBlockAt(&result.Assistant, event.BlockIndex, provider.BlockText)
		if err != nil {
			return err
		}
		block.Text += event.Delta
		if !emit(Event{Type: EventTextDelta, Iteration: iteration, Phase: PhaseStreaming, Text: event.Delta}) {
			return context.Canceled
		}
	case provider.EventToolCallStart, provider.EventToolCallDelta, provider.EventToolCallDone:
		block, err := roundBlockAt(&result.Assistant, event.BlockIndex, provider.BlockToolCall)
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
				if string(block.ToolCall.Arguments) == "{}" {
					block.ToolCall.Arguments = nil
				}
				block.ToolCall.Arguments = append(block.ToolCall.Arguments, event.ToolCall.ArgumentsDelta...)
			}
		}
	case provider.EventUsage:
		if event.Usage != nil {
			result.Usage.Add(*event.Usage)
			usage := *event.Usage
			if !emit(Event{Type: EventUsage, Iteration: iteration, Phase: PhaseStreaming, Usage: &usage}) {
				return context.Canceled
			}
		}
	case provider.EventCompleted:
		*completed = true
	default:
		return fmt.Errorf("unknown provider stream event %q", event.Type)
	}
	return nil
}

func roundBlockAt(message *provider.Message, index int, kind provider.BlockType) (*provider.ContentBlock, error) {
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
