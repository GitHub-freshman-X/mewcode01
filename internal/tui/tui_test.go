package tui

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/subagent"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type tuiScriptedProvider struct{}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string { return ansiEscape.ReplaceAllString(value, "") }

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

func TestStatusUsesSessionUsage(t *testing.T) {
	session := conversation.NewSession()
	if err := session.RecordUsage(provider.Usage{InputTokens: 21, OutputTokens: 8}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, session)
	m.current.usage = provider.Usage{InputTokens: 1, OutputTokens: 1}
	if got := m.TokenUsage(); got.InputTokens != 21 || got.OutputTokens != 8 {
		t.Fatalf("usage=%+v", got)
	}
	m.textarea.SetValue("/status")
	m.Update(tea.KeyPressMsg{Text: keySubmit})
	if view := m.View().Content; !strings.Contains(view, "tokens in:21 out:8") {
		t.Fatalf("view=%q", view)
	}
}

func TestStatusShowsCacheHitRate(t *testing.T) {
	session := conversation.NewSession()
	if err := session.RecordUsage(provider.Usage{InputTokens: 10, CacheReadInputTokens: 20, CacheCreationInputTokens: 5}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, session)
	if got := m.statusText(); !strings.Contains(got, "缓存：57%") {
		t.Fatalf("status=%q", got)
	}
	if got := cacheStatus(provider.Usage{}); got != "缓存：—" {
		t.Fatalf("empty cache status=%q", got)
	}
}

