# MewCode Skill System Plan

## 架构概览

本章新增 `internal/skills` 作为 Skill 的唯一领域模块。它负责从两级目录发现定义、解析 frontmatter、校验元数据和工具白名单、管理已发现与已激活状态，并以候选快照方式实现原子刷新。它不执行模型请求、不直接操作 TUI，也不读取目录资源正文以外的任何附属资源。

`agent.Runner` 持有一个 `skills.Manager` 和原始工具注册表。每轮调用模型前，Runner 从 Manager 取得稳定快照：轻量目录、已激活 SOP、当前任务的执行模式及白名单；据此构建环境补充、生成可见工具的受限注册表，并向 Provider 发送同一视图的工具定义。工具执行也使用这份受限注册表，保证模型可见工具与实际可执行工具一致。

`LoadSkill` 是由 Skill Manager 适配为 `tools.Tool` 的系统级工具。它被固定添加到每一轮工具视图，执行时激活指定 Skill 并返回安全的名称、模式与说明；下一轮模型请求才看到完整 SOP。该顺序避免在同一轮内改变已发送的 prompt 与工具定义。

命令层从静态默认命令改为“内置命令 + 当前 Skill 命令”的可替换注册表。TUI 持有 Skill 服务，`/skills reload` 调用原子刷新并替换命令表；`/commit`、`/review`、`/test` 默认以 inline 方式构造带 Skill 名称和参数的 Agent 请求，以保留已讨论的需求与偏好。创建、恢复或清空会话时清除激活状态，不影响已发现的 Skill。

fork 模式由 Runner 建立临时、独立的会话快照执行。它不复用主会话，也不向主会话写入执行轮次；完成后 Runner 将最终文本整理为一个摘要回写主会话。历史范围决定临时会话的初始消息集：`full` 使用主历史完整快照，`recent` 使用最近五条消息，`none` 使用空历史。

## 核心数据结构

### `skills.Metadata` 与 `skills.Skill`

```go
type Mode string
const (
    ModeInline Mode = "inline"
    ModeFork   Mode = "fork"
)

type ContextScope string
const (
    ContextFull   ContextScope = "full"
    ContextRecent ContextScope = "recent"
    ContextNone   ContextScope = "none"
)

type Metadata struct {
    Name, Description, Model string
    Tools                    []string
    Mode                     Mode
    Context                  ContextScope
}

type Skill struct {
    Metadata
    Path string
    Body string
}
```

发现阶段读取入口 Markdown，并将 frontmatter 解析为 `Metadata`。单文件 Skill 的入口就是该文件；目录型 Skill 的入口固定为目录内 `SKILL.md`。`Name` 使用小写字母、数字和连字符，且不得与内置命令或其他有效 Skill 冲突。`Description` 为必填非空字符串。`Mode` 默认 `inline`；`Context` 默认 `full`，但仅在 fork 中使用。`Tools` 为空代表不额外收窄基础工具，非空时只保留列出的工具与系统级 `load_skill`。

### `skills.Catalog` 与 `skills.Manager`

```go
type Catalog struct {
    Skills map[string]Skill
}

type Activation struct {
    Name string
    Args string
}

type Snapshot struct {
    Catalog      Catalog
    Activations  []Activation
}

type Manager struct { /* mutex-protected catalog and activations */ }
```

`Catalog` 是一次完整发现的不可变候选结果。发现按“用户级 → 项目级”顺序合并，后者覆盖前者；校验通过后才可替换 Manager 当前 Catalog。`Manager.Reload` 先构建并校验候选 Catalog，再在单次锁定中替换；任何失败均不修改当前 Catalog 或激活列表。

`Manager.Activate(name, args)` 在当前 Catalog 中查找 Skill，记录一次激活并返回只含元信息的结果。激活列表按调用顺序保留，允许同名 Skill 以不同参数重复激活。`Manager.ClearActivations` 只清除会话态激活记录。`Manager.Snapshot` 返回深拷贝，供一轮 Agent 任务稳定使用。

### `skills.Runtime`

```go
type Runtime struct {
    ActivePrompts []string
    AllowedTools  []string
    Mode          Mode
    Context       ContextScope
    Model         string
}
```

Manager 从 `Snapshot` 和某次请求的显式 Skill 选择构建 Runtime。每项激活 Skill 的正文在这里将 `{{args}}` 替换为调用参数，并按名称包裹成可辨识的 SOP 段落。多项白名单取交集，防止任一激活 Skill 扩大工具面；若交集为空，仍保留 `load_skill`。多项指定模型不同时返回可诊断错误，而非静默选择其一。

### Agent 请求与 Provider 请求

```go
type Request struct {
    Mode   Mode
    Prompt string
    Skill  *SkillInvocation
}

type SkillInvocation struct {
    Name string
    Args string
}
```

显式 Skill 命令创建 `SkillInvocation`。Runner 先读取 Skill 元数据：inline 在主会话中运行；fork 转入独立执行路径。`provider.ChatRequest` 增加可选 `Model` 字段；Anthropic 与 OpenAI 请求构造器优先使用请求模型，未指定时继续使用客户端配置的默认模型。

## 模块设计

