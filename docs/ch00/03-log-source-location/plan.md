# MewCode 日志调用位置 Implementation Plan

**Goal:** 为每条文件日志增加业务调用点的项目相对文件与行号。

**Architecture:** `Logger` 保存项目根目录；实际文件写入前通过 Go 调用栈捕获调用者，跳过日志模块帧，将路径相对化后填入 Event 的 `source` 字段。no-op 记录器在写入前返回，不调用栈捕获。

**Tech Stack:** Go 标准库 `runtime`、`path/filepath`、既有 `internal/logging`。

## 设计

- `Event` 新增可选 `Source string`，JSON 字段为 `source`。
- `Logger.New` 保存清理后的项目根；`Nop` 不保存根且不会捕获位置。
- `log` 在确认文件可写后用 `runtime.Caller` 查找日志包外首个调用帧；仅接受可相对化且不以 `..` 开头的路径。
- 位置格式固定为 `<slash-separated-relative-path>:<line>`；无法定位时为空。
- 不改变 Info、Error、WithFields 或 MCP/dotenv 调用点接口。

## 文件

| 操作 | 文件 | 内容 |
|---|---|---|
| 修改 | `internal/logging/logger.go` | Event 字段、根目录保存、调用点捕获 |
| 修改 | `internal/logging/logger_test.go` | 相对路径、行号、no-op 与并发回归 |
| 新建 | `docs/ch00/03-log-source-location/task.md` | 实现任务 |
| 新建 | `docs/ch00/03-log-source-location/checklist.md` | 验收清单 |

## 覆盖

- F1/F2：调用点帧选择与 `source` 格式测试。
- F3：工作区外帧为空且仍写入测试。
- F4/N1/N2：既有接口、race 与 no-op 回归测试。
