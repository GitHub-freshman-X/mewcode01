# 会话切换后 Token 用量未恢复

- 状态：已修复，自动化验证通过，待真实 API 验证
- 发现日期：2026-08-16
- 影响范围：`internal/tui` 状态栏与 `/status`、`internal/conversation` 会话持久化、`internal/agent` 用量记账

## 现象

`/status` 和底部状态栏显示最近一次 Agent 任务的 Token，而不是当前会话累计值。执行 `/session resume <id>` 后，TUI 的最近任务状态被清空，用量显示归零；历史会话的 Provider 用量也没有写入 JSONL，因此无法恢复。

## 根因

用量只保存在 TUI 的 `current` 临时任务视图和 Agent 任务摘要中，`Session`、JSONL Journal 与恢复流程均未存储或聚合用量。上下文压缩管理器使用的输入估算与账单累计用量概念不同，不能直接以累计值作为恢复后的压缩判断。

## 修复方案

在既有 JSONL 中追加仅包含 `input_tokens`、`output_tokens` 与时间戳的 `usage` 增量记录；`Session` 先持久化后累加内存值，恢复时聚合这些行且不计入消息数、标题或对话历史。Runner 在普通、计划、执行和压缩 Provider 调用完成后记入当前 Session。TUI 的 `/status` 与状态栏统一读取 Session 用量。`/session resume` 保持零网络调用，下一条 Agent 消息仍按恢复历史的现有上下文估算决定压缩。

## 验证

`go test ./internal/conversation ./internal/agent ./internal/command ./internal/tui -count=1` 已通过，覆盖用量 JSONL 内容、旧历史隔离、恢复聚合、普通与压缩调用累计、TUI 状态读取，以及恢复延迟压缩。

待按真实 API 验证：在会话 A 完成多轮任务后切换到会话 B，再恢复 A；`/status` 与状态栏应恢复 A 的累计用量，且恢复动作本身不得触发模型调用。
