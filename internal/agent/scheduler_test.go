package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

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
	s := NewScheduler(registry, tools.NewExecutor(time.Second))
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
	s := NewScheduler(registry, tools.NewExecutor(time.Second))
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
	s := NewScheduler(registry, tools.NewExecutor(time.Second))
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
	batches := NewScheduler(registry, tools.NewExecutor(time.Second)).batches([]provider.ToolCall{{Name: "unknown_class"}})
	if len(batches) != 1 || batches[0].concurrent {
		t.Fatalf("batches=%+v", batches)
	}
}

func closedSignal() <-chan struct{} { ch := make(chan struct{}); close(ch); return ch }
