package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type streamEnvelope struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	Message struct {
		Usage rawUsage `json:"usage"`
	} `json:"message"`
	Usage rawUsage `json:"usage"`
}

type rawUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreation            struct {
		Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

func parseEvent(data []byte) (provider.StreamEvent, bool, error) {
	var e streamEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: "invalid Anthropic stream event", Cause: err}
	}
	switch e.Type {
	case "message_start":
		return provider.StreamEvent{Type: provider.EventStarted, Usage: usageOrNil(e.Message.Usage)}, true, nil
	case "message_stop":
		return provider.StreamEvent{Type: provider.EventCompleted}, true, nil
	case "content_block_delta":
		switch e.Delta.Type {
		case "text_delta":
			return provider.StreamEvent{Type: provider.EventTextDelta, BlockIndex: e.Index, Delta: e.Delta.Text}, true, nil
		case "thinking_delta":
			return provider.StreamEvent{Type: provider.EventThinkingDelta, BlockIndex: e.Index, Delta: e.Delta.Thinking}, true, nil
		case "signature_delta":
			return provider.StreamEvent{Type: provider.EventSignatureDelta, BlockIndex: e.Index, Delta: e.Delta.Signature}, true, nil
		case "input_json_delta":
			return provider.StreamEvent{Type: provider.EventToolCallDelta, BlockIndex: e.Index, ToolCall: &provider.ToolCallDelta{ArgumentsDelta: e.Delta.PartialJSON}}, true, nil
		default:
			return provider.StreamEvent{}, false, nil
		}
	case "content_block_start":
		if e.ContentBlock.Type == "tool_use" {
			args := "{}"
			if len(e.ContentBlock.Input) > 0 {
				args = string(e.ContentBlock.Input)
			}
			if args == "null" {
				args = "{}"
			}
			return provider.StreamEvent{Type: provider.EventToolCallStart, BlockIndex: e.Index, ToolCall: &provider.ToolCallDelta{ID: e.ContentBlock.ID, Name: e.ContentBlock.Name, Arguments: args}}, true, nil
		}
		return provider.StreamEvent{}, false, nil
	case "error":
		msg := e.Error.Message
		if msg == "" {
			msg = "Anthropic stream error"
		}
		return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: msg}
	case "message_delta":
		if usage := usageOrNil(e.Usage); usage != nil {
			return provider.StreamEvent{Type: provider.EventUsage, Usage: usage}, true, nil
		}
		return provider.StreamEvent{}, false, nil
	case "ping", "content_block_stop":
		return provider.StreamEvent{}, false, nil
	default:
		if e.Type == "" {
			return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: fmt.Sprintf("Anthropic event missing type")}
		}
		return provider.StreamEvent{}, false, nil
	}
}

func usageOrNil(raw rawUsage) *provider.Usage {
	cacheCreation := raw.CacheCreationInputTokens + raw.CacheCreation.Ephemeral5mInputTokens + raw.CacheCreation.Ephemeral1hInputTokens
	if raw.InputTokens == 0 && raw.OutputTokens == 0 && raw.CacheReadInputTokens == 0 && cacheCreation == 0 {
		return nil
	}
	return &provider.Usage{
		InputTokens:              raw.InputTokens,
		OutputTokens:             raw.OutputTokens,
		CacheReadInputTokens:     raw.CacheReadInputTokens,
		CacheCreationInputTokens: cacheCreation,
	}
}
