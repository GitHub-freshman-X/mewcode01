# `cmd/mewcode` 包测试在源码目录遗留运行目录

- 状态：待修复
- 发现日期：2026-09-06
- 影响范围：从仓库执行 `go test ./cmd/mewcode` 或 `go test ./...` 时的 CLI 启动测试。

## 现象

测试结束后，`cmd/mewcode/` 下会出现 `logs/` 以及 `.mewcode/sessions/<id>.jsonl`。这些目录是运行产物，不应留在源码包目录中。

## 最小复现

在目录不存在的干净状态下运行：

```sh
go test ./cmd/mewcode -run TestRunConfigOverride -count=1
```

该测试通过后，确认生成：

```text
cmd/mewcode/logs/
cmd/mewcode/.mewcode/sessions/<id>.jsonl
```

## 根因

`TestRunConfigOverride` 未切换至临时工作目录。Go 测试运行时的当前目录是包目录 `cmd/mewcode`；`launch` 以 `os.Getwd()` 取得该目录作为项目根，并在读取配置前初始化 logger。随后启动流程从同一根目录创建会话存储。因此 logger 创建 `logs/`，会话存储创建 `.mewcode/sessions/`。

这不是 `go build ./cmd/mewcode` 造成的；它只会生成被 `.gitignore` 忽略的根目录二进制。若用户手动在 `cmd/mewcode` 内运行 CLI，同一“当前目录即项目根”的设计也会在该目录创建运行数据。

## 修复方向

让启动测试统一在 `t.TempDir()` 中运行，或将创建日志与会话的启动准备提取为可显式传入工作区的函数供测试调用。修复后添加回归断言：运行 `TestRunConfigOverride` 前后，源码 `cmd/mewcode/` 不出现 `.mewcode` 或 `logs`。

## 验证进展

2026-09-06：运行最小复现后确认生成上述两个目录；本轮已删除该命令产生的 `cmd/mewcode/logs/` 和 `cmd/mewcode/.mewcode/`。本次只完成诊断，未修改实现。
