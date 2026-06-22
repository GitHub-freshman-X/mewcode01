package openai

import (
	"encoding/json"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type streamEnvelope struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
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
		return provider.StreamEvent{Type: provider.EventTextDelta, Delta: e.Delta}, true, nil
	case "response.completed":
		return provider.StreamEvent{Type: provider.EventCompleted}, true, nil
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
