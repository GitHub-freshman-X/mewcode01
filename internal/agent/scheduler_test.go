package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/hooks"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type blockingTool struct {
	name            string
	safety          tools.Safety
	started         chan<- string
	release         <-chan struct{}
	active, maximum *atomic.Int32
}

func (t *blockingTool) Metadata() tools.Metadata {
	return tools.Metadata{Name: t.name, Description: "test tool", Safety: t.safety, Schema: tools.Schema{"type": "object"}}
}
func (t *blockingTool) Execute(ctx context.Context, _ json.RawMessage) tools.Result {
	if t.active != nil {
		n := t.active.Add(1)
		for {
			old := t.maximum.Load()
			if n <= old || t.maximum.CompareAndSwap(old, n) {
				break
			}
		}
		defer t.active.Add(-1)
	}
	select {
	case t.started <- t.name:
	case <-ctx.Done():
		return tools.Failure(t.name, tools.ErrorTimeout, "cancelled", nil)
	}
	select {
	case <-t.release:
		return tools.Success(t.name, nil)
	case <-ctx.Done():
		return tools.Failure(t.name, tools.ErrorTimeout, "cancelled", nil)
	}
}

func TestSchedulerReadOnlyConcurrentAndResultOrder(t *testing.T) {
	registry := tools.NewRegistry()
	started := make(chan string, 2)
	release := make(chan struct{})
	var active, maximum atomic.Int32
	for _, name := range []string{"r1", "r2"} {
		if err := registry.Register(&blockingTool{name: name, safety: tools.SafetyReadOnly, started: started, release: release, active: &active, maximum: &maximum}); err != nil {
			t.Fatal(err)
		}
	}
	s := NewScheduler(registry, tools.NewExecutor(time.Second), nil, nil)
	done := make(chan []provider.ToolResult, 1)
	go func() {
		results, _ := s.Execute(context.Background(), []provider.ToolCall{{ID: "1", Name: "r1", Arguments: []byte(`{}`)}, {ID: "2", Name: "r2", Arguments: []byte(`{}`)}}, func(Event) bool { return true })
		done <- results
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("read-only tools did not overlap")
		}
	}
	close(release)
	results := <-done
	if maximum.Load() != 2 || results[0].CallID != "1" || results[1].CallID != "2" {
		t.Fatalf("max=%d results=%+v", maximum.Load(), results)
	}
}

func TestSchedulerSideEffectsSerial(t *testing.T) {
	registry := tools.NewRegistry()
	started := make(chan string, 2)
	release := make(chan struct{}, 2)
	var active, maximum atomic.Int32
	for _, name := range []string{"w1", "w2"} {
		if err := registry.Register(&blockingTool{name: name, safety: tools.SafetySideEffect, started: started, release: release, active: &active, maximum: &maximum}); err != nil {
			t.Fatal(err)
		}
	}
	s := NewScheduler(registry, tools.NewExecutor(time.Second), nil, nil)
	done := make(chan struct{})
	go func() {
		_, _ = s.Execute(context.Background(), []provider.ToolCall{{ID: "1", Name: "w1", Arguments: []byte(`{}`)}, {ID: "2", Name: "w2", Arguments: []byte(`{}`)}}, func(Event) bool { return true })
		close(done)
	}()
	if name := <-started; name != "w1" {
		t.Fatalf("first=%s", name)
	}
	release <- struct{}{}
	if name := <-started; name != "w2" {
		t.Fatalf("second=%s", name)
	}
	release <- struct{}{}
	<-done
	if maximum.Load() != 1 {
		t.Fatalf("max concurrent=%d", maximum.Load())
	}
}

func TestSchedulerBatchBarriers(t *testing.T) {
	registry := tools.NewRegistry()
	for _, meta := range []struct {
		name   string
		safety tools.Safety
	}{{"r1", tools.SafetyReadOnly}, {"r2", tools.SafetyReadOnly}, {"w", tools.SafetySideEffect}, {"r3", tools.SafetyReadOnly}, {"cmd", tools.SafetySideEffect}} {
		if err := registry.Register(&blockingTool{name: meta.name, safety: meta.safety, started: make(chan string, 1), release: closedSignal()}); err != nil {
			t.Fatal(err)
		}
	}
	s := NewScheduler(registry, tools.NewExecutor(time.Second), nil, nil)
	batches := s.batches([]provider.ToolCall{{Name: "r1"}, {Name: "r2"}, {Name: "w"}, {Name: "r3"}, {Name: "cmd"}})
	if len(batches) != 4 || len(batches[0].calls) != 2 || batches[1].concurrent || !batches[2].concurrent || batches[3].concurrent {
		t.Fatalf("batches=%+v", batches)
	}
}

