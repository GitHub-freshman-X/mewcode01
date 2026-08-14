# Ch09 记忆提取过频并产生重复偏好

- 状态：已修复，待真实 API 验证
- 发现日期：2026-08-14
- 影响范围：`internal/agent/runner.go`、`internal/memory/service.go`

## 现象

一次明确表达“Go 中偏好使用 any”的用户偏好，在用户级 `memory/` 目录中被写成多个语义等价的笔记，例如 `go-any-preference.md`、`prefer-go-any.md`、`go-use-any.md`、`prefers-any.md`。

## 根因

`Runner.startMemoryTasks` 会在每次正常最终回复后无条件调用 `Memory.Extract`，不区分该轮是否包含持久记忆候选。提取请求没有携带既有记忆索引或笔记内容，模型无法选择更新既有条目，因此会反复生成不同名称的 `create` 操作。随后还会无条件尝试治理，使一次普通对话可能带来额外 API 请求。

## 建议修复

在调用模型前以保守规则筛选明确的长期记忆候选，对相同内容做会话级去重；提取时提供现有索引并要求语义等价时更新既有条目；仅在实际写入后再尝试治理。

## 修复与验证

已实现明确长期记忆信号筛选、会话生命周期内的相同输入去重，并在提取请求中提供用户和项目索引。`go test ./internal/memory ./internal/agent -count=1` 已通过。
