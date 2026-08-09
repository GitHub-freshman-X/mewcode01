package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestClientInitializesListsAndCallsTool(t *testing.T) {
	transport := newScriptedTransport(t, func(request map[string]any) Inbound {
		id, hasID := request["id"]
		if !hasID {
			return Inbound{}
		}
		result := any(map[string]any{})
		switch request["method"] {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18"}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo text", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}}
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
		return Inbound{Message: body}
	})
	client := NewClient(transport)
	defer client.Close(context.Background())
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	tools, err := client.ListTools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools=%+v err=%v", tools, err)
	}
	result, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`))
	if err != nil || result.Text != "ok" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
