# MewCode Permission System Plan

## 架构概览

权限系统新增为独立的 `internal/permissions` 包，负责把一次工具调用转换为可判定的权限请求，并按黑名单、路径沙箱、规则层级、权限模式和用户确认的顺序给出决策。该包不依赖 TUI，也不直接执行工具；它只返回 allow、deny 或 ask，并提供可写入会话/本地规则的结果。

工具层继续负责工具参数校验和真实执行，但会为权限系统补充“权限元数据”：每个工具声明自己的匹配对象来源、路径参数和安全分类。现有 `tools.Executor` 保持“执行一个工具调用并返回结构化工具结果”的边界，权限门禁放在 `agent.Scheduler` 调用 Executor 之前，确保同一响应里的每个工具调用都先判定再执行。

Agent 层负责把权限决策接入现有循环：允许的调用按原调度规则执行；拒绝的调用转成 `permission_error` 工具结果写回模型；需要确认的调用通过确认器阻塞等待用户选择。确认等待期间上下文可取消，取消后不启动工具并让任务进入取消终态。

TUI 层负责人在回路体验：展示权限请求，提供本次允许、本会话允许、永久允许、拒绝和取消。TUI 不实现权限规则判断，只把用户选择交回 Agent。脚本化测试使用内存确认器，不需要启动真实界面。

配置层负责加载主配置里的权限模式，以及三层规则 YAML。规则文件采用简单映射结构，用户级、项目级、本地级分别加载并严格校验；永久允许只写入本地级规则文件。

## 核心数据结构

### permissions.Mode

```go
type Mode string

const (
    ModeStrict Mode = "strict"
    ModeDefault Mode = "default"
    ModeRelaxed Mode = "relaxed"
)
```

`Mode` 表示未命中显式规则时的默认处理方式：严格模式全部询问，默认模式只读默认允许且副作用询问，放行模式默认允许。放行模式的配置值使用 `relaxed`，面向文档和 UI 展示时显示为“放行”。

### permissions.Effect

```go
type Effect string

const (
    EffectAllow Effect = "allow"
    EffectDeny  Effect = "deny"
)
```

`Effect` 是规则结果，只有 allow 和 deny 两种。

### permissions.Scope

```go
type Scope string

const (
    ScopeSession Scope = "session"
    ScopeLocal   Scope = "local"
    ScopeProject Scope = "project"
    ScopeUser    Scope = "user"
)
```

`Scope` 标记规则来源和优先级。会话级规则仅存在内存中；本地级规则可由“永久允许”写入。

### permissions.Rule

```go
type Rule struct {
    Key     string
    Tool    string
    Pattern string
    Effect  Effect
    Scope   Scope
    Index   int
}
```

`Rule` 表示一条 `工具名(模式)` 规则。`Index` 保存同一文件中的声明顺序，用于在同一层级内保持确定性。规则加载时解析 `Key`，校验工具名、括号结构、Effect 和 glob 模式。

### permissions.RuleSet

```go
type RuleSet struct {
    Session []Rule
    Local   []Rule
    Project []Rule
    User    []Rule
}
```

`RuleSet` 保存四层规则。决策顺序固定为 Session、Local、Project、User；每层内部按文件顺序匹配，先命中者生效。

### permissions.Request

```go
type Request struct {
    CallID      string
    Tool        string
    Arguments   json.RawMessage
    Safety      tools.Safety
    MatchTarget string
    Paths       []PathCheck
}
```

`Request` 是权限判断的输入。`MatchTarget` 是规则匹配对象：命令工具使用命令加参数的规范化文本；文件工具使用项目相对路径；查找和搜索工具使用模式文本。`Paths` 是需要进行真实路径沙箱检查的路径列表。

### permissions.PathCheck

```go
type PathCheck struct {
    Raw       string
    Real      string
    Relative  string
    Parameter string
}
```

`PathCheck` 记录路径参数解析结果。`Raw` 来自工具参数，`Real` 是解析符号链接后的绝对路径，`Relative` 是项目内相对路径，`Parameter` 用于错误诊断。

