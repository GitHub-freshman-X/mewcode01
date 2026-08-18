package skills

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

const LoadToolName = "load_skill"

type loadTool struct{ manager *Manager }

func NewLoadTool(manager *Manager) tools.Tool { return loadTool{manager: manager} }

func (t loadTool) Metadata() tools.Metadata {
	return tools.Metadata{Name: LoadToolName, Description: "加载指定 Skill 的操作流程，供下一轮任务使用。", Safety: tools.SafetyReadOnly, Schema: tools.Schema{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "description": "要加载的 Skill 名称"}}, "required": []string{"name"}, "additionalProperties": false}}
}

func (t loadTool) Execute(_ context.Context, input json.RawMessage) tools.Result {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil || strings.TrimSpace(args.Name) == "" {
		return tools.Failure(LoadToolName, tools.ErrorValidation, "name is required", nil)
	}
	skill, err := t.manager.Activate(args.Name, "")
	if err != nil {
		return tools.Failure(LoadToolName, tools.ErrorNotFound, err.Error(), nil)
	}
	return tools.Success(LoadToolName, map[string]any{"name": skill.Name, "description": skill.Description, "mode": skill.Mode})
}
