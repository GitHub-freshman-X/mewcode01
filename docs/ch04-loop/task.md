# MewCode Agent Loop Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/agent/event.go` | Agent 事件、阶段、停止原因和任务句柄 |
| 新建 | `internal/agent/runner.go` | ReAct 循环、任务生命周期和停止策略 |
| 新建 | `internal/agent/collector.go` | Provider 流双路收集 |
| 新建 | `internal/agent/scheduler.go` | 工具安全分批和执行调度 |
| 新建 | `internal/agent/mode.go` | Act、Plan、Do 请求构造与多计划快照 |
| 新建 | `internal/agent/history.go` | Plan Mode 单任务临时历史 |
| 新建 | `internal/agent/runner_test.go` | 循环、停止条件和提交边界测试 |
| 新建 | `internal/agent/collector_test.go` | 流分片、Usage、错误和取消测试 |
| 新建 | `internal/agent/scheduler_test.go` | 并发、串行、顺序和取消测试 |
| 新建 | `internal/agent/mode_test.go` | Plan/Do 模式与多计划提示测试 |
| 新建 | `internal/agent/history_test.go` | 临时历史多轮提交与共享隔离测试 |
| 新建 | `internal/conversation/session.go` | 模型历史、展示记录、轮次构造和有序待执行计划列表 |
| 新建 | `internal/conversation/session_test.go` | 双历史隔离、原子提交、计划追加与消费测试 |
| 修改 | `internal/provider/event.go` | 增加规范化 Token Usage |
| 修改 | `internal/provider/anthropic/stream.go` | 合并 Anthropic 请求用量 |
| 修改 | `internal/provider/anthropic/anthropic_test.go` | 增加 Anthropic Usage 测试 |
| 修改 | `internal/provider/openai/stream.go` | 提取 OpenAI 请求用量 |
| 修改 | `internal/provider/openai/openai_test.go` | 增加 OpenAI Usage 测试 |
| 修改 | `internal/tools/tool.go` | 增加工具安全分类 |
| 修改 | `internal/tools/read_file.go` | 声明只读分类 |
| 修改 | `internal/tools/find_files.go` | 声明只读分类 |
| 修改 | `internal/tools/search_code.go` | 声明只读分类 |
| 修改 | `internal/tools/write_file.go` | 声明副作用分类 |
| 修改 | `internal/tools/edit_file.go` | 声明副作用分类 |
| 修改 | `internal/tools/run_command.go` | 声明副作用分类 |
| 修改 | `internal/tools/registry.go` | 按安全分类过滤 Registry |
| 修改 | `internal/tools/executor.go` | 支持任务范围 Registry |
| 修改 | `internal/tools/tools_test.go` | 安全分类、过滤和范围执行测试 |
| 修改 | `internal/config/config.go` | 增加 AgentConfig 和默认值 |
| 修改 | `internal/config/validate.go` | 校验最大迭代数 |
| 修改 | `internal/config/config_test.go` | Agent 配置测试 |
| 修改 | `internal/tui/commands.go` | 等待 Agent 事件流 |
| 修改 | `internal/tui/model.go` | 保存 Runner、Session、Task 和临时展示状态 |
| 修改 | `internal/tui/update.go` | 启动任务、消费事件和传播取消 |
| 修改 | `internal/tui/view.go` | 展示循环进度、工具、Usage 和终态 |
| 修改 | `internal/tui/run.go` | 更新 TUI 构造依赖 |
| 修改 | `internal/tui/tui_test.go` | Agent 命令和事件消费测试 |
| 修改 | `cmd/mewcode/main.go` | 组装 Session、Runner 与 TUI |
| 修改 | `cmd/mewcode/main_test.go` | 启动依赖和配置传递测试 |
| 修改 | `config.example.yaml` | 增加 `agent.max_iterations` 示例 |
| 修改 | `README.md` | 说明 Agent Loop、`/plan` 和 `/do` |
| 修改 | `docs/README.md` | 增加第 4 章索引 |
| 删除 | `internal/conversation/conversation.go` | 移除旧单轮编排入口 |
| 删除 | `internal/conversation/stream.go` | 移除旧流事件应用逻辑 |
| 删除 | `internal/conversation/conversation_test.go` | 由 Agent 与 Session 测试替代 |

