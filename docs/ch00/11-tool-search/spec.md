# 原生 Tool Search 与 MCP 工具延迟加载 Spec

## 背景

当前 Agent 在每次 Provider 请求中平铺发送 Registry 内所有工具的完整定义。MCP Server 的工具数量增长后，这会扩大模型初始上下文，并且任何工具目录变化都会改变工具前缀。Anthropic 与 OpenAI Responses API 均提供服务端 Tool Search：完整目录仍提交给 API 服务端，但只将需要的工具定义放入模型上下文。

本章为现有本地 MCP 执行模型接入两家原生 Tool Search。OpenAI 的托管 MCP 工具会由 OpenAI 连接和执行远端 Server，不符合本项目由本地 MCP Client 执行的边界；因此 OpenAI 路径使用 function namespace，而不使用 OpenAI `mcp` 工具类型。

官方参考：<https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool>、<https://developers.openai.com/api/docs/guides/tools-tool-search>。

## 目标

- 在支持模型上自动使用 Provider 服务端 Tool Search，延迟 MCP 工具的模型上下文加载。
- 保持 Registry、权限门禁与本地 MCP Client 的实际调用路径不变。
- 对不支持、未知模型或兼容网关无错误地回退到现有全量工具定义。
- 保持内置工具立即可见，并通过稳定前缀改善缓存复用条件。

## 功能需求

- **F1 配置模式**：新增 Tool Search 模式 `auto`、`enabled`、`disabled`，默认 `auto`。`disabled` 始终使用现有全量工具定义；`auto` 仅在明确支持的官方 Provider/模型组合启用；`enabled` 在不支持的组合上返回本地配置错误，不向 API 发送不兼容字段。

- **F2 能力判定与回退**：能力判定在请求构造前完成。OpenAI 仅对 Responses API 且白名单模型启用；模型未知、第三方 OpenAI 兼容端点或不支持模型均不携带 `tool_search`、`defer_loading` 或 namespace，直接发送平铺完整 function tools。模型快照按其所属稳定模型族判定。

- **F3 Anthropic 请求与服务端历史**：内置工具保持非延迟；MCP 工具完整定义带 `defer_loading: true`；请求加入 Anthropic 官方 Tool Search。服务端搜索后扩展 `tool_reference`，后续普通 `tool_use` 仍以现有唯一工具名进入本地执行。流中的 `server_tool_use` 与 `tool_search_tool_result` 必须作为服务端搜索历史无损保存；本地 MCP 工具执行后的下一次 Messages 请求必须原样回传这两个块及其相对顺序，且不得为 `srvtoolu_...` 生成本地 `tool_result` 或权限确认。

- **F4 OpenAI 请求**：内置工具保持顶层、完整且立即可调用。每个 MCP Server 的远端工具形成一个或多个 `namespace`，namespace 暴露稳定名称和简短能力描述，成员 function 均带 `defer_loading: true`，请求另带 `{"type":"tool_search","execution":"server"}`。不得把本地 MCP Server 直接编码为 OpenAI `mcp` 工具。

- **F5 OpenAI namespace 分块**：每个 namespace 目标不超过 10 个成员。优先用稳定的功能类别拆分；无法可靠分类时，按稳定排序分块。namespace 名必须可逆映射到 MCP Server 与分块，并避免和内置工具、其他 namespace 冲突。

- **F6 本地执行映射**：OpenAI 返回的 `function_call.namespace + name` 映射回 Registry 中唯一的 `<server>__<tool>` 名称，之后继续经过现有权限检查、输入校验、`RemoteToolAdapter` 与本地 MCP Client。不得将工具调用委托给 Provider。

- **F7 响应与会话兼容**：OpenAI 流式解析能忽略服务端 Tool Search 的观测事件，并保留最终 function call 的 namespace 信息。Anthropic 服务端搜索块必须在流解析、任务历史、会话持久化、克隆、上下文回放和请求编码中保持完整；它们无需展示为本地工具调用。已发现工具无需被误存为用户可执行调用。

- **F8 提示词行为**：在启用 Tool Search 时，稳定提示词包含“当前可见工具不足以完成任务时，使用 Tool Search 发现相关能力”的规则；不得要求模型猜测具体隐藏工具名。

## 支持范围

OpenAI `auto` 白名单为 `gpt-6-astra`、`gpt-5.6` / `gpt-5.6-sol` / `gpt-5.6-terra` / `gpt-5.6-luna`、`gpt-5.6-cyber`、`gpt-daybreak-red-latest`、`gpt-daybreak-blue-latest`、`gpt-5.5`、`gpt-5.4`、`gpt-5.4-pro`、`gpt-5.4-mini` 及对应快照。Daybreak 仍受 OpenAI 专项准入限制。

`gpt-5.5-pro`、`gpt-5.4-nano`、`chat-latest` 明确不启用；GPT-5.3 及更早模型、未知名称及第三方兼容网关一律按不支持处理。白名单必须集中维护并有单元测试，不能由字符串版本比较推导。

## 非功能需求

- **N1 缓存稳定性**：支持路径的新发现定义由 Provider 加入上下文末尾；内置工具、系统提示词和 namespace 目录保持确定排序。回退路径维持当前语义。
- **N2 安全性**：日志仅记录 Provider、模式、能力判定、namespace 数、工具数、阶段、状态和耗时；不得记录工具 schema 正文、工具调用参数、结果、密钥或 HTTP headers。
- **N3 确定性**：相同 Registry、配置和模型名必须产生相同的工具请求布局与映射。
- **N4 可测试性**：所有 Provider 请求体、回退、namespace 分块和最终本地 MCP 调用均由离线受控测试覆盖。Anthropic 测试必须以官方 SSE 样例覆盖“服务端搜索 → 本地工具调用 → 下一请求回传服务端搜索历史”的完整链路。

## 不做的事

- 不实现 OpenAI `execution: "client"` Tool Search，也不实现本地模糊检索。
- 不把 MCP URL、认证头或连接职责交给 OpenAI。
- 不延迟内置工具，不改变现有权限确认策略。
- 不通过“请求失败后重试”探测能力，也不对未知模型乐观发送 beta 字段。
- 不新增跨请求的工具搜索结果缓存或改变 MCP 工具发现生命周期。

## 验收标准

- **AC1**：Anthropic 支持模型的请求包含官方 Tool Search；MCP 工具为 deferred，内置工具非 deferred。
- **AC2**：OpenAI 支持模型的 Responses 请求包含顶层内置 function、服务端 `tool_search` 和按 MCP Server 分组的 deferred namespaces。
- **AC3**：OpenAI 最终 `namespace + name` 调用可映射到唯一的 `<server>__<tool>`，并由本地受控 MCP Client 收到调用。
- **AC4**：不支持、未知模型和兼容端点请求完全不含 Tool Search、namespace 与 `defer_loading`，且全量工具可照常调用。
- **AC5**：`disabled` 始终回退；`enabled` 遇不支持组合在本地明确失败。
- **AC6**：namespace 的名称、成员和分块结果在多次构造中稳定，且单组不超过 10 个成员。
- **AC7**：服务端 Tool Search 事件不会被当作本地工具调用或写入错误的历史块。
- **AC8**：现有 Provider、Registry、MCP、权限与 Agent Loop 回归测试通过；README 和配置示例反映新增配置。
- **AC9**：Anthropic Tool Search 的 `server_tool_use` 与 `tool_search_tool_result` 在后续请求中原样存在，且不被本地执行、权限确认或伪造的 `tool_result` 处理。
