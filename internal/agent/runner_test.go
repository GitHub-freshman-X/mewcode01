package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	contextmanager "github.com/GitHub-freshman-X/mewcode01/internal/context"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/memory"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/prompt"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/skills"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type scriptedRound struct {
	events []provider.StreamEvent
	err    error
}

type scriptedProvider struct {
	mu       sync.Mutex
	rounds   []scriptedRound
	requests []provider.ChatRequest
}

func (p *scriptedProvider) Stream(_ context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	p.mu.Lock()
	index := len(p.requests)
	p.requests = append(p.requests, req)
	round := p.rounds[index]
	p.mu.Unlock()
	events := make(chan provider.StreamEvent, len(round.events))
	done := make(chan error, 1)
	for _, event := range round.events {
		events <- event
	}
	close(events)
	done <- round.err
	close(done)
	return events, done
}

func textRound(text string, usage provider.Usage) scriptedRound {
	return scriptedRound{events: []provider.StreamEvent{
		{Type: provider.EventStarted},
		{Type: provider.EventTextDelta, BlockIndex: 0, Delta: text},
		{Type: provider.EventUsage, Usage: &usage},
		{Type: provider.EventCompleted},
	}}
}

func toolRound(calls ...provider.ToolCall) scriptedRound {
	events := []provider.StreamEvent{{Type: provider.EventStarted}}
	for i, call := range calls {
		events = append(events, provider.StreamEvent{Type: provider.EventToolCallStart, BlockIndex: i,
			ToolCall: &provider.ToolCallDelta{ID: call.ID, Name: call.Name, Arguments: string(call.Arguments)}})
	}
	events = append(events, provider.StreamEvent{Type: provider.EventCompleted})
	return scriptedRound{events: events}
}

func testRunner(t *testing.T, p provider.Provider, options Options) (*Runner, *conversation.Session) {
	t.Helper()
	registry, err := tools.NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session := conversation.NewSession()
	return NewRunner(p, session, registry, tools.NewExecutor(time.Second), options), session
}

func drainTask(t *testing.T, task *Task) []Event {
	t.Helper()
	var events []Event
	for event := range task.Events {
		events = append(events, event)
	}
	if len(events) == 0 || !isTerminal(events[len(events)-1].Type) {
		t.Fatal("missing terminal event")
	}
	return events
}

func TestRunnerFinalAnswer(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("done", provider.Usage{InputTokens: 10, OutputTokens: 4})}}
	runner, session := testRunner(t, p, Options{})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "请记住：我在 Go 中偏好 any。"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	terminal := events[len(events)-1]
	if terminal.Type != EventCompleted || terminal.Summary.Reason != StopFinalAnswer || terminal.Summary.Usage.InputTokens != 10 {
		t.Fatalf("terminal=%+v", terminal)
	}
	if got := len(session.Snapshot()); got != 2 {
		t.Fatalf("history=%d", got)
	}
	if got := session.Usage(); got.InputTokens != 10 || got.OutputTokens != 4 {
		t.Fatalf("usage=%+v", got)
	}
}

func TestRunnerForkSkillReturnsOnlyFinalSummaryToMainSession(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "review.md"), []byte("---\nname: review\ndescription: Review changes independently.\nmode: fork\ncontext: none\n---\nReview the current changes."), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := skills.NewManager(skills.DiscoverOptions{ProjectDir: project})
	if err != nil {
		t.Fatal(err)
	}
	p := &scriptedProvider{rounds: []scriptedRound{textRound("review summary", provider.Usage{InputTokens: 7, OutputTokens: 3})}}
	runner, session := testRunner(t, p, Options{Skills: manager})
	if err := session.CommitRound(&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "prior request"}}}, provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "prior answer"}}}, nil); err != nil {
		t.Fatal(err)
	}
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "Execute the requested skill.", Skill: &SkillInvocation{Name: "review"}})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted {
		t.Fatalf("events=%+v", events)
	}
	if len(p.requests) != 1 || len(p.requests[0].Messages) != 1 {
		t.Fatalf("fork request=%+v", p.requests)
	}
	history := session.Snapshot()
	if len(history) != 4 || messageText(history[2]) != "/review" || messageText(history[3]) != "review summary" {
		t.Fatalf("history=%+v", history)
	}
	if usage := session.Usage(); usage.InputTokens != 7 || usage.OutputTokens != 3 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestRunnerInjectsOptionalModulesIntoFirstPrompt(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("done", provider.Usage{})}}
	runner, _ := testRunner(t, p, Options{OptionalModules: prompt.OptionalModules{
		CustomInstructions: []string{"project rules"},
		LongTermMemory:     []string{"memory index"},
	}})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "请记住：我在 Go 中偏好 any。"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if len(p.requests) != 1 || !strings.Contains(p.requests[0].Prompt.StableSystem, "project rules") || !strings.Contains(p.requests[0].Prompt.StableSystem, "memory index") {
		t.Fatalf("prompt=%+v", p.requests)
	}
}

func TestRunnerExtractsMemoryAfterSuccessfulFinalAnswer(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("done", provider.Usage{})}}
	root := t.TempDir()
	called := make(chan struct{}, 1)
	service := memory.NewService(memory.NewPaths(filepath.Join(root, "config"), root), memory.ServiceOptions{
		Caller: memoryCaller(func(context.Context, provider.ChatRequest) (string, error) {
			called <- struct{}{}
			return `[{"action":"noop"}]`, nil
		}),
	})
	runner, _ := testRunner(t, p, Options{Memory: service})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "请记住：我在 Go 中偏好 any。"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("memory extraction was not started")
	}
}

type memoryCaller func(context.Context, provider.ChatRequest) (string, error)

func (f memoryCaller) Call(ctx context.Context, request provider.ChatRequest) (string, error) {
	return f(ctx, request)
}

func TestStandaloneConsumer(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("done", provider.Usage{})}}
	runner, _ := testRunner(t, p, Options{})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted {
		t.Fatalf("events=%v", events)
	}
}

