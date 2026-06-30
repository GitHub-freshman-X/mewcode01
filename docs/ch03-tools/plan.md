# MewCode 工具系统 Plan

## 架构概览

本章在第二章的分层架构上增加工具系统，并继续保持单向依赖：

```text
CLI 启动层
   │
   ├── 配置层
   │
   ├── 工具注册与工作区工具
   │        │
   │        ▼
   │   Tool Registry ──→ Tool Definitions
   │
   ├── Provider 工厂 ──→ Anthropic Adapter
   │        │              OpenAI Adapter
   │        ▼
   │   Provider 统一接口
   │
   ▼
TUI 层 ──→ 对话应用层 ──→ Provider 统一接口
                    │
                    ▼
              Tool Executor
```

- **工具层 `tools`**：定义统一 `Tool` 接口、工具元信息、JSON Schema、结构化结果、执行器和注册中心。六个核心工具都放在该层，默认限制在工作区根目录内。
- **Provider 层**：扩展统一消息、请求和流事件，使上层可以提供工具声明、接收工具调用分片、保存 assistant tool call，并把 tool result 作为后续历史发回模型。Anthropic 与 OpenAI 的工具格式差异只存在于各自 adapter。
- **对话应用层**：一次用户请求仍从 `Conversation.Start` 发起。流式事件到达时先累积 assistant 文本和 tool call；当模型完成且存在 tool call 时，对话层调用工具执行器，追加工具结果到历史，然后停止本轮，不自动再次请求模型。
- **TUI 层**：继续只消费 `Conversation` 的规范化状态。新增工具执行状态展示，包括工具名称、执行中、成功、失败和摘要；不解析供应商原始 SSE。
- **CLI 启动层**：确定工作区根目录，创建默认工具注册中心，把工具声明传给 `Conversation` 和 Provider 请求。

该设计覆盖 F1-F17：F1-F12 由 `tools` 层提供，F13/F17 由 Provider 事件扩展提供，F14/F15 由对话层编排提供，F16 由 TUI 状态呈现提供。

## 核心数据结构

### `tools.Tool`

```go
package tools

import "context"

type Tool interface {
    Metadata() Metadata
    Execute(ctx context.Context, input json.RawMessage) Result
}
```

每个工具自行完成参数解析和语义校验。`Execute` 不向外返回 Go error，而是始终返回 `Result`，保证工具失败也能结构化回灌给模型。

### `tools.Metadata`

```go
type Metadata struct {
    Name        string
    Description string
    Schema      Schema
}

type Schema map[string]any
```

`Schema` 保存 JSON Schema object，用于：

- 工具执行前的轻量校验。
- Provider adapter 生成 OpenAI / Anthropic 工具声明。
- 测试中断言工具参数契约。

### `tools.Result`

```go
type Result struct {
    ToolName string         `json:"tool_name"`
    Success  bool           `json:"success"`
    Data     map[string]any `json:"data,omitempty"`
    Error    *ToolError     `json:"error,omitempty"`
}

type ToolError struct {
    Type    ErrorType      `json:"type"`
    Message string         `json:"message"`
    Details map[string]any `json:"details,omitempty"`
}

type ErrorType string

const (
    ErrorValidation ErrorType = "validation_error"
    ErrorNotFound   ErrorType = "not_found"
    ErrorPermission ErrorType = "permission_error"
    ErrorConflict   ErrorType = "conflict"
    ErrorTimeout    ErrorType = "timeout"
    ErrorExecution  ErrorType = "execution_error"
    ErrorInternal   ErrorType = "internal_error"
)
```

所有工具结果都可 JSON 序列化。成功结果放入 `Data`；失败结果放入 `Error`，并保留 `ToolName` 与 `Success=false`。

### `tools.Registry`

```go
type Registry struct {
    tools map[string]Tool
}

func NewRegistry() *Registry
func (r *Registry) Register(tool Tool) error
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) List() []Tool
func (r *Registry) Definitions() []provider.ToolDefinition
```

`Register` 校验工具名非空、描述非空、schema 是 object，并拒绝重复名称。`Definitions` 输出 provider 层的中立工具定义，由 adapter 再转换为供应商格式。

### `tools.Executor`

```go
type Executor struct {
    Registry *Registry
    Timeout  time.Duration
}

func (e *Executor) Execute(ctx context.Context, call provider.ToolCall) provider.ToolResult
```

