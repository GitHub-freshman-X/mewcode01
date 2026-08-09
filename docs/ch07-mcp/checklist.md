# MewCode MCP Client Checklist

> 每一项通过运行代码或观察行为来验证，聚焦用户可见与协议可观测行为。

## 配置与发现

- [ ] **AC1 单一主配置来源**：MCP Server 仅来自已选定 `config.yaml` 顶层的 `mcp_servers`；项目目录中的额外 MCP 配置不会参与加载。（验证：运行 `go test ./internal/config ./internal/mcp -run 'MCP|Config'`，期望最终 Server map 仅与主配置一致。）
- [ ] **AC1 缺失变量只隔离一个 Server**：引用缺失 `${VAR}` 的 Server 未注册，其他 Server 仍会连接和发现；诊断包含 Server 与配置阶段但不含变量值。（验证：运行 `go test ./internal/mcp -run 'Expand|MissingVariable|Diagnostic'`，期望健康 Server 工具存在且诊断已脱敏。）
- [ ] **AC1 非法主配置可诊断**：主配置中的 MCP YAML 非法或含未知字段时产生明确错误，不把它静默解释为空或更宽松配置。（验证：运行 `go test ./internal/config -run 'MCP|Invalid|UnknownField'`，期望错误包含字段安全摘要。）
- [ ] **AC5 仅完整发现后注册**：Server 依次完成初始化、初始化完成通知和 `tools/list` 后才向 Registry 暴露工具；握手或发现失败不会留下工具。（验证：运行 `go test ./internal/mcp -run 'Manager|Initialize|ListTools'`，期望失败 Server 工具数为 0。）

## stdio 与 Streamable HTTP 协议

- [ ] **AC2 stdio JSON Lines 正确**：受控子进程收到逐行有效 JSON-RPC `initialize`、`notifications/initialized`、`tools/list` 和 `tools/call` 消息；stderr 不会进入协议解析。（验证：运行 `go test ./internal/mcp -run 'Stdio|JSONLines|Subprocess'`，期望方法序列正确、stderr 仅见安全诊断。）
- [ ] **AC3 HTTP 请求和会话正确**：HTTP Server 收到配置 headers、JSON-RPC body；初始化返回 session ID 后，发现与调用请求带 `MCP-Session-Id` 和协商协议版本。（验证：运行 `go test ./internal/mcp -run 'HTTP|SessionHeader|ProtocolVersion'`，期望请求记录断言通过。）
- [ ] **AC3 Streamable HTTP SSE 响应可用**：同一 POST 的 SSE response 中携带 JSON-RPC 消息时，Client 能完成对应请求。（验证：运行 `go test ./internal/mcp -run 'Streamable|SSE'`，期望 JSON 与 SSE 两种响应均得到相同结果。）
- [ ] **AC4 异步 ID 配对正确**：同一 Session 并发请求且服务端乱序响应时，每个调用收到与自身 ID 一致的结果。（验证：运行 `go test -race ./internal/mcp -run 'Session|OutOfOrder|Concurrent'`，期望结果无交换且无 race。）
- [ ] **AC4 协议/传输错误局部失败**：JSON-RPC error、无效 response、未知 ID、调用取消和传输中断只使对应请求失败，并清理 pending 项。（验证：运行 `go test ./internal/mcp -run 'Session|ProtocolError|UnknownID|Cancel|Transport'`，期望错误分类与 pending 计数断言通过。）

## 工具接入与调用

- [ ] **AC6 命名空间与元数据正确**：远端 `search` 注册为 `<server>__search`，保留 description 和 input schema，且 Provider 工具定义可见。（验证：运行 `go test ./internal/mcp -run 'RemoteTool|Adapter|Definitions'`，期望名称、描述和 schema 断言通过。）
- [ ] **AC6 远端工具最小权限默认**：每个远端工具均为 `SafetySideEffect` 和 `PermissionTargetNone`，默认权限模式会要求确认。（验证：运行 `go test ./internal/mcp ./internal/permissions -run 'RemoteTool|Safety|DefaultDecision'`，期望 Safety 与权限决策断言通过。）
- [ ] **AC7 tools/call 参数与结果映射正确**：适配工具把 JSON 参数原样发送到 `tools/call`；文本、结构化内容、`isError`、JSON-RPC error 和传输失败分别转为正确的结构化工具结果。（验证：运行 `go test ./internal/mcp -run 'CallTool|ToolResult|Adapter'`，期望请求参数、success/error 类型断言通过。）
- [ ] **AC7 Agent Loop 可继续**：远端工具的 MCP 失败以工具结果回灌，脚本化模型可在下一轮给出最终答复而非让任务崩溃。（验证：运行 `go test ./internal/agent ./internal/mcp -run 'MCP.*Continues|RemoteTool'`，期望终态为 completed。）
- [ ] **AC9 无自动恢复**：带 HTTP session 的调用收到 404 时，返回结构化会话失效失败；不会重新 initialize、重新列工具或重试调用。（验证：运行 `go test ./internal/mcp -run 'SessionExpired|NoRetry|NoReconnect'`，期望请求计数与错误分类断言通过。）

