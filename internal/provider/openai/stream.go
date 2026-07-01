package openai

import (
	"encoding/json"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type streamEnvelope struct {
	Type        string `json:"type"`
	Delta       string `json:"delta"`
	OutputIndex int    `json:"output_index"`
	Item        struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
	ResponseOutput struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"output"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Response struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"response"`
}

func parseEvent(data []byte) (provider.StreamEvent, bool, error) {
	var e streamEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: "invalid OpenAI stream event", Cause: err}
	}
	switch e.Type {
	case "response.created":
		return provider.StreamEvent{Type: provider.EventStarted}, true, nil
	case "response.output_text.delta":
		return provider.StreamEvent{Type: provider.EventTextDelta, BlockIndex: e.OutputIndex, Delta: e.Delta}, true, nil
	case "response.output_item.added":
		if e.Item.Type == "function_call" {
			id := e.Item.CallID
			if id == "" {
				id = e.Item.ID
			}
			return provider.StreamEvent{Type: provider.EventToolCallStart, BlockIndex: e.OutputIndex, ToolCall: &provider.ToolCallDelta{ID: id, Name: e.Item.Name, Arguments: e.Item.Arguments}}, true, nil
		}
		return provider.StreamEvent{}, false, nil
	case "response.function_call_arguments.delta":
		return provider.StreamEvent{Type: provider.EventToolCallDelta, BlockIndex: e.OutputIndex, ToolCall: &provider.ToolCallDelta{ArgumentsDelta: e.Delta}}, true, nil
	case "response.output_item.delta":
		if e.Item.Type == "function_call" || e.ResponseOutput.Type == "function_call" {
			return provider.StreamEvent{Type: provider.EventToolCallDelta, BlockIndex: e.OutputIndex, ToolCall: &provider.ToolCallDelta{ArgumentsDelta: e.Delta}}, true, nil
		}
		return provider.StreamEvent{}, false, nil
	case "response.function_call_arguments.done":
		args := e.Item.Arguments
		name := e.Item.Name
		id := e.Item.CallID
		if id == "" {
			id = e.Item.ID
		}
		if args == "" {
			args = e.ResponseOutput.Arguments
			name = e.ResponseOutput.Name
			id = e.ResponseOutput.CallID
			if id == "" {
				id = e.ResponseOutput.ID
			}
		}
		return provider.StreamEvent{Type: provider.EventToolCallDone, BlockIndex: e.OutputIndex, ToolCall: &provider.ToolCallDelta{ID: id, Name: name, Arguments: args}}, true, nil
	case "response.output_item.done":
		if e.Item.Type != "function_call" && e.ResponseOutput.Type != "function_call" {
			return provider.StreamEvent{}, false, nil
		}
		args := e.Item.Arguments
		name := e.Item.Name
		id := e.Item.CallID
		if id == "" {
			id = e.Item.ID
		}
		if args == "" {
			args = e.ResponseOutput.Arguments
			name = e.ResponseOutput.Name
			id = e.ResponseOutput.CallID
			if id == "" {
				id = e.ResponseOutput.ID
			}
		}
		return provider.StreamEvent{Type: provider.EventToolCallDone, BlockIndex: e.OutputIndex, ToolCall: &provider.ToolCallDelta{ID: id, Name: name, Arguments: args}}, true, nil
	case "response.completed":
		event := provider.StreamEvent{Type: provider.EventCompleted}
		if e.Response.Usage != nil {
			event.Usage = &provider.Usage{InputTokens: e.Response.Usage.InputTokens, OutputTokens: e.Response.Usage.OutputTokens}
		}
		return event, true, nil
	case "response.failed", "response.incomplete", "error":
		msg := e.Error.Message
		if e.Response.Error != nil {
			msg = e.Response.Error.Message
		}
		if msg == "" && e.Response.IncompleteDetails != nil {
			msg = "response incomplete: " + e.Response.IncompleteDetails.Reason
		}
		if msg == "" {
			msg = "OpenAI stream failed"
		}
		return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: msg}
	default:
		return provider.StreamEvent{}, false, nil
	}
}
