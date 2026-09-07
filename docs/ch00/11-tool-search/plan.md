# 原生与本地 Tool Search 的 MCP 工具加载 Plan

## 架构概览

保留现有 `Registry → Provider request → Agent Scheduler → 本地 MCP Client` 主链路。在请求构造前，将 Tool Search 策略解析为三种呈现方式：全量、原生和本地。原生策略沿用现有 Provider 编码；全量策略沿用现有平铺定义；本地策略由一个只读 MCP 目录和两个虚拟工具构成，MCP 工具本身不进入 Provider 工具列表。

```text
Registry（内置工具 + RemoteToolAdapter）
                 │
        ToolSearch 策略与目录快照
        ├─ Full：全部定义
        ├─ Native：全部定义 + Provider defer/search
        └─ Local：内置定义 + tool_search + mcp_call
                                      │
                        tool_search → schema 工具结果
                        mcp_call → 目标唯一工具名
                                      │
                     Scheduler 权限/校验 → MCP Client
```

## 核心数据结构

### ToolSearchStrategy

在 Provider 中立层取代现有单一 `Enabled` 标志，表示 `Full`、`NativeAnthropic`、`NativeOpenAI` 或 `Local`。能力解析器根据配置、协议、官方端点和模型白名单确定策略：`disabled` 返回 Full；支持的官方组合返回各自 Native；其他组合返回 Local。

### LocalCatalog

以一次会话中 Registry 的 MCP `ToolDefinition` 为输入，保存按唯一名排序的不可变条目。它提供本地路径 Provider 可见定义、稳定名称目录和按完整唯一名加载 schema 的能力；不承担关键词、自然语言或语义检索。

### LocalToolCall

定义两个虚拟工具的输入：

- `tool_search`：`tool_name` 字符串，必须与名称目录的一项完全一致。
- `mcp_call`：`server`、`tool` 与对象形式的 `arguments`。

`mcp_call` 的 `server + tool` 必须解析到同一 Registry 中的 MCP adapter，且不接受内置工具。

## 模块设计

### `internal/provider`

**职责：** 解析策略并保留 Provider 原生能力判断。

**改动：** 将二元配置升级为策略；更新测试覆盖官方支持、非原生、禁用和未知配置。Anthropic/OpenAI 仅在各自 Native 策略下发送官方字段；Local 和 Full 的请求编码均不带原生字段。

### `internal/toolsearch`

**职责：** 构建本地 MCP 工具目录和 Provider 可见虚拟工具定义；处理按名称的 schema 加载与 `mcp_call` 目标解析。

**接口：** 输入 `[]provider.ToolDefinition`，输出可见 definitions、稳定目录文本、工具结果及目标 MCP 唯一名。该包不执行工具、不访问 Provider、不记录 schema 或查询正文。

### `internal/agent`

**职责：** 在每轮构造请求时使用稳定的策略与目录；将本地虚拟调用纳入现有 Agent Loop。

**改动：** Local 策略添加目录提示；请求仅使用 LocalCatalog 的可见 definitions。Scheduler 截获 `tool_search` 并生成普通工具结果；截获 `mcp_call`，先解析并校验目标，再以原调用 ID 经目标工具的现有权限、Hook、输入校验和 Executor 执行。原生与全量路径不使用这些截获逻辑。

### `internal/prompt`、配置与用户文档

**职责：** 提示模型先搜索再分发，并准确公开策略行为。

**改动：** 本地路径稳定系统提示列出 MCP 工具名，并强制模型先选目录名称、再调用 `tool_search`；README、示例配置、人工测试方案与 checklist 改写回退场景为本地精确加载，保留 `disabled` 全量场景。

## 模块交互

1. MCP 连接完成后，Registry 仍保留所有 RemoteToolAdapter。
2. Agent 创建策略和 LocalCatalog；同一会话内策略与目录顺序固定。
3. Native 请求编码完整 MCP schema；Local 请求编码内置工具、`tool_search` 和 `mcp_call`。
4. 模型调用 `tool_search` 后，Scheduler 将命中的 schema 序列化为该调用的普通结果；下一次请求通过既有 history 提交该结果。
5. 模型调用 `mcp_call` 后，Scheduler 将其解析到唯一 MCP 工具，并对该目标执行既有权限决策和 MCP RPC；结果仍以原 `mcp_call` 调用 ID 回传。

## 文件组织

| 操作 | 文件 |
|---|---|
| 修改 | `internal/provider/{provider,tool_search}.go` 与测试 |
| 新建 | `internal/toolsearch/catalog.go` 与测试 |
| 修改 | `internal/agent/{runner,scheduler}.go` 与测试 |
| 修改 | `internal/prompt/tools.go` 与测试 |
| 修改 | `internal/provider/{anthropic,openai}/request.go` 与测试 |
| 修改 | `.mewcode/config.example.yaml`、`README.md`、`docs/ch00/11-tool-search/{task,checklist,manual_scenarios}.md` |

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 非原生策略 | 本地 ToolSearch | 避免发送不可控的 MCP schema，同时不依赖兼容网关支持 beta 字段。 |
| schema 回传方式 | 普通工具结果 | 不修改会话中 Provider 的 `tools[]`，适配普通工具调用协议。 |
| 最终调用入口 | 常驻 `mcp_call` | 已返回的 schema 本身不可直接调用，且可保持工具列表稳定。 |
| 权限检查对象 | 被分发的真实 MCP 工具 | 防止把 `mcp_call` 误当成无风险代理而绕过目标权限。 |
| 加载算法 | 名称目录的精确查找 | 避免模型自然语言查询与 schema 描述语言不一致导致的歧义。 |
| `enabled` 语义 | 与 `auto` 相同的原生优先、本地兜底 | 用户要求所有非原生组合不再回退全量。 |
