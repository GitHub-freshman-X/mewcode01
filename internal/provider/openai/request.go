package openai

import "github.com/GitHub-freshman-X/mewcode01/internal/provider"

type requestBody struct {
	Model           string         `json:"model"`
	Input           []inputMessage `json:"input"`
	MaxOutputTokens int            `json:"max_output_tokens"`
	Stream          bool           `json:"stream"`
}
type inputMessage struct {
	Role    provider.Role `json:"role"`
	Content []inputBlock  `json:"content"`
}
type inputBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func buildRequest(model string, req provider.ChatRequest) (requestBody, error) {
	body := requestBody{Model: model, MaxOutputTokens: req.MaxTokens, Stream: true}
	for _, message := range req.Messages {
		if message.Role != provider.RoleUser && message.Role != provider.RoleAssistant {
			return body, requestErr("unsupported message role", nil)
		}
		out := inputMessage{Role: message.Role}
		blockType := "input_text"
		if message.Role == provider.RoleAssistant {
			blockType = "output_text"
		}
		for _, block := range message.Blocks {
			if block.Type == provider.BlockText && block.Text != "" {
				out.Content = append(out.Content, inputBlock{Type: blockType, Text: block.Text})
			}
		}
		if len(out.Content) == 0 {
			return body, requestErr("message has no visible text", nil)
		}
		body.Input = append(body.Input, out)
	}
	return body, nil
}

func requestErr(message string, cause error) error {
	return &provider.AppError{Stage: provider.StageRequest, Message: message, Cause: cause}
}
