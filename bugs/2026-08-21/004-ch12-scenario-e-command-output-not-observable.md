# 第十二章场景 E 无法观察命令变量展开结果

- 状态：待处理
- 发现日期：2026-08-21
- 影响范围：`docs/ch12-hook/manual_scenarios.md` 场景 E 的真实 Provider 手工验收。

## 现象

场景 E 将 Hook 的 `command` 改为 `printf`，并要求确认变量已展开。但命令 stdout/stderr 仅被 `hooks.Executor` 捕获到 `Result.Output`；格式化规则是异步 `post_tool_use` Hook，`Engine.runAsync` 丢弃该结果。TUI 不显示该内容，日志也按脱敏要求不记录它。

因此，测试者无法通过当前场景给出的 TUI 或日志证据判断 `$EVENT`、`$TOOL_NAME`、`$FILE_PATH`、`$TOOL_ARGS.path` 和未提供变量是否被正确展开。

## 根因

实现刻意将 Hook 命令输出排除在 TUI 和日志之外，以避免泄露提示词、工具参数或命令输出正文；但手工场景没有提供独立、安全的观察通道。

2026-08-21 现场检查进一步确认，场景的格式化条件 `args.path ~= "*.go"` 也无法匹配 `fixtures/quicksort.go`。条件匹配使用 `filepath.Match`，其中 `*` 不匹配路径分隔符；无条件的 `project-write-notification` 在同一次写入后已产生 `post_tool_use` 日志，说明 Hook 事件本身正常。由于格式化规则未匹配，重定向命令从未执行，自然不会创建 `hook-variables.txt`。

## 建议修复

将格式化规则的条件改为 `args.path ~= "**/*.go"`，使其覆盖 fixture 下的 Go 文件。场景 E 的临时命令再把展开结果写入 fixture 中专用的临时文件，例如 `hook-variables.txt`，随后用 `cat` 检查该文件。命令应同时包含 `$TOOL_ARGS.missing`，并断言其值为空。该文件须在场景结束时删除；不得通过日志或普通 TUI 消息回显 Hook 输出。

## 验证进展

2026-08-21：代码检查确认 `Executor.command` 以 `CombinedOutput` 捕获输出，而 `Engine.runAsync` 不消费返回值；`go test ./internal/hooks ./internal/agent ./internal/tui ./internal/command` 通过。另运行 `go test ./internal/hooks -run 'Test.*(Glob|Condition)' -count=1 -v`，确认现有测试只覆盖根目录 `main.go` 与 `src/**/*.go`，未覆盖 `*.go` 对带目录路径不匹配的边界。尚未执行真实 Provider 手工场景。
