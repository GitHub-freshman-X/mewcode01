# `cmd/mewcode` 包测试在源码目录遗留运行目录

- 状态：已修复
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

2026-09-06：在复现前确认源码包目录不存在目标运行目录后，执行 `GOCACHE=/private/tmp/mewcode-go-build-cache go test ./cmd/mewcode -run TestRunConfigOverride -count=1`。测试通过，但随后确认生成 `cmd/mewcode/logs/2026/09` 与 `cmd/mewcode/.mewcode/sessions/<id>.jsonl`；该命令是稳定、可自动运行的复现。当前优先验证 `TestRunConfigOverride` 未切换至 `t.TempDir()` 的假设。

2026-09-06：已确认 `.mewcode` 与 `logs` 均由本轮最小复现生成，并已从 `cmd/mewcode/` 删除；未删除任何预先存在的用户文件。

2026-09-06：已创建 `docs/ch00/10-test-runtime-artifacts/spec.md`，明确修复范围为启动测试的临时工作目录隔离，不改变真实 CLI 的项目根目录语义。规格已确认，Plan、Tasks、Checklist 已生成，等待一次合并确认后修改测试。

2026-09-06：已在 `TestRunConfigOverride` 中将当前项目目录和用户配置目录切换为各自的 `t.TempDir()`，并以 `defer` 恢复全局函数与工作目录；生产启动入口未修改。

2026-09-06：最小复现已转绿：在修复前后均检查源码包目录不存在目标运行目录的前提下，执行 `GOCACHE=/private/tmp/mewcode-go-build-cache go test ./cmd/mewcode -run TestRunConfigOverride -count=1` 通过，结束后仍不存在 `cmd/mewcode/.mewcode` 与 `cmd/mewcode/logs`。继续运行完整包测试。

2026-09-06：完整受影响包验证 `GOCACHE=/private/tmp/mewcode-go-build-cache go test ./cmd/mewcode -count=1` 通过，`git diff --check` 通过；但随后发现源码 `cmd/mewcode/` 仍出现 `.mewcode/` 与 `logs/`。因此最小复现已修复、完整包仍有另一条启动测试路径，Bug 尚未完成；继续定位该路径。

2026-09-06：已确认上述两个目录由本轮完整包测试生成，并已删除；未删除任何预先存在的用户文件。

2026-09-06：进一步检查发现 `TestRunPermissionRulesMissingAllowed`、`TestRunPermissionInvalidRuleFails` 与 `TestRunSafeFailure` 同样未切换工作目录；它们分别执行成功启动或在 logger 初始化后失败。更新 Spec 的验收范围为整个 `cmd/mewcode` 包测试后源码目录无运行产物，等待用户重新确认后扩展 Plan、Tasks、Checklist 和测试修复。

2026-09-06：扩展后的 Spec 已确认；Plan、Tasks、Checklist 已同步纳入四条未隔离启动测试，等待一次合并确认后继续实现。

2026-09-06：已为 `TestRunPermissionRulesMissingAllowed`、`TestRunPermissionInvalidRuleFails` 与 `TestRunSafeFailure` 添加临时项目工作目录隔离；前两条同时隔离用户配置目录。待运行四条启动测试和完整包验证。

2026-09-06：四条启动测试的定向验证通过：`GOCACHE=/private/tmp/mewcode-go-build-cache go test ./cmd/mewcode -run 'TestRun(ConfigOverride|PermissionRulesMissingAllowed|PermissionInvalidRuleFails|SafeFailure)' -count=1`。执行前后源码包目录均不存在 `.mewcode` 与 `logs`；继续完整包验证。

2026-09-06：全仓验证首次执行中，`cmd/mewcode` 及多数包通过；`internal/hooks`、`internal/mcp`、`internal/provider/anthropic`、`internal/provider/openai` 因受限环境禁止 `httptest` 绑定本地端口而失败，错误为 `listen tcp6 [::1]:0: bind: operation not permitted`。该失败不涉及运行目录隔离；将以获得授权的相同命令复验。

2026-09-06：以获得授权的相同命令复验，`GOCACHE=/private/tmp/mewcode-go-build-cache go test ./... -count=1` 全部通过。四条未隔离启动测试现均使用临时工作目录；完整 `cmd/mewcode` 包测试后源码目录没有 `.mewcode` 或 `logs`，Bug 已修复。
