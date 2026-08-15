# 命令系统反馈渲染在当前命令输出之前

- 状态：已修复，自动化验证通过，待真实 API 验证
- 发现日期：2026-08-16
- 影响范围：`internal/tui/view.go` 的 `refreshContent`

## 现象

`/do` 的“没有可执行计划”等系统反馈显示在当前 `/do` 用户输入之前；多次提交失败命令时，旧反馈会堆叠在后续命令上方，阅读顺序与交互顺序不一致。

## 根因

`refreshContent` 固定先渲染 Session display history，再渲染所有 `systemMessages`，最后渲染未提交到 Session 的当前任务输入与输出。命令反馈没有携带与命令输入关联的位置，因此无法按发生顺序插入。

## 建议修复

将系统消息与当前命令作为有序的 transient display entries，或至少在渲染当前命令后输出其本次系统反馈；完成后保留正确顺序但不写入 Session history。增加连续未知命令和 `/do` 失败的视图回归测试。

## 验证方式

输入 `/do`（没有计划）或未知命令，观察用户输入在上、对应系统反馈紧随其后；系统反馈仍不得写入 Session history 或后续模型请求。

## 修复进展

2026-08-16：初次修复只将所有系统消息整体移到当前任务之后，导致历史命令反馈永久堆在后续任务底部，未满足真实交互顺序。

2026-08-16：系统消息改为携带产生时的 Session display 消息数，并在该位置插入渲染；与当前失败/取消命令绑定的反馈在该命令之后渲染。新增“第一轮对话 → 系统反馈 → 第二轮对话”回归测试。

2026-08-16：临时条目扩展为用户命令或系统反馈两种角色；纯本地命令在分发完成且未启动 Agent 时，插入本次反馈之前。新增“第一轮任务 → `/help` → 命令反馈 → 第二轮任务”回归测试，确认命令与反馈均不进入会话历史。`go test ./internal/command ./internal/conversation ./internal/tui ./internal/agent ./internal/memory ./cmd/mewcode -count=1` 已通过，待按真实 API 场景验证。

2026-08-16：补充会话重置后的边界测试，确认 `/clear` 或 `/session new` 清除旧临时条目后，当前命令仍位于“已创建新会话”反馈之前；相关包测试、`go vet ./...` 与 `git diff --check` 均通过。

2026-08-16：真实 API 手工测试发现成功的 `/memory add` 不产生系统反馈时，命令显示到了最早聊天记录之前。根因是会话重置与“本次没有反馈”共用 `start == len(systemMessages)` 的边界判断，后者错误复用了最早条目的锚点。修复为在会话切换时递增临时消息代次，仅当代次变化时重置插入点；新增无反馈命令锚点回归测试。`go test ./internal/command ./internal/conversation ./internal/tui ./internal/agent ./internal/memory ./cmd/mewcode -count=1`、`go vet ./...` 与 `git diff --check` 已通过，待真实 API 回归验证。
