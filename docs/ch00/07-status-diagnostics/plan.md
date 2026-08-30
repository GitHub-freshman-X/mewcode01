# `/status` 诊断信息 Plan

## 架构概览

新增独立的 `internal/status` 只读状态 DTO，由 Agent Runner 聚合现有运行时元数据。`/status` 命令取得单次状态快照，再与 UI 提供的模式、会话 ID/消息数和累计 Token 组合为固定多行文本。

```text
/status
  → command.statusCommand
  → StatusService.Snapshot()
      → Runner：工作区、权限、实际工具数、Skill、SubAgent 注册表
      → Memory：缓存的用户/项目数量
      → TaskManager：同一锁保护下的运行中后台任务快照
  → 格式化安全字段
  → TUI 系统消息
```

## 核心接口

### `internal/status`

```go
type BackgroundTask struct {
    ID, Name string
    Status   string // 仅 running
    Elapsed  time.Duration
}

type Snapshot struct {
    Workspace, LogDirectory, PermissionMode string
    ToolCount, SkillCount                    int
    UserMemoryCount, ProjectMemoryCount      int
    MemoryAvailable                          bool
    SubAgentDefinitionCount                  int
    BackgroundTasks                          []BackgroundTask
}

type Provider interface {
    StatusSnapshot() Snapshot
}
```

`Snapshot` 不含任务描述、结果、失败原因、Prompt 或配置正文。格式化层只处理此 DTO，不直接接触运行时内部对象。

### `internal/agent.Runner`

新增 `StatusSnapshot()`，实现 `status.Provider`。它读取 `Options.Workspace`、新增的 `Options.LogDirectory`、权限引擎、实际 Runner registry、Skill 目录、Memory 缓存及 SubAgent 运行时，生成单次快照。

### `internal/subagent`

`LaunchRequest` 和 `TaskInfo` 增加 `Background bool`。`TaskManager` 新增：

```go
func (m *TaskManager) MarkBackground(id string) bool
func (m *TaskManager) RunningBackground() []TaskInfo
```

两者均在 TaskManager 锁保护下工作。`RunningBackground` 只复制 `running && background` 任务，并按 `StartedAt`、ID 稳定排序。ESC 和 120 秒超时接管调用 `MarkBackground`；显式后台和 Fork 在创建时带上后台标记。

### `internal/memory`

Memory Service 新增只读的缓存摘要。启动时初始化；`/memory add`、`/memory clear` 和自动提取成功后更新。`/status` 仅读取该缓存，不扫描目录。

### `internal/command`

`CommandContext` 增加 `status.Provider`。`statusCommand` 合并：

- UI：模式和累计 Token；
- SessionService：会话 ID、消息数；
- Status Provider：环境、扩展和后台任务快照。

Provider 缺失时保留已有模式与 Token，扩展字段显示为不可用。

## 模块交互

```text
main
  → 创建 Runner（Workspace、LogDirectory、权限、注册表、Memory、SubAgents）
  → TUI Model 持有 Runner

用户输入 /status
  → command.statusCommand
  → UI：模式、累计 Token
  → SessionService：会话 ID、消息数
  → Runner.StatusSnapshot()
      → registry / skills / memory cache / subagent registry
      → TaskManager.RunningBackground()
  → 固定多行安全文本 → TUI 系统消息
```

后台接管路径：

```text
前台子 Agent
  ├─ ESC → TaskManager.MarkBackground(taskID) → async 返回
  └─ 120 秒 → TaskManager.MarkBackground(taskID) → async 返回
```

任务标记与终态更新均由同一锁保护，避免 `/status` 把已完成任务显示为运行中后台任务。

## 文件组织

```text
internal/status/
├── status.go          — Snapshot、BackgroundTask、Provider 接口
└── status_test.go     — 数据脱敏和快照字段测试

internal/agent/
├── event.go           — Options 增加 LogDirectory
├── runner.go          — StatusSnapshot 运行时聚合
├── subagent.go        — ESC/超时接管时标记后台
└── *_test.go          — 实际 registry、扩展和后台状态集成测试

internal/subagent/
├── task_manager.go    — Background 字段、MarkBackground、RunningBackground
└── *_test.go          — 状态转换、稳定排序、并发快照测试

internal/memory/
├── service.go         — 缓存状态摘要及成功操作后的更新
└── *_test.go          — 初始化、手动和自动更新测试

internal/command/
├── command.go         — Status Provider 注入
├── builtins.go        — /status 固定多行格式
└── *_test.go          — 输出、降级和零副作用测试

internal/tui/
├── model.go / update.go — 将 Runner 作为 Status Provider 传给命令上下文
└── tui_test.go          — 本地多行系统反馈验证

cmd/mewcode/
└── main.go             — 传入日志目录
```

## 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 命令形式 | 默认 `/status` 完整多行输出 | 已确认的交互，避免新增参数。 |
| 快照边界 | DTO + 窄 Provider 接口 | 命令层不访问运行时私有状态。 |
| 任务范围 | 仅运行中后台任务 | 符合需求，避免历史与结果泄露。 |
| 记忆统计 | 内存缓存 | 保证本地、快速，不在命令时扫描目录。 |
| 日志展示 | 仅日志根目录路径 | 可定位诊断文件，不读取敏感日志内容。 |
| 降级策略 | 缺失模块显示 `不可用`/`0` | 测试装配和兼容路径不因诊断失败。 |

## Spec 覆盖

