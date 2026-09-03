# MCP Modern Lifecycle Compatibility Plan

## 架构概览

在现有 `Transport → Session → Client → Manager` 分层中新增生命周期协商，而不改变 `stdio` 与 Streamable HTTP 的配置模型。`Client` 在首次使用 Server 时优先探测 `server/discover`；探测成功则以新版无状态请求执行工具发现与调用，探测被识别为旧版不兼容或超时时，切换为既有握手路径。`Manager` 保持“连接、发现、注册”的调用边界，只改为调用协商后的 Client 生命周期。

协议代际只保存在 Client 与 HTTP Transport 的进程内状态中。新版本请求通过 Client 注入 JSON-RPC `params._meta`；HTTP Transport 在新版本模式下从请求消息提取方法与工具名称，注入规范 HTTP headers。旧版路径保留原有 `Mcp-Session-Id`、协议版本 header 与 `DELETE` 关闭行为。

## 核心数据结构

### `Lifecycle`

```go
type Lifecycle string

const (
    LifecycleUnknown Lifecycle = ""
    LifecycleModern  Lifecycle = "modern"
    LifecycleLegacy  Lifecycle = "legacy"
)
```

表示一个 Client 当前确定的协议生命周期。它只在 Client 创建至关闭期间有效；不得写入配置或磁盘。

### `Client`

`Client` 新增生命周期、探测超时和工具目录缓存状态。

```go
type Client struct {
    session          *Session
    logger           *logging.Logger
    lifecycle        Lifecycle
    discoveryTimeout time.Duration
    toolCache        toolCache
}
```

`Negotiate(ctx)` 负责一次性确定生命周期；`ListTools` 与 `CallTool` 必须先确保协商完成。新版请求由 Client 统一附加 `_meta.io.modelcontextprotocol/{protocolVersion,clientCapabilities,clientInfo}`。

### `HTTPTransport`

`HTTPTransport` 新增“新版每请求模式”状态。该状态决定是否为每个 HTTP POST 添加：

- `MCP-Protocol-Version: 2026-07-28`
- `Mcp-Method: <JSON-RPC method>`
- 当方法为 `tools/call` 时的 `Mcp-Name: <tool name>`

新版模式不读取、不保存、不发送 `Mcp-Session-Id`，关闭时不发送 `DELETE`。旧版模式继续使用既有会话状态。

### 协商错误分类

会话层保留结构化 JSON-RPC error code，HTTP 层保留 HTTP 状态，以便 Client 判断能否降级：

| 结果 | 生命周期动作 |
|---|---|
| `DiscoverResult` 包含 `2026-07-28` | 使用 `modern` |
| 明确的“不支持版本”或 `server/discover` 方法不存在 | 使用 `legacy` |
| 仅本次探测的 deadline 超时 | 使用 `legacy` |
| 认证、授权、TLS、网络、429、5xx、外层取消 | 失败，不降级 |

降级仅发生在 `Negotiate`；已选定 `modern` 后的工具调用错误不会改变生命周期。

## 模块设计

### `internal/mcp/client.go`

**职责：** 管理自动协商、旧/新生命周期下的工具发现与调用、每请求元数据和进程内工具目录缓存。

**对外接口：** 保持 `NewClient`、`ListTools`、`CallTool`、`Close` 可用；新增 `Negotiate` 供 Manager 显式调用。`Initialize` 保留为旧版内部流程，以维持现有测试与调用语义。

**流程：**

1. 在新版模式下发送带 `_meta` 的 `server/discover`。
2. 校验 `supportedVersions` 是否包含 `2026-07-28`；成功则固定为新版。
3. 对可降级错误恢复 HTTP Transport 的旧版模式，并执行原 `initialize → notifications/initialized`。
4. `tools/list` 与 `tools/call` 通过统一请求构造器发送；新版自动附加 `_meta`，旧版保持原参数格式。
5. `tools/list` 读取 `ttlMs` 与 `cacheScope`，仅在当前 Client 内按 TTL 复用完整工具定义；缺失或非正 TTL 视为不缓存。

### `internal/mcp/session.go` 与 `internal/mcp/protocol.go`

**职责：** 在不改变请求 ID 路由的前提下，向 Client 暴露可识别的 JSON-RPC 错误代码。保持乱序响应、取消和关闭的既有语义。

### `internal/mcp/http.go`

**职责：** 在单一 HTTP Transport 内隔离新版与旧版 headers、session 保存和关闭路径；继续支持 JSON 与 SSE 响应解码。

### `internal/mcp/manager.go`

**职责：** 对每个已配置 Server 在工具发现前调用 `Client.Negotiate`；按协商或失败结果写入安全诊断，其他注册与关闭逻辑不变。

### `internal/testutil/mcpserver.go` 与 `internal/mcp/*_test.go`

**职责：** 增强受控 Server，使其可断言新版元数据、HTTP headers、探测次数、会话 headers 和关闭请求；覆盖新版、降级及不降级失败。

## 模块交互

```text
Manager.ConnectAndRegister
  │
  ├─ 创建 Transport 与 Client
  │
  ├─ Client.Negotiate
  │    ├─ 新版模式：server/discover + 每请求 _meta / HTTP headers
  │    │      └─ 支持 2026-07-28 → LifecycleModern
  │    └─ 不支持或探测超时
  │           └─ 切回旧版模式 → initialize → initialized → LifecycleLegacy
  │
  ├─ Client.ListTools → TTL 缓存 → Remote Tool Adapter → Registry
  └─ Agent tools/call → Client.CallTool → 已确定的生命周期
```

## 文件组织

```text
internal/mcp/
├── client.go          — 生命周期协商、元数据、工具缓存
├── client_test.go     — 新版/旧版 Client 生命周期测试
├── http.go            — 新版 HTTP headers 与无 session 关闭
├── http_test.go       — HTTP 新旧 headers、JSON/SSE、关闭测试
├── manager.go         — 协商后发现与注册
├── manager_test.go    — 协商、降级和故障隔离测试
├── protocol.go        — JSON-RPC 错误类型
├── session.go         — 保留结构化 RPC 错误
└── session_test.go    — 错误路由回归测试
internal/testutil/mcpserver.go — 受控 MCP HTTP Server 断言支点
docs/ch00/09-mcp-modern-lifecycle/
├── spec.md
├── plan.md
├── task.md
└── checklist.md
README.md — MCP 生命周期兼容策略与用户可见支持说明
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 配置 | 无新增字段 | 自动协商避免把协议细节暴露给用户。 |
| 协商策略 | 新版优先，识别不兼容或超时才降级 | 让新 Server 自动获得无状态行为，同时维持旧生态可用。 |
| 降级时机 | 仅首次协商 | 避免把网络或业务错误误判为版本不兼容，也避免副作用调用被重放。 |
| 新版元数据 | Client 统一注入 `_meta`，HTTP Transport 注入 headers | 传输与协议职责分离，stdio 与 HTTP 共享协议逻辑。 |
| 工具目录 | 启动发现一次并按 `ttlMs` 缓存 | 保证首次模型请求已有工具 Schema，同时不强制重复请求。 |
| 新版能力范围 | 仅工具发现与调用 | 不将 MRTR、Tasks、订阅等独立能力混入生命周期兼容改动。 |
| 旧 HTTP+SSE | 不实现 | 该传输已弃用，且不属于当前 Streamable HTTP 升级目标。 |
