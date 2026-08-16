# MewCode System Prompt Plan

## 架构概览

本章采用 Provider 中立 Prompt Bundle 架构。Agent 层不直接拼 OpenAI `instructions`、Anthropic `system/cache_control` 等字段，而是每轮构建一个统一的提示词包：稳定系统指令、动态系统补充、当前用户任务、工具定义、缓存偏好和环境信息。Provider 适配器只负责把这个包映射到各自 API 请求体。

API 约束依据：

- OpenAI prompt caching 依赖精确前缀匹配，静态内容应放在前面，Responses API 支持系统/开发者级 instructions，并通过 `cached_tokens` 观测命中。
- Anthropic prompt caching 的缓存前缀顺序是 `tools -> system -> messages`，可用 `cache_control` 标记断点，并通过 `cache_creation_input_tokens` / `cache_read_input_tokens` 观测缓存表现。

核心组件：

1. **Prompt Builder**：负责拼装七个固定模块和可选模块，输出稳定的系统指令文本。模块有固定 key、标题、优先级和缓存属性；相同输入下输出必须完全一致。
2. **Environment Collector**：负责收集工作区路径、OS、Shell、当前日期、任务模式和可用工具范围，并生成动态环境段。它不写入稳定系统指令，避免破坏缓存前缀。
3. **Mode Injector**：负责 `/plan`、`/do`、普通执行模式的系统级补充指令。`/plan` 的只读约束从用户提示前缀迁移到动态补充消息；首轮完整注入，间隔轮次注入关键提醒，其余轮次精简或跳过。
4. **Tool Description Enhancer**：在工具原有 metadata 基础上追加稳定规则强化文本，例如“编辑前先读取”“搜索优先使用搜索工具”“规划模式不得请求副作用工具”。增强后的工具定义在相同工具集合下稳定输出。
5. **Provider Prompt Adapter**：OpenAI 适配器把稳定指令映射到 `instructions` 或系统/开发者消息，把动态补充作为高优先级系统/开发者消息放在用户任务前；Anthropic 适配器把稳定指令映射到 top-level `system` blocks，并在支持时给稳定工具/系统段添加缓存断点。Agent 层只看到统一结构。
6. **Cache Usage Normalizer**：Provider 流解析层把缓存相关 usage 归一化到 `provider.Usage`：OpenAI 映射 `cached_tokens`；Anthropic 映射 `cache_creation_input_tokens`、`cache_read_input_tokens`，以及可选的 5m/1h 细分。Agent 事件沿用现有 usage 汇总机制扩展字段。

数据流：

```text
TUI/Request
  -> agent.prepareRequest 只整理用户任务和计划文本
  -> prompt.CollectEnvironment 收集环境、模式、工具范围
  -> prompt.Builder 生成 PromptBundle
  -> provider.ChatRequest 携带 PromptBundle + Messages + Tools
  -> OpenAI/Anthropic Adapter 映射请求体与缓存标记
  -> Stream parser 归一化普通 token + cache usage
  -> Agent Event 输出每轮和累计用量
```

## 核心数据结构

### `prompt.Module`

表示一个系统提示模块。

```go
type Module struct {
	Key       ModuleKey
	Title     string
	Priority  int
	Content   string
	Stable    bool
	Optional  bool
}
```

字段说明：

- `Key`：模块稳定标识，用于排序、测试和去重。
- `Title`：输出到提示词中的标题。
- `Priority`：模块优先级，数值越小越靠前。
- `Content`：模块正文。
- `Stable`：是否属于稳定缓存段。
- `Optional`：是否为可选模块；可选模块为空时不输出。

### `prompt.ModuleKey`

```go
type ModuleKey string
```

固定值：

```go
const (
	ModuleIdentity       ModuleKey = "identity"
	ModuleSystemRules    ModuleKey = "system_rules"
	ModuleTaskMode       ModuleKey = "task_mode"
	ModuleAction         ModuleKey = "action"
	ModuleToolUse        ModuleKey = "tool_use"
	ModuleTone           ModuleKey = "tone"
	ModuleOutput         ModuleKey = "output"
	ModuleEnvironment    ModuleKey = "environment"
	ModuleCustom         ModuleKey = "custom"
	ModuleSkills         ModuleKey = "skills"
	ModuleMemory         ModuleKey = "memory"
)
```

