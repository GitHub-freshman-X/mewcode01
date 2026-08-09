# MewCode 应用日志 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/logging/logger.go` | JSONL 事件结构、文件记录器、no-op 记录器与并发安全写入 |
| 新建 | `internal/logging/logger_test.go` | 记录器文件、字段、并发、关闭和降级测试 |
| 修改 | `cmd/mewcode/main.go` | 项目根确定后初始化、注入并关闭记录器 |
| 修改 | `cmd/mewcode/main_test.go` | 日志初始化失败不影响既有启动错误处理的测试 |
| 修改 | `internal/mcp/manager.go` | 每个 Server 的配置、连接、注册、关闭事件 |
| 修改 | `internal/mcp/client.go` | 初始化、通知、发现和远端调用事件 |
| 修改 | `internal/mcp/tool.go` | 适配工具的调用开始、成功和失败事件 |
| 修改 | `internal/mcp/manager_test.go` | 成功注册、隔离失败和日志字段断言 |
| 修改 | `internal/mcp/client_test.go` | 初始化、发现、调用成功/失败事件断言 |
| 修改 | `internal/mcp/tool_test.go` | 适配工具调用状态与无载荷泄露断言 |
| 新建 | `docs/ch00/01-logging/checklist.md` | 后续阶段的可观测验收清单 |

## T1：实现结构化文件记录器

**文件：**

- 新建：`internal/logging/logger.go`
- 新建：`internal/logging/logger_test.go`

**依赖：** 无。

**步骤：**

1. 在测试中创建临时项目根目录，用固定时间和 PID 调用 `logging.New`；记录一条 Info 事件并关闭记录器。
2. 断言只创建一个 `logs/mewcode-*.jsonl` 文件；逐行解码 JSON，断言存在 `time`、`level`、`component`、`event`、`message`，且自定义字段保持可解析。
3. 添加并发测试：多个 goroutine 经同一记录器写入固定数量事件；关闭后逐行解码并断言记录数准确。
4. 添加 no-op 测试：`logging.Nop()` 的记录和关闭不创建文件、不返回错误。
5. 在 `logger.go` 定义 `Fields`、`Event` 与 `Logger`，实现 `New(root, now, pid)`、`Nop()`、`WithFields`、`Info`、`Error` 和 `Close`。
6. `New` 创建 `<root>/logs` 和包含时间、纳秒、PID 的独立 `.jsonl` 文件；`Logger` 用 mutex 保护 JSON 编码和单次文件写入。写入错误静默返回，`Close` 幂等。
7. `WithFields` 复制基础字段和调用方字段，避免后续调用修改共享 map；Info/Error 把 `level` 固定为 `info`/`error`。
8. 运行 `go test -race ./internal/logging`，期望全部通过。

**验证：** `go test -race ./internal/logging`。

## T2：在启动流程创建并注入记录器

**文件：**

- 修改：`cmd/mewcode/main.go`
- 修改：`cmd/mewcode/main_test.go`

**依赖：** T1。

**步骤：**

1. 在 `main_test.go` 为日志构造函数增加可替换测试入口，使测试可让日志初始化返回错误和 no-op 记录器。
2. 添加测试：日志初始化返回错误时，`run` 不因该错误提前退出，且 stderr 包含不带敏感内容的日志初始化诊断。
3. 在 `main.go` 取得工作区根目录后调用 `logging.New(root, time.Now, os.Getpid())`；失败时写 stderr，并使用 `logging.Nop()` 继续。
4. 使用 `defer logger.Close()` 关闭文件，关闭错误仅写 stderr，不改变已有退出码。
5. 将记录器传给 `mcp.NewManager`；不改变 Provider、权限、TUI 初始化顺序和既有 stderr 用户错误输出。
6. 运行 `go test ./cmd/mewcode`，期望通过。

**验证：** `go test ./cmd/mewcode`。

## T3：记录 MCP Server 启动、发现、注册与关闭

**文件：**

- 修改：`internal/mcp/manager.go`
- 修改：`internal/mcp/manager_test.go`

**依赖：** T1、T2。

**步骤：**

