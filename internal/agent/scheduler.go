package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/hooks"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Scheduler struct {
	registry  *tools.Registry
	executor  *tools.Executor
	gate      *permissions.Engine
	confirmer PermissionBridge
	Hooks     *hooks.Engine
}

type scheduledCall struct {
	index int
	call  provider.ToolCall
}
type toolBatch struct {
	concurrent bool
	calls      []scheduledCall
}

func NewScheduler(registry *tools.Registry, executor *tools.Executor, gate *permissions.Engine, confirmer PermissionBridge) *Scheduler {
	return &Scheduler{registry: registry, executor: executor, gate: gate, confirmer: confirmer}
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
		if batch.concurrent && len(batch.calls) > 1 && s.gate == nil {
			var wg sync.WaitGroup
			var stop sync.Once
			errCh := make(chan error, 1)
			wg.Add(len(batch.calls))
			for _, item := range batch.calls {
				item := item
				go func() {
					defer wg.Done()
					result, err := s.executePermitted(ctx, item.call, emit)
					if err != nil {
						stop.Do(func() { errCh <- err })
						return
					}
					results[item.index] = result
				}()
			}
			wg.Wait()
			select {
			case err := <-errCh:
				return nil, err
			default:
			}
		} else {
			for _, item := range batch.calls {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				result, err := s.executePermitted(ctx, item.call, emit)
				if err != nil {
					return nil, err
				}
				results[item.index] = result
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

func (s *Scheduler) executePermitted(ctx context.Context, call provider.ToolCall, emit func(Event) bool) (provider.ToolResult, error) {
	if s.Hooks != nil {
		var args map[string]any
		_ = json.Unmarshal(call.Arguments, &args)
		result, rejected := s.Hooks.RunPreTool(ctx, hooks.Context{Tool: call.Name, Args: args})
		if rejected {
			payload := tools.Failure(call.Name, tools.ErrorPermission, "tool call rejected by hook", map[string]any{"stage": "hook", "rule_id": result.RuleID, "reason": result.Output})
			s.Hooks.Run(ctx, hooks.EventPostToolUse, hookContext(call))
			return provider.ToolResult{CallID: call.ID, Name: call.Name, Content: payload.JSON(), IsError: true}, nil
		}
	}
	if s.gate == nil {
		return s.executeAndNotify(ctx, call), nil
	}
	tool, ok := s.registry.Get(call.Name)
	if !ok {
		return s.executeAndNotify(ctx, call), nil
	}
	decision, err := s.gate.Decide(ctx, call, tool)
	if err != nil {
		return provider.ToolResult{}, err
	}
	if !emit(Event{Type: EventPermissionDecision, Phase: PhaseRunningTools, PermissionDecision: &decision}) {
		return provider.ToolResult{}, context.Canceled
	}
	switch decision.Action {
	case permissions.ActionAllow:
		return s.executeAndNotify(ctx, call), nil
	case permissions.ActionDeny:
		return permissions.DeniedToolResult(call, decision), nil
	case permissions.ActionAsk:
		if s.Hooks != nil {
			s.Hooks.Run(ctx, hooks.EventPermissionRequest, hookContext(call))
		}
		if s.confirmer == nil {
			return permissions.DeniedToolResult(call, decision), nil
		}
		if !emit(Event{Type: EventPermissionRequest, Phase: PhaseRunningTools, PermissionDecision: &decision}) {
			return provider.ToolResult{}, context.Canceled
		}
		confirmation, err := s.confirmer.Confirm(ctx, decision)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return provider.ToolResult{}, context.Canceled
			}
			return provider.ToolResult{}, err
		}
		if confirmation.Decision.SuggestedKey == "" {
			confirmation.Decision = decision
		}
		if !emit(Event{Type: EventPermissionResponse, Phase: PhaseRunningTools, PermissionConfirmation: &confirmation}) {
			return provider.ToolResult{}, context.Canceled
		}
		if confirmation.Choice == permissions.ChoiceCancel {
			return provider.ToolResult{}, context.Canceled
		}
		if confirmation.Choice == permissions.ChoiceDeny {
			denied := decision
			denied.Action = permissions.ActionDeny
			denied.Stage = permissions.StageConfirm
			denied.Reason = "user denied permission"
			return permissions.DeniedToolResult(call, denied), nil
		}
		if err := s.gate.ApplyConfirmation(confirmation); err != nil {
			return provider.ToolResult{}, err
		}
		return s.executeAndNotify(ctx, call), nil
	default:
		return provider.ToolResult{}, errors.New("unknown permission decision")
	}
}

func (s *Scheduler) executeAndNotify(ctx context.Context, call provider.ToolCall) provider.ToolResult {
	result := s.executor.Execute(ctx, s.registry, call)
	if s.Hooks == nil {
		return result
	}
	hookCtx := hookContext(call)
	s.Hooks.Run(ctx, hooks.EventPostToolUse, hookCtx)
	switch call.Name {
	case "write_file", "edit_file":
		s.Hooks.Run(ctx, hooks.EventFileChange, hookCtx)
	}
	return result
}

func hookContext(call provider.ToolCall) hooks.Context {
	var args map[string]any
	_ = json.Unmarshal(call.Arguments, &args)
	path, _ := args["path"].(string)
	return hooks.Context{Tool: call.Name, Args: args, FilePath: path}
}