执行器负责：

- 查找工具名称。
- 为每次工具执行创建带超时的 context。
- 捕获 panic 并转成 `internal_error`。
- 把 `tools.Result` 编码为 `provider.ToolResult`。
- 对未知工具、超时和参数错误返回统一结构。

### `provider.ToolDefinition`

```go
type ToolDefinition struct {
    Name        string
    Description string
    Schema      map[string]any
}
```

`ChatRequest` 增加：

```go
type ChatRequest struct {
    Messages  []Message
    MaxTokens int
    Thinking  ThinkingOptions
    Tools     []ToolDefinition
}
```

### `provider.Message` 与内容块扩展

```go
const (
    BlockText       BlockType = "text"
    BlockThinking   BlockType = "thinking"
    BlockToolCall   BlockType = "tool_call"
    BlockToolResult BlockType = "tool_result"
)

type ContentBlock struct {
    Type      BlockType
    Text      string
    Signature string
    ToolCall  *ToolCall
    ToolResult *ToolResult
}

type ToolCall struct {
    ID        string
    Name      string
    Arguments json.RawMessage
}

type ToolResult struct {
    CallID   string
    Name     string
    Content  string
    IsError  bool
}
```

历史中保存两类工具相关块：

- assistant 消息中的 `BlockToolCall`：表示模型请求执行工具。
- user 消息中的 `BlockToolResult`：表示程序把工具结果返回给模型。

OpenAI adapter 将 `BlockToolResult` 转为 `function_call_output` input item；Anthropic adapter 将其转为 user message 中的 `tool_result` block。

### `provider.StreamEvent` 扩展

```go
const (
    EventStarted       EventType = "started"
    EventThinkingDelta EventType = "thinking_delta"
    EventTextDelta     EventType = "text_delta"
    EventSignatureDelta EventType = "signature_delta"
    EventToolCallStart EventType = "tool_call_start"
    EventToolCallDelta EventType = "tool_call_delta"
    EventToolCallDone  EventType = "tool_call_done"
    EventCompleted     EventType = "completed"
)

type StreamEvent struct {
    Type       EventType
    BlockIndex int
    Delta      string
    ToolCall   *ToolCallDelta
}

type ToolCallDelta struct {
    ID             string
    Name           string
    ArgumentsDelta string
    Arguments      string
}
```

Provider adapter 可以通过 delta 分片逐步发送参数，也可以在 done 事件中发送完整参数。`Conversation.Apply` 负责按 block index / call id 累积参数并在 done 时校验 JSON。

### `conversation.ChatOptions`

```go
type ChatOptions struct {
    MaxTokens int
    Thinking  provider.ThinkingOptions
    Tools     []provider.ToolDefinition
    Executor  *tools.Executor
}
```

`Conversation.Start` 会把 `Tools` 填入 `provider.ChatRequest`。当本轮完成且累积了工具调用时，`Conversation.Complete` 执行工具并追加历史。

### `conversation.Turn` 扩展

```go
const (
    TurnIdle          TurnState = "idle"
    TurnConnecting    TurnState = "connecting"
    TurnThinking      TurnState = "thinking"
    TurnGenerating    TurnState = "generating"
    TurnToolRequested TurnState = "tool_requested"
    TurnToolRunning   TurnState = "tool_running"
    TurnToolCompleted TurnState = "tool_completed"
    TurnCompleted     TurnState = "completed"
    TurnCancelled     TurnState = "cancelled"
    TurnFailed        TurnState = "failed"
)

type Turn struct {
    UserMessage      provider.Message
    AssistantMessage provider.Message
    ToolResults      []provider.ToolResult
    State            TurnState
    Err              error
}
```

`TurnCompleted` 仍表示本轮模型流完成；若存在工具调用，状态会短暂进入 `TurnToolRunning`，工具结果追加后进入 `TurnToolCompleted`。该状态允许 TUI 显示“工具已完成，可继续输入”。

## 模块设计

### `tools`

**职责：**

- 定义 `Tool`、`Metadata`、`Result` 和错误类型。
- 提供通用参数校验辅助、结果编码辅助、输出截断辅助。
- 提供 `Registry` 与 `Executor`。

**对外接口：**

