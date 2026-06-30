# MewCode 纯对话基础 Plan

## 架构概览

MewCode 采用单向依赖的分层结构：

```text
CLI 启动层
   │
   ├── 配置层 ──→ Provider 工厂
   │                  │
   │          ┌───────┴────────┐
   │          ▼                ▼
   │    Anthropic Adapter  OpenAI Adapter
   │          └───────┬────────┘
   │                  ▼
   │             HTTP / SSE
   ▼
TUI 层 ──→ 对话应用层 ──→ Provider 统一接口
```

- **CLI 启动层**：解析 `--config`，确定默认配置路径，加载并校验配置，创建 Provider，最后启动全屏 TUI。启动失败时在普通终端输出脱敏错误，不进入 alternate screen。
- **配置层**：使用 `go.yaml.in/yaml/v4` 严格解析单文档 YAML，拒绝未知字段。负责默认值、字段校验、协议约束与密钥脱敏，不依赖 TUI 或 Provider 实现。
- **对话应用层**：拥有当前进程内的已完成消息历史，协调一次生成的开始、增量、完成、取消和失败。只有成功完成的模型回复会提交到历史；取消或失败的部分回复仅保留为界面记录。
- **Provider 层**：定义与供应商无关的请求、消息和流事件。Anthropic 与 OpenAI 适配器分别负责认证、请求 JSON、端点拼接、HTTP 错误转换和供应商 SSE 事件归一化。
- **SSE 基础层**：基于 `net/http` 与流式读取实现通用 SSE 帧解析，支持多行 `data`、空行分帧、EOF、不完整事件、未知字段和上下文取消；不理解具体供应商事件语义。
- **TUI 层**：使用 Bubble Tea v2 的 MVU 模型，组合 Bubbles v2 的 textarea 与 viewport。Provider 事件通过异步命令逐个送回 Update 循环，确保网络读取不阻塞键盘、滚动和 resize。TUI 负责显示，不解析供应商协议。
- **依赖方向**：启动层负责组装；TUI 依赖应用层抽象；应用层依赖 Provider 接口；Provider 适配器依赖 SSE 与 HTTP。配置层仅提供构造参数。各层保持无环依赖。

## 核心数据结构

### 配置模型

```go
type Config struct {
    Protocol  Protocol       `yaml:"protocol"`
    Model     string         `yaml:"model"`
    BaseURL   string         `yaml:"base_url"`
    APIKey    string         `yaml:"api_key"`
    MaxTokens int            `yaml:"max_tokens,omitempty"`
    Thinking  ThinkingConfig `yaml:"thinking,omitempty"`
}

type ThinkingConfig struct {
    Enabled      bool `yaml:"enabled"`
    BudgetTokens int  `yaml:"budget_tokens,omitempty"`
}

type Protocol string

const (
    ProtocolAnthropic Protocol = "anthropic"
    ProtocolOpenAI    Protocol = "openai"
)
```

`max_tokens` 默认值为 `4096`。thinking 仅允许 Anthropic 使用；开启时 `budget_tokens >= 1024` 且 `< max_tokens`。YAML 只允许一个文档，未知字段视为错误。配置错误只引用字段名，不回显字段值或 API Key。

配置示例：

```yaml
protocol: anthropic
model: claude-sonnet-4-6
base_url: https://api.anthropic.com
api_key: replace-with-your-key
max_tokens: 4096
thinking:
  enabled: true
  budget_tokens: 2048
```

### 统一消息模型

```go
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

type Message struct {
    Role   Role
    Blocks []ContentBlock
}

type BlockType string

const (
    BlockText     BlockType = "text"
    BlockThinking BlockType = "thinking"
)

type ContentBlock struct {
    Type      BlockType
    Text      string
    Signature string
}
```

`Signature` 不展示给用户。它用于保留 Claude thinking 返回的签名，并在后续轮次中将 thinking block 原样送回 Anthropic，以维持多轮协议连续性。OpenAI 适配器只转换文本块。

### Provider 请求与流事件