## T1：定义 Provider Usage 契约

**文件：** `internal/provider/event.go`

**依赖：** 无

**步骤：**

1. 定义包含输入和输出 Token 的 `Usage`，实现累加方法。
2. 为 Provider 流事件增加 `EventUsage` 和可选 Usage 字段。
3. 保持现有事件类型和值不变，确保无 Usage 的 Provider 流继续可用。

**验证：** 运行 `go test ./internal/provider/...`，期望现有 Provider 测试全部通过。

## T2：输出 Anthropic 单轮 Usage

**文件：** `internal/provider/anthropic/stream.go`、`internal/provider/anthropic/anthropic_test.go`

**依赖：** T1

**步骤：**

1. 从消息开始事件读取输入 Token，从消息增量或结束数据读取输出 Token。
2. 在一次请求结束前合并为一个 `provider.Usage` 事件，避免重复上报累计快照。
3. 增加包含输入、输出和缺失 Usage 的流测试。

**验证：** 运行 `go test ./internal/provider/anthropic -run Usage`，期望恰好收到一个正确汇总的 Usage 事件。

## T3：输出 OpenAI 单轮 Usage

**文件：** `internal/provider/openai/stream.go`、`internal/provider/openai/openai_test.go`

**依赖：** T1

**步骤：**

1. 从完成响应中读取输入和输出 Token。
2. 每次请求最多发出一个规范化 Usage 事件。
3. 增加有 Usage 和无 Usage 的流测试，确认原有文本及工具事件顺序不变。

**验证：** 运行 `go test ./internal/provider/openai -run Usage`，期望 Usage 数值和事件数量均正确。

## T4：声明工具安全分类

**文件：** `internal/tools/tool.go`、`internal/tools/read_file.go`、`internal/tools/find_files.go`、`internal/tools/search_code.go`、`internal/tools/write_file.go`、`internal/tools/edit_file.go`、`internal/tools/run_command.go`、`internal/tools/tools_test.go`

**依赖：** 无

**步骤：**

1. 定义 `SafetyReadOnly` 和 `SafetySideEffect`，并在 Metadata 增加 Safety。
2. 为三个读取类工具声明只读，为写、改、命令工具声明副作用。
3. 提供归一化辅助，使空值和未知值按副作用处理。
4. 增加六个内置工具及未分类测试工具的分类断言。

**验证：** 运行 `go test ./internal/tools -run Safety`，期望六个工具分类正确且默认分类为副作用。

## T5：实现 Registry 安全过滤

**文件：** `internal/tools/registry.go`、`internal/tools/tools_test.go`

**依赖：** T4

**步骤：**

1. 实现 `FilterBySafety`，返回独立 Registry，不共享可变名称映射。
2. 确保只读过滤结果仅含 `read_file`、`find_files`、`search_code`。
3. 验证过滤结果的 Definitions 与 Get 使用同一工具范围。

**验证：** 运行 `go test ./internal/tools -run FilterBySafety`，期望只读副本恰含三个工具且原 Registry 不变。

## T6：增加范围化工具执行能力

**文件：** `internal/tools/executor.go`、`internal/tools/tools_test.go`

**依赖：** T5

**步骤：**

1. 抽取以调用时 Registry 查找工具的执行路径，保留现有入口作为迁移期兼容层。
2. 确保超时、参数校验、panic 捕获和结构化错误语义不变。
3. 测试完整 Registry 可执行写工具，而只读 Registry 对同名写工具返回未找到且不修改文件。

**验证：** 运行 `go test ./internal/tools -run 'Scoped|Executor'`，期望范围隔离及原执行器回归测试通过。

## T7：实现 Conversation Session

**文件：** `internal/conversation/session.go`、`internal/conversation/session_test.go`

**依赖：** 无

**步骤：**

1. 将模型上下文历史与 TUI 展示记录拆成两个线程安全、可深拷贝的快照。
2. 抽取 `BuildRound` 纯函数，校验 user、assistant、全部 tool result 的调用标识对应关系并返回克隆消息。
3. 让普通 `CommitRound` 原子地把完整轮次写入模型历史和展示记录。
4. 实现 `CommitPlan`：只把用户可见的原始规划请求与最终计划写入展示记录，并原子追加非空计划，模型历史保持不变。
5. 保留按计划快照消费逻辑：只移除匹配前缀并保留并发追加的尾部计划。
6. 增加双历史隔离、返回切片隔离、Plan 提交、计划消费和完整轮次顺序测试。