### permissions.Decision

```go
type Decision struct {
    Action      Action
    Stage       Stage
    Reason      string
    Rule        *Rule
    Request     Request
    SuggestedKey string
}
```

`Decision` 是权限系统输出。`Action` 取 allow、deny 或 ask；`Stage` 标记黑名单、沙箱、规则、模式或确认；`SuggestedKey` 是确认后可生成的规则键。

### permissions.Confirmation

```go
type Confirmation struct {
    Decision Decision
    Choice   Choice
}
```

`Confirmation` 表示用户对一次 ask 决策的回应。`Choice` 包含 deny、allow_once、allow_session、allow_permanent、cancel。

### tools.PermissionMetadata

```go
type PermissionMetadata struct {
    Target      PermissionTarget
    PathParams  []string
}
```

工具元数据新增权限声明。`Target` 描述如何从参数生成 `MatchTarget`，`PathParams` 声明哪些参数需要真实路径沙箱检查。未声明的新增工具按安全策略保守处理：副作用工具未命中规则时至少进入确认。

### agent.PermissionBridge

```go
type PermissionBridge interface {
    Confirm(context.Context, permissions.Decision) (permissions.Confirmation, error)
}
```

`PermissionBridge` 是 Agent 到界面的确认接口。测试可用脚本化实现；TUI 使用桥接对象把确认请求发送给 Model 并等待用户按键回应。

## 模块设计

### internal/permissions/parser.go

**职责：** 解析 `工具名(模式)` 规则键，校验 effect，识别 glob 元字符。

**对外接口：**

```go
func ParseRule(key string, effect Effect, scope Scope, index int) (Rule, error)
func IsGlob(pattern string) bool
func MatchRule(rule Rule, req Request) (bool, error)
```

**依赖：** 标准库字符串、路径匹配能力。

### internal/permissions/config.go

**职责：** 定义规则文件 YAML 结构，加载用户级、项目级、本地级规则，并提供本地级永久规则写入。

**对外接口：**

```go
type FilePaths struct {
    User    string
    Project string
    Local   string
}

func DefaultFilePaths(workspace string) (FilePaths, error)
func LoadRuleSet(paths FilePaths) (RuleSet, error)
func AppendLocalAllow(paths FilePaths, rule Rule) error
```

**依赖：** `go.yaml.in/yaml/v4`、文件系统、`permissions.ParseRule`。

### internal/permissions/path.go

**职责：** 根据项目根真实路径解析工具路径参数，处理已存在部分的符号链接，并返回项目内相对路径。

**对外接口：**

```go
type Sandbox struct {
    Root string
    RealRoot string
}

func NewSandbox(root string) (Sandbox, error)
func (s Sandbox) Resolve(raw string, parameter string) (PathCheck, error)
```

**依赖：** 标准库路径和文件系统。写入新文件时，对已存在的父目录解析符号链接；不存在的尾部按普通路径拼回。

### internal/permissions/blacklist.go

**职责：** 维护不可配置放行的危险命令正则列表，并判断命令文本是否命中。

**对外接口：**

```go
type BlacklistMatch struct {
    Pattern string
    Reason  string
}

func CheckCommandBlacklist(commandText string) (*BlacklistMatch, error)
```

**依赖：** 标准库 regexp。正则在包初始化或测试初始化时编译失败即返回错误，避免静默失效。

### internal/permissions/request.go

**职责：** 把 `provider.ToolCall`、工具元数据和参数 JSON 转换为 `permissions.Request`。

**对外接口：**

```go
func BuildRequest(call provider.ToolCall, tool tools.Tool, sandbox Sandbox) (Request, error)
func CommandText(args map[string]any) string
func SuggestedRuleKey(req Request) string
```

**依赖：** `provider`、`tools`、参数校验辅助逻辑。此处只解析权限判断所需字段，不执行真实工具动作。

### internal/permissions/engine.go

**职责：** 实现完整权限决策顺序：黑名单、路径沙箱、会话规则、本地规则、项目规则、用户规则、权限模式、询问。

**对外接口：**

