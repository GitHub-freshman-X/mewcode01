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

func TestPlanAndDo(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("the plan", provider.Usage{}), textRound("executed", provider.Usage{})}}
	runner, session := testRunner(t, p, Options{})
	plan, err := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "change it"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, plan)
	if saved, ok := session.LatestPlan(); !ok || saved != "the plan" {
		t.Fatalf("plan=%q ok=%v", saved, ok)
	}
	if len(p.requests[0].Tools) != 3 {
		t.Fatalf("plan tools=%d", len(p.requests[0].Tools))
	}
	do, err := runner.Start(context.Background(), Request{Mode: ModeDo})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, do)
	if len(p.requests[1].Tools) != 6 {
		t.Fatalf("do tools=%d", len(p.requests[1].Tools))
	}
	first := p.requests[1].Messages[len(p.requests[1].Messages)-1]
	if !strings.Contains(first.Blocks[0].Text, "the plan") {
		t.Fatalf("do prompt=%q", first.Blocks[0].Text)
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
}

func TestPlanPreservedOnNonSuccess(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{textRound("good plan", provider.Usage{}), {err: errors.New("failed")}}}
	runner, session := testRunner(t, p, Options{})
	first, _ := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "one"})
	drainTask(t, first)
	second, _ := runner.Start(context.Background(), Request{Mode: ModePlan, Prompt: "two"})
	events := drainTask(t, second)
	plan, _ := session.LatestPlan()
	if events[len(events)-1].Type != EventFailed || plan != "good plan" {
		t.Fatalf("terminal=%+v plan=%q", events[len(events)-1], plan)
	}
}
