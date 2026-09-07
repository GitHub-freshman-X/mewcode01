# `mcp_call` 不接受 MCP 远端工具名，模型分发调用失败

**状态：** 已修复

## 现象

`tool_search` 成功返回 `demo__lookup_fixture` 的 schema 后，模型调用 `mcp_call(server: "demo", tool: "lookup_fixture")` 返回“未找到工具”。

## 根因

本地目录的 `Resolve` 只将 `mcp_call.tool` 与 Registry 唯一名 `demo__lookup_fixture` 比较；模型则传入了 MCP Server 的远端原始工具名 `lookup_fixture`。两种名称均可从搜索结果的工具定义推导，却只有前者被接受。

## 影响

本地 ToolSearch 路径的模型可能无法完成已发现工具的实际调用，尤其是模型按 MCP 习惯使用远端工具名时。

失败结果会正常作为下一轮模型输入回传；最新人工运行日志在搜索、分发失败和最终回答之间记录了连续三次 Provider 请求，证明 Agent Loop 未在工具错误处终止。模型读到该错误后选择结束任务，因此用户会看到错误成为最终回答。

## 建议修复

已在已验证 `server` 的前提下，让 `mcp_call` 同时接受唯一名和该 server 的 `RemoteName`，并统一解析到唯一 Registry 名。跨 server、内置工具和未注册名称仍被拒绝。

## 验证方式

新增回归测试 `TestCatalogResolveAcceptsLocalAndRemoteNames`：同一 catalog 中，`server: demo` 搭配 `tool: demo__lookup_fixture` 与 `tool: lookup_fixture` 都解析到 `demo__lookup_fixture`；错误 server 仍被拒绝。

2026-09-07 真实验证：最新 OpenAI 与 Anthropic 会话都以 `server: demo`、`tool: lookup_fixture` 调用成功，并由 `demo__lookup_fixture` 返回固定标记。
