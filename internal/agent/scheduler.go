package agent

import (
	"context"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Scheduler struct {
	registry *tools.Registry
	executor *tools.Executor
}

type scheduledCall struct {
	index int
	call  provider.ToolCall
}
type toolBatch struct {
	concurrent bool
	calls      []scheduledCall
}

func NewScheduler(registry *tools.Registry, executor *tools.Executor) *Scheduler {
	return &Scheduler{registry: registry, executor: executor}
}

func (s *Scheduler) batches(calls []provider.ToolCall) []toolBatch {
	var batches []toolBatch
	for i, call := range calls {
		tool, ok := s.registry.Get(call.Name)
		readOnly := ok && tools.NormalizeSafety(tool.Metadata().Safety) == tools.SafetyReadOnly
		if readOnly && len(batches) > 0 && batches[len(batches)-1].concurrent {
			batches[len(batches)-1].calls = append(batches[len(batches)-1].calls, scheduledCall{i, call})
			continue
		}
		batches = append(batches, toolBatch{concurrent: readOnly, calls: []scheduledCall{{i, call}}})
	}
	return batches
}

func (s *Scheduler) Execute(ctx context.Context, calls []provider.ToolCall, emit func(Event) bool) ([]provider.ToolResult, error) {
	results := make([]provider.ToolResult, len(calls))
	for _, batch := range s.batches(calls) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, item := range batch.calls {
			call := item.call
			if !emit(Event{Type: EventToolCall, Phase: PhaseRunningTools, ToolCall: &call}) {
				return nil, context.Canceled
			}
		}
		if batch.concurrent && len(batch.calls) > 1 {
			var wg sync.WaitGroup
			wg.Add(len(batch.calls))
			for _, item := range batch.calls {
				item := item
				go func() { defer wg.Done(); results[item.index] = s.executor.Execute(ctx, s.registry, item.call) }()
			}
			wg.Wait()
		} else {
			for _, item := range batch.calls {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				results[item.index] = s.executor.Execute(ctx, s.registry, item.call)
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, item := range batch.calls {
			result := results[item.index]
			if !emit(Event{Type: EventToolResult, Phase: PhaseRunningTools, ToolResult: &result}) {
				return nil, context.Canceled
			}
		}
	}
	return results, nil
}
