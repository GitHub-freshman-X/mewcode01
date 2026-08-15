package command

import (
	"context"
	"strings"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestRegistryRejectsConflicts(t *testing.T) {
	handler := func(CommandContext) error { return nil }
	for _, commands := range [][]Command{
		{{Name: "one", Handler: handler}, {Name: "one", Handler: handler}},
		{{Name: "one", Aliases: []string{"x"}, Handler: handler}, {Name: "x", Handler: handler}},
		{{Name: "one", Aliases: []string{"x"}, Handler: handler}, {Name: "two", Aliases: []string{"x"}, Handler: handler}},
	} {
		if _, err := NewRegistry(commands...); err == nil || !strings.Contains(err.Error(), "x") && !strings.Contains(err.Error(), "one") {
			t.Fatalf("conflict error = %v", err)
		}
	}
}

func TestParseAndComplete(t *testing.T) {
	if got := Parse(" /HELP  topic "); !got.IsCommand || got.Name != "help" || got.Args != "topic" {
		t.Fatalf("parse=%+v", got)
	}
	if got := Parse("plain"); got.IsCommand {
		t.Fatalf("parse=%+v", got)
	}
	r := DefaultRegistry()
	if got := r.Complete("/he"); len(got) != 1 || got[0] != "/help" {
		t.Fatalf("complete=%v", got)
	}
	if c, ok := r.Find("H"); !ok || c.Name != "help" {
		t.Fatalf("alias=%+v %v", c, ok)
	}
}

type fakeUI struct {
	messages []string
	requests []agent.Request
	plan     bool
}

func (u *fakeUI) AddSystemMessage(s string)        { u.messages = append(u.messages, s) }
func (u *fakeUI) StartAgent(r agent.Request) error { u.requests = append(u.requests, r); return nil }
func (u *fakeUI) SetPlanMode(v bool)               { u.plan = v }
func (u *fakeUI) PlanMode() bool                   { return u.plan }
func (u *fakeUI) TokenUsage() provider.Usage       { return provider.Usage{} }
func (u *fakeUI) RefreshStatus()                   {}
func (u *fakeUI) MemoryClearPending() bool         { return false }
func (u *fakeUI) SetMemoryClearPending(bool)       {}

type fakeSessions struct {
	current SessionMeta
	list    []SessionMeta
}

func (s fakeSessions) Current() SessionMeta                 { return s.current }
func (s fakeSessions) List() ([]SessionMeta, error)         { return s.list, nil }
func (s fakeSessions) New(context.Context) error            { return nil }
func (s fakeSessions) Resume(context.Context, string) error { return nil }
func (s fakeSessions) Delete(string) error                  { return nil }

func TestDispatchLocalAndPrompt(t *testing.T) {
	u := &fakeUI{}
	ctx := CommandContext{Context: context.Background(), UI: u}
	if err := Dispatch(DefaultRegistry(), Parse("/help"), ctx); err != nil || len(u.messages) != 1 || len(u.requests) != 0 {
		t.Fatalf("help messages=%v requests=%v err=%v", u.messages, u.requests, err)
	}
	if err := Dispatch(DefaultRegistry(), Parse("/review concurrency"), ctx); err != nil || len(u.requests) != 1 || u.requests[0].Mode != agent.ModeAct || !strings.Contains(u.requests[0].Prompt, "concurrency") {
		t.Fatalf("review=%+v err=%v", u.requests, err)
	}
	if err := Dispatch(DefaultRegistry(), Parse("/plan task"), ctx); err != nil || !u.plan || len(u.requests) != 2 || u.requests[1].Mode != agent.ModePlan {
		t.Fatalf("plan=%+v mode=%v err=%v", u.requests, u.plan, err)
	}
}

func TestSessionListUsesTitle(t *testing.T) {
	u := &fakeUI{}
	longTitle := "  第一条消息含有中文和 emoji 😀，并且它足够长，应该在字符边界截断而不会破坏最后一个字符，同时还要保留省略号  "
	sessions := fakeSessions{list: []SessionMeta{
		{ID: "20260816-100000-a1b2", Title: longTitle, MessageCount: 99},
		{ID: "20260816-090000-c3d4"},
	}}
	ctx := CommandContext{Context: context.Background(), UI: u, Sessions: sessions}
	if err := Dispatch(DefaultRegistry(), Parse("/session list"), ctx); err != nil {
		t.Fatal(err)
	}
	if len(u.messages) != 1 {
		t.Fatalf("messages=%v", u.messages)
	}
	message := u.messages[0]
	if !strings.Contains(message, "20260816-100000-a1b2 — 第一条消息") || !strings.Contains(message, "…") {
		t.Fatalf("message=%q", message)
	}
	if !strings.Contains(message, "20260816-090000-c3d4 — （空会话）") || strings.Contains(message, "99 条消息") {
		t.Fatalf("message=%q", message)
	}
}

func TestFormatSessionTitle(t *testing.T) {
	if got := formatSessionTitle(" \n "); got != "（空会话）" {
		t.Fatalf("empty title=%q", got)
	}
	value := strings.Repeat("😀", sessionTitleLimit+1)
	if got := formatSessionTitle(value); got != strings.Repeat("😀", sessionTitleLimit)+"…" {
		t.Fatalf("title=%q", got)
	}
}