### `prompt.Environment`

每轮动态环境信息。

```go
type Environment struct {
	Workspace string
	OS        string
	Shell     string
	Date      string
	Mode      Mode
	Tools     []string
}
```

说明：`Date` 由可注入 clock 生成，测试中固定；`Tools` 使用当前请求实际开放的工具集合，Plan Mode 只显示只读工具。

### `prompt.Mode`

提示词包内部使用的任务模式枚举，避免 `internal/prompt` 依赖 `internal/agent`。

```go
type Mode string

const (
	ModeAct  Mode = "act"
	ModePlan Mode = "plan"
	ModeDo   Mode = "do"
)
```

Agent 层将 `agent.Mode` 显式转换为 `prompt.Mode`。

### `prompt.ModeInjection`

描述某一轮要注入的模式补充指令。

```go
type ModeInjection struct {
	Kind      InjectionKind
	Content   string
	Cacheable bool
}
```

```go
type InjectionKind string

const (
	InjectionFull     InjectionKind = "full"
	InjectionReminder InjectionKind = "reminder"
	InjectionBrief    InjectionKind = "brief"
	InjectionNone     InjectionKind = "none"
)
```

### `prompt.InjectionPolicy`

控制会话级模式指令注入频率。

```go
type InjectionPolicy struct {
	ReminderEvery int
	BriefEvery    int
}
```

默认策略：首轮 `full`；之后每 `ReminderEvery` 轮输出关键规则提醒；其余轮次按 `BriefEvery` 决定是否输出精简提醒。默认可取 `ReminderEvery=3`、`BriefEvery=1`，具体值留给实现测试校准。

### `provider.PromptBundle`

Provider 中立的提示词包。

```go
type PromptBundle struct {
	StableSystem  string
	DynamicSystem []SystemMessage
	CachePolicy   CachePolicy
}
```

说明：

- `StableSystem`：七个固定稳定模块加稳定可选模块。
- `DynamicSystem`：环境信息、模式补充等运行时系统级消息。
- `CachePolicy`：向 Provider 表达哪些段落可缓存。

### `provider.SystemMessage`

```go
type SystemMessage struct {
	Tag       string
	Content   string
	Cacheable bool
}
```

`Tag` 使用稳定标签，例如 `mew.environment`、`mew.mode.plan`、`mew.mode.do`。Provider 适配器可把标签渲染为 XML 风格文本块或等效 system/developer 内容。

### `provider.CachePolicy`

```go
type CachePolicy struct {
	Enable          bool
	StableSystem    bool
	StableTools     bool
	DynamicMessages bool
}
```

默认启用稳定系统和稳定工具缓存，不缓存动态消息。

### `provider.ChatRequest` 扩展

```go
type ChatRequest struct {
	Prompt    PromptBundle
	Messages  []Message
	MaxTokens int
	Thinking  ThinkingOptions
	Tools     []ToolDefinition
}
```

### `provider.ToolDefinition` 扩展

```go
type ToolDefinition struct {
	Name        string
	Description string
	Schema      map[string]any
	Cacheable   bool
}
```

### `provider.Usage` 扩展

```go
type Usage struct {
	InputTokens  int
	OutputTokens int

	CacheReadInputTokens     int
	CacheCreationInputTokens int
	CacheUnavailable         bool
}
```

如果 Provider 有更细的缓存写入窗口字段，可先在内部解析，统一字段只暴露本章需要的创建与读取总量。

## 核心接口

```go
func BuildBundle(ctx BuildContext) (provider.PromptBundle, []Module, error)
```

构建稳定系统提示、动态系统消息、缓存策略，并返回模块列表供测试审阅。

```go
type BuildContext struct {
	Environment     Environment
	Mode            Mode
	Iteration       int
	InjectionPolicy InjectionPolicy
	OptionalModules OptionalModules
}
```

