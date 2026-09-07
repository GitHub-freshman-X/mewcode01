# 本地 ToolSearch 同轮多调用只回传首个工具结果

**状态：** 已修复

## 现象

用户以自然语言请求通过 Context7 查询 React 文档时，OpenAI Provider 返回：

```
request failed: tool call and result counts do not match
```

最近会话 `20260907-094409-26bb` 的日志确认 Context7 已成功发现并注册两个工具；首次 Provider 请求后即失败，没有后续工具结果回合。

## 根因

本地 `Scheduler.Execute` 在遍历同一模型回合的工具调用时，遇到第一个 `tool_search` 就直接返回一个结果。模型若在该回合并行发出多个 `tool_search` 或同时发出 `tool_search` 与 `mcp_call`，其余 call ID 没有对应结果。OpenAI Responses API 会拒绝这类不完整的函数调用输出。

## 修复

本地虚拟工具现在逐个生成结果，普通 MCP 调用继续进入现有权限、校验和执行路径；最终结果按原始调用顺序合并，确保每一个模型工具调用都有同 ID 的工具结果。虚拟工具结果也会发出正常的工具调用与结果事件。

## 验证

- 新增 `TestSchedulerLocalToolSearchReturnsAResultForEveryCall`，以同一回合的 `tool_search` 与 `mcp_call` 调用复现旧问题；修复前只得到 `tool_search` 的一条结果，修复后得到两条、call ID 顺序一致，并确认 MCP 调用执行一次。
- 已运行 `go test ./internal/agent -run '^(TestSchedulerLocalToolSearchReturnsAResultForEveryCall|TestSchedulerReadOnlyConcurrentAndResultOrder|TestSchedulerPermissionMultiToolOrder)$' -count=1`，通过。

## 人工复验

2026-09-07 使用同一句自然语言请求“使用 MCP 查询 React 官方文档中 useEffect 的 cleanup 行为”分别完成了 OpenAI 与 Anthropic 两轮会话：

- OpenAI 会话 `20260907-095135-e596`：依次精确加载 `context7__query-docs`、`context7__resolve-library-id`，再通过 `mcp_call` 成功调用两个 Context7 工具，并输出最终总结。
- Anthropic 会话 `20260907-095223-638b`：依次精确加载 `context7__resolve-library-id`、调用它、精确加载 `context7__query-docs`、调用它，并输出最终总结。

两份会话中每个 `tool_use_id` 都有一份 `is_error: false` 的同 ID 结果，未再出现 OpenAI 的工具调用/结果计数不一致错误。此次人工路径是顺序单调用；并行同轮调用的防回归由上述自动化测试覆盖。
