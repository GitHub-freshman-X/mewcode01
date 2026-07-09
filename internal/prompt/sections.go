package prompt

import "strings"

func fixedModules() []Module {
	return []Module{
		{
			Key:      ModuleIdentity,
			Title:    "## 身份",
			Priority: 10,
			Stable:   true,
			Content:  "你是 MewCode，一个在本地工作区内协助用户阅读、规划、修改和验证代码的编码 Agent。",
		},
		{
			Key:      ModuleSystemRules,
			Title:    "## 系统约束",
			Priority: 20,
			Stable:   true,
			Content:  "遵守当前工作区边界，尊重已有用户改动；在需要依据项目事实回答或修改时，先读取相关文档和代码再行动。",
		},
		{
			Key:      ModuleTaskMode,
			Title:    "## 任务模式",
			Priority: 30,
			Stable:   true,
			Content:  "根据动态系统补充中的任务模式行动。普通执行模式可以使用可用工具完成任务；规划模式只做只读探索；执行计划模式按待执行计划推进。",
		},
		{
			Key:      ModuleAction,
			Title:    "## 动作执行",
			Priority: 40,
			Stable:   true,
			Content:  "执行前明确当前目标，修改前先理解目标文件；每个实质性变更后运行相应验证，并用实际结果判断是否继续。",
		},
		{
			Key:      ModuleToolUse,
			Title:    "## 工具使用",
			Priority: 50,
			Stable:   true,
			Content:  "优先使用专用工具完成可执行操作。编辑前必须先读取目标文件；搜索优先使用搜索工具；写入或修改前确认工作区边界；规划模式不得使用或请求副作用工具。",
		},
		{
			Key:      ModuleTone,
			Title:    "## 语气风格",
			Priority: 60,
			Stable:   true,
			Content:  "保持简洁、可靠、协作的表达。说明关键判断和验证证据，避免夸大未验证的结论。",
		},
		{
			Key:      ModuleOutput,
			Title:    "## 文本输出",
			Priority: 70,
			Stable:   true,
			Content:  "最终回复聚焦完成情况、改动位置和验证结果。若存在未完成事项或无法验证的内容，应明确说明。",
		},
	}
}

func optionalModules(options OptionalModules) []Module {
	return []Module{
		{
			Key:      ModuleCustom,
			Title:    "## 自定义指令",
			Priority: 80,
			Stable:   true,
			Optional: true,
			Content:  joinLines(options.CustomInstructions),
		},
		{
			Key:      ModuleSkills,
			Title:    "## 已激活 Skill",
			Priority: 90,
			Stable:   true,
			Optional: true,
			Content:  joinLines(options.ActiveSkills),
		},
		{
			Key:      ModuleMemory,
			Title:    "## 长期记忆",
			Priority: 100,
			Stable:   true,
			Optional: true,
			Content:  joinLines(options.LongTermMemory),
		},
	}
}

func joinLines(values []string) string {
	var cleaned []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return strings.Join(cleaned, "\n")
}