```go
type OptionalModules struct {
	CustomInstructions []string
	ActiveSkills       []string
	LongTermMemory     []string
}
```

```go
func EnhanceDefinitions(defs []provider.ToolDefinition, mode Mode) []provider.ToolDefinition
```

为工具描述追加稳定规则强化，并设置 `Cacheable=true`。

```go
func ModeInjectionFor(mode Mode, iteration int, policy InjectionPolicy) ModeInjection
```

按模式和轮次生成动态补充指令。

```go
func CollectEnvironment(mode Mode, registry *tools.Registry, workspace string, clock Clock) (Environment, error)
```

收集动态环境信息；缺少工作区或工具范围时返回错误。Shell 优先取 `SHELL`，否则取 Windows 的 `COMSPEC`，两者均缺失时使用稳定回退值，确保环境描述不阻断请求。

```go
type Clock interface {
	Now() time.Time
}
```

## 模块设计

### `internal/prompt`

**职责：** 承载本章新增的提示词核心逻辑：模块定义、稳定系统提示拼装、动态系统补充、环境信息渲染、模式注入策略、工具描述增强和人工评估场景文本。

**对外接口：**

- `BuildBundle(ctx BuildContext) (provider.PromptBundle, []Module, error)`
- `CollectEnvironment(mode Mode, registry *tools.Registry, workspace string, clock Clock) (Environment, error)`
- `ModeInjectionFor(mode Mode, iteration int, policy InjectionPolicy) ModeInjection`
- `EnhanceDefinitions(defs []provider.ToolDefinition, mode Mode) []provider.ToolDefinition`

**依赖：** 依赖 `internal/provider` 的请求承载类型、`internal/tools` 的工具注册表。不得依赖具体 Provider 子包或 `internal/agent`。

**设计要点：**

- 固定模块用静态函数返回，避免运行时顺序漂移。
- 动态环境模块不进入 `StableSystem`，而是以 `SystemMessage{Tag:"mew.environment"}` 放入 `DynamicSystem`。
- Shell 仅作为环境描述字段：优先使用 `SHELL`，缺失时使用 `COMSPEC`，再缺失时使用稳定回退值；不因平台特有变量缺失而终止 Agent 请求。
- 可选模块先保留入口，不接项目指令加载、真实 Skill 或记忆系统。
- 提示词内容使用稳定标题；正文可以中文/英文混合，但同一实现必须稳定，便于测试。

### `internal/agent`

**职责：** 在每轮模型调用前生成 Prompt Bundle，并把用户任务、会话历史、增强后工具定义一起传给 Provider。继续负责 Agent Loop、历史提交和事件流，不承担 Provider 请求体细节。

**改动点：**

- `Runner` 增加 prompt 相关配置：workspace 路径、clock、injection policy、cache policy。
- `prepareRequest` 不再把 `/plan` 系统约束拼到用户 prompt 里；只保留用户任务文本。
- `/do` 仍生成包含全部 pending plans 的任务上下文，但去掉长期工具/行为规则。
- 每轮调用 Provider 前，根据当前 mode、iteration、registry 构建 `provider.PromptBundle`。
- `provider.ChatRequest` 携带 `Prompt` 和增强后工具定义。
- 现有 Plan Mode 临时历史、CommitPlan、ConsumePlans 语义保持不变。

**依赖：** 依赖 `internal/prompt`、`internal/provider`、`internal/tools`、`internal/conversation`。

### `internal/tools`

**职责：** 继续定义工具 metadata、schema、安全分类和执行行为。工具执行不因本章改变。

**改动点：**

- 保持工具原始 metadata 简洁稳定。
- 工具描述增强由 Agent 层发送前调用 `prompt.EnhanceDefinitions` 完成，避免 `tools` 包依赖 `prompt`。
- 工具描述增强文本必须稳定，不能包含动态工作区、日期或模式轮次。

**依赖：** 不新增对 `internal/prompt` 的依赖，避免环。

### `internal/provider`

**职责：** 扩展统一请求模型与用量模型，承载 Provider 中立的提示词包和缓存用量字段。

