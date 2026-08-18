package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type requestBody struct {
	Model     string           `json:"model"`
	System    []systemBlock    `json:"system,omitempty"`
	Messages  []requestMessage `json:"messages"`
	MaxTokens int              `json:"max_tokens"`
	Stream    bool             `json:"stream"`
	Thinking  *thinkingConfig  `json:"thinking,omitempty"`
	Tools     []requestTool    `json:"tools,omitempty"`
}

type systemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type requestMessage struct {
	Role    provider.Role  `json:"role"`
	Content []requestBlock `json:"content"`
}
type requestBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	Signature string         `json:"signature,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}
type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}
type cacheControl struct {
	Type string `json:"type"`
}
type requestTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema"`
	CacheControl *cacheControl  `json:"cache_control,omitempty"`
}

func buildRequest(model string, req provider.ChatRequest) (requestBody, error) {
	if strings.TrimSpace(req.Model) != "" {
		model = req.Model
	}
	body := requestBody{Model: model, MaxTokens: req.MaxTokens, Stream: true}
	if req.Thinking.Enabled {
		body.Thinking = &thinkingConfig{Type: "enabled", BudgetTokens: req.Thinking.BudgetTokens}
	}
	for _, tool := range req.Tools {
		requestTool := requestTool{Name: tool.Name, Description: tool.Description, InputSchema: tool.Schema}
		if req.Prompt.CachePolicy.Enable && req.Prompt.CachePolicy.StableTools && tool.Cacheable {
			requestTool.CacheControl = &cacheControl{Type: "ephemeral"}
		}
		body.Tools = append(body.Tools, requestTool)
	}
	if req.Prompt.StableSystem != "" {
		block := systemBlock{Type: "text", Text: req.Prompt.StableSystem}
		if req.Prompt.CachePolicy.Enable && req.Prompt.CachePolicy.StableSystem {
			block.CacheControl = &cacheControl{Type: "ephemeral"}
		}
		body.System = append(body.System, block)
	}
	for _, message := range req.Prompt.DynamicSystem {
		if message.Content == "" {
			continue
		}
		body.System = append(body.System, systemBlock{Type: "text", Text: taggedSystemText(message)})
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
			case provider.BlockToolCall:
				if message.Role != provider.RoleAssistant || block.ToolCall == nil {
					return body, requestErr("assistant tool call block is invalid", nil)
				}
				var input map[string]any
				if len(block.ToolCall.Arguments) > 0 {
					if err := json.Unmarshal(block.ToolCall.Arguments, &input); err != nil {
						return body, requestErr("assistant tool call arguments must be a JSON object", err)
					}
				}
				if input == nil {
					input = map[string]any{}
				}
				out.Content = append(out.Content, requestBlock{Type: "tool_use", ID: block.ToolCall.ID, Name: block.ToolCall.Name, Input: input})
			case provider.BlockToolResult:
				if message.Role != provider.RoleUser || block.ToolResult == nil {
					return body, requestErr("user tool result block is invalid", nil)
				}
				out.Content = append(out.Content, requestBlock{Type: "tool_result", ToolUseID: block.ToolResult.CallID, Content: block.ToolResult.Content, IsError: block.ToolResult.IsError})
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

func taggedSystemText(message provider.SystemMessage) string {
	tag := strings.TrimSpace(message.Tag)
	if tag == "" {
		return message.Content
	}
	return fmt.Sprintf("<mew.system tag=\"%s\">\n%s\n</mew.system>", tag, message.Content)
}
