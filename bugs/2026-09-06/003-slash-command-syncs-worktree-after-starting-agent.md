# Slash Command 启动 Agent 后错误同步 Worktree

- 状态：已修复
- 发现日期：2026-09-06
- 影响范围：启用 Worktree Manager 时，所有通过 Slash Command 启动 Agent 的路径，包括动态 Skill 命令 `/isolated-review`。

## 现象

输入 `/isolated-review` 后界面显示：

```text
错误: cannot change worktree while an agent task is active
```

## 根因

TUI 处理 Slash Command 时，先调用 `command.Dispatch`。动态 Skill 命令会在 Dispatch 内调用 `StartAgent`，使 Runner 进入 active 状态。Dispatch 返回后，TUI 无条件调用 `runner.SyncWorktreeWorkspace()`；该方法为了防止运行中修改工具根目录，发现 active 状态后返回上述错误。

因此问题不依赖 Skill 的 `mode: fork`、`context` 或具体 SOP；任何可从 Slash Command 启动 Agent 的命令都可能触发同一顺序错误。普通文本输入路径不执行这次命令后 Worktree 同步，因此不受此问题影响。

## 修复方向

仅对不会启动 Agent 的本地 Worktree 管理命令执行 `SyncWorktreeWorkspace`，或在调用前确认 Runner 空闲。必须保留 `/worktree` 成功切换后重绑 Workspace 的行为，并确保 Agent 启动命令不会再额外尝试切换。

## 验证进展

2026-09-06：用户在真实 TUI 中以 `/isolated-review` 复现。静态调用链确认：`internal/tui/update.go` 在 Dispatch 后无条件调用 `SyncWorktreeWorkspace`；`internal/agent/runner.go` 在 active 时返回同一错误。

2026-09-06：已调整 TUI 分支顺序：`Dispatch` 成功且已创建任务时，直接等待任务事件；只有未启动任务的命令才调用 `SyncWorktreeWorkspace`。`TestCommandPlanConsumesAgentEvents` 现以配置 Worktree Manager 的 Runner 覆盖该时序，待运行相关测试确认结果。

2026-09-06：首次定向测试中 `internal/agent` 已通过；`internal/tui` 因受限环境无法读取 Go build cache 而未启动。以获得授权的相同测试命令复验后，`go test ./internal/tui ./internal/agent -run 'Test(CommandPlanConsumesAgentEvents|.*Worktree.*|.*Skill.*)' -count=1` 已通过，`git diff --check` 亦通过。

2026-09-06：完整受影响包验证 `go test ./internal/tui ./internal/agent -count=1` 已通过。修复后的行为为：会启动 Agent 的 Slash Command 直接订阅任务事件，空闲本地命令仍同步 Worktree Workspace。
