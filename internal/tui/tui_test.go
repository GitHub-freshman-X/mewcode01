package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestActPlanDoCommands(t *testing.T) {
	tests := []struct {
		input  string
		mode   agent.Mode
		prompt string
	}{
		{"hello", agent.ModeAct, "hello"},
		{"/plan inspect this", agent.ModePlan, "inspect this"},
		{"/do", agent.ModeDo, ""},
	}
	for _, test := range tests {
		req, err := parseRequest(test.input)
		if err != nil || req.Mode != test.mode || req.Prompt != test.prompt {
			t.Fatalf("%q: req=%+v err=%v", test.input, req, err)
		}
	}
	if _, err := parseRequest("/plan"); err == nil {
		t.Fatal("empty plan accepted")
	}
}

func TestViewAgentEventPartialUsage(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if view := m.View().Content; !strings.Contains(view, "idle") {
		t.Fatalf("view=%q", view)
	}
	m.current.prompt = "hello"
	m.applyAgentEvent(agent.Event{Type: agent.EventTextDelta, Iteration: 1, Phase: agent.PhaseStreaming, Text: "partial"})
	m.applyAgentEvent(agent.Event{Type: agent.EventUsage, Iteration: 1, Usage: &provider.Usage{InputTokens: 3, OutputTokens: 2}})
	m.applyAgentEvent(agent.Event{Type: agent.EventFailed, Iteration: 1, Summary: &agent.Summary{Reason: agent.StopStreamError, Partial: true}, Err: errors.New("boom")})
	m.refreshContent()
	view := m.View().Content
	if !strings.Contains(view, "partial") || !strings.Contains(view, "部分输出") || !strings.Contains(view, "failed") {
		t.Fatalf("view=%q", view)
	}
}

func TestDisplayHistoryShowsPlanWithoutModelHistory(t *testing.T) {
	session := conversation.NewSession()
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "/plan create hello.txt"}}}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "Create hello.txt with hello world"}}}
	if err := session.CommitPlan(user, assistant, "Create hello.txt with hello world"); err != nil {
		t.Fatal(err)
	}
	if len(session.Snapshot()) != 0 {
		t.Fatal("plan leaked into model history")
	}
	m := NewModel(nil, session)
	view := m.View().Content
	if !strings.Contains(view, "/plan create hello.txt") || !strings.Contains(view, "Create hello.txt with hello world") {
		t.Fatalf("view=%q", view)
	}
	if strings.Contains(view, "mew.environment") || strings.Contains(view, "mew.mode") {
		t.Fatalf("system prompt tag leaked into view: %q", view)
	}
}

func TestTextareaFocusedInputHasVisibleStyle(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	styles := m.textarea.Styles()
	if isNoColor(styles.Focused.Text.GetForeground()) {
		t.Fatal("focused textarea text should set an explicit foreground color")
	}
	if isNoColor(styles.Focused.CursorLine.GetForeground()) {
		t.Fatal("focused cursor line should set an explicit foreground color")
	}
	if isNoColor(styles.Focused.Prompt.GetForeground()) {
		t.Fatal("focused prompt should set an explicit foreground color")
	}
}

func isNoColor(value any) bool {
	return reflect.TypeOf(value) == reflect.TypeOf(lipgloss.NoColor{})
}
