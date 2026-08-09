package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func TestRemoteToolAdapterUsesNamespaceAndSideEffectSafety(t *testing.T) {
	client := NewClient(newScriptedTransport(t, func(request map[string]any) Inbound {
		result := any(map[string]any{"content": []any{map[string]any{"type": "text", "text": "done"}}})
		if request["method"] == "initialize" {
			result = map[string]any{"protocolVersion": "2025-06-18"}
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result})
		return Inbound{Message: body}
	}))
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	adapter := NewRemoteToolAdapter("server", RemoteTool{Name: "run", Description: "Run", InputSchema: tools.Schema{"type": "object"}}, client)
	meta := adapter.Metadata()
	if meta.Name != "server__run" || meta.Safety != tools.SafetySideEffect || meta.Permission.Target != tools.PermissionTargetNone {
		t.Fatalf("metadata=%+v", meta)
	}
	result := adapter.Execute(context.Background(), json.RawMessage(`{}`))
	if !result.Success || result.Data["text"] != "done" {
		t.Fatalf("result=%+v", result)
	}
}
