# TUI 消息视觉层级与会话分隔 Plan

## 架构概览

本次改动限定在 `internal/tui`。样式层为各类消息块定义前景、背景与文字强调；视图层按消息角色与内容块类型选择样式，并将同一会话的消息按现有顺序渲染。

为保留同一 TUI 进程内已经显示过的会话，`Model` 增加仅内存存在的历史显示段。会话替换前，TUI 将当前会话可见内容快照为一个历史段；替换成功后，创建带会话 ID 与标题的分隔段。每次刷新时按“历史段 → 当前会话分隔段 → 当前会话实时内容”重建 viewport 内容。历史段和分隔段不进入 `conversation.Session`、JSONL journal 或 Agent 请求。

## 核心数据结构

### `displaySegment`

```go
type displaySegment struct {
    content string
}
```

`content` 是已完成会话在切换瞬间的 TUI 纯显示快照。它只保存在 `Model` 内存中，用于随后刷新时重现旧会话；不传递给会话、命令、Agent 或 Provider 模块。

### `sessionBoundary`

```go
type sessionBoundary struct {
    id    string
    title string
}
```

它描述当前会话开始前要渲染的视觉分隔条。新建或恢复成功后以该会话的元数据填充；初始启动没有分隔条。标题为空时使用固定占位文本，并按既有会话标题规则截断。

## 模块设计

### `internal/tui/styles.go`

**职责：** 定义统一的颜色层级与分隔条样式。

**对外接口：** 包内样式变量及一个会话标题格式化辅助函数。

**设计：**

- 用户消息和最终答复采用高对比度、不同色相的背景。
- 思考、工具调用与工具结果采用较低饱和度背景；工具失败结果使用可辨识的失败色。
- 系统反馈和错误分别使用助手与错误背景，避免退回无背景纯文本。
- 分隔条使用反色或高对比度横向样式，正文包含 `会话开始 · <ID> · <标题>`。
- 保留现有输入框、状态栏和标签前景色，仅扩展消息区样式。

### `internal/tui/view.go`

**职责：** 将各类型消息、历史显示段与会话分隔条组合为 viewport 内容。

**对外接口：** 保留 `refreshContent` 与 `renderMessage` 的职责；新增当前会话内容渲染、历史段渲染和分隔条渲染的包内辅助函数。

**设计：**

- 将现有 `refreshContent` 中的当前会话拼接逻辑提取为单独渲染函数，确保正常刷新与会话切换前快照使用同一顺序和规则。
- `renderMessage` 按 `Role` 和 `ContentBlock.Type` 将标签、正文或状态包装到对应样式中：用户文本、助手文本、思考、工具调用、工具结果、系统反馈和错误均有背景。
- 对正在执行的任务继续沿用 `current` 状态渲染；流式文本每次刷新都走最终答复样式，思考与工具事件分别使用过程样式。
- `refreshContent` 先输出已归档段，再输出当前会话分隔条，最后输出当前会话内容、候选项和权限提示；现有自动跟随和滚动位置判定不变。

### `internal/tui/model.go`

**职责：** 在会话成功切换的边界保存旧会话显示快照，并记录新会话分隔条信息。

**对外接口：** `New` 与 `Resume` 继续满足既有 `command.SessionService` 接口；新增包内 `archiveCurrentDisplay` 与 `beginSessionDisplay` 辅助方法。

**设计：**

1. `New` / `Resume` 在调用 `runner.ReplaceSession` 前不修改当前 TUI 状态。
2. 仅当替换成功后，将当前会话的可见内容加入 `displaySegment`；这样失败的创建或恢复不会产生虚假的分隔条。
3. 保持既有会话、任务、系统消息、计划模式与 Skill 激活清理顺序；清理后设置新会话的 `sessionBoundary`。
4. 新会话的命令文本与反馈仍由既有 `AddCommandMessage` / `AddSystemMessage` 插入当前会话段，保证真实交互顺序。

### `internal/tui/tui_test.go`

**职责：** 覆盖样式选择、实时任务渲染与三类会话切换的显示和隔离行为。

**设计：**

- 为用户、最终答复、思考、工具调用、成功与失败工具结果、系统反馈和错误分别断言对应 ANSI 背景样式或渲染片段存在。
- 用活动任务状态断言流式文本、thinking 与工具事件同时渲染时仍使用各自层级。
- 针对 `/clear`、`/session new` 与 `/session resume` 建立带可识别旧内容的会话，断言旧内容、分隔条、新会话 ID / 标题和命令反馈顺序正确。
- 断言分隔条只保存在 `Model` 显示状态，不增加 `Session.Snapshot` / `DisplaySnapshot` 条目；已有 JSONL 与 Provider 请求测试继续作为持久化和上下文隔离的回归保护。

## 模块交互

```text
用户执行 /clear、/session new 或 /session resume <id>
  └─ command 调用 Model.New 或 Model.Resume
       ├─ 创建 / 恢复目标 Session
       ├─ Runner.ReplaceSession 成功
       ├─ Model.archiveCurrentDisplay()
       │    └─ 当前会话显示快照加入仅内存历史段
       ├─ 清理既有会话态 UI / Skill 状态
       └─ Model.beginSessionDisplay(meta)
            └─ 设置“会话开始 · ID · 标题”分隔条

后续任何 TUI 刷新
  └─ refreshContent
       ├─ 渲染历史显示段
       ├─ 渲染当前会话分隔条
       └─ 按角色 / 内容块类型渲染当前实时内容
```

## 文件组织

```text
internal/tui/
├── styles.go     — 消息层级背景色、错误与会话分隔条样式
├── model.go      — 内存历史显示段、会话分隔状态和切换辅助方法
├── view.go       — 当前会话快照、消息块样式选择、分隔条与总视图组装
└── tui_test.go   — 样式、流式过程和会话边界测试
docs/ch00/05-tui-message-visuals/
├── spec.md       — 已批准的行为规格
└── plan.md       — 本技术设计
```

## Spec 覆盖

| Spec | 设计归属 |
|---|---|
| F1、F2、F3、F4 | `styles.go` 的层级配色与 `view.go` 的按块渲染 |
| F5 | `model.go` 的成功切换快照与 `view.go` 的会话分隔条 |
| F6 | `Model` 私有内存段；不变更 conversation、journal、agent、provider |
| N1、N2、N3 | Lipgloss ANSI 样式、现有 viewport 与 resize 机制 |
| N4 | `tui_test.go` 的无真实 Provider 自动化测试 |
| N5 | 不新增日志或持久化字段；显示段仅保留已在本地 TUI 可见的内容 |

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 背景色实现 | 复用 Lipgloss 样式 | 项目已使用该库，输出 ANSI 兼容现有 TUI 框架与跨平台目标。 |
| 会话历史保留位置 | `Model` 的仅内存显示段 | 满足同一终端进程中的可追溯性，不污染会话持久化和模型上下文。 |
| 快照时机 | 目标会话替换成功后归档旧显示 | 避免创建或恢复失败时显示不存在的会话边界。 |
| 分隔条内容 | ID + 截断后的标题 | 与已有 `/session list` 的用户识别方式一致，空会话仍可辨别。 |
| 历史段表现 | 保留切换时的已完成显示文本 | 无需重新读取或修改旧会话，避免在切换后把旧会话的数据带入当前上下文。 |