```go
func NewRegistry() *Registry
func NewDefaultRegistry(root string) (*Registry, error)
func NewExecutor(registry *Registry, timeout time.Duration) *Executor
```

**依赖：** Go 标准库、`internal/provider`。

### `tools/workspace`

**职责：**

- 提供工作区路径解析与边界检查。
- 拒绝路径逃逸、空路径、目录路径和二进制文本读取。
- 统一文件大小、输出长度和结果数量限制。

**核心接口：**

```go
type Workspace struct {
    Root string
    MaxReadBytes int64
    MaxOutputBytes int
    MaxResults int
}

func (w Workspace) Resolve(rel string) (string, error)
func (w Workspace) ReadText(rel string) ([]byte, FileInfo, error)
func (w Workspace) WriteText(rel string, content string) (int, error)
```

**依赖：** Go 标准库。

### 六个核心工具

#### `read_file`

**参数：**

```json
{
  "path": "internal/provider/provider.go"
}
```

**成功结果：** `path`、`bytes`、`content`、`truncated`。

**失败：** 工作区外路径、文件不存在、目录、不可读、二进制、超过读取上限。

#### `write_file`

**参数：**

```json
{
  "path": "notes/example.txt",
  "content": "..."
}
```

**成功结果：** `path`、`bytes_written`。

**失败：** 工作区外路径、父目录不存在、路径是目录、不可写、内容过大。

#### `edit_file`

**参数：**

```json
{
  "path": "internal/example.go",
  "old_text": "before",
  "new_text": "after"
}
```

**行为：**

- 读取目标文本文件。
- 统计 `old_text` 出现次数。
- 恰好一次时替换并写回。
- 零次或多次时保持文件不变。

**成功结果：** `path`、`replacements`、`bytes_written`。

**失败：** `old_text` 为空、匹配次数为 0、匹配次数大于 1、文件读写错误。

#### `run_command`

**参数：**

```json
{
  "command": "go",
  "args": ["test", "./..."],
  "timeout_ms": 30000
}
```

**行为：**

- 使用独立 `command` 与 `args`，不通过 shell 拼接。
- 工作目录固定为 workspace root。
- `timeout_ms` 可选，但不得超过执行器全局上限。

**结果：** `exit_code`、`stdout`、`stderr`、`timed_out`、`duration_ms`。

#### `find_files`

**参数：**

```json
{
  "pattern": "**/*.go",
  "limit": 100
}
```

**行为：**

- 在 workspace root 内递归遍历。
- 支持 `*`、`?`、`**` 风格模式。
- 跳过 `.git` 和常见依赖目录。

**结果：** `matches`、`count`、`truncated`。

#### `search_code`

**参数：**

```json
{
  "pattern": "StreamEvent",
  "regex": false,
  "limit": 50
}
```

**行为：**

- 遍历工作区文本文件。
- `regex=false` 时按普通子串搜索。
- `regex=true` 时使用 Go regexp。
- 返回文件、行号、列号和片段。

**结果：** `matches`、`count`、`truncated`。

### `provider`

**职责：**

- 增加中立工具定义、工具调用和工具结果数据结构。
- 扩展 `ChatRequest` 和 `StreamEvent`。
- 保持 TUI/Conversation 不接触供应商原始 SSE。

**对外接口变化：**

```go
type Provider interface {
    Stream(context.Context, ChatRequest) (<-chan StreamEvent, <-chan error)
}
```

接口签名不变，只扩展 `ChatRequest` 与事件内容。

### `provider/anthropic`

**职责：**

- 把 `provider.ToolDefinition` 转为 Messages API 的 `tools` 数组：`name`、`description`、`input_schema`。
- 把 assistant `BlockToolCall` 转为 `tool_use` content block。
- 把 user `BlockToolResult` 转为 `tool_result` content block。
- 在流式解析中识别 `content_block_start` 的 `tool_use`，累积 `input_json_delta`，在 `content_block_stop` 或消息结束前发出 `EventToolCallDone`。

**关键映射：**

| 中立结构 | Anthropic Messages |
|----------|--------------------|
| `ToolDefinition` | `tools[].name/description/input_schema` |
| assistant `BlockToolCall` | `content[].type = "tool_use"` |
| user `BlockToolResult` | `content[].type = "tool_result"` |
| tool 参数分片 | `content_block_delta.delta.type = "input_json_delta"` |

