package hooks

import (
	"context"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
)

type Engine struct {
	rules    []Rule
	executor Executor
	logger   *logging.Logger
	mu       sync.Mutex
	executed map[string]bool
}

func NewEngine(rules []Rule, executor Executor, logger *logging.Logger) *Engine {
	if logger == nil {
		logger = logging.Nop()
	}
	return &Engine{rules: append([]Rule(nil), rules...), executor: executor, logger: logger, executed: make(map[string]bool)}
}

func (e *Engine) SetPromptSink(sink PromptSink) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executor.PromptSink = sink
}

func (e *Engine) SetAgentRunner(runner AgentRunner) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executor.AgentRunner = runner
}

func (e *Engine) Run(ctx context.Context, event Event, hookCtx Context) []Result {
	if e == nil {
		return nil
	}
	hookCtx.Event = event
	var results []Result
	for _, rule := range e.matching(event, hookCtx) {
		if rule.Async {
			e.runAsync(ctx, rule, hookCtx)
			continue
		}
		result := e.run(ctx, rule, hookCtx)
		results = append(results, result)
	}
	return results
}

func (e *Engine) RunPreTool(ctx context.Context, hookCtx Context) (Result, bool) {
	if e == nil {
		return Result{}, false
	}
	hookCtx.Event = EventPreToolUse
	for _, rule := range e.matching(EventPreToolUse, hookCtx) {
		result := e.run(ctx, rule, hookCtx)
		if rule.Reject {
			result.Rejected = true
			return result, true
		}
	}
	return Result{}, false
}

func (e *Engine) matching(event Event, hookCtx Context) []Rule {
	var matched []Rule
	for _, rule := range e.rules {
		if rule.Event == event && rule.Condition.Match(hookCtx) && e.claim(rule) {
			matched = append(matched, rule)
		}
	}
	return matched
}

func (e *Engine) claim(rule Rule) bool {
	if !rule.Once {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.executed[rule.ID] {
		return false
	}
	e.executed[rule.ID] = true
	return true
}

func (e *Engine) runAsync(ctx context.Context, rule Rule, hookCtx Context) {
	go func() { result := e.run(ctx, rule, hookCtx); result.Async = true }()
}
func (e *Engine) run(ctx context.Context, rule Rule, hookCtx Context) Result {
	result := e.executor.Execute(ctx, rule.Action, hookCtx)
	result.RuleID = rule.ID
	fields := logging.Fields{"stage": "hook", "event": string(rule.Event), "rule_id": rule.ID, "action": string(rule.Action.Type), "status": "completed", "async": rule.Async, "duration_ms": result.Duration.Milliseconds(), "exit_code": result.ExitCode, "http_status": result.StatusCode}
	if result.Err != nil {
		fields["status"] = "failed"
		e.logger.Error("hook action failed", fields)
	} else {
		e.logger.Info("hook action completed", fields)
	}
	return result
}
