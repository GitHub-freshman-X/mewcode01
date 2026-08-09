package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type RemoteToolAdapter struct {
	server string
	remote RemoteTool
	client *Client
	logger *logging.Logger
}

func NewRemoteToolAdapter(server string, remote RemoteTool, client *Client, loggers ...*logging.Logger) *RemoteToolAdapter {
	return &RemoteToolAdapter{server: server, remote: remote, client: client, logger: normalizedLogger(loggers)}
}
func (t *RemoteToolAdapter) Metadata() tools.Metadata {
	return tools.Metadata{Name: t.server + "__" + t.remote.Name, Description: t.remote.Description, Schema: t.remote.InputSchema, Safety: tools.SafetySideEffect, Permission: tools.PermissionMetadata{Target: tools.PermissionTargetNone}}
}
func (t *RemoteToolAdapter) Execute(ctx context.Context, input json.RawMessage) tools.Result {
	meta := t.Metadata()
	if _, err := tools.Validate(meta.Schema, input); err != nil {
		t.logger.Error("mcp", "tool_call_failed", "MCP tool call failed", logging.Fields{"stage": "call", "status": "validation_failed", "remote_tool": t.remote.Name, "tool": meta.Name})
		return tools.Failure(meta.Name, err.Type, err.Message, err.Details)
	}
	t.logger.Info("mcp", "tool_call_started", "MCP tool call started", logging.Fields{"stage": "call", "status": "started", "remote_tool": t.remote.Name, "tool": meta.Name})
	result, err := t.client.CallTool(ctx, t.remote.Name, input)
	if err != nil {
		t.logger.Error("mcp", "tool_call_failed", "MCP tool call failed", logging.Fields{"stage": "call", "status": "rpc_failed", "remote_tool": t.remote.Name, "tool": meta.Name})
		return tools.Failure(meta.Name, tools.ErrorExecution, "MCP tool call failed", map[string]any{"cause": safeError(err)})
	}
	if result.IsError {
		t.logger.Error("mcp", "tool_call_failed", "MCP tool call failed", logging.Fields{"stage": "call", "status": "tool_error", "remote_tool": t.remote.Name, "tool": meta.Name})
		return tools.Failure(meta.Name, tools.ErrorExecution, "MCP tool reported an error", map[string]any{"text": result.Text, "content": result.Content})
	}
	data := map[string]any{"text": result.Text, "content": result.Content}
	if result.StructuredContent != nil {
		data["structured_content"] = result.StructuredContent
	}
	t.logger.Info("mcp", "tool_call_succeeded", "MCP tool call succeeded", logging.Fields{"stage": "call", "status": "succeeded", "remote_tool": t.remote.Name, "tool": meta.Name})
	return tools.Success(meta.Name, data)
}
func safeError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
