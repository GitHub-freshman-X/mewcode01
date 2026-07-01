# MewCode Agent Loop Checklist

> 每一项都通过运行代码、自动化测试或观察用户可见行为验证。实现文件如何拆分不影响这些行为标准。

## Agent Loop 核心行为

- [x] **AC1 独立任务入口**：不启动 TUI，仅使用测试消费者提交任务，也能持续收到事件直至任务终态。（验证：运行 `go test ./internal/agent -run StandaloneConsumer`，期望测试通过）
- [x] **AC2 多步自主循环**：模型依次请求读工具、写工具并返回最终文本时，系统自动完成所有轮次，中途不需要新的用户输入。（验证：运行 `go test ./internal/agent -run RunnerToolLoop`，期望模型请求次数为 3 且任务正常完成）
- [x] **AC3 最终回答停止**：模型返回无工具调用的完整响应后，不再请求模型并报告 `final_answer`。（验证：运行 `go test ./internal/agent -run RunnerFinalAnswer`，期望仅有一次模型请求和一个完成终态）
- [x] **完整轮次提交**：每次后续模型请求只看到已完整处理的 assistant 调用及其全部工具结果。（验证：运行 `go test ./internal/agent -run AtomicRoundHistory`，期望每个调用 ID 均有对应结果）
- [x] **AC13 工具失败可自我调整**：已知工具返回结构化失败后，模型收到失败结果并可在下一轮改用正确参数完成任务。（验证：运行 `go test ./internal/agent -run RunnerToolFailure`，期望失败结果进入下一请求且任务最终完成）

## 停止条件与取消

- [x] **AC4 默认迭代上限**：未配置上限且模型持续调用工具时，最多发起 20 次模型请求，随后报告 `iteration_limit`。（验证：运行 `go test ./internal/agent -run RunnerDefaultIterationLimit`，期望请求数恰为 20）
- [x] **AC4 配置迭代上限**：把上限设为 2 后，不出现第 3 次模型请求。（验证：运行 `go test ./internal/agent -run RunnerConfiguredIterationLimit`，期望请求数恰为 2）
- [x] **边界轮最终回答优先**：上限轮返回最终文本时按正常完成处理，而不是误报达到上限。（验证：运行 `go test ./internal/agent -run RunnerFinalAnswerAtLimit`，期望原因是 `final_answer`）
- [x] **AC5 模型流期间取消**：生成文本期间取消后进入取消终态，不提交部分轮次、不发起下一轮。（验证：运行 `go test ./internal/agent -run RunnerCancelDuringStream`，期望历史不含当前轮且请求数不再增加）
- [x] **AC5 并发只读期间取消**：只读批次运行时取消后，后续副作用批次不启动。（验证：运行 `go test ./internal/agent -run RunnerCancelDuringReadBatch`，期望写工具调用次数为 0）
- [x] **AC5 副作用工具期间取消**：当前执行收到取消，后续工具和模型轮次不启动。（验证：运行 `go test ./internal/agent -run RunnerCancelDuringSideEffect`，期望唯一终态为取消）
- [x] **AC6 连续未知工具停止**：连续第三次未知工具调用处理并提交结果后停止，不再请求模型。（验证：运行 `go test ./internal/agent -run RunnerUnknownToolLimit`，期望原因是 `unknown_tool_limit`）
- [x] **AC6 已知工具重置计数**：两个未知、一个已知、再两个未知不会触发连续三次阈值。（验证：运行 `go test ./internal/agent -run RunnerUnknownToolReset`，期望循环继续到模型最终回答）
- [x] **AC7 流读取失败**：流错误后不执行该轮收集到的工具、不提交该轮、不继续调用模型。（验证：运行 `go test ./internal/agent -run RunnerStreamError`，期望失败终态且工具调用次数为 0）
- [x] **AC7 流解析失败**：缺失调用 ID、名称或非法 JSON 均产生可诊断失败终态，而不是 panic。（验证：运行 `go test ./internal/agent -run CollectorMalformedToolCall`，期望各错误场景通过）

## 异步事件与双路收集