```go
type Engine struct {
    Mode    Mode
    Rules   *RuleStore
    Sandbox Sandbox
}

func (e *Engine) Decide(ctx context.Context, call provider.ToolCall, tool tools.Tool) (Decision, error)
func (e *Engine) ApplyConfirmation(conf Confirmation) error
```

**依赖：** `permissions` 内部模块、`tools`、`provider`。`RuleStore` 封装内存会话规则和文件规则，保证并发安全。

### internal/permissions/result.go

**职责：** 把权限拒绝转换为现有 Provider 工具结果格式。

**对外接口：**

```go
func DeniedToolResult(call provider.ToolCall, decision Decision) provider.ToolResult
```

**依赖：** `tools.Failure` 的 JSON 结构，错误类型使用 `permission_error`。

### internal/tools/tool.go

**职责：** 扩展工具元数据，允许工具声明权限匹配对象和路径参数。

**对外接口变化：**

```go
type Metadata struct {
    Name        string
    Description string
    Schema      Schema
    Safety      Safety
    Permission  PermissionMetadata
}
```

**依赖：** 无新增外部依赖。已有工具补充元数据，不改变工具名称、Schema 或执行返回。

### internal/agent/scheduler.go

**职责：** 在执行工具前调用权限门禁。允许则继续执行，拒绝则生成工具失败结果，需要确认则通过 `PermissionBridge` 等待用户选择。

**对外接口变化：**

```go
type Scheduler struct {
    registry   *tools.Registry
    executor   *tools.Executor
    gate       *permissions.Engine
    confirmer  PermissionBridge
}

func NewScheduler(registry *tools.Registry, executor *tools.Executor, gate *permissions.Engine, confirmer PermissionBridge) *Scheduler
```

**依赖：** `internal/permissions`。Scheduler 仍负责保持工具结果顺序和批次语义。

### internal/agent/event.go

**职责：** 增加权限相关事件，供 TUI 展示权限判断和确认状态。

**对外接口变化：**

```go
const (
    EventPermissionDecision EventType = "permission_decision"
    EventPermissionRequest  EventType = "permission_request"
    EventPermissionResponse EventType = "permission_response"
)

type Event struct {
    ...
    PermissionDecision *permissions.Decision
    PermissionConfirmation *permissions.Confirmation
}
```

**依赖：** `internal/permissions`。

### internal/agent/mode.go 与 internal/agent/runner.go

**职责：** 在 Agent options 中携带权限引擎和确认桥。Plan Mode 继续先过滤只读工具，再让权限系统处理剩余只读调用；Do Mode 使用完整 registry 后再走权限系统。

**对外接口变化：**

```go
type Options struct {
    ...
    Permissions *permissions.Engine
    Confirmer   PermissionBridge
}
```

**依赖：** `internal/permissions`。

### internal/config/config.go、load.go、validate.go

**职责：** 增加主配置中的权限模式字段，保持旧配置可加载。

**配置结构：**

```yaml
permissions:
  mode: default
```

**对外接口变化：**

```go
type PermissionConfig struct {
    Mode permissions.Mode `yaml:"mode,omitempty"`
}

type Config struct {
    ...
    Permissions PermissionConfig `yaml:"permissions,omitempty"`
}
```

**依赖：** `internal/permissions` 的模式类型或等价字符串校验。默认值为 `default`。

### cmd/mewcode/main.go

**职责：** 组装权限系统：确定工作区，加载三层规则文件，创建权限引擎，创建 TUI 确认桥，并传入 Runner。

**依赖：** `internal/permissions`、已有 config/tools/agent/tui。

### internal/tui/permissions.go

**职责：** 实现 TUI 确认桥和权限提示状态。用户在确认状态下可按键选择本次允许、本会话允许、永久允许、拒绝或取消。

**对外接口：**

```go
func NewPermissionBridge() *PermissionBridge
```

**依赖：** Bubble Tea 消息机制和 `internal/permissions`。Bridge 不解析规则，只传递请求和响应。

### docs/README.md、config.example.yaml、.gitignore