func TestRunnerToolLoop(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "r1", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)}),
		toolRound(provider.ToolCall{ID: "w1", Name: "write_file", Arguments: []byte(`{"path":"x.txt","content":"ok"}`)}),
		textRound("finished", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted || len(p.requests) != 3 {
		t.Fatalf("events=%v requests=%d", events, len(p.requests))
	}
	if got := len(session.Snapshot()); got != 6 {
		t.Fatalf("history=%d", got)
	}
	if got := len(p.requests[1].Messages); got != 3 {
		t.Fatalf("second request messages=%d", got)
	}
}

func TestRunnerPermissionDeniedContinues(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "w1", Name: "write_file", Arguments: []byte(`{"path":"x.txt","content":"no"}`)}),
		textRound("adjusted", provider.Usage{}),
	}}
	root := t.TempDir()
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := permissions.NewSandbox(root)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := permissions.ParseRule("write_file(x.txt)", permissions.EffectDeny, permissions.ScopeSession, 0)
	if err != nil {
		t.Fatal(err)
	}
	session := conversation.NewSession()
	runner := NewRunner(p, session, registry, tools.NewExecutor(time.Second), Options{
		Workspace:   root,
		Permissions: &permissions.Engine{Mode: permissions.ModeRelaxed, Rules: permissions.NewRuleStore(permissions.RuleSet{Session: []permissions.Rule{rule}}), Sandbox: sandbox},
	})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if terminal := events[len(events)-1]; terminal.Type != EventCompleted {
		t.Fatalf("terminal=%+v", terminal)
	}
	if len(p.requests) != 2 {
		t.Fatalf("requests=%d", len(p.requests))
	}
	messages := p.requests[1].Messages
	found := false
	for _, block := range messages[len(messages)-1].Blocks {
		if block.ToolResult != nil && block.ToolResult.IsError && strings.Contains(block.ToolResult.Content, string(tools.ErrorPermission)) {
			found = true
		}
	}
	if !found {
		t.Fatalf("permission failure not written back: %+v", messages[len(messages)-1])
	}
	if _, err := os.Stat(filepath.Join(root, "x.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied write touched file, stat err=%v", err)
	}
}

func TestRunnerIterationLimit(t *testing.T) {
	call := provider.ToolCall{ID: "x", Name: "missing", Arguments: []byte(`{}`)}
	p := &scriptedProvider{rounds: []scriptedRound{toolRound(call), toolRound(provider.ToolCall{ID: "y", Name: "missing", Arguments: []byte(`{}`)})}}
	runner, _ := testRunner(t, p, Options{MaxIterations: 2})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "loop"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if terminal := events[len(events)-1]; terminal.Type != EventStopped || terminal.Summary.Reason != StopIterationLimit {
		t.Fatalf("terminal=%+v", terminal)
	}
	if len(p.requests) != 2 {
		t.Fatalf("requests=%d", len(p.requests))
	}
}

func TestRunnerDefaultIterationLimit(t *testing.T) {
	rounds := make([]scriptedRound, DefaultMaxIterations)
	for i := range rounds {
		rounds[i] = toolRound(provider.ToolCall{ID: string(rune('a' + i)), Name: "missing", Arguments: []byte(`{}`)})
	}
	// A registered call resets the unknown-tool counter while still keeping the loop alive.
	for i := 2; i < len(rounds); i += 3 {
		rounds[i] = toolRound(provider.ToolCall{ID: string(rune('a' + i)), Name: "read_file", Arguments: []byte(`{"path":"missing"}`)})
	}
	p := &scriptedProvider{rounds: rounds}
	runner, _ := testRunner(t, p, Options{})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "loop"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if len(p.requests) != DefaultMaxIterations || events[len(events)-1].Summary.Reason != StopIterationLimit {
		t.Fatalf("requests=%d terminal=%+v", len(p.requests), events[len(events)-1])
	}
}

func TestRunnerFinalAnswerAtLimit(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{toolRound(provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)}), textRound("done", provider.Usage{})}}
	runner, _ := testRunner(t, p, Options{MaxIterations: 2})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	events := drainTask(t, task)
	if events[len(events)-1].Summary.Reason != StopFinalAnswer {
		t.Fatalf("terminal=%+v", events[len(events)-1])
	}
}

func TestRunnerUnknownToolLimit(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{toolRound(
		provider.ToolCall{ID: "1", Name: "a", Arguments: []byte(`{}`)},
		provider.ToolCall{ID: "2", Name: "b", Arguments: []byte(`{}`)},
		provider.ToolCall{ID: "3", Name: "c", Arguments: []byte(`{}`)},
	)}}
	runner, _ := testRunner(t, p, Options{})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "loop"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if terminal := events[len(events)-1]; terminal.Summary.Reason != StopUnknownToolLimit {
		t.Fatalf("terminal=%+v", terminal)
	}
}

func TestRunnerUnknownToolReset(t *testing.T) {
	unknown := func(id string) scriptedRound {
		return toolRound(provider.ToolCall{ID: id, Name: "missing", Arguments: []byte(`{}`)})
	}
	known := toolRound(provider.ToolCall{ID: "k", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)})
	p := &scriptedProvider{rounds: []scriptedRound{unknown("1"), unknown("2"), known, unknown("3"), unknown("4"), textRound("done", provider.Usage{})}}
	runner, _ := testRunner(t, p, Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	events := drainTask(t, task)
	if events[len(events)-1].Summary.Reason != StopFinalAnswer {
		t.Fatalf("terminal=%+v", events[len(events)-1])
	}
}

func TestRunnerStreamErrorPartialNotCommitted(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{{events: []provider.StreamEvent{{Type: provider.EventTextDelta, Delta: "partial"}}, err: errors.New("stream failed")}}}
	runner, session := testRunner(t, p, Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	events := drainTask(t, task)
	terminal := events[len(events)-1]
	if terminal.Type != EventFailed || !terminal.Summary.Partial || len(session.Snapshot()) != 0 {
		t.Fatalf("terminal=%+v history=%v", terminal, session.Snapshot())
	}
}

