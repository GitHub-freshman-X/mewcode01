# MewCode SubAgent 子任务分发 Plan

## 架构概览

本章采用“工具外壳 + Agent 运行时编排 + 独立子任务域”三层设计。

`internal/subagent` 管理 Agent 定义、四级来源覆盖、内置角色、工具过滤和进程内后台任务生命周期。该包只表达定义和任务状态，不依赖 TUI。

`internal/tools/agent.go` 提供一个固定的 `agent` 工具。它只负责稳定 schema、参数校验和调用运行时桥接接口，不直接创建 Runner，从而避免 `tools → agent → tools` 的包循环。

`internal/agent` 负责根据父 Runner 创建定义式或 Fork 子 Runner、运行子任务直到终态、汇总结果和用量，以及把子任务进度和后台通知桥接回主 Agent。定义式与 Fork 各自拥有独立 Session、权限引擎和上下文管理器，但复用 Provider、Hook、工作区和工具执行器。

`internal/tui` 订阅 TaskManager 状态。它展示前台子 Agent 进度和后台任务状态；在有可转后台的前台子任务时，ESC 请求 TaskManager 接管而非取消；后台终态以系统消息显示。

`cmd/mewcode/main.go` 是组合根：使用配置根、项目根、注册工具、Provider、权限和 Hook 创建定义 Registry、TaskManager 和 SubAgentRuntime，注册统一 Agent 工具并将运行时注入主 Runner。

```text
主 Agent 调用 agent 工具
  → 参数分流：定义式 / Fork
  → Runner 工厂创建隔离子 Runner + 过滤后的工具集
  → TaskManager 独占消费子任务事件
  → 前台等待终态或转后台；后台立即返回任务 ID
  → 主对话 task-notification + TUI 状态更新
```

## Fork 参数兼容

统一 `agent` 工具不改变参数集合：省略 `subagent_type` 仍是标准 Fork 写法；值为 `fork` 是兼容写法。`internal/agent` 在创建子 Runner 前将两种输入归一为 Fork 模式，因此两者共用强制后台、父历史复制和递归阻断路径；不会对定义式角色做名为 `fork` 的查找。

`internal/subagent` 在定义校验阶段拒绝名称（忽略首尾空白及大小写）为 `fork` 的角色，防止项目、用户、内置或插件来源的定义与兼容语义冲突。角色列表中的其他名称和现有覆盖顺序不变。

## 核心数据结构

### `subagent.Definition` 与 `subagent.Registry`

```go
type Source string // project / user / builtin / plugin

type Definition struct {
    Name, Description, Model, SystemPrompt string
    Tools, DisallowedTools                []string
    MaxTurns                               int
    PermissionMode                         permissions.Mode
    Path                                   string
    Source                                 Source
}

type Registry struct {
    definitions map[string]Definition
}
```

`Registry` 按 plugin → builtin → user → project 的写入顺序合并，同名定义由后写入的高优先级来源覆盖。定义从 YAML frontmatter 读取名称、用途、白名单、黑名单、模型、最大轮次和权限模式；Markdown body 成为 `SystemPrompt`。frontmatter 的权限模式仅接受现有的 `strict`、`default`、`relaxed`。

### `subagent.TaskInfo` 与 `subagent.TaskManager`

```go
type TaskStatus string // running / completed / failed / cancelled

type TaskInfo struct {
    ID, Name, Description string
    Status                TaskStatus
    Result, Failure       string
    StartedAt, EndedAt    time.Time
    Usage                 provider.Usage
    ToolCalls             int
}

type TaskManager interface {
    Launch(context.Context, LaunchRequest) (TaskInfo, error)
    AdoptRunning(taskID string, running RunningTask) error
    Get(taskID string) (TaskInfo, bool)
    List() []TaskInfo
    Subscribe() <-chan TaskNotification
}
```

TaskManager 仅保存进程内状态。它用锁保护状态快照，以单一消费者读取子 Runner 的事件流，归集终态、工具调用计数和用量，并向订阅者发布安全摘要。合法状态迁移仅为 `running → completed`、`running → failed` 或 `running → cancelled`。

### 子 Agent 终态日志

`LaunchRequest` 增加可选的每任务终态回调。`TaskManager.finish` 规范化 `Outcome`、冻结终态 `TaskInfo`、关闭等待通道并发布既有通知后，异步调用该回调。回调只接收冻结快照；同一任务只能由 `finish` 完成一次，因此每个终态最多触发一次回调。日志 I/O 即使变慢，也不会延迟任务状态、前台等待结果或 TUI/主对话通知的可见性。