func TestExitCommandQuitsWithoutSessionWrite(t *testing.T) {
	session := conversation.NewSession()
	m := NewModel(nil, session)
	m.textarea.SetValue("/exit")
	_, cmd := m.Update(tea.KeyPressMsg{Text: keySubmit})
	if cmd == nil {
		t.Fatal("/exit did not return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("message=%T", cmd())
	}
	if len(session.Snapshot()) != 0 || len(session.DisplaySnapshot()) != 0 {
		t.Fatalf("exit changed session: history=%v display=%v", session.Snapshot(), session.DisplaySnapshot())
	}
}

func TestInitialViewDisplaysStartupCatBannerOutsideMessages(t *testing.T) {
	session := conversation.NewSession()
	m := NewModel(nil, session)
	view := m.renderViewportContent()
	banner := strings.TrimSuffix(startupCatBanner, "\n")
	if count := strings.Count(view, banner); count != 1 {
		t.Fatalf("banner count=%d view=%q", count, view)
	}
	if strings.Contains(m.renderCurrentContent(), banner) {
		t.Fatalf("banner rendered as message content: %q", m.renderCurrentContent())
	}
	if len(session.DisplaySnapshot()) != 0 || len(m.systemMessages) != 0 {
		t.Fatalf("banner leaked into session state: display=%v system=%v", session.DisplaySnapshot(), m.systemMessages)
	}
}

func TestSystemMessageFollowsCurrentCommand(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.current = taskView{prompt: "/do", terminalTy: agent.EventFailed, err: errors.New("no valid plan")}
	m.systemMessages = []systemMessage{{content: "错误: no valid plan", after: -1}}
	m.refreshContent()
	view := stripANSI(m.View().Content)
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
	view := stripANSI(m.View().Content)
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

func TestEscapeSystemMessageStaysBeforeLaterConversationTurn(t *testing.T) {
	session := conversation.NewSession()
	commitTUIRound(t, session, "before user", "before assistant")
	m := NewModel(nil, session)
	m.task = &agent.Task{}
	m.AddSystemMessage("子 Agent 已转入后台。")
	if len(m.systemMessages) != 1 || !m.systemMessages[0].pendingCurrentTaskAfter {
		t.Fatalf("ESC message was not marked for the current task: %+v", m.systemMessages)
	}
	commitTUIRound(t, session, "ESC user", "ESC assistant")
	m.applyAgentEvent(agent.Event{Type: agent.EventCompleted})
	commitTUIRound(t, session, "later user", "later assistant")
	m.refreshContent()
	view := stripANSI(m.View().Content)
	current := strings.Index(view, "ESC assistant")
	notice := strings.Index(view, "子 Agent 已转入后台。")
	later := strings.Index(view, "later user")
	if current < 0 || notice < 0 || later < 0 || !(current < notice && notice < later) {
		t.Fatalf("ESC system message order is wrong: %q", view)
	}
	if m.systemMessages[0].pendingCurrentTaskAfter {
		t.Fatal("ESC message remained pending after the task completed")
	}
}

func TestInitConsumesFirstSubAgentTerminalNotification(t *testing.T) {
	registry, err := tools.NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := subagent.NewTaskManager()
	session := conversation.NewSession()
	runner := agent.NewRunner(tuiScriptedProvider{}, session, registry, tools.NewExecutor(time.Second), agent.Options{SubAgents: agent.NewSubAgentRuntime(nil, manager)})
	m := NewModel(runner, session)
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("Init did not batch the notification listener: %#v", batch)
	}
	if _, err := manager.Launch(context.Background(), subagent.LaunchRequest{Name: "terminal-task", Background: true, Worker: func(context.Context, func(subagent.Progress)) subagent.Outcome {
		return subagent.Outcome{Status: subagent.TaskCompleted, Result: "safe summary"}
	}}); err != nil {
		t.Fatal(err)
	}
	_, next := m.Update(batch[len(batch)-1]()) // running notification
	if next == nil {
		t.Fatal("running notification did not continue listening")
	}
	_, next = m.Update(next()) // terminal notification
	if next == nil {
		t.Fatal("terminal notification did not continue listening")
	}
	view := stripANSI(m.View().Content)
	for _, want := range []string{"terminal-task", "completed", "safe summary"} {
		if !strings.Contains(view, want) {
			t.Fatalf("terminal notification missing %q: %q", want, view)
		}
	}
}

func TestSubAgentTerminalNotificationsContinueInOrder(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	notifications := make(chan subagent.TaskNotification, 2)
	m.subAgentNotifications = notifications
	wait := waitForSubAgentNotification(notifications)
	notifications <- subagent.TaskNotification{Task: subagent.TaskInfo{Name: "first", Status: subagent.TaskCompleted, Result: "first safe"}}
	_, wait = m.Update(wait())
	notifications <- subagent.TaskNotification{Task: subagent.TaskInfo{Name: "second", Status: subagent.TaskFailed, Failure: "second safe"}}
	_, wait = m.Update(wait())
	if wait == nil {
		t.Fatal("terminal notification did not continue listening")
	}
	view := stripANSI(m.View().Content)
	first, second := strings.Index(view, "first safe"), strings.Index(view, "second safe")
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("notifications are missing or out of order: %q", view)
	}
	m.textarea.SetValue("")
	m.Update(tea.KeyPressMsg{Text: "x"})
	if got := m.textarea.Value(); got != "x" {
		t.Fatalf("notification blocked input: %q", got)
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
	view := stripANSI(m.View().Content)
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

func TestViewDoesNotDuplicatePersistedToolOutputFromActiveTask(t *testing.T) {
	session := conversation.NewSession()
	call := provider.ToolCall{ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}
	result := provider.ToolResult{CallID: "call-1", Name: "read_file", Content: "contents"}
	if err := session.CommitRound(&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "inspect"}}}, provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockToolCall, ToolCall: &call}}}, []provider.ToolResult{result}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(nil, session)
	m.task = &agent.Task{}
	m.current = taskView{prompt: "inspect", toolCalls: []provider.ToolCall{call}, toolResult: []provider.ToolResult{result}}
	m.refreshContent()
	view := m.View().Content
	if got := strings.Count(view, "工具调用: read_file"); got != 1 {
		t.Fatalf("tool calls=%d view=%q", got, view)
	}
	if got := strings.Count(view, "工具结果: read_file 成功"); got != 1 {
		t.Fatalf("tool results=%d view=%q", got, view)
	}
}

