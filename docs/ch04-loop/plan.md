# MewCode Agent Loop Plan

## 架构概览

```text
TUI
 │  提交普通任务、/plan、/do
 ▼
Agent Runner ───────────────→ 规范化 Agent Event 流 ──→ TUI
 │
 ├── Loop Controller
 │    ├── 调用 Provider
 │    ├── 判断停止条件
 │    └── 提交完整轮次
 │
 ├── Stream Collector
 │    ├── 实时转发文本与用量事件
 │    └── 累积完整模型响应
 │
 ├── Tool Scheduler
 │    ├── 连续只读工具并发
 │    └── 副作用工具按序串行
 │
 ▼
Conversation Session
 ├── 模型上下文历史
 ├── 界面展示记录
 └── 当前会话有序待执行计划列表
```

- **`agent` 层**：新增独立编排层，拥有 Agent Loop、停止策略、事件流、流式收集器和工具调度器。它直接依赖统一 Provider、工具注册中心和会话状态，不依赖 TUI。
- **`conversation` 层**：从“单轮模型与工具编排器”收敛为会话状态容器，分别保存模型上下文历史、界面展示记录和有序待执行计划列表。普通执行的完整轮次同时进入模型历史与展示记录；Plan Mode 仅把最终计划写入展示记录与待执行列表。半轮响应不会提交，计划追加与消费均为原子操作。
- **`provider` 层**：继续屏蔽供应商差异，并补充规范化 Token 用量事件。Agent 不接触 Anthropic/OpenAI 原始流格式。
- **`tools` 层**：工具元信息增加安全分类。调度器使用分类生成有序批次，实际超时、panic 捕获及结构化结果仍由现有执行器负责。
- **`tui` 层**：识别普通输入、`/plan <任务>` 和 `/do`，提交 Agent 请求后只消费事件并更新视图；不再调用会话层推进模型或执行工具。
- **`config` 与启动层**：增加 Agent 最大迭代数配置，默认 20；启动时组装 Session、Agent、Provider、Registry 与 Executor。

需求归属：F1–F6 由 Runner 与 Session 覆盖，F7–F8/F12 由事件流和 Collector 覆盖，F9–F11 由 Scheduler 与 tools 覆盖，F13–F16 由模式请求、Runner 和 Session 计划状态覆盖，F17 由 TUI 单向消费覆盖。

## 核心数据结构

### `agent.Request` 与模式

```go
type Mode string

const (
    ModeAct  Mode = "act"
    ModePlan Mode = "plan"
    ModeDo   Mode = "do"
)

type Request struct {
    Mode   Mode
    Prompt string
}
```

- 普通输入使用 `ModeAct`。
- `/plan <任务>` 使用 `ModePlan`，`Prompt` 为任务内容。
- `/do` 使用 `ModeDo`，由 Runner 从 Session 读取全部待执行计划；此时 `Prompt` 必须为空。

内部准备结果保留 `/do` 启动时取得的计划快照：

```go
type preparedRequest struct {
    prompt   string
    registry *tools.Registry
    plans    []string
}
```

`plans` 仅在 `ModeDo` 使用。Runner 在正常完成后以该快照为依据消费计划，避免清除未包含在本次请求中的新计划。

### `agent.Options`

```go
const (
    DefaultMaxIterations     = 20
    UnknownToolStopThreshold = 3
)

type Options struct {
    MaxIterations int
}
```

`MaxIterations <= 0` 时使用 20。未知工具阈值固定为 3，不暴露配置。

### `agent.Runner` 与任务句柄

```go
type Runner struct {
    provider provider.Provider
    session  *conversation.Session
    registry *tools.Registry
    executor *tools.Executor
    options  Options
}

func NewRunner(
    p provider.Provider,
    session *conversation.Session,
    registry *tools.Registry,
    executor *tools.Executor,
    options Options,
) *Runner

func (r *Runner) Start(ctx context.Context, req Request) (*Task, error)

type Task struct {
    Events <-chan Event
    Cancel context.CancelFunc
}
```

