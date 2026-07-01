package anthropic

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
