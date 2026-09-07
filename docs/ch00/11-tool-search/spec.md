# 原生与本地 Tool Search 的 MCP 工具加载 Spec

## 背景

当前 MCP 工具在 `auto` 模式下仅对明确支持的官方 Anthropic 与 OpenAI Provider 启用服务端 Tool Search；其他端点、未知模型与不支持模型会发送全部工具定义。这会让第三方兼容端点的初始上下文随 MCP 工具目录增长。

官方 Provider 的原生延迟加载不能由客户端替代：完整 MCP schema 必须仍发送给 Provider，并以 `defer_loading` 让服务端控制模型可见性。对不支持原生能力、但可调用普通工具的端点，需要在本地检索 MCP 工具定义，而不把 MCP schema 发送给模型。

## 目标

- 在官方端点与明确支持的模型组合上继续使用各 Provider 的原生 Tool Search。
- 在其他可用 Provider 组合上使用本地 Tool Search，初始请求不发送 MCP 工具的完整 schema。
- 让本地检索得到的 schema 仅作为模型下一轮可读的工具结果，而不在会话中动态修改 Provider 的工具列表。
- 保持真实 MCP 调用始终经过本地 Registry、权限确认、输入校验与 MCP Client。
- 保留显式关闭 Tool Search 后的全量工具加载，用作兼容和诊断开关。

## 功能需求

- **F1 配置与策略选择**：保留 `auto`、`enabled`、`disabled` 三种配置。`disabled` 始终发送当前平铺的完整工具定义。`auto` 与 `enabled` 均优先使用明确支持的官方原生 Tool Search；其余 Provider/模型组合选择本地 Tool Search，而不是全量加载。配置校验继续拒绝未知取值。
- **F2 原生路径**：对官方 Anthropic 与 OpenAI 的支持组合，完整 MCP schema 仍发送给 Provider；MCP 工具带相应的 `defer_loading` 标记，并使用 Provider 官方 Tool Search。内置工具保持立即可见，最终 MCP 调用仍由本地执行。
- **F3 本地工具目录与决策规则**：本地路径的 Provider 请求只包含内置工具、`tool_search` 与 `mcp_call`，不包含任一 MCP 工具 schema。稳定系统提示包含可检索 MCP 工具的完整、稳定名称清单，并明确要求模型先从该目录选择最合适的名称，再加载定义；不得以自然语言查询替代名称选择。
- **F4 本地精确加载**：`tool_search` 只接受目录中某个 MCP 工具的完整唯一名，并返回该工具的完整 schema。输入不再接受关键词、自然语言查询或 `select:` 前缀。名称不在目录中时返回可理解错误。检索结果作为该工具调用的普通结果进入下一轮模型输入，不加入或改写 Provider 的 `tools[]`。
- **F5 本地分发调用**：`mcp_call` 始终可见，接收目标 MCP Server、工具唯一名或该 Server 的原始工具名与工具参数。它只允许调用当前 Registry 中存在的 MCP 工具，并将请求转交现有权限门禁、输入校验和本地 MCP Client；不得绕过这些边界或执行内置工具。
- **F6 回合与错误行为**：本地检索无命中、精确工具不存在、目标并非 MCP 工具或 `mcp_call` 参数不合法时，向模型返回可理解的普通工具错误结果，并允许 Agent Loop 继续。Provider 请求中的工具布局在同一会话中保持不变。
- **F7 文档与可观测性**：README、配置示例、人工测试方案与本章文档应反映三种配置及两类搜索路径。日志只能记录策略、阶段、状态、工具计数、命中计数和耗时等安全元数据，不得记录 schema 正文、查询内容、参数、结果、密钥或请求头。

## 非功能需求

- **N1 上下文控制**：本地路径初始请求的 MCP schema 数量为零；仅工具名目录及两个常驻本地工具进入初始上下文。
- **N2 确定性**：相同 Registry、配置、Provider 与模型必须生成相同的工具目录；相同工具名必须加载相同的 schema。
- **N3 兼容性**：`disabled` 的工具请求与改动前的全量加载布局保持等价；官方原生路径的请求与既有原生 Tool Search 行为保持兼容。
- **N4 安全性**：本地 ToolSearch 是只读目录查询；所有真正的 MCP 调用继续复用既有安全控制。
- **N5 可测试性**：策略选择、本地工具布局、检索、无命中、分发、权限与最终本地 MCP 调用均由离线受控测试覆盖。

## 不做的事

- 不改变官方 Provider 原生 Tool Search 的请求格式、搜索算法或服务端历史处理。
- 不在本地路径中将搜索结果动态追加到 Provider `tools[]`。
- 不延迟内置工具，不增加跨会话的检索缓存，也不修改 MCP Server 发现生命周期。
- 不实现关键词、自然语言或语义检索，也不引入远程索引或对 MCP schema/description 的额外配置副本；本地加载只使用 Registry 中已有的唯一名称。
- 不把 MCP 连接、认证或真实执行交给 Provider。

## 验收标准

- **AC1**：官方 Anthropic/OpenAI 的支持组合保留完整 deferred MCP schema 和官方 Tool Search；最终 MCP 调用仍由本地 Client 处理。
- **AC2**：`auto` 或 `enabled` 的非原生组合初始请求不含 MCP schema，仅含内置工具、`tool_search` 与 `mcp_call`。
- **AC3**：本地 `tool_search` 仅可按目录中的完整唯一名返回对应工具 schema；关键词、自然语言和带 `select:` 前缀的输入均被明确拒绝。
- **AC4**：本地 ToolSearch 结果作为下一轮模型输入的工具结果存在，但 Provider 工具列表在会话内不因该结果改变。
- **AC5**：模型可通过 `mcp_call` 触发命中的 MCP 工具，且该调用经过现有权限、校验与本地 MCP Client。
- **AC6**：`disabled` 始终发送全量定义；未知配置值被本地拒绝。
- **AC7**：无命中、无效精确选择与无效分发均产生可理解错误，且不会执行未注册工具或绕过权限。
- **AC8**：配置示例、README、人工测试方案和自动化测试与实际行为一致；相关目标测试、全量测试、构建和 diff 检查通过。