### `provider/openai`

**职责：**

- 把 `provider.ToolDefinition` 转为 Responses API function tool。
- 把 assistant `BlockToolCall` 保存为 function call input item。
- 把 user `BlockToolResult` 转为 `function_call_output` input item。
- 在流式解析中识别 function call 参数 delta 与 done 事件，并发出中立工具事件。

**关键映射：**

| 中立结构 | OpenAI Responses |
|----------|------------------|
| `ToolDefinition` | `tools[].type = "function"` + name/description/parameters |
| assistant `BlockToolCall` | `type = "function_call"` |
| user `BlockToolResult` | `type = "function_call_output"` |
| tool 参数分片 | `response.function_call_arguments.delta` |
| tool 参数完成 | `response.function_call_arguments.done` |

### `conversation`

**职责：**

- 在 `Start` 中把工具定义传给 Provider。
- 在 `Apply` 中累积 tool call block。
- 在 `Complete` 中：
  1. 将本轮用户消息和 assistant 消息追加到历史。
  2. 如果 assistant 消息没有工具调用，结束本轮。
  3. 如果存在工具调用，调用 `tools.Executor` 执行。
  4. 生成一条 user tool result 消息并追加到历史。
  5. 设置状态为 `TurnToolCompleted`，不再自动调用 Provider。

**新增接口：**

```go
func (c *Conversation) Complete(ctx context.Context) error
func (c *Conversation) LastToolResults() []provider.ToolResult
```

`Complete` 增加 context，用于工具执行超时和取消。TUI 在收到 `EventCompleted` 后调用它。

### `tui`

**职责：**

- 渲染 assistant 中的 tool call 摘要。
- 渲染工具结果摘要。
- 状态栏区分 `tool_requested`、`tool_running`、`tool_completed`。
- 工具执行完成或失败后恢复输入。

**展示规则：**

- tool call：显示 `工具调用: <name>`，参数只显示截断摘要。
- tool result 成功：显示 `工具完成: <name>` 和简短结果。
- tool result 失败：显示 `工具失败: <name>` 和错误类型/消息。
- 完整 JSON 结果进入历史供模型使用，不在 TUI 中完整展开。

### `cmd/mewcode`

**职责：**

- 获取当前工作目录作为工具工作区根目录。
- 创建默认工具注册中心。
- 创建工具执行器，默认超时 `30s`。
- 把工具定义和执行器放入 `conversation.ChatOptions`。

后续如果需要配置工具开关或超时时间，再扩展配置层；本章先使用固定默认值。

## 模块交互

### 普通文本回复

```text
User submits input
  → TUI calls Conversation.Start
  → Conversation sends ChatRequest{Messages, Tools} to Provider
  → Provider streams text events
  → Conversation.Apply appends text blocks
  → Provider emits EventCompleted
  → Conversation.Complete commits user + assistant messages
  → TUI restores input
```

### 单次工具调用

```text
User submits input
  → Conversation.Start sends tools with model request
  → Provider emits text/tool call events
  → Conversation.Apply accumulates assistant text + tool call arguments
  → Provider emits EventCompleted
  → Conversation.Complete commits user + assistant(tool_call)
  → Conversation executes tool through Executor
  → Executor returns provider.ToolResult
  → Conversation appends user(tool_result) to history
  → TUI shows tool completed/failed and restores input
  → No automatic second model request in this chapter
```

### 错误路径

```text
Malformed tool JSON / unknown tool / validation failure / execution failure / timeout
  → Executor produces provider.ToolResult{IsError: true}
  → Conversation appends tool result to history
  → TUI shows failure summary
  → User may submit another message
```

## 文件组织

