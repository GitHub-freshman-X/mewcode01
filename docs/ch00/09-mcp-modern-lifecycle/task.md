# MCP Modern Lifecycle Compatibility Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `internal/mcp/protocol.go` | 结构化 JSON-RPC 错误 |
| 修改 | `internal/mcp/session.go` | 保留 RPC error code 供协商分类 |
| 修改 | `internal/mcp/client.go` | 新旧生命周期协商、每请求元数据、工具缓存 |
| 修改 | `internal/mcp/http.go` | 新版 HTTP headers、无 session 生命周期 |
| 修改 | `internal/mcp/manager.go` | 协商后工具发现与安全日志 |
| 修改 | `internal/testutil/mcpserver.go` | 新版协议受控测试能力 |
| 修改 | `internal/mcp/*_test.go` | 生命周期、降级、故障和回归测试 |
| 修改 | `README.md` | 用户可见的自动协商说明 |
| 新建 | `docs/ch00/09-mcp-modern-lifecycle/{plan,task,checklist,manual_scenarios}.md` | 本独立需求的设计、任务、验收和人工场景 |

## T1: 建立协议代际与错误分类基础

**文件：** `internal/mcp/protocol.go`、`internal/mcp/session.go`、`internal/mcp/session_test.go`

**依赖：** 无

**步骤：**

1. 定义现代协议版本、生命周期状态和可供 `errors.As` 识别的 JSON-RPC 错误类型。
2. 让 Session 请求在收到 JSON-RPC error 时保留 code、message 与可选 data。
3. 保持请求 ID 路由、通知、关闭与原有错误文本的兼容性。

**验证：** `go test ./internal/mcp -run 'Test(Session|Protocol)' -count=1` 通过。

## T2: 实现新版每请求元数据与 HTTP 传输模式

**文件：** `internal/mcp/client.go`、`internal/mcp/http.go`、`internal/mcp/http_test.go`

**依赖：** T1

**步骤：**

1. 为 Client 增加新版请求 `_meta` 构造逻辑。
2. 为 HTTP Transport 增加新版 headers 注入与旧版 mode 恢复，不改变 JSON/SSE 解码。
3. 保证新版不存取 session ID、不发 DELETE；旧版仍携带 session ID 并发 DELETE。
4. 覆盖新版 `tools/list`、`tools/call` 的 headers 与旧版 HTTP session 回归。

**验证：** `go test ./internal/mcp -run 'TestHTTPTransport' -count=1` 通过。

## T3: 实现自动协商、降级与工具缓存

**文件：** `internal/mcp/client.go`、`internal/mcp/client_test.go`

**依赖：** T1、T2

**步骤：**

1. 实现 `server/discover` 成功后的新版生命周期选择。
2. 对不支持与探测超时执行一次旧版握手；对网络、认证、TLS、429、5xx 和外层取消不降级。
3. 在新版和旧版下让 `ListTools`、`CallTool` 使用已协商路径。
4. 为 `tools/list` 读取并遵循 `ttlMs` / `cacheScope` 的当前 Client 缓存。

**验证：** `go test ./internal/mcp -run 'TestClient' -count=1` 通过。

## T4: 接入 Manager 与受控 Server 场景

**文件：** `internal/mcp/manager.go`、`internal/mcp/manager_test.go`、`internal/testutil/mcpserver.go`

**依赖：** T3

**步骤：**

1. 在 Manager 工具发现前运行协商，按实际代际写入安全日志与诊断。
2. 增强受控 HTTP Server，记录现代 headers、请求参数和 DELETE。
3. 覆盖 HTTP/stdio 新版成功、显式旧版降级、超时降级、不降级错误和多 Server 故障隔离。

**验证：** `go test ./internal/mcp -run 'TestManager' -count=1` 通过。

## T5: 更新文档并完成回归验收

**文件：** `README.md`、`docs/ch00/09-mcp-modern-lifecycle/{spec,plan,task,checklist}.md`

**依赖：** T4

**步骤：**

1. 在 README 说明无需配置的新版优先、旧版自动降级、现有 `stdio`/`http` 格式不变及能力边界。
2. 编写隔离 fixture、真实 Provider 和受控 Server 的人工场景，覆盖新版、旧版、超时、认证隔离和关闭语义。
3. 将实际执行的自动化验证和未验证事项回填 Checklist。
4. 格式化代码并执行 MCP 包、全量测试、构建和 diff 检查。

**验证：** `gofmt -w internal/mcp`、`go test -race ./internal/mcp/...`、`go test ./...`、`go build ./cmd/mewcode`、`git diff --check` 通过。

## 执行顺序

```text
T1 → T2 → T3 → T4 → T5
```
