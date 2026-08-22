# Ch13 子 Agent 终态未写入运行日志

- 状态：自动化修复完成；真实 Provider fixture 复测待执行
- 发现日期：2026-08-23
- 影响范围：第十三章 SubAgent 可观测性与故障诊断。

## 现象

对 ch13 隔离 fixture 的 22 个日志文件（148 条有效 JSON 记录）做只读结构化扫描后，所有 22 条 `fields.stage=subagent` 的记录均为 `fields.status=running`；没有 `completed`、`failed` 或 `cancelled` 的子 Agent 终态记录。

同一批会话记录已证明终态实际发生：13 个前台任务返回 `completed`，8 个任务已异步启动，且人工场景 D/E 已观察到后台 completed 通知。因此这不是“任务未终止”，而是终态未进入应用日志。

## 影响

这不符合第十三章 N5 对关键阶段记录状态、工具调用计数、Token 用量和耗时等安全元数据的要求。任务在真实 Provider 环境中失败、取消或完成时，日志无法独立重建其终态和基本诊断信息。

## 初步定位

`internal/agent/subagent.go` 在任务启动后记录 `subagent launched`（状态为 running）；当前现场日志未见与 `TaskManager.finish()` 或子 Runner 终态对应的日志调用。需核对完成、失败和取消三个路径，并在统一终态边界记录安全元数据；不得记录 prompt、模型消息、工具结果正文、密钥、请求头或原始错误载荷。

## 建议修复与验证

1. 在任务终态统一边界增加一条 `stage=subagent` 日志，含 creation mode、任务状态、工具调用计数、Token 用量、耗时、模型和安全失败摘要（如适用）。
2. 为 completed、failed、cancelled 增加自动化断言，确认每种终态恰有一条终态日志且不含请求/响应正文。
3. 复跑 ch13 fixture 或等价集成测试，确认运行与终态日志成对出现。

## 当前进展

用户已授权修复此问题。当前章节位于 `pi/ch13-subagent` 开发分支；已确认启动日志写入点在 `internal/agent/subagent.go`，而 `internal/subagent/task_manager.go` 的 `finish` 是统一终态边界。尚未修改实现代码；将先按章节既有 Spec → Plan → Tasks → Checklist 流程取得批准。

用户已确认终态覆盖所有创建与前后台路径。已将 F16–F18、N13–N14、AC17–AC19 及范围边界回写到 `docs/ch13-subagent/spec.md`；Spec 已获批准，正在编写 Plan。

设计核对确认：`TaskManager.finish` 是单次合法终态迁移点，`LaunchRequest` 当前不携带终态回调；`SubAgentRuntime.dispatch` 已同时持有父 Logger、创建模式、调用参数和子 Runner 的实际模型。日志实现为同步文件写入，故终态回调必须在 TaskManager 完成状态更新和通知发布之后执行，避免日志 I/O 延迟任务可见终态。

Plan 已经用户逐段确认并回写 `docs/ch13-subagent/plan.md`：TaskManager 提供异步终态回调，Runtime 捕获日志上下文并仅记录安全元数据。Plan 已获批准。

已按该计划将 T26（终态回调）、T27（安全终态日志）、T28（验收与记录）回写 `docs/ch13-subagent/task.md`，并明确依赖顺序为 `T26 → T27 → T28`。Tasks 已获批准。

已将 AC17–AC19 的一次性记录、字段脱敏、创建/接管路径覆盖、非阻塞兼容和真实 Provider 复测项回写 `docs/ch13-subagent/checklist.md`；Checklist 已获批准，现按 T26–T28 实现。

实现前核对确认 `logging.Logger` 为同步文件写入；终态回调会在 `finish` 完成状态、关闭等待通道并发布通知后另起 goroutine 执行。`provider.Usage` 可提供 input、output、cache-read 与 cache-creation 用量；终态日志将仅汇总这些计数。

T26 已完成并验证：`LaunchRequest` 与受管任务已增加可选 `OnTerminal(TaskInfo)`；`finish` 在既有通知发布后异步调用它。`gofmt -w internal/subagent/task_manager.go internal/subagent/subagent_test.go && go test -race ./internal/subagent -run 'TestTaskManager.*(Completion|Terminal|Callback)' -count=1` 通过，覆盖 completed、failed、cancelled 快照回调及阻塞回调不阻塞 Wait/通知。

T27 实现中：`SubAgentRuntime.dispatch` 已为每个任务注册终态回调；回调以父 Logger、创建模式及子 Runner 实际模型写入 `subagent finished` 安全元数据日志。`gofmt -w internal/agent/subagent.go internal/agent/subagent_test.go && go test ./internal/agent ./internal/subagent -run 'Test(LogSubAgentTerminalWritesSafeMetadata|TaskManager.*(Completion|Terminal|Callback))' -count=1` 通过：completed、failed、cancelled 各有安全字段，且结果/失败 canary 不会进入日志。已新增定义式前台任务从 `dispatch` 注册回调、记录实际模型和最终 Token 用量的端到端测试。`gofmt -w internal/agent/subagent_test.go && go test ./internal/agent -run 'Test(DefinitionSubAgentRegistersTerminalLogCallback|LogSubAgentTerminalWritesSafeMetadata)$' -count=1` 通过。

随后已运行 `go test -race ./internal/subagent ./internal/agent -count=1 && go test ./internal/tools ./internal/tui -count=1 && go build ./cmd/mewcode && git diff --check`，全部通过。已在 checklist 记录 completed/failed/cancelled 单次回调、终态字段和脱敏自动化证据；Fork、显式/接管后台的专门终态日志测试及真实 Provider fixture 复测仍待完成。

`go test ./... -count=1` 的本次结果：除 `internal/provider/openai.TestStreamCapturesFinalRequestPayload` 外全部通过；该测试按当前用户临时保留完整 Provider 消息日志的行为，实际看到了 `request-canary`，并非本次终态日志改动导致。全量测试还生成既有 `cmd/mewcode/.mewcode/` 测试产物，已删除并同步 `bugs/2026-08-20/001-main-tests-leave-session-files.md`。已同步 Provider 日志问题记录。

实现结论：终态回调在任务状态提交、通知发布后异步执行；Runtime 为所有由 `dispatch` 创建的任务注册同一日志回调，因此前台、显式后台、Fork、ESC 与自动接管共用终态记录路径。`git diff --check` 已通过。

2026-08-23 真实 Provider fixture 已复测定义式前台 completed 路径。最新日志仅提取安全元数据后显示同一运行中有 `stage=subagent,status=running,mode=definition,background=false,model=default`，以及一条 `status=completed` 终态，含 `tool_calls=1`、`input_tokens=984`、`output_tokens=153`、两类 cache Token 均为 0、`duration_ms=11395`。这确认终态日志实际写入，且没有在复核输出中暴露正文。该子 Agent 的唯一读工具因模型选取的路径越过沙箱边界而失败，但模型随后结束任务，故终态为 completed；这不影响终态日志验证。真实 failed/cancelled 与后台路径复测仍待执行。
