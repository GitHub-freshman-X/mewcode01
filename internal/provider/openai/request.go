package openai

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

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
	Type         string         `json:"type"`
	Name         string         `json:"name,omitempty"`
	Description  string         `json:"description,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
	Tools        []toolObject   `json:"tools,omitempty"`
	DeferLoading bool           `json:"defer_loading,omitempty"`
	Execution    string         `json:"execution,omitempty"`
}

func buildRequest(model string, req provider.ChatRequest) (requestBody, error) {
	if strings.TrimSpace(req.Model) != "" {
		model = req.Model
	}
	body := requestBody{Model: model, MaxOutputTokens: req.MaxTokens, Stream: true}
	body.Tools = openAITools(req.Tools, req.ToolSearch.NativeOpenAI())
	if strings.TrimSpace(req.Prompt.StableSystem) != "" {
		body.Input = append(body.Input, inputItem{Role: "system", Content: []inputBlock{{Type: "input_text", Text: req.Prompt.StableSystem}}})
	}
	for _, message := range req.Prompt.DynamicSystem {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		body.Input = append(body.Input, inputItem{Role: "system", Content: []inputBlock{{Type: "input_text", Text: taggedSystemText(message)}}})
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

func openAITools(definitions []provider.ToolDefinition, enabled bool) []toolObject {
	if !enabled {
		tools := make([]toolObject, 0, len(definitions))
		for _, tool := range definitions {
			tools = append(tools, toolObject{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Schema})
		}
		return tools
	}
	tools := make([]toolObject, 0, len(definitions)+1)
	groups := map[string][]provider.ToolDefinition{}
	for _, tool := range definitions {
		if tool.MCPServer == "" {
			tools = append(tools, toolObject{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Schema})
			continue
		}
		groups[tool.MCPServer] = append(groups[tool.MCPServer], tool)
	}
	servers := make([]string, 0, len(groups))
	for server := range groups {
		servers = append(servers, server)
	}
	sort.Strings(servers)
	for _, server := range servers {
		members := groups[server]
		sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
		for start := 0; start < len(members); start += 10 {
			end := start + 10
			if end > len(members) {
				end = len(members)
			}
			name := "mcp_" + namespacePart(server)
			if start > 0 {
				name += fmt.Sprintf("_%d", start/10+1)
			}
			namespace := toolObject{Type: "namespace", Name: name, Description: "MCP server " + server + " tools."}
			for _, member := range members[start:end] {
				namespace.Tools = append(namespace.Tools, toolObject{Type: "function", Name: member.Name, Description: member.Description, Parameters: member.Schema, DeferLoading: true})
			}
			tools = append(tools, namespace)
		}
	}
	return append(tools, toolObject{Type: "tool_search", Execution: "server"})
}

func namespacePart(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "server"
	}
	return out.String()
}

func taggedSystemText(message provider.SystemMessage) string {
	tag := strings.TrimSpace(message.Tag)
	if tag == "" {
		return message.Content
	}
	return fmt.Sprintf("<mew.system tag=\"%s\">\n%s\n</mew.system>", tag, message.Content)
}

func requestErr(message string, cause error) error {
	return &provider.AppError{Stage: provider.StageRequest, Message: message, Cause: cause}
}
