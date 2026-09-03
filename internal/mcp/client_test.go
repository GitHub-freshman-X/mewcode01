package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestClientUsesModernLifecycleAndHTTPHeaders(t *testing.T) {
	type captured struct {
		method  string
		headers http.Header
		params  map[string]any
	}
	var requests []captured
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if r.Method == http.MethodDelete {
			t.Fatalf("modern client sent DELETE")
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, captured{request.Method, r.Header.Clone(), request.Params})
		result := any(map[string]any{})
		switch request.Method {
		case "server/discover":
			result = map[string]any{"supportedVersions": []string{modernProtocolVersion}, "capabilities": map[string]any{}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo", "inputSchema": map[string]any{"type": "object"}}}, "ttlMs": 1000, "cacheScope": "private"}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}}
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	client := NewClient(NewHTTPTransport(server.URL, nil, server.Client()))
	defer client.Close(context.Background())
	if _, err := client.ListTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if result, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`)); err != nil || result.Text != "ok" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if client.lifecycle != LifecycleModern || len(requests) != 3 {
		t.Fatalf("lifecycle=%s requests=%+v", client.lifecycle, requests)
	}
	for _, request := range requests {
		if request.method == "initialize" || request.headers.Get("MCP-Session-Id") != "" || request.headers.Get("MCP-Protocol-Version") != modernProtocolVersion || request.headers.Get("Mcp-Method") != request.method {
			t.Fatalf("request=%+v", request)
		}
		meta, ok := request.params["_meta"].(map[string]any)
		if !ok || meta["io.modelcontextprotocol/protocolVersion"] != modernProtocolVersion {
			t.Fatalf("params=%v", request.params)
		}
	}
	if requests[2].headers.Get("Mcp-Name") != "echo" {
		t.Fatalf("call headers=%v", requests[2].headers)
	}
}

func TestClientFallsBackToLegacyAfterDiscoverMethodNotFound(t *testing.T) {
	var methods []string
	transport := newScriptedTransport(t, func(request map[string]any) Inbound {
		method, _ := request["method"].(string)
		methods = append(methods, method)
		id, ok := request["id"]
		if !ok {
			return Inbound{}
		}
		response := map[string]any{"jsonrpc": "2.0", "id": id}
		switch method {
		case "server/discover":
			response["error"] = map[string]any{"code": -32601, "message": "method not found"}
		case "initialize":
			response["result"] = map[string]any{"protocolVersion": legacyProtocolVersion}
		case "tools/list":
			response["result"] = map[string]any{"tools": []any{}}
		default:
			t.Fatalf("unexpected method %q", method)
		}
		body, _ := json.Marshal(response)
		return Inbound{Message: body}
	})
	client := NewClient(transport)
	defer client.Close(context.Background())
	if _, err := client.ListTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.lifecycle != LifecycleLegacy {
		t.Fatalf("lifecycle=%s", client.lifecycle)
	}
	want := []string{"server/discover", "initialize", "notifications/initialized", "tools/list"}
	if len(methods) != len(want) {
		t.Fatalf("methods=%v", methods)
	}
	for i := range want {
		if methods[i] != want[i] {
			t.Fatalf("methods=%v", methods)
		}
	}
}

func TestClientDoesNotFallbackOnHTTPUnauthorized(t *testing.T) {
	var initializeCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Method == "initialize" {
			initializeCalls++
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := NewClient(NewHTTPTransport(server.URL, nil, server.Client()))
	defer client.Close(context.Background())
	if err := client.Negotiate(context.Background()); err == nil {
		t.Fatal("expected negotiation failure")
	}
	if initializeCalls != 0 {
		t.Fatalf("initializeCalls=%d", initializeCalls)
	}
}

func TestClientFallsBackAfterDiscoveryTimeout(t *testing.T) {
	transport := newScriptedTransport(t, func(request map[string]any) Inbound {
		method, _ := request["method"].(string)
		if method == "server/discover" {
			return Inbound{}
		}
		id, ok := request["id"]
		if !ok {
			return Inbound{}
		}
		result := any(map[string]any{"protocolVersion": legacyProtocolVersion})
		if method == "tools/list" {
			result = map[string]any{"tools": []any{}}
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
		return Inbound{Message: body}
	})
	client := NewClient(transport)
	client.discoveryTimeout = time.Millisecond
	defer client.Close(context.Background())
	if _, err := client.ListTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.lifecycle != LifecycleLegacy {
		t.Fatalf("lifecycle=%s", client.lifecycle)
	}
}

func TestClientUsesModernLifecycleOverStdio(t *testing.T) {
	var methods []string
	transport := newScriptedTransport(t, func(request map[string]any) Inbound {
		method, _ := request["method"].(string)
		methods = append(methods, method)
		params, _ := request["params"].(map[string]any)
		meta, _ := params["_meta"].(map[string]any)
		if meta["io.modelcontextprotocol/protocolVersion"] != modernProtocolVersion {
			t.Fatalf("params=%v", params)
		}
		id, ok := request["id"]
		if !ok {
			return Inbound{}
		}
		result := any(map[string]any{"tools": []any{}})
		if method == "server/discover" {
			result = map[string]any{"supportedVersions": []string{modernProtocolVersion}, "capabilities": map[string]any{}}
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
		return Inbound{Message: body}
	})
	client := NewClient(transport)
	defer client.Close(context.Background())
	if _, err := client.ListTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.lifecycle != LifecycleModern || len(methods) != 2 || methods[0] != "server/discover" || methods[1] != "tools/list" {
		t.Fatalf("lifecycle=%s methods=%v", client.lifecycle, methods)
	}
}

func TestClientLegacyHTTPFallbackKeepsSessionAndCloses(t *testing.T) {
	var methods []string
	var toolHeaders http.Header
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes++
			if r.Header.Get("MCP-Session-Id") != "session" {
				t.Fatalf("delete headers=%v", r.Header)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		defer r.Body.Close()
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		methods = append(methods, request.Method)
		if request.Method == "server/discover" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if request.Method == "tools/list" {
			toolHeaders = r.Header.Clone()
		}
		if request.Method == "initialize" {
			w.Header().Set("MCP-Session-Id", "session")
		}
		result := any(map[string]any{"protocolVersion": legacyProtocolVersion})
		if request.Method == "tools/list" {
			result = map[string]any{"tools": []any{}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	client := NewClient(NewHTTPTransport(server.URL, nil, server.Client()))
	if _, err := client.ListTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.lifecycle != LifecycleLegacy || toolHeaders.Get("MCP-Session-Id") != "session" || deletes != 1 {
		t.Fatalf("lifecycle=%s toolHeaders=%v deletes=%d methods=%v", client.lifecycle, toolHeaders, deletes, methods)
	}
}