func TestMessageBackgroundStyles(t *testing.T) {
	var b strings.Builder
	renderMessage(&b, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "question"}}}, false, false)
	renderMessage(&b, provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{
		{Type: provider.BlockText, Text: "answer"},
		{Type: provider.BlockThinking, Text: "reasoning"},
		{Type: provider.BlockToolCall, ToolCall: &provider.ToolCall{Name: "read_file"}},
	}}, false, true)
	renderMessage(&b, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{
		{Type: provider.BlockToolResult, ToolResult: &provider.ToolResult{Name: "read_file", Content: "ok"}},
		{Type: provider.BlockToolResult, ToolResult: &provider.ToolResult{Name: "write_file", Content: "denied", IsError: true}},
	}}, false, false)
	renderMessage(&b, provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "final answer"}}}, false, false)
	content := b.String()
	for _, expected := range []string{
		userStyle.Render("你"),
		userMessageStyle.Render("question"),
		assistantProgressStyle.Render("MewCode"),
		assistantProgressMsgStyle.Render("answer"),
		thinkingStyle.Render("思考\nreasoning"),
		toolCallStyle.Render("工具调用: read_file"),
		userStyle.Render("你"),
		toolResultStyle.Render("工具结果: read_file 成功\nok"),
		toolErrorStyle.Render("工具结果: write_file 失败\ndenied"),
		assistantStyle.Render("MewCode"),
		assistantMsgStyle.Render("final answer"),
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("message style is missing %q from %q", expected, content)
		}
	}
	if userMessageStyle.Render("question") == toolResultStyle.Render("ok") || assistantMsgStyle.Render("final answer") == toolResultStyle.Render("ok") {
		t.Fatal("message categories do not use the expected styles")
	}
}

func TestSystemMessageAndErrorUseBackgroundStyles(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.AddSystemMessage("feedback")
	if view := m.renderCurrentContent(); !strings.Contains(view, assistantMsgStyle.Render("feedback")) {
		t.Fatalf("system message lacks assistant background: %q", view)
	}
	m.current = taskView{terminalTy: agent.EventFailed, err: errors.New("boom")}
	if view := m.renderCurrentContent(); !strings.Contains(view, errorStyle.Render("错误: "+provider.UserError(m.current.err))) {
		t.Fatalf("error lacks background: %q", view)
	}
}

func TestActiveMessageBackgroundStyles(t *testing.T) {
	m := NewModel(nil, conversation.NewSession())
	m.task = &agent.Task{}
	m.current = taskView{
		prompt:     "question",
		text:       "partial answer",
		toolCalls:  []provider.ToolCall{{Name: "read_file"}},
		toolResult: []provider.ToolResult{{Name: "read_file", Content: "ok"}},
	}
	view := m.renderCurrentContent()
	for _, expected := range []string{
		userMessageStyle.Render("question"),
		assistantProgressMsgStyle.Render("partial answer"),
		toolCallStyle.Render("工具调用: read_file"),
		toolResultStyle.Render("工具结果: read_file 成功"),
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("active message style is missing %q from %q", expected, view)
		}
	}
}

func TestClearPreservesDisplayAndStartsNewSessionBoundary(t *testing.T) {
	m, previous := newPersistentTUIModel(t)
	commitTUIRound(t, previous, "old user", "old assistant")
	m.textarea.SetValue("/clear")
	m.Update(tea.KeyPressMsg{Text: keySubmit})

	view := stripANSI(m.View().Content)
	boundary := "会话开始 · " + m.runner.SessionID() + " · （空会话）"
	if old, marker, commandText := strings.Index(view, "old assistant"), strings.Index(view, boundary), strings.Index(view, "/clear"); old < 0 || marker < 0 || commandText < 0 || !(old < marker && marker < commandText) {
		t.Fatalf("clear boundary order is wrong: %q", view)
	}
	if len(previous.Snapshot()) != 2 || len(m.session.Snapshot()) != 0 || len(m.session.DisplaySnapshot()) != 0 {
		t.Fatalf("session display leaked into history: previous=%d current=%d display=%d", len(previous.Snapshot()), len(m.session.Snapshot()), len(m.session.DisplaySnapshot()))
	}
	if count := strings.Count(m.renderViewportContent(), strings.TrimSuffix(startupCatBanner, "\n")); count != 1 {
		t.Fatalf("banner count after clear=%d", count)
	}
}

func TestSessionNewPreservesDisplayAndStartsNewSessionBoundary(t *testing.T) {
	m, previous := newPersistentTUIModel(t)
	commitTUIRound(t, previous, "old user", "old assistant")
	m.textarea.SetValue("/session new")
	m.Update(tea.KeyPressMsg{Text: keySubmit})

	view := stripANSI(m.View().Content)
	boundary := "会话开始 · " + m.runner.SessionID() + " · （空会话）"
	if old, marker, commandText := strings.Index(view, "old assistant"), strings.Index(view, boundary), strings.Index(view, "/session new"); old < 0 || marker < 0 || commandText < 0 || !(old < marker && marker < commandText) {
		t.Fatalf("session new boundary order is wrong: %q", view)
	}
	if len(previous.Snapshot()) != 2 || len(m.session.Snapshot()) != 0 {
		t.Fatalf("session switch changed history: previous=%d current=%d", len(previous.Snapshot()), len(m.session.Snapshot()))
	}
	if count := strings.Count(m.renderViewportContent(), strings.TrimSuffix(startupCatBanner, "\n")); count != 1 {
		t.Fatalf("banner count after session new=%d", count)
	}
}

