# 已注册的 `fork` Skill 无法执行

- 状态：已修复，定向自动化验证通过
- 发现日期：2026-08-18
- 影响范围：第十一章所有 `mode: fork` Skill；当前内置 `review`。

## 现象

输入 `/review` 后界面显示 `错误: fork skills are not implemented`，不会启动审查任务。

## 根因

`internal/agent/runner.go` 在显式 Skill 调用时直接拒绝 `ModeFork`。第十一章手工测试方案也标明 fork 场景尚未实现，因此这是未完成的章节功能，而不是 Provider 或配置故障。

## 后续工作

实现独立 Session、按 `full`/`recent`/`none` 构造 fork 历史、仅将最终摘要回流主会话，以及取消和失败时不写入伪摘要；补充 Runner 与 TUI 回归测试。

## 验证进展

2026-08-18：`go test ./internal/agent ./internal/tui ./internal/command -count=1` 通过，但现有测试没有覆盖 `ModeFork` 可执行性，因此不能证明 `/review` 可用。

2026-08-18：已开始实现独立临时会话、历史范围隔离、最终摘要回流和 Token 累计；待补充回归测试与验证。

2026-08-18：已新增 `context: none` 的 fork 回归测试，验证主历史只新增 `/review` 与最终摘要，且用量累计到主会话。`go test ./...` 与 `go build ./cmd/mewcode` 已通过；`full`、`recent`、取消和失败边界仍需补充专门回归测试。
