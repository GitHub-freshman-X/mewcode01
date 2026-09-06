package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestRequestStream(t *testing.T) {
	var got requestBody
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/v1/responses" || r.Header.Get("Authorization") != "Bearer canary" {
			t.Errorf("request path/headers wrong")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range []string{`{"type":"response.created"}`, `{"type":"response.output_text.delta","delta":"你"}`, `{"type":"response.output_text.delta","delta":"好"}`, `{"type":"response.completed","response":{"usage":{"input_tokens":9,"output_tokens":2}}}`} {
			_, _ = w.Write([]byte("data: " + frame + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer s.Close()
	u, _ := url.Parse(s.URL + "/proxy")
	p := New(Options{BaseURL: u, APIKey: "canary", Model: "gpt", HTTPClient: s.Client()})
	messages := []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "hi"}}}, {Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockThinking, Text: "hidden", Signature: "sig"}, {Type: provider.BlockText, Text: "ok"}}}}
	events, done := p.Stream(context.Background(), provider.ChatRequest{Messages: messages, MaxTokens: 42})
	var text string
	var usage *provider.Usage
	for e := range events {
		if e.Type == provider.EventTextDelta {
			text += e.Delta
		}
		if e.Type == provider.EventUsage {
			usage = e.Usage
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if text != "你好" || usage == nil || usage.InputTokens != 9 || usage.OutputTokens != 2 || got.MaxOutputTokens != 42 || len(got.Input[1].Content) != 1 {
		t.Fatalf("text=%q body=%+v", text, got)
	}
}

func TestStreamCapturesFinalRequestPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()
	root := t.TempDir()
	logger, err := logging.New(root, func() time.Time { return time.Unix(1, 0).UTC() }, 1)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(server.URL)
	p := New(Options{BaseURL: u, APIKey: "secret", Model: "gpt", HTTPClient: server.Client(), Logger: logger})
	events, done := p.Stream(context.Background(), provider.ChatRequest{Messages: []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "request-canary"}}}}})
	for range events {
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*", "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	payload, err := os.ReadFile(files[0])
	if err != nil || strings.Contains(string(payload), "request-canary") || strings.Contains(string(payload), "secret") {
		t.Fatalf("payload=%s err=%v", payload, err)
	}
}

func TestErrorsSafe(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "canary-secret", 401) }))
	defer s.Close()
	u, _ := url.Parse(s.URL)
	p := New(Options{BaseURL: u, APIKey: "canary-secret", Model: "x", HTTPClient: s.Client()})
	_, done := p.Stream(context.Background(), provider.ChatRequest{Messages: []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "x"}}}}, MaxTokens: 10})
	err := <-done
	if err == nil || strings.Contains(err.Error(), "canary-secret") {
		t.Fatalf("err=%v", err)
	}
}

