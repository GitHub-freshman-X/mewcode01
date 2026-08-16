# Windows 启动时错误要求 Unix `SHELL` 环境变量

- 状态：已修复，待真实 Windows API 验证
- 发现日期：2026-08-16
- 影响范围：Windows 版 MewCode 的所有 Agent 请求

## 现象

在 Windows PowerShell 中启动 Windows 交叉编译版本后，输入消息会立即显示 `错误：request failed: prompt environment requires shell`，不会向模型服务发送请求。

## 复现条件

运行环境没有 `SHELL` 环境变量。Windows 的 PowerShell 和 CMD 默认符合该条件。

## 根因

`internal/prompt.CollectEnvironment` 无条件读取 `SHELL`，将空值视为不可恢复错误。该变量是 Unix shell 的惯例，并非 Windows 的通用环境变量。

## 诊断与验证

以下离线命令稳定复现同一错误：

```sh
env -u SHELL go test ./internal/prompt -run '^TestCollectEnvironment$' -count=1
```

输出：`CollectEnvironment returned error: prompt environment requires shell`。

## 修复方案

环境采集现在按 `SHELL`、`COMSPEC`、`unknown` 的顺序确定 Shell。这样保留 Unix 终端原有描述，并兼容 Windows PowerShell/CMD；两种变量均缺失时仍可构建环境系统消息，不会在模型请求前终止任务。

## 验证

- `go test ./internal/prompt -run 'TestCollectEnvironment(ShellPriority|COMSPECFallback|ShellFallback)$' -count=1` 已通过。
- `env -u SHELL go test ./internal/prompt -run '^TestCollectEnvironment$' -count=1` 已通过，原始复现不再报 `prompt environment requires shell`。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /private/tmp/mewcode-windows-amd64.exe ./cmd/mewcode` 已通过。
- `go test ./...` 的 Shell 相关包均通过；全量命令仍仅失败于已记录的 `bugs/2026-08-14/003-provider-request-body-logged.md`：`TestStreamCapturesFinalRequestPayload` 仍错误地要求日志包含请求正文，与当前安全日志实现不符。本次未改动 OpenAI Provider。

待在真实 Windows PowerShell 中启动新构建的程序并发送一次模型请求，确认界面不再出现该错误。
