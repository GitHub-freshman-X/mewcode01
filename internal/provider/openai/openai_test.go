package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestRequestStream(t *testing.T) {
	var got requestBody
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/responses" || r.Header.Get("Authorization") != "Bearer canary" {
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
	if !emit || event.Type != provider.EventToolCallDone || event.BlockIndex != 2 || event.ToolCall.ID != "call_1" || event.ToolCall.Name != "read_file" {
		t.Fatalf("event=%+v emit=%v", event, emit)
	}
}
