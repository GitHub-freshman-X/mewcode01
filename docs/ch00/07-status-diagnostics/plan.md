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
