# `/help` 未显示第十一章注册的内置 Skill 命令

- 状态：已修复，自动化验证通过
- 发现日期：2026-08-18
- 影响范围：`internal/command` 的帮助输出；`/commit`、`/review`、`/test` 及其他动态 Skill 命令不可从 `/help` 发现。

## 现象

Skill 注册和 Tab 补全可用，但输入 `/help` 时仅显示默认命令，遗漏第十一章内置 Skill。

## 最小复现

1. 使用默认命令与 `commit`、`review`、`test` Skill 元数据构造命令注册表。
2. 分发 `/help`。
3. 输出中不包含动态 Skill 命令。

## 根因

`helpCommand` 重新创建并查询 `DefaultRegistry()`，而没有使用 `Dispatch` 所接收的运行时注册表。TUI 的运行时注册表本身已包含 `SkillCommands`，所以 Tab 补全和命令执行正常；只有帮助输出回退为默认命令集合。

## 当前进展

新增命令层回归断言后，已确认组合注册表中的 `/review` 不在 `/help` 输出内。现已将分发中的运行时注册表透传给命令处理器，帮助命令会使用该注册表。

## 修复方案

在 `CommandContext` 中传递当前 `Registry`，由 `Dispatch` 注入；`helpCommand` 统一用该运行时注册表生成列表和查询单个命令。没有通过分发调用时仍回退到默认注册表，保留既有直接调用语义。

## 验证方式

- `go test ./internal/command -run '^TestDispatchLocalAndPrompt$' -count=1`
- `go test ./internal/command ./internal/tui ./internal/agent -count=1`

两项均通过。新增回归断言覆盖组合注册表中 `/help` 显示 `/review`。