`internal/subagent` 不依赖 `internal/logging`。`SubAgentRuntime.dispatch` 在创建任务时以回调闭包捕获父 Runner 的 Logger、创建模式和子 Runner 的实际模型，并写入一条 `stage=subagent` 的终态日志。字段仅包括 `status`、`mode`、`background`、`model`、`tool_calls`、`input_tokens`、`output_tokens`、`cached_tokens` 与 `duration_ms`；`failed` 或 `cancelled` 可追加既有安全失败摘要。不得记录任务 prompt、模型消息、工具结果正文、密钥、请求头或原始错误。

### `tools.AgentInput` 与 `tools.SubAgentHost`

```go
type AgentInput struct {
    Prompt, Description, SubagentType, Model, Name, Isolation string
    RunInBackground                                        bool
}

type SubAgentHost interface {
    DispatchSubAgent(context.Context, AgentInput) (Result, error)
}
```

`AgentTool` 固定暴露 `prompt`、`description`、`subagent_type`、`model`、`run_in_background`、`name`、`isolation` 参数。工具层只校验输入并从 Context 取得 `SubAgentHost`；无 Host 时返回配置失败。`isolation: "worktree"` 返回结构化的本章未支持错误。schema 描述明确省略类型或传入 `fork` 都会创建 Fork，减少模型错误构造参数的概率。

### `agent.SubAgentRuntime` 与运行结果

```go
type SubAgentRuntime struct {
    Definitions *subagent.Registry
    Tasks       subagent.TaskManager
}

type SubAgentRun struct {
    Task    *Task
    Session *conversation.Session
    Mode    subagent.CreationMode // definition / fork
}
```

Runtime 解析角色、创建隔离 Session 和权限引擎、克隆或过滤工具注册表、构造子 Runner，并消费子任务终态。Fork 来源标记保存在运行时对象与 Context 中，不只保存在可被上下文压缩的消息文本中。

### 配置与内置定义

`config.AgentConfig` 增加 `EnableVerificationAgent bool`，YAML 字段为：

```yaml
agent:
  enable_verification_agent: false
```

内置定义由嵌入 Markdown 文件提供。`Explore`、`Plan`、`general-purpose` 总是载入；`Verification` 仅在开关为 true 时载入。

## 模块设计

### `internal/subagent`

- `definition.go`：定义 `Definition`、`Source`、创建模式以及模型、权限模式校验；拒绝保留的 `fork` 角色名。
- `discover.go`：复用第 11 章 frontmatter 的解析约定，扫描 `<项目根>/.mewcode/agents/*.md` 和 `os.UserConfigDir()/mewcode/agents/*.md`，再合并插件注入和内置定义。
- `builtins/*.md`：嵌入 `Explore`、`Plan`、`general-purpose`、`Verification` 的系统提示和角色元数据。
- `filter.go`：先移除全局禁止工具，再应用角色黑名单，最后在有白名单时取交集；后台路径额外和固定后台白名单取交集。未知白名单工具在加载期报错。
- `task_manager.go`：实现任务状态、订阅、事件消费、启动与接管。任务结果和失败仅保存安全摘要。

### `internal/tools/agent.go`

Agent 工具声明为有副作用的固定单一工具。它校验必填的 `prompt`、`description`，以及可选字段类型、空白值和 `isolation`，然后经 `SubAgentHost` 分发。工具不持有父 Runner，确保 registry 的副本不会错误复用父调用状态。

### `internal/agent`

- `subagent.go`：实现运行时桥接、定义解析及子 Runner 工厂。
  - **定义式**：新建 Session、permissions.Engine 和 context Manager；复制基础 Options 并应用定义和调用覆盖；使用过滤后且不含 Agent 工具的 registry。
  - **Fork**：省略类型或类型为 `fork` 时，复制父历史，为末尾悬空工具调用补齐结果，追加 Fork Boilerplate 和任务；复制工具 registry，但由运行时来源标记拒绝再次 Fork。
- `run_completion.go`：消费子 Runner 事件至终态，归集最终文本、Token 用量、工具调用计数和失败安全摘要。
- `event.go`：增加子任务状态事件和 Options 中的 SubAgentRuntime 注入。
- `runner.go`：在 Scheduler Context 中注入当前 `SubAgentHost`、来源标记及前台子任务事件转发；维护带锁的待发送任务通知队列。
- `scheduler.go`：保留既有工具协议，仅为 Agent 工具提供当前调用的上下文桥接。