func TestConsecutiveSessionSwitchesKeepEveryBoundary(t *testing.T) {
	m, previous := newPersistentTUIModel(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 200})
	commitTUIRound(t, previous, "first user", "first assistant")
	m.textarea.SetValue("/clear")
	m.Update(tea.KeyPressMsg{Text: keySubmit})
	firstNewID := m.runner.SessionID()
	commitTUIRound(t, m.session, "second user", "second assistant")
	m.textarea.SetValue("/clear")
	m.Update(tea.KeyPressMsg{Text: keySubmit})

	view := stripANSI(m.View().Content)
	firstBoundary := "会话开始 · " + firstNewID + " · （空会话）"
	secondBoundary := "会话开始 · " + m.runner.SessionID() + " · （空会话）"
	if first, firstMarker, second, secondMarker := strings.Index(view, "first assistant"), strings.Index(view, firstBoundary), strings.Index(view, "second assistant"), strings.Index(view, secondBoundary); first < 0 || firstMarker < 0 || second < 0 || secondMarker < 0 || !(first < firstMarker && firstMarker < second && second < secondMarker) {
		t.Fatalf("consecutive boundaries are missing or out of order: %q", view)
	}
}

func TestSessionResumePreservesDisplayAndStartsSessionBoundary(t *testing.T) {
	m, previous := newPersistentTUIModel(t)
	commitTUIRound(t, previous, "old user", "old assistant")
	store := m.runner.SessionStore()
	restored, meta, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}
	commitTUIRound(t, restored, "restored user", "restored assistant")
	m.textarea.SetValue("/session resume " + meta.ID)
	m.Update(tea.KeyPressMsg{Text: keySubmit})

	view := stripANSI(m.View().Content)
	boundary := "会话开始 · " + meta.ID + " · restored user"
	if old, marker, restoredText := strings.Index(view, "old assistant"), strings.Index(view, boundary), strings.Index(view, "restored assistant"); old < 0 || marker < 0 || restoredText < 0 || !(old < marker && marker < restoredText) {
		t.Fatalf("session resume boundary order is wrong: %q", view)
	}
	if len(previous.Snapshot()) != 2 || len(m.session.Snapshot()) != 2 || len(m.session.DisplaySnapshot()) != 2 {
		t.Fatalf("session boundary leaked into history: previous=%d current=%d display=%d", len(previous.Snapshot()), len(m.session.Snapshot()), len(m.session.DisplaySnapshot()))
	}
	if count := strings.Count(m.renderViewportContent(), strings.TrimSuffix(startupCatBanner, "\n")); count != 1 {
		t.Fatalf("banner count after session resume=%d", count)
	}
}

func TestFailedSessionResumeDoesNotCreateBoundary(t *testing.T) {
	m, previous := newPersistentTUIModel(t)
	commitTUIRound(t, previous, "old user", "old assistant")
	if err := m.Resume(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid session ID was accepted")
	}
	if m.session != previous || len(m.historySegments) != 0 || m.sessionBoundary != nil {
		t.Fatalf("failed resume changed display state: session=%p previous=%p history=%v boundary=%+v", m.session, previous, m.historySegments, m.sessionBoundary)
	}
}

func newPersistentTUIModel(t *testing.T) (*Model, *conversation.Session) {
	t.Helper()
	store := conversation.NewSessionStore(t.TempDir())
	session, meta, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tools.NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(tuiScriptedProvider{}, session, registry, tools.NewExecutor(time.Second), agent.Options{SessionStore: store, SessionID: meta.ID})
	return NewModel(runner, session), session
}

func commitTUIRound(t *testing.T, session *conversation.Session, user, assistant string) {
	t.Helper()
	if err := session.CommitRound(
		&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: user}}},
		provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: assistant}}}, nil,
	); err != nil {
		t.Fatal(err)
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
