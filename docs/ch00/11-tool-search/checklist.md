# 原生 Tool Search 与 MCP 工具延迟加载 Checklist

> 实现完成后回填实际命令与结果。

## 配置与回退

- [ ] `auto` 在 Anthropic/OpenAI 明确支持组合启用；未知模型与兼容端点回退全量定义。
- [ ] `disabled` 始终发送当前平铺完整工具定义。
- [ ] `enabled` 在不支持组合于本地返回可理解配置错误，不请求 Provider。
- [ ] OpenAI 白名单、快照归一化和明确不支持型号均有单元测试。

## Anthropic

- [ ] 请求包含官方 Tool Search，内置工具没有 `defer_loading`，MCP 工具均为 deferred。
- [ ] 最终 `tool_use` 仍以 Registry 唯一名经本地 MCP Client 成功执行。
- [ ] `server_tool_use` 和 `tool_search_tool_result` 被保留在 assistant 历史，并按原顺序编码到执行本地 MCP 工具后的下一次 Messages 请求。
- [ ] Anthropic 服务端搜索块不触发本地执行、权限确认或 `tool_result`；仅最终普通 `tool_use` 进入本地 MCP Client。
- [ ] 保存、恢复和克隆会话后，Anthropic 服务端搜索历史仍可无损回传。

## OpenAI

- [ ] 支持模型的 Responses 请求包含顶层内置 functions、MCP namespaces 与服务端 Tool Search。
- [ ] namespace 初始描述不泄露成员函数目录；成员 schema 带 `defer_loading: true`。
- [ ] 每个 namespace 至多 10 个成员，分组和排序稳定。
- [ ] 最终 `namespace + name` 映射到唯一 `<server>__<tool>` 并本地执行。
- [ ] `tool_search_call` 与 `tool_search_output` 不会被当作本地工具调用或历史结果。

## 回归、安全与文档

- [ ] 内置工具、权限确认、输入校验、MCP 调用及历史回放回归通过。
- [ ] 日志不含 schema 正文、工具参数、结果、凭据或请求头。
- [ ] `.mewcode/config.example.yaml`、README 与本章文档描述一致。
- [ ] 真实 Anthropic / OpenAI 的支持与自动回退场景按 [人工测试方案](manual_scenarios.md) 执行并记录。
- [ ] `go test ./...`、`go build ./cmd/mewcode`、`git diff --check` 通过。

## 本次执行记录（2026-09-06）

- [x] `auto`、`enabled`、`disabled`、OpenAI 支持/不支持型号、官方端点与兼容网关回退，已由 `go test ./internal/provider -run TestResolveToolSearch -count=1` 覆盖。
- [x] Anthropic 请求仅将 MCP 工具标记为 deferred，并加入官方 Tool Search，已由 `go test ./internal/provider/anthropic -run TestBuildRequestDefersOnlyMCPTools -count=1` 覆盖。
- [x] OpenAI 请求将 MCP 工具稳定分组为最多 10 个成员的 namespace，并加入服务端 Tool Search；回退请求保持平铺 function，已由 `go test ./internal/provider/openai -run 'TestBuildRequest(UsesNamespacesForDeferredMCPTools|FallbackKeepsFlatTools)' -count=1` 覆盖。
- [x] OpenAI hosted `tool_search_call` 的结构化 `arguments` 被流解析器安全忽略，最终 `function_call` 的字符串参数保持可用；由 `TestHostedToolSearchEventsAreIgnored` 覆盖。
- [x] 已执行 `go test ./...`、`go build -o /private/tmp/mewcode-tool-search-verify ./cmd/mewcode` 和 `git diff --check`，均通过。
- [x] 2026-09-06 修复 OpenAI hosted Tool Search 流解析后，已执行 `go test ./internal/provider/openai -count=1`、`go test ./internal/provider/... -count=1`、`go test ./...` 和 `go build -o /private/tmp/mewcode-openai-tool-search-verify ./cmd/mewcode`，均通过；临时构建产物已删除。
- [x] 已使用官方 OpenAI 账户完成 `gpt-5.4-mini` 支持路径和 `gpt-4.1` 自动回退路径的真实 MCP 调用；详情见 [人工测试方案](manual_scenarios.md) 的实际执行记录。Anthropic 场景与 `disabled` / `enabled` 模式检查仍未执行。
- [x] Anthropic 服务端搜索历史已由官方 SSE 样例覆盖：`server_tool_use` 在输入流结束后完整保存，`tool_search_tool_result` 原样回传，且两者不触发本地工具调度；`go test ./internal/provider/anthropic ./internal/agent ./internal/conversation -count=1` 通过。
- [x] Anthropic 服务端搜索历史修复后，已执行 `go test ./...`、`go build -o /private/tmp/mewcode-anthropic-tool-search-verify ./cmd/mewcode` 和 `git diff --check`，均通过；临时构建产物已删除。