```go
type ChatRequest struct {
    Messages  []Message
    MaxTokens int
    Thinking  ThinkingOptions
}

type ThinkingOptions struct {
    Enabled      bool
    BudgetTokens int
}

type EventType string

const (
    EventStarted        EventType = "started"
    EventThinkingDelta  EventType = "thinking_delta"
    EventTextDelta      EventType = "text_delta"
    EventSignatureDelta EventType = "signature_delta"
    EventCompleted      EventType = "completed"
)

type StreamEvent struct {
    Type       EventType
    BlockIndex int
    Delta      string
}

type Provider interface {
    Stream(ctx context.Context, request ChatRequest) (<-chan StreamEvent, <-chan error)
}
```

每次调用创建一条事件流和一条终结错误流。Provider 是事件通道的唯一写入者；终结通道恰好发送一次结果，成功为 `nil`，失败为具体错误，取消为 `context.Canceled`。正常结束必须先产生一次已被消费者接收的 `EventCompleted`，再发送成功终结结果。Provider 负责关闭两个通道，并在发送时同时监听取消信号，避免消费者停止读取后泄漏 goroutine。

### 对话状态

```go
type TurnState string

const (
    TurnIdle       TurnState = "idle"
    TurnConnecting TurnState = "connecting"
    TurnThinking   TurnState = "thinking"
    TurnGenerating TurnState = "generating"
    TurnCompleted  TurnState = "completed"
    TurnCancelled  TurnState = "cancelled"
    TurnFailed     TurnState = "failed"
)

type Turn struct {
    UserMessage      Message
    AssistantMessage Message
    State            TurnState
    Err              error
}

type Conversation struct {
    provider Provider
    history  []Message
    active   *Turn
    cancel   context.CancelFunc
}
```

对外行为：

```go
func NewConversation(provider Provider, options ChatOptions) *Conversation
func (c *Conversation) Start(ctx context.Context, input string) (<-chan StreamEvent, <-chan error, error)
func (c *Conversation) Apply(event StreamEvent)
func (c *Conversation) Complete() error
func (c *Conversation) Fail(err error)
func (c *Conversation) Cancel()
func (c *Conversation) History() []Message
func (c *Conversation) ActiveTurn() *Turn
```

只有 `Complete` 会把本轮用户消息和完整 assistant 消息提交到 `history`。`Cancel` 与 `Fail` 保留当前 Turn 供界面显示，但不污染后续请求上下文。所有切片查询返回副本，避免 UI 修改内部历史。

### SSE 解码接口

```go
type SSEEvent struct {
    Event string
    Data  []byte
    ID    string
}

type Decoder interface {
    Next() (SSEEvent, error)
}
```

解码器按空行分帧，拼接多行 `data:`，忽略注释，接受 CRLF/LF，并为单帧设置 1 MiB 上限。未知业务事件交给 Provider 适配器安全忽略；不完整 JSON 返回带阶段信息的协议错误。

### 错误模型

```go
type ErrorStage string

const (
    StageConfig   ErrorStage = "config"
    StageRequest  ErrorStage = "request"
    StageResponse ErrorStage = "response"
    StageStream   ErrorStage = "stream"
)

type AppError struct {
    Stage      ErrorStage
    StatusCode int
    Message    string
    Cause      error
}
```

`AppError` 提供用户可理解的摘要与可追踪的底层原因，但面向用户的格式化结果不得包含 API Key、Authorization 或请求头。

## 模块设计

### `config`

**职责：**

- 使用系统标准配置目录计算 `mewcode/config.yaml` 的默认路径。
- 严格解析单文档 YAML、应用默认值并执行完整校验。
- 生成不包含密钥值的诊断错误。

**对外接口：**

```go
func DefaultPath() (string, error)
func Load(path string) (Config, error)
func Validate(cfg Config) error
```

**依赖：** Go 标准库、`go.yaml.in/yaml/v4`。

### `provider`

**职责：** 声明统一消息、请求、事件和 Provider 接口，不包含供应商判断、HTTP 或 TUI 逻辑。

**依赖：** Go 标准库。

### `provider/sse`

**职责：** 从流式响应体读取并解析 SSE 帧，处理 `event`、多行 `data`、`id`、注释、LF/CRLF、EOF 与帧大小限制，不解析 JSON，不理解供应商事件名称。

**对外接口：**

```go
func NewDecoder(r io.Reader, maxEventBytes int) Decoder
func (d *decoder) Next() (SSEEvent, error)
```

**依赖：** Go 标准库。

### `provider/anthropic`