```text
mewcode01/
├── cmd/mewcode/
│   ├── main.go                    — 创建默认工具注册中心和执行器
│   └── main_test.go               — 启动组装测试
├── internal/conversation/
│   ├── conversation.go            — ChatOptions、Start、Complete、历史回灌
│   ├── stream.go                  — StreamEvent 应用与 tool call 累积
│   └── conversation_test.go       — 工具调用、回灌、单轮边界测试
├── internal/provider/
│   ├── event.go                   — 工具事件类型与错误模型
│   ├── message.go                 — ToolCall、ToolResult、内容块扩展
│   └── provider.go                — ChatRequest 增加工具定义
├── internal/provider/anthropic/
│   ├── request.go                 — Anthropic tool/tool_result 请求转换
│   ├── stream.go                  — Anthropic tool_use 流事件解析
│   └── anthropic_test.go          — Anthropic 工具请求与流解析测试
├── internal/provider/openai/
│   ├── request.go                 — OpenAI Responses function tool 请求转换
│   ├── stream.go                  — OpenAI function call 流事件解析
│   └── openai_test.go             — OpenAI 工具请求与流解析测试
├── internal/tools/
│   ├── tool.go                    — Tool、Metadata、Result、错误类型
│   ├── registry.go                — 注册中心
│   ├── executor.go                — 超时、panic 捕获、结果转换
│   ├── schema.go                  — Schema 校验辅助
│   ├── workspace.go               — 工作区路径与文本文件辅助
│   ├── read_file.go               — read_file
│   ├── write_file.go              — write_file
│   ├── edit_file.go               — edit_file
│   ├── run_command.go             — run_command
│   ├── find_files.go              — find_files
│   ├── search_code.go             — search_code
│   └── *_test.go                  — 工具层单元测试
└── internal/tui/
    ├── model.go                   — stream/tool 状态字段
    ├── update.go                  — EventCompleted 后执行工具
    ├── view.go                    — 工具调用与结果摘要渲染
    └── tui_test.go                — 状态展示测试
```

## 技术决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 工具结果错误处理 | `Tool.Execute` 总是返回结构化 `Result` | 满足 F6，避免工具失败破坏对话流程 |
| 工具参数 | 每个工具接收 `json.RawMessage` 并自行解析 | 保持统一接口，同时允许工具拥有不同参数结构 |
| Schema 校验 | 实现最小 JSON Schema 校验辅助，只覆盖本章工具所需类型、required、enum、数字范围 | 避免新增依赖；本章工具参数简单，可测试覆盖 |
| 工作区边界 | 所有文件类工具通过 `Workspace.Resolve` 做 clean + abs + rel 检查 | 满足 N1，集中避免路径逃逸 |
| 命令执行 | `command` + `args`，不走 shell 字符串 | 跨平台更可控，也减少 shell 注入风险 |
| 文件编辑 | 原文唯一匹配替换 | 与 spec 一致，失败时不修改文件，方便模型重试 |
| 输出限制 | 工具层统一截断大内容并标记 `truncated` | 满足 N2，保护上下文和 TUI |
| 工具调用历史 | assistant tool call 和 user tool result 都进入 `provider.Message` 历史 | Anthropic/OpenAI 都需要看到调用与结果的配对关系 |
| 单轮边界 | 工具结果回灌后停止，不自动再次请求模型 | 严格满足 F15，Agent Loop 留到后续章节 |
| Provider 抽象 | 上层只处理 `EventToolCall*` 和 `ToolResult`，adapter 负责供应商格式 | 满足 F17/N6，避免 TUI/Conversation 绑定 API 细节 |
| 默认超时 | 执行器默认 30 秒，命令工具可请求更短超时但不能超过上限 | 避免长命令挂死，同时给测试稳定边界 |
| 搜索实现 | 使用标准库遍历 + string/regexp 匹配，不依赖 `rg` 二进制 | 保证跨平台和测试环境一致 |

## Spec 覆盖检查

| Spec | Plan 归属 |
|------|-----------|
| F1 | `tools.Tool`、`Metadata`、`Result` |
| F2 | 六个核心工具模块 |
| F3 | `tools.Registry`、`Definitions` |
| F4 | `schema.go` 与各工具参数解析 |
| F5 | `tools.Executor` 超时 context |
| F6 | `tools.Result`、`ToolError`、panic 捕获 |
| F7 | `read_file`、`Workspace.ReadText` |
| F8 | `write_file`、`Workspace.WriteText` |
| F9 | `edit_file` 唯一匹配替换 |
| F10 | `run_command` |
| F11 | `find_files` |
| F12 | `search_code` |
| F13 | `provider.StreamEvent` 扩展与 adapter 流解析 |
| F14 | `Conversation.Complete` 工具执行与历史回灌 |
| F15 | 单轮工具边界设计 |
| F16 | TUI 工具状态和摘要渲染 |
| F17 | Provider 中立工具结构与 adapter 映射 |

