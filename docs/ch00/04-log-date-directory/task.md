# MewCode 日志日期目录 Tasks

## T1：按日期创建目录

**文件：** `internal/logging/logger.go`、`internal/logging/logger_test.go`。

1. 写失败测试：固定本地时间创建记录器，断言日志文件位于 `logs/2026/08/06/`。
2. 运行 `go test ./internal/logging -run TestNewUsesDateDirectory`，确认失败。
3. 以 `now().Local()` 的年、月、日构造目录，保留现有 UTC 文件名和 `O_EXCL`。
4. 补充同日期不同 PID 不覆盖测试。
5. 运行 `go test -race ./internal/logging`。
6. 提交实现。

## T2：回归记录

1. 新建 `checklist.md`，覆盖日期路径、独立文件、日志降级与回归。
2. 运行 `go test ./...` 并记录结果。