- [x] **AC8 部分文本可见但不提交**：取消或流错误前的文本增量可以被消费者看到，并被标记为部分输出，但 Session 不含该响应。（验证：运行 `go test ./internal/agent -run PartialOutputNotCommitted`，期望事件含文本、历史不含文本）
- [x] **AC9 事件类型完整**：完整工具任务可观察到进度、文本、工具调用、工具结果、Usage 和唯一终态。（验证：运行 `go test ./internal/agent -run CompleteEventSequence`，期望所需事件集合全部出现）
- [x] **AC9 终态唯一且最后**：完成、停止、取消和失败场景都恰有一个终态，终态后通道关闭且无普通事件。（验证：运行 `go test ./internal/agent -run TerminalEventInvariant`，期望四种场景通过）
- [x] **AC10 文本实时转发**：模型流尚未结束时，消费者已经收到首个文本增量。（验证：运行 `go test ./internal/agent -run CollectorStreamsImmediately`，期望测试在释放结束信号前收到文本）
- [x] **AC10 完整响应一致**：Collector 返回的文本等于全部文本分片按序拼接结果，工具参数分片也完整拼接。（验证：运行 `go test ./internal/agent -run CollectorReassemblesResponse`，期望文本和 JSON 均完全一致）
- [x] **进度可观测**：每个模型轮次和工具阶段都携带正确轮次号与阶段，终态包含完成轮数。（验证：运行 `go test ./internal/agent -run ProgressEvents`，期望轮次从 1 单调递增）
- [x] **消费者取消可回收**：消费者调用 Cancel 后继续排空通道，生产任务能结束且通道关闭。（验证：运行 `go test -race ./internal/agent -run ConsumerCancelCleanup`，期望无超时或竞态）

## 工具安全调度

- [x] **AC11 连续只读并发**：同一连续只读批次中的两个阻塞工具存在执行时间重叠。（验证：运行 `go test ./internal/agent -run SchedulerReadOnlyConcurrent`，期望并发屏障测试通过）
- [x] **AC11 副作用串行**：两个副作用工具不会重叠执行，并保持模型给出的顺序。（验证：运行 `go test ./internal/agent -run SchedulerSideEffectsSerial`，期望最大并发数为 1）
- [x] **AC11 跨批次屏障**：“读、读、写、读、命令”严格按四个批次推进，后批次不早于前批次结束。（验证：运行 `go test ./internal/agent -run SchedulerBatchBarriers`，期望记录时序符合设计）
- [x] **AC11 结果原序**：只读工具以相反顺序完成时，结果事件与模型上下文仍按原始调用顺序排列。（验证：运行 `go test ./internal/agent -run SchedulerResultOrder`，期望调用 ID 顺序保持不变）
- [x] **AC12 保守默认分类**：没有声明分类的工具按副作用串行处理。（验证：运行 `go test ./internal/agent -run SchedulerUnclassifiedTool`，期望不进入并发批次）
- [x] **Plan Mode 执行范围隔离**：模型直接请求隐藏的写工具时得到未找到结果，工作区保持不变。（验证：运行 `go test ./internal/agent -run PlanHiddenWriteTool`，期望写工具执行次数为 0）

## Token Usage

- [x] **AC14 单轮 Usage**：每次 Provider 请求最多产生一个包含输入和输出 Token 的规范化 Usage 事件。（验证：分别运行 `go test ./internal/provider/anthropic -run Usage` 和 `go test ./internal/provider/openai -run Usage`，期望均通过）
- [x] **AC14 任务累计 Usage**：两轮用量分别为 `(10, 4)` 和 `(20, 6)` 时，终态累计为 `(30, 10)`。（验证：运行 `go test ./internal/agent -run RunnerUsageTotal`，期望数值精确相等）
- [x] **缺失 Usage 可兼容**：Provider 不提供 Usage 时任务仍正常完成，累计值保持零。（验证：运行 `go test ./internal/agent -run RunnerWithoutUsage`，期望无错误终态）

## Plan Mode

- [x] **AC15 `/plan` 只开放读工具**：规划请求的工具声明恰好包含读文件、找文件和搜代码。（验证：运行 `go test ./internal/agent -run PlanToolDefinitions`，期望工具集合精确匹配）
- [x] **AC15 规划可多轮探索**：模型可连续使用只读工具并最终输出计划，期间工作区内容不变。（验证：运行 `go test ./internal/agent -run PlanReadOnlyLoop`，期望多次模型请求后正常完成并比较工作区快照一致）
- [x] **AC16 成功计划保存**：正常完成的非空计划成为当前会话最近计划。（验证：运行 `go test ./internal/agent -run PlanSavedOnSuccess`，期望 LatestPlan 返回最终文本）
- [x] **AC16 失败计划不覆盖**：已有有效计划后，再执行取消、流失败、迭代停止或未知工具停止的规划，旧计划保持不变。（验证：运行 `go test ./internal/agent -run PlanPreservedOnNonSuccess`，期望四种场景均保留旧值）
- [x] **AC17 `/do` 恢复全工具**：执行最近计划时请求包含全部六个工具，并能修改工作区直到最终回答。（验证：运行 `go test ./internal/agent -run DoUsesFullRegistry`，期望工具集合和写入结果正确）
- [x] **AC17 `/do` 明确携带计划**：执行首轮 user 消息包含最近计划，而不是只依赖较早历史。（验证：运行 `go test ./internal/agent -run DoPromptContainsPlan`，期望请求正文包含保存的计划）
- [x] **AC18 无计划 `/do`**：当前会话没有计划时立即返回清晰错误，不创建 Task、不调用模型和工具。（验证：运行 `go test ./internal/agent -run DoWithoutPlan`，期望 Provider 请求数和工具调用数均为 0）

