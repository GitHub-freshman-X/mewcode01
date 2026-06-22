package anthropic

import (
	"fmt"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type requestBody struct {
	Model     string           `json:"model"`
	Messages  []requestMessage `json:"messages"`
	MaxTokens int              `json:"max_tokens"`
	Stream    bool             `json:"stream"`
	Thinking  *thinkingConfig  `json:"thinking,omitempty"`
}

type requestMessage struct {
	Role    provider.Role  `json:"role"`
	Content []requestBlock `json:"content"`
}
type requestBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}
type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

func buildRequest(model string, req provider.ChatRequest) (requestBody, error) {
	body := requestBody{Model: model, MaxTokens: req.MaxTokens, Stream: true}
	if req.Thinking.Enabled {
		body.Thinking = &thinkingConfig{Type: "enabled", BudgetTokens: req.Thinking.BudgetTokens}
	}
	for _, message := range req.Messages {
		if message.Role != provider.RoleUser && message.Role != provider.RoleAssistant {
			return body, requestErr("unsupported message role", nil)
		}
		out := requestMessage{Role: message.Role}
		for _, block := range message.Blocks {
			switch block.Type {
			case provider.BlockText:
				out.Content = append(out.Content, requestBlock{Type: "text", Text: block.Text})
			case provider.BlockThinking:
				if message.Role != provider.RoleAssistant || block.Signature == "" {
					return body, requestErr("assistant thinking requires a signature", nil)
				}
				out.Content = append(out.Content, requestBlock{Type: "thinking", Thinking: block.Text, Signature: block.Signature})
			default:
				return body, requestErr(fmt.Sprintf("unsupported content block %q", block.Type), nil)
			}
		}
		body.Messages = append(body.Messages, out)
	}
	return body, nil
}

func requestErr(message string, cause error) error {
	return &provider.AppError{Stage: provider.StageRequest, Message: message, Cause: cause}
}
