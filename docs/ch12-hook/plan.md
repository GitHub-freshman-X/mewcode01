# MewCode Hook 生命周期钩子 Plan

## 架构概览

新增 `internal/hooks` 作为独立的 Hook 引擎：它拥有规则模型、配置加载、条件解析、上下文变量展开和动作执行器，并通过一个小型 `Engine` API 向 Agent 层暴露同步事件运行与工具前拦截。该包不依赖 Agent Loop、TUI 或权限引擎；它只接收已归一化的事件上下文和注入的动作依赖，因此可独立测试。

`internal/agent` 是生命周期事件的编排层。Runner 在会话、轮次、模型请求和终态处触发 Hook；Scheduler 在工具调用进入权限和真实执行器之前同步触发工具前 Hook，在获得工具结果后触发工具后 Hook。Hook 拒绝转换成标准的工具失败结果，随后沿现有结果回灌链写入模型历史。权限门禁仍保留在真实工具执行之前，Hook 只能增加拒绝，不能提供放行路径。

`cmd/mewcode` 是组合根：解析 Hook 配置路径、加载三个配置来源、构造 Hook 引擎并注入 Runner。根目录配置示例、README 和文档索引同步记录用户可配置的规则格式和第 12 章入口。

## 核心数据结构

### `hooks.Event`

生命周期事件枚举，包含 `session_start`、`session_end`、`turn_start`、`turn_end`、`pre_send`、`post_receive`、`pre_tool_use`、`post_tool_use`、`startup`、`shutdown`、`error`、`compact`、`permission_request`、`file_change` 与 `command_execute`。`IsValid` 用于加载期校验；`pre_tool_use` 是唯一可拒绝且必须同步的事件。

### `hooks.Context`

事件运行时输入，包含事件名、工具名、工具参数、文件路径、消息和安全化错误摘要。提供 `Field(name)` 给条件读取 `event`、`tool`、`args.<name>`，以及 `Expand(template)` 替换 `$EVENT`、`$TOOL_NAME`、`$FILE_PATH`、`$MESSAGE`、`$ERROR`、`$TOOL_ARGS.<name>`。缺失字段统一返回空字符串。

### `hooks.Rule` 与 `hooks.Action`

`Rule` 包含标识、事件、可选条件、动作、`Reject`、`Once`、`Async` 和在合并规则中的声明顺序。`Action` 以类型区分 command、prompt、http、agent，并保存对应字段：命令和超时、提示词消息、HTTP 请求定义或子 Agent 提示词。运行时执行状态不进入 YAML，也不持久化。

### `hooks.ConditionGroup`

预解析后的条件组，包含组合模式（AND 或 OR）和有字段、操作符、值构成的原子条件。支持 `==`、`!=`、`=~`、`~=`；正则与 glob 在加载期编译或校验。解析器拒绝空条件、非法子条件、未知操作符和 `&&`/`||` 混用。

### `hooks.Engine`

持有合并后的有序规则、一次执行状态、动作执行器与 Logger。公开两个入口：

```go
Run(ctx context.Context, event Event, hookCtx Context) []Result
RunPreTool(ctx context.Context, hookCtx Context) (Result, bool)
```

`Run` 只用于非拦截事件：同步失败记录日志后继续，异步规则转入受控后台任务。`RunPreTool` 顺序运行匹配规则，在第一个 `Reject` 规则完成后返回其输出与拒绝标记；调用方负责生成工具失败结果。两入口均在运行前检查一次执行状态，且只在实际调度后标记规则已执行。

### `hooks.Executor` 与 `hooks.Result`

Executor 根据动作类型执行经变量展开后的动作。`Result` 只向编排层提供成功/失败、是否拒绝、可供模型读取的简短输出、退出码或 HTTP 状态、耗时等结果；日志字段只采用类型、状态、计数、耗时、大小和状态码等安全元数据。Command Executor 使用带取消的子进程上下文实现超时；HTTP Executor 通过可注入客户端；Prompt Executor 通过通知接收器追加上下文；Agent Executor 调用可注入 Runner，默认返回“未接入”。

### `hooks.FilePaths` 与 `hooks.RuleSet`

`FilePaths` 解析用户级 `~/.mewcode/config.yaml`、项目级 `.mewcode/config.yaml`、本地级 `.mewcode/config.local.yaml`。加载器只读取各文件的 `hooks` 字段，按用户、项目、本地顺序追加有效规则；缺失文件和缺失字段均表示空规则集。由于主运行配置目前只从用户配置读取，Hook 加载器直接处理三份 YAML，避免改变现有 Provider 配置的覆盖语义。

## 模块设计

### `internal/hooks/types.go`

**职责：** 定义事件、规则、动作、上下文、结果与动作依赖的稳定模型。

**对外接口：** 事件校验、上下文字段读取与模板展开、规则匹配所需的公开类型。

**依赖：** 标准库、`internal/logging` 的安全日志接口。

### `internal/hooks/condition.go`

**职责：** 将条件文本解析为 `ConditionGroup`，并按上下文执行精确、反向、正则与 glob 匹配。

**对外接口：** `ParseCondition`、`ConditionGroup.Match`。

**依赖：** `types.go`、标准库正则与路径匹配能力；不依赖权限包，避免把权限规则键语法耦合到 Hook 条件语法。