func TestSchedulerUnclassifiedTool(t *testing.T) {
	registry := tools.NewRegistry()
	if err := registry.Register(&blockingTool{name: "unknown_class", started: make(chan string, 1), release: closedSignal()}); err != nil {
		t.Fatal(err)
	}
	batches := NewScheduler(registry, tools.NewExecutor(time.Second), nil, nil).batches([]provider.ToolCall{{Name: "unknown_class"}})
	if len(batches) != 1 || batches[0].concurrent {
		t.Fatalf("batches=%+v", batches)
	}
}

func TestPermissionOptionsAndEventsCompile(t *testing.T) {
	var bridge PermissionBridge = permissionBridgeFunc(func(context.Context, permissions.Decision) (permissions.Confirmation, error) {
		return permissions.Confirmation{Choice: permissions.ChoiceDeny}, nil
	})
	opts := Options{Permissions: &permissions.Engine{}, Confirmer: bridge}
	if opts.Permissions == nil || opts.Confirmer == nil {
		t.Fatalf("permission options were not retained: %#v", opts)
	}
	decision := permissions.Decision{Action: permissions.ActionAsk, Stage: permissions.StageMode}
	event := Event{Type: EventPermissionRequest, PermissionDecision: &decision}
	if event.Type != EventPermissionRequest || event.PermissionDecision.Action != permissions.ActionAsk {
		t.Fatalf("unexpected permission event: %#v", event)
	}
	scheduler := NewScheduler(tools.NewRegistry(), tools.NewExecutor(time.Second), nil, nil)
	if scheduler == nil {
		t.Fatal("nil permission scheduler was not created")
	}
}

type countingTool struct {
	name   string
	safety tools.Safety
	count  *atomic.Int32
}

func (t countingTool) Metadata() tools.Metadata {
	return tools.Metadata{
		Name:        t.name,
		Description: "counting tool",
		Safety:      t.safety,
		Schema:      tools.Schema{"type": "object"},
		Permission:  tools.PermissionMetadata{Target: tools.PermissionTargetNone},
	}
}

func (t countingTool) Execute(context.Context, json.RawMessage) tools.Result {
	t.count.Add(1)
	return tools.Success(t.name, map[string]any{"ok": true})
}

func TestSchedulerPermissionDenyDoesNotExecute(t *testing.T) {
	registry := tools.NewRegistry()
	var executed atomic.Int32
	if err := registry.Register(countingTool{name: "write", safety: tools.SafetySideEffect, count: &executed}); err != nil {
		t.Fatal(err)
	}
	gate := &permissions.Engine{Mode: permissions.ModeRelaxed, Rules: permissions.NewRuleStore(permissions.RuleSet{
		Session: []permissions.Rule{mustAgentRule(t, "write(*)", permissions.EffectDeny, permissions.ScopeSession)},
	}), Sandbox: mustSandbox(t)}
	s := NewScheduler(registry, tools.NewExecutor(time.Second), gate, nil)
	results, err := s.Execute(context.Background(), []provider.ToolCall{{ID: "1", Name: "write", Arguments: []byte(`{}`)}}, func(Event) bool { return true })
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if executed.Load() != 0 || len(results) != 1 || !results[0].IsError {
		t.Fatalf("executed=%d results=%+v", executed.Load(), results)
	}
}

