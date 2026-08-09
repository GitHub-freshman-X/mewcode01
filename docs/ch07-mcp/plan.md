# MewCode MCP Client Plan

## 架构概览

新增独立 `internal/mcp` 包，分为配置、协议会话、传输、发现管理与工具适配五层：

```text
已选定的 config.yaml ── MCP 配置加载 ─────────┐
                                            ▼
                                   Server Manager
                         ┌──────────┴──────────┐
                         ▼                     ▼
                 stdio Transport       Streamable HTTP Transport
                         └──────────┬──────────┘
                                    ▼
                         JSON-RPC Session
                 initialize → initialized → tools/list
                                    ▼
                     Remote Tool Adapter / Registry
                                    ▼
              现有 Permissions → Agent Scheduler → Executor
```

- `config` 负责完整主配置；`mcp_servers` 与 `protocol`、`model` 等字段同级，只从 CLI 选定的 `config.yaml` 读取。项目目录不提供 MCP 覆盖配置。
- `Server Manager` 在 CLI 创建内置工具注册中心后，独立初始化每个已合并 Server。每个 Server 有一份缓存连接；失败记录诊断并跳过，不影响其他 Server。
- `Transport` 只负责字节收发：stdio 管理子进程与 JSON 行；HTTP 管理 POST、headers、`MCP-Session-Id` 和关闭时 DELETE。
- `JSON-RPC Session` 负责唯一 ID、请求等待表、响应分派、协议错误和关闭传播；上层不感知具体传输。
- `Remote Tool Adapter` 将发现的 MCP 工具转换为现有 `tools.Tool`，名称为 `<server>__<tool>`，Safety 固定为 `SideEffect`。现有 Scheduler、权限和 Executor 无须为 MCP 特判。
- CLI 在 TUI 退出后关闭 Server Manager，确保进程与会话清理；运行期故障只返回该工具的结构化结果，不做恢复或重试。

## 核心数据结构

### config.MCPServerConfig

`internal/config` 为用户主配置增加 `MCPServers map[string]MCPServerConfig`。配置类型定义在 `config` 包，供 `mcp` 使用，以保持依赖单向：

```go
type MCPServerConfig struct {
    Type    MCPTransportType  `yaml:"type"`
    Command string            `yaml:"command,omitempty"`
    Args    []string          `yaml:"args,omitempty"`
    Env     map[string]string `yaml:"env,omitempty"`
    URL     string            `yaml:"url,omitempty"`
    Headers map[string]string `yaml:"headers,omitempty"`
}
```

```yaml
mcp_servers:
  filesystem:
    type: stdio
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "."]
    env:
      API_TOKEN: ${FILESYSTEM_TOKEN}
  issues:
    type: http
    url: https://mcp.example.com/mcp
    headers:
      Authorization: Bearer ${ISSUES_TOKEN}
```

MCP Server 配置由 `internal/config` 随主配置一起严格解析；`internal/mcp` 只消费已解析的 Server map 并展开环境变量。

### mcp.Transport 与 mcp.Session

`internal/mcp/protocol.go` 定义 JSON-RPC 2.0 请求、响应、错误和 notification 的最小类型；`Session` 持有原子递增 ID、并发安全的 pending map 与关闭状态：

```go
type Transport interface {
    Start(context.Context) error
    Send(context.Context, []byte) error
    Receive() <-chan Inbound
    Close(context.Context) error
}

type Session struct {
    transport Transport
    // nextID、pending、done、protocolVersion 等受同步保护
}

func (s *Session) Request(ctx context.Context, method string, params any, result any) error
func (s *Session) Notify(ctx context.Context, method string, params any) error
func (s *Session) Close(ctx context.Context) error
```

`Request` 将唯一 ID 写入 JSON-RPC 消息并等待对应 response；接收循环按 ID 投递，任何 JSON-RPC error、协议格式错误或 transport 关闭只结束关联等待者（关闭时才统一结束全部 pending）。`Notify` 用于 `notifications/initialized`，不创建 pending 项。

### mcp.StdioTransport 与 mcp.HTTPTransport

`StdioTransport` 管理 `exec.Cmd`、stdin writer、stdout scanner 和 stderr 安全摘要；`HTTPTransport` 管理 endpoint、配置 headers、协商后的协议版本和可选 `MCP-Session-Id`。POST 使用 JSON-RPC body，接受 JSON 或 SSE 响应；初始化后的请求附带协议版本与会话标识。收到已带会话标识的 HTTP 404 时返回会话失效错误，不恢复。

