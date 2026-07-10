package permissions

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func BuildRequest(call provider.ToolCall, tool tools.Tool, sandbox Sandbox) (Request, error) {
	if tool == nil {
		return Request{}, errors.New("tool is required")
	}
	if !json.Valid(call.Arguments) {
		return Request{}, errors.New("tool arguments are not valid JSON")
	}
	var args map[string]any
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return Request{}, fmt.Errorf("decode tool arguments: %w", err)
	}
	meta := tool.Metadata()
	req := Request{
		CallID:    call.ID,
		Tool:      call.Name,
		Arguments: append(json.RawMessage(nil), call.Arguments...),
		Safety:    tools.NormalizeSafety(meta.Safety),
	}
	if req.Tool == "" {
		req.Tool = meta.Name
	}
	for _, param := range meta.Permission.PathParams {
		raw, ok := args[param].(string)
		if !ok {
			return Request{}, fmt.Errorf("path parameter %q must be a string", param)
		}
		check, err := sandbox.Resolve(raw, param)
		if err != nil {
			return Request{}, err
		}
		req.Paths = append(req.Paths, check)
	}
	switch meta.Permission.Target {
	case tools.PermissionTargetPath:
		if len(req.Paths) == 0 {
			return Request{}, errors.New("path target requires at least one path parameter")
		}
		req.MatchTarget = req.Paths[0].Relative
	case tools.PermissionTargetCommand:
		req.MatchTarget = CommandText(args)
		if req.MatchTarget == "" {
			return Request{}, errors.New("command target requires command")
		}
	case tools.PermissionTargetPattern:
		pattern, ok := args["pattern"].(string)
		if !ok || pattern == "" {
			return Request{}, errors.New("pattern target requires pattern")
		}
		req.MatchTarget = filepath.ToSlash(pattern)
	case tools.PermissionTargetNone, "":
		req.MatchTarget = "*"
	default:
		return Request{}, fmt.Errorf("unknown permission target %q", meta.Permission.Target)
	}
	return req, nil
}

func CommandText(args map[string]any) string {
	command, _ := args["command"].(string)
	var parts []string
	if command != "" {
		parts = append(parts, command)
	}
	if rawArgs, ok := args["args"].([]any); ok {
		for _, raw := range rawArgs {
			if s, ok := raw.(string); ok {
				parts = append(parts, s)
			}
		}
	}
	return normalizeCommandText(strings.Join(parts, " "))
}

func SuggestedRuleKey(req Request) string {
	return fmt.Sprintf("%s(%s)", req.Tool, req.MatchTarget)
}
