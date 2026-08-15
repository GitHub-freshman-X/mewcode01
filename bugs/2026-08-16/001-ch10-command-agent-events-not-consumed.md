# Slash Command 启动 Agent 后未订阅事件，`/plan <需求>` 与 `/do` 假死

- 状态：已修复，自动化验证通过，待真实 API 验证
- 发现日期：2026-08-16
- 影响范围：`internal/tui/update.go` 的命令回车分流

## 现象

真实 API 手工测试中，`/plan <需求>` 或已有计划后的 `/do` 会显示 `[PLAN]` 或 `[DEFAULT] · iteration 0`，随后界面不再更新。按 `Ctrl+C` 后才显示此前已产生的模型输出；若取消的是计划任务，计划不会提交，后续 `/do` 提示没有可执行计划。

## 最小复现

1. 启动连接真实 API 的 MewCode。
2. 输入 `/plan 用一句话制定测试计划`，等待流式事件。
3. TUI 停在 `iteration 0`；按 `Ctrl+C` 后才显示结果，终态为 `cancelled`。
4. 输入 `/do`，观察同一停滞现象或因计划被取消而报无计划。

## 根因

命令分支调用 `command.Dispatch` 后，无论 Handler 是否通过 `UIController.StartAgent` 创建了任务，`Model.Update` 都返回 `nil`。因此没有调用 `waitForAgent(m.task.Events)`，Bubble Tea 不会消费 Agent event channel。`Ctrl+C` 分支恰好调用了 `waitForAgent`，使已缓冲的事件在取消后才被渲染。

## 建议修复

命令分发成功后，若 `m.task != nil`，立即返回 `waitForAgent(m.task.Events)`；仅纯本地命令返回 `nil`。增加 TUI 回归测试，覆盖 `/plan <需求>` 与 `/do` 在不按 `Ctrl+C` 的情况下消费首个进度、文本和终态事件，以及计划完成后可被 `/do` 消费。

## 验证方式

- 使用替身 Provider 运行 `/plan <需求>`，断言 Update 返回的 Cmd 可接收 Agent 事件并进入完成态。
- 计划完成后运行 `/do`，断言请求携带 `ModeDo` 且不出现“no valid plan”。
- 按真实 API 人工方案重复 C2，整个流程无需按 `Ctrl+C`。

## 修复进展

2026-08-16：命令分发成功后，若 `Model` 已创建 Agent Task，TUI 立即返回 `waitForAgent(task.Events)`。新增定向回归测试覆盖 `/plan <需求>` 的事件消费、计划提交、随后 `/do` 的事件消费与计划清空；`go test ./internal/command ./internal/conversation ./internal/tui ./internal/agent ./internal/memory ./cmd/mewcode -count=1` 已通过，待按真实 API 场景验证。
