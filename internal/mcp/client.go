package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

const (
	legacyProtocolVersion   = "2025-06-18"
	modernProtocolVersion   = "2026-07-28"
	defaultDiscoveryTimeout = 2 * time.Second
)

type Lifecycle string

const (
	LifecycleUnknown Lifecycle = ""
	LifecycleModern  Lifecycle = "modern"
	LifecycleLegacy  Lifecycle = "legacy"
)

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
	session          *Session
	logger           *logging.Logger
	initialized      bool
	lifecycle        Lifecycle
	discoveryTimeout time.Duration
	toolCache        struct {
		tools   []RemoteTool
		expires time.Time
	}
}

type modernTransport interface{ SetModern(bool) }

func NewClient(transport Transport, loggers ...*logging.Logger) *Client {
	return &Client{session: NewSession(transport), logger: normalizedLogger(loggers), discoveryTimeout: defaultDiscoveryTimeout}
}

func (c *Client) setModern(modern bool) {
	if transport, ok := c.session.transport.(modernTransport); ok {
		transport.SetModern(modern)
	}
}

func modernMeta() map[string]any {
	return map[string]any{
		"io.modelcontextprotocol/protocolVersion":    modernProtocolVersion,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "mewcode", "version": "0.1"},
	}
}

func (c *Client) modernRequest(ctx context.Context, method string, params map[string]any, result any) error {
	params["_meta"] = modernMeta()
	return c.session.Request(ctx, method, params, result)
}

func (c *Client) Negotiate(ctx context.Context) error {
	if c.lifecycle != LifecycleUnknown {
		return nil
	}
	c.logger.Info("MCP lifecycle negotiation started", logging.Fields{"stage": "negotiate", "status": "started"})
	c.setModern(true)
	probeCtx, cancel := context.WithTimeout(ctx, c.discoveryTimeout)
	defer cancel()
	var result struct {
		SupportedVersions []string `json:"supportedVersions"`
	}
	err := c.modernRequest(probeCtx, "server/discover", map[string]any{}, &result)
	if err == nil && hasVersion(result.SupportedVersions, modernProtocolVersion) {
		c.lifecycle = LifecycleModern
		c.logger.Info("MCP lifecycle negotiation succeeded", logging.Fields{"stage": "negotiate", "status": "modern"})
		return nil
	}
	if err == nil || canFallback(err, probeCtx, ctx) {
		c.setModern(false)
		c.logger.Info("MCP lifecycle fallback started", logging.Fields{"stage": "negotiate", "status": "legacy_fallback"})
		if err := c.Initialize(ctx); err != nil {
			return err
		}
		c.lifecycle = LifecycleLegacy
		c.logger.Info("MCP lifecycle fallback succeeded", logging.Fields{"stage": "negotiate", "status": "legacy"})
		return nil
	}
	c.setModern(false)
	c.logger.Error("MCP lifecycle negotiation failed", logging.Fields{"stage": "negotiate", "status": "failed"})
	return fmt.Errorf("server/discover: %w", err)
}

func hasVersion(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func canFallback(err error, probeCtx, parent context.Context) bool {
	if errors.Is(err, context.DeadlineExceeded) && parent.Err() == nil && probeCtx.Err() != nil {
		return true
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		return rpcErr.Code == -32601 || rpcErr.Code == -32022
	}
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status == 404 || statusErr.Status == 405
	}
	return false
}

func (c *Client) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	c.setModern(false)
	c.logger.Info("MCP initialization started", logging.Fields{"stage": "initialize", "status": "started"})
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.session.Request(ctx, "initialize", map[string]any{"protocolVersion": legacyProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "mewcode", "version": "0.1"}}, &result); err != nil {
		c.logger.Error("MCP initialization failed", logging.Fields{"stage": "initialize", "status": "initialize_failed"})
		return fmt.Errorf("initialize: %w", err)
	}
	if result.ProtocolVersion == "" {
		c.logger.Error("MCP initialization failed", logging.Fields{"stage": "initialize", "status": "initialize_failed"})
		return fmt.Errorf("initialize: server did not select a protocol version")
	}
	if err := c.session.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		c.logger.Error("MCP initialized notification failed", logging.Fields{"stage": "initialize", "status": "notification_failed"})
		return fmt.Errorf("initialized notification: %w", err)
	}
	c.initialized = true
	if c.lifecycle == LifecycleUnknown {
		c.lifecycle = LifecycleLegacy
	}
	c.logger.Info("MCP initialization succeeded", logging.Fields{"stage": "initialize", "status": "initialized"})
	c.logger.Info("MCP initialized notification sent", logging.Fields{"stage": "initialize", "status": "notification_sent"})
	return nil
}
func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	if err := c.Negotiate(ctx); err != nil {
		return nil, err
	}
	if time.Now().Before(c.toolCache.expires) {
		return append([]RemoteTool(nil), c.toolCache.tools...), nil
	}
	c.logger.Info("MCP tool discovery started", logging.Fields{"stage": "discover", "status": "started"})
	var result struct {
		Tools []struct {
			Name        string       `json:"name"`
			Description string       `json:"description"`
			InputSchema tools.Schema `json:"inputSchema"`
		} `json:"tools"`
		TTLMS      int64  `json:"ttlMs"`
		CacheScope string `json:"cacheScope"`
	}
	var err error
	if c.lifecycle == LifecycleModern {
		err = c.modernRequest(ctx, "tools/list", map[string]any{}, &result)
	} else {
		err = c.session.Request(ctx, "tools/list", map[string]any{}, &result)
	}
	if err != nil {
		c.logger.Error("MCP tool discovery failed", logging.Fields{"stage": "discover", "status": "discover_failed"})
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
	if result.TTLMS > 0 && (result.CacheScope == "" || result.CacheScope == "public" || result.CacheScope == "private") {
		c.toolCache.tools = append([]RemoteTool(nil), out...)
		c.toolCache.expires = time.Now().Add(time.Duration(result.TTLMS) * time.Millisecond)
	}
	c.logger.Info("MCP tool discovery succeeded", logging.Fields{"stage": "discover", "status": "discovered", "tool_count": len(out)})
	return out, nil
}
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error) {
	if err := c.Negotiate(ctx); err != nil {
		return CallResult{}, err
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	c.logger.Info("MCP remote tool call started", logging.Fields{"stage": "call", "status": "started", "remote_tool": name})
	var arguments any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return CallResult{}, fmt.Errorf("tools/call: arguments: %w", err)
	}
	var result struct {
		Content           []any `json:"content"`
		StructuredContent any   `json:"structuredContent"`
		IsError           bool  `json:"isError"`
	}
	params := map[string]any{"name": name, "arguments": arguments}
	var requestErr error
	if c.lifecycle == LifecycleModern {
		requestErr = c.modernRequest(ctx, "tools/call", params, &result)
	} else {
		requestErr = c.session.Request(ctx, "tools/call", params, &result)
	}
	if requestErr != nil {
		c.logger.Error("MCP remote tool call failed", logging.Fields{"stage": "call", "status": "rpc_failed", "remote_tool": name})
		return CallResult{}, fmt.Errorf("tools/call: %w", requestErr)
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
	c.logger.Info("MCP remote tool call completed", logging.Fields{"stage": "call", "status": status, "remote_tool": name})
	return CallResult{Text: text, Content: result.Content, StructuredContent: result.StructuredContent, IsError: result.IsError}, nil
}
func (c *Client) Close(ctx context.Context) error { return c.session.Close(ctx) }
