# MCP Modern Lifecycle Compatibility Checklist

> 每项均以可观察行为或自动化命令验证；实际结果将在实现完成后回填。

## 生命周期协商

- [ ] **AC1 新版协商**：受控 stdio 与 HTTP Server 返回含 `2026-07-28` 的 `server/discover` 结果时，不接收 `initialize` 或 `notifications/initialized`。（验证：`go test ./internal/mcp -run 'Test.*Modern.*(HTTP|Stdio)' -count=1`）
- [ ] **AC4 显式降级**：不支持新版或不存在 `server/discover` 的 Server 恰好执行一次旧版握手后注册工具。（验证：`go test ./internal/mcp -run 'Test.*Legacy.*Fallback' -count=1`）
- [ ] **AC5 超时降级**：新版探测超时后仅尝试一次旧版握手，且不会无限阻塞。（验证：`go test ./internal/mcp -run 'Test.*Discovery.*Timeout' -count=1`）
- [ ] **AC6 不降级错误**：401/403、TLS/网络、429、5xx 与外层取消不会触发 `initialize`。（验证：`go test ./internal/mcp -run 'Test.*NoFallback' -count=1`）
- [ ] **AC8 协商复用**：同一 Client 多次列工具或调用工具只协商一次。（验证：`go test ./internal/mcp -run 'Test.*Negotiation.*Once' -count=1`）

## 新版请求与旧版兼容

- [ ] **AC2 新版元数据**：新版 HTTP `tools/list`、`tools/call` 含版本、方法、工具名 headers 和 `_meta`；不含 `Mcp-Session-Id`，关闭不发 DELETE。（验证：`go test ./internal/mcp -run 'Test.*Modern.*Headers' -count=1`）
- [ ] **AC3 新版调用**：新版远端工具能收到原始参数，并将成功/工具错误结果回灌现有工具路径。（验证：`go test ./internal/mcp -run 'Test.*Modern.*Call' -count=1`）
- [ ] **AC7 旧版会话**：自动降级的 HTTP Server 返回 session ID 后，后续请求携带该 ID，关闭发 DELETE。（验证：`go test ./internal/mcp -run 'Test.*Legacy.*Session' -count=1`）
- [ ] **AC10 回归**：已有 stdio、旧版 Streamable HTTP、Registry、权限门禁与 Agent Loop 测试均保持通过。（验证：`go test -race ./internal/mcp/... && go test ./...`）

## 安全、文档与端到端

- [ ] **AC9 安全诊断**：协商结果和失败阶段可见，且日志不含 header 值、密钥、请求正文、响应正文或环境变量值。（验证：敏感 canary 测试和日志字段断言。）
- [ ] **AC11 文档**：本章四份文档完整，README 准确描述自动协商与边界。（验证：`rg -n '2026-07-28|server/discover|自动降级' README.md docs/ch00/09-mcp-modern-lifecycle`）
- [ ] **端到端场景**：使用一个新版和一个仅旧版的受控 Server 启动 Manager，观察两个 Server 的工具均注册成功并可分别调用。（验证：`go test ./internal/mcp -run 'TestManager.*(Modern|Legacy)' -count=1`）

## 本次执行记录（2026-09-03）

- [x] 新版 HTTP 生命周期、每请求 `_meta`、`MCP-Protocol-Version`、`Mcp-Method` 和 `Mcp-Name` 已由 `TestClientUsesModernLifecycleAndHTTPHeaders` 验证；未发送 `initialize` 或 `DELETE`。
- [x] 新版 stdio 生命周期与每请求 `_meta` 已由 `TestClientUsesModernLifecycleOverStdio` 验证；请求序列为 `server/discover → tools/list`。
- [x] `server/discover` 方法不存在、HTTP 404 与探测超时的旧版降级分别由 `TestClientFallsBackToLegacyAfterDiscoverMethodNotFound`、`TestClientLegacyHTTPFallbackKeepsSessionAndCloses`、`TestClientFallsBackAfterDiscoveryTimeout` 验证。
- [x] HTTP 401 不会触发旧版握手，已由 `TestClientDoesNotFallbackOnHTTPUnauthorized` 验证。
- [x] 新版 Server 经 Manager 完成工具注册，已由 `TestManagerRegistersModernServer` 验证。
- [x] 并发安全和既有 MCP 回归已通过：`go test -race ./internal/mcp/...`。
- [x] 全仓测试、CLI 构建与 diff 检查已通过：`go test ./...`、`go build ./cmd/mewcode`、`git diff --check`；构建产生的 `mewcode` 已清理。
- [ ] 未使用真实第三方新版或旧版 MCP Server 进行人工端到端验证；自动化测试使用本地受控 HTTP/stdio transport。
