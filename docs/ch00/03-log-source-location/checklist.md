# MewCode 日志调用位置 Checklist

- [ ] 文件日志事件含 `source`，格式为项目相对路径加 1-based 行号。（验证：`go test ./internal/logging -run TestLoggerWritesCallerSource`。）
- [ ] `source` 指向业务调用点而非 `logger.go`。（验证：同一测试断言 `logger_test.go`。）
- [ ] `source` 不含项目绝对路径；无法相对化时省略该字段但日志仍写入。（验证：`go test ./internal/logging -run TestLoggerOmitsExternalSource`。）
- [ ] no-op 不创建文件且不捕获调用位置。（验证：`go test ./internal/logging -run TestNop`。）
- [ ] 并发日志保持完整 JSON 行且 race detector 通过。（验证：`go test -race ./internal/logging`。）
- [ ] MCP 与 dotenv 现有日志测试、全项目测试均通过。（验证：`go test -race ./internal/mcp/... && go test ./...`。）

## 验收记录

| 日期 | 命令 | 结果 | 备注 |
|---|---|---|---|
| 待执行 | `go test -race ./internal/logging ./internal/mcp/...` | 未执行 | 实现后填写 |
| 待执行 | `go test ./...` | 未执行 | 实现后填写 |