### mcp.Client

`Client` 封装每个已连接 Server 的标准三步操作：

```go
type Client struct {
    session *Session
}

func (c *Client) Initialize(ctx context.Context) error
func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error)
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error)
func (c *Client) Close(ctx context.Context) error
```

### mcp.Manager 与 mcp.RemoteToolAdapter

`Manager` 保存 `map[string]*Client` 与可关闭资源，按配置逐个建立 Client、初始化并发现工具；成功后才把该 Server 的 `RemoteTool` 交给适配层注册。每个 Server 的失败由 `Diagnostic{Server, Stage, Err}` 表示，供 CLI 输出安全摘要。

`RemoteToolAdapter` 实现既有 `tools.Tool`：名称为 `server + "__" + remote.Name`，Schema/描述来自远端，Safety 固定为 `tools.SafetySideEffect`，执行时调用关联 Client 的 `CallTool`，并把 MCP 内容映射为 `tools.Result`。

## 模块设计

### internal/config

**职责：** 定义并加载已选定主配置中的 `mcp_servers`，保持既有主配置校验规则。

**接口变化：** `Config` 新增 `MCPServers map[string]MCPServerConfig`；新增传输类型和 Server 声明类型。已选定主配置继续严格校验 Provider 必填字段；MCP Server 的结构校验在加载时完成，运行期变量缺失按 Server 产出诊断。

### internal/mcp/config.go

**职责：** 展开已经由主配置加载器读取和校验的 `mcp_servers` 值；不访问项目目录中的额外配置文件，也不执行跨文件合并。

**对外接口：**

```go
func ExpandServer(name string, raw config.MCPServerConfig, lookup func(string) (string, bool)) (config.MCPServerConfig, error)
```

**依赖：** `config` 与标准库字符串能力。

### internal/mcp/transport.go 与 stdio.go

**职责：** 定义 `Transport` 与入站消息事件；启动 stdio 子进程并使用 JSON Lines 收发 MCP 消息。

**接口：** `StdioTransport` 满足 `Transport`。stdout 在单独 goroutine 中持续读取和解码；写入串行化；进程退出、扫描或 JSON 错误关闭传输并唤醒关联请求。stderr 仅形成截断安全摘要，不进入协议通道。

### internal/mcp/http.go 与 sse.go

**职责：** 实现 Streamable HTTP POST、JSON 与 SSE response 解码、会话 header 和关闭 DELETE。

**接口：** `HTTPTransport` 满足 `Transport`。初始化后每个请求附加协商的 `MCP-Protocol-Version` 与可选 `MCP-Session-Id`；保留配置 headers。带 session 的 HTTP 404 映射为会话失效错误，不触发恢复。仅支持 Streamable HTTP，不实现旧 HTTP+SSE GET stream、endpoint discovery 或回退。

### internal/mcp/protocol.go 与 session.go

**职责：** 严格 JSON-RPC 编解码、唯一 ID、pending 路由、request/notification、远端错误映射和关闭语义。

**接口：** `Session.Request`、`Session.Notify` 与 `Session.Close`。`Client` 固定执行 `initialize → notifications/initialized → tools/list`；工具调用通过 `tools/call`。

### internal/mcp/client.go

**职责：** 根据 MCP 工具子协议完成初始化、工具发现与调用，记录协商协议版本，转换远端结果。

**接口：** `Initialize`、`ListTools`、`CallTool`、`Close`。只覆盖工具能力，不暴露资源、提示词、采样或其他 MCP API。

### internal/mcp/manager.go

**职责：** 按稳定 Server 名排序进行连接、发现、注册、诊断和关闭；缓存成功 Client。

**接口：**

```go
func NewManager(httpClient *http.Client, reporter Reporter) *Manager
func (m *Manager) ConnectAndRegister(ctx context.Context, registry *tools.Registry, servers map[string]config.MCPServerConfig) []Diagnostic
func (m *Manager) Close(ctx context.Context) error
```

仅在工具全部适配并可注册时将该 Server 提交到 Registry；一个 Server 的发现或注册冲突失败只影响该 Server。

