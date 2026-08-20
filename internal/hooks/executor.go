package hooks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Executor struct {
	HTTPClient  *http.Client
	PromptSink  PromptSink
	AgentRunner AgentRunner
}

func (e Executor) Execute(ctx context.Context, action Action, hookCtx Context) Result {
	started := time.Now()
	result := Result{Action: action.Type}
	switch action.Type {
	case ActionCommand:
		result = e.command(ctx, action, hookCtx)
	case ActionPrompt:
		message := hookCtx.Expand(action.Message)
		if e.PromptSink == nil {
			result.Err = fmt.Errorf("hook prompt sink is not configured")
		} else {
			e.PromptSink.AddHookNotification(message)
			result.Output = message
		}
	case ActionHTTP:
		result = e.http(ctx, action, hookCtx)
	case ActionAgent:
		prompt := hookCtx.Expand(action.Prompt)
		if e.AgentRunner == nil {
			result.Output = "hook agent runtime is not connected"
			result.Err = fmt.Errorf("%s", result.Output)
		} else {
			result.Output, result.Err = e.AgentRunner.RunHookAgent(ctx, prompt)
		}
	default:
		result.Err = fmt.Errorf("unsupported hook action %q", action.Type)
	}
	result.Action = action.Type
	result.Duration = time.Since(started)
	return result
}

func (e Executor) command(ctx context.Context, action Action, hookCtx Context) Result {
	if action.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, action.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", hookCtx.Expand(action.Command))
	output, err := cmd.CombinedOutput()
	result := Result{Output: string(output), Err: err}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.Err = fmt.Errorf("hook command timed out: %w", ctx.Err())
	}
	return result
}

func (e Executor) http(ctx context.Context, action Action, hookCtx Context) Result {
	method := action.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, hookCtx.Expand(action.URL), strings.NewReader(hookCtx.Expand(action.Body)))
	if err != nil {
		return Result{Err: err}
	}
	for key, value := range action.Headers {
		req.Header.Set(key, hookCtx.Expand(value))
	}
	client := e.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return Result{Err: err}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	if readErr != nil {
		return Result{StatusCode: response.StatusCode, Err: readErr}
	}
	result := Result{StatusCode: response.StatusCode, Output: string(body)}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Err = fmt.Errorf("hook HTTP request returned %s", response.Status)
	}
	return result
}
