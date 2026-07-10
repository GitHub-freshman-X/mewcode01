package permissions

import (
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func DeniedToolResult(call provider.ToolCall, decision Decision) provider.ToolResult {
	toolName := call.Name
	if toolName == "" {
		toolName = decision.Request.Tool
	}
	details := map[string]any{
		"stage": string(decision.Stage),
		"tool":  toolName,
	}
	if decision.Request.MatchTarget != "" {
		details["match_target"] = decision.Request.MatchTarget
	}
	if decision.Rule != nil {
		details["scope"] = string(decision.Rule.Scope)
		details["effect"] = string(decision.Rule.Effect)
	}
	if decision.Reason != "" {
		details["reason"] = decision.Reason
	}
	payload := tools.Failure(toolName, tools.ErrorPermission, "tool call denied by permission policy", details)
	return provider.ToolResult{
		CallID:  call.ID,
		Name:    toolName,
		Content: payload.JSON(),
		IsError: true,
	}
}
