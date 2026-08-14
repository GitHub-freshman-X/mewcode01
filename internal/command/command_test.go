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
