# OpenAI Hosted Tool Search 流事件参数无法解析

- 状态：已修复
- 发现日期：2026-09-06
- 影响范围：使用官方 OpenAI Responses API 且启用 `tool_search: auto` 或 `enabled` 的支持模型；当模型执行 hosted Tool Search 时请求失败。

## 现象

使用官方 OpenAI API、`gpt-5.4` 和 Tool Search 发起对话后，界面显示：

```text
错误: stream: invalid OpenAI stream event
```

## 根因

`internal/provider/openai/stream.go` 将 SSE 事件中的 `item.arguments` 和 `output.arguments` 固定声明为字符串。OpenAI hosted Tool Search 的 `tool_search_call` 则使用结构化 JSON 参数，例如 `{"paths":["mcp_demo"]}`。解码该事件时，`encoding/json` 返回类型不匹配错误，外层将其包装为 `invalid OpenAI stream event`。

普通 `function_call.arguments` 仍是 JSON 字符串，因此未启用 Tool Search 的原有工具调用流程不受此问题影响。

## 修复方向

流事件的工具参数须按输出项类型区分：保留 `function_call` 的字符串参数，同时容忍并忽略 hosted Tool Search 的结构化参数与其输出项。解析未知事件时不得因其无关字段的类型变化中断整个流。

## 验证进展

2026-09-06：依据 OpenAI 官方 Tool Search 文档中的 hosted 输出样本，构造包含 `tool_search_call` 和对象型 `arguments` 的 `response.output_item.done` SSE 帧；修复前 `parseEvent` 可稳定返回 `invalid OpenAI stream event`。

2026-09-06：流解析器改为以原始 JSON 承载 `arguments`，仅在 `function_call` 分支提取字符串参数；含对象型 `arguments` 的 `tool_search_call` 和 `tool_search_output` 现在均被忽略。`go test ./internal/provider/openai -count=1`、`go test ./internal/provider/... -count=1`、`go test ./...` 及 `go build -o /private/tmp/mewcode-openai-tool-search-verify ./cmd/mewcode` 均通过；临时构建产物已删除。

2026-09-06：使用官方 `https://api.openai.com/v1` 和 `gpt-5.4-mini` 完成真实 MCP 调用验证。日志显示请求工具数为 11（9 个内置工具、1 个 MCP namespace、1 个服务端 Tool Search），随后本地 `demo__lookup_fixture` 调用开始并成功，工具结果已被提交到后续模型请求；未再出现流解析错误。
