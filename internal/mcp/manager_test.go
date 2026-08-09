package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func TestManagerRegistersHealthyServerAndIsolatesBrokenServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result := any(map[string]any{"protocolVersion": "2025-06-18"})
		if request.Method == "tools/list" {
			result = map[string]any{"tools": []any{map[string]any{"name": "search", "description": "Search", "inputSchema": map[string]any{"type": "object"}}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	registry := tools.NewRegistry()
	manager := NewManager(server.Client(), nil)
	diagnostics := manager.ConnectAndRegister(context.Background(), registry, map[string]config.MCPServerConfig{
		"healthy": {Type: config.MCPTransportHTTP, URL: server.URL},
		"broken":  {Type: config.MCPTransportStdio, Command: "/not/a/real/mcp-server"},
	})
	if _, ok := registry.Get("healthy__search"); !ok {
		t.Fatalf("healthy tool was not registered: %v", registry.List())
	}
	if len(diagnostics) != 1 || diagnostics[0].Server != "broken" {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
