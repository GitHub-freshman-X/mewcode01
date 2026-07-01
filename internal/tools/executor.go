package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Executor struct {
	Timeout time.Duration
}

func NewExecutor(timeout time.Duration) *Executor {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Executor{Timeout: timeout}
}

func (e *Executor) Execute(ctx context.Context, registry *Registry, call provider.ToolCall) provider.ToolResult {
	name := call.Name
	callID := call.ID
	result := e.execute(ctx, registry, call)
	return provider.ToolResult{CallID: callID, Name: name, Content: result.JSON(), IsError: !result.Success}
}

func (e *Executor) execute(ctx context.Context, registry *Registry, call provider.ToolCall) (result Result) {
	if e == nil || registry == nil {
		return Failure(call.Name, ErrorInternal, "tool executor is not configured", nil)
	}
	tool, ok := registry.Get(call.Name)
	if !ok {
		return Failure(call.Name, ErrorNotFound, "tool is not registered", map[string]any{"tool": call.Name})
	}
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	done := make(chan Result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- Failure(call.Name, ErrorInternal, "tool panicked", map[string]any{"panic": fmt.Sprint(r)})
			}
		}()
		if !json.Valid(call.Arguments) {
			done <- Failure(call.Name, ErrorValidation, "tool arguments are not valid JSON", nil)
			return
		}
		done <- tool.Execute(ctx, call.Arguments)
	}()
	select {
	case <-ctx.Done():
		return Failure(call.Name, ErrorTimeout, "tool execution timed out", map[string]any{"timeout_ms": e.Timeout.Milliseconds()})
	case result := <-done:
		return result
	}
}
