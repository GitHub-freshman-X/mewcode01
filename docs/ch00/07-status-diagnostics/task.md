# `/status` 诊断信息 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/status/status.go`、`status_test.go` | 安全状态 DTO、Provider 接口与字段边界测试 |
| 修改 | `internal/subagent/task_manager.go`、`subagent_test.go` | 后台标记、接管和运行中后台快照 |
| 修改 | `internal/agent/subagent.go`、`subagent_test.go` | ESC/自动接管时标记后台 |
| 修改 | `internal/memory/service.go`、`memory_test.go` | 内存记忆计数缓存与更新 |
| 修改 | `internal/agent/event.go`、`runner.go`、`runner_test.go` | 日志目录配置、运行时状态快照 |
| 修改 | `internal/command/command.go`、`builtins.go`、`command_test.go` | Status Provider 注入、多行输出与降级 |
| 修改 | `internal/tui/update.go`、`tui_test.go` | 命令分发传入 Runner Status Provider |
| 修改 | `cmd/mewcode/main.go`、`main_test.go` | 组合根传入日志目录与初始记忆状态 |
| 修改 | `README.md`、`bugs/2026-08-22/*` | 用户可见命令说明与缺口记录/验证证据 |

## T1：定义安全状态 DTO 与命令依赖

**文件：** `internal/status/status.go`、`internal/status/status_test.go`、`internal/command/command.go`

**依赖：** 无

**步骤：**
1. 定义 Snapshot、运行中后台任务条目和 Provider 接口，只保留 Spec 允许输出的字段。
2. 为 `CommandContext` 增加可选 Status Provider，不改变其他本地命令的依赖。
3. 测试 DTO 不含任务描述、结果、失败、Prompt 或配置正文等敏感字段；Provider 缺失可由调用方识别。

**验证：** `go test ./internal/status ./internal/command -count=1` 通过。

## T2：标记并安全快照运行中后台任务

**文件：** `internal/subagent/task_manager.go`、`internal/subagent/subagent_test.go`、`internal/agent/subagent.go`、`internal/agent/subagent_test.go`

**依赖：** T1

**步骤：**
1. 在启动请求和任务信息中记录 Background；显式后台与 Fork 在创建时标记。
2. 实现 TaskManager 的后台接管标记和运行中后台快照；使用同一锁保护标记、进度和终态更新。
3. 让 ESC 和 120 秒自动接管在返回异步结果前标记同一任务，不重启 Worker。
4. 按 StartedAt、ID 稳定排序快照；终态任务不进入结果。
5. 覆盖显式后台、Fork、ESC、自动接管、终态移除和并发查询。

**验证：** `go test -race ./internal/subagent ./internal/agent -run 'Test.*(Background|SubAgent|TaskManager)' -count=1` 通过。

## T3：维护记忆数量缓存

**文件：** `internal/memory/service.go`、`internal/memory/memory_test.go`、`cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** T1

**步骤：**
1. 增加只读记忆状态摘要和线程安全缓存；缓存同时表达可用性及用户/项目数量。
2. 在启动装配期读取初始索引或目录并初始化缓存；该 I/O 不发生在 `/status` 调用中。
3. 在手动新增、清空及自动提取成功后更新缓存；失败操作不得虚报成功后的数量。
4. 覆盖空目录、未配置/初始化失败、手动增删、自动操作和并发读取。

**验证：** `go test -race ./internal/memory ./cmd/mewcode -run 'Test.*(Memory|Status)' -count=1` 通过。

## T4：聚合 Runner 运行时状态

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T1、T2、T3

**步骤：**
1. 在 Options 中加入日志目录，由启动入口传入；保持现有默认与测试装配兼容。
2. 实现 Runner 的 StatusSnapshot：工作区、日志目录、权限模式、实际 registry 工具数、Skill 数、记忆缓存、定义数及运行中后台任务。
3. 将 TaskManager 任务快照转换为仅含 ID、名称、`running` 和时长的状态 DTO；使用可注入时钟覆盖时长。
4. 对 nil 权限、Skill、Memory、SubAgent 与日志目录执行降级，不 panic。
5. 测试实际 Runner registry（含动态 Load Skill 工具）、安全字段、稳定任务顺序、可选模块降级和零外部调用。

**验证：** `go test ./internal/agent -run 'Test.*(Status|Snapshot|SubAgent)' -count=1` 通过。

## T5：格式化并接入 `/status`

**文件：** `internal/command/builtins.go`、`internal/command/command_test.go`、`internal/tui/update.go`、`internal/tui/tui_test.go`

**依赖：** T4

**步骤：**
1. 将 UI 的模式/Token、SessionService 的会话 ID/消息数和 Status Snapshot 格式化为固定顺序的中文多行文本。
2. 在 TUI 命令分发中把当前 Runner 作为 Status Provider 传入；保持 `/status` 为本地命令。
3. 无后台任务时显示 `后台任务：0`；有任务时显示仅允许的安全元数据和可读时长。
4. Provider 或 SessionService 缺失时保留模式/Token，其他字段显示不可用或零。
5. 覆盖新会话、恢复会话、计划模式、后台任务显示/终态移除、敏感标记不泄露及零 Agent/Provider/会话写入。

**验证：** `go test ./internal/command ./internal/tui -run 'Test.*(Status|Local)' -count=1` 通过。

## T6：同步用户文档、回归与问题记录

**文件：** `README.md`、`bugs/2026-08-22/README.md`、`bugs/2026-08-22/001-status-documentation-mismatch.md`

**依赖：** T5

**步骤：**
1. 更新 README 的 `/status` 说明，使其列出实际多行诊断范围且不承诺敏感内容。
2. 更新 bug 记录的状态、实现方案和实际验证证据；未完成项明确标为待处理。
3. 格式化变更 Go 文件，运行相关包、竞态、全量测试和构建。

**验证：** `gofmt -w $(git diff --name-only -- '*.go') && go test -race ./internal/status ./internal/subagent ./internal/agent ./internal/memory ./internal/command ./internal/tui && go test ./... -count=1 && go build ./cmd/mewcode` 通过。

## 执行顺序

```text
T1 ─┬→ T2 ─┐
    ├→ T3 ─┼→ T4 → T5 → T6
    └──────┘
```