`Start` 校验请求、计划状态和忙碌状态后启动后台循环。同一 Runner 同时只允许一个活动任务。调用方停止消费事件前必须调用 `Cancel`。

### `agent.Event`

```go
type EventType string

const (
    EventProgress   EventType = "progress"
    EventTextDelta  EventType = "text_delta"
    EventToolCall   EventType = "tool_call"
    EventToolResult EventType = "tool_result"
    EventUsage      EventType = "usage"
    EventCompleted  EventType = "completed"
    EventStopped    EventType = "stopped"
    EventCancelled  EventType = "cancelled"
    EventFailed     EventType = "failed"
)

type Phase string

const (
    PhaseCallingModel Phase = "calling_model"
    PhaseStreaming    Phase = "streaming"
    PhaseRunningTools Phase = "running_tools"
    PhaseFinishing    Phase = "finishing"
)

type Event struct {
    Type       EventType
    Iteration  int
    Phase      Phase
    Text       string
    ToolCall   *provider.ToolCall
    ToolResult *provider.ToolResult
    Usage      *provider.Usage
    Summary    *Summary
    Err        error
}
```

每个事件只填充与其类型相关的字段。事件通道由 Runner 关闭，并保证恰好发出一个终态事件。

### `agent.Summary` 与停止原因

```go
type StopReason string

const (
    StopFinalAnswer      StopReason = "final_answer"
    StopIterationLimit   StopReason = "iteration_limit"
    StopUnknownToolLimit StopReason = "unknown_tool_limit"
    StopCancelled        StopReason = "cancelled"
    StopStreamError      StopReason = "stream_error"
)

type Summary struct {
    Reason     StopReason
    Iterations int
    Usage      provider.Usage
    Partial    bool
}
```

终态映射：

- 最终回答：`EventCompleted + StopFinalAnswer`
- 迭代上限或未知工具阈值：`EventStopped`
- 用户取消：`EventCancelled`
- 流读取或解析错误：`EventFailed + StopStreamError`

只有 `StopFinalAnswer` 可将 Plan Mode 输出追加为有效计划；只有 `ModeDo + StopFinalAnswer` 可消费本次执行的计划快照。

### `provider.Usage`

```go
type Usage struct {
    InputTokens  int
    OutputTokens int
}

func (u *Usage) Add(other Usage)
```

`provider.StreamEvent` 增加：

```go
const EventUsage EventType = "usage"

type StreamEvent struct {
    // 现有字段保持不变
    Usage *Usage
}
```

每次 Provider 请求最多发出一个规范化用量事件。Adapter 负责把供应商可能分散在多个原始事件中的用量合并后再上报。

### 完整轮次收集结果

```go
type roundResult struct {
    Assistant provider.Message
    ToolCalls []provider.ToolCall
    Usage     provider.Usage
}

func collectRound(
    ctx context.Context,
    iteration int,
    events <-chan provider.StreamEvent,
    done <-chan error,
    emit func(Event) bool,
) (roundResult, error)
```

`collectRound` 同时执行两条路径：

- 收到文本分片时立即调用 `emit`；
- 将文本、Thinking、工具调用分片和 Usage 累积为 `roundResult`。

只有收到完整结束信号且调用数据有效时才返回成功。

### `conversation.Session`

```go
type Session struct {
    history      []provider.Message // 仅供 Provider 请求
    display      []provider.Message // 仅供 TUI 展示
    pendingPlans []string
}

func NewSession() *Session
func (s *Session) Snapshot() []provider.Message
func (s *Session) DisplaySnapshot() []provider.Message
func BuildRound(
    user *provider.Message,
    assistant provider.Message,
    results []provider.ToolResult,
) ([]provider.Message, error)
func (s *Session) CommitRound(
    user *provider.Message,
    assistant provider.Message,
    results []provider.ToolResult,
) error
func (s *Session) CommitPlan(
    user provider.Message,
    assistant provider.Message,
    plan string,
) error
func (s *Session) PendingPlans() []string
func (s *Session) ConsumePlans(plans []string) error
```