**改动点：**

- `ChatRequest` 增加 `Prompt PromptBundle`。
- `ToolDefinition` 增加 `Cacheable bool`。
- `Usage` 增加缓存读取/创建字段和不可用标记。
- `Clone` 相关逻辑不需要复制 Prompt，因为 Prompt 不进入会话历史；Provider 请求体构建时只读使用。

**依赖：** 不依赖 `internal/prompt`，避免 import cycle。

### `internal/provider/openai`

**职责：** 把统一提示词包映射到 OpenAI Responses 请求格式，并解析 OpenAI usage 中的缓存命中字段。

**映射策略：**

- `StableSystem` 放在请求最前面的 `instructions` 或等效 system/developer input item。
- `DynamicSystem` 按顺序放在用户任务前，使用 system/developer 语义；如果当前 API 形态无法表达多个 system/developer items，则渲染为带 `<mew.system tag="...">...</mew.system>` 的高优先级补充文本。
- 工具定义保持稳定顺序，`Cacheable` 不暴露给不需要显式缓存标记的 API，但用于测试断言。
- usage 解析 `cached_tokens` 并写入统一 `Usage.CacheReadInputTokens`；无字段时设置不可用或零值。

**依赖：** 只依赖 `internal/provider`。

### `internal/provider/anthropic`

**职责：** 把统一提示词包映射到 Anthropic Messages 请求格式，并解析 Anthropic cache usage 字段。

**映射策略：**

- 工具列表保持在请求 `tools` 字段，工具描述稳定；支持时在最后一个稳定工具或稳定系统 block 添加 `cache_control`。
- `StableSystem` 作为 top-level `system` block，支持时加缓存断点。
- `DynamicSystem` 作为后续 system blocks 或带特殊标签的 system 文本，默认不加缓存控制。
- Messages 历史保持原有 user/assistant/tool_use/tool_result 映射。
- usage 解析 `cache_creation_input_tokens`、`cache_read_input_tokens`，以及可能出现的 5m/1h 写入细分，汇总到统一字段。

**依赖：** 只依赖 `internal/provider`。

### `internal/tui`

**职责：** 保持展示 Agent 事件、当前任务、工具调用和结果；不展示系统级动态补充消息。

**改动点：**

- 普通用户输入展示不变。
- `/plan` 展示仍显示 `/plan <任务>`，但模型请求中用户消息只包含任务本身。
- 用量展示如果已有 token 显示，可追加缓存读/写信息；如果当前 UI 暂无细分展示，本章可只通过事件和测试暴露，UI 展示留到后续优化。

### `docs/ch05-system_prompt/manual_scenarios.md`

**职责：** 记录人工对比场景，覆盖 spec 的 AC13。它不是自动测试，不要求跑真实模型。

**内容：**

- 场景名称。
- 用户输入。
- 观察点。
- 改造后期望行为。
- 可能的失败信号。

## 模块交互

### 普通执行模式

```text
TUI 输入普通任务
  -> agent.prepareRequest
     - 保留用户原始任务文本
     - 使用完整工具 registry
  -> agent.run 第 N 轮
     - prompt.CollectEnvironment(mode=act, tools=完整工具集合)
     - prompt.BuildBundle(iteration=N)
     - prompt.EnhanceDefinitions(mode=act)
  -> provider.ChatRequest
     - Prompt: 稳定系统指令 + 动态系统补充
     - Messages: 会话历史 + 当前用户任务
     - Tools: 增强后的稳定工具定义
  -> Provider adapter
     - 映射系统提示、工具和缓存标记
  -> Stream parser
     - 返回文本、工具调用和 usage/cache usage
  -> Agent Loop
     - 按第 4 章逻辑收集、调度工具、提交历史
```

### Plan Mode

