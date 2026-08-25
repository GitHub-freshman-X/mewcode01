# Ch14 Worktree 依赖目录软链接未被目录忽略规则匹配

## 状态

已修复。

## 影响

当 `worktree.symlink_directories` 创建 `node_modules` 软链接，而仓库仅以 `node_modules/` 忽略该目录时，Git 将该软链接显示为未跟踪文件。这会使新建 Worktree 看起来存在未提交修改，可能阻止“仅删除干净 Worktree”和自动清理流程。

## 复现

在 `/private/tmp/mewcode-ch14-manual.rUcyWa` 的场景 A fixture 创建 `review/a` 后：

```sh
git -C .mewcode/worktrees/review+a status --short --untracked-files=all
```

输出 `?? node_modules`。`node_modules` 是指向 fixture 主目录依赖目录的软链接，且主仓库 `.gitignore` 包含 `node_modules/`。

## 根因

Git 的带末尾斜杠目录忽略模式不会匹配符号链接；初始化逻辑创建了软链接，但没有为其提供能够匹配链接自身的忽略规则或其他状态过滤。

## 建议修复

删除保护改用 NUL 分隔的 Git porcelain 状态，并只忽略同时满足以下条件的未跟踪条目：位于 `symlink_directories` 配置中、当前文件仍为软链接、其目标严格等于主工作区对应路径。正常删除时，再把这些已验证路径写入一次性 Git 排除文件并传给 `git worktree remove`，使 Git 的二次检查保持对其他改动的保护。其他未跟踪文件、链接替换或目标变更仍会被视为修改并阻止删除。

## 验证

- 2026-08-25：场景 A 的创建、列表和快速恢复均通过；上述 Git 状态检查复现问题。
- 2026-08-25：在独立 Git 最小仓库中，`.gitignore` 仅含 `node_modules/` 时，指向目录的 `node_modules` 软链接稳定显示为 `?? node_modules`；追加不带斜杠的 `node_modules` 后，`git check-ignore` 确认该链接被忽略。此结果排除 Worktree 未读取 `.gitignore` 和删除保护误判已忽略文件的可能。
- 2026-08-25：新增回归测试，含 `node_modules/` 目录忽略规则的 Worktree 在初始化软链接后不再被判为有修改；并验证 Git `worktree remove` 能移除该干净目录，同时含额外用户文件的 Worktree 仍被拒绝。`go test ./internal/worktree -count=1` 与 `go test ./...` 均通过。
- 待手工验证：使用重新构建的二进制执行 `/worktree remove init-check`，应无需 `--discard` 即可删除。