- `Snapshot` 返回模型上下文历史的深拷贝；`DisplaySnapshot` 返回展示记录的深拷贝。
- `BuildRound` 是纯函数，校验 user、assistant 与全部 tool result 的对应关系并返回克隆后的完整轮次消息；Runner 用它构建 Plan Mode 临时历史。
- `CommitRound` 调用 `BuildRound`，在同一临界区把普通执行轮次同时追加到模型历史和展示记录。
- 第一轮传入 user 消息，后续自动循环轮次传入 `nil`。
- `CommitPlan` 仅把用户可见的原始 `/plan` 请求与最终 assistant 计划写入展示记录，并原子追加非空计划；不得修改模型上下文历史。
- `PendingPlans` 返回按追加顺序排列的独立切片副本。
- `ConsumePlans` 校验参数与当前列表前缀完全一致后，仅移除该前缀；若执行期间出现新的尾部计划，它们不会被清除。
- 空消费、顺序不一致或计划内容不一致均返回错误且不改变列表。

### Plan Mode 任务临时历史

```go
type taskHistory struct {
    messages []provider.Message
}

func newTaskHistory(base []provider.Message) *taskHistory
func (h *taskHistory) Snapshot() []provider.Message
func (h *taskHistory) CommitRound(
    user *provider.Message,
    assistant provider.Message,
    results []provider.ToolResult,
) error
```

`taskHistory` 仅存在于单次 Runner goroutine 内，无需互斥锁。它以 `Session.Snapshot()` 的普通模型历史为初始上下文，通过 `conversation.BuildRound` 追加规划轮次；任务结束后整体释放。

### 工具安全分类

```go
type Safety string

const (
    SafetyReadOnly   Safety = "read_only"
    SafetySideEffect Safety = "side_effect"
)

type Metadata struct {
    Name        string
    Description string
    Schema      Schema
    Safety      Safety
}
```

空值和未知值统一视为 `SafetySideEffect`。

注册中心增加：

```go
func (r *Registry) FilterBySafety(safety Safety) *Registry
```

Plan Mode 使用只读 Registry 副本生成工具声明和执行范围，避免隐藏的副作用工具被模型按名称绕过。

执行器调整为接受本次任务的工具范围：

```go
type Executor struct {
    Timeout time.Duration
}

func (e *Executor) Execute(
    ctx context.Context,
    registry *Registry,
    call provider.ToolCall,
) provider.ToolResult
```

### `agent.Scheduler`

```go
type Scheduler struct {
    registry *tools.Registry
    executor *tools.Executor
}

func NewScheduler(
    registry *tools.Registry,
    executor *tools.Executor,
) *Scheduler

func (s *Scheduler) Execute(
    ctx context.Context,
    calls []provider.ToolCall,
    emit func(Event) bool,
) ([]provider.ToolResult, error)
```

Scheduler 按原始顺序构造最大连续只读批次；每个副作用或未知工具形成单独批次。并发批次等待全部完成后，按原始顺序发出结果事件并返回结果。

## 模块设计

### `internal/agent/runner.go`

**职责：**

- 校验任务模式、忙碌状态及 `/do` 的计划前置条件。
- 为每个任务创建可取消上下文和事件通道。
- 选择完整工具范围或 Plan Mode 只读范围。
- 驱动模型调用、收集、工具执行、历史提交和下一轮循环。
- 维护迭代数、累计 Token 和连续未知工具数。
- 保证只产生一个终态事件并释放活动任务状态。
- Plan Mode 每轮从 `taskHistory` 构造请求并只提交到该临时历史；普通 Act/Do 仍从 Session 模型历史构造请求并调用 `CommitRound`。
- 在 Plan Mode 正常完成时调用 `CommitPlan`，仅记录用户可见结果并追加计划；在 Do Mode 正常完成时消费本次执行的计划快照，其他终态不改变计划列表。

