package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type fakeProvider struct {
	events  chan provider.StreamEvent
	done    chan error
	request provider.ChatRequest
}

func (f *fakeProvider) Stream(_ context.Context, r provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	f.request = r
	return f.events, f.done
}

func TestViewInputResizeAndStreamingThinking(t *testing.T) {
	f := &fakeProvider{events: make(chan provider.StreamEvent, 8), done: make(chan error, 1)}
	c := conversation.NewConversation(f, conversation.ChatOptions{MaxTokens: 4096})
	m := NewModel(c)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if view := m.View().Content; !strings.Contains(view, "idle") || !strings.Contains(view, "Enter 发送") {
		t.Fatalf("initial view=%q", view)
	}
	m.textarea.SetValue("你好")
	_, cmd := m.Update(key(tea.KeyEnter, 0))
	if cmd == nil || f.request.Messages[0].Blocks[0].Text != "你好" {
		t.Fatal("submit failed")
	}

	f.events <- provider.StreamEvent{Type: provider.EventStarted}
	_, cmd = m.Update(cmd())
	f.events <- provider.StreamEvent{Type: provider.EventThinkingDelta, BlockIndex: 0, Delta: "秘密思考"}
	_, cmd = m.Update(cmd())
	f.events <- provider.StreamEvent{Type: provider.EventSignatureDelta, BlockIndex: 0, Delta: "signature-must-hide"}
	_, cmd = m.Update(cmd())
	f.events <- provider.StreamEvent{Type: provider.EventTextDelta, BlockIndex: 1, Delta: "第一段"}
	_, cmd = m.Update(cmd())
	if view := m.View().Content; !strings.Contains(view, "秘密思考") || !strings.Contains(view, "第一段") || strings.Contains(view, "signature-must-hide") {
		t.Fatalf("stream view=%q", view)
	}
	f.events <- provider.StreamEvent{Type: provider.EventCompleted}
	_, cmd = m.Update(cmd())
	if view := m.View().Content; !strings.Contains(view, "第一段") || !strings.Contains(view, "已折叠") {
		t.Fatalf("completed view=%q", view)
	}
	f.done <- nil
	_, _ = m.Update(cmd())
	if !m.textarea.Focused() {
		t.Fatal("input did not regain focus")
	}
	m.Update(key('t', tea.ModCtrl))
	if !strings.Contains(m.View().Content, "秘密思考") {
		t.Fatal("thinking did not expand")
	}
}

func TestCancelAndIdleQuit(t *testing.T) {
	f := &fakeProvider{events: make(chan provider.StreamEvent), done: make(chan error, 1)}
	c := conversation.NewConversation(f, conversation.ChatOptions{MaxTokens: 10})
	m := NewModel(c)
	m.textarea.SetValue("x")
	_, _ = m.Update(key(tea.KeyEnter, 0))
	_, cmd := m.Update(key('c', tea.ModCtrl))
	if cmd == nil || c.ActiveTurn().State != conversation.TurnCancelled {
		t.Fatal("cancel failed")
	}
	f.done <- context.Canceled
	_, _ = m.Update(cmd())
	if len(c.History()) != 0 {
		t.Fatal("cancelled turn committed")
	}
	_, quit := m.Update(key('c', tea.ModCtrl))
	if quit == nil {
		t.Fatal("idle ctrl+c should quit")
	}
}

func key(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: mod})
}
