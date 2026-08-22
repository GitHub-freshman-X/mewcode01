# `/status` 缺少工作目录、会话 ID 和日志目录等诊断信息

- 状态：已修复，自动化验证通过；真实 Provider/TUI fixture 场景待执行
- 发现日期：2026-08-22
- 影响范围：`/status` 本地命令，以及第十三章真实 Provider 人工验收的启动诊断。

## 现象

原 `/status` 初始反馈仅为：

```text
[DEFAULT] tokens in:0 out:0
```

它无法显示当前工作目录、会话 ID、日志目录、权限、扩展发现或运行中后台 SubAgent 状态。

## 根因

`internal/command/builtins.go` 中的 `statusCommand` 原先仅根据当前模式和 `UI.TokenUsage()` 生成单行文本。第十章规划和第十三章 fixture 验收所需的本地诊断状态没有运行时聚合与安全快照接口。

## 修复方案

- 新增 `internal/status` 安全 DTO，并由 Runner 聚合工作区、日志目录、权限、实际工具数、Skill、缓存记忆数量、SubAgent 定义和运行中后台任务。
- `TaskManager` 为后台任务保存标记并提供稳定的运行中后台快照；显式后台、Fork、ESC 和自动接管均正确标记同一任务。
- Memory Service 在启动和成功变更后维护缓存数量，`/status` 不扫描 memory 目录。
- `/status` 输出固定多行本地诊断；不包含 API Key、配置正文、Prompt、任务描述、结果或失败正文。
- README 和 `docs/ch00/07-status-diagnostics/` 的 Spec、Plan、Tasks、Checklist 已同步。

## 验证进展

2026-08-22：

- `go test ./internal/status ./internal/subagent ./internal/memory ./internal/agent ./internal/command ./internal/tui ./cmd/mewcode -count=1` 通过。
- `go test -race ./internal/status ./internal/subagent ./internal/agent ./internal/memory ./internal/command ./internal/tui -count=1` 通过。
- `go test ./... -count=1` 通过。
- `go build ./cmd/mewcode` 通过。
- `git diff --check` 通过。

尚未使用真实 Provider 在 `/private/tmp` fixture 中执行第十三章的后台任务人工场景；该项不影响上述自动化验证结论。