| Spec | 设计归属 |
|---|---|
| F1-F3 | `command.statusCommand`、UI、SessionService、Runner Snapshot |
| F4 | Runner registry/Skill/SubAgent 聚合、Memory 缓存 |
| F5 | TaskManager 后台标记与安全快照 |
| F6 | status DTO 字段边界与格式化测试 |
| F7 | Provider 降级和可选模块快照 |
| N1-N5 | 内存缓存、锁保护、稳定排序、本地命令与兼容输出 |

## 缓存命中率扩展

### 架构概览

Provider 适配器继续把原始 usage 归一化为 `provider.Usage`，并标记缓存细分是否已包含在输入总数中。会话层把缓存读取、缓存写入、计量口径以及“缓存用量是否已知”与既有输入/输出 Token 一同追加到 JSONL usage 记录；TUI 与 `/status` 都从会话累计快照读取并格式化，不访问 Provider。

```text
OpenAI / Claude 流式完成事件
  → provider.Usage（普通输入、缓存读取、缓存写入）
  → Session.RecordUsage()
      → JSONL usage record
      → Session 累计快照
  → TUI TokenUsage() → 状态栏缓存百分比
  → /status → 缓存读/写与百分比
```

### 核心数据与计算

`provider.Usage` 保持 Provider 中立的输入、缓存读取、缓存写入和“已计入输入总数的缓存 Token”累计字段。会话额外保存缓存统计是否可用：新建会话和记录了新版 usage 的会话为可用；恢复缺少计量口径的旧版 usage 记录的会话为未知。

统一计算函数只接受聚合 usage 与可用标记：

```text
denominator = input + cache_read + cache_creation - cache_tokens_included_in_input
hit_rate = cache_read / denominator
```

缓存统计未知或 `denominator == 0` 时返回“不可显示”，调用方渲染 `—`。百分比按整数四舍五入展示。OpenAI 将缓存读取和缓存写入标记为已包含在 `input_tokens` 中；Claude 的读取和写入不包含在 `input_tokens` 中。OpenAI 的缓存写入字段不可用时以 `0` 参与计算。

### 模块设计

#### `internal/conversation`

- 扩展 usage journal record 的安全聚合字段与兼容解码；不改变历史消息记录格式。
- `Session.RecordUsage` 在成功追加后更新缓存累计及“已知”状态；缓存 Token 与普通 Token 一样必须非负。
- `SessionStore.Restore` 通过 usage 记录是否包含缓存统计字段判断可用性。旧记录继续恢复输入/输出 Token，但不会伪造缓存为零。
- 提供会话累计缓存统计的只读快照/格式化辅助，供 TUI 与命令层复用。

#### `internal/tui`

- 现有四种状态栏文案（idle、流式、终态、前台子 Agent）统一追加缓存片段。
- 只渲染会话累计快照；运行中当前轮的临时 usage 与会话已持久化 usage 的合并方式保持既有 TokenUsage 语义。
- 复用同一个百分比格式化结果，窄窗口下不新增换行或可变长度详情。

#### `internal/command`

- `/status` 复用会话快照与缓存命中率格式化辅助，新增固定字段“缓存读取”“缓存写入”“缓存命中率”。
- 缓存未知时明确输出 `—`，并继续输出既有 Token、会话和运行时诊断字段。

### 模块交互

```text
本轮请求完成
  → Adapter 解析实际 usage
  → Agent 汇总每轮 usage
  → Session 先追加安全 JSONL usage，再更新内存累计
  → 状态栏 /status 读取同一会话累计值

恢复会话
  → 扫描 usage journal records
  ├─ 有缓存字段 → 恢复累计值，显示命中率
  └─ 无缓存字段 → 保留 in/out，缓存标记未知，显示 —
```

### 文件组织

```text
internal/provider/
└── event.go                   — 统一 usage 与缓存字段的聚合语义

internal/conversation/
├── session.go                 — 缓存用量校验、累计和只读快照
├── store.go                   — JSONL usage 的持久化、恢复和旧记录识别
└── *_test.go                  — 新旧记录兼容、比例和持久化测试

internal/tui/
├── view.go                    — 状态栏缓存片段
└── *_test.go                  — 四种状态、未知值和窄布局测试

internal/command/
├── builtins.go                — /status 缓存字段
└── *_test.go                  — 数值、未知值和安全输出测试
```

### 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 统计范围 | 当前会话累计 | 与用户确认的展示口径一致，避免单轮波动误导。 |
| 分母 | 实际总输入，按 Provider 计量口径去重 | OpenAI 的缓存细分属于 `input_tokens`，Claude 的缓存细分独立计量。 |
| 未知历史 | 显示 `—` | 旧会话没有缓存数据，显示 0% 会造成错误结论。 |
| 数据来源 | Provider usage 字段 | 不预测缓存，不增加网络请求或请求体变更。 |
| 格式化位置 | 会话层共享辅助 | 防止状态栏和 `/status` 的分母、舍入或未知值处理不一致。 |

### Spec 覆盖（缓存扩展）

| Spec | 设计归属 |
|---|---|
| F8 | Provider 归一化、Session 累计与 JSONL usage record |
| F9 | TUI 状态栏共享缓存片段 |
| F10 | 共享计算辅助、`/status` 固定字段 |
| F11 | SessionStore 旧/新 usage record 兼容恢复 |
| N6-N8 | 实际 usage、不可显示结果与聚合数据边界 |
| AC8-AC12 | Provider、conversation、command、TUI 的单元与集成测试 |
