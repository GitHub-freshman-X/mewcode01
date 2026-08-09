# MewCode `.env` 自动加载 Checklist

## 加载与优先级

- [ ] 项目根 `.env` 中的合法 `KEY=value` 在主配置读取前进入当前进程环境。（验证：`go test ./cmd/mewcode -run TestRunLoadsDotEnvBeforeConfig`。）
- [ ] 空行和 `#` 注释不影响其他变量加载。（验证：`go test ./internal/envfile -run TestLoadSetsMissingValues`。）
- [ ] 系统环境变量优先于 `.env` 同名变量，setter 不覆盖系统值。（验证：`go test ./internal/envfile -run TestLoadSkipsExistingValue`。）
- [ ] 父目录和用户目录的 `.env` 不被读取。（验证：`go test ./internal/envfile -run TestLoadOnlyUsesExplicitPath`。）

## 安全与降级

- [ ] 不存在 `.env` 时启动保持可用，并记录 `dotenv_not_found`。（验证：`go test ./cmd/mewcode -run TestRunLogsMissingDotEnv`。）
- [ ] 无效 `.env` 行只报告安全行号；stderr 和日志不含原行或变量值。（验证：`go test ./internal/envfile ./cmd/mewcode -run Test.*DotEnv.*Invalid`。）
- [ ] 系统冲突写入 `dotenv_variable_skipped`，日志包含变量名和状态但不含系统或 `.env` 值。（验证：`go test ./cmd/mewcode -run TestRunLogsDotEnvConflictWithoutValues`。）
- [ ] `.env` 不执行 shell、插值或命令。（验证：`go test ./internal/envfile -run TestLoadTreatsValueAsLiteral`。）

## MCP 集成与回归

- [ ] `.env` 的 `MCP_TEST_ROOT` 能在 `mcp_servers` 的 `${MCP_TEST_ROOT}` 中展开，成功 Server 继续注册工具。（验证：受控 MCP 集成测试或 filesystem 手工场景。）
- [ ] 日志按顺序包含 dotenv 成功事件和 MCP 注册事件，且不包含 `.env` 值。（验证：检查临时工作区 `logs/*.jsonl`。）
- [ ] `go test -race ./internal/envfile ./internal/mcp/...` 通过。
- [ ] `go test ./...` 通过。

## 验收记录

| 日期 | 命令或场景 | 结果 | 证据/备注 |
|---|---|---|---|
| 待执行 | `go test -race ./internal/envfile ./internal/mcp/...` | 未执行 | 实现完成后填写 |
| 待执行 | `go test ./...` | 未执行 | 实现完成后填写 |
| 待执行 | filesystem `.env` 手工场景 | 未执行 | 实现完成后填写 |
