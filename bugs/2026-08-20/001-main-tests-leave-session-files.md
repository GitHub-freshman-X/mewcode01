# 001：启动测试在包目录遗留会话文件

- **状态：** 待处理
- **影响：** 运行 `go test ./cmd/mewcode` 或全量测试后，工作树可能出现未跟踪的 `cmd/mewcode/.mewcode/sessions/*.jsonl`。

## 复现

在仓库根目录执行：

```sh
go test ./cmd/mewcode -count=1
```

随后执行 `git status --short`，可观察到 `cmd/mewcode/.mewcode/`。

## 当前诊断

部分 `run` 测试没有切换到临时工作目录。启动流程始终在当前工作目录创建项目级会话，因此测试在包目录运行时会写入 `cmd/mewcode/.mewcode/sessions/`。

## 当前处理

2026-08-22 执行 `go test ./... -count=1` 时再次生成 `cmd/mewcode/.mewcode/sessions/`（共 9 个 JSONL 文件）；已立即删除该目录并确认工作树不再包含该产物。尚未修改既有启动测试的工作目录隔离，原因是该问题独立于 `/status` 诊断功能。

## 建议下一步

为所有调用 `run` 的启动测试统一注入临时工作目录，并在测试结束时恢复原工作目录；随后验证 `go test ./cmd/mewcode -count=1` 后工作树无新增 `.mewcode` 产物。