type cancellableProvider struct{}

func (cancellableProvider) Stream(ctx context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	events := make(chan provider.StreamEvent)
	done := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(done)
		select {
		case events <- provider.StreamEvent{Type: provider.EventTextDelta, Delta: "partial"}:
		case <-ctx.Done():
			done <- ctx.Err()
			return
		}
		<-ctx.Done()
		done <- ctx.Err()
	}()
	return events, done
}

func TestRunnerCancelDuringStream(t *testing.T) {
	runner, session := testRunner(t, cancellableProvider{}, Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	var got []Event
	for event := range task.Events {
		got = append(got, event)
		if event.Type == EventTextDelta {
			task.Cancel()
		}
	}
	terminal := got[len(got)-1]
	if terminal.Type != EventCancelled || !terminal.Summary.Partial || len(session.Snapshot()) != 0 {
		t.Fatalf("terminal=%+v history=%v", terminal, session.Snapshot())
	}
}

func TestRunnerCancelDuringReadBatch(t *testing.T) {
	testRunnerToolCancel(t, tools.SafetyReadOnly, "read")
}
func TestRunnerCancelDuringSideEffect(t *testing.T) {
	testRunnerToolCancel(t, tools.SafetySideEffect, "write")
}

func testRunnerToolCancel(t *testing.T, safety tools.Safety, prefix string) {
	t.Helper()
	registry := tools.NewRegistry()
	started := make(chan string, 2)
	release := make(chan struct{})
	for _, name := range []string{prefix + "1", prefix + "2"} {
		if err := registry.Register(&blockingTool{name: name, safety: safety, started: started, release: release}); err != nil {
			t.Fatal(err)
		}
	}
	calls := []provider.ToolCall{{ID: "1", Name: prefix + "1", Arguments: []byte(`{}`)}, {ID: "2", Name: prefix + "2", Arguments: []byte(`{}`)}}
	forbidden := prefix + "2"
	if safety == tools.SafetyReadOnly {
		if err := registry.Register(&blockingTool{name: "later", safety: tools.SafetySideEffect, started: started, release: release}); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, provider.ToolCall{ID: "3", Name: "later", Arguments: []byte(`{}`)})
		forbidden = "later"
	}
	p := &scriptedProvider{rounds: []scriptedRound{toolRound(calls...)}}
	session := conversation.NewSession()
	runner := NewRunner(p, session, registry, tools.NewExecutor(time.Second), Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	var events []Event
	for event := range task.Events {
		events = append(events, event)
		if event.Type == EventToolCall && event.ToolCall.Name == prefix+"1" {
			task.Cancel()
		}
	}
	close(release)
	if events[len(events)-1].Type != EventCancelled {
		t.Fatalf("events=%v", events)
	}
	for _, event := range events {
		if event.Type == EventToolCall && event.ToolCall.Name == forbidden {
			t.Fatal("later tool started")
		}
	}
}

func TestRunnerUsageTotal(t *testing.T) {
	round := toolRound(provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)})
	round.events = append(round.events[:len(round.events)-1], provider.StreamEvent{Type: provider.EventUsage, Usage: &provider.Usage{InputTokens: 10, OutputTokens: 4, CacheCreationInputTokens: 2}}, provider.StreamEvent{Type: provider.EventCompleted})
	p := &scriptedProvider{rounds: []scriptedRound{round, textRound("done", provider.Usage{InputTokens: 20, OutputTokens: 6, CacheReadInputTokens: 5})}}
	runner, _ := testRunner(t, p, Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	events := drainTask(t, task)
	usage := events[len(events)-1].Summary.Usage
	if usage.InputTokens != 30 || usage.OutputTokens != 10 {
		t.Fatalf("usage=%+v", usage)
	}
	if usage.CacheCreationInputTokens != 2 || usage.CacheReadInputTokens != 5 {
		t.Fatalf("cache usage=%+v", usage)
	}
}

func TestRunnerAutomaticCompactBeforeNormalRequest(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("<summary>draft state</summary><summary>compressed state</summary>", provider.Usage{InputTokens: 30}),
		textRound("done", provider.Usage{InputTokens: 12}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("a", 144))})

	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if len(p.requests) != 2 {
		t.Fatalf("requests=%d", len(p.requests))
	}
	if len(p.requests[0].Tools) != 0 || p.requests[0].MaxTokens != 10 {
		t.Fatalf("summary request=%+v", p.requests[0])
	}
	if got := allMessageText(p.requests[1].Messages); !strings.Contains(got, "compressed state") || strings.Contains(got, "draft state") {
		t.Fatalf("normal request messages:\n%s", got)
	}
	event := firstCompactionEvent(events)
	if event == nil || event.Trigger != contextmanager.TriggerAutomatic || event.BeforeTokens == 0 || event.AfterTokens == 0 {
		t.Fatalf("compaction event=%+v", event)
	}
	if got := session.Usage(); got.InputTokens != 42 {
		t.Fatalf("usage=%+v", got)
	}
}

func TestRunnerReplaceSessionDefersCompactionUntilNextTask(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("<summary>compressed state</summary>", provider.Usage{InputTokens: 30}),
		textRound("done", provider.Usage{InputTokens: 12}),
	}}
	runner, _ := testRunner(t, p, Options{Context: compactTestConfig()})
	restored := conversation.NewSession()
	restored.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("a", 144))})
	if err := runner.ReplaceSession(restored, "20260816-120000-a1b2"); err != nil {
		t.Fatal(err)
	}
	if len(p.requests) != 0 {
		t.Fatalf("resume started provider requests=%d", len(p.requests))
	}
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if len(p.requests) != 2 || firstCompactionEvent(events) == nil {
		t.Fatalf("requests=%d events=%+v", len(p.requests), events)
	}
}

