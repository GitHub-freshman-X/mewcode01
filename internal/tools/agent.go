package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type AgentInput struct {
	Prompt          string `json:"prompt"`
	Description     string `json:"description"`
	SubagentType    string `json:"subagent_type"`
	Model           string `json:"model"`
	RunInBackground bool   `json:"run_in_background"`
	Name            string `json:"name"`
	Isolation       string `json:"isolation"`
}

type SubAgentHost interface {
	DispatchSubAgent(context.Context, AgentInput) (Result, error)
}

type subAgentHostKey struct{}

func WithSubAgentHost(ctx context.Context, host SubAgentHost) context.Context {
	return context.WithValue(ctx, subAgentHostKey{}, host)
}

func SubAgentHostFromContext(ctx context.Context) (SubAgentHost, bool) {
	host, ok := ctx.Value(subAgentHostKey{}).(SubAgentHost)
	return host, ok && host != nil
}

type AgentTool struct{}

func NewAgentTool() *AgentTool { return &AgentTool{} }

func (t *AgentTool) Metadata() Metadata {
	return Metadata{Name: "agent", Description: "Delegate a focused task to an independent subagent.", Safety: SafetySideEffect, Permission: PermissionMetadata{Target: PermissionTargetNone}, Schema: Schema{
		"type": "object",
		"properties": map[string]any{
			"prompt":            map[string]any{"type": "string", "description": "The complete subtask instructions."},
			"description":       map[string]any{"type": "string", "description": "A short description of the subtask."},
			"subagent_type":     map[string]any{"type": "string", "description": "Optional predefined agent role. Omit it or set it to fork for a forked subagent."},
			"model":             map[string]any{"type": "string", "description": "Optional model override."},
			"run_in_background": map[string]any{"type": "boolean", "description": "Run without blocking the caller."},
			"name":              map[string]any{"type": "string", "description": "Optional task name."},
			"isolation":         map[string]any{"type": "string", "description": "Optional isolation mode; use worktree for an isolated Git worktree."},
		},
		"required": []string{"prompt", "description"},
	}}
}

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage) Result {
	var request AgentInput
	if err := json.Unmarshal(input, &request); err != nil {
		return Failure(t.Metadata().Name, ErrorValidation, "invalid agent input", map[string]any{"cause": err.Error()})
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.Description = strings.TrimSpace(request.Description)
	request.SubagentType = strings.TrimSpace(request.SubagentType)
	request.Model = strings.TrimSpace(request.Model)
	request.Name = strings.TrimSpace(request.Name)
	request.Isolation = strings.TrimSpace(request.Isolation)
	if request.Prompt == "" || request.Description == "" {
		return Failure(t.Metadata().Name, ErrorValidation, "prompt and description are required", nil)
	}
	if request.Isolation != "" && request.Isolation != "none" && request.Isolation != "worktree" {
		return Failure(t.Metadata().Name, ErrorValidation, fmt.Sprintf("unsupported isolation %q", request.Isolation), nil)
	}
	host, ok := SubAgentHostFromContext(ctx)
	if !ok {
		return Failure(t.Metadata().Name, ErrorInternal, "subagent runtime is not configured", nil)
	}
	result, err := host.DispatchSubAgent(ctx, request)
	if err != nil {
		return Failure(t.Metadata().Name, ErrorExecution, "subagent dispatch failed", map[string]any{"cause": err.Error()})
	}
	return result
}
