package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestRequestStreamThinking(t *testing.T) {
	var got requestBody
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/v1/messages" || r.Header.Get("x-api-key") != "canary" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("request path/headers wrong")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range []string{`{"type":"message_start","message":{"usage":{"input_tokens":7}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"想"}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`, `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"好"}}`, `{"type":"message_delta","usage":{"output_tokens":3}}`, `{"type":"message_stop"}`} {
			_, _ = w.Write([]byte("data: " + frame + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer s.Close()
	u, _ := url.Parse(s.URL + "/proxy")
	p := New(Options{BaseURL: u, APIKey: "canary", Model: "claude", HTTPClient: s.Client()})
	events, done := p.Stream(context.Background(), provider.ChatRequest{Messages: []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "hi"}}}}, MaxTokens: 4096, Thinking: provider.ThinkingOptions{Enabled: true, BudgetTokens: 1024}})
	var types []provider.EventType
	var usage *provider.Usage
	for e := range events {
		types = append(types, e.Type)
		if e.Type == provider.EventUsage {
			usage = e.Usage
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(types) != 6 || usage == nil || usage.InputTokens != 7 || usage.OutputTokens != 3 || got.Thinking == nil || !got.Stream {
		t.Fatalf("types=%v body=%+v", types, got)
	}
}

func TestErrorsSafeAndTruncated(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "canary-secret", status) }))
		u, _ := url.Parse(s.URL)
		p := New(Options{BaseURL: u, APIKey: "canary-secret", Model: "x", HTTPClient: s.Client()})
		_, done := p.Stream(context.Background(), provider.ChatRequest{Messages: []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "x"}}}}, MaxTokens: 10})
		err := <-done
		s.Close()
		if err == nil || strings.Contains(err.Error(), "canary-secret") {
			t.Fatalf("status=%d err=%v", status, err)
		}
	}
}

func TestBuildRequestPromptSystemAndCache(t *testing.T) {
	body, err := buildRequest("claude", provider.ChatRequest{
		Prompt: provider.PromptBundle{
			StableSystem: "stable system",
			DynamicSystem: []provider.SystemMessage{
				{Tag: "mew.environment", Content: "Workspace: /tmp/project"},
			},
			CachePolicy: provider.CachePolicy{Enable: true, StableSystem: true, StableTools: true},
		},
		Messages: []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "user task"}}}},
		Tools:    []provider.ToolDefinition{{Name: "read_file", Description: "Read", Schema: map[string]any{"type": "object"}, Cacheable: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(body.System) != 2 {
		t.Fatalf("system count=%d body=%+v", len(body.System), body)
	}
	if body.System[0].Text != "stable system" || body.System[0].CacheControl == nil {
		t.Fatalf("stable system missing cache control: %+v", body.System)
	}
	if body.System[1].CacheControl != nil || !strings.Contains(body.System[1].Text, `tag="mew.environment"`) {
		t.Fatalf("dynamic system should not be cacheable: %+v", body.System[1])
	}
	if len(body.Tools) != 1 || body.Tools[0].CacheControl == nil {
		t.Fatalf("stable tool missing cache control: %+v", body.Tools)
	}
	if len(body.Messages) != 1 || body.Messages[0].Content[0].Text != "user task" {
		t.Fatalf("messages wrong: %+v", body.Messages)
	}
}

func TestBuildRequestCachesOnlyLastStableTool(t *testing.T) {
	tools := make([]provider.ToolDefinition, 6)
	for i := range tools {
		tools[i] = provider.ToolDefinition{Name: fmt.Sprintf("tool_%d", i), Description: "Tool", Schema: map[string]any{"type": "object"}, Cacheable: true}
	}
	body, err := buildRequest("claude", provider.ChatRequest{
		Prompt:   provider.PromptBundle{StableSystem: "stable system", CachePolicy: provider.CachePolicy{Enable: true, StableSystem: true, StableTools: true}},
		Messages: []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "user task"}}}},
		Tools:    tools,
	})
	if err != nil {
		t.Fatal(err)
	}
	if body.System[0].CacheControl == nil {
		t.Fatal("stable system missing cache control")
	}
	for i, tool := range body.Tools {
		wantCached := i == len(body.Tools)-1
		if (tool.CacheControl != nil) != wantCached {
			t.Fatalf("tool %d cache control=%v, want cached=%v", i, tool.CacheControl != nil, wantCached)
		}
	}
}