func TestRunnerManualCompactUsesNoToolsAndNoNormalRequest(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("<summary>manual state</summary>", provider.Usage{InputTokens: 8})}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, "small history")})

	task, err := runner.Start(context.Background(), Request{Mode: ModeCompact})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if len(p.requests) != 1 || len(p.requests[0].Tools) != 0 {
		t.Fatalf("requests=%d first=%+v", len(p.requests), p.requests[0])
	}
	if got := allMessageText(session.Snapshot()); !strings.Contains(got, "manual state") {
		t.Fatalf("history=%s", got)
	}
	event := firstCompactionEvent(events)
	if event == nil || event.Trigger != contextmanager.TriggerManual {
		t.Fatalf("compaction event=%+v", event)
	}
}

func TestRunnerPersistsToolResultsBeforeCommit(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "big", Name: "big_result", Arguments: []byte(`{}`)}),
		textRound("done", provider.Usage{}),
	}}
	root := t.TempDir()
	registry := tools.NewRegistry()
	if err := registry.Register(bigResultTool{content: strings.Repeat("x", 80)}); err != nil {
		t.Fatal(err)
	}
	session := conversation.NewSession()
	cfg := compactTestConfig()
	cfg.WindowTokens = 1000
	cfg.SingleResultChars = 20
	cfg.MessageResultChars = 100
	cfg.PreviewChars = 8
	runner := NewRunner(p, session, registry, tools.NewExecutor(time.Second), Options{Workspace: root, SessionID: "20260814-103000-a1b2", Context: cfg})

	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "call tool"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if len(p.requests) != 2 {
		t.Fatalf("requests=%d", len(p.requests))
	}
	history := allMessageText(p.requests[1].Messages)
	if !strings.Contains(history, "Tool result persisted") || strings.Contains(history, strings.Repeat("x", 80)) {
		t.Fatalf("tool result history:\n%s", history)
	}
	if _, err := os.Stat(filepath.Join(root, ".mewcode", "context", "20260814-103000-a1b2", "tool-results")); err != nil {
		t.Fatalf("new result directory missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mew")); !os.IsNotExist(err) {
		t.Fatalf("legacy result directory exists: %v", err)
	}
}

func TestRunnerAutomaticCompactBreakerAfterThreeFailures(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("missing summary", provider.Usage{}),
		textRound("missing summary", provider.Usage{}),
		textRound("missing summary", provider.Usage{}),
		textRound("done", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("a", 144))})

	for i := 0; i < 3; i++ {
		task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
		if err != nil {
			t.Fatal(err)
		}
		events := drainTask(t, task)
		if events[len(events)-1].Type != EventFailed {
			t.Fatalf("attempt %d terminal=%+v", i+1, events[len(events)-1])
		}
	}
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted {
		t.Fatalf("terminal=%+v", events[len(events)-1])
	}
	if len(p.requests) != 4 || len(p.requests[3].Tools) == 0 {
		t.Fatalf("breaker did not skip fourth automatic summary: requests=%d fourth tools=%d", len(p.requests), len(p.requests[3].Tools))
	}
}

func TestRunnerManualCompactResetsAutomaticBreaker(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("missing summary", provider.Usage{}),
		textRound("missing summary", provider.Usage{}),
		textRound("missing summary", provider.Usage{}),
		textRound("<summary>manual state</summary>", provider.Usage{}),
		textRound("<summary>automatic state</summary>", provider.Usage{}),
		textRound("done", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("a", 144))})
	for i := 0; i < 3; i++ {
		task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
		if err != nil {
			t.Fatal(err)
		}
		drainTask(t, task)
	}
	manual, err := runner.Start(context.Background(), Request{Mode: ModeCompact})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, manual)
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("b", 144))})

	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted {
		t.Fatalf("terminal=%+v", events[len(events)-1])
	}
	if len(p.requests) != 6 || len(p.requests[4].Tools) != 0 {
		t.Fatalf("automatic did not recover after manual compact: requests=%d fifth tools=%d", len(p.requests), len(p.requests[4].Tools))
	}
}

func TestRunnerAutomaticSuccessResetsFailureStreak(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("missing summary", provider.Usage{}),
		textRound("missing summary", provider.Usage{}),
		textRound("<summary>automatic state</summary>", provider.Usage{}),
		textRound("done", provider.Usage{}),
		textRound("missing summary", provider.Usage{}),
		textRound("<summary>automatic recovered</summary>", provider.Usage{}),
		textRound("done", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("a", 144))})
	for i := 0; i < 2; i++ {
		task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
		if err != nil {
			t.Fatal(err)
		}
		drainTask(t, task)
	}
	for i := 0; i < 3; i++ {
		session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("b", 144))})
		task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
		if err != nil {
			t.Fatal(err)
		}
		drainTask(t, task)
	}
	if len(p.requests) != 7 || len(p.requests[5].Tools) != 0 {
		t.Fatalf("automatic success did not reset failure streak: requests=%d sixth tools=%d", len(p.requests), len(p.requests[5].Tools))
	}
}

func TestRunnerPlanCompactReplacesOnlyTaskHistory(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("<summary>plan compressed</summary>", provider.Usage{}),
		textRound("plan output", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	original := testTextMessage(provider.RoleUser, strings.Repeat("a", 144))
	session.ReplaceHistory([]provider.Message{original})

	task, err := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if got := session.Snapshot(); len(got) != 1 || got[0].Blocks[0].Text != original.Blocks[0].Text {
		t.Fatalf("shared history changed: %+v", got)
	}
	if len(p.requests) != 2 || !strings.Contains(allMessageText(p.requests[1].Messages), "plan compressed") {
		t.Fatalf("plan compact requests=%d second=%+v", len(p.requests), p.requests[1])
	}
}

func TestRunnerPlanCompactDoesNotPolluteSharedContextAnchor(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("<summary>plan compressed</summary>", provider.Usage{}),
		textRound("plan output", provider.Usage{}),
		textRound("<summary>act compressed</summary>", provider.Usage{}),
		textRound("done", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	session.ReplaceHistory([]provider.Message{
		testTextMessage(provider.RoleUser, strings.Repeat("a", 48)),
		testTextMessage(provider.RoleAssistant, strings.Repeat("b", 48)),
		testTextMessage(provider.RoleUser, strings.Repeat("c", 48)),
	})

	plan, err := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, plan)
	act, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, act)
	if len(p.requests) != 4 || len(p.requests[2].Tools) != 0 {
		t.Fatalf("shared context anchor was polluted by plan: requests=%d third tools=%d", len(p.requests), len(p.requests[2].Tools))
	}
}

