# 第十四章 Worktree 开发交接

## 当前目标

继续完成第十四章 Git Worktree 文件系统隔离的实现、验证与文档同步。

## 已批准的设计文档

- [Spec](spec.md)
- [Plan](plan.md)
- [Tasks](task.md)
- [Checklist](checklist.md)
- [教学文档](理论学习：Git Worktree 并行隔离.md)

四份开发前文档已由用户明确批准。应继续按 `task.md` 执行，无需再次请求逐任务确认；用户特别要求不要每个任务暂停。

## 已完成的实现进度

- 已在基于 `main` 的 `codex/ch14-worktree` 分支开发。
- T1 已完成并验证：新增 `internal/worktree/validate.go`、`manager.go`、`validate_test.go`。
- `ValidateSlug` 已覆盖长度、允许字符、嵌套分段、`.`、`..` 与空段拒绝；`go test ./internal/worktree` 曾通过。
- T2 刚开始：`Manager.Create` 已加入 `git worktree add -B <branch> <path> HEAD` 与非交互 Git 环境变量。此实现尚未测试、恢复、删除或并发保护，需审阅后继续。

## 2026-08-25 本轮进度

- T2/T3 主链已实现并有临时 Git 仓库测试：创建、只读快速恢复、会话持久化/恢复、退出、未提交修改与新提交保护、确认删除和过期临时目录 fail-closed 清理。
- T4/T5 已接入：工具注册表可按 `Workspace` 重建；定义式 Agent 和调用参数均接受 `isolation: worktree`。
- T6/T7 已接入运行时与启动入口：Fork 在已组装的 Runtime 中创建临时 Worktree；定义式隔离 Agent 使用独立 Workspace、权限 sandbox 和工具注册表；`/worktree` 与配置字段可用。
- T8 已同步 README、示例配置和文档索引；`go test ./...` 于本轮通过，随后运行测试产生的 `cmd/mewcode/.mewcode/` 已清理。

## 最终完成状态

- 创建后初始化已支持配置本地文件复制、Git hooks 配置继承、依赖目录软链接及 `.worktreeinclude` 中明确列出的忽略运行文件。
- 手动进入/退出与恢复会重建显式 Workspace 工具注册表和权限 sandbox，不修改进程 cwd；同名文件读取不会复用主工作区工具实例。
- Fork 强制要求已组装的 Worktree Manager；不能安全隔离时启动失败，不会退回共享目录。干净临时 Worktree 自动删除；保留的目录会在子任务结果中返回路径和分支。
- 命令生命周期、安全拒绝、初始化和工具隔离均有自动化覆盖；`go test ./...` 通过后，本章 checklist 已全部完成。

## 关键已确认决策

- Fork 子 Agent 继承完整对话、强制后台且可以修改文件，因此必须在启动时强制创建临时 Worktree。
- 定义式子 Agent 只有在 frontmatter 或调用参数声明 `isolation: worktree` 时才隔离。
- 共享主工作区的前台定义式子 Agent 不得在运行中迁移到 Worktree，也不得自动或 ESC 转后台。
- Worktree 使用独立 `tools.Workspace` 和独立工具 registry 作为显式 cwd；禁止通过 `chdir` 切换进程 cwd。
- Worktree 放在 `.mewcode/worktrees/`；手动创建默认基于当前 `HEAD`；不做内置 merge/cherry-pick。
- 删除与过期清理 fail-closed：状态不确定则保留目录。

## 接下来建议

1. 先补全并测试 `internal/worktree` 的 Git 执行、快速恢复、会话、初始化、变更保护与清理（T2、T3）。
2. 增加按 Workspace 构造工具 registry 的能力（T4）。
3. 再扩展 Agent 定义、Runtime 和命令入口（T5-T7）。
4. 最后同步 README、示例配置、章节索引，按 `checklist.md` 验收（T8）。

## 工作区状态

当前未跟踪的本章内容包括 `docs/ch14-worktree/` 和 `internal/worktree/`。不要删除或覆盖其中已有文件。尚未提交。

## 建议技能

- `mew-spec`：已完成审批流程，可用于核对实现不偏离批准的文档；不要重新开始规格流程。
- `diagnosing-bugs`：仅在 Git、隔离或测试失败时使用。
