# 测试运行目录隔离 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `cmd/mewcode/main_test.go` | 为配置覆盖启动测试隔离当前工作目录，并保留既有配置断言。 |
| 修改 | `bugs/2026-09-06/002-cmd-mewcode-tests-leave-runtime-directories.md` | 记录修复方案、验证命令与最终状态。 |
| 修改 | `bugs/2026-09-06/README.md`、`bugs/README.md` | 同步 Bug 索引状态。 |

## T1：隔离所有未隔离的启动测试

**文件：** `cmd/mewcode/main_test.go`  
**依赖：** 无

**步骤：**

1. 在 `TestRunConfigOverride`、`TestRunPermissionRulesMissingAllowed`、`TestRunPermissionInvalidRuleFails` 和 `TestRunSafeFailure` 创建 `t.TempDir()` 并保存当前工作目录。
2. 在每条测试调用 `run` 前切换到临时目录，并通过 `defer` 恢复原目录。
3. 对成功路径同时将用户配置目录替换为临时目录；保留全部现有替身、配置路径、退出码与错误断言。

**验证：** `go test ./cmd/mewcode -run 'TestRun(ConfigOverride|PermissionRulesMissingAllowed|PermissionInvalidRuleFails|SafeFailure)' -count=1` 通过。

## T2：验证源码目录不遗留运行产物

**文件：** `cmd/mewcode/main_test.go`、`bugs/2026-09-06/002-cmd-mewcode-tests-leave-runtime-directories.md`  
**依赖：** T1

**步骤：**

1. 在确认源码包目录不存在本轮运行目录的前提下执行完整 `cmd/mewcode` 包测试。
2. 断言测试通过后源码 `cmd/mewcode/` 仍没有 `.mewcode` 和 `logs`。
3. 仅清理由本轮验证生成的运行产物；将命令与结果写入 Bug 记录。

**验证：** `go test ./cmd/mewcode -count=1` 后目录检查通过。

## T3：完成受影响包验证与记录

**文件：** `bugs/2026-09-06/{README.md,002-cmd-mewcode-tests-leave-runtime-directories.md}`  
**依赖：** T1、T2

**步骤：**

1. 运行 `cmd/mewcode` 包完整测试与 `git diff --check`。
2. 将 Bug 002 标为已修复，并同步日期索引与根索引摘要。
3. 检查本轮没有在源码目录留下 `.mewcode/` 或 `logs/`。

**验证：** `go test ./cmd/mewcode -count=1` 与 `git diff --check` 均通过。

## 执行顺序

```text
T1 → T2 → T3
```
