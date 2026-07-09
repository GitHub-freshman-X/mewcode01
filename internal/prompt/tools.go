package prompt

import (
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

const globalToolRule = "规则强化：优先使用专用工具完成可执行操作；编辑前必须先读取目标文件；搜索优先使用搜索工具；写入或修改前确认工作区边界。"

func EnhanceDefinitions(defs []provider.ToolDefinition, mode Mode) []provider.ToolDefinition {
	out := make([]provider.ToolDefinition, len(defs))
	for i, def := range defs {
		out[i] = def
		out[i].Description = strings.TrimSpace(def.Description)
		out[i].Description = appendRule(out[i].Description, globalToolRule)
		if rule := ruleForTool(def.Name); rule != "" {
			out[i].Description = appendRule(out[i].Description, rule)
		}
		if mode == ModePlan {
			out[i].Description = appendRule(out[i].Description, "规划模式不得请求副作用工具；如果工具不在当前可用列表中，不要尝试调用。")
		}
		out[i].Cacheable = true
	}
	return out
}

func appendRule(description, rule string) string {
	if description == "" {
		return rule
	}
	if strings.Contains(description, rule) {
		return description
	}
	return description + "\n\n" + rule
}

func ruleForTool(name string) string {
	switch name {
	case "read_file":
		return "读取规则：在编辑或解释文件前，先读取目标文件并基于实际内容回答。"
	case "write_file", "edit_file":
		return "写入规则：写入或修改前确认工作区边界，并确保目标文件内容已被读取或明确了解。"
	case "run_command":
		return "命令规则：执行命令前说明目的，避免与当前任务无关或会越过工作区边界的操作。"
	case "find_files":
		return "查找规则：需要定位文件时优先使用文件查找工具。"
	case "search_code":
		return "搜索规则：需要定位符号、文本或实现时优先使用搜索工具。"
	default:
		return ""
	}
}