func TestTextOutputItemDoneIsIgnored(t *testing.T) {
	event, emit, err := parseEvent([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"message"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if emit || event.Type != "" {
		t.Fatalf("event=%+v emit=%v", event, emit)
	}
}

func TestFunctionOutputItemDoneEmitsToolDone(t *testing.T) {
	event, emit, err := parseEvent([]byte(`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"README.md\"}"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !emit || event.Type != provider.EventToolCallDone || event.BlockIndex != 2 || event.ToolCall.ID != "call_1" || event.ToolCall.Name != "read_file" || event.ToolCall.Arguments != `{"path":"README.md"}` {
		t.Fatalf("event=%+v emit=%v", event, emit)
	}
}

func TestHostedToolSearchEventsAreIgnored(t *testing.T) {
	frames := [][]byte{
		[]byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"tool_search_call","execution":"server","call_id":null,"status":"in_progress","arguments":{"paths":["mcp_demo"]}}}`),
		[]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"tool_search_call","execution":"server","call_id":null,"status":"completed","arguments":{"paths":["mcp_demo"]}}}`),
		[]byte(`{"type":"response.output_item.done","output_index":1,"item":{"type":"tool_search_output","execution":"server","call_id":null,"status":"completed","tools":[]}}`),
	}
	for _, frame := range frames {
		event, emit, err := parseEvent(frame)
		if err != nil || emit || event.Type != "" {
			t.Fatalf("event=%+v emit=%v err=%v", event, emit, err)
		}
	}
}

func TestBuildRequestPromptSystemOrder(t *testing.T) {
	body, err := buildRequest("gpt", provider.ChatRequest{
		Prompt: provider.PromptBundle{
			StableSystem: "stable system",
			DynamicSystem: []provider.SystemMessage{
				{Tag: "mew.environment", Content: "Workspace: /tmp/project"},
				{Tag: "mew.mode.plan", Content: "Plan mode"},
			},
			CachePolicy: provider.CachePolicy{Enable: true, StableSystem: true, StableTools: true},
		},
		Messages: []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "user task"}}}},
		Tools:    []provider.ToolDefinition{{Name: "read_file", Description: "Read", Schema: map[string]any{"type": "object"}, Cacheable: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Input) != 4 {
		t.Fatalf("input count=%d body=%+v", len(body.Input), body)
	}
	if body.Input[0].Role != "system" || body.Input[0].Content[0].Text != "stable system" {
		t.Fatalf("stable system not first: %+v", body.Input)
	}
	if body.Input[1].Role != "system" || !strings.Contains(body.Input[1].Content[0].Text, `tag="mew.environment"`) {
		t.Fatalf("environment system not second: %+v", body.Input)
	}
	if body.Input[2].Role != "system" || !strings.Contains(body.Input[2].Content[0].Text, `tag="mew.mode.plan"`) {
		t.Fatalf("mode system not third: %+v", body.Input)
	}
	if body.Input[3].Role != provider.RoleUser || body.Input[3].Content[0].Text != "user task" {
		t.Fatalf("user task not after system messages: %+v", body.Input)
	}
	if len(body.Tools) != 1 || body.Tools[0].Name != "read_file" {
		t.Fatalf("tools not stable: %+v", body.Tools)
	}
}

func TestParseCachedTokensUsage(t *testing.T) {
	event, emit, err := parseEvent([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":9,"output_tokens":2,"input_tokens_details":{"cached_tokens":6,"cache_write_tokens":1}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !emit || event.Type != provider.EventCompleted || event.Usage == nil {
		t.Fatalf("event=%+v emit=%v", event, emit)
	}
	if event.Usage.InputTokens != 9 || event.Usage.OutputTokens != 2 || event.Usage.CacheReadInputTokens != 6 || event.Usage.CacheCreationInputTokens != 1 || event.Usage.CacheTokensIncludedInInput != 7 {
		t.Fatalf("usage=%+v", event.Usage)
	}
}

func TestBuildRequestUsesNamespacesForDeferredMCPTools(t *testing.T) {
	defs := []provider.ToolDefinition{{Name: "read_file", Description: "Read", Schema: map[string]any{"type": "object"}}}
	for i := 0; i < 11; i++ {
		defs = append(defs, provider.ToolDefinition{Name: fmt.Sprintf("github__tool_%02d", i), Description: "GitHub", Schema: map[string]any{"type": "object"}, MCPServer: "github"})
	}
	body, err := buildRequest("gpt-5.4", provider.ChatRequest{Tools: defs, ToolSearch: provider.ToolSearchConfig{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) != 4 || body.Tools[0].Name != "read_file" || body.Tools[1].Type != "namespace" || len(body.Tools[1].Tools) != 10 || body.Tools[2].Name != "mcp_github_2" || len(body.Tools[2].Tools) != 1 || body.Tools[3].Type != "tool_search" || body.Tools[3].Execution != "server" {
		t.Fatalf("tools=%+v", body.Tools)
	}
	if !body.Tools[1].Tools[0].DeferLoading || body.Tools[1].Tools[0].Name != "github__tool_00" {
		t.Fatalf("namespace member=%+v", body.Tools[1].Tools[0])
	}
}

func TestBuildRequestFallbackKeepsFlatTools(t *testing.T) {
	body, err := buildRequest("gpt-4.1", provider.ChatRequest{Tools: []provider.ToolDefinition{{Name: "github__issue", Description: "Issue", Schema: map[string]any{"type": "object"}, MCPServer: "github"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) != 1 || body.Tools[0].Type != "function" || body.Tools[0].DeferLoading || len(body.Tools[0].Tools) != 0 {
		t.Fatalf("tools=%+v", body.Tools)
	}
}
