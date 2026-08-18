# TUI 消息视觉层级与会话分隔 Checklist

> 每项均以运行测试或观察 TUI 行为验证；实现完成后在方框中记录实际结果与证据。

## 消息视觉层级

- [x] **用户与最终答复背景（AC1）**：提交一条普通用户消息并完成模型答复后，用户消息和最终答复分别使用两种醒目的背景色。（证据：`go test ./internal/tui -count=1` 通过；`TestMessageBackgroundStyles` 断言两类 ANSI 背景存在且不同。）
- [x] **过程消息背景（AC2）**：在含思考、工具调用、成功工具结果和失败工具结果的显示中，每类过程消息都有可区分的较弱背景，且与两类重点消息不同。（证据：`go test ./internal/tui -count=1` 通过；`TestMessageBackgroundStyles` 覆盖四类过程样式。）
- [x] **流式样式稳定（AC3）**：模型流式输出尚未完成时，已到达文本仍按最终答复背景显示；结束、失败或取消后保留已渲染样式。（证据：`go test ./internal/tui -count=1` 通过；`TestActiveMessageBackgroundStyles` 覆盖活跃任务文本和工具过程。）
- [x] **命令与反馈顺序（AC4）**：本地命令显示为用户样式，系统反馈显示为助手样式；二者仍处于原有真实交互顺序。（证据：`go test ./internal/tui -count=1` 通过；既有顺序测试与 `TestSystemMessageAndErrorUseBackgroundStyles` 通过。）

## 会话边界与隔离

- [x] **新建与清空会话边界（AC5）**：执行 `/session new` 和 `/clear` 后，旧会话内容仍可向上滚动查看，新内容前存在含新会话 ID 和空标题占位的“会话开始”分隔条。（证据：`go test ./internal/tui -count=1` 通过；`TestClearPreservesDisplayAndStartsNewSessionBoundary` 与 `TestSessionNewPreservesDisplayAndStartsNewSessionBoundary` 通过。）
- [x] **恢复会话边界（AC5）**：执行 `/session resume <id>` 后，旧终端内容保留，恢复会话前显示含目标 ID 和已保存标题的分隔条。（证据：`go test ./internal/tui -count=1` 通过；`TestSessionResumePreservesDisplayAndStartsSessionBoundary` 通过。）
- [x] **失败切换不产生边界（AC5）**：创建、恢复或替换会话失败时，不新增分隔条或归档段，当前显示和会话继续可用。（证据：`go test ./internal/tui -count=1` 通过；`TestFailedSessionResumeDoesNotCreateBoundary` 通过。）
- [x] **不进入持久化和上下文（AC6）**：会话分隔条不会增加 Session history、display history、JSONL journal 或下一轮 Provider 请求的记录；切换后 Token 用量与既有清理行为正确。（证据：`go test ./internal/tui ./internal/command ./internal/conversation -count=1` 与 `go test ./... -count=1` 通过；会话边界测试断言 history 和 display history 未增加显示记录。）

## 集成与回归

- [ ] **终端交互兼容（AC7）**：在窄窗口下，背景样式消息、分隔条、输入区和状态栏均可渲染并继续滚动、输入和调整尺寸。（未执行：需要有效模型配置的手工终端验证；自动化 TUI 测试已通过。）
- [x] **TUI 与关联模块回归**：命令、权限、任务取消、工具结果去重、计划显示和状态栏测试全部通过。（证据：`go test ./internal/tui ./internal/command ./internal/conversation -count=1` 通过。）
- [x] **全项目验证**：全量自动化测试和可执行文件构建通过。（证据：`go test ./... -count=1` 与 `go build ./cmd/mewcode` 均通过。）
- [x] **文档索引**：文档索引链接到本目录的 Spec、Plan、Tasks 与 Checklist，链接路径均存在。（证据：已更新 `docs/README.md`；`git diff --check` 通过。）

## 端到端场景

- [ ] **多会话终端追溯**：在同一终端依次完成会话 A 的一次问答，执行 `/clear` 完成会话 B 的一次问答，再 `/session resume <会话 A 的 ID>`；向上滚动时可依次看到 A 内容、B 开始分隔条与 B 内容、A 恢复分隔条与恢复后的 A 内容，且各消息的背景层级清晰。（未执行：需要有效模型配置的手工终端验证；自动化 `TestConsecutiveSessionSwitchesKeepEveryBoundary` 与恢复会话测试已通过。）
