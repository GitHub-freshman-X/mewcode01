package skills

func Builtins() []Skill {
	return []Skill{
		{
			Metadata: Metadata{Name: "commit", Description: "分析当前变更并创建规范的 Git 提交。", Mode: ModeInline},
			Path:     "builtin:commit",
			Body: `# Git 提交

分析当前工作区变更，拟定准确、简洁的英文 Conventional Commit message。先检查状态和 diff，避免提交密钥、凭据或无关文件。根据用户补充要求调整提交范围与信息：{{args}}`,
		},
		{
			Metadata: Metadata{Name: "review", Description: "结合当前对话审查代码变更并报告可操作问题。", Mode: ModeInline},
			Path:     "builtin:review",
			Body: `# 代码审查

审查当前变更，优先报告会造成正确性、安全性、兼容性或可维护性问题的事项。每项应包含文件位置、影响和建议；没有问题时明确说明。`,
		},
		{
			Metadata: Metadata{Name: "test", Description: "识别并运行与当前变更相关的验证。", Mode: ModeInline},
			Path:     "builtin:test",
			Body: `# 测试验证

根据当前变更识别最相关的测试与构建命令，按风险从窄到宽运行。报告实际结果、失败原因及仍未验证的部分。用户附加要求：{{args}}`,
		},
	}
}
