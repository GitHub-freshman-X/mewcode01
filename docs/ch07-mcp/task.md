# MewCode MCP Client Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `internal/config/config.go` | MCP Server 配置类型和主配置字段 |
| 修改 | `internal/config/load.go` | 主配置 `mcp_servers` 读取 |
| 修改 | `internal/config/validate.go` | MCP Server 声明校验 |
| 修改 | `internal/config/config_test.go` | 主配置兼容与校验测试 |
| 新建 | `internal/mcp/config.go` | 环境变量展开与诊断 |
| 新建 | `internal/mcp/config_test.go` | 配置变量展开测试 |
| 新建 | `internal/mcp/protocol.go` | JSON-RPC 类型、编码、解码与错误 |
| 新建 | `internal/mcp/protocol_test.go` | 协议消息合法性测试 |
| 新建 | `internal/mcp/transport.go` | 统一传输接口与入站事件 |
| 新建 | `internal/mcp/session.go` | ID、pending 路由、通知与关闭 |
| 新建 | `internal/mcp/session_test.go` | 乱序响应、错误、取消与关闭测试 |
| 新建 | `internal/mcp/stdio.go` | stdio 子进程 JSON Lines 传输 |
| 新建 | `internal/mcp/stdio_test.go` | stdio 协议通道与退出测试 |
| 新建 | `internal/mcp/http.go` | Streamable HTTP、header、会话与 DELETE |
| 新建 | `internal/mcp/sse.go` | POST 响应 SSE 解码 |
| 新建 | `internal/mcp/http_test.go` | JSON/SSE、header、404、关闭测试 |
| 新建 | `internal/mcp/client.go` | 初始化、发现与工具调用 |
| 新建 | `internal/mcp/client_test.go` | 三步会话与调用结果测试 |
| 新建 | `internal/mcp/tool.go` | 远端工具对现有 Tool 的适配 |
| 新建 | `internal/mcp/tool_test.go` | Schema、安全分类、结果映射测试 |
| 新建 | `internal/mcp/manager.go` | 多 Server 连接缓存、注册、诊断与关闭 |
| 新建 | `internal/mcp/manager_test.go` | 隔离、冲突、缓存和生命周期测试 |
| 新建 | `internal/testutil/mcpserver.go` | 受控 MCP stdio/HTTP 测试 Server |
| 修改 | `cmd/mewcode/main.go` | 启动发现与退出关闭 |
| 修改 | `cmd/mewcode/main_test.go` | 启动集成与安全诊断测试 |
| 修改 | `config.example.yaml` | MCP 配置示例 |
| 修改 | `docs/README.md` | 第七章索引 |

## T1: 扩展主配置 MCP 配置类型

**文件：** `internal/config/config.go`、`internal/config/load.go`、`internal/config/validate.go`、`internal/config/config_test.go`

**依赖：** 无

**步骤：**

1. 定义 `MCPTransportType`、`MCPServerConfig` 与 `Config.MCPServers`。
2. 在主配置 YAML loader 中读取 `mcp_servers` map，并保持 `WithKnownFields` 严格校验。
3. 校验 Server 名、传输类型、stdio 的 command 及 HTTP 的绝对 URL；拒绝不属于该传输的必填项缺失。
4. 保持没有 `mcp_servers` 的旧配置可加载，且 Provider 配置规则不改变。
5. 添加主配置 stdio、HTTP、非法类型、缺失字段和旧配置兼容测试。

**验证：** `go test ./internal/config` 通过。

## T2: 展开主配置中的 MCP 变量

**文件：** `internal/mcp/config.go`、`internal/mcp/config_test.go`

**依赖：** T1

**步骤：**

1. 仅接收主配置加载器已解析的 `mcp_servers`；不读取 `<root>/.mewcode/config.yaml` 或其他项目级 MCP 配置文件。
2. 对 command、args、env、url、headers 的字符串值展开 `${VAR}`，保留 map/数组结构。
3. 对缺失变量生成带 Server/阶段的安全诊断；不得回显展开值。
4. 添加测试覆盖多 Server、未命中变量和敏感 header 脱敏。

**验证：** `go test ./internal/mcp -run 'Config|Merge|Expand'` 通过。

## T3: 定义 JSON-RPC 协议模型

**文件：** `internal/mcp/protocol.go`、`internal/mcp/protocol_test.go`

**依赖：** 无

**步骤：**

1. 定义 JSON-RPC 2.0 request、response、error、notification 和 ID 类型。
2. 实现 request/notification 编码，确保 `jsonrpc` 固定为 `2.0`。
3. 实现 response 解码，区分 result、error、无效 JSON 和不支持的消息形态。
4. 定义可安全呈现的协议错误，避免把原始 headers、环境或全部 payload 带入结果。
5. 添加测试覆盖正常结果、JSON-RPC error、未知字段、缺失 id、非法 JSON 和 notification。

