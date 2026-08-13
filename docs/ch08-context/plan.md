# 上下文管理 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development`（推荐）或 `executing-plans` 按任务执行；步骤使用 `- [ ]` 跟踪。

**Goal:** 为 MewCode 的 Agent Loop 增加两层上下文压缩，使长任务不会因累积历史超限而失败。

**Architecture:** 新增 `internal/context` 作为与 Provider、TUI 解耦的上下文管理单元。`Runner` 在提交工具结果前调用第一层持久化，并在每次普通模型请求前根据管理器决策执行摘要；`Session` 与 `taskHistory` 只提供历史的原子替换。`/compact` 使用新的无工具 Agent 模式复用同一摘要流程。

**Tech Stack:** Go、现有 `provider.Provider` 流接口、Bubble Tea、YAML 配置、标准库文件系统与现有 Go 测试框架。

## Global Constraints

- 默认窗口、摘要预留、自动余量、手动余量分别为 200000、20000、13000、3000 Token。
- Token 估算锚定最近真实输入 usage，仅估算锚点后的新增内容；不引入 tokenizer 依赖。
- 摘要复用当前 Provider、禁止工具、仅保留 `<summary>`，不保留 `<analysis>`。
- `/plan` 仅替换其 `taskHistory`，不得修改共享会话历史。
- 正常请求遇到上下文超限时，紧急压缩后最多重试一次。

---

## 架构概览

```text
Tool Scheduler → Context Manager.PrepareResults → CommitRound
                                                     │
Session / taskHistory ← Context Manager rebuilt history ← summary Provider request
                                                     │
Runner（每轮请求前）← Estimate + automatic/forced decision
                                                     │
TUI /compact → ModeCompact → Runner compact-only task
```

- **`internal/context`**：保存阈值、用量锚点和熔断状态；提供第一层、Token 估算、压缩请求与重建的纯业务能力；`ResultStore` 封装会话专属文件写入。
- **`internal/conversation`**：在锁内把共享历史整体替换；保留现有 `CommitRound` 的成组校验语义。
- **`internal/agent`**：管理摘要 Provider 调用、四种触发原因、一次紧急重试和事件转发；Plan 使用其临时历史的替换方法。
- **`internal/config` 与启动层**：为 `AgentConfig` 增加嵌套的上下文配置，并映射为 `agent.Options`。
- **`internal/tui`**：解析 `/compact` 并渲染压缩事件，不承担阈值判断。

## 核心数据结构

### `context.Config` 与状态

```go
type Config struct {
	WindowTokens         int
	SummaryOutputTokens  int
	AutoSafetyTokens     int
	ManualSafetyTokens   int
	SingleResultChars    int
	MessageResultChars   int
	PreviewChars         int
	RecentTokens         int
	RecentMessageMinimum int
}

type Trigger string
const (
	TriggerAutomatic Trigger = "automatic"
	TriggerManual    Trigger = "manual"
	TriggerForced    Trigger = "forced"
	TriggerEmergency Trigger = "emergency"
)

type State struct {
	UsageAnchorInput int
	AnchorMessages   []provider.Message
	AutomaticFailures int
}
```

`Config` 提供默认值和校验：所有预算非负，窗口严格大于摘要预留加自动/手动余量；第一层与近期保留预算为正。`State` 只在一个 Runner 任务内变更；共享 Session 不保存模型调用状态。

### `context.Manager`

```go
type Manager struct { /* Config、ResultStore、State */ }

func (m *Manager) PrepareResults([]provider.ToolResult) ([]provider.ToolResult, []Persistence, error)
func (m *Manager) Estimate([]provider.Message) int
func (m *Manager) Decision([]provider.Message, bool) (Trigger, bool)
func (m *Manager) BuildSummaryRequest([]provider.Message, Trigger) provider.ChatRequest
func (m *Manager) Rebuild([]provider.Message, Trigger, string) ([]provider.Message, error)
func (m *Manager) RecordUsage(provider.Usage, []provider.Message)
func (m *Manager) RecordSummaryResult(success bool, manual bool)
```

`PrepareResults` 先处理单项，再在同一 user 工具结果消息内按内容长度降序替换，输出的结果保持原始调用顺序。`Rebuild` 保留末尾约 10K Token 或至少五条消息，并以整个 assistant/tool-result 组为不可拆分单元。摘要文本必须抽取非空 `<summary>`；失败不改动输入历史。