func TestRunnerEmergencyCompactRetriesContextTooLongOnce(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		{err: errors.New("context length exceeded")},
		textRound("<summary>emergency state</summary>", provider.Usage{}),
		textRound("done", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig()})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, "short")})

	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted {
		t.Fatalf("terminal=%+v", events[len(events)-1])
	}
	if len(p.requests) != 3 || len(p.requests[1].Tools) != 0 {
		t.Fatalf("requests=%d summary tools=%d", len(p.requests), len(p.requests[1].Tools))
	}
	event := firstCompactionEvent(events)
	if event == nil || event.Trigger != contextmanager.TriggerEmergency {
		t.Fatalf("compaction event=%+v", event)
	}
}

func TestRunnerLogsContextCompactionLifecycle(t *testing.T) {
	root := t.TempDir()
	logger, err := logging.New(root, func() time.Time { return time.Unix(1, 0).UTC() }, 123)
	if err != nil {
		t.Fatal(err)
	}
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("<summary>draft state</summary><summary>logged state</summary>", provider.Usage{InputTokens: 30}),
		textRound("done", provider.Usage{InputTokens: 12}),
	}}
	runner, session := testRunner(t, p, Options{Context: compactTestConfig(), Logger: logger})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("a", 144))})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	events := readLogEvents(t, root)
	assertLogEvent(t, events, "context compaction started", map[string]any{"stage": "context_compaction", "status": "started", "trigger": "automatic"})
	assertLogEvent(t, events, "context compaction completed", map[string]any{"stage": "context_compaction", "status": "completed", "trigger": "automatic", "summary_candidates": float64(2), "summary_used_last": true, "summary_chars": float64(len("logged state"))})
	if logText := marshalLogEvents(t, events); strings.Contains(logText, "draft state") || strings.Contains(logText, strings.Repeat("a", 24)) {
		t.Fatalf("context log leaked discarded summary or prompt: %s", logText)
	}
}

func TestLoggerCapturesBoundedGenericDiagnostic(t *testing.T) {
	root := t.TempDir()
	logger, err := logging.New(root, func() time.Time { return time.Unix(1, 0).UTC() }, 123)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := logger.CaptureDiagnostic("generic content", "diagnostic-payload", 10)
	if err != nil {
		t.Fatal(err)
	}
	if ref.OriginalChars != len("diagnostic-payload") || ref.CapturedChars != 10 || !ref.Truncated || ref.Path == "" {
		t.Fatalf("ref=%+v", ref)
	}
	info, err := os.Stat(filepath.Join(root, ref.Path))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%#o", info.Mode().Perm())
	}
	content, err := os.ReadFile(filepath.Join(root, ref.Path))
	if err != nil || string(content) != "diagnostic" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}

func TestRunnerLogsMalformedSummaryResponse(t *testing.T) {
	root := t.TempDir()
	logger, err := logging.New(root, func() time.Time { return time.Unix(1, 0).UTC() }, 123)
	if err != nil {
		t.Fatal(err)
	}
	cfg := compactTestConfig()
	p := &scriptedProvider{rounds: []scriptedRound{textRound("<summary>draft<summary>diagnostic-payload</summary></summary>", provider.Usage{})}}
	runner, session := testRunner(t, p, Options{Context: cfg, Logger: logger})
	session.ReplaceHistory([]provider.Message{testTextMessage(provider.RoleUser, strings.Repeat("a", 144))})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	events := readLogEvents(t, root)
	var fields map[string]any
	for _, event := range events {
		if event["message"] == "context compaction failed" {
			fields, _ = event["fields"].(map[string]any)
		}
	}
	if fields["parse_reason"] != "summary wrapper contains content" || fields["summary_response"] != "<summary>draft<summary>diagnostic-payload</summary></summary>" {
		t.Fatalf("fields=%+v", fields)
	}
}

func TestRunnerLogsToolResultPersistenceAndEmergencyRetry(t *testing.T) {
	root := t.TempDir()
	logger, err := logging.New(root, func() time.Time { return time.Unix(1, 0).UTC() }, 123)
	if err != nil {
		t.Fatal(err)
	}
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "big", Name: "big_result", Arguments: []byte(`{}`)}),
		{err: errors.New("context length exceeded")},
		textRound("<summary>emergency state</summary>", provider.Usage{}),
		textRound("done", provider.Usage{}),
	}}
	registry := tools.NewRegistry()
	if err := registry.Register(bigResultTool{content: strings.Repeat("x", 80)}); err != nil {
		t.Fatal(err)
	}
	session := conversation.NewSession()
	cfg := compactTestConfig()
	cfg.WindowTokens = 1000
	cfg.SingleResultChars = 20
	cfg.MessageResultChars = 100
	cfg.PreviewChars = 8
	runner := NewRunner(p, session, registry, tools.NewExecutor(time.Second), Options{Workspace: root, SessionID: "20260814-103000-a1b2", Context: cfg, Logger: logger})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "call tool"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	events := readLogEvents(t, root)
	assertLogEvent(t, events, "context tool results persisted", map[string]any{"stage": "tool_result_persistence", "status": "persisted", "persisted_count": float64(1)})
	assertLogEvent(t, events, "context emergency retry started", map[string]any{"stage": "context_emergency_retry", "status": "started"})
	assertLogEvent(t, events, "context compaction completed", map[string]any{"stage": "context_compaction", "status": "completed", "trigger": "emergency"})
	if logText := marshalLogEvents(t, events); strings.Contains(logText, strings.Repeat("x", 24)) || strings.Contains(logText, "context length exceeded") {
		t.Fatalf("context log leaked tool content or raw error: %s", logText)
	}
}

