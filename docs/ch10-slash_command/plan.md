# MewCode Slash Command Plan

## 架构概览

命令系统分为纯逻辑的 `command` 模块与 Bubble Tea 适配层。`command` 负责命令定义、启动校验、解析、查找、帮助信息与补全，不依赖 TUI。内置 Handler 通过 `CommandContext` 访问抽象的 UI 控制能力及会话、记忆服务；TUI `Model` 实现这些边界并负责渲染反馈、状态栏与补全菜单。

用户提交输入时，TUI 先调用命令解析器。非命令文本根据持久 UI 模式转为 `agent.ModeAct` 或 `agent.ModePlan` 请求；命令则由注册中心分发。纯本地与界面状态命令直接更新 UI，预设提示词与既有 Agent 功能命令通过统一的任务启动路径调用 `agent.Runner`。

`/clear` 与 `/session resume` 需要切换当前会话，因此 Runner 提供仅在空闲状态调用的会话替换操作：同时更新 Session 和会话 ID，并重建该会话对应的上下文结果存储。`/do` 先退出计划模式，再启动既有 `ModeDo` 任务，继续执行当前会话中保存的计划。

## 核心数据结构

### `command.Kind`

```go
type Kind string

const (
	KindLocal   Kind = "local"
	KindLocalUI Kind = "local_ui"
	KindPrompt  Kind = "prompt"
)
```

标识命令是否完全本地完成、会改变 UI 状态，或会构造并提交 Agent 请求。

### `command.Command` 与 `command.Invocation`

```go
type Command struct {
	Name, Description, Usage, ArgPrompt string
	Aliases                             []string
	Kind                                Kind
	Hidden                              bool
	Handler                             Handler
}

type Invocation struct {
	IsCommand bool
	Name      string
	Args      string
}
```

`Command` 是注册中心的唯一命令定义。`Invocation` 保留解析结果：`Name` 已规范化为小写，`Args` 为命令名后经修剪的剩余文本。

### `command.Registry`

```go
type Registry struct { /* ordered commands and normalized lookup index */ }

func NewRegistry(commands ...Command) (*Registry, error)
func (r *Registry) Find(name string) (Command, bool)
func (r *Registry) Visible() []Command
func (r *Registry) Complete(prefix string) []string
```

`NewRegistry` 校验命令名和别名的规范化唯一性。`Complete` 仅返回可见命令及别名的稳定、有序候选。

### `command.CommandContext` 与运行边界

```go
type UIController interface {
	AddSystemMessage(string)
	StartAgent(agent.Request) error
	SetPlanMode(bool)
	PlanMode() bool
	TokenUsage() provider.Usage
	RefreshStatus()
}

type CommandContext struct {
	UI       UIController
	Sessions SessionService
	Memory   MemoryService
	Logger   Logger
	Args     string
}

type Handler func(CommandContext) error
```

`SessionService` 封装创建、列表、恢复与删除，并在成功切换时协调 TUI 与 Runner。`MemoryService` 封装既有用户级、项目级记忆的概要、列表、添加和清空。两者均只暴露本章所需操作，Handler 不接触渲染细节或文件路径。

### TUI 状态

`tui.Model` 增加当前计划模式、系统消息队列、补全候选与选中索引、命令注册中心及会话/记忆运行依赖。任务仍由现有 `task` 字段追踪；活跃任务或待确认权限时维持既有输入限制。

## 模块设计

### `internal/command`

**职责：** 提供不依赖 TUI 的注册、解析、查找、帮助、补全及九个内置 Handler。

**对外接口：** `NewRegistry`、`Parse`、`Registry.Find`、`Registry.Complete`、`DefaultCommands`、`Dispatch`。

**依赖：** `agent` 的请求模型、会话与记忆服务接口、日志接口；不依赖 `tui`。

内置命令按下列行为注册：

- `/help`：列出可见命令，或按名称/别名显示单条详细用法。
- `/compact`：以 `ModeCompact` 请求调用既有手动压缩能力。
- `/clear`：创建并切换到新会话，清除当前视图状态。
- `/plan`：无参数时切换计划模式；有参数时确保进入计划模式并以 `ModePlan` 提交需求。
- `/do`：退出计划模式并以 `ModeDo` 执行当前会话的待执行计划。
- `/session`：显示当前会话概要，并支持 `list`、`new`、`resume <id>`、`delete <id>`。
- `/memory`：显示概要，并支持 `list`、`add <category> <content>`、`clear`；清空操作先显示确认提示，再由明确确认完成。
- `/status`：显示模式、Token 用量、工具数量、记忆概要、工作目录及版本信息。
- `/review`：构造固定的 git diff 审查请求，附加用户给出的关注点后以普通 Agent 对话提交。

### `internal/conversation` 与 `internal/memory`

**职责：** 以现有第九章存储格式为基础，为命令提供受限服务入口。

**对外接口：** 会话存储继续使用 `Create`、`List`、`Restore`、`Delete`；记忆服务新增面向命令的概要、枚举、手动写入与清空入口。

**依赖：** 不依赖命令模块；返回结构化元数据或已脱敏的用户可见摘要。

### `internal/agent.Runner`

**职责：** 在没有活跃任务时安全替换当前 Session、SessionID 以及关联的上下文结果存储。

