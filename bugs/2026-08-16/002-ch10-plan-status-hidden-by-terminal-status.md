# `/plan` 切换后状态栏被上一任务终态覆盖

- 状态：已修复，自动化验证通过，待真实 API 验证
- 发现日期：2026-08-16
- 影响范围：`internal/tui/view.go` 的 `statusText`

## 现象

一次普通对话完成后，输入无参数 `/plan` 已进入计划模式，但状态栏仍显示上一任务的 `completed · final_answer ...`，而不是 `[PLAN]`。

## 根因

`statusText` 虽然先计算了 `mode`，但 `m.current.terminal != nil` 的分支优先返回历史任务终态字符串，且该字符串没有包含 `mode`。`/plan` 的本地切换不会清除 `m.current.terminal`。

## 建议修复

让所有状态栏分支包含当前模式，或在本地模式切换后将旧任务终态降级为历史显示、状态栏回到当前模式与空闲状态。增加“任务完成后切换 `/plan`”的视图回归测试。

## 验证方式

完成一次普通任务后输入 `/plan`，立即观察状态栏为 `[PLAN]`；再次输入 `/plan` 后为 `[DEFAULT]`，无需发起模型请求。

## 修复进展

2026-08-16：任务终态与失败状态栏均加入当前模式前缀。新增定向视图测试；`go test ./internal/command ./internal/conversation ./internal/tui ./internal/agent ./internal/memory ./cmd/mewcode -count=1` 已通过，待按真实 API 场景验证。