### `internal/hooks/config.go`

**职责：** 解析三份 YAML 的 `hooks` 序列、注入默认规则标识、编译条件并集中校验每类动作的必填字段和事件约束。

**对外接口：** `DefaultFilePaths`、`LoadRuleSet`、`ValidateRules`。

**依赖：** `types.go`、`condition.go`、YAML 库、标准库文件系统。

### `internal/hooks/engine.go`

**职责：** 保存合并后规则与一次执行状态，按声明顺序分发事件，处理异步隔离和工具前拒绝短路，并记录安全日志。

**对外接口：** `NewEngine`、`Run`、`RunPreTool`。

**依赖：** `types.go`、`executor.go`、`internal/logging`。

### `internal/hooks/executor.go`

**职责：** 实现 command、prompt、http、agent 四种动作，并将动作结果归一化；所有失败返回给 Engine 记录，不直接终止主流程。

**对外接口：** `Executor`、`PromptSink`、`AgentRunner` 及用于测试的依赖注入选项。

**依赖：** `types.go`、标准库进程与 HTTP；不依赖 Agent Runner 的具体类型。

### `internal/agent`

**职责：** 将 Hook 引擎注入 Runner 和 Scheduler，映射已有 Agent Loop 的会话、轮次、模型、工具、权限、压缩和终态节点。

**改动：** `Options` 增加 Hook 引擎与提示词通知存储；`Runner.run` 在各生命周期点调用非拦截入口，并将 prompt 通知作为普通用户侧 Hook 通知追加到下一次模型请求的历史；`Scheduler` 在权限判断和真实工具执行前调用 `RunPreTool`，将拒绝转为标准失败工具结果，工具完成后触发 `post_tool_use`。系统、命令和文件变更事件通过现有启动、关闭、Slash Command、权限与工具元数据对应节点触发；无法自然产生的事件不制造伪事件。

**依赖：** `internal/hooks`、既有工具、权限、Provider、会话与日志模块。

### `cmd/mewcode/main.go`、README 与示例配置

**职责：** 构造 Hook 配置路径与 Engine，注入 Runner；公开完整配置面与章节入口。

**改动：** 在创建 Runner 前加载 Hook 规则；加载错误阻止启动并保留可诊断路径；把同一 Logger 与动作依赖交给引擎。`.mewcode/config.example.yaml` 添加三份配置均可声明的 `hooks` 示例和动作字段；README 增补 Hook 配置位置、加载顺序、限制及安全边界；`docs/README.md` 新增第 12 章索引。

## 模块交互

```text
三个 YAML 文件
  → hooks.LoadRuleSet / ValidateRules
  → hooks.NewEngine
  → agent.Runner / Scheduler

Agent 生命周期节点
  → Engine.Run（非拦截事件）
  → Executor（command / prompt / http / agent）
  → 安全日志 或 下一次模型请求的 Hook 通知

Scheduler 收到工具调用
  → Engine.RunPreTool
  → [拒绝] 标准失败工具结果 → 原有回灌链 → 模型下一轮
  → [允许] 权限引擎 → 工具执行器 → post_tool_use Hook → 原有回灌链
```

## 文件组织

```text
internal/hooks/
├── types.go          — 事件、规则、动作、上下文和结果
├── condition.go      — 条件解析、校验和匹配
├── config.go         — 三来源 YAML 加载、合并和校验
├── executor.go       — 四种动作执行器及注入接口
├── engine.go         — 分发、一次执行、异步与拒绝短路
└── *_test.go          — 单元与集成测试

internal/agent/
├── event.go           — Hook 选项与通知状态
├── runner.go          — 非工具生命周期节点与提示词通知接入
├── scheduler.go       — 工具前拦截和工具后事件
└── *_test.go          — Agent Loop 回灌和隔离测试

cmd/mewcode/main.go                 — Hook 组合根
.mewcode/config.example.yaml        — 配置全量示例
README.md                           — 用户可见配置与运行说明
docs/ch12-hook/{spec,plan,task,checklist}.md
docs/README.md                       — 章节索引
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 配置位置 | 复用三层 `config.yaml` 的 `hooks` 字段 | 与理论文档及用户心智一致，同时不改变现有主配置的 Provider 覆盖语义。 |
| Hook 包边界 | 新建独立 `internal/hooks` 包 | 条件、配置和执行器可独立测试，避免 Agent Loop 膨胀。 |
| 工具拦截位置 | Scheduler 在权限与真实执行前调用 | 同一个入口覆盖所有调度批次，确保拒绝时真实工具零执行。 |
| 权限与 Hook 的关系 | Hook 仅可拒绝 | 保留第 6 章硬安全边界，禁止利用 Hook 反向放行。 |
| 提示词注入 | 下一次模型请求前追加普通 Hook 通知 | 不改 System Prompt 前缀，避免破坏提示词缓存和既有 Prompt 构建器。 |
| 异步模型 | Engine 内部受控 goroutine，错误只记日志 | 保证非关键外部通知不阻塞主流程，且不把失败传播为 Agent 终态。 |
| 子 Agent | 注入接口加默认占位实现 | 保留第 13 章对接点，不提前引入真实运行时依赖。 |
| 条件实现 | Hook 自己解析四种操作符 | 满足“复用匹配语义”而不耦合权限系统不同的规则键格式。 |
