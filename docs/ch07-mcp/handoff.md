# 第七章 MCP 客户端开发交接

> 状态：**自动化开发与回归已完成；真实 Provider/Node 环境下的手工验收待执行。**
>
> 最后更新：2026-08-03

## 已完成并提交的内容

### 文档与设计

第七章的 Spec 驱动开发文档已获用户批准：

- `spec.md`：范围、功能与验收标准。
- `plan.md`：配置、协议、传输、Client、Manager 与工具适配设计。
- `task.md`：T1–T13 的有序实现任务。
- `checklist.md`：AC1–AC12 与端到端验收项。
- `manual_scenarios.md`：手工测试场景；**只能在本章实现完成后执行**。

### T1：用户级 MCP 配置

已实现并提交 `d79de76 feat: add mcp server config`：

- `internal/config.Config` 新增 `MCPServers`。
- 支持 `mcp_servers` 中的 `stdio` 与 `http` Server 声明。
- 校验 stdio 的 `command`、HTTP 的绝对 URL 和传输类型。
- 旧配置不带 `mcp_servers` 时仍可加载。
- 已验证：`go test ./internal/config` 通过。

### T2：历史实现的项目级 MCP 覆盖（已被新配置规范废止）

已实现并提交 `3bc9b33 feat: load and merge mcp configs`：

- 当时的 `internal/mcp/config.go` 曾读取 `<项目根>/.mewcode/config.yaml` 中的 `mcp_servers`。
- 当时项目级同名 Server 会完整覆盖用户级定义。
- `command`、`args`、`env`、`url`、`headers` 支持 `${VAR}` 展开。
- 缺失变量会返回不含敏感值的错误；尚未被 Manager 接入为“仅跳过该 Server”的启动诊断。
- 已验证：配置合并和展开的聚焦测试通过。

### T3：JSON-RPC 2.0 最小协议模型

已实现并提交 `4a0524c feat: add mcp jsonrpc protocol`：

- JSON-RPC request、notification、response 和 error 的编码/解码。
- request 使用唯一数值 ID；notification 不带 ID。
- 检查错误 JSON、错误版本、缺失 ID、缺失 result/error、同时包含 result/error 的非法响应。
- 已验证：`go test ./internal/mcp -run 'Test(EncodeRequestAndNotification|DecodeResponse)'` 通过。

### T5：Session 请求 ID 路由

已实现并提交 `ce75801 feat: add mcp session routing`：

- `Transport` 接口、入站消息类型和 `Session`。
- 并发 pending map、递增请求 ID、乱序响应按 ID 配对。
- `Notify`、context 取消、JSON-RPC error、传输终止和关闭时的 pending 清理。
- 已验证：`go test -race ./internal/mcp -run TestSession` 通过。

### T6（部分）：stdio JSON Lines transport

已实现并提交 `8ec8c54 feat: add mcp stdio transport`：

- `StdioTransport` 可启动子进程、向 stdin 写入 JSON Lines、从 stdout 读取行、关闭子进程。
- 已验证：`go test ./internal/mcp -run TestStdioTransportSendsAndReceivesJSONLines` 通过。
- 限制：尚未与 Client/Manager/CLI 集成，stderr 当前也还没有按计划形成安全诊断。

## 未完成内容（必须继续实现）

1. **T4 受控 MCP 测试 Server**：补 `internal/testutil/mcpserver.go`，供 stdio/HTTP/Client/Manager 离线测试使用。
2. **完成 T6**：完善 stdio 错误、stderr 诊断、关闭和子进程生命周期测试。
3. **T7 Streamable HTTP 与 SSE**：实现 HTTP POST、JSON/SSE 响应、`MCP-Session-Id`、`MCP-Protocol-Version`、DELETE 关闭、404 会话失效且不重试。
4. **T8 MCP Client**：实现 `initialize → notifications/initialized → tools/list` 和 `tools/call`；解析远端工具与调用结果。
5. **T9 Remote Tool Adapter**：把远端工具转换为现有 `tools.Tool`，固定命名 `<server>__<tool>`，Safety 为 `side_effect`。
6. **T10 Manager**：多 Server 初始化、故障隔离、原子注册、缓存和关闭。
7. **T11 CLI 启动集成**：使用已选定主配置中的 `mcp_servers`、展开 Server、调用 Manager 注册工具、输出安全诊断，并在退出时关闭连接。
8. **T12 文档/示例**：更新 `config.example.yaml` 与 `docs/README.md`。
9. **T13 验收**：执行 checklist、`go test -race ./internal/mcp/...`、`go test ./...`，并重跑手工场景。

## 2026-08-03 完成的实现与验证

- 已实现受控 MCP HTTP test server、stdio stderr 消费、Streamable HTTP（JSON/SSE、session header、DELETE/404）、Client、Remote Tool Adapter、Manager 和 CLI 启动/关闭集成。
- 已更新配置示例和文档索引。
- 通过：`go test -race ./internal/mcp/...`、`go test ./...`。
- 尚未执行 `manual_scenarios.md`，因为它需要用户准备有效 Provider、Node/npx 与独立临时目录；这不影响离线自动化回归结论。

## 当前已知问题

记录位置：`bugs/2026-07-12/001-mcp-startup-does-not-register-servers.md`。

根因：`cmd/mewcode/main.go` 当前只创建内置 `tools.NewDefaultRegistry(root)`，没有加载或注册任何 MCP Server。因此模型只会看到 `find_files`、`search_code` 等内置工具，绝不会看到 `filesystem__...`。

这不是 `MCP_TEST_ROOT` 设置问题；内置 `read_file` 对工作区外路径的拒绝是既有权限沙箱的正确行为。

该 bug 在 T8–T11 完成、启动集成测试通过，并重新执行 `manual_scenarios.md` 场景 1 后才能标记为“已修复”。每次继续排查或修改该问题时，都必须按项目约定同步更新 `bugs/`。

## 工作区注意事项

- 当前分支：`codex/ch07-mcp`。
- 用户存在未跟踪的 `.mewcode/config.yaml` 和 `.mewcode/permissions.yaml`，用于手工测试；其中 `.mewcode/config.yaml` 已不属于当前配置规范，不要删除、覆盖或提交它。
- `bugs/README.md` 与 `bugs/2026-07-12/` 是本次问题的未提交记录；继续工作时应保留并最终一并提交合适的 bug 更新。
- 手工场景文档此前被过早生成；继续开发期间应始终说明它尚不可执行，直至 T13 通过。

## 推荐继续顺序

1. 阅读 `task.md`、`checklist.md` 与本文件，检查当前 `git status`。
2. 完成 T4 的受控 test server。
3. 以测试先行的方式完成 T6 剩余部分与 T7。
4. 完成 T8–T10 后，先为 `cmd/mewcode/main.go` 写失败的启动集成测试，再实现 T11。
5. 使用一个受控 stdio MCP Server 验证：启动时 Registry 中出现 `filesystem__...`，Provider 接收到该工具定义，Agent 的调用到达 Server。
6. 最后完成 HTTP/SSE、文档与全量验收，更新 bug 为“已修复”。

## 已知验证证据

- 在继续开发前，基线 `go test ./...` 曾通过（需要允许 Go 使用工作区外的 build cache）。
- 当前已经验证的聚焦测试见各任务说明；它们仅证明基础模块，不证明 MCP 可被 MewCode 启动和使用。
