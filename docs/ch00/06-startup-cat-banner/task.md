# 启动猫头横幅 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `cmd/mewcode/main.go` | 删除进入 TUI 前的猫头输出。 |
| 修改 | `cmd/mewcode/main_test.go` | 删除旧横幅断言，覆盖入口输出不含猫头。 |
| 修改 | `internal/tui/model.go` | 初始化仅显示的猫头状态。 |
| 修改 | `internal/tui/view.go` | 在初始 viewport 直接渲染无消息语义的猫头。 |
| 修改 | `internal/tui/tui_test.go` | 覆盖初始显示、会话隔离和不重复插入。 |
| 修改 | `docs/ch00/06-startup-cat-banner/checklist.md` | 修订并记录验收结果。 |
| 修改 | `bugs/2026-08-21/002-startup-cat-banner-rendered-outside-tui.md` | 记录修复方案、验证与最终状态。 |

## T1：移除入口横幅

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** 无

**步骤：**

1. 删除入口层的猫头常量、输出辅助函数和 TUI 前调用。
2. 调整现有启动测试，使正常路径断言入口诊断输出不含猫头。
3. 保留现有错误处理、退出码和 TUI 调用语义。

**验证：** `go test ./cmd/mewcode -count=1` 通过。

## T2：在初始 TUI 视图渲染猫头

**文件：** `internal/tui/model.go`、`internal/tui/view.go`

**依赖：** T1

**步骤：**

1. 在 TUI 包定义用户确认的猫头文本。
2. 让新建模型的初始 viewport 内容在既有显示内容前包含猫头。
3. 直接写入纯文本，不调用系统消息或消息块渲染函数。
4. 保持会话创建、清空和恢复流程不产生额外猫头。

**验证：** `go test ./internal/tui -run 'Test.*(View|Session|Clear|Resume|Banner)' -count=1` 通过。

## T3：覆盖隔离与不重复显示

**文件：** `internal/tui/tui_test.go`

**依赖：** T2

**步骤：**

1. 断言初始 TUI 内容含完整猫头且只出现一次。
2. 断言猫头没有消息标签或消息背景，且不进入会话显示快照和系统消息。
3. 覆盖新建、清空和恢复会话，断言当前 TUI 内容中猫头不会被额外插入。

**验证：** `go test ./internal/tui -count=1` 通过。

## T4：完成文档、Bug 记录与回归

**文件：** `docs/ch00/06-startup-cat-banner/checklist.md`、`bugs/2026-08-21/002-startup-cat-banner-rendered-outside-tui.md`

**依赖：** T1、T2、T3

**步骤：**

1. 根据已批准规格更新可观察验收项，并填写实际命令证据。
2. 更新 Bug 记录的修复方案、验证方式和最终状态。
3. 运行入口测试、TUI 测试、全项目测试、构建和差异检查。
4. 仅清理由本轮验证产生的运行目录或构建产物。

**验证：** `go test ./... -count=1`、`go build ./cmd/mewcode` 和 `git diff --check` 均通过。

## 执行顺序

```text
T1 → T2 → T3 → T4
```
