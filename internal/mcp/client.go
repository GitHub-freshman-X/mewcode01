package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

const protocolVersion = "2025-06-18"

type RemoteTool struct {
	Name        string
	Description string
	InputSchema tools.Schema
}
type CallResult struct {
	Text              string
	Content           []any
	StructuredContent any
	IsError           bool
}
type Client struct {
	session     *Session
	initialized bool
}

func NewClient(transport Transport) *Client { return &Client{session: NewSession(transport)} }
func (c *Client) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.session.Request(ctx, "initialize", map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "mewcode", "version": "0.1"}}, &result); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if result.ProtocolVersion == "" {
		return fmt.Errorf("initialize: server did not select a protocol version")
	}
	if err := c.session.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}
	c.initialized = true
	return nil
}
func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	if !c.initialized {
		return nil, fmt.Errorf("tools/list: client is not initialized")
	}
	var result struct {
		Tools []struct {
			Name        string       `json:"name"`
			Description string       `json:"description"`
			InputSchema tools.Schema `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := c.session.Request(ctx, "tools/list", map[string]any{}, &result); err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	out := make([]RemoteTool, 0, len(result.Tools))
	for _, remote := range result.Tools {
		if remote.Name == "" || remote.Description == "" || remote.InputSchema == nil {
			return nil, fmt.Errorf("tools/list: invalid tool definition")
		}
		if typ, _ := remote.InputSchema["type"].(string); typ != "object" {
			return nil, fmt.Errorf("tools/list: tool %q schema must be object", remote.Name)
		}
		out = append(out, RemoteTool{Name: remote.Name, Description: remote.Description, InputSchema: remote.InputSchema})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error) {
	if !c.initialized {
		return CallResult{}, fmt.Errorf("tools/call: client is not initialized")
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var arguments any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return CallResult{}, fmt.Errorf("tools/call: arguments: %w", err)
	}
	var result struct {
		Content           []any `json:"content"`
		StructuredContent any   `json:"structuredContent"`
		IsError           bool  `json:"isError"`
	}
	if err := c.session.Request(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result); err != nil {
		return CallResult{}, fmt.Errorf("tools/call: %w", err)
	}
	text := ""
	for _, item := range result.Content {
		if object, ok := item.(map[string]any); ok {
			if value, ok := object["text"].(string); ok {
				text += value
			}
		}
	}
	return CallResult{Text: text, Content: result.Content, StructuredContent: result.StructuredContent, IsError: result.IsError}, nil
}
func (c *Client) Close(ctx context.Context) error { return c.session.Close(ctx) }
