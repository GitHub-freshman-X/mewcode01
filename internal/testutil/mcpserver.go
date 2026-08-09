package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

// MCPRequest is a safely inspectable request captured by the controlled test server.
type MCPRequest struct {
	Method string
	ID     any
	Params json.RawMessage
	Header http.Header
}
type MCPHandler func(MCPRequest) (result any, sessionID string, sse bool, status int)
type MCPServer struct {
	Server   *httptest.Server
	mu       sync.Mutex
	Requests []MCPRequest
	Handler  MCPHandler
}

func NewMCPServer(handler MCPHandler) *MCPServer {
	s := &MCPServer{Handler: handler}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		defer r.Body.Close()
		var request struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		captured := MCPRequest{Method: request.Method, ID: request.ID, Params: request.Params, Header: r.Header.Clone()}
		s.mu.Lock()
		s.Requests = append(s.Requests, captured)
		s.mu.Unlock()
		result, sessionID, sse, status := handler(captured)
		if status == 0 {
			status = http.StatusOK
		}
		if sessionID != "" {
			w.Header().Set("MCP-Session-Id", sessionID)
		}
		if request.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		if sse {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: " + string(body) + "\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	return s
}
func (s *MCPServer) Close() { s.Server.Close() }