type bigResultTool struct {
	content string
}

func (t bigResultTool) Metadata() tools.Metadata {
	return tools.Metadata{Name: "big_result", Description: "return a large result", Schema: tools.Schema{"type": "object"}, Safety: tools.SafetyReadOnly}
}

func (t bigResultTool) Execute(context.Context, json.RawMessage) tools.Result {
	return tools.Success("big_result", map[string]any{"content": t.content})
}

func compactTestConfig() contextmanager.Config {
	cfg := contextmanager.DefaultConfig()
	cfg.WindowTokens = 50
	cfg.SummaryOutputTokens = 10
	cfg.AutoSafetyTokens = 5
	cfg.ManualSafetyTokens = 3
	cfg.RecentTokens = 4
	cfg.RecentMessageMinimum = 1
	return cfg
}

func testTextMessage(role provider.Role, text string) provider.Message {
	return provider.Message{Role: role, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: text}}}
}

func allMessageText(messages []provider.Message) string {
	var b strings.Builder
	for _, message := range messages {
		for _, block := range message.Blocks {
			b.WriteString(block.Text)
			if block.ToolResult != nil {
				b.WriteString(block.ToolResult.Content)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func firstCompactionEvent(events []Event) *CompactionEvent {
	for _, event := range events {
		if event.ContextCompaction != nil {
			return event.ContextCompaction
		}
	}
	return nil
}

func readLogEvents(t *testing.T, root string) []map[string]any {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*", "*.jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("log files=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	events := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func assertLogEvent(t *testing.T, events []map[string]any, message string, fields map[string]any) {
	t.Helper()
	for _, event := range events {
		if event["message"] != message {
			continue
		}
		gotFields, ok := event["fields"].(map[string]any)
		if !ok {
			t.Fatalf("log event %q has no fields: %#v", message, event)
		}
		matches := true
		for key, want := range fields {
			if gotFields[key] != want {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("missing log event %q with fields %#v in %#v", message, fields, events)
}

func marshalLogEvents(t *testing.T, events []map[string]any) string {
	t.Helper()
	data, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCompleteEventSequenceAndProgress(t *testing.T) {
	round := toolRound(provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)})
	round.events = append(round.events[:len(round.events)-1], provider.StreamEvent{Type: provider.EventUsage, Usage: &provider.Usage{InputTokens: 1}}, provider.StreamEvent{Type: provider.EventCompleted})
	p := &scriptedProvider{rounds: []scriptedRound{round, textRound("done", provider.Usage{})}}
	runner, _ := testRunner(t, p, Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	events := drainTask(t, task)
	want := map[EventType]bool{EventProgress: false, EventTextDelta: false, EventToolCall: false, EventToolResult: false, EventUsage: false, EventCompleted: false}
	terminals := 0
	lastIteration := 0
	for _, event := range events {
		if _, ok := want[event.Type]; ok {
			want[event.Type] = true
		}
		if isTerminal(event.Type) {
			terminals++
		}
		if event.Iteration < lastIteration {
			t.Fatal("iteration regressed")
		}
		lastIteration = event.Iteration
	}
	for typ, seen := range want {
		if !seen {
			t.Fatalf("missing %s in %v", typ, events)
		}
	}
	if terminals != 1 || !isTerminal(events[len(events)-1].Type) {
		t.Fatalf("terminal invariant failed: %v", events)
	}
}

func TestPlanAppendsInOrderAndDoConsumesPlansOnSuccess(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("first plan", provider.Usage{}), textRound("second plan", provider.Usage{}), textRound("executed", provider.Usage{})}}
	runner, session := testRunner(t, p, Options{})
	for _, prompt := range []string{"first task", "second task"} {
		plan, err := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: prompt})
		if err != nil {
			t.Fatal(err)
		}
		drainTask(t, plan)
	}
	if plans := session.PendingPlans(); len(plans) != 2 || plans[0] != "first plan" || plans[1] != "second plan" {
		t.Fatalf("plans=%q", plans)
	}
	if len(session.Snapshot()) != 0 || len(session.DisplaySnapshot()) != 4 {
		t.Fatalf("model history=%d display history=%d", len(session.Snapshot()), len(session.DisplaySnapshot()))
	}
	if len(p.requests[0].Tools) != 3 {
		t.Fatalf("plan tools=%d", len(p.requests[0].Tools))
	}
	planMessage := p.requests[0].Messages[len(p.requests[0].Messages)-1].Blocks[0].Text
	if planMessage != "first task" || strings.Contains(planMessage, "read-only") || strings.Contains(planMessage, "只读") {
		t.Fatalf("plan user prompt should be pure task, got %q", planMessage)
	}
	if !strings.Contains(systemContent(p.requests[0], "mew.mode.plan"), "只读") {
		t.Fatalf("plan mode system injection missing: %+v", p.requests[0].Prompt.DynamicSystem)
	}
	do, err := runner.Start(context.Background(), Request{Mode: ModeDo})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, do)
	if len(p.requests[2].Tools) != 6 {
		t.Fatalf("do tools=%d", len(p.requests[2].Tools))
	}
	if strings.Contains(systemContent(p.requests[2], "mew.mode.do"), "只读") {
		t.Fatalf("do mode system injection contains plan readonly rule: %q", systemContent(p.requests[2], "mew.mode.do"))
	}
	message := p.requests[2].Messages[len(p.requests[2].Messages)-1].Blocks[0].Text
	firstAt, secondAt := strings.Index(message, "first plan"), strings.Index(message, "second plan")
	if firstAt < 0 || secondAt <= firstAt {
		t.Fatalf("do prompt=%q", message)
	}
	if plans := session.PendingPlans(); len(plans) != 0 {
		t.Fatalf("plans were not consumed: %q", plans)
	}
	before := len(p.requests)
	if task, err := runner.Start(context.Background(), Request{Mode: ModeDo}); err == nil || task != nil || len(p.requests) != before {
		t.Fatalf("second do: task=%v err=%v requests=%d", task, err, len(p.requests))
	}
}

func TestRunnerBuildsPromptBundleAndEnhancedTools(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("done", provider.Usage{})}}
	runner, session := testRunner(t, p, Options{})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "inspect workspace"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if len(p.requests) != 1 {
		t.Fatalf("requests=%d", len(p.requests))
	}
	req := p.requests[0]
	if !strings.Contains(req.Prompt.StableSystem, "## 身份") || !strings.Contains(req.Prompt.StableSystem, "## 工具使用") {
		t.Fatalf("stable prompt missing fixed modules:\n%s", req.Prompt.StableSystem)
	}
	if !strings.Contains(systemContent(req, "mew.environment"), "Workspace:") ||
		!strings.Contains(systemContent(req, "mew.environment"), "Mode: act") {
		t.Fatalf("environment injection missing: %+v", req.Prompt.DynamicSystem)
	}
	if !req.Prompt.CachePolicy.Enable || !req.Prompt.CachePolicy.StableSystem || !req.Prompt.CachePolicy.StableTools {
		t.Fatalf("cache policy not enabled: %+v", req.Prompt.CachePolicy)
	}
	for _, tool := range req.Tools {
		if !tool.Cacheable {
			t.Fatalf("tool %s is not cacheable", tool.Name)
		}
		if !strings.Contains(tool.Description, "编辑前") {
			t.Fatalf("tool %s missing enhanced rules: %q", tool.Name, tool.Description)
		}
	}
	for _, message := range session.Snapshot() {
		text := messageText(message)
		if strings.Contains(text, "mew.environment") || strings.Contains(text, "mew.mode") {
			t.Fatalf("system injection leaked into history: %q", text)
		}
	}
}

func systemContent(req provider.ChatRequest, tag string) string {
	for _, message := range req.Prompt.DynamicSystem {
		if message.Tag == tag {
			return message.Content
		}
	}
	return ""
}

func TestDoPromptContainsAllPlans(t *testing.T) {
	p := &scriptedProvider{}
	runner, session := testRunner(t, p, Options{})
	for _, plan := range []string{"plan alpha", "plan beta", "plan gamma"} {
		if err := appendTestPlan(session, plan); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepareRequest(Request{Mode: ModeDo}, session, runner.registry)
	if err != nil {
		t.Fatal(err)
	}
	last := -1
	for i, plan := range prepared.plans {
		at := strings.Index(prepared.prompt, plan)
		if plan != session.PendingPlans()[i] || at <= last {
			t.Fatalf("plans=%q prompt=%q", prepared.plans, prepared.prompt)
		}
		last = at
	}
}

func TestDoPreservesConflictingPlans(t *testing.T) {
	p := &scriptedProvider{}
	runner, session := testRunner(t, p, Options{})
	conflicts := []string{"replace storage with SQLite", "keep storage in memory and do not use SQLite"}
	for _, plan := range conflicts {
		if err := appendTestPlan(session, plan); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := prepareRequest(Request{Mode: ModeDo}, session, runner.registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, plan := range conflicts {
		if !strings.Contains(prepared.prompt, plan) {
			t.Fatalf("conflicting plan was dropped: %q", prepared.prompt)
		}
	}
}

func TestDoWithoutPlan(t *testing.T) {
	p := &scriptedProvider{}
	runner, _ := testRunner(t, p, Options{})
	task, err := runner.Start(context.Background(), Request{Mode: ModeDo})
	if err == nil || task != nil || len(p.requests) != 0 {
		t.Fatalf("task=%v err=%v requests=%d", task, err, len(p.requests))
	}
}

func TestPlanHiddenWriteToolAndReadOnlyLoop(t *testing.T) {
	root := t.TempDir()
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	p := &scriptedProvider{rounds: []scriptedRound{toolRound(provider.ToolCall{ID: "1", Name: "write_file", Arguments: []byte(`{"path":"bad.txt","content":"bad"}`)}), toolRound(provider.ToolCall{ID: "2", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)}), textRound("plan", provider.Usage{})}}
	session := conversation.NewSession()
	runner := NewRunner(p, session, registry, tools.NewExecutor(time.Second), Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "inspect"})
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted || len(p.requests) != 3 {
		t.Fatalf("events=%v requests=%d", events, len(p.requests))
	}
	if _, err := os.Stat(filepath.Join(root, "bad.txt")); !os.IsNotExist(err) {
		t.Fatalf("hidden write changed workspace: %v", err)
	}
	if len(session.Snapshot()) != 0 || len(session.DisplaySnapshot()) != 2 {
		t.Fatalf("model history=%d display history=%d", len(session.Snapshot()), len(session.DisplaySnapshot()))
	}
}

func TestPlanTaskHistoryMultiRoundAndDoExcludesPlanInternalHistory(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)}),
		textRound("create the file", provider.Usage{}),
		textRound("executed", provider.Usage{}),
	}}
	runner, session := testRunner(t, p, Options{})
	plan, err := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "create a file"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, plan)
	if len(p.requests[1].Messages) != 3 || p.requests[1].Messages[2].Blocks[0].Type != provider.BlockToolResult {
		t.Fatalf("second plan request lacks temporary history: %+v", p.requests[1].Messages)
	}
	if len(session.Snapshot()) != 0 {
		t.Fatalf("plan leaked into model history: %+v", session.Snapshot())
	}
	do, err := runner.Start(context.Background(), Request{Mode: ModeDo})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, do)
	request := p.requests[2]
	if len(request.Tools) != 6 || messagesContain(request.Messages, "read-only tools only") || hasToolResult(request.Messages) {
		t.Fatalf("do request contains plan internals or lacks tools: %+v", request)
	}
}