定义式调用在前台时同步等待 RunToCompletion 的终态；显式后台及 Fork 调用立即返回任务 ID。前台任务运行满 120 秒时由 TaskManager 接管同一个事件流，不重启子任务。

### `internal/tui`

`agent.Event` 的子任务状态载荷包含任务 ID、名称、模式、状态、轮次、工具调用数、用量和安全摘要。`Model` 保存任务视图集合：前台显示阶段和进度，后台显示名称、ID、状态、耗时和终态。

当主任务仍在运行且存在可转后台子任务时，ESC 调用 `Runner.BackgroundForegroundSubAgent()`；成功后，工具调用以 `async_launched` 结果返回父循环，输入框恢复。Ctrl+C 继续取消主任务。TUI 订阅 TaskManager 通知，完成、失败和取消只渲染安全摘要的系统消息。

### ESC 提示时序与后台终态通知修复

TUI 的任务终态展示与主 Agent 的 `<task-notification>` 注入保持两条独立路径：前者只影响用户可见视图，后者继续由 Runner 在下一次模型请求前消费，二者均不写入持久会话历史。

`Model.Init()` 使用 `tea.Batch` 并行启动输入框焦点、已有权限桥监听（若存在）和首次 `waitForSubAgentNotification`。`Update` 在每次收到 `subAgentNotificationMsg` 后继续返回新的等待命令，保证 TaskManager 依次发布的多个终态均可显示；该等待不阻塞输入或主任务事件循环。

系统消息增加内部“等待当前主任务提交后定位”标记。ESC 成功接管时产生的提示在主任务仍处于临时视图时使用该标记，视图仍临时渲染在当前回合之后。主任务以完成或停止终态结束、且该回合已提交到 `DisplaySnapshot` 后，TUI 将带该标记的提示锚定为当前显示历史长度；后续轮次因此始终排在提示之后。若主任务失败或取消，则清除该标记而保留末尾渲染，维持现有失败/取消临时回合的显示语义，避免未来成功回合错误重定位旧提示。

测试在 TUI 层直接驱动模型、会话和 TaskManager：覆盖 ESC 提示在主回合提交后的锚定、首次通知监听、连续终态通知顺序，以及输入焦点和既有 ESC/Ctrl+C 行为回归。

### 前台期限与接管 context 修复

通用工具执行器继续为普通工具使用既有 30 秒 deadline，但对 `agent` 工具保留调用方 context，不附加该通用 deadline，使 SubAgentRuntime 的 120 秒前台自动后台计时器成为前台等待的唯一期限。

前台子任务以剥离 deadline 的独立 lifetime context 启动；运行时在任务仍为前台时转发父调用的取消，在 ESC 或自动后台接管信号先到达时停止转发。接管只改变同一 TaskManager 任务的背景状态，不重启 Worker 或事件流。由此，未接管任务仍随 `Ctrl+C` 取消，而已接管任务不再继承原工具调用的超时和取消。

回归测试使用可控 context 和阻塞 Worker 验证：Agent 工具不获得 Executor 的短 deadline、普通工具仍会超时；ESC/自动接管后取消父 context 不会取消任务；未接管时取消父 context 会取消任务。

### 主对话通知

Runner 订阅 TaskManager 终态通知并写入带锁队列。每轮模型请求前，队列内容会转换为独立 user 消息：

```text
<task-notification>
任务 ID、名称、终态、结果摘要、用量
</task-notification>
```

该路径不并发修改主会话历史，也不修改系统提示前缀；主 Agent 空闲时，通知保留到下一次模型请求。通知不包含 prompt、工具结果正文或原始错误载荷。

### 组合根、Hook、配置与文档

启动入口使用已获取的 `configRoot`、项目根、主 registry、Provider、权限和 Hook 构造定义 Registry、TaskManager 和 SubAgentRuntime，再注册 Agent 工具和创建主 Runner。Hook 的 `agent` executor 从“未接入”替换为调用同一 Runtime，保证 Hook 与模型调用共享定义、权限和后台限制。

配置的 `agent.enable_verification_agent` 默认 false，并同步更新配置加载、校验、`.mewcode/config.example.yaml` 与 README。README 还需说明定义目录、frontmatter、优先级、后台行为、TUI 按键、限制与跨平台用户路径；`docs/README.md` 增加第 13 章；理论文档将用户级路径改为跨平台配置目录。

## 模块交互