**核心循环：**

1. 构造首轮 user 消息和 Provider 请求。
2. 发出轮次进度，调用 Provider。
3. 由 Collector 收集完整响应并实时转发事件。
4. 流失败或取消时停止，不提交本轮。
5. 无工具调用时原子提交本轮并正常完成。
6. 有工具调用时交给 Scheduler 执行。
7. 工具全部处理后原子提交 assistant 调用与结果。
8. 更新未知工具计数，检查停止条件。
9. 未停止则进入下一轮，且不重复添加首轮 user 消息。

### `internal/agent/collector.go`

**职责：**

- 将 Provider 流事件转为 Agent 事件。
- 累积 Thinking、文本、工具参数分片和 Token 用量。
- 校验工具调用标识、名称及完整 JSON。
- 区分完整响应、取消和流错误。
- 返回可提交的 `roundResult`。

文本增量立即发出；Thinking 继续累积在 assistant 消息中，但不新增公开 Agent 事件类型，TUI 可通过进度与最终会话快照沿用当前折叠展示。

### `internal/agent/scheduler.go`

**职责：**

- 查询工具安全分类并构建有序批次。
- 批次启动前按原始顺序发出工具调用事件。
- 只读批次为每个调用启动独立执行任务并等待全部完成。
- 副作用和未知工具逐个执行。
- 并发结果统一按原始调用顺序发出和返回。
- 上下文取消后停止创建新执行任务。

已知工具执行失败仍是正常的结构化 `ToolResult`；只有取消或调度器自身错误才返回 Go error。

### `internal/agent/mode.go`

**职责：**

- 将普通输入直接构造为执行任务。
- 为 `/plan` 构造“只读探索并输出可执行计划”的任务说明。
- 为 `/do` 读取全部待执行计划，按顺序编号并构造“综合执行以下全部计划”的任务说明。
- 将 `/do` 使用的计划快照交给 Runner，在正常完成后精确消费。
- 从正常完成的 Plan Mode 最终响应中拼接非空文本并追加计划。

Plan Mode 同时通过提示约束和只读 Registry 限制副作用；即使模型猜出写工具名称，也只能得到未知工具结果，不能实际执行。

### `internal/conversation/session.go`

**职责：**

- 线程安全地分离模型上下文历史、界面展示记录和有序待执行计划列表。
- 为 Provider 请求与 TUI 分别提供深拷贝快照。
- 原子提交普通完整轮次，或原子提交仅供展示的最终计划与待执行项。
- 提供纯轮次构造能力，供 Plan Mode 临时历史复用校验逻辑。
- 快照读取与按前缀消费进程内计划。

规划探索轮次永不进入 Session；成功规划仅把原始用户请求与最终计划放入展示记录，并追加待执行计划。失败或取消规划不提交任何 Session 状态。成功 `/do` 仅消费本次提交的计划快照；其他终态保留计划。

### `internal/provider`

**职责变化：**

- `event.go` 增加规范化 Usage。
- Anthropic Adapter 合并输入与输出用量后，每次请求发出一次 Usage。
- OpenAI Adapter 从完成响应中提取 Usage。
- Adapter 仍负责供应商事件到统一流事件的转换，Agent 不接触原始协议。

若供应商未提供 Usage，该轮不发 Usage 事件，累计值保持不变，不把缺失误报为错误。

### `internal/tools`

**职责变化：**

- 六个内置工具在 Metadata 中声明安全分类。
- Registry 可生成按安全分类过滤的独立副本。
- Executor 的工具查找基于调用时传入的 Registry，使 Plan Mode 的执行范围与模型可见范围一致。
- 原有超时、panic 捕获、参数校验及结构化错误语义保持不变。