**职责：** 文档索引新增第六章；示例配置展示 `permissions.mode`；`.gitignore` 建议忽略 `.mewcode/permissions.local.yaml`，避免本机永久允许规则进入版本库。

## 模块交互

1. 启动时，`cmd/mewcode` 读取主配置，得到权限模式；根据工作区计算用户级、项目级、本地级规则路径。
2. `permissions.LoadRuleSet` 加载三层规则文件，严格校验规则键、effect 和 glob 模式；缺失的规则文件视为空规则。
3. `cmd/mewcode` 创建 `permissions.Engine`、TUI `PermissionBridge` 和 Agent Runner。
4. 模型返回工具调用后，`agent.Scheduler` 按现有只读/副作用批次遍历每个调用。
5. Scheduler 对每个调用先发出 `EventToolCall`，再调用 `Engine.Decide`。
6. Engine 根据工具元数据构建权限请求，先检查命令黑名单，再检查路径沙箱，再按会话、本地、项目、用户规则匹配，最后按模式默认行为决定 allow、deny 或 ask。
7. allow：Scheduler 调用 `tools.Executor.Execute`，并按原有逻辑发出 `EventToolResult`。
8. deny：Scheduler 调用 `permissions.DeniedToolResult` 生成结构化失败结果，发出权限决策事件和工具结果事件，不执行真实工具。
9. ask：Scheduler 发出权限请求事件，并通过 `PermissionBridge.Confirm` 等待用户选择。
10. 用户选择本次允许：Scheduler 执行当前工具。
11. 用户选择本会话允许：Engine 写入会话规则，再执行当前工具。
12. 用户选择永久允许：Engine 写入本地级规则文件并刷新规则，再执行当前工具。
13. 用户拒绝：Scheduler 生成权限失败结果写回模型。
14. 用户取消或上下文取消：Scheduler 返回取消错误，Runner 发出取消终态，不启动剩余工具。
15. Agent Runner 将允许和拒绝的工具结果按原始工具调用顺序提交到会话或 Plan Mode 临时历史，保持既有历史边界。

## 文件组织

```text
mew01/
├── cmd/mewcode/
│   ├── main.go                         — 组装权限配置、规则加载、确认桥和 Runner
│   └── main_test.go                    — 启动配置与权限组装测试
├── internal/
│   ├── agent/
│   │   ├── event.go                    — 权限事件类型与事件字段
│   │   ├── mode.go                     — Options 增加权限引擎和确认桥
│   │   ├── runner.go                   — 创建带权限门禁的 Scheduler
│   │   ├── scheduler.go                — 执行前权限判定与确认等待
│   │   ├── scheduler_test.go           — 多工具调度与权限结果顺序测试
│   │   └── runner_test.go              — Agent Loop 权限拒绝继续、取消确认测试
│   ├── config/
│   │   ├── config.go                   — PermissionConfig
│   │   ├── load.go                     — YAML 读取默认值
│   │   ├── validate.go                 — 权限模式校验
│   │   └── config_test.go              — 旧配置兼容与权限模式测试
│   ├── permissions/
│   │   ├── blacklist.go                — 危险命令正则
│   │   ├── blacklist_test.go
│   │   ├── config.go                   — 三层规则加载与本地规则写入
│   │   ├── config_test.go
│   │   ├── engine.go                   — 权限决策主流程
│   │   ├── engine_test.go
│   │   ├── parser.go                   — 规则键解析和匹配
│   │   ├── parser_test.go
│   │   ├── path.go                     — 真实路径沙箱
│   │   ├── path_test.go
│   │   ├── request.go                  — 工具调用转权限请求
│   │   ├── request_test.go
│   │   ├── result.go                   — 权限失败工具结果
│   │   └── result_test.go
│   ├── tools/
│   │   ├── tool.go                     — Metadata 增加 PermissionMetadata
│   │   ├── read_file.go                — 声明 path 匹配和路径参数
│   │   ├── write_file.go               — 声明 path 匹配和路径参数
│   │   ├── edit_file.go                — 声明 path 匹配和路径参数
│   │   ├── run_command.go              — 声明 command 匹配
│   │   ├── find_files.go               — 声明 pattern 匹配
│   │   ├── search_code.go              — 声明 pattern 匹配
│   │   └── tools_test.go               — 元数据和旧工具行为回归
│   └── tui/
│       ├── model.go                    — 增加 pending permission 状态
│       ├── update.go                   — 权限确认按键处理
│       ├── view.go                     — 展示权限提示和状态
│       ├── permissions.go              — TUI PermissionBridge
│       └── tui_test.go                 — 确认交互测试
├── config.example.yaml                 — 示例 permissions.mode
├── docs/
│   ├── README.md                       — 第六章索引
│   └── ch06/
│       ├── spec.md
│       ├── plan.md
│       ├── task.md
│       └── checklist.md
└── .gitignore                          — 忽略 .mewcode/permissions.local.yaml
```