### `internal/skills`

**职责：** 定义 YAML frontmatter 格式；扫描两级路径；解析并校验单文件和目录型入口；执行优先级合并；管理原子 Catalog、激活列表、轻量目录和运行时视图；提供 `LoadSkill` 工具和样板 Skill。

**对外接口：** `NewManager`、`Discover`、`Reload`、`Snapshot`、`Activate`、`ClearActivations`、`RuntimeFor`、`DirectoryPrompt`、`NewLoadTool`、`BuiltinFS`。

**依赖：** 标准库文件系统与同步原语、`go.yaml.in/yaml/v4`、`internal/tools` 的 Tool 接口。它不依赖 agent、command、tui 或 provider，避免循环依赖。

**解析与校验：** frontmatter 必须由首行 `---` 到下一个 `---` 包围，正文必须非空。目录中存在 `SKILL.md` 时按目录型解析；其余顶层 `.md` 作为单文件 Skill。内置样板由嵌入式文件系统提供，并作为最低优先级来源。发现后结合基础工具注册表校验 `tools`，同时校验名称与内置命令冲突。

### `internal/tools`

**职责：** 在不复制工具实例的情况下生成名称过滤后的注册表。

**对外接口：** 增加 `Names`、`Require` 或等价只读查询，以及 `Subset(names, required...)`。`Subset` 对未知名称返回错误，并总是将系统级 `load_skill` 加入结果。

**依赖：** 保持现有 Provider 定义转换和执行器接口，不了解 Skill frontmatter 或会话状态。

### `internal/agent`

**职责：** 将 Skill Manager 纳入 Options 与 Runner；每轮构建稳定 Runtime、可见工具注册表和可选模型；执行 inline 与 fork；完成后维护主会话历史与 Token 用量。

**对外接口：** `Options` 增加 `Skills *skills.Manager`，`Request` 增加显式 `SkillInvocation`。`Runner` 暴露受限的 Skill 服务访问器给 TUI 命令协调层。

**inline 路径：** 显式 invocation 先激活 Skill；每轮从 Manager Snapshot 生成 `prompt.OptionalModules.ActiveSkills`，并把轻量目录以独立可选模块注入系统 prompt。Runner 对当前 Runtime 的工具集合调用 `CollectEnvironment`、`Definitions`、调度器与执行器。

**fork 路径：** 对应 Skill 不进入主 Runner 的普通 round 提交路径。Runner 创建无 journal 的临时 `conversation.Session`，按 context scope 复制历史，使用同一 Provider、权限引擎、工具执行器与 Skill Runtime 完成任务；将最终回答裁剪为摘要后以合成的用户请求和助手摘要原子提交主会话。fork 产生的 Provider Token 用量仍累加到主会话，确保状态栏反映用户发起的实际成本。

**模型选择：** 运行时指定的 `Model` 写入所有该任务的 `provider.ChatRequest`，包括 fork；上下文压缩与后台记忆仍使用默认模型，避免 Skill 的局部模型意外影响既有后台任务。

### `internal/prompt`

**职责：** 在固定模块之后，以两个明确模块注入 Skill 目录和已激活 SOP。

**对外接口：** `OptionalModules` 新增 `AvailableSkills []string`；保留 `ActiveSkills []string`。`optionalModules` 生成优先级低于自定义指令、高于长期记忆的“可用 Skill”模块，以及优先级最高的“已激活 Skill”模块，确保完整 SOP 位于环境中最显眼的稳定位置。

**依赖：** 只接收已渲染字符串，不解析 Skill 文件，不关心工具过滤策略。

### `internal/command` 与 `internal/tui`

**职责：** 让动态 Skill 命令与 `/skills reload` 纳入现有解析、帮助和补全；在会话生命周期内清除激活状态。

**对外接口：** 命令注册表新增可组合构造函数，将内置命令、`skills reload` 命令和当前 Catalog 的 Skill 命令合并并执行重复名校验。`CommandContext` 增加最小化 `SkillService` 接口（刷新、命令列表、显式调用、清除激活）。

**交互：** `/skills reload` 先执行 Manager 原子刷新，再重建 TUI 的 command Registry；失败时显示诊断且不替换当前 Registry。`/commit`、`/review`、`/test` 和用户发现的 Skill 命令调用 `StartAgent`，携带 `SkillInvocation`。`New`、`Resume`、`Clear` 对应的会话切换路径调用 `ClearActivations`；任务完成摘要按现有视图显示。

### `internal/provider`

**职责：** 支持请求级模型覆盖。

**对外接口：** `ChatRequest.Model string`。Anthropic 与 OpenAI 的 `buildRequest` 根据非空请求模型覆盖各客户端默认模型，并在各自测试中验证覆盖与回退。

### `cmd/mewcode`、样板与文档

**职责：** 在所有本地工具和 MCP 工具注册完成后创建 Skill Manager，传入完整工具目录和项目/用户路径；将其注入 Runner 和 TUI。通过 `embed` 或稳定的内置来源提供 `commit`、`review`、`test` 三项样板。

