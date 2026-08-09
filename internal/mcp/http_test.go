package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPTransportCarriesSessionAndParsesSSE(t *testing.T) {
	var requests []http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Header.Clone())
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Method == "initialize" {
			w.Header().Set("MCP-Session-Id", "session")
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/event-stream")
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"protocolVersion": "2025-06-18"}})
		if request.Method == "initialize" {
			_, _ = w.Write(body)
		} else {
			_, _ = w.Write([]byte("data: " + string(body) + "\n\n"))
		}
	}))
	defer server.Close()
	transport := NewHTTPTransport(server.URL, map[string]string{"Authorization": "Bearer secret"}, server.Client())
	if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)); err != nil {
		t.Fatal(err)
	}
	<-transport.Receive()
	if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)); err != nil {
		t.Fatal(err)
	}
	if inbound := <-transport.Receive(); len(inbound.Message) == 0 || inbound.Err != nil {
		t.Fatalf("inbound=%+v", inbound)
	}
	if len(requests) != 2 || requests[1].Get("MCP-Session-Id") != "session" || requests[1].Get("MCP-Protocol-Version") == "" {
		t.Fatalf("headers=%v", requests)
	}
}
