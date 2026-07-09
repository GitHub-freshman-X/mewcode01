package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
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
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "hello"})
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
	round.events = append(round.events[:len(round.events)-1], provider.StreamEvent{Type: provider.EventUsage, Usage: &provider.Usage{InputTokens: 10, OutputTokens: 4}}, provider.StreamEvent{Type: provider.EventCompleted})
	p := &scriptedProvider{rounds: []scriptedRound{round, textRound("done", provider.Usage{InputTokens: 20, OutputTokens: 6})}}
	runner, _ := testRunner(t, p, Options{})
	task, _ := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "x"})
	events := drainTask(t, task)
	usage := events[len(events)-1].Summary.Usage
	if usage.InputTokens != 30 || usage.OutputTokens != 10 {
		t.Fatalf("usage=%+v", usage)
	}
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
	do, err := runner.Start(context.Background(), Request{Mode: ModeDo})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, do)
	if len(p.requests[2].Tools) != 6 {
		t.Fatalf("do tools=%d", len(p.requests[2].Tools))
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
