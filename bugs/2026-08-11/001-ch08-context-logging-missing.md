# 第八章上下文管理流程未接入应用日志

## 状态

已修复。

## 影响范围

第八章新增的上下文管理流程，包括工具结果持久化、自动/手动/强制/紧急压缩、自动摘要熔断与紧急重试。

## 用户可见现象

这些流程只通过 Agent/TUI 事件可见，没有写入 `logs/*.jsonl`。长期任务遇到压缩失败、熔断或紧急重试时，缺少可回溯的文件日志。

## 复现条件

运行第八章相关 Agent 流程并检查日志文件，只能看到 main/env/MCP 等既有日志，缺少上下文压缩和持久化阶段记录。

## 根因

第八章实现时 `agent.Options` 没有接收 `*logging.Logger`，`Runner` 和 `internal/context` 也没有使用既有日志模块记录关键状态。

## 修复方案

- 将既有 logger 从 `cmd/mewcode` 注入 `agent.Runner`。
- 在上下文压缩、工具结果持久化、熔断和紧急重试处写入安全字段日志。
- 更新 `AGENTS.md`，要求后续章节开发必须接入必要日志。

## 验证方式

已新增失败测试：

- `go test ./internal/agent -run 'LogsContext|LogsToolResult'`
- `go test ./cmd/mewcode -run 'InjectsLogger'`

当前状态：红测确认第八章 Runner 缺少 logger 注入与上下文日志写入。

修复后目标验证：

- `go test ./internal/agent -run 'LogsContext|LogsToolResult'` 通过。
- `go test ./cmd/mewcode -run 'InjectsLogger'` 通过。

最终回归验证：

- `go test ./internal/agent ./cmd/mewcode ./internal/logging` 通过。
- `go test ./...` 通过。
