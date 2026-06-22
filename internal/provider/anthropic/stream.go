package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type streamEnvelope struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		Thinking  string `json:"thinking"`
		Signature string `json:"signature"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func parseEvent(data []byte) (provider.StreamEvent, bool, error) {
	var e streamEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: "invalid Anthropic stream event", Cause: err}
	}
	switch e.Type {
	case "message_start":
		return provider.StreamEvent{Type: provider.EventStarted}, true, nil
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
		default:
			return provider.StreamEvent{}, false, nil
		}
	case "error":
		msg := e.Error.Message
		if msg == "" {
			msg = "Anthropic stream error"
		}
		return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: msg}
	case "ping", "content_block_start", "content_block_stop", "message_delta":
		return provider.StreamEvent{}, false, nil
	default:
		if e.Type == "" {
			return provider.StreamEvent{}, false, &provider.AppError{Stage: provider.StageStream, Message: fmt.Sprintf("Anthropic event missing type")}
		}
		return provider.StreamEvent{}, false, nil
	}
}