**验证：** 运行 `go test ./internal/conversation -run 'BuildRound|Session.*History|Session.*Plan|SessionSnapshotIsolation'`，期望模型/展示历史隔离、轮次校验和计划原子提交通过。

## T8：定义 Agent 公共事件与请求类型

**文件：** `internal/agent/event.go`

**依赖：** T1

**步骤：**

1. 定义 Mode、Request、Options、默认迭代数和未知工具阈值。
2. 定义进度、文本、工具、Usage 和四类终态事件。
3. 定义 Phase、StopReason、Summary 与可取消 Task 句柄。
4. 提供 Options 默认值归一化辅助。

**验证：** 运行 `go test ./internal/agent`，期望新包可编译且默认最大迭代数为 20。

## T9：实现模式请求构造

**文件：** `internal/agent/mode.go`、`internal/agent/mode_test.go`

**依赖：** T5、T7、T8

**步骤：**

1. 普通模式保留用户任务原文并使用完整 Registry。
2. Plan Mode 包装只读探索和最终计划指令，并使用只读 Registry。
3. Do Mode 从 Session 读取全部待执行计划，保存本次快照，并按追加顺序编号构造包含全部计划原文的执行指令。
4. 无计划或额外 Prompt 时返回验证错误；计划之间存在重复或冲突时不得在系统层过滤、覆盖或合并。
5. 测试三种模式的消息内容、工具声明范围、多计划顺序与冲突计划完整保留。

**验证：** 运行 `go test ./internal/agent -run 'Mode|DoPromptContainsAllPlans'`，期望 Act/Plan/Do 请求、工具范围和多计划提示符合设计。

## T10：实现文本与 Usage 双路收集

**文件：** `internal/agent/collector.go`、`internal/agent/collector_test.go`

**依赖：** T8

**步骤：**

1. 消费 Provider 事件与 done 通道，直到同时确认完成事件和无错误结束。
2. 收到文本分片时立即 emit，同时累积到 assistant 文本块。
3. 累积 Thinking、Signature 和每轮 Usage，并发出 Agent Usage 事件。
4. 测试分片即时可见、完整文本一致和缺失 Usage 不报错。

**验证：** 运行 `go test ./internal/agent -run 'Collector.*Text|Collector.*Usage'`，期望增量事件与完整响应一致。

## T11：实现工具调用收集与流错误边界

**文件：** `internal/agent/collector.go`、`internal/agent/collector_test.go`

**依赖：** T10

**步骤：**

1. 按 block index 累积工具 ID、名称和 JSON 参数分片。
2. 完成时输出保持模型顺序的 ToolCalls。
3. 对缺失 ID、缺失名称、非法 JSON、未知 Provider 事件和 done error 返回可诊断错误。
4. 测试错误前文本已 emit，但 `roundResult` 不可提交。

**验证：** 运行 `go test ./internal/agent -run 'Collector.*Tool|Collector.*Error'`，期望合法调用完整收集、非法流失败。

## T12：构建工具执行批次

**文件：** `internal/agent/scheduler.go`、`internal/agent/scheduler_test.go`

**依赖：** T4、T8

**步骤：**

1. 为调用保留原始索引并构造最大连续只读批次。
2. 让每个副作用、未分类或未知工具形成单独批次。
3. 测试“读、读、写、读、命令”生成四个设计批次。

**验证：** 运行 `go test ./internal/agent -run SchedulerBatch`，期望批次数、并发标志和原始索引正确。

## T13：执行批次并保持结果顺序

**文件：** `internal/agent/scheduler.go`、`internal/agent/scheduler_test.go`

**依赖：** T6、T12

**步骤：**

1. 只读批次并发调用范围化 Executor，副作用批次同步调用。
2. 每批开始前按原序 emit 工具调用；并发完成后按原序 emit 工具结果。
3. 使用受控阻塞测试工具证明只读调用重叠、副作用不重叠且跨批次有屏障。
4. 验证已知工具失败结果仍正常返回，不转换为调度错误。

