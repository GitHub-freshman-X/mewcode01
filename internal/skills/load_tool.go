package skills

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

const (
	LoadToolName = "load_skill"
	RunToolName  = "run_skill"
)

type loadTool struct{ manager *Manager }

type runTool struct{ manager *Manager }

type ForkSkillInput struct {
	Name   string
	Prompt string
}

type ForkSkillHost interface {
	ExecuteForkSkill(context.Context, ForkSkillInput) (tools.Result, error)
}

type forkSkillHostKey struct{}

func WithForkSkillHost(ctx context.Context, host ForkSkillHost) context.Context {
	return context.WithValue(ctx, forkSkillHostKey{}, host)
}

func ForkSkillHostFromContext(ctx context.Context) (ForkSkillHost, bool) {
	host, ok := ctx.Value(forkSkillHostKey{}).(ForkSkillHost)
	return host, ok && host != nil
}

func NewLoadTool(manager *Manager) tools.Tool { return loadTool{manager: manager} }

func NewRunTool(manager *Manager) tools.Tool { return runTool{manager: manager} }

func (t loadTool) Metadata() tools.Metadata {
	return tools.Metadata{Name: LoadToolName, Description: "加载 inline Skill 的操作流程；fork Skill 会返回执行指引。", Safety: tools.SafetyReadOnly, Schema: tools.Schema{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "要加载的 Skill 名称"}}, "required": []string{"name"}, "additionalProperties": false}}
}

func (t loadTool) Execute(_ context.Context, input json.RawMessage) tools.Result {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil || strings.TrimSpace(args.Name) == "" {
		return tools.Failure(LoadToolName, tools.ErrorValidation, "name is required", nil)
	}
	skill, ok := t.manager.Skill(args.Name)
	if !ok {
		return tools.Failure(LoadToolName, tools.ErrorNotFound, "unknown skill "+strings.TrimSpace(args.Name), nil)
	}
	if skill.Mode == ModeFork {
		return tools.Success(LoadToolName, map[string]any{"name": skill.Name, "description": skill.Description, "mode": skill.Mode, "requires_execution": true})
	}
	if _, err := t.manager.Activate(args.Name, ""); err != nil {
		return tools.Failure(LoadToolName, tools.ErrorNotFound, err.Error(), nil)
	}
	return tools.Success(LoadToolName, map[string]any{"name": skill.Name, "description": skill.Description, "mode": skill.Mode})
}

func (t runTool) Metadata() tools.Metadata {
	return tools.Metadata{Name: RunToolName, Description: "在独立会话中执行已选择的 fork Skill，并返回最终摘要。", Safety: tools.SafetyReadOnly, Schema: tools.Schema{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "fork Skill 名称"}, "prompt": map[string]any{"type": "string", "description": "要在独立会话中完成的完整子任务"}}, "required": []string{"name", "prompt"}, "additionalProperties": false}}
}

func (t runTool) Execute(ctx context.Context, input json.RawMessage) tools.Result {
	var args ForkSkillInput
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Failure(RunToolName, tools.ErrorValidation, "invalid run_skill input", nil)
	}
	args.Name = strings.TrimSpace(args.Name)
	args.Prompt = strings.TrimSpace(args.Prompt)
	if args.Name == "" || args.Prompt == "" {
		return tools.Failure(RunToolName, tools.ErrorValidation, "name and prompt are required", nil)
	}
	skill, ok := t.manager.Skill(args.Name)
	if !ok {
		return tools.Failure(RunToolName, tools.ErrorNotFound, "unknown skill "+args.Name, nil)
	}
	if skill.Mode != ModeFork {
		return tools.Failure(RunToolName, tools.ErrorValidation, "skill "+args.Name+" is not a fork skill", nil)
	}
	host, ok := ForkSkillHostFromContext(ctx)
	if !ok {
		return tools.Failure(RunToolName, tools.ErrorInternal, "fork skill runtime is not configured", nil)
	}
	result, err := host.ExecuteForkSkill(ctx, args)
	if err != nil {
		return tools.Failure(RunToolName, tools.ErrorExecution, "fork skill execution failed", map[string]any{"cause": err.Error()})
	}
	return result
}