安全分类：

| 工具 | 分类 |
|---|---|
| `read_file` | 只读 |
| `find_files` | 只读 |
| `search_code` | 只读 |
| `write_file` | 副作用 |
| `edit_file` | 副作用 |
| `run_command` | 副作用 |

### `internal/tui`

**职责变化：**

- 解析普通输入、`/plan <任务>` 和精确匹配的 `/do`。
- 保存当前 Task 句柄，持续等待 Agent 事件。
- 根据事件维护当前任务的临时展示状态。
- 从 `DisplaySnapshot` 渲染已完成内容，不使用 Provider 的模型上下文快照。
- 收到终态后恢复输入焦点并清除 Task 句柄。
- Ctrl+C 在任务运行时调用 `Task.Cancel`，随后继续消费直到终态和通道关闭。
- 完整历史从 Session 展示；取消或失败的部分输出仅保留在 TUI 临时记录中并标注状态。

TUI 不调用 Provider、Executor、Collector 或 Scheduler。

### `internal/config` 与 `cmd/mewcode`

配置增加：

```go
type AgentConfig struct {
    MaxIterations int `yaml:"max_iterations,omitempty"`
}

type Config struct {
    // 现有字段
    Agent AgentConfig `yaml:"agent,omitempty"`
}
```

启动层创建默认 Registry、Executor、Session 和 Runner，再把 Runner 与 Session 注入 TUI。示例配置增加：

```yaml
agent:
  max_iterations: 20
```

## 模块交互

### 普通 Agent Loop

```text
用户输入
  │
  ▼
TUI ── Start(ModeAct) ──→ Runner
                            │
                            ├─ Session.Snapshot()
                            ├─ Provider.Stream()
                            │      │
                            │      ▼
                            │   Collector
                            │    ├─ 文本增量 ──→ Event ──→ TUI
                            │    └─ 完整响应 ──→ Runner
                            │
                            ├─ 无工具调用
                            │    ├─ Session.CommitRound()
                            │    └─ EventCompleted
                            │
                            └─ 有工具调用
                                 ├─ Scheduler.Execute()
                                 ├─ Session.CommitRound()
                                 ├─ 检查停止条件
                                 └─ 进入下一轮 Provider.Stream()
```

一次“迭代”严格定义为一次 Provider 请求。每次请求发出前递增迭代计数，因此不会出现第 21 次请求。

### 单轮收集顺序

1. Runner 发出 `PhaseCallingModel`。
2. Provider 返回事件流后发出 `PhaseStreaming`。
3. Collector 实时转发文本增量，并在内部累积所有内容块。
4. Usage 到达时立即发出该轮用量事件并加入任务累计值。
5. Collector 必须同时确认模型完成事件和流结束无错误，才返回完整轮次。
6. 任一工具调用缺少 ID、名称或合法 JSON 时，视为流解析失败。
7. 失败或取消时，累积内容不交给 Session。

### 多工具调度

以模型返回以下调用为例：

```text
read_file(A)
find_files(B)
write_file(C)
search_code(D)
run_command(E)
```

调度批次为：

```text
批次 1：read_file(A) + find_files(B)  并发
批次 2：write_file(C)                 串行
批次 3：search_code(D)                单个只读批次
批次 4：run_command(E)                串行
```

每个批次开始前按原始顺序发出工具调用事件。并发批次全部结束后，再按原始顺序发出结果事件。只有当前批次结束，后续批次才可启动。

### 未知工具停止计数

Runner 按模型原始调用顺序检查名称：

- 未知工具：计数加一。
- 已注册工具：计数归零。
- 一旦计数达到 3，记录“阈值已触发”，本轮后续调用仍处理完，以保证每个 tool call 都有对应结果。
- 当前完整轮次提交后停止，不再调用模型。
- 若一轮中从未达到 3，即使存在未知工具，也继续下一轮。

这避免为了立即停止而制造缺失工具结果的半轮历史。