## 多 Server、生命周期与诊断

- [ ] **AC8 单 Server 故障隔离**：多 Server 中一个启动、握手、发现或调用失败时，其他 Server 与内置工具仍可用。（验证：运行 `go test ./internal/mcp ./cmd/mewcode -run 'Isolation|Manager|MCP'`，期望健康工具可执行、失败工具未注册或仅该调用失败。）
- [ ] **AC6/AC8 冲突无半注册**：与内置工具、其他 Server 或本 Server 重复的命名空间工具发生冲突时，不覆盖既有工具，也不留下该 Server 的部分工具。（验证：运行 `go test ./internal/mcp -run 'Conflict|AtomicRegistration'`，期望 Registry 保持原集合。）
- [ ] **AC8 连接缓存和关闭完整**：同一 Server 的多次远端调用复用已初始化 Client；退出时 stdio 子进程退出、HTTP session 发送 DELETE。（验证：运行 `go test ./internal/mcp -run 'Cache|Lifecycle|Close|Delete'`，期望初始化一次、关闭记录完整。）
- [ ] **AC10 诊断不泄露凭据**：配置、连接、握手、发现和调用失败都包含 Server 和阶段，但不包含 Authorization/header 值、展开环境变量值或完整子进程环境。（验证：运行 `go test ./internal/mcp ./cmd/mewcode -run 'Diagnostic|Redact|Sensitive'`，期望所有敏感 canary 均不出现。）

## 既有系统兼容

- [ ] **AC11 空 MCP 配置兼容**：没有 `mcp_servers` 的既有用户配置仍可加载和启动，内置工具和既有 Agent 路径不变。（验证：运行 `go test ./internal/config ./cmd/mewcode -run 'MCP|Load|Run'`，期望空配置场景成功。）
- [ ] **AC11 权限与 Plan/Do 集成**：远端工具通过既有权限门禁；Plan Mode 仍只暴露内置只读工具，Do Mode 恢复远端工具但仍受权限约束。（验证：运行 `go test ./internal/agent ./internal/permissions ./internal/mcp -run 'Plan|Do|Permission|RemoteTool'`，期望工具集合与决策断言通过。）
- [ ] **AC12 可重复离线测试**：受控 stdio/HTTP Server 不访问真实外网；重复运行注册、调用与失败场景，名称、调用次数、结果和错误分类稳定。（验证：连续两次运行 `go test ./internal/mcp`，期望均通过且无时间/网络依赖。）

## 编译与回归

- [ ] **MCP 包测试和竞态检查通过**。（验证：运行 `go test -race ./internal/mcp/...`，期望通过。）
- [ ] **关联模块回归通过**。（验证：运行 `go test ./internal/config ./internal/tools ./internal/agent ./internal/permissions ./internal/tui ./cmd/mewcode`，期望通过。）
- [ ] **全项目测试通过**。（验证：运行 `go test ./...`，期望通过。）
- [ ] **示例和文档可发现**：示例包含 stdio/HTTP `mcp_servers` 与变量展开用法，文档索引链接第七章四份文档。（验证：运行 `rg -n 'mcp_servers|ch07-mcp' config.example.yaml docs/README.md`，期望匹配到示例和索引。）

## 端到端场景

- [ ] **E2E 1：单一配置来源的多 Server 调用**：同一 `config.yaml` 声明 HTTP 与 stdio Server；启动后发现 `issues__query` 与 `filesystem__read`，模型调用其中之一并得到工具结果。（验证：运行受控 CLI/Agent 集成测试，期望两个命名空间工具可见、调用参数正确、结果回灌。）
- [ ] **E2E 2：故障不阻塞可用工具**：同一配置中一个 Server 缺少环境变量或握手失败，另一个 Server 正常发现；Agent 仍能调用健康 Server。（验证：运行受控集成测试，期望 stderr 仅有安全诊断、健康工具调用成功。）
- [ ] **E2E 3：远端工具默认需确认**：默认权限模式下，模型调用远端工具；确认器允许本次调用后工具执行并回灌结果。（验证：运行 Agent/Manager 集成测试，期望先出现权限请求、再收到 `tools/call`、最终任务完成。）
- [ ] **E2E 4：HTTP 会话失效不重试**：完成发现后 Server 对 `tools/call` 返回 404；Agent 收到结构化失败并继续下一轮，Server 未收到第二次 initialize 或 tools/call。（验证：运行受控 HTTP/Agent 集成测试，期望计数和最终文本断言通过。）
