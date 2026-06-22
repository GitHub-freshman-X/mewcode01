package conversation

import (
	"context"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"testing"
)

type fakeProvider struct {
	requests []provider.ChatRequest
	events   chan provider.StreamEvent
	done     chan error
}

func (f *fakeProvider) Stream(_ context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	f.requests = append(f.requests, req)
	return f.events, f.done
}

func TestCompleteMultiTurnThinkingAndCopy(t *testing.T) {
	f := &fakeProvider{events: make(chan provider.StreamEvent), done: make(chan error)}
	c := NewConversation(f, ChatOptions{MaxTokens: 4096, Thinking: provider.ThinkingOptions{Enabled: true, BudgetTokens: 1024}})
	if _, _, err := c.Start(context.Background(), "你好"); err != nil {
		t.Fatal(err)
	}
	for _, e := range []provider.StreamEvent{{Type: provider.EventStarted}, {Type: provider.EventThinkingDelta, BlockIndex: 0, Delta: "思考"}, {Type: provider.EventSignatureDelta, BlockIndex: 0, Delta: "secret-signature"}, {Type: provider.EventTextDelta, BlockIndex: 1, Delta: "回答"}, {Type: provider.EventCompleted}} {
		if err := c.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Complete(); err != nil {
		t.Fatal(err)
	}
	h := c.History()
	if len(h) != 2 || h[1].Blocks[0].Signature != "secret-signature" {
		t.Fatalf("history=%+v", h)
	}
	h[1].Blocks[1].Text = "mutated"
	if c.History()[1].Blocks[1].Text != "回答" {
		t.Fatal("history was mutated")
	}
	if _, _, err := c.Start(context.Background(), "第二轮"); err != nil {
		t.Fatal(err)
	}
	if got := len(f.requests[1].Messages); got != 3 {
		t.Fatalf("messages=%d", got)
	}
}

func TestCancelDoesNotCommit(t *testing.T) {
	f := &fakeProvider{events: make(chan provider.StreamEvent), done: make(chan error)}
	c := NewConversation(f, ChatOptions{MaxTokens: 10})
	if _, _, err := c.Start(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	_ = c.Apply(provider.StreamEvent{Type: provider.EventTextDelta, Delta: "partial"})
	c.Cancel()
	if len(c.History()) != 0 || c.ActiveTurn().State != TurnCancelled {
		t.Fatal("cancel boundary failed")
	}
	if _, _, err := c.Start(context.Background(), "again"); err != nil {
		t.Fatal(err)
	}
}