**验证：** 运行 `go test ./internal/agent -run 'Scheduler.*Order|Scheduler.*Concurrent|Scheduler.*Failure'`，期望时序与结果顺序通过。

## T14：传播 Scheduler 取消

**文件：** `internal/agent/scheduler.go`、`internal/agent/scheduler_test.go`

**依赖：** T13

**步骤：**

1. 在启动每个批次和调用前检查任务 context。
2. 并发批次等待时响应取消，并等待已启动执行路径退出。
3. 验证取消后不启动后续副作用批次，也不发出伪造成功结果。

**验证：** 运行 `go test ./internal/agent -run SchedulerCancel`，期望测试在短超时内结束且后续工具调用次数为零。

## T15：实现 Runner 启动与单任务生命周期

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T7、T8、T9

**步骤：**

1. 实现 NewRunner、Start、请求校验和默认 Options。
2. 创建任务级 context、事件通道、Cancel，并用互斥状态拒绝并发 Start。
3. 确保任务退出时清理忙碌状态、发出唯一终态并关闭事件通道。
4. 测试空请求、非法模式、重复 Start 和任务结束后可再次 Start。

**验证：** 运行 `go test ./internal/agent -run RunnerLifecycle`，期望生命周期与忙碌保护通过。

## T16：实现最终回答路径

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T10、T11、T15

**步骤：**

1. 每轮从 Session 快照构造 ChatRequest，发出 calling 和 streaming 进度。
2. 调用 Collector，并累计迭代次数及 Usage。
3. 无工具调用时原子提交 user 与 assistant，发出 `EventCompleted`。
4. 测试首轮最终回答只请求一次模型、提交两条消息且 Summary 正确。

**验证：** 运行 `go test ./internal/agent -run RunnerFinalAnswer`，期望正常完成且终态后通道关闭。

## T17：实现工具循环与历史回灌

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T13、T16

**步骤：**

1. 有工具调用时交给 Scheduler，并在全部完成后调用 Session.CommitRound。
2. 下一轮只使用更新后的 Session 快照，不重复首轮 user 消息。
3. 让结构化工具失败照常回灌并继续循环。
4. 测试“读工具 → 写工具 → 最终回答”自动完成，检查三次模型请求的上下文。

**验证：** 运行 `go test ./internal/agent -run 'RunnerToolLoop|RunnerToolFailure'`，期望多轮无需外部推进且历史成组提交。

## T18：实现迭代与未知工具停止条件

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T17

**步骤：**

1. 在 Provider 请求前限制迭代次数，默认最多 20，配置值覆盖默认值。
2. 按原始调用顺序更新连续未知工具计数，并锁存本轮是否达到阈值。
3. 达到阈值或第 N 轮仍有工具时，处理并提交完整轮次后发出 `EventStopped`。
4. 测试已知工具重置计数、第 N 轮最终文本正常完成、永不出现第 N+1 次请求。

**验证：** 运行 `go test ./internal/agent -run 'RunnerIterationLimit|RunnerUnknownTool'`，期望停止原因、请求次数和历史完整性正确。

## T19：实现取消与流失败终态

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T14、T18

**步骤：**

1. 区分 context 取消与 Collector 流错误，映射为 Cancelled 或 Failed。
2. 两种路径都保留已 emit 的部分事件，但不提交当前轮次。
3. 确保取消时不启动下一工具批次或下一模型请求。
4. 测试模型生成期、只读批次和副作用工具期取消，以及完成事件后的 done error。

**验证：** 运行 `go test ./internal/agent -run 'RunnerCancel|RunnerStreamError'`，期望唯一终态、Partial 标记和空半轮历史正确。

## T20：完成 Plan/Do 两阶段循环

**文件：** `internal/agent/history.go`、`internal/agent/history_test.go`、`internal/agent/mode.go`、`internal/agent/runner.go`、`internal/agent/mode_test.go`、`internal/agent/runner_test.go`

**依赖：** T18、T19

**步骤：**

