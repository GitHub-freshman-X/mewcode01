# MewCode 应用日志 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供本地、安全的结构化日志模块，并记录 MCP Server 的完整发现、注册、调用和关闭生命周期。

**Architecture:** 新增 `internal/logging`，由 `main` 在项目根目录下初始化一个按运行实例分文件的 JSON Lines 记录器，并在初始化失败时退化为 no-op 记录器。MCP Manager、Client 和远端工具适配器接收同一个带 Server 字段的记录器；它们只提交经过设计的状态字段，绝不提交请求、响应、参数或原始错误文本。

**Tech Stack:** Go 标准库（`encoding/json`、`os`、`sync`、`time`），现有 Go 测试与 race detector。

## Global Constraints

- 日志仅写到 `<项目根>/logs/`，不增加第三方依赖、不上传日志。
- 每次运行创建独立 `.jsonl` 文件；单条事件是单行 JSON。
- 事件字段必须包含 `time`、`level`、`source` 和 `message`。
- MCP 日志字段仅允许状态、阶段、Server 名、工具名、传输类型和数量；禁止记录密钥、headers、环境变量值、URL、工具参数、文件内容、工具结果与原始错误文本。
- 日志失败必须退化，不得改变 MewCode、MCP 或退出路径的结果。
- 本章只接入 MCP；后续模块复用 `internal/logging`，不在本章扩展 Agent、Provider、权限或内置工具的历史路径。

---

## 文件组织

```text
internal/logging/
├── logger.go        # JSONL 文件创建、事件序列化、并发写入、WithFields 和 no-op
└── logger_test.go   # 文件、字段、并发、关闭与降级测试
internal/mcp/
├── manager.go       # Server 生命周期、工具发现与注册事件
├── client.go        # 初始化、初始化通知、tools/list 与 tools/call 状态事件
├── tool.go          # 远端工具调用完成状态事件
└── *_test.go        # MCP 日志事件断言
cmd/mewcode/
├── main.go          # 项目根确定后初始化与关闭记录器，并注入 Manager
└── main_test.go     # 日志初始化失败不阻断启动的回归覆盖
docs/ch00/01-logging/
├── spec.md
├── plan.md
├── task.md
└── checklist.md
```

## 核心接口

### `internal/logging`

```go
package logging

type Fields map[string]any

type Event struct {
    Time      time.Time `json:"time"`
    Level     string    `json:"level"`
    Source    string    `json:"source,omitempty"`
    Message   string    `json:"message"`
    Fields    Fields    `json:"fields,omitempty"`
}

type Logger struct { /* writer, mutex, base fields and close state */ }

func New(root string, now func() time.Time, pid int) (*Logger, error)
func Nop() *Logger
func (l *Logger) WithFields(fields Fields) *Logger
func (l *Logger) Info(message string, fields Fields)
func (l *Logger) Error(message string, fields Fields)
func (l *Logger) Close() error
```

`New` 使用 `logs/mewcode-YYYYMMDDTHHMMSS.<nanoseconds>-<pid>.jsonl` 创建文件；`WithFields` 复制并合并基础字段，使 `server` 能在 MCP 子对象间安全复用。`Info` 和 `Error` 只接受调用方显式提供的安全字段，写入失败被吞掉；`Nop` 具有相同接口且不写入。

### MCP 构造函数变化

```go
func NewManager(httpClient *http.Client, reporter Reporter, logger *logging.Logger) *Manager
func NewClient(transport Transport, logger *logging.Logger) *Client
func NewRemoteToolAdapter(server string, remote RemoteTool, client *Client, logger *logging.Logger) *RemoteToolAdapter
```

Manager 为每个 Server 创建 `logger.WithFields(logging.Fields{"server": name})`。Client 和 Adapter 不保留或输出任何远端原始载荷；它们只记录预定义事件及安全字段。

## MCP 事件约定

| 事件 | 产生位置 | 必填安全字段 |
|---|---|---|
| `server_configuration_started` / `server_configuration_failed` | Manager | `server`、`stage=configuration`、`status` |
| `server_connect_started` / `server_connected` / `server_connect_failed` | Manager | `server`、`transport`、`stage=connect`、`status` |
| `initialize_started` / `initialize_succeeded` / `initialize_failed` | Client | `server`、`stage=initialize`、`status` |
| `initialized_notification_sent` / `initialized_notification_failed` | Client | `server`、`stage=initialize`、`status` |
| `tool_discovery_started` / `tool_discovery_succeeded` / `tool_discovery_failed` | Client/Manager | `server`、`stage=discover`、`status`、可选 `tool_count` |
| `tool_registered` / `tool_registration_failed` | Manager | `server`、`stage=register`、`status`、`remote_tool`、`tool` |
| `tool_call_started` / `tool_call_succeeded` / `tool_call_failed` | Adapter/Client | `server`、`tool`、`remote_tool`、`stage=call`、`status` |
| `server_close_started` / `server_closed` / `server_close_failed` | Manager | `server`、`stage=close`、`status` |

错误事件使用固定的 `status` 值（例如 `configuration_failed`、`connect_failed`、`rpc_failed`、`tool_error`），不写入 `err.Error()`。现有 stderr 诊断继续保持原状。

## 模块交互

```text
main: root → logging.New
  ├─ 成功：File Logger
  └─ 失败：stderr 诊断 + logging.Nop
       ↓
mcp.NewManager(..., logger)
       ↓
Manager.WithFields(server)
  → Stdio/HTTP Transport → Client.Initialize / ListTools
  → RemoteToolAdapter.Execute → Client.CallTool
       ↓
Logger（mutex）→ JSON marshal → 单行追加到 logs/*.jsonl
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 输出格式 | JSON Lines | 每行独立可解析，既可人工搜索也可被后续工具处理。 |
| 日志生命周期 | 每次运行一个文件 | 避免多个 MewCode 进程互相覆盖，并免除本章的轮转复杂度。 |
| 脱敏策略 | 调用点白名单字段，不记录原始错误 | 通用正则无法可靠识别所有 token、URL 或文件内容；最小字段集合更可证明安全。 |
| 日志故障行为 | 初始化失败转 no-op，写入失败吞掉 | 可观测性不得改变 Agent 或 MCP 行为。 |
| 并发写入 | Logger 内单 mutex 覆盖 JSON 编码和一次写入 | 让每条记录保持完整，且满足 MCP 并发调用场景。 |
| 依赖注入 | 从 main 向 MCP 显式传递 Logger | 避免包级全局状态，测试能使用临时目录与 no-op/文件记录器。 |

## 规格覆盖自检

| Spec 要求 | 实现归属 |
|---|---|
| F1、F2 | `internal/logging/logger.go` 的文件命名、事件结构与 JSONL 写入 |
| F3、N1、N2 | `Logger`/`Nop`、mutex、`main` 的降级初始化 |
| F4、N3 | `Manager` 与 `Client` 的启动、发现、注册和关闭事件 |
| F5 | `RemoteToolAdapter` 与 `Client` 的调用状态事件 |
| F6 | 全部 MCP 调用点的白名单字段约定与脱敏回归测试 |
| F7、N4、N5 | 独立内部包、MCP 显式注入和仅根目录输出 |
