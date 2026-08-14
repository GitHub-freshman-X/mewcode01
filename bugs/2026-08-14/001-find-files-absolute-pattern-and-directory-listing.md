# find_files 无法匹配绝对路径且不返回目录

- 状态：已修复，待完整验证
- 发现日期：2026-08-14
- 影响范围：`internal/tools/find_files.go`

## 现象

在 `/private/tmp/mewcode-ch09-manual.Gk0Y93` 中，用户可通过 `ls -la` 看到 `.mew`、`.mewcode`、`docs`、`logs` 等目录，但 MewCode 对这些目录和其中 sessions 的查询均得到空结果。

## 根因

会话日志显示模型传入了工作区内的绝对 glob，例如 `/private/tmp/mewcode-ch09-manual.Gk0Y93/.mewcode/sessions/*`。`FindFilesTool.Execute` 将遍历项转换为相对工作区路径后，直接用原始模式匹配，因此绝对模式不能匹配相对路径。

同时，遍历逻辑在遇到目录时直接返回，不会把目录作为匹配结果返回。它会遍历 `.mew` 和 `.mewcode`，但不会列出这两个目录本身。

## 建议修复

在工作区边界内将绝对模式规范化为相对模式，并让目录查询能够返回目录条目；保留工作区边界校验，拒绝工作区外路径。

## 修复与验证

已实现工作区内绝对模式的规范化，并将目录纳入匹配结果。新增定向回归测试，`go test ./internal/tools -count=1` 已通过。