1. 实现 `taskHistory`，以 Session 模型历史快照为基线，通过 `BuildRound` 在单次 Plan 任务内追加完整多轮上下文。
2. Plan Mode 同时使用只读 Definitions、只读 Scheduler Registry、规划指令和 `taskHistory`；所有规划轮次禁止调用 Session `CommitRound`。
3. 仅在 `StopFinalAnswer` 时拼接最终文本并调用 `CommitPlan`，连续成功规划按完成顺序累积；其他终态不改变 Session。
4. Do Mode 携带全部待执行计划快照并恢复完整 Registry；计划冲突原样交给模型判断，Provider 请求不得包含规划内部历史。
5. 仅在 Do Mode 以 `StopFinalAnswer` 正常完成后调用 ConsumePlans；取消、流失败、迭代上限或未知工具停止均保留原列表。
6. 测试 Plan 多轮临时历史连续、共享模型历史不变、最终计划可展示、`/do` 无只读提示、完整工具恢复及原有计划生命周期。
7. 使用脚本化 Provider 回归“规划创建 `hello.txt` → 规划把 world 改为 changan → `/do` 调用写入与编辑工具”，断言最终文件为 `hello changan`。

**验证：** 运行 `go test ./internal/agent -run 'Plan|Do|TaskHistory'`，期望临时历史、共享隔离、计划追加、完整提交、成功消费和失败保留全部通过。

## T21：增加 Agent 配置

**文件：** `internal/config/config.go`、`internal/config/validate.go`、`internal/config/config_test.go`、`config.example.yaml`

**依赖：** 无

**步骤：**

1. 增加 `AgentConfig.MaxIterations` 和默认值 20。
2. 配置显式为负数时返回清晰校验错误，零值应用默认值。
3. 保持现有模型、MaxTokens 和 Thinking 默认行为。
4. 在示例 YAML 中增加 Agent 配置段。

**验证：** 运行 `go test ./internal/config`，期望缺省值为 20、正数保留、负数失败且旧配置测试通过。

## T22：迁移 TUI 模型与事件等待

**文件：** `internal/tui/model.go`、`internal/tui/commands.go`、`internal/tui/run.go`、`internal/tui/tui_test.go`

**依赖：** T20

**步骤：**

1. 用 Runner、Session 和当前 Task 替换旧 Conversation 与 Provider 流字段，并从 Session `DisplaySnapshot` 读取已完成展示记录。
2. 定义 TUI 当前任务的临时文本、工具状态、Usage、进度和终态记录。
3. 将 wait command 改为读取 `agent.Event`，通道关闭后返回明确消息。
4. 更新构造函数与基础模型测试。

**验证：** 运行 `go test ./internal/tui -run 'Model|Wait'`，期望 TUI 不再直接依赖 Provider 流通道。

## T23：接入普通输入、`/plan` 与 `/do`

**文件：** `internal/tui/update.go`、`internal/tui/tui_test.go`

**依赖：** T22

**步骤：**

1. 普通非空输入构造 `ModeAct` 请求。
2. 解析 `/plan <任务>`；缺少任务时显示本地错误且不启动 Runner。
3. 精确 `/do` 构造 `ModeDo`；无有效计划时显示 Runner 验证错误。
4. 启动成功后保存 Task、清空输入并持续等待事件。

**验证：** 运行 `go test ./internal/tui -run 'ActCommand|PlanCommand|DoCommand'`，期望命令映射和错误边界正确。

## T24：消费 Agent 事件并传播取消

**文件：** `internal/tui/update.go`、`internal/tui/tui_test.go`

**依赖：** T23

**步骤：**

1. 按事件类型更新临时文本、工具调用、结果、Usage 和阶段。
2. 终态到达后保留展示记录、清除当前 Task 并恢复输入焦点。
3. 任务运行时 Ctrl+C 调用 Task.Cancel，并继续读取直到终态和通道关闭。
4. 测试取消后不退出程序、终态后可提交下一任务、终态之后普通事件不被接受。

**验证：** 运行 `go test ./internal/tui -run 'AgentEvent|Cancel'`，期望事件消费和取消生命周期通过。

## T25：渲染 Agent 进度与部分输出

**文件：** `internal/tui/view.go`、`internal/tui/styles.go`、`internal/tui/tui_test.go`

**依赖：** T24

**步骤：**

