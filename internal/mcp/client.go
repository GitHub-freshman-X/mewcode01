package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
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
	logger      *logging.Logger
	initialized bool
}

func NewClient(transport Transport, loggers ...*logging.Logger) *Client {
	return &Client{session: NewSession(transport), logger: normalizedLogger(loggers)}
}
func (c *Client) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	c.logger.Info("mcp", "initialize_started", "MCP initialization started", logging.Fields{"stage": "initialize", "status": "started"})
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.session.Request(ctx, "initialize", map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "mewcode", "version": "0.1"}}, &result); err != nil {
		c.logger.Error("mcp", "initialize_failed", "MCP initialization failed", logging.Fields{"stage": "initialize", "status": "initialize_failed"})
		return fmt.Errorf("initialize: %w", err)
	}
	if result.ProtocolVersion == "" {
		c.logger.Error("mcp", "initialize_failed", "MCP initialization failed", logging.Fields{"stage": "initialize", "status": "initialize_failed"})
		return fmt.Errorf("initialize: server did not select a protocol version")
	}
	if err := c.session.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		c.logger.Error("mcp", "initialized_notification_failed", "MCP initialized notification failed", logging.Fields{"stage": "initialize", "status": "notification_failed"})
		return fmt.Errorf("initialized notification: %w", err)
	}
	c.initialized = true
	c.logger.Info("mcp", "initialize_succeeded", "MCP initialization succeeded", logging.Fields{"stage": "initialize", "status": "initialized"})
	c.logger.Info("mcp", "initialized_notification_sent", "MCP initialized notification sent", logging.Fields{"stage": "initialize", "status": "notification_sent"})
	return nil
}
func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	if !c.initialized {
		return nil, fmt.Errorf("tools/list: client is not initialized")
	}
	c.logger.Info("mcp", "tool_discovery_started", "MCP tool discovery started", logging.Fields{"stage": "discover", "status": "started"})
	var result struct {
		Tools []struct {
			Name        string       `json:"name"`
			Description string       `json:"description"`
			InputSchema tools.Schema `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := c.session.Request(ctx, "tools/list", map[string]any{}, &result); err != nil {
		c.logger.Error("mcp", "tool_discovery_failed", "MCP tool discovery failed", logging.Fields{"stage": "discover", "status": "discover_failed"})
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
	c.logger.Info("mcp", "tool_discovery_succeeded", "MCP tool discovery succeeded", logging.Fields{"stage": "discover", "status": "discovered", "tool_count": len(out)})
	return out, nil
}
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error) {
	if !c.initialized {
		return CallResult{}, fmt.Errorf("tools/call: client is not initialized")
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	c.logger.Info("mcp", "remote_tool_call_started", "MCP remote tool call started", logging.Fields{"stage": "call", "status": "started", "remote_tool": name})
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
		c.logger.Error("mcp", "remote_tool_call_failed", "MCP remote tool call failed", logging.Fields{"stage": "call", "status": "rpc_failed", "remote_tool": name})
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
	status := "succeeded"
	if result.IsError {
		status = "tool_error"
	}
	c.logger.Info("mcp", "remote_tool_call_completed", "MCP remote tool call completed", logging.Fields{"stage": "call", "status": status, "remote_tool": name})
	return CallResult{Text: text, Content: result.Content, StructuredContent: result.StructuredContent, IsError: result.IsError}, nil
}
func (c *Client) Close(ctx context.Context) error { return c.session.Close(ctx) }
