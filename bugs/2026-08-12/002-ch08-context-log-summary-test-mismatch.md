# 上下文压缩日志测试要求摘要正文但实现按安全规则不记录

## 状态

待处理。

## 现象

`go test ./internal/agent -run 'Context|Persist|Readback' -count=1` 失败于 `TestRunnerLogsContextCompactionLifecycle`：测试要求完成日志包含 `summary: "logged state"`，实际日志没有该字段。

## 根因

`internal/agent/runner.go` 的完成日志只输出摘要候选数、长度和是否采用最后候选等安全元数据；包含摘要正文的日志调用已被注释。`internal/agent/runner_test.go` 仍断言日志正文存在，两者不一致。

该实现行为符合项目规则：关键流程日志不得记录 prompt 或工具结果正文；摘要正文同样不应写入诊断日志。

## 建议

将测试改为断言安全元数据并断言日志不含摘要正文；不要恢复正文日志。

## 验证

- `go test ./internal/agent -run 'Context|Persist|Readback' -count=1`：稳定复现。
- `go test ./...`：仅 `internal/agent/TestRunnerLogsContextCompactionLifecycle` 失败；其余包通过。
- `go test -race ./internal/agent -run '^TestRunnerLogsContextCompactionLifecycle$' -count=1`：以相同断言失败，确认 race CI 失败不是数据竞争。
- GitHub Actions 邮件对应运行中，macOS `test` 与 Linux `race` 失败；Windows、Ubuntu 的普通 `test` 为 fail-fast 取消，并非独立失败。
- 本问题与路径回读修复无共同修改文件；相关代码最后由 `b6e19eb feat: add chapter 8 context management` 引入。