1. 从 Session `DisplaySnapshot` 渲染完整可见记录，从临时状态渲染当前或失败任务；不得使用模型上下文 `Snapshot`。
2. 展示轮次、阶段、工具执行中/成功/失败和累计输入/输出 Token。
3. 对取消或失败文本标记“部分输出”，但不把它伪装为已提交历史。
4. 保持 Thinking 折叠、自动滚动和现有状态文本行为。

**验证：** 运行 `go test ./internal/tui -run 'View|Partial|Usage|DisplayHistory'`，期望 Plan 最终结果持续可见且模型历史隔离不影响界面。

## T26：更新启动组装

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** T21、T25

**步骤：**

1. 创建默认 Registry、Executor 和 Session。
2. 用配置的 MaxIterations 创建 Runner，并将 Runner 与 Session 注入 TUI。
3. 删除启动层对旧 Conversation 编排器的构造。
4. 测试配置值正确传递、默认工具仍完整注册且启动错误继续返回。

**验证：** 运行 `go test ./cmd/mewcode`，期望启动组装测试通过。

## T27：移除旧编排入口并收敛 Executor API

**文件：** `internal/conversation/conversation.go`、`internal/conversation/stream.go`、`internal/conversation/conversation_test.go`、`internal/tools/executor.go`、`internal/agent/scheduler.go`、相关测试和调用方

**依赖：** T26

**步骤：**

1. 删除旧 Conversation、Apply/Complete 流程及其测试文件。
2. 删除 T6 的迁移兼容入口和 Executor 内默认 Registry 状态。
3. 将范围化执行入口正式命名为 `Execute(ctx, registry, call)`，更新 Scheduler 和测试调用方。
4. 搜索并确认不存在旧 `Conversation`、`Apply`、`Complete` 或旧 Executor 调用。

**验证：** 运行 `rg 'NewConversation|\.Apply\(|\.Complete\(' internal cmd` 应无旧编排调用；再运行 `go test ./...`，期望全部包通过。

## T28：更新用户文档与章节索引

**文件：** `README.md`、`docs/README.md`

**依赖：** T27

**步骤：**

1. 在 README 说明普通输入会自动循环及各停止条件。
2. 说明 `/plan <任务>` 采用追加语义，`/do` 执行全部待执行计划，冲突交由模型判断，且仅成功执行后消费计划。
3. 在 `docs/README.md` 增加 `ch04` 的 Spec、Plan、Tasks、Checklist 链接。
4. 说明 Plan 内部只读历史与 `/do` 执行历史隔离，核对命令、默认值、多计划生命周期和最终实现一致。

**验证：** 运行 `rg -n 'Agent Loop|/plan|/do|max_iterations|ch04' README.md docs/README.md config.example.yaml`，期望所有用户入口和章节链接均可找到。

## T29：执行全量质量检查

**文件：** 全部本章涉及文件

**依赖：** T28

**步骤：**

1. 对所有修改过的 Go 文件运行 `gofmt`。
2. 运行完整测试、竞态检测和静态检查。
3. 对照 `spec.md` 的 AC1–AC20 检查是否均有自动化证据或明确的端到端验证入口。
4. 若发现回归，回到对应任务修复并重新执行本任务全部命令。

**验证：** 依次运行 `go test ./...`、`go test -race ./...`、`go vet ./...`，期望全部退出码为 0。

## 执行顺序

```text
T1 ─┬─→ T2
    ├─→ T3
    └─→ T8 ───────────────┐
                           │
T4 ─→ T5 ─→ T6 ──────────┼─→ T13 ─→ T14 ───────────────┐
 │         │              │                             │
 └────────→ T12 ──────────┘                             │
                                                       │
T7 ───────→ T9 ─────────────→ T15                      │
T8 ─→ T10 ─→ T11 ───────────→ T16 ─→ T17 ─→ T18 ─→ T19 ─→ T20
                                                        │
T21（可与 T1–T20 并行）────────────────────────────────┤
                                                        ▼
T22 → T23 → T24 → T25 → T26 → T27 → T28 → T29
```

T2 与 T3 可并行；T4–T6、T7、T8–T11 和 T21 在依赖满足后可并行推进。T27 是迁移收口点，在它之前保留最小兼容层以保证每个任务都能独立编译验证。