### 迭代上限

- 第 20 轮若返回最终文本，按正常完成处理。
- 第 20 轮若返回工具调用，系统处理全部工具并提交完整轮次，然后以 `StopIterationLimit` 停止。
- 不发起第 21 次 Provider 请求。

### `/plan` 数据流

```text
/plan <任务>
  │
  ├─ mode.go 构造规划指令
  ├─ Registry.FilterBySafety(read_only)
  ├─ taskHistory ← Session.Snapshot()
  ├─ 使用只读范围和 taskHistory 运行完整 Agent Loop
  ├─ 每轮只提交到 taskHistory
  └─ 最终文本正常完成
       ├─ Session.CommitPlan(原始请求, 最终响应, 计划文本)
       ├─ 仅更新展示记录与待执行列表
       ├─ 模型上下文历史保持不变
       └─ EventCompleted
```

只有正常最终回答追加计划并记录可见结果。迭代上限、未知工具阈值、取消和流错误均不改变 Session；Plan Mode 临时历史随任务退出释放。

### `/do` 数据流

```text
/do
  │
  ├─ Session.PendingPlans()
  ├─ 无计划：同步返回验证错误，不创建 Task
  └─ 有计划：按原序编号并构造包含全部计划的执行指令
       ├─ 使用完整 Registry
       ├─ 运行普通 Agent Loop
       ├─ 正常完成：Session.ConsumePlans(本次快照)
       └─ 其他终态：不改变待执行列表
```

`/do` 的 Session 模型历史只包含普通 Act/Do 轮次；规划内部只读提示、探索响应和工具结果不会出现。执行指令明确携带每个待执行计划的原文和顺序，系统不预处理冲突，由模型结合完整计划集与当前工作区决定执行方式。

### 取消与错误

- TUI 收到 Ctrl+C 后调用 `Task.Cancel`，并继续读取事件直到终态和通道关闭。
- Collector 或 Scheduler 所有等待都监听同一任务上下文。
- 取消时不启动新批次、不提交当前轮次，发出 `EventCancelled`。
- 流错误时不执行该轮工具、不提交当前轮次，发出 `EventFailed`。
- 已经完成的副作用无法回滚，但不会继续启动剩余工具。
- Runner 在发送终态后清理忙碌状态并关闭事件通道。

## 文件组织

```text
internal/
├── agent/
│   ├── event.go              — Agent 事件、阶段、终态与用量汇总
│   ├── runner.go             — 任务入口、ReAct 循环和停止策略
│   ├── collector.go          — Provider 流双路收集
│   ├── scheduler.go          — 工具安全分批与有序结果
│   ├── mode.go               — Act、Plan、Do 请求构造
│   ├── history.go            — Plan Mode 任务临时历史
│   ├── runner_test.go        — 循环、停止条件和历史提交测试
│   ├── collector_test.go     — 分片、错误、Usage 和部分输出测试
│   ├── scheduler_test.go     — 并发、串行、顺序和取消测试
│   ├── mode_test.go          — Plan/Do 行为与计划保存测试
│   └── history_test.go       — 临时历史多轮提交与隔离测试
├── conversation/
│   ├── session.go            — 模型历史、展示记录与待执行计划列表
│   └── session_test.go       — 原子提交、深拷贝和计划状态测试
├── provider/
│   ├── event.go              — 增加规范化 Usage 事件
│   ├── anthropic/
│   │   ├── stream.go         — 合并并输出 Anthropic Usage
│   │   └── anthropic_test.go — 增加 Usage 流测试
│   └── openai/
│       ├── stream.go         — 提取 OpenAI Usage
│       └── openai_test.go    — 增加 Usage 流测试
├── tools/
│   ├── tool.go               — Metadata 增加 Safety
│   ├── registry.go           — 安全分类过滤
│   ├── executor.go           — 使用任务范围 Registry
│   └── tools_test.go         — 分类、过滤与范围执行测试
├── tui/
│   ├── commands.go           — 等待 Agent 事件
│   ├── model.go              — 保存 Runner、Session 和 Task
│   ├── update.go             — 提交任务、消费事件和取消
│   ├── view.go               — 渲染进度、Token、部分输出与终态
│   ├── run.go                — 更新构造依赖
│   └── tui_test.go           — 命令、事件和取消回归测试
└── config/
    ├── config.go             — AgentConfig 与默认值
    ├── validate.go           — 最大迭代数校验
    └── config_test.go        — 默认值和非法值测试

cmd/mewcode/
├── main.go                   — 组装 Session、Runner 与 TUI
└── main_test.go              — 启动依赖测试

docs/
├── README.md                 — 增加第 4 章索引
└── ch04-loop/
    ├── spec.md
    ├── plan.md
    ├── task.md
    └── checklist.md

config.example.yaml           — 增加 agent.max_iterations
README.md                     — 说明 Agent Loop、/plan 与 /do
```