**对外接口：** 新增会话替换方法，拒绝 nil 会话和活跃任务中的切换。

**依赖：** 继续依赖 `conversation.Session` 与现有上下文管理器，不依赖命令或 TUI。

### `internal/tui`

**职责：** 实现 `UIController` 与命令服务适配；在 Enter 前进行分流；维护计划模式、消息与补全菜单；将命令结果渲染为系统消息。

**对外接口：** `Run` 和 `RunWithPermissions` 扩展为接收命令运行依赖，保持应用启动层单一装配点。

**依赖：** `command`、`agent`、`conversation`、`memory` 和 Bubble Tea。

## 模块交互

1. `main` 创建会话存储、记忆服务、Runner 和命令注册中心，并将命令运行依赖交给 TUI。
2. TUI 在 Enter 时修剪输入；空输入直接返回。
3. `command.Parse` 判定非命令时，TUI 根据当前计划模式启动 `ModeAct` 或 `ModePlan` 请求。
4. 对命令输入，TUI 调用 `Registry.Find`：输入 `/` 显示可见命令；未知命令写入带 `/help` 引导的系统消息；缺少参数时显示 `ArgPrompt`。
5. 命中命令后，`Dispatch` 记录安全日志并调用 Handler。Handler 通过 `UIController` 写系统消息、改变模式或启动任务。
6. `/plan` 更新模式后可启动 `ModePlan`；`/do` 先更新为默认模式，再启动 `ModeDo`。任务事件仍由既有 `applyAgentEvent` 消费。
7. `/clear` 与 `/session resume` 成功创建或恢复会话后，TUI 调用 Runner 会话替换方法，再同步更新 `Model.session` 与视图。
8. Tab 触发 `Registry.Complete`：零候选保持输入；一项候选直接替换；多项候选显示菜单，方向键改变选择，Tab 或 Enter 写入选中项。

## 文件组织

```text
internal/command/
├── command.go       — Kind、Command、Invocation、Context 与服务接口
├── registry.go      — 索引、冲突检测、查找与可见命令
├── parser.go        — 斜杠解析
├── complete.go      — 前缀补全
├── builtins.go      — 九个命令的元数据与 Handler
└── command_test.go  — 注册、解析、帮助、补全和分发测试
internal/agent/runner.go       — 空闲会话替换
internal/agent/runner_test.go  — 会话替换的并发与上下文存储测试
internal/conversation/store.go — 命令使用的会话元数据辅助能力（如需要）
internal/memory/service.go     — 受限记忆管理入口
internal/memory/memory_test.go — 记忆管理入口测试
internal/tui/model.go          — 命令运行依赖及 UI 状态
internal/tui/update.go         — Enter 分流与补全键盘交互
internal/tui/view.go           — 系统消息、补全菜单、模式状态栏
internal/tui/run.go            — 命令依赖注入
internal/tui/tui_test.go       — TUI 分流、模式、菜单和兼容性测试
cmd/mewcode/main.go            — 命令运行依赖装配
cmd/mewcode/main_test.go       — 启动装配测试
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 命令核心位置 | 独立 `internal/command` | 可直接测试注册、解析与补全，不与 Bubble Tea 耦合。 |
| 注册冲突 | 启动时建立规范化索引并返回错误 | 尽早失败，避免运行时不确定性。 |
| 模式归属 | TUI 持久 UI 状态 + 每次请求显式 Mode | `/plan` toggle 能影响后续输入，同时不引入 Agent 全局可变模式。 |
| `/do` 行为 | 先退出计划模式，再使用既有 `ModeDo` | 同时满足用户可见模式切换与第 4 章待执行计划语义。 |
| 本地反馈 | TUI 系统消息队列 | 不污染会话历史，不会被发送给模型。 |
| Tab 多候选 | TUI 内部菜单，方向键选择、Tab/Enter 接受 | 保持键盘优先且无需新增渲染依赖。 |
| 会话切换 | Runner 空闲时原子替换 | 防止活跃任务向旧会话写入或跨会话串数据。 |
| 记忆管理 | 复用第 9 章目录与 Markdown 格式 | 不创建第二套数据源或改变自动提取、治理行为。 |
| 日志内容 | 仅元数据 | 满足可观测性且避免记录参数、prompt、Token 与密钥。 |

## Spec 覆盖

| 需求 | 设计归属 |
|---|---|
| F1、F2、N1、N3 | `internal/command` Registry、Parser、补全和单元测试 |
| F3、F4 | CommandContext、UIController、Dispatcher 与内置 Handler |
| F5、N5 | `tui.Update` 输入分流和既有任务/权限路径守卫 |
| F6 | `command.Complete` 与 TUI 补全菜单 |
| F7、F8 | 默认命令目录、内置 Handler、TUI 持久模式与 `ModeDo` 复用 |
| F9 | `tui.View` 状态栏 |
| N2 | 本地 Registry 操作与 Agent 请求边界测试 |
| N4、AC9 | Dispatcher/TUI 生命周期日志及日志断言 |
| N6 | 不新增配置，`config.example.yaml` 不改动 |
| AC1–AC8 | command、agent、memory、TUI 与启动层自动化测试 |
