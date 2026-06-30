package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

type RunCommandTool struct {
	workspace  *Workspace
	MaxTimeout time.Duration
}

func NewRunCommandTool(workspace *Workspace) *RunCommandTool {
	return &RunCommandTool{workspace: workspace, MaxTimeout: 30 * time.Second}
}

func (t *RunCommandTool) Metadata() Metadata {
	return Metadata{
		Name:        "run_command",
		Description: "Run a local command in the workspace using command and args without shell expansion.",
		Schema: Schema{"type": "object", "required": []any{"command"}, "properties": map[string]any{
			"command":    map[string]any{"type": "string"},
			"args":       map[string]any{"type": "array"},
			"timeout_ms": map[string]any{"type": "integer", "minimum": 1.0, "maximum": float64(t.MaxTimeout.Milliseconds())},
		}},
	}
}

func (t *RunCommandTool) Execute(ctx context.Context, input json.RawMessage) Result {
	args, verr := Validate(t.Metadata().Schema, input)
	if verr != nil {
		return Failure(t.Metadata().Name, verr.Type, verr.Message, verr.Details)
	}
	command := args["command"].(string)
	if command == "" {
		return Failure(t.Metadata().Name, ErrorValidation, "command must not be empty", nil)
	}
	timeout := t.MaxTimeout
	if raw, ok := args["timeout_ms"]; ok {
		if n, ok := numeric(raw); ok {
			timeout = time.Duration(n) * time.Millisecond
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var argv []string
	if rawArgs, ok := args["args"].([]any); ok {
		for _, rawArg := range rawArgs {
			s, ok := rawArg.(string)
			if !ok {
				return Failure(t.Metadata().Name, ErrorValidation, "args must contain only strings", nil)
			}
			argv = append(argv, s)
		}
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, command, argv...)
	cmd.Dir = t.workspace.Root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	duration := time.Since(start)
	out, outTruncated := TruncateString(stdout.String(), t.workspace.OutputLimit)
	errOut, errTruncated := TruncateString(stderr.String(), t.workspace.OutputLimit)
	exitCode := 0
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if !timedOut {
			return Failure(t.Metadata().Name, ErrorExecution, "failed to start command", map[string]any{"cause": err.Error(), "command": command})
		}
	}
	if timedOut && runtime.GOOS == "windows" {
		exitCode = -1
	}
	return Success(t.Metadata().Name, map[string]any{
		"command": command, "args": argv, "exit_code": exitCode, "stdout": out, "stderr": errOut,
		"stdout_truncated": outTruncated, "stderr_truncated": errTruncated, "timed_out": timedOut,
		"duration_ms": duration.Milliseconds(),
	})
}
