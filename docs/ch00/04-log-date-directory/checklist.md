# MewCode 日志日期目录 Checklist

- [ ] 固定本地日期创建日志后，文件位于 `logs/YYYY/MM/DD/`。（验证：`go test ./internal/logging -run TestNewUsesDateDirectory`。）
- [ ] 同日不同进程创建不同文件，不覆盖。（验证：`go test ./internal/logging -run TestNewUsesDistinctRunFiles`。）
- [ ] 文件创建失败仍遵循既有 no-op 降级。（验证：`go test ./cmd/mewcode -run TestRunContinuesWhenLoggingInitializationFails`。）
- [ ] 日志模块竞态测试与全项目测试通过。（验证：`go test -race ./internal/logging && go test ./...`。）