**文档：** 根 `README.md` 新增 Skill 目录、frontmatter、触发方式、`/skills reload` 和样板说明，并说明内置 `/review` 默认 inline；`.mewcode/config.example.yaml` 无本章新增配置项，补充说明 Skill 路径与无需配置的默认行为；`docs/README.md` 增加 ch11 索引。

## 模块交互

```text
启动
  ├─ 建立基础工具与 MCP 工具 Registry
  ├─ skills.Discover(内置 → 用户级 → 项目级, Registry.Names)
  ├─ skills.Manager 取得已校验 Catalog
  └─ Runner / TUI 共享 Manager，TUI 构建动态命令 Registry

普通自然语言任务
  ├─ Runner Snapshot → 可用 Skill 轻量清单 + 当前已激活 SOP
  ├─ Runtime 工具白名单 → tools.Subset(..., load_skill)
  ├─ Provider 接收同一工具视图与 prompt
  ├─ Agent 调用 load_skill(name)
  └─ 下一轮 Snapshot 注入完整 SOP 并按白名单继续

/skill-name args
  ├─ command 查 Catalog，创建 SkillInvocation
  ├─ Runner 根据 mode 选择 inline 或 fork
  ├─ inline：激活并进入主会话 Agent Loop
  └─ fork：建立临时会话执行，最终摘要提交主会话

/skills reload
  ├─ Manager 构建并校验候选 Catalog
  ├─ 失败：返回错误，原 Catalog / 命令 / 激活状态不变
  └─ 成功：一次替换 Catalog，TUI 重建命令 Registry
```

## 文件组织

```text
internal/skills/
├── skill.go                 — 元数据、Skill、模式和运行时类型
├── discover.go              — 两级发现、frontmatter 解析、优先级与校验
├── manager.go               — Catalog、激活状态、快照与原子刷新
├── runtime.go               — SOP 渲染、{{args}} 替换、白名单交集
├── load_tool.go             — 系统级 LoadSkill 工具
├── builtins.go              — 内置样板来源与发现适配
├── testdata/                — 单文件、目录型、错误与覆盖样例
└── skills_test.go           — 发现、校验、刷新、激活与运行时测试
internal/tools/registry.go   — 名称查询与受限工具子集
internal/agent/event.go      — Skill invocation 与 Manager Options
internal/agent/runner.go     — 每轮 Skill Runtime、inline / fork 分流
internal/agent/runner_test.go — prompt、工具过滤、fork、模型与清理集成测试
internal/prompt/builder.go   — 可用 Skill 模块输入
internal/prompt/sections.go  — 目录与激活 SOP 的模块顺序
internal/prompt/prompt_test.go — 注入顺序和渐进式披露测试
internal/provider/provider.go — 请求级 Model 字段
internal/provider/{anthropic,openai}/request.go — 模型覆盖编码
internal/provider/{anthropic,openai}/*_test.go — 覆盖和默认模型测试
internal/command/command.go  — SkillService 与命令上下文
internal/command/builtins.go — `/skills reload` 与动态 Skill 命令
internal/command/registry.go — 内置与动态命令组合
internal/command/command_test.go — 帮助、补全、刷新和显式调用测试
internal/tui/model.go        — Skill 服务与会话切换清理
internal/tui/update.go       — 刷新后命令表替换
internal/tui/tui_test.go     — TUI 命令分流和清理测试
cmd/mewcode/main.go          — Manager 初始化及注入
README.md                    — Skill 用户说明
.mewcode/config.example.yaml — 无配置 Skill 路径说明
docs/README.md               — 第十一章索引
docs/ch11-skills/*           — 本章正式文档与人工场景
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| Skill 存储 | 内置、用户级、项目级三层发现；项目级覆盖用户级 | 同时满足开箱样板、个人复用与项目共享，且与既有指令/记忆路径一致。 |
| 目录入口 | 单文件 `.md` 或目录中的 `SKILL.md` | 入口确定，目录资源不自动注入，支持渐进式披露。 |
| frontmatter 解析 | `go.yaml.in/yaml/v4` | 项目已有依赖，避免新增解析器。 |
| 刷新机制 | `/skills reload` 的候选 Catalog 原子替换 | 行为可预测；失败可完整回滚到旧内存状态。 |
| 完整 SOP 位置 | Prompt 的稳定“已激活 Skill”模块，优先级高于其他可选模块 | 每轮稳定存在且在环境补充中醒目，满足持续注入。 |
| 自动发现 | 轻量目录 + 固定 `load_skill` 工具 | 启动低成本，模型按意图选择后才读取 SOP。 |
| 工具收窄 | 每轮从原始 Registry 生成受限视图，prompt、定义、调度共享该视图 | 防止“模型看不见但仍可执行”或反向不一致。 |
| 多 Skill 白名单 | 白名单交集；空白名单表示不额外限制 | 多项 SOP 同时生效时取最小权限集合，避免权限面扩大。 |
| fork 实现 | 临时无 journal Session 执行，最终摘要提交主 Session | 隔离上下文和中间过程，同时保留用户可追溯的结果与 Token 成本。 |
| 模型选择 | `ChatRequest.Model` 覆盖客户端默认模型 | Skill 可局部指定模型，不改变全局配置或后台任务。 |
