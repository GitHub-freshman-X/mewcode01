# 001 MCP 启动流程未注册远端 Server 工具

## 状态

待手工验证（2026-08-03）

## 用户可见现象

按 `docs/ch07-mcp/manual_scenarios.md` 的场景 1 配置并导出 `MCP_TEST_ROOT` 后，Agent 没有调用 `filesystem__...` 工具，而是调用内置 `find_files`、`search_code` 和 `read_file`。读取工作区外的 `${MCP_TEST_ROOT}/hello.txt` 时，内置 `read_file` 被工作区沙箱拒绝。

## 复现条件

- 在用户级或项目级配置声明 `mcp_servers.filesystem` 为 stdio filesystem Server。
- 导出 `MCP_TEST_ROOT`，从临时工作区启动 MewCode。
- 请求“读取 MCP filesystem server 中 hello.txt 的内容”。

## 根因

第七章已有 `internal/mcp` 的项目配置合并、变量展开、JSON-RPC 和会话基础代码，但启动入口 `cmd/mewcode/main.go` 只创建内置 `tools.NewDefaultRegistry(root)`。它没有加载项目级 MCP 配置、合并及展开 Server 声明、启动/初始化 Server、发现远端工具，或将其注册到该 registry；同时仓库缺少任务清单中所列的 stdio、HTTP、client、tool、manager 和启动集成实现。因此 Provider 从未收到 `filesystem__...` 工具定义。

## 当前进展

- 已核对场景文档、MCP 规格、任务清单和实际启动代码。
- 已确认这不是 `MCP_TEST_ROOT` 未导出导致：配置完全没有被启动入口消费。
- 未修改 `.mewcode/config.yaml`，该未跟踪文件保留为手工测试配置。
- 2026-07-12 手工复现再次确认：模型仅获得内置 `find_files`、`search_code` 和 `read_file`，不会看到 `filesystem__...`；随后对工作区外 `${MCP_TEST_ROOT}` 的内置读取会被既有路径沙箱正确拒绝。
- 用户已授权完成第七章剩余实现；将从 T4、T6–T13 继续，并在启动集成与场景 1 通过后更新本记录为“已修复”。
- 已补充 MCP Client（初始化、工具发现、调用）、HTTP Streamable transport（JSON/SSE、会话 header、DELETE）与远端工具适配、Manager 的初版实现。
- 新增 Client/Adapter 的测试先失败（缺少类型与构造函数），实现后已通过；`go test ./internal/mcp` 于 2026-08-03 通过。
- `cmd/mewcode/main.go` 已接入项目级配置加载、两层合并、Manager 注册与退出关闭；受控 HTTP Server 的 Manager 隔离测试已确认健康 Server 注册为命名空间工具，失败 Server 不阻断注册。
- 2026-08-03 已通过 `go test -race ./internal/mcp/...` 和 `go test ./...`。尚未执行依赖真实 Provider、Node/npx 的手工场景 1，因此保留“待手工验证”状态。
- 已在 `docs/ch07-mcp/handoff.md` 记录当前已实现范围、未完成任务、验证证据和继续顺序，供下一会话接续。
- 交接文档已提交为 `7b35263 docs: add ch07 implementation handoff`；本 bug 记录仍保持未提交和“待处理”状态。
- 2026-08-03 新发现：CLI 仍会调用已被第七章规格废止的 `LoadProjectServers(root)`。当项目 `.mewcode/config.yaml` 是完整主配置时，严格 YAML 解析器只接受 `mcp_servers`，因而将 `protocol`、`model` 等顶层字段报为未知字段并导致启动失败。
- 已添加 `TestRunIgnoresProjectMCPConfig` 作为回归用例；修复前该测试稳定失败（`code=1`、TUI 未启动）。
- 已移除 CLI 对项目级 MCP 覆盖配置的读取与合并；启动现在只使用 `--config` 指定（或默认定位）的主配置内 `mcp_servers`。修复后 `go test ./cmd/mewcode -run TestRunIgnoresProjectMCPConfig -count=1` 与 `go test ./internal/mcp -run 'TestExpandServer' -count=1` 通过；`go test ./cmd/mewcode ./internal/mcp` 和 `go test ./...` 也均通过。

## 修复方案

补齐 MCP client/manager、stdio 与 HTTP transport、远端工具适配，并在 `cmd/mewcode/main.go` 中完成配置加载、发现、注册和退出清理；添加覆盖场景 1 的启动集成测试。

## 验证方式

自动化修复验证已通过：`go test -race ./internal/mcp/...` 与 `go test ./...`。仍应重新执行 `docs/ch07-mcp/manual_scenarios.md` 场景 1，确认真实 Provider 可见并调用 `filesystem__...` 工具，用户得到 `hello MCP`。

本次调查执行 `go test ./...` 时，现有包测试均通过，但 Go build cache 位于工作区外且当前沙箱无权读取，导致汇总命令以 `operation not permitted` 结束；这不是本 MCP 问题的测试失败。

## 后续工作

在具备独立测试目录、有效 Provider 配置与 Node/npx 环境后，执行手工场景 1–6，并把结果补入本记录；若场景 1 成功，可将状态更新为“已修复”。
