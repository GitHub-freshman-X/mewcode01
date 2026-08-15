package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type tuiScriptedProvider struct{}

func (tuiScriptedProvider) Stream(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	events := make(chan provider.StreamEvent, 3)
	done := make(chan error, 1)
	events <- provider.StreamEvent{Type: provider.EventStarted}
	events <- provider.StreamEvent{Type: provider.EventTextDelta, Delta: "planned"}
	events <- provider.StreamEvent{Type: provider.EventCompleted}
	close(events)
	done <- nil
	close(done)
	return events, done
}

func TestCommandPlanConsumesAgentEvents(t *testing.T) {
	registry, err := tools.NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session := conversation.NewSession()
	runner := agent.NewRunner(tuiScriptedProvider{}, session, registry, tools.NewExecutor(time.Second), agent.Options{})
	m := NewModel(runner, session)
	m.textarea.SetValue("/plan write a plan")
	_, cmd := m.Update(tea.KeyPressMsg{Text: keySubmit})
	if cmd == nil {
		t.Fatal("/plan command did not schedule agent event consumption")
	}
	for cmd != nil && m.task != nil {
		message := cmd()
		_, cmd = m.Update(message)
	}
	if len(session.PendingPlans()) != 1 {
		t.Fatalf("plans=%v", session.PendingPlans())
	}
	m.textarea.SetValue("/do")
	_, cmd = m.Update(tea.KeyPressMsg{Text: keySubmit})
	if cmd == nil {
		t.Fatal("/do command did not schedule agent event consumption")
	}
	for cmd != nil && m.task != nil {
		message := cmd()
		_, cmd = m.Update(message)
	}
	if len(session.PendingPlans()) != 0 {
		t.Fatalf("plans=%v", session.PendingPlans())
	}
}

func TestPlanStatusOverridesPreviousTerminal(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.planMode = true
	m.current.terminalTy = agent.EventCompleted
	m.current.terminal = &agent.Summary{Reason: agent.StopFinalAnswer}
	if got := m.statusText(); !strings.HasPrefix(got, "[PLAN]") {
		t.Fatalf("status=%q", got)
	}
}

func TestSystemMessageFollowsCurrentCommand(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.current = taskView{prompt: "/do", terminalTy: agent.EventFailed, err: errors.New("no valid plan")}
	m.systemMessages = []systemMessage{{content: "错误: no valid plan", after: -1}}
	m.refreshContent()
	view := m.View().Content
	if strings.Index(view, "/do") > strings.Index(view, "错误: no valid plan") {
		t.Fatalf("system message precedes command: %q", view)
	}
}

func TestSystemMessageStaysBetweenConversationTurns(t *testing.T) {
	session := conversation.NewSession()
	if err := session.CommitRound(
		&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "first user"}}},
		provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "first assistant"}}}, nil,
	); err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, session)
	m.AddSystemMessage("command feedback")
	if err := session.CommitRound(
		&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "second user"}}},
		provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "second assistant"}}}, nil,
	); err != nil {
		t.Fatal(err)
	}
	m.refreshContent()
	view := m.View().Content
	first := strings.Index(view, "first assistant")
	feedback := strings.Index(view, "command feedback")
	second := strings.Index(view, "second user")
	if first < 0 || feedback < 0 || second < 0 || !(first < feedback && feedback < second) {
		t.Fatalf("messages are not chronological: %q", view)
	}
	if len(session.Snapshot()) != 4 {
		t.Fatalf("system message leaked into history: %v", session.Snapshot())
	}
}

func TestLocalCommandAndFeedbackStayBetweenConversationTurns(t *testing.T) {
	session := conversation.NewSession()
	if err := session.CommitRound(
		&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "first user"}}},
		provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "first assistant"}}}, nil,
	); err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, session)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 80})
	m.textarea.SetValue("/help")
	m.Update(tea.KeyPressMsg{Text: keySubmit})
	if err := session.CommitRound(
		&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "second user"}}},
		provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "second assistant"}}}, nil,
	); err != nil {
		t.Fatal(err)
	}
	m.refreshContent()
	view := m.View().Content
	first := strings.Index(view, "first assistant")
	commandText := strings.Index(view, "\n/help")
	feedback := strings.Index(view, "可用命令:")
	second := strings.Index(view, "second user")
	if first < 0 || commandText < 0 || feedback < 0 || second < 0 || !(first < commandText && commandText < feedback && feedback < second) {
		t.Fatalf("messages are not chronological: %q", view)
	}
	if len(session.Snapshot()) != 4 {
		t.Fatalf("command leaked into history: %v", session.Snapshot())
	}
}