func TestPlanIsolationHelloChanganEndToEnd(t *testing.T) {
	root := t.TempDir()
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	p := &scriptedProvider{rounds: []scriptedRound{
		textRound("Create hello.txt containing hello world.", provider.Usage{}),
		textRound("Replace world with changan in hello.txt.", provider.Usage{}),
		toolRound(provider.ToolCall{ID: "write", Name: "write_file", Arguments: []byte(`{"path":"hello.txt","content":"hello world"}`)}),
		toolRound(provider.ToolCall{ID: "edit", Name: "edit_file", Arguments: []byte(`{"path":"hello.txt","old_text":"world","new_text":"changan"}`)}),
		textRound("done", provider.Usage{}),
	}}
	session := conversation.NewSession()
	runner := NewRunner(p, session, registry, tools.NewExecutor(time.Second), Options{})
	for _, prompt := range []string{"create hello.txt with hello world", "replace world with changan"} {
		task, err := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: prompt})
		if err != nil {
			t.Fatal(err)
		}
		drainTask(t, task)
	}
	if _, err := os.Stat(filepath.Join(root, "hello.txt")); !os.IsNotExist(err) {
		t.Fatalf("planning modified workspace: %v", err)
	}
	task, err := runner.Start(context.Background(), Request{Mode: ModeDo})
	if err != nil {
		t.Fatal(err)
	}
	events := drainTask(t, task)
	if events[len(events)-1].Type != EventCompleted {
		t.Fatalf("terminal=%+v", events[len(events)-1])
	}
	content, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil || string(content) != "hello changan" {
		t.Fatalf("content=%q err=%v", content, err)
	}
	for _, request := range p.requests[2:] {
		if messagesContain(request.Messages, "read-only tools only") {
			t.Fatalf("plan restriction leaked into do request: %+v", request.Messages)
		}
	}
	toolNames := make(map[string]bool)
	for _, definition := range p.requests[2].Tools {
		toolNames[definition.Name] = true
	}
	if !toolNames["write_file"] || !toolNames["edit_file"] {
		t.Fatalf("do tools=%v", toolNames)
	}
	if len(session.PendingPlans()) != 0 {
		t.Fatalf("plans were not consumed: %q", session.PendingPlans())
	}
}