### internal/mcp/tool.go

**职责：** 将 `RemoteTool` 映射为既有 `tools.Tool`。

**接口：** 适配器先用现有 `tools.Validate` 校验远端 Schema，再调用 Client；把 MCP `content`、`structuredContent`、`isError` 与 JSON-RPC/传输错误转换为 `tools.Success` 或 `tools.Failure`；始终使用 `SafetySideEffect` 与 `PermissionTargetNone`。

### cmd/mewcode/main.go

**职责：** 在内置 Registry 创建后发现 MCP Server，在 TUI 退出或启动后续步骤失败时关闭 Manager。

**交互：** 启动阶段诊断安全输出到 stderr；运行期远端调用沿用现有 Agent 事件和工具结果路径。

## 模块交互

启动调用链：

```text
load selected config → find project root
→ create built-in Registry → Manager.ConnectAndRegister
→ create Runner/TUI
→ TUI exits → Manager.Close
```

工具调用链保持现有路径：

```text
Agent Scheduler → Permission Engine → Executor
→ RemoteToolAdapter → Client.CallTool → Session.Request → Transport
→ tools.Result → ToolResult 回灌 Agent
```

## 文件组织

```text
internal/
├── config/
│   ├── config.go          # 增加主配置 mcp_servers 字段
│   ├── load.go            # 读取该字段
│   ├── validate.go        # 基础结构校验
│   └── config_test.go     # 主配置测试
├── mcp/
│   ├── config.go          # MCP 变量展开与诊断
│   ├── config_test.go
│   ├── protocol.go        # JSON-RPC 类型与解码
│   ├── session.go         # ID 与异步请求配对
│   ├── session_test.go
│   ├── transport.go       # 传输抽象
│   ├── stdio.go           # 子进程 JSONL 传输
│   ├── stdio_test.go
│   ├── http.go            # Streamable HTTP 与会话 header
│   ├── sse.go             # POST 响应 SSE 解析
│   ├── http_test.go
│   ├── client.go          # 初始化、发现、调用
│   ├── manager.go         # 多 Server 生命周期与注册
│   ├── manager_test.go
│   ├── tool.go            # 现有 Tool 适配层
│   └── tool_test.go
├── testutil/
│   └── mcpserver.go       # 受控 stdio/HTTP MCP 测试 Server
└── tools/
    └── registry_test.go   # 远端命名/冲突集成断言（如需独立覆盖）

cmd/mewcode/
├── main.go                # 启动时发现、退出时关闭
└── main_test.go           # 空配置、局部故障、关闭路径

config.example.yaml         # 补充 mcp_servers 示例与环境变量用法
docs/
├── README.md               # 登记 ch07 索引
└── ch07-mcp/
    ├── spec.md
    ├── plan.md
    ├── task.md
    └── checklist.md
```

## 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 协议实现 | 标准库自实现最小 JSON-RPC/MCP 工具子集 | 满足本章对 ID 配对与生命周期的要求，避免为未做的 MCP 能力引入完整 SDK。 |
| 协议版本 | 初始化声明并记录当前 MCP 协商版本；HTTP 后续请求带协商版本 header | 符合 Streamable HTTP 的版本协商要求。 |
| HTTP 响应 | 支持 JSON 与 POST 响应 SSE | 覆盖 Streamable HTTP 合规 Server，不支持旧式 HTTP+SSE 回退。 |
| 并发模型 | 每 Server 一个 Session 与 pending map；跨 Server 彼此独立 | 保证乱序回包正确关联，并隔离故障。 |
| MCP 配置来源 | 仅使用选定 `config.yaml` 的顶层 `mcp_servers` | 主配置来源唯一，避免项目目录中的隐式覆盖。 |
| 变量缺失 | 跳过该 Server、继续其他 Server | 符合故障隔离，同时避免空值连接。 |
| 远端名称 | 始终 `<server>__<tool>` | 与内置/其他 Server 稳定隔离。 |
| 权限 | 一律 `SafetySideEffect` | MCP 没有可信的通用安全分类，默认最小权限。 |
| 关闭 | stdio kill/wait；HTTP 带会话 header DELETE | 防止子进程泄漏，显式结束会话。 |
| 外部依赖 | 不新增运行时依赖 | Go 标准库足以支持进程、HTTP、JSON、SSE 行解析。 |