**职责：**

- 将统一消息转换为 Anthropic Messages 请求。
- 向 `{base_url}/v1/messages` 发送 `POST`。
- 设置 `x-api-key`、`anthropic-version: 2023-06-01`、JSON 与 SSE 请求头。
- 请求包含 `model`、`messages`、`max_tokens`、`stream: true`；开启 thinking 时加入 `{type: enabled, budget_tokens: ...}`。
- 保持 assistant thinking block 与 signature 原样进入后续请求。
- 将 `message_start`、thinking/text/signature delta、`message_stop` 和 Anthropic `error` 归一化。
- 安全忽略不影响纯文本对话的已知生命周期事件与未知扩展事件。

**对外接口：**

```go
type Options struct {
    BaseURL    *url.URL
    APIKey     string
    Model      string
    HTTPClient *http.Client
}

func New(options Options) provider.Provider
```

### `provider/openai`

**职责：**

- 将本地完整历史转换为 Responses API 的 `input` 消息数组，不使用服务端 conversation 持久化。
- 向 `{base_url}/responses` 发送 `POST`。
- 设置 Bearer Authentication、JSON 与 SSE 请求头。
- 请求包含 `model`、`input`、`max_output_tokens`、`stream: true`。
- 将 `response.created`、`response.output_text.delta`、`response.completed`、失败、不完整和错误事件归一化。
- 忽略纯文本对话暂不消费的生命周期事件和未知扩展事件。

**对外接口：**

```go
type Options struct {
    BaseURL    *url.URL
    APIKey     string
    Model      string
    HTTPClient *http.Client
}

func New(options Options) provider.Provider
```

### `provider/factory`

**职责：** 根据已校验的协议创建正确适配器，把配置转换为 Provider 构造参数，并统一注入 HTTP Client。

**对外接口：**

```go
func New(cfg config.Config, client *http.Client) (provider.Provider, error)
```

**依赖：** `config`、`provider` 及两个适配器。其他模块不直接选择供应商。

### `conversation`

**职责：** 管理已完成历史和当前 Turn，生成不可变请求快照并启动 Provider 流，按 block index 聚合 thinking、signature 和文本增量，在完成时提交历史，在失败或取消时丢弃未完成上下文，并保证同一时间最多一个活动请求。

**依赖：** `provider`。

### `tui`

**职责：**

- 组合 viewport、textarea 与状态栏。
- 将 Provider 的事件通道和错误通道复用为 Bubble Tea 消息。
- 每消费一个流结果便安排下一次异步等待，网络读取不进入 `Update`。
- 根据 Turn 状态控制输入焦点、状态文字和按键行为。
- 渲染用户消息、回答、错误以及 thinking 折叠摘要。
- 处理窗口 resize、滚动、`Ctrl+T` 展开/折叠最近一条 thinking、`Ctrl+C` 取消或退出。

**关键内部消息：**

```go
type streamEventMsg struct {
    Event provider.StreamEvent
}

type streamErrorMsg struct {
    Err error
}

type streamClosedMsg struct{}
```

**依赖：** `conversation`、`provider`、Bubble Tea v2、Bubbles v2、Lip Gloss v2。

### `cmd/mewcode`

**职责：** 使用标准库 `flag` 只解析 `--config`，按“路径 → 配置 → HTTP Client → Provider → Conversation → TUI”的顺序组装程序；初始化失败时输出脱敏错误并返回非零退出码，不包含业务状态或协议转换。

### 测试支持

测试使用 `httptest.Server` 模拟两种 API，逐帧 flush SSE，并记录请求用于断言。TUI 的 `Update` 使用合成消息做确定性测试；自动化测试不访问真实模型或读取用户真实配置。

## 模块交互

### 启动流程

```text
main
  → 解析 --config
  → 确定配置路径
  → 严格加载并校验 YAML
  → 创建共享 HTTP Client
  → Provider Factory 创建对应适配器
  → 创建 Conversation
  → 创建 TUI Model
  → 进入 alternate screen
```

任一步骤失败都立即停止；错误在普通终端输出，API Key 必须经过脱敏检查。HTTP Client 不设置覆盖整个流生命周期的固定总超时，避免截断长回复；连接建立与响应头等待使用 Transport 级超时，用户取消由 context 控制。