func TestPlanPreservedOnNonSuccess(t *testing.T) {
	tests := []struct {
		name     string
		provider provider.Provider
		options  Options
		cancel   bool
	}{
		{name: "stream error", provider: &scriptedProvider{rounds: []scriptedRound{{err: errors.New("failed")}}}},
		{name: "iteration limit", provider: &scriptedProvider{rounds: []scriptedRound{toolRound(provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)})}}, options: Options{MaxIterations: 1}},
		{name: "unknown tool limit", provider: &scriptedProvider{rounds: []scriptedRound{toolRound(
			provider.ToolCall{ID: "1", Name: "missing1", Arguments: []byte(`{}`)},
			provider.ToolCall{ID: "2", Name: "missing2", Arguments: []byte(`{}`)},
			provider.ToolCall{ID: "3", Name: "missing3", Arguments: []byte(`{}`)},
		)}}},
		{name: "cancelled", provider: cancellableProvider{}, cancel: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner, session := testRunner(t, test.provider, test.options)
			for _, plan := range []string{"first", "second"} {
				if err := appendTestPlan(session, plan); err != nil {
					t.Fatal(err)
				}
			}
			task, err := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "another task"})
			if err != nil {
				t.Fatal(err)
			}
			if test.cancel {
				for event := range task.Events {
					if event.Type == EventTextDelta {
						task.Cancel()
					}
				}
			} else {
				drainTask(t, task)
			}
			plans := session.PendingPlans()
			if len(plans) != 2 || plans[0] != "first" || plans[1] != "second" {
				t.Fatalf("plans=%q", plans)
			}
			if len(session.Snapshot()) != 0 {
				t.Fatalf("non-success plan leaked into model history: %+v", session.Snapshot())
			}
		})
	}
}

func TestDoPreservesPlansOnNonSuccess(t *testing.T) {
	tests := []struct {
		name     string
		provider provider.Provider
		options  Options
		cancel   bool
	}{
		{name: "stream error", provider: &scriptedProvider{rounds: []scriptedRound{{err: errors.New("failed")}}}},
		{name: "iteration limit", provider: &scriptedProvider{rounds: []scriptedRound{toolRound(provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"missing"}`)})}}, options: Options{MaxIterations: 1}},
		{name: "unknown tool limit", provider: &scriptedProvider{rounds: []scriptedRound{toolRound(
			provider.ToolCall{ID: "1", Name: "missing1", Arguments: []byte(`{}`)},
			provider.ToolCall{ID: "2", Name: "missing2", Arguments: []byte(`{}`)},
			provider.ToolCall{ID: "3", Name: "missing3", Arguments: []byte(`{}`)},
		)}}},
		{name: "cancelled", provider: cancellableProvider{}, cancel: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner, session := testRunner(t, test.provider, test.options)
			for _, plan := range []string{"first", "second"} {
				if err := appendTestPlan(session, plan); err != nil {
					t.Fatal(err)
				}
			}
			task, err := runner.Start(context.Background(), Request{Mode: ModeDo})
			if err != nil {
				t.Fatal(err)
			}
			if test.cancel {
				for event := range task.Events {
					if event.Type == EventTextDelta {
						task.Cancel()
					}
				}
			} else {
				drainTask(t, task)
			}
			plans := session.PendingPlans()
			if len(plans) != 2 || plans[0] != "first" || plans[1] != "second" {
				t.Fatalf("plans=%q", plans)
			}
		})
	}
}

func appendTestPlan(session *conversation.Session, plan string) error {
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "/plan test"}}}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: plan}}}
	return session.CommitPlan(user, assistant, plan)
}

func messagesContain(messages []provider.Message, value string) bool {
	for _, message := range messages {
		for _, block := range message.Blocks {
			if strings.Contains(block.Text, value) {
				return true
			}
		}
	}
	return false
}

func hasToolResult(messages []provider.Message) bool {
	for _, message := range messages {
		for _, block := range message.Blocks {
			if block.Type == provider.BlockToolResult {
				return true
			}
		}
	}
	return false
}