func TestParseAnthropicCacheUsage(t *testing.T) {
	event, emit, err := parseEvent([]byte(`{"type":"message_start","message":{"usage":{"input_tokens":7,"cache_creation_input_tokens":3,"cache_read_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":11,"ephemeral_1h_input_tokens":13}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !emit || event.Usage == nil {
		t.Fatalf("event=%+v emit=%v", event, emit)
	}
	if event.Usage.InputTokens != 7 || event.Usage.CacheReadInputTokens != 5 || event.Usage.CacheCreationInputTokens != 27 {
		t.Fatalf("usage=%+v", event.Usage)
	}
}

func TestBuildRequestDefersOnlyMCPTools(t *testing.T) {
	body, err := buildRequest("claude", provider.ChatRequest{Tools: []provider.ToolDefinition{
		{Name: "read_file", Description: "Read", Schema: map[string]any{"type": "object"}},
		{Name: "github__issue", Description: "Issue", Schema: map[string]any{"type": "object"}, MCPServer: "github"},
	}, ToolSearch: provider.ToolSearchConfig{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) != 3 || body.Tools[0].DeferLoading || !body.Tools[1].DeferLoading || body.Tools[2].Type != "tool_search_tool_bm25_20251119" {
		t.Fatalf("tools=%+v", body.Tools)
	}
}

func TestToolSearchHistoryIsParsedAndReplayed(t *testing.T) {
	serverUse := []byte(`{"type":"server_tool_use","id":"srvtoolu_1","name":"tool_search_tool_bm25","input":{"query":"fixture"}}`)
	searchResult := []byte(`{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_1","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"demo__lookup_fixture"}]}}`)
	parser := newStreamParser()
	for _, frame := range [][]byte{
		[]byte(fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"tool_search_tool_bm25"}}`)),
		[]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"fixture\"}"}}`),
		[]byte(`{"type":"content_block_stop","index":0}`),
	} {
		event, emit, err := parser.parseEvent(frame)
		if err != nil {
			t.Fatal(err)
		}
		if string(frame) == `{"type":"content_block_stop","index":0}` && (!emit || event.Type != provider.EventProviderHistory || event.ProviderHistory == nil || string(event.ProviderHistory.Payload) != string(serverUse)) {
			t.Fatalf("event=%+v emit=%v", event, emit)
		}
	}
	event, emit, err := parser.parseEvent([]byte(fmt.Sprintf(`{"type":"content_block_start","index":1,"content_block":%s}`, searchResult)))
	if err != nil || !emit || event.Type != provider.EventProviderHistory || event.ProviderHistory == nil || string(event.ProviderHistory.Payload) != string(searchResult) {
		t.Fatalf("event=%+v emit=%v err=%v", event, emit, err)
	}
	body, err := buildRequest("claude", provider.ChatRequest{Messages: []provider.Message{{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{
		{Type: provider.BlockProviderHistory, ProviderHistory: &provider.ProviderHistory{Provider: "anthropic", Payload: serverUse}},
		{Type: provider.BlockProviderHistory, ProviderHistory: &provider.ProviderHistory{Provider: "anthropic", Payload: searchResult}},
		{Type: provider.BlockToolCall, ToolCall: &provider.ToolCall{ID: "toolu_1", Name: "demo__lookup_fixture", Arguments: []byte(`{}`)}},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body.Messages[0].Content)
	if err != nil || !strings.Contains(string(encoded), string(serverUse)) || !strings.Contains(string(encoded), string(searchResult)) {
		t.Fatalf("content=%s err=%v", encoded, err)
	}
}
