package openai

import "github.com/GitHub-freshman-X/mewcode01/internal/provider"

type requestBody struct {
	Model           string       `json:"model"`
	Input           []inputItem  `json:"input"`
	MaxOutputTokens int          `json:"max_output_tokens"`
	Stream          bool         `json:"stream"`
	Tools           []toolObject `json:"tools,omitempty"`
}
type inputItem struct {
	Role      provider.Role `json:"role,omitempty"`
	Content   []inputBlock  `json:"content,omitempty"`
	Type      string        `json:"type,omitempty"`
	CallID    string        `json:"call_id,omitempty"`
	Name      string        `json:"name,omitempty"`
	Arguments string        `json:"arguments,omitempty"`
	Output    string        `json:"output,omitempty"`
}
type inputBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type toolObject struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func buildRequest(model string, req provider.ChatRequest) (requestBody, error) {
	body := requestBody{Model: model, MaxOutputTokens: req.MaxTokens, Stream: true}
	for _, tool := range req.Tools {
		body.Tools = append(body.Tools, toolObject{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Schema})
	}
	for _, message := range req.Messages {
		if message.Role != provider.RoleUser && message.Role != provider.RoleAssistant {
			return body, requestErr("unsupported message role", nil)
		}
		out := inputItem{Role: message.Role}
		blockType := "input_text"
		if message.Role == provider.RoleAssistant {
			blockType = "output_text"
		}
		for _, block := range message.Blocks {
			if block.Type == provider.BlockText && block.Text != "" {
				out.Content = append(out.Content, inputBlock{Type: blockType, Text: block.Text})
				continue
			}
			if block.Type == provider.BlockToolCall {
				if message.Role != provider.RoleAssistant || block.ToolCall == nil {
					return body, requestErr("assistant tool call block is invalid", nil)
				}
				if len(out.Content) > 0 {
					body.Input = append(body.Input, out)
					out = inputItem{Role: message.Role}
				}
				body.Input = append(body.Input, inputItem{Type: "function_call", CallID: block.ToolCall.ID, Name: block.ToolCall.Name, Arguments: string(block.ToolCall.Arguments)})
				continue
			}
			if block.Type == provider.BlockToolResult {
				if message.Role != provider.RoleUser || block.ToolResult == nil {
					return body, requestErr("user tool result block is invalid", nil)
				}
				if len(out.Content) > 0 {
					body.Input = append(body.Input, out)
					out = inputItem{Role: message.Role}
				}
				body.Input = append(body.Input, inputItem{Type: "function_call_output", CallID: block.ToolResult.CallID, Output: block.ToolResult.Content})
			}
		}
		if len(out.Content) == 0 {
			if len(message.Blocks) == 0 {
				return body, requestErr("message has no visible text", nil)
			}
			continue
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
