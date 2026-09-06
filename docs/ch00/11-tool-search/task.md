# 原生 Tool Search 与 MCP 工具延迟加载 Tasks

## 文件清单

| 操作 | 位置 | 职责 |
|---|---|---|
| 修改 | `internal/config/*` | 模式配置、默认值、校验。 |
| 修改 | `internal/tools/*`、`internal/mcp/tool.go` | MCP 来源与分组元数据。 |
| 修改 | `internal/provider/*` | 中立定义、能力判定、namespace 调用信息。 |
| 修改 | `internal/provider/anthropic/*` | Anthropic Tool Search 请求。 |
| 修改 | `internal/conversation/*` | 服务端搜索历史的校验、持久化和恢复。 |
| 修改 | `internal/provider/openai/*` | OpenAI namespace、Tool Search 与流式解析。 |
| 修改 | `internal/agent/*` | 请求策略、提示词、执行映射。 |
| 修改 | `.mewcode/config.example.yaml`、`README.md` | 配置与用户说明。 |
| 新建/修改 | 相应 `*_test.go` | 离线单元、请求体与端到端测试。 |

## T1：配置、能力判定与中立模型

1. 定义 `auto`、`enabled`、`disabled` 及默认值。
2. 实现 Provider/模型/端点能力解析器和 OpenAI 白名单快照归一化。
3. 扩展中立工具、调用定义以携带 MCP 来源和 namespace。
4. 测试所有模式、明确支持、不支持、未知与兼容端点。

**验证：** 相关 config/provider 单元测试通过。

## T2：MCP 来源传播与 namespace 规划

1. 从 `RemoteToolAdapter` 至 Registry Definitions 传播 Server 和远端名称。
2. 实现稳定分组、类别分块、最大 10 成员限制和双向映射。
3. 验证工具名冲突、同名远端工具、空描述和大目录。

**验证：** `go test ./internal/mcp ./internal/tools ./internal/provider -run 'Test.*(Namespace|ToolSearch|Definition)' -count=1`。

## T3：Provider 编码和响应解析

1. Anthropic 请求加入官方 Tool Search，并只 defer MCP 工具。
2. OpenAI 请求生成顶层内置函数、MCP namespaces 和 `execution: "server"` Tool Search。
3. 解析 OpenAI 最终 namespaced `function_call`，忽略搜索观测事件。
4. 覆盖精确 JSON 请求体与流事件序列。

**验证：** `go test ./internal/provider/... -count=1`。

## T4：Agent Loop、本地执行与提示词

1. 在构造请求前选择模式，并只在启用时插入 Tool Search 指令。
2. 将 OpenAI `namespace + name` 转回 Registry 唯一名。
3. 经受控 MCP Client 验证最终调用仍通过本地权限、校验和 RPC。
4. 验证回退路径与当前平铺工具调用完全兼容。

**验证：** `go test ./internal/agent ./internal/mcp -count=1`。

## T5：文档和回归

1. 更新 `.mewcode/config.example.yaml` 与 README。
2. 回填本章 Checklist 的实际命令和结果。
3. 执行格式化、目标包测试、全量测试、构建和 diff 检查。

**验证：** `go test ./...`、`go build ./cmd/mewcode`、`git diff --check`。

## T6：Anthropic 服务端搜索历史回传

**文件：** `internal/provider/message.go`、`internal/provider/event.go`、`internal/provider/anthropic/{stream,request}.go`、`internal/agent/collector.go`、`internal/conversation/*` 及相应测试。

**依赖：** T3、T4。

**步骤：**

1. 为 Provider 服务端历史块定义中立内容和流事件，并保证克隆不共享原始 JSON。
2. 解析 Anthropic `server_tool_use` 与 `tool_search_tool_result`，保留完整载荷但不产生本地工具调用。
3. 扩展 Agent 收集、轮次校验、会话日志/恢复及 Anthropic 请求编码，使两种块在下一请求中按原顺序回传。
4. 添加受控 SSE + Agent Loop 测试，覆盖服务端搜索、普通 `tool_use`、本地 MCP 结果和后续请求；断言没有对 `srvtoolu_...` 执行本地调度。
5. 回填 Bug 记录和本章 Checklist。

**验证：** `go test ./internal/provider/anthropic ./internal/agent ./internal/conversation -count=1`，随后运行 `go test ./...`。
