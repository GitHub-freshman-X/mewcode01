# Anthropic Tool Search 服务端历史块被丢弃

- 状态：待处理
- 发现日期：2026-09-06
- 影响范围：使用官方 Anthropic API、支持 Tool Search 的模型，并在同一任务中执行发现后的本地 MCP 工具调用。

## 现象

Anthropic Tool Search 的流会包含 `server_tool_use` 与 `tool_search_tool_result` 块。当前 MewCode 不会因此报流解析错误，但在本地 MCP 工具执行后发起下一轮 Messages 请求时，无法把这两个块原样放回 assistant 历史。

## 根因

`internal/provider/anthropic/stream.go` 仅把 `content_block.type == "tool_use"` 转为中立工具调用事件；`server_tool_use` 和 `tool_search_tool_result` 均返回“无事件”。同时，`provider.ContentBlock` 只支持文本、思考、本地工具调用和工具结果，不能表示 Anthropic 专有的服务端搜索块；`internal/provider/anthropic/request.go` 也没有对应的回传编码。

Anthropic 官方要求在后续请求中原样回传上述块，并且不能为 `srvtoolu_...` 生成本地 `tool_result`。因此此问题不能通过把服务端搜索伪装成本地工具调用解决。

## 修复方向

扩展中立流事件、会话内容块和 Anthropic 请求编解码，以无本地执行语义的方式保存并回传 `server_tool_use`、`tool_search_tool_result`（及其原始结构化内容）。在受控端到端测试中验证：服务端搜索、发现后的本地 MCP 调用、带本地工具结果的后续请求三者完整衔接。

## 验证进展

2026-09-06：依据 Anthropic 官方 Tool Search 流式样例构造两个 `content_block_start` 事件；临时测试 `TestDiagnosisToolSearchServerBlocksArePreserved` 运行 `go test ./internal/provider/anthropic -run TestDiagnosisToolSearchServerBlocksArePreserved -count=1` 稳定失败，错误为“Anthropic Tool Search block was dropped instead of being preserved for the next request”。临时测试已移除，尚未实施修复。