现有 `internal/conversation/conversation.go`、`stream.go` 和 `conversation_test.go` 的单轮编排职责由 `agent` 与 `session.go` 取代，实施时删除，不保留两套循环入口。

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 编排位置 | 独立 `agent` 包 | 让循环可脱离 TUI 测试与复用 |
| 会话职责 | 分离模型历史、展示记录和待执行计划列表 | 防止界面需求与模型上下文隔离互相冲突 |
| 事件模型 | Runner 拥有单向异步事件通道 | 消费者只能观察，不会反向驱动循环 |
| 任务取消 | `Task.Cancel` 与统一 context | 同时覆盖 Provider、Collector 和工具执行 |
| 流式收集 | 同一 Collector 实时转发并完整累积 | 保证界面低延迟，同时保留可靠判断依据 |
| 历史提交 | 每轮工具全部处理后原子提交 | 后续模型永远看不到半轮上下文 |
| 工具调度 | 最大连续只读批次并发，副作用逐个串行 | 在保留模型顺序语义的同时获得安全并发 |
| 结果顺序 | 完成可乱序，发出和写回必须原序 | 让 UI、测试和模型上下文确定一致 |
| 未分类工具 | 默认副作用 | 新工具遗漏声明时采用保守行为 |
| Plan Mode 隔离 | 过滤后的 Registry 同时用于声明和执行 | 防止模型通过猜测隐藏工具名称绕过只读边界 |
| Plan 历史隔离 | 任务内临时历史；最终计划仅进展示记录与待执行列表 | 从上下文结构上消除只读指令污染，同时保留多轮规划和用户可见结果 |
| Usage 语义 | 每次 Provider 请求最多一个汇总事件 | 避免把供应商累计快照重复相加 |
| 迭代限制 | 默认 20，可配置；一次请求算一轮 | 含义明确，且在请求前即可判断边界 |
| 未知工具限制 | 固定连续 3 次，触发后处理完当前响应再停 | 保证工具调用与结果成对提交 |
| Plan 保存 | 成功规划追加，成功执行按快照消费，进程内保存 | 不丢弃早期计划，同时避免成功任务被重复执行 |
| 多计划冲突 | 原样、按序交给 `/do` 的模型判断 | 保留全部用户意图，不在系统层静默取舍 |
| 并发实现 | Go context、channel、goroutine、WaitGroup | 无需新增依赖，符合现有并发模型 |
| 工具失败 | 写回模型，不由 Runner 自动重试 | 保留 ReAct 自我调整能力，避免隐藏重试策略 |
| 迁移策略 | 删除旧 Conversation 编排入口 | 防止新旧流程并存造成行为分叉 |

依赖保持无环：

```text
tui ──→ agent ──→ conversation ──→ provider
          ├──────────────────────→ provider
          └────→ tools ─────────→ provider

cmd/mewcode ──→ config + provider + tools + conversation + agent + tui
```