**验证：** `go test ./internal/mcp -run 'Protocol|JSONRPC'` 通过。

## T4: 建立受控 MCP 测试 Server

**文件：** `internal/testutil/mcpserver.go`

**依赖：** T3

**步骤：**

1. 提供可脚本化的 JSON-RPC handler，记录收到的方法、ID、参数和 headers。
2. 提供 HTTP `httptest.Server` 工厂，可配置 JSON、SSE、错误状态和 session header 响应。
3. 提供 stdio helper 模式，使测试子进程能按 JSON Lines 收发并把 stderr 与 stdout 分离。
4. 提供等待/断言辅助，避免时间竞争和真实网络依赖。

**验证：** `go test ./internal/testutil ./internal/mcp -run 'Test.*Server'` 通过。

## T5: 定义传输边界并实现 Session 请求配对

**文件：** `internal/mcp/transport.go`、`internal/mcp/session.go`、`internal/mcp/session_test.go`

**依赖：** T3

**步骤：**

1. 定义 `Transport`、`Inbound` 和传输关闭/错误事件。
2. 实现 `Session` 的唯一递增 ID、pending 等待表、接收分派循环和幂等关闭。
3. 实现 `Request` 的 context 取消清理，避免已取消调用遗留 pending 项。
4. 实现 `Notify`，确保 `notifications/initialized` 不创建 pending 项。
5. 让 JSON-RPC error、未知 ID、无效 response 和 transport 终止按定义映射为调用失败；关闭时结束所有仍等待的请求。
6. 添加乱序双响应、并发请求、JSON-RPC error、未知 ID、取消、关闭和数据竞争测试。

**验证：** `go test -race ./internal/mcp -run 'Session|Pending|OutOfOrder|Close'` 通过。

## T6: 实现 stdio JSON Lines 传输

**文件：** `internal/mcp/stdio.go`、`internal/mcp/stdio_test.go`

**依赖：** T3、T4、T5

**步骤：**

1. 实现 `StdioTransport.Start`，以配置 command、args 和展开后的 env 启动独立子进程。
2. 对 stdin 写入加锁，并为每个 JSON-RPC 消息追加单一换行符。
3. 在 goroutine 中逐行读取 stdout 并发布 JSON-RPC 入站消息；非 JSON stdout 视为协议错误。
4. 单独消费 stderr，生成长度受限的 Server 诊断，绝不混入协议消息。
5. 实现关闭时取消、终止和等待子进程，避免泄漏。
6. 使用受控子进程测试初始化/通知/列工具/调用消息、stderr 隔离、异常 stdout 和子进程退出。

**验证：** `go test ./internal/mcp -run 'Stdio|JSONLines|Subprocess'` 通过。

## T7: 实现 Streamable HTTP 与 SSE 响应传输

**文件：** `internal/mcp/http.go`、`internal/mcp/sse.go`、`internal/mcp/http_test.go`

**依赖：** T3、T4、T5

**步骤：**

1. 实现 HTTP POST JSON-RPC body，合并配置 headers 与协议所需 headers。
2. 处理 JSON response 和同一 POST response 中的 SSE event stream，并把每条 JSON-RPC 消息发布给 Session。
3. 从初始化 response 提取 `MCP-Session-Id`，后续 POST 附带 session 和协商 `MCP-Protocol-Version`。
4. 对带 session 的 HTTP 404 生成会话失效错误，不执行重新握手、重试或回退。
5. 实现关闭时带 session header 的 DELETE；没有 session 时不发 DELETE。
6. 使用受控 HTTP Server 测试配置 headers、JSON、SSE、协议版本、会话 header、404、DELETE 与敏感值不泄露。

**验证：** `go test ./internal/mcp -run 'HTTP|Streamable|SSE|SessionHeader|SessionExpired'` 通过。

## T8: 实现 MCP Client 初始化、发现与调用

**文件：** `internal/mcp/client.go`、`internal/mcp/client_test.go`

**依赖：** T5、T6、T7

**步骤：**

1. 实现 `Client.Initialize`，发送客户端信息和能力，校验并记录服务端选择的协议版本。
2. 初始化成功后发送 `notifications/initialized`，再允许工具发现或调用。
3. 实现 `ListTools`，解析并校验远端 name、description、inputSchema，返回稳定顺序工具列表。
4. 实现 `CallTool`，把原始 JSON 对象作为 `arguments` 发送，并解析 content、structuredContent 与 `isError`。
5. 将 JSON-RPC、协议、传输和会话失效错误归类为可诊断调用错误，不进行重试。
6. 使用 stdio 和 HTTP 受控 Server 验证三步顺序、请求参数、调用成功、MCP 工具错误和协议失败。

