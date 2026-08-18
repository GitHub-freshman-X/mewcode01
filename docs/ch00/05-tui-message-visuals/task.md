# TUI 消息视觉层级与会话分隔 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `internal/tui/styles.go` | 定义消息层级背景色、错误和会话分隔条样式。 |
| 修改 | `internal/tui/view.go` | 按消息块应用样式，渲染历史显示段和会话分隔条。 |
| 修改 | `internal/tui/model.go` | 在成功会话切换后归档旧显示并设置新会话边界。 |
| 修改 | `internal/tui/tui_test.go` | 覆盖消息样式、流式过程和会话切换隔离。 |
| 修改 | `docs/README.md` | 将本补充文档加入项目文档索引。 |
| 新建 | `docs/ch00/05-tui-message-visuals/checklist.md` | 记录实现后的可观察验收项。 |

## T1：定义消息与会话边界样式

**文件：** `internal/tui/styles.go`

**依赖：** 无

**步骤：**

1. 为用户消息、最终答复、思考、工具调用、成功工具结果、失败工具结果、系统反馈、错误和会话分隔条定义包内 Lipgloss 样式。
2. 使用户消息与最终答复使用明显强于过程信息的背景色，并保证文字、标签和折叠提示可辨认。
3. 添加会话标题显示辅助逻辑，统一空标题占位与长度截断规则。

**验证：** `go test ./internal/tui -run 'Test.*(Message|Style|View)' -count=1` 通过，且新增样式测试能在 ANSI 输出中识别相应背景色。

## T2：按消息块渲染视觉层级

**文件：** `internal/tui/view.go`、`internal/tui/tui_test.go`

**依赖：** T1

**步骤：**

1. 提取当前会话内容的渲染函数，保持已有消息、命令反馈、任务状态、权限提示与候选项的显示顺序。
2. 修改消息和系统信息渲染，使角色与内容块类型选择 T1 中对应的背景样式。
3. 为流式任务文本、thinking、工具调用、成功与失败工具结果、系统反馈和错误增加视图断言。
4. 保留工具调用、结果去重及思考折叠行为。

**验证：** `go test ./internal/tui -run 'Test(View|SystemMessage|LocalCommand|MessageStyle)' -count=1` 通过；测试断言最终答复与过程信息的 ANSI 样式不同。

## T3：保存会话显示段并插入分隔条

**文件：** `internal/tui/model.go`、`internal/tui/view.go`、`internal/tui/tui_test.go`

**依赖：** T2

**步骤：**

1. 在 `Model` 中增加仅内存存在的历史显示段和当前会话边界状态。
2. 在 `New` 与 `Resume` 的目标会话替换成功后，归档旧会话可见内容，清理既有会话态状态，并设置新会话的 ID、标题分隔条。
3. 在 viewport 重建时先渲染历史显示段，再渲染当前会话分隔条，最后渲染当前实时内容；初始会话不显示分隔条。
4. 覆盖 `/clear`、`/session new` 和 `/session resume <id>`：断言旧内容保留、分隔条文本和顺序正确，且创建或恢复失败不会新增分隔条。
5. 断言分隔条不会新增 Session history 或 display history 条目，也不改变现有命令反馈的相对顺序。

**验证：** `go test ./internal/tui -run 'Test.*(Session|Clear|Resume|Boundary)' -count=1` 通过；测试中 Session 快照长度仅包含真实对话记录。

## T4：执行 TUI 回归与项目验证

**文件：** `internal/tui/tui_test.go`

**依赖：** T2、T3

**步骤：**

1. 运行完整 TUI 测试，确认命令、权限、任务取消、工具输出去重、计划显示和 Token 状态栏没有回归。
2. 运行全项目测试和构建，确认不影响持久化、命令、Agent 或 Provider 边界。
3. 如测试或运行产生本轮专属 `.mew/`、`logs/` 等中间目录，仅在确认由本轮产生后清理。

**验证：** `go test ./internal/tui ./internal/command ./internal/conversation -count=1`、`go test ./... -count=1` 和 `go build ./cmd/mewcode` 均通过。

## T5：补全文档索引与验收清单

**文件：** `docs/README.md`、`docs/ch00/05-tui-message-visuals/checklist.md`

**依赖：** T4

**步骤：**

1. 在“中途补充”索引中加入本目录，并链接 Spec、Plan、Tasks 和 Checklist。
2. 根据已批准 Spec 和实际测试命令编写验收清单；每项给出可观察结果和验证方式。
3. 在最终验收后将清单更新为实际通过或未通过状态，并保留证据摘要。

**验证：** 检查索引链接指向现有文件，且 `git diff --check` 通过。

## 执行顺序

```text
T1 → T2 → T3 → T4 → T5
```