```text
main
  → subagent.Discover(project agents, user-config agents, builtins, plugins)
  → subagent.TaskManager
  → agent.SubAgentRuntime
  → tools.AgentTool + agent.Runner

Scheduler 执行 agent 工具
  → Context 中的 SubAgentHost
  → SubAgentRuntime.DispatchSubAgent
  → 定义式：独立 Session / 权限 / Context / 过滤 registry
  → Fork：父历史副本 / 占位结果 / Fork 标记
  → TaskManager 消费子 Runner 事件
  → 前台进度转发或后台状态通知
  → Runner task-notification 队列 + TUI 订阅
```

## 文件组织

```text
internal/subagent/
├── definition.go              — Definition、Source、frontmatter 校验
├── discover.go                — 四来源加载、覆盖与插件注入
├── task_manager.go            — 任务状态、通知与终态回调
├── builtins/
│   ├── explore.md
│   ├── plan.md
│   ├── general-purpose.md
│   └── verification.md
├── filter.go                  — 多层工具过滤
├── task_manager.go            — 状态、接管、订阅、通知
└── *_test.go

internal/tools/
└── agent.go                   — 固定 Agent 工具、输入 schema、运行时桥接

internal/agent/
├── subagent.go                — 创建模式、Runtime、Runner 工厂
├── run_completion.go          — 子任务事件消费与终态汇总
├── event.go                   — 子任务事件与 Options 注入
├── runner.go                  — 通知队列、上下文桥接
└── *_test.go

internal/tui/
├── model.go                   — 子任务视图与订阅状态
├── update.go                  — ESC 转后台与子任务事件
├── view.go                    — 任务进度/通知渲染
└── *_test.go

internal/config/{config.go,load.go,validate.go} — Verification 开关
cmd/mewcode/main.go                              — 组合根与 Hook 对接
.mewcode/config.example.yaml                     — 完整配置示例
README.md · docs/README.md · docs/ch13-subagent/* — 用户与章节文档
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| Agent 工具注册 | 一个固定 `agent` 工具 | 角色新增、删除或覆盖不改变模型工具列表。 |
| 运行时依赖方向 | 工具调用 Agent 层注入的桥接接口 | 避免 `tools` 与 `agent` 循环依赖，并确保每次调用使用正确父上下文。 |
| 定义加载 | 独立 `internal/subagent`，复用 Skill 的解析约定 | 保持 Markdown 用户体验一致，避免与 Skill 生命周期耦合。 |
| 用户级目录 | `os.UserConfigDir()/mewcode/agents` | 支持 macOS、Linux 和 Windows。 |
| 角色权限模式 | 仅 `strict/default/relaxed` | 复用既有权限引擎，避免本章扩展权限语义。 |
| 前台转后台 | TaskManager 从一开始独占事件流，前台仅等待其状态 | 可无重启、无双消费者地在 120 秒或 ESC 时接管。 |
| 终态日志归属 | TaskManager 提供终态回调，Runtime 负责日志上下文与写入 | 保持任务域不依赖 logging，同时用统一终态边界保证全路径覆盖和最多一次记录。 |
| 异步回传 | 线程安全待发送通知队列 | 不并发写主会话历史，且不修改 prompt cache 前缀。 |
| Fork 递归防护 | 运行时来源标记 + 工具过滤 | 不依赖可压缩的消息文本，防止绕过与上下文爆炸。 |
| 插件来源 | 构造 Registry 时注入定义集合 | 本章提供来源与覆盖语义，不引入插件安装系统。 |
| Verification 开关 | `agent.enable_verification_agent`，默认 false | 与项目现有 YAML 分组和 snake_case 风格一致。 |

## Spec 覆盖

| Spec 需求 | 设计归属 |
|---|---|
| F1、F1a | `tools.AgentTool`、`agent.SubAgentRuntime`、`subagent` 定义校验 |
| F2-F3 | `subagent` 定义、发现、内置角色与配置 |
| F4-F6 | `agent/subagent.go`、独立 Session/权限/Context、Provider/Hook/工作区复用 |
| F7 | `subagent/filter.go` 和运行时 Fork 标记 |
| F8 | `agent/run_completion.go` |
| F9 | `subagent/task_manager.go`、120 秒接管与 TUI 转后台 |
| F10 | Runner 待发送通知队列 |
| F16-F18、N13-N14 | TaskManager 终态回调与 SubAgentRuntime 安全终态日志 |
| F11 | TUI 子任务视图、ESC 和订阅 |
| N1-N7 | 过滤、隔离、前缀保持、失败隔离、安全日志、跨平台发现和自动测试 |