## 技术决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 权限系统位置 | 新增 `internal/permissions` 包，Scheduler 执行前调用 | 权限逻辑集中、可测试，不污染每个工具实现，也能覆盖所有 Agent 工具调用入口 |
| 规则文件格式 | `rules` 映射，键为 `工具名(模式)`，值为 `allow` 或 `deny` | 符合需求里的规则表达，简单可审阅，严格校验容易 |
| 权限模式位置 | 主配置 `permissions.mode` | 模式是整体运行策略，不和三层规则混在一起；旧配置可默认 `default` |
| 三层规则路径 | 用户级 `~/Library/Application Support/mewcode/permissions.yaml`，项目级 `.mewcode/permissions.yaml`，本地级 `.mewcode/permissions.local.yaml` | 用户默认、项目共享、本机私有三类用途清晰；本地级适合永久允许 |
| 规则优先级 | 黑名单、沙箱、会话、本地、项目、用户、模式、确认 | 严格满足 spec；显式 deny 不被放行模式覆盖 |
| 命令匹配对象 | `command` 与 `args` 以 shell-like quoting 规范化成一行文本 | 用户可写 `run_command(git *)`；同时不启用 shell 展开，保留现有执行模型 |
| 路径沙箱 | 对项目根和输入路径解析真实路径后做边界判断 | 防止符号链接逃逸；写新文件时解析已存在父目录即可覆盖关键风险 |
| 人在回路 | `PermissionBridge` 阻塞确认，TUI 通过桥回传选择 | 保持 Agent Loop 的顺序语义，测试可用脚本化确认器 |
| 权限拒绝 | 转成 `permission_error` 工具结果写回模型 | 与现有工具失败反馈一致，让模型调整策略而不中断循环 |
| Plan Mode 关系 | 先按只读过滤工具，再经过权限系统 | 不扩大规划阶段工具集合，同时允许用户对只读路径继续加规则 |
| 永久允许写入 | 只追加本地级 allow 规则 | 避免交互操作修改共享项目规则或全局用户默认；符合本机私有覆盖 |
| 配置错误处理 | 规则文件存在但非法时失败；规则文件缺失时视为空 | 避免错误配置导致意外宽松；首次使用无需创建文件 |

## Spec 覆盖

| Spec | 设计归属 |
|------|----------|
| F1 | Scheduler 调用 `permissions.Engine.Decide` 后再执行工具 |
| F2, F3 | `permissions/blacklist.go` |
| F4, F5 | `permissions/path.go` 与工具 `PermissionMetadata.PathParams` |
| F6, F7 | `permissions/parser.go` |
| F8, F9, F10 | `RuleSet`、`RuleStore`、`engine.go` 决策顺序 |
| F11 | `permissions.Mode` 与模式默认决策 |
| F12, F15, F20 | `PermissionBridge`、权限事件、TUI 确认状态 |
| F13, F19 | `ApplyConfirmation` 与 `AppendLocalAllow` |
| F14 | `permissions/result.go` |
| F16 | `agent/scheduler.go` 权限集成 |
| F17, F18 | `permissions/config.go` 与 config 默认值 |
| F21 | `prepareRequest` 继续 Plan Mode 只读过滤，Runner/Scheduler 统一权限处理 |
