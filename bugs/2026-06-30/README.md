# 2026-06-30 Bug 记录

## 问题列表

| 编号 | 状态 | 问题 |
|---|---|---|
| [001](001-openai-output-item-done-misclassified.md) | 已修复 | 普通文本完成事件被误判为工具调用完成 |
| [002](002-generic-error-hides-cause.md) | 已修复 | 非 AppError 被统一提示隐藏真实原因 |
| [003](003-tool-result-duplicated-in-tui.md) | 待修复 | 工具结果在 TUI 中可能重复展示 |

## 本日结论

工具系统已经能够执行并回灌结果，但 Provider 事件分类、Conversation 提交边界和 TUI 渲染边界需要严格区分。工具调用相关状态应保证：

1. 只把真正的 function call 事件转换成工具事件。
2. 同一 Turn 只能提交一次。
3. 已进入历史的 Turn 不再作为 active turn 重复渲染。
4. 内部错误应保留可诊断信息，同时避免泄露密钥。