```text
/plan <任务>
  -> agent.prepareRequest
     - prompt = "<任务>"，不再拼接只读系统约束
     - registry = 只读工具集合
     - displayPrompt = "/plan <任务>"
  -> prompt.BuildBundle
     - StableSystem: 固定系统指令
     - DynamicSystem:
       - mew.environment: mode=plan, tools=只读工具
       - mew.mode.plan: 首轮完整只读规划规则，后续按策略提醒
  -> Provider request
     - 用户消息只包含任务本身
     - 系统级补充承载只读探索和最终计划要求
  -> Agent Loop
     - 临时 planHistory 保持第 4 章语义
     - 成功后只提交 displayPrompt + 最终计划到展示历史和 pendingPlans
```

### Do Mode

```text
/do
  -> agent.prepareRequest
     - prompt = 待执行计划列表，按顺序编号
     - registry = 完整工具集合
  -> prompt.BuildBundle
     - DynamicSystem 包含 mode=do 与完整工具范围
     - 不包含 Plan Mode 只读补充
  -> Provider request
     - 用户任务携带全部计划
     - 系统提示承担通用执行、工具和环境规则
  -> Agent Loop
     - 正常完成后 ConsumePlans
     - 非成功终态保留计划
```

### 缓存用量回流

```text
Provider raw stream/done event
  -> provider/openai 或 provider/anthropic stream parser
  -> provider.Usage{
       InputTokens,
       OutputTokens,
       CacheReadInputTokens,
       CacheCreationInputTokens,
       CacheUnavailable,
     }
  -> agent total.Add(round.Usage)
  -> EventUsage / Summary.Usage
  -> 测试或 TUI 可观察
```

## 文件组织

```text
internal/
├── prompt/
│   ├── module.go              — Module、ModuleKey、排序与渲染
│   ├── sections.go            — 七个固定模块和可选模块构造
│   ├── environment.go         — Environment、CollectEnvironment、环境段渲染
│   ├── mode.go                — Mode、ModeInjection、InjectionPolicy、ModeInjectionFor
│   ├── builder.go             — BuildContext、BuildBundle
│   ├── tools.go               — EnhanceDefinitions 与工具规则强化文本
│   ├── clock.go               — Clock 接口和系统 clock
│   └── prompt_test.go         — 模块顺序、动态隔离、模式注入、工具增强测试
├── provider/
│   ├── provider.go            — ChatRequest、ToolDefinition、Usage 扩展
│   ├── prompt.go              — PromptBundle、SystemMessage、CachePolicy
│   ├── openai/
│   │   ├── request.go         — OpenAI prompt/tool/cache 请求映射
│   │   ├── stream.go          — OpenAI cache usage 解析
│   │   └── openai_test.go     — 请求快照与 cached_tokens 测试
│   └── anthropic/
│       ├── request.go         — Anthropic system/tool/cache_control 映射
│       ├── stream.go          — Anthropic cache usage 解析
│       └── anthropic_test.go  — 请求快照与 cache usage 测试
├── agent/
│   ├── mode.go                — prepareRequest 去除 /plan 用户前缀规则
│   ├── runner.go              — 每轮构建 PromptBundle 与增强工具定义
│   └── runner_test.go         — Plan/Do 注入隔离与回归测试
└── tui/
    └── tui_test.go            — 展示历史不含系统补充消息

docs/
├── README.md                  — ch05 索引
└── ch05-system_prompt/
    ├── spec.md
    ├── plan.md
    ├── task.md
    ├── checklist.md
    └── manual_scenarios.md    — 人工对比场景
```

## 依赖方向

```text
agent -> prompt -> provider
agent -> tools
prompt -> tools
provider/openai -> provider
provider/anthropic -> provider
tui -> agent/provider display types
```

