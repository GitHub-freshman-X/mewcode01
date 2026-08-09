# MewCode 应用日志 Checklist

> 每项均通过运行代码或观察输出验证；日志检查使用测试 token、测试环境变量值和测试文件内容，确认它们均未出现在日志中。

## 实现完整性

- [ ] **独立日志文件**：在临时项目根目录创建记录器后，存在唯一的 `logs/mewcode-*.jsonl` 文件。（验证：运行 `go test ./internal/logging -run TestNewWritesJSONL`，期望测试解码到一条事件。）
- [ ] **稳定事件结构**：每行日志可解码为 JSON，并包含 `time`、`level`、`source`、`message`；调用方字段置于 `fields`。（验证：运行 `go test ./internal/logging -run TestNewWritesJSONL`，期望字段断言通过。）
- [ ] **运行实例隔离**：相同项目根目录内以不同时间或 PID 创建两次记录器时，生成两个不同文件且不覆盖已有日志。（验证：运行 `go test ./internal/logging -run TestNewUsesDistinctRunFiles`，期望文件数为 2。）
- [ ] **无操作降级**：no-op 记录器记录、派生字段和关闭均不创建文件且不返回错误。（验证：运行 `go test ./internal/logging -run TestNop`，期望通过。）
- [ ] **并发完整性**：并发写入后，日志行数等于写入次数，所有行可独立解码。（验证：运行 `go test -race ./internal/logging -run TestLoggerConcurrentWrites`，期望通过且 race detector 无报告。）
- [ ] **日志故障隔离**：日志初始化失败时 MewCode 保持既有启动路径，stderr 给出日志初始化诊断而非直接退出。（验证：运行 `go test ./cmd/mewcode -run TestRunContinuesWhenLoggingInitializationFails`，期望通过。）

## MCP 生命周期

- [ ] **成功 Server 注册可观测**：受控健康 HTTP MCP Server 完成握手和发现后，日志按生命周期至少包含连接开始、连接成功、初始化成功、发现成功、`tool_registered` 和关闭成功。（验证：运行 `go test ./internal/mcp -run TestManagerLogsSuccessfulRegistration`，期望通过。）
- [ ] **最终工具名可观测**：远端 `search` 被注册后，`tool_registered` 事件包含 `server=healthy`、`remote_tool=search` 和 `tool=healthy__search`。（验证：运行 `go test ./internal/mcp -run TestManagerLogsSuccessfulRegistration`，期望通过。）
- [ ] **失败 Server 隔离可观测**：无效 stdio Server 写入带 `server=broken` 和 `status=connect_failed` 的事件，同时健康 Server 仍注册工具。（验证：运行 `go test ./internal/mcp -run TestManagerLogsFailedConnectionWithoutBreakingHealthyServer`，期望通过。）
- [ ] **初始化、通知和发现状态可观测**：初始化成功、初始化通知失败和 `tools/list` 失败各自产生对应的固定事件与状态字段。（验证：运行 `go test ./internal/mcp -run TestClientLogsLifecycleStates`，期望通过。）
- [ ] **调用结果可观测**：远端工具调用会出现 `tool_call_started` 及恰好一个完成事件；成功、MCP `isError`、JSON-RPC/传输错误和输入校验错误的状态可区分。（验证：运行 `go test ./internal/mcp -run TestRemoteToolAdapterLogsCallOutcomes`，期望通过。）

## 安全与兼容性

- [ ] **配置秘密不落盘**：带有测试 API Key、Authorization header、环境变量值和带凭据 URL 的受控配置参与 MCP 初始化后，日志不含这些原始字符串。（验证：运行 `go test ./internal/mcp -run TestMCPLogsRedactConfigurationValues`，期望通过。）
- [ ] **调用载荷不落盘**：使用包含测试 token、路径、文件内容和结构化响应的工具调用后，日志不含参数 JSON、文本内容、结构化内容和原始错误文本。（验证：运行 `go test ./internal/mcp -run TestRemoteToolAdapterLogsCallOutcomes`，期望通过。）
- [ ] **现有 MCP 行为保持不变**：所有 MCP 单元测试和竞态检查通过。（验证：运行 `go test -race ./internal/mcp/...`，期望退出码为 0。）
- [ ] **全项目回归通过**：所有现有包测试通过。（验证：运行 `go test ./...`，期望退出码为 0。）

## 端到端场景

- [ ] **filesystem stdio Server**：在独立临时目录创建 `client/` 和 `fixtures/hello.txt`，令 `MCP_TEST_ROOT` 指向 `fixtures/`；在 `client/` 启动 MewCode 后，允许一次 `filesystem__read_text_file` 调用并读取 `hello.txt`。（验证：TUI 显示带 `filesystem__` 前缀的工具调用并返回 `hello MCP`。）
- [ ] **filesystem 日志检查**：退出上述 MewCode 进程后，检查 `client/logs/` 的新增 JSONL 文件。（验证：文件包含 `server=filesystem` 的连接、发现、`tool_registered` 和调用完成事件；不包含 `MCP_TEST_ROOT` 展开后的绝对路径或 `hello MCP` 正文。）
- [ ] **无 MCP 配置兼容性**：移除 `mcp_servers` 后启动 MewCode 并退出。（验证：仍创建本次运行的日志文件；不存在 MCP Server 成功注册事件；内置工具和 TUI 行为保持可用。）

## 验收记录

| 日期 | 命令或场景 | 结果 | 证据/备注 |
|---|---|---|---|
| 2026-08-06 | `go test -race ./internal/logging ./internal/mcp/...` | 通过 | Logger 与 MCP 包通过 race detector。 |
| 2026-08-06 | `go test ./...` | 通过 | 全部 Go 包测试通过。 |
| 待执行 | filesystem stdio 手工场景 | 未执行 | 需要可用 Provider 配置与本地 Node.js/npx 环境。 |