**验证：** `go test ./internal/mcp -run 'Client|Initialize|ListTools|CallTool'` 通过。

## T9: 实现远端工具适配层

**文件：** `internal/mcp/tool.go`、`internal/mcp/tool_test.go`

**依赖：** T8

**步骤：**

1. 定义 `RemoteTool` 与 `RemoteToolAdapter`，拼接 `<server>__<tool>` 注册名。
2. 保留远端 description 与 inputSchema，并在执行前调用现有 `tools.Validate`。
3. 将适配工具的 Safety 固定为 `tools.SafetySideEffect`，Permission target 固定为 none。
4. 将文本和结构化 MCP 成功内容转为 `tools.Success`，将 `isError`、协议或传输失败转为 `tools.Failure`。
5. 添加测试验证名称、Schema、权限安全分类、参数原样传递和各种结果映射。

**验证：** `go test ./internal/mcp -run 'RemoteTool|Adapter|ToolResult'` 通过。

## T10: 实现多 Server Manager、原子注册与关闭

**文件：** `internal/mcp/manager.go`、`internal/mcp/manager_test.go`

**依赖：** T2、T8、T9

**步骤：**

1. 定义 `Manager`、`Diagnostic` 和安全 reporter，按 Server 名稳定排序依次处理已合并配置。
2. 对每个可用 Server 创建正确 transport 和 Client，执行初始化与发现，并缓存成功 Client。
3. 在注册前验证该 Server 的所有命名空间工具均不与 Registry 或自身重复，避免半注册状态。
4. 某 Server 配置、连接、握手、发现或注册失败时，关闭其资源并记录诊断，继续处理其他 Server。
5. 实现 `Close`，反向关闭所有成功 Client，并聚合关闭错误。
6. 添加多 Server 成功、一个失败不影响另一个、与内置工具冲突、同 Server 工具冲突、单连接复用和关闭清理测试。

**验证：** `go test ./internal/mcp -run 'Manager|Isolation|Conflict|Lifecycle|Cache'` 通过。

## T11: 将 MCP Manager 接入主程序启动与退出

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** T1、T2、T10

**步骤：**

1. 使用已选定主配置中的 `mcp_servers`；确定工作区根目录时不得读取项目级 MCP 配置。
2. 在内置 Registry 建立后创建 Manager、发现并注册远端工具；安全输出启动诊断。
3. 用 `defer` 确保 TUI 返回、Provider/Runner 创建失败或后续启动失败时均关闭 Manager。
4. 保持空 MCP 配置的启动路径和现有依赖注入测试可用。
5. 添加 main 测试覆盖空配置、单 Server 失败继续启动、诊断脱敏及退出关闭。

**验证：** `go test ./cmd/mewcode -run 'MCP|Config|Run|Close'` 通过。

## T12: 更新示例和文档索引

**文件：** `config.example.yaml`、`docs/README.md`

**依赖：** T1、T11

**步骤：**

1. 增加最小 stdio 与 HTTP `mcp_servers` 示例，展示 `${VAR}` 用法但不包含真实凭据。
2. 说明 `mcp_servers` 是主配置顶层字段，且不存在项目级 MCP 覆盖文件，以及远端工具命名规则。
3. 在文档索引加入 `ch07-mcp` 的四份文档链接，同时修正已有 ch06 索引指向实际目录。
4. 确认示例配置仍可被配置 loader 解析。

**验证：** `go test ./internal/config && rg -n 'ch07-mcp|mcp_servers' config.example.yaml docs/README.md` 输出预期条目。

## T13: 执行 MCP 与全项目回归

**文件：** 全项目

**依赖：** T1–T12

**步骤：**

1. 运行 `internal/mcp` 的完整测试与 race 检查。
2. 运行 `internal/config`、`internal/tools`、`internal/agent`、`internal/permissions`、`internal/tui` 与 `cmd/mewcode` 测试，确认远端工具默认副作用权限不破坏既有路径。
3. 运行全项目测试；若失败，回到归属任务修复并重跑对应验证。
4. 执行一个受控端到端场景：用户级 HTTP Server 与项目级 stdio Server 同时发现，Agent 调用命名空间工具，关闭后资源都被清理。

**验证：** `go test -race ./internal/mcp/... && go test ./...` 通过；受控端到端测试通过。

## 执行顺序

```text
T1 → T2 ──────────────────┐
T3 → T4 → T5 ─┬─ T6 ─┐   │
              └─ T7 ─┴→ T8 → T9 → T10 → T11 → T12 → T13
```
