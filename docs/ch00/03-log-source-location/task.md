# MewCode 日志调用位置 Tasks

## T1：记录业务调用位置

**文件：** 修改 `internal/logging/logger.go`、`internal/logging/logger_test.go`。

1. 写失败测试：调用 Info 后解析 JSON，断言 `source` 为 `internal/logging/logger_test.go:<行号>`，且不含项目绝对路径。
2. 运行 `go test ./internal/logging -run TestLoggerWritesCallerSource`，确认失败。
3. 给 Event 增加 `Source`，Logger 保存项目根；在实际文件写入路径中捕获日志包外的首个调用帧并相对化。
4. 对无法相对化和 no-op 记录器断言不写绝对路径、不失败。
5. 运行 `go test -race ./internal/logging`。
6. 提交实现与测试。

## T2：验收

**文件：** 新建 `docs/ch00/03-log-source-location/checklist.md`。

1. 覆盖 source 格式、外部路径降级、no-op、并发与 MCP/dotenv 回归。
2. 运行 `go test -race ./internal/logging ./internal/mcp/... && go test ./...`。
3. 填写实际验收记录并提交。