### 正常对话流程

```text
用户按 Enter
  → TUI 校验输入非空并暂时禁用输入
  → Conversation.Start 创建活动 Turn
  → 复制“已完成历史 + 当前用户消息”
  → Provider 构造并发送 HTTP 请求
  → SSE Decoder 持续读取事件
  → Adapter 转换为统一 StreamEvent
  → Bubble Tea Cmd 等待一个事件
  → Update 调用 Conversation.Apply
  → View 增量重绘
  → 安排 Cmd 等待下一个事件
  → EventCompleted
  → Conversation.Complete 提交本轮历史
  → 输入区恢复焦点
```

### 历史提交规则

请求开始时：

```text
request.messages = committed history + current user message
```

正常完成后：

```text
committed history += current user message
committed history += complete assistant message
active turn = nil
```

失败或取消后：

```text
committed history 不变
当前部分回复只保留为 TUI 展示记录
active turn 标记 failed/cancelled
```

### Thinking 数据流

```text
Anthropic thinking_delta
  → EventThinkingDelta(block index)
  → 聚合到 assistant thinking block
  → TUI 实时显示 thinking 面板

Anthropic signature_delta
  → EventSignatureDelta(block index)
  → 聚合到同一个 thinking block
  → 不显示

message_stop
  → 完整 thinking block 连同 signature 提交历史
  → thinking 面板自动折叠
```

下一轮 Anthropic 请求会原样序列化已提交 assistant 消息中的 thinking block 与 signature。OpenAI 请求只提取历史中的可见文本块，不发送 Claude 专属 thinking 内容。`Ctrl+T` 展开或折叠最近一条包含 thinking 的消息；若没有 thinking，按键不产生状态变化。

### 滚动与流式刷新

- viewport 位于底部时，新增内容后自动跟随到底部。
- 用户主动向上滚动后，后续增量不强制拉回底部。
- 用户滚回底部后恢复自动跟随。
- resize 重新计算 viewport 与 textarea 尺寸，并保留尽可能接近原位置的滚动状态。
- textarea 在生成期间失焦并禁用提交，但 viewport 与取消快捷键仍可用。

### 取消流程

```text
生成中按 Ctrl+C
  → Conversation.Cancel
  → 调用当前请求 cancel function
  → HTTP body 读取解除阻塞
  → Provider 发送 context.Canceled 终结结果
  → TUI 将 Turn 标记为 cancelled
  → 不提交本轮历史
  → 输入区恢复
```

空闲时 `Ctrl+C` 直接返回 Bubble Tea 的退出命令。程序退出时再次取消活动 context，并关闭响应体，确保网络 goroutine 不遗留。

### 错误流程

- 非 2xx HTTP 响应：读取有限长度的错误体，转换为带状态码的 `AppError`。
- JSON/SSE 解析失败：标记为 stream 阶段错误。
- `401/403`：提示检查认证配置。
- `429`：提示限流，可显示服务端提供的等待信息，但本章不自动重试。
- `5xx`：提示供应商服务异常。
- 未知事件：忽略并继续；若流结束前没有成功完成事件，则按协议流不完整处理。
- 所有错误进入同一 TUI 错误展示路径，并在恢复输入后允许继续提问。

## 文件组织

```text
mew01/
├── cmd/
│   └── mewcode/
│       ├── main.go
│       └── main_test.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   ├── load.go
│   │   ├── validate.go
│   │   └── config_test.go
│   ├── provider/
│   │   ├── provider.go
│   │   ├── message.go
│   │   ├── event.go
│   │   ├── sse/
│   │   │   ├── decoder.go
│   │   │   └── decoder_test.go
│   │   ├── anthropic/
│   │   │   ├── client.go
│   │   │   ├── request.go
│   │   │   ├── stream.go
│   │   │   └── anthropic_test.go
│   │   ├── openai/
│   │   │   ├── client.go
│   │   │   ├── request.go
│   │   │   ├── stream.go
│   │   │   └── openai_test.go
│   │   └── factory/
│   │       ├── factory.go
│   │       └── factory_test.go
│   ├── conversation/
│   │   ├── conversation.go
│   │   ├── stream.go
│   │   └── conversation_test.go
│   ├── tui/
│   │   ├── model.go
│   │   ├── update.go
│   │   ├── commands.go
│   │   ├── view.go
│   │   ├── keymap.go
│   │   ├── styles.go
│   │   ├── run.go
│   │   └── tui_test.go
│   └── testutil/
│       └── streamserver.go
├── docs/
│   └── ch02-chat/
│       ├── spec.md
│       ├── plan.md
│       ├── task.md
│       └── checklist.md
├── .github/
│   └── workflows/
│       └── ci.yml
├── .gitignore
├── config.example.yaml
├── go.mod
├── go.sum
└── README.md
```