func TestLocalCommandPrecedesFeedbackAfterSessionReset(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.systemMessages = []systemMessage{{content: "old feedback", after: 0}}
	m.systemMessages = nil
	m.AddSystemMessage("已创建新会话。")
	m.AddCommandMessage("/session new", 1, true)
	view := m.View().Content
	commandText := strings.Index(view, "/session new")
	feedback := strings.Index(view, "已创建新会话。")
	if commandText < 0 || feedback < 0 || commandText > feedback {
		t.Fatalf("command does not precede feedback: %q", view)
	}
}

func TestLocalCommandWithoutFeedbackUsesCurrentConversationAnchor(t *testing.T) {
	session := conversation.NewSession()
	if err := session.CommitRound(
		&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "first user"}}},
		provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "first assistant"}}}, nil,
	); err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, session)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m.systemMessages = []systemMessage{{content: "older feedback", after: 0}}
	m.AddCommandMessage("/memory add user marker", len(m.systemMessages), false)
	view := m.View().Content
	commandText := strings.Index(view, "/memory add user marker")
	first := strings.Index(view, "first user")
	if commandText < 0 || first < 0 || commandText < first {
		t.Fatalf("command used the oldest anchor: %q", view)
	}
}

func TestActPlanDoCommands(t *testing.T) {
	tests := []struct {
		input  string
		mode   agent.Mode
		prompt string
	}{
		{"hello", agent.ModeAct, "hello"},
		{"/plan inspect this", agent.ModePlan, "inspect this"},
		{"/do", agent.ModeDo, ""},
		{"/compact", agent.ModeCompact, ""},
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

func TestViewContextCompactionEvent(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.current.prompt = "/compact"
	m.applyAgentEvent(agent.Event{Type: agent.EventContextCompaction, ContextCompaction: &agent.CompactionEvent{Trigger: "manual", BeforeTokens: 120, AfterTokens: 40}})
	m.applyAgentEvent(agent.Event{Type: agent.EventCompleted, Summary: &agent.Summary{Reason: agent.StopFinalAnswer, Iterations: 1}})
	m.refreshContent()

	view := m.View().Content
	if !strings.Contains(view, "上下文压缩: manual") || !strings.Contains(view, "120 -> 40") {
		t.Fatalf("view=%q", view)
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

func TestPermissionViewAndChoices(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	decision := permissions.Decision{
		Action:  permissions.ActionAsk,
		Stage:   permissions.StageMode,
		Reason:  "default mode requires confirmation",
		Request: permissions.Request{Tool: "write_file", MatchTarget: "docs/out.md"},
	}
	reply := make(chan permissions.Choice, 1)
	m.pendingPermission = &pendingPermission{decision: decision, reply: reply}
	m.refreshContent()
	view := m.View().Content
	if !strings.Contains(view, "write_file") || !strings.Contains(view, "docs/out.md") || !strings.Contains(view, "default mode") {
		t.Fatalf("view=%q", view)
	}
	m.Update(tea.KeyPressMsg{Text: "o"})
	if got := <-reply; got != permissions.ChoiceAllowOnce {
		t.Fatalf("choice=%q", got)
	}
}

func TestPermissionDenyAndCancelChoices(t *testing.T) {
	for key, want := range map[string]permissions.Choice{"d": permissions.ChoiceDeny, keyCancelOrQuit: permissions.ChoiceCancel} {
		t.Run(key, func(t *testing.T) {
			m := NewModel(nil, conversation.NewSession())
			reply := make(chan permissions.Choice, 1)
			m.pendingPermission = &pendingPermission{decision: permissions.Decision{Request: permissions.Request{Tool: "run_command", MatchTarget: "git status"}}, reply: reply}
			m.Update(tea.KeyPressMsg{Text: key})
			if got := <-reply; got != want {
				t.Fatalf("choice=%q want %q", got, want)
			}
		})
	}
}

func TestPermissionBridgeConfirmWaitsForModelChoice(t *testing.T) {
	bridge := NewPermissionBridge()
	done := make(chan permissions.Confirmation, 1)
	go func() {
		conf, _ := bridge.Confirm(context.Background(), permissions.Decision{Request: permissions.Request{Tool: "write_file", MatchTarget: "x.txt"}})
		done <- conf
	}()
	msg := bridge.Wait()
	msg.reply <- permissions.ChoiceAllowSession
	conf := <-done
	if conf.Choice != permissions.ChoiceAllowSession || conf.Decision.Request.Tool != "write_file" {
		t.Fatalf("confirmation=%#v", conf)
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