### `conversation` 与 `agent` 扩展

```go
func (s *Session) ReplaceHistory(messages []provider.Message)
func (h *taskHistory) Replace(messages []provider.Message)

const ModeCompact Mode = "compact"
const EventContextCompaction EventType = "context_compaction"

type CompactionEvent struct {
	Trigger Trigger
	BeforeTokens int
	AfterTokens int
	Persisted int
	Err error
}
```

`ReplaceHistory` 深拷贝输入并在单个临界区替换共享历史；`taskHistory.Replace` 仅改本次 Plan goroutine 的切片。`Event` 增加可选 `Compaction *CompactionEvent` 字段；成功与失败均可观察，但摘要失败不改变历史。

## 模块交互

1. Runner 在每轮开始取得当前历史快照并向 Manager 查询 `Decision`。
2. 若自动、强制或手动触发，Runner 构建 `Tools:nil` 的摘要请求，最大输出为 `SummaryOutputTokens`；读取完整流文本，抽取 `<summary>`，调用 `Rebuild` 后替换当前历史目标。
3. Runner 使用替换后的历史发起正常请求。Provider usage 返回后，`RecordUsage` 更新锚点。
4. 工具返回后，Runner 调用 `PrepareResults`，发出持久化事件，再以替换结果调用现有 `CommitRound`。
5. 正常请求被统一错误分类认定为上下文超限时，执行一次 `TriggerEmergency`，然后用重建历史重试原请求；第二次错误进入现有失败终态。
6. `/compact` 创建 `ModeCompact` 任务，仅执行步骤 2 并发出终态；活跃任务时沿用 Runner 的单任务拒绝。

## 文件组织

```text
internal/context/
├── config.go          — 默认值、预算校验与触发线
├── estimate.go        — usage 锚点与字符近似估算
├── results.go         — ResultStore、单项/聚合持久化与预览
├── compact.go         — 摘要 prompt、XML 标签提取、近期边界与历史重建
└── context_test.go    — 上述纯逻辑与文件存储测试
internal/conversation/session.go       — ReplaceHistory
internal/conversation/session_test.go  — 原子替换与隔离测试
internal/agent/event.go                — ModeCompact、压缩事件和 Options
internal/agent/history.go              — taskHistory.Replace
internal/agent/runner.go               — 请求前决策、摘要、紧急重试、结果预处理
internal/agent/runner_test.go          — Runner 集成场景
internal/config/config.go              — YAML ContextConfig 与默认值
internal/config/validate.go            — 预算组合校验
internal/config/config_test.go         — 配置加载与非法值测试
cmd/mewcode/main.go                    — Config 到 agent.Options 的装配
internal/tui/update.go                 — /compact 解析和压缩事件消费
internal/tui/tui_test.go               — 命令与展示状态测试
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 压缩位置 | 独立 `internal/context` | Runner 保持编排职责，Session 保持状态职责，纯逻辑易测。 |
| Token 计数 | usage 锚点 + 字符增量近似 | 无新依赖，误差限制在最近增量。 |
| 摘要模型 | 当前 Provider，空工具定义 | Provider 中立，避免摘要意外执行工具。 |
| 工具结果完整性 | 会话专属磁盘文件 + 预览引用 | 第一层零 API 成本且可按需读取。 |
| 摘要历史 | 边界消息 + 9 段摘要 + 近期成组原文 | 保留用户约束和最近执行细节，防止模型臆测。 |
| 熔断 | 仅自动路径三次失败暂停 | 防止循环，同时保留用户主动恢复路径。 |
| 错误恢复 | 单次紧急压缩重试 | 避免无限重试且覆盖估算漏网情形。 |

## Spec 覆盖

| 需求 | 设计归属 |
|---|---|
| F1–F3 | `context.results`、Runner 提交前处理、ResultStore |
| F4–F6 | `context.estimate`、Manager 决策、ModeCompact |
| F7–F9 | `context.compact`、摘要 Provider 调用、Session/taskHistory 替换 |
| F10 | Runner 的共享与 Plan 历史目标选择 |
| F11–F12 | Manager 状态、Runner 摘要/重试控制 |
| F13、N1–N5 | config、agent event、TUI、单测与集成测试 |