## 集成与架构边界

- [x] **AC1/AC16 TUI 单向消费**：TUI 仅启动 Agent 任务和消费 Agent 事件，不直接调用 Provider 或工具执行器推进循环。（验证：运行 `rg -n 'provider\.Stream|Executor\.Execute|\.Complete\(|\.Apply\(' internal/tui`，期望无匹配）
- [x] **Provider 中立**：Agent 包不导入 Anthropic 或 OpenAI adapter，也不解析供应商原始事件名称。（验证：运行 `rg -n 'provider/(anthropic|openai)|response\.|message_start' internal/agent`，期望无匹配）
- [x] **旧编排入口已移除**：项目中不存在旧 Conversation 单轮推进调用。（验证：运行 `rg -n 'NewConversation|\.Apply\(|\.Complete\(' internal cmd`，期望无匹配）
- [x] **Session 深拷贝**：修改 Snapshot 返回值不会改变内部历史，工具参数字节也不会共享。（验证：运行 `go test ./internal/conversation -run SessionSnapshotIsolation`，期望通过）
- [x] **配置默认与覆盖**：旧配置自动获得 20 轮默认值，合法正数生效，负数返回明确错误。（验证：运行 `go test ./internal/config -run Agent`，期望三类配置场景通过）
- [x] **TUI 命令映射**：普通输入、`/plan <任务>`、精确 `/do` 映射到正确模式，空 `/plan` 不启动任务。（验证：运行 `go test ./internal/tui -run 'ActCommand|PlanCommand|DoCommand'`，期望全部通过）
- [x] **TUI 用户可见状态**：界面可观察轮次、阶段、工具状态、累计 Token、正常完成、停止、取消、失败和部分输出标记。（验证：运行 `go test ./internal/tui -run 'View|Partial|Usage'`，期望关键文本断言通过）
- [x] **用户文档一致**：README、示例配置和章节索引包含正确命令、默认值与边界。（验证：运行 `rg -n 'Agent Loop|/plan|/do|max_iterations|ch04' README.md config.example.yaml docs/README.md`，期望所有条目均可找到）

## 稳定性与质量

- [x] **AC19 工具异常不崩溃**：工具 panic、超时和结构化失败不会产生未处理 panic。（验证：运行 `go test ./internal/agent ./internal/tools -run 'Panic|Timeout|Failure'`，期望全部通过）
- [x] **AC19 无半轮历史**：取消、流错误及调度取消场景均不留下只有 tool call 没有 tool result 的历史。（验证：运行 `go test ./internal/agent -run IncompleteRoundNeverCommitted`，期望全部场景通过）
- [x] **AC19 并发无竞态**：Agent、Session、Scheduler 和 TUI 并发测试无数据竞争。（验证：运行 `go test -race ./...`，期望退出码为 0）
- [x] **AC20 无真实 API 依赖**：完整测试套件不需要模型凭据或网络访问。（验证：清除模型相关环境变量后运行 `go test ./...`，期望退出码为 0）
- [x] **项目编译通过**：所有生产包和命令可编译。（验证：运行 `go build ./...`，期望退出码为 0）
- [x] **全部测试通过**：单元与集成测试全部通过。（验证：运行 `go test ./...`，期望退出码为 0）
- [x] **静态检查通过**：没有 Go 静态检查错误。（验证：运行 `go vet ./...`，期望退出码为 0）
- [x] **格式检查通过**：所有 Go 文件均已格式化。（验证：运行 `test -z "$(gofmt -l cmd internal)"`，期望无输出）

## 端到端场景

- [ ] **E2E 1：自主完成代码任务**：在临时工作区要求 MewCode“读取目标文件、修改指定内容并运行测试”；观察它自动经历多轮工具调用，文件修改正确、测试完成并给出最终回答。（验证：使用本地脚本化 Provider 启动完整 TUI/Runner 组装，期望无需第二次用户输入且终态为 `final_answer`）
- [ ] **E2E 2：先规划再执行**：输入 `/plan <修改任务>`，确认规划期间工作区不变；随后输入 `/do`，确认执行使用保存计划、可调用全部工具并完成修改。（验证：比较规划前后及执行后的工作区快照，期望仅 `/do` 阶段发生目标变更）
- [ ] **E2E 3：中途取消**：在长时间模型流或工具执行期间按 Ctrl+C，观察部分输出标记为取消、没有后续副作用，随后可立即提交新任务。（验证：运行 TUI 端到端取消场景，期望进程不退出且第二个任务正常完成）
- [ ] **E2E 4：安全网停止**：使用持续请求工具的脚本化 Provider，把最大迭代数配置为 2；观察两轮完成后明确显示达到上限，且没有第 3 次请求。（验证：检查 Provider 调用计数和 TUI 终态文本）