为避免 `prompt -> agent` 和 `agent -> prompt` 形成环，采用 `prompt.Mode` 独立枚举，Agent 层做显式转换。

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 提示词核心边界 | 新增 `internal/prompt` 包 | 把提示词拼装、环境注入、模式提醒和工具描述增强集中管理，避免继续散落在 Agent 和 Provider 请求构造里。 |
| Provider 中立模型 | `provider.ChatRequest` 携带 `provider.PromptBundle` | Agent 层不感知 OpenAI/Anthropic 字段，满足 F6/N1；Provider 子包只做映射。 |
| Prompt 类型归属 | `PromptBundle/SystemMessage/CachePolicy` 放在 `internal/provider` | 避免 `provider -> prompt -> provider` import cycle；`prompt` 包生成 provider 请求承载类型。 |
| 模式类型 | `internal/prompt` 定义独立 `Mode`，Agent 显式转换 | 避免 `prompt` 依赖 `agent`，保持依赖方向单向。 |
| 稳定与动态分离 | `StableSystem` + `DynamicSystem[]` | 稳定段可快照测试和缓存，动态段每轮变化且不污染历史。 |
| 系统级补充表示 | 带 `Tag` 的 `SystemMessage` | 统一表达环境、模式、临时系统约束；Provider 可映射为 system/developer block 或带标签文本。 |
| `/plan` 用户文本 | 用户消息只保留任务本身 | 只读约束迁移到动态系统补充，解决“系统约束伪装成用户输入”的问题。 |
| `/do` 用户文本 | 继续携带全部计划，但去掉长期行为规则 | 计划本身是任务上下文，应保留在用户任务；通用规则由系统提示承担。 |
| 工具描述增强位置 | Agent 层在发送前增强 definitions | 避免修改工具执行层和工具原始 metadata，也避免 `tools` 包依赖 `prompt`。 |
| 缓存策略 | 默认缓存稳定系统指令和稳定工具定义，不缓存动态消息 | 符合 OpenAI 前缀缓存和 Anthropic tools/system/messages 缓存顺序，也满足 F4/F5。 |
| OpenAI 映射 | 静态系统指令放最前，动态补充放用户任务前；缓存命中从 `cached_tokens` 归一化 | 利用 OpenAI 前缀缓存特性；不在 Agent 层暴露 OpenAI 专有字段。 |
| Anthropic 映射 | 工具与稳定 system block 加缓存断点，动态 system block 不缓存 | 符合 Anthropic 缓存顺序和 `cache_control` 语义；动态内容变化不会破坏稳定段。 |
| Usage 扩展 | 在 `provider.Usage` 增加缓存读取/创建字段 | 复用第 4 章已有用量事件和 Summary 汇总，不额外设计事件流。 |
| 环境日期 | 通过 `Clock` 注入 | 保证测试确定性，避免快照随真实日期漂移。 |
| Shell 解析优先级 | `SHELL` → `COMSPEC` → 稳定回退值 | 保留 Unix 已有信息；兼容 Windows 默认终端；环境描述字段不应成为请求可用性的单点故障。 |
| 人工评估 | 新增 `manual_scenarios.md` | 满足 F17，同时不扩大为自动评估系统，守住“不做的事”。 |

## Spec 覆盖检查

- F1-F3：由 `internal/prompt` 的模块、排序、可选模块覆盖。
- F4-F8：由 `PromptBundle`、`SystemMessage`、`Environment`、Provider Adapter 覆盖。
- F9-F10：由 `EnhanceDefinitions` 和固定工具使用模块覆盖。
- F11-F13：由 `ModeInjectionFor`、Agent 每轮构建和 `prepareRequest` 调整覆盖。
- F14/F18：由 Agent 维持第 4 章历史提交、Plan/Do 逻辑和回归测试覆盖。
- F15-F16：由 `provider.Usage` 扩展、OpenAI/Anthropic stream 解析和请求快照测试覆盖。
- F17：由 `docs/ch05-system_prompt/manual_scenarios.md` 覆盖。
- F19：由 `CollectEnvironment` 的跨平台 Shell 解析和离线回归测试覆盖。

## 设计风险

- OpenAI 和 Anthropic 对系统消息及缓存标记的 API 形态不同，计划中通过 Provider 子包隔离；实现阶段需要用请求快照测试锁住实际 JSON。
- 如果某个 Provider 当前模型不返回缓存字段，统一 Usage 只能报告不可用或零值；这符合 spec，但验收需要覆盖“不可用不失败”。
- 系统级补充消息的“不会被当作用户输入”最终取决于 Provider 支持形态；实现中应优先使用原生 system/developer 语义，标签文本只作为退化表达。