请求 JSON 类型保持在各 Provider 内，不暴露给 conversation 或 TUI。测试尽量与被测包同目录；跨 Provider 共用的流式测试服务单独复用。配置示例只使用假密钥；CI 不调用真实 API。

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| Go module | `github.com/GitHub-freshman-X/mewcode01` | 与现有 origin 一致 |
| Go 基线 | Go 1.25 | 在当前 Go 1.26 环境下兼顾支持周期与使用门槛 |
| TUI | `charm.land/bubbletea/v2` | MVU 适合异步流事件和确定性状态测试 |
| TUI 组件 | `charm.land/bubbles/v2` | 提供 Unicode textarea 与可滚动 viewport |
| 样式 | `charm.land/lipgloss/v2` | 与 Bubble Tea/Bubbles v2 配套，只使用基础样式 |
| YAML | `go.yaml.in/yaml/v4` | 支持已知字段与单文档严格校验 |
| API 接入 | `net/http` + 自有协议类型 | 精确控制地址、认证、取消、错误和 SSE |
| CLI 参数 | 标准库 `flag` | 首版只有 `--config`，无需引入命令框架 |
| 会话状态 | 仅内存、本地完整历史 | 满足本章范围，不依赖供应商服务端状态 |
| 并发模型 | 每个请求一个 Provider goroutine，TUI 单线程更新 | 网络读取可取消，界面状态无并发写入 |
| 输出上限 | 默认 4096，可由 YAML 覆盖 | 提供可用默认值并支持 thinking 预算约束 |
| Base URL | 保留已有路径前缀，再追加资源段 | OpenAI 的 `/v1` 与代理前缀不会被路径清理误删 |
| HTTP 超时 | 连接、TLS、响应头分别限时；流正文无总超时 | 防止连接永久卡住，同时允许长时间生成 |
| SSE 帧上限 | 1 MiB | 防止异常流无限占用内存，足够容纳文本事件 |
| 未知事件 | 忽略并继续；缺少完成事件则报错 | 兼容协议扩展，同时识别截断流 |
| 密钥保护 | 不记录请求头；用户界面只显示安全摘要 | 底层错误不直接渲染 |
| 依赖版本 | 实现时选择兼容的稳定 v2 版本并锁入 `go.mod/go.sum` | 避免浮动依赖，不在设计期虚构补丁版本 |
| 自动检查 | `gofmt`、`go vet`、`go test`、三平台 build | 覆盖格式、静态问题、行为与跨平台编译 |
| Race 检查 | Linux CI 运行 `go test -race ./...` | 检查流 goroutine、取消与通道生命周期 |
| 真实 API | 自动测试禁止访问 | 测试可重复且不消耗密钥与额度 |

## Spec 覆盖

| Spec | 设计归属 |
|---|---|
| F1–F2 | `config`、`cmd/mewcode` |
| F3–F4 | `tui`、异步流 Cmd、Provider 流 |
| F5 | `conversation` 的历史提交规则 |
| F6 | `provider` 抽象与 `factory` |
| F7 | `provider/anthropic`、`provider/sse` |
| F8 | `provider/openai`、`provider/sse` |
| F9 | thinking block、signature、Anthropic 映射、TUI 折叠状态 |
| F10 | `AppError`、Provider 错误转换、TUI 状态机 |
| F11 | context 取消、Conversation 提交边界、按键映射 |
| F12 | viewport、textarea、resize 与 thinking 操作 |
| N1–N3 | v2 TUI 栈、HTTP 生命周期、三平台 CI |
| N4、N9 | 安全错误摘要、密钥不渲染、文档警告 |
| N5–N7 | 单向分层、接口隔离、mock HTTP 测试 |
| N8 | Unicode 组件与 UTF-8 流测试 |
