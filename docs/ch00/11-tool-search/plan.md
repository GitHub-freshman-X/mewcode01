# 原生 Tool Search 与 MCP 工具延迟加载 Plan

## 架构

在 `tools.Metadata → Registry → provider.ToolDefinition → Provider request/response` 的现有链路中引入“工具来源与呈现分组”元数据。Registry 继续以 `<server>__<tool>` 保存和执行工具；Provider 仅在序列化阶段将同一 MCP Server 的定义组织成对应厂商的延迟加载结构。

```text
MCP tools/list → RemoteToolAdapter(server__tool) → Registry
                                              │
                                      ToolDefinition（来源/分组）
                                      ├─ Anthropic：单工具 deferred
                                      └─ OpenAI：namespace → deferred functions
                                                            │
Responses function_call(namespace, name) ──映射──> server__tool
                                                            │
                                               Registry / permission / MCP Client
```

## 核心数据与职责

### 工具呈现元数据

扩展 Provider 中立的工具定义，使其表达本地唯一名称、来源类型、MCP Server、远端原始名称和可供模型使用的 namespace 描述。Registry 由 `RemoteToolAdapter` 的 metadata 传播这些信息；内置工具保持本地来源。

### Tool Search 能力解析器

新增纯函数能力解析器，输入 Provider、端点性质、模型名和模式，输出：禁用、启用 Anthropic、启用 OpenAI，或本地配置错误。OpenAI 模型白名单与快照归一化在此集中维护。

### OpenAI namespace 规划器

输入已排序的 MCP 工具定义，输出确定的 namespace 及成员。首先按显式/可识别类别归类，再按固定大小分块；为每个成员保存 `namespace + remote name → local name` 映射。内置工具不进入 namespace。

### Provider 编码与解码

- Anthropic 编码器增加 Tool Search 工具及 deferred 标志。
- OpenAI 编码器支持顶层 function、namespace、Tool Search 三类对象；解码器读取 `function_call.namespace`。
- 流事件层可观测但不执行 `tool_search_call` / `tool_search_output`。

### 配置与提示词

配置默认 `auto`，示例文件列出三种模式。仅在实际启用时，稳定提示词添加使用 Tool Search 的行为规则；回退请求不带该规则，以维持现有模型行为。

## 模块改动

| 模块 | 改动 |
|---|---|
| `internal/config` | Tool Search 模式解析、默认值和校验。 |
| `internal/tools`、`internal/mcp` | 保留远端工具来源与 Server 分组信息。 |
| `internal/provider` | 扩展工具定义和工具调用的 namespace 字段；增加纯能力解析/namespace 规划接口。 |
| `internal/provider/anthropic` | 编码官方 Tool Search 与 deferred 工具。 |
| `internal/provider/openai` | 编码 namespace、服务端 Tool Search，解析最终 namespaced call。 |
| `internal/agent` | 在构造请求前解析模式、选择提示词规则，并按 namespace 映射执行。 |
| `.mewcode/config.example.yaml`、`README.md` | 更新配置和兼容性说明。 |

## 执行顺序

1. 定义中立数据结构、配置模式和能力解析器，并为回退语义建测试。
2. 传播 MCP 来源信息，实施 namespace 规划与双向映射。
3. 扩展 Anthropic/OpenAI 请求与流式响应编解码。
4. 接入 Agent Loop、提示词与执行映射，补充端到端受控 MCP 测试。
5. 更新 README、示例配置和本章 Checklist，执行全量验证。

## 技术决策

| 决策 | 选择 | 原因 |
|---|---|---|
| OpenAI 搜索执行方 | `execution: "server"` | 使用官方服务端搜索，不要求本地模糊匹配。 |
| OpenAI MCP 呈现 | namespace | 保留本地 MCP Client 执行边界。 |
| 默认策略 | `auto` | 支持模型获得优化，未知组合零风险回退。 |
| 内置工具 | 始终立即加载 | 高频、数量小且避免额外搜索回合。 |
| 模型兼容 | 明确白名单 | beta 能力和兼容网关差异不能安全推断。 |
| 分组上限 | 10 | 遵循 OpenAI 官方的 namespace 建议。 |
