# MewCode 日志日期目录 Plan

日志器用注入的 `now()` 获取本地日期，构造 `logs/YYYY/MM/DD` 并在其中创建既有带 UTC 时间戳和 PID 的独立文件名。仅修改 `internal/logging/logger.go` 与测试；无新依赖、无事件格式变更。

| 文件 | 改动 |
|---|---|
| `internal/logging/logger.go` | 用 `timestamp.Local()` 构造年/月/日目录，再保留现有文件名和 `O_EXCL` 行为。 |
| `internal/logging/logger_test.go` | 断言固定时间的目录结构与同日文件独立性。 |
| `docs/ch00/04-log-date-directory/task.md` | 任务拆解。 |
| `docs/ch00/04-log-date-directory/checklist.md` | 可观察验收。 |

验证：`go test -race ./internal/logging`，随后 `go test ./...`。