func TestSchedulerPermissionAskChoices(t *testing.T) {
	tests := []struct {
		name       string
		choice     permissions.Choice
		wantExec   int32
		wantErr    bool
		wantCancel bool
	}{
		{name: "allow once", choice: permissions.ChoiceAllowOnce, wantExec: 1},
		{name: "allow session", choice: permissions.ChoiceAllowSession, wantExec: 1},
		{name: "deny", choice: permissions.ChoiceDeny, wantExec: 0, wantErr: true},
		{name: "cancel", choice: permissions.ChoiceCancel, wantExec: 0, wantCancel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := tools.NewRegistry()
			var executed atomic.Int32
			if err := registry.Register(countingTool{name: "write", safety: tools.SafetySideEffect, count: &executed}); err != nil {
				t.Fatal(err)
			}
			gate := &permissions.Engine{Mode: permissions.ModeStrict, Rules: permissions.NewRuleStore(permissions.RuleSet{}), Sandbox: mustSandbox(t)}
			confirmer := permissionBridgeFunc(func(context.Context, permissions.Decision) (permissions.Confirmation, error) {
				return permissions.Confirmation{Choice: tt.choice}, nil
			})
			s := NewScheduler(registry, tools.NewExecutor(time.Second), gate, confirmer)
			results, err := s.Execute(context.Background(), []provider.ToolCall{{ID: "1", Name: "write", Arguments: []byte(`{}`)}}, func(Event) bool { return true })
			if tt.wantCancel {
				if err == nil {
					t.Fatal("expected cancel error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if executed.Load() != tt.wantExec {
				t.Fatalf("executed=%d want %d", executed.Load(), tt.wantExec)
			}
			if len(results) != 1 || results[0].IsError != tt.wantErr {
				t.Fatalf("results=%+v wantErr=%v", results, tt.wantErr)
			}
		})
	}
}

func TestSchedulerPermissionMultiToolOrder(t *testing.T) {
	registry := tools.NewRegistry()
	var read1, write, read2 atomic.Int32
	for _, tool := range []countingTool{
		{name: "read1", safety: tools.SafetyReadOnly, count: &read1},
		{name: "write", safety: tools.SafetySideEffect, count: &write},
		{name: "read2", safety: tools.SafetyReadOnly, count: &read2},
	} {
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}
	gate := &permissions.Engine{Mode: permissions.ModeRelaxed, Rules: permissions.NewRuleStore(permissions.RuleSet{
		Session: []permissions.Rule{mustAgentRule(t, "write(*)", permissions.EffectDeny, permissions.ScopeSession)},
	}), Sandbox: mustSandbox(t)}
	s := NewScheduler(registry, tools.NewExecutor(time.Second), gate, nil)
	results, err := s.Execute(context.Background(), []provider.ToolCall{
		{ID: "1", Name: "read1", Arguments: []byte(`{}`)},
		{ID: "2", Name: "write", Arguments: []byte(`{}`)},
		{ID: "3", Name: "read2", Arguments: []byte(`{}`)},
	}, func(Event) bool { return true })
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if read1.Load() != 1 || write.Load() != 0 || read2.Load() != 1 {
		t.Fatalf("executions read1=%d write=%d read2=%d", read1.Load(), write.Load(), read2.Load())
	}
	if len(results) != 3 || results[0].CallID != "1" || results[1].CallID != "2" || results[2].CallID != "3" || !results[1].IsError {
		t.Fatalf("unexpected results order: %+v", results)
	}
}

func TestSchedulerHookRejectDoesNotExecute(t *testing.T) {
	registry := tools.NewRegistry()
	var executed atomic.Int32
	if err := registry.Register(countingTool{name: "write", safety: tools.SafetySideEffect, count: &executed}); err != nil {
		t.Fatal(err)
	}
	engine := hooks.NewEngine([]hooks.Rule{{ID: "block", Event: hooks.EventPreToolUse, Action: hooks.Action{Type: hooks.ActionPrompt, Message: "blocked"}, Reject: true}}, hooks.Executor{PromptSink: hookTestSink{}}, nil)
	scheduler := NewScheduler(registry, tools.NewExecutor(time.Second), nil, nil)
	scheduler.Hooks = engine
	results, err := scheduler.Execute(context.Background(), []provider.ToolCall{{ID: "1", Name: "write", Arguments: []byte(`{}`)}}, func(Event) bool { return true })
	if err != nil || executed.Load() != 0 || len(results) != 1 || !results[0].IsError || !strings.Contains(results[0].Content, "hook") {
		t.Fatalf("err=%v executed=%d results=%+v", err, executed.Load(), results)
	}
}

func TestSchedulerRunCommandDoesNotEmitSlashCommandHook(t *testing.T) {
	registry := tools.NewRegistry()
	var executed atomic.Int32
	if err := registry.Register(countingTool{name: "run_command", safety: tools.SafetySideEffect, count: &executed}); err != nil {
		t.Fatal(err)
	}
	sink := &recordingHookSink{}
	engine := hooks.NewEngine([]hooks.Rule{{ID: "slash-only", Event: hooks.EventCommandExecute, Action: hooks.Action{Type: hooks.ActionPrompt, Message: "slash notification"}}}, hooks.Executor{PromptSink: sink}, nil)
	scheduler := NewScheduler(registry, tools.NewExecutor(time.Second), nil, nil)
	scheduler.Hooks = engine

	results, err := scheduler.Execute(context.Background(), []provider.ToolCall{{ID: "1", Name: "run_command", Arguments: []byte(`{"command":"printf","args":["ok"]}`)}}, func(Event) bool { return true })
	if err != nil || executed.Load() != 1 || len(results) != 1 || results[0].IsError {
		t.Fatalf("err=%v executed=%d results=%+v", err, executed.Load(), results)
	}
	if len(sink.messages) != 0 {
		t.Fatalf("run_command emitted command_execute hook messages=%q", sink.messages)
	}
}

type hookTestSink struct{}

func (hookTestSink) AddHookNotification(string) {}

type recordingHookSink struct{ messages []string }

func (s *recordingHookSink) AddHookNotification(message string) {
	s.messages = append(s.messages, message)
}

func mustAgentRule(t *testing.T, key string, effect permissions.Effect, scope permissions.Scope) permissions.Rule {
	t.Helper()
	rule, err := permissions.ParseRule(key, effect, scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func mustSandbox(t *testing.T) permissions.Sandbox {
	t.Helper()
	sandbox, err := permissions.NewSandbox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return sandbox
}

type permissionBridgeFunc func(context.Context, permissions.Decision) (permissions.Confirmation, error)

func (f permissionBridgeFunc) Confirm(ctx context.Context, decision permissions.Decision) (permissions.Confirmation, error) {
	return f(ctx, decision)
}

func closedSignal() <-chan struct{} { ch := make(chan struct{}); close(ch); return ch }