1. 将 `NewManager` 扩展为接收 `*logging.Logger`；nil 值规范化为 `logging.Nop()`，以兼容未更新的测试调用点。
2. 在测试中用临时文件记录器创建一个健康 HTTP Server 和一个无效 stdio Server，完成注册、关闭并解析 JSONL。
3. 断言健康 Server 有 `server_connect_started`、`server_connected`、`tool_registered`、`server_closed`；`tool_registered` 的字段含 `server=healthy`、`remote_tool=search`、`tool=healthy__search`。
4. 断言失败 Server 有 `server_connect_failed`，且事件字段不包含 command、args、环境变量值或错误文本。
5. 在 Manager 中为每个名称创建 `logger.WithFields({"server": name})`；在配置展开、连接、Client 初始化/发现后的注册循环、冲突以及 `Close` 的每个分支记录计划定义的固定事件。
6. 仅写入 `server`、`transport`、`stage`、`status`、`remote_tool`、`tool` 和 `tool_count`；禁止将 `server.URL`、headers、`err.Error()` 或配置结构写入事件。
7. 运行 `go test -race ./internal/mcp -run TestManager`，期望通过。

**验证：** `go test -race ./internal/mcp -run TestManager`。

## T4：记录 Client 初始化、通知、发现与 RPC 调用状态

**文件：**

- 修改：`internal/mcp/client.go`
- 修改：`internal/mcp/client_test.go`

**依赖：** T1、T3。

**步骤：**

1. 将 `NewClient` 扩展为接收 `*logging.Logger`，nil 值使用 no-op；Manager 用每个 Server 的记录器构造 Client。
2. 扩展受控 transport 测试，在初始化成功、初始化通知失败、`tools/list` 失败和 `tools/call` JSON-RPC 失败场景结束后解析日志。
3. 断言成功初始化写入 `initialize_started`、`initialize_succeeded` 和 `initialized_notification_sent`；工具发现写入开始、成功及 `tool_count`。
4. 断言失败只记录固定 `status` 和 `stage`，不包含请求参数、远端响应体、协议错误字符串或传输错误字符串。
5. 在 `Initialize`、`ListTools`、`CallTool` 各自的开始、成功和错误返回分支写入计划中的事件；调用事件仅携带 `remote_tool`，最终命名工具由 Adapter 补齐。
6. 运行 `go test -race ./internal/mcp -run TestClient`，期望通过。

**验证：** `go test -race ./internal/mcp -run TestClient`。

## T5：记录远端工具适配器的调用结果并验证脱敏

**文件：**

- 修改：`internal/mcp/tool.go`
- 修改：`internal/mcp/tool_test.go`

**依赖：** T1、T4。

**步骤：**

1. 将 `NewRemoteToolAdapter` 扩展为接收 `*logging.Logger`；Manager 创建 Adapter 时传入同一 Server 记录器。
2. 在现有适配器成功、MCP `isError`、Client 返回错误和输入 Schema 校验失败场景中，使用包含测试 token、参数与内容的输入和响应。
3. 断言每次实际远端调用有 `tool_call_started` 与其对应的 `tool_call_succeeded` 或 `tool_call_failed`，字段包含 `server`、`tool`、`remote_tool`、`stage=call`、`status`。
4. 断言日志不包含测试 token、JSON 参数、MCP 文本内容、结构化内容或 `err.Error()` 中的原始载荷。
5. 在 Adapter 中于 Client 调用前写开始事件；按成功、`isError`、Client 错误分支写固定状态。输入 Schema 在远端调用前失败时仅记录 `validation_failed`，不记录输入。
6. 运行 `go test -race ./internal/mcp -run TestRemoteToolAdapter`，期望通过。

**验证：** `go test -race ./internal/mcp -run TestRemoteToolAdapter`。

## T6：完成端到端回归与文档验收

**文件：**

- 修改：`docs/ch00/01-logging/checklist.md`

**依赖：** T1 至 T5。

**步骤：**

1. 按已批准的 spec 编写可观察的 checklist，覆盖日志文件、成功 MCP 注册、失败隔离、调用结果、敏感信息和降级行为。
2. 执行 `go test -race ./internal/mcp/...`，期望全部通过。
3. 执行 `go test ./...`，期望全部通过。
4. 在独立临时工作区配置 filesystem stdio Server，启动 MewCode、允许一次 `filesystem__read_text_file` 调用并退出。
5. 检查临时工作区 `logs/` 的新 JSONL 文件包含 `filesystem` 的连接、发现、`tool_registered` 与工具调用完成事件，且不包含 `MCP_TEST_ROOT` 的展开值或文件正文。
6. 记录每项实际命令、结果和任何未验证事项；仅在全部通过后更新 checklist 状态并提交。

**验证：** `go test -race ./internal/mcp/... && go test ./...`，以及 filesystem 手工场景。

## 执行顺序

```text
T1 → T2 → T3 → T4 → T5 → T6
```

