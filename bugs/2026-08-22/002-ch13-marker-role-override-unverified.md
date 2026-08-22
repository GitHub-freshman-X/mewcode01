# 第十三章 `marker-role` 覆盖结果异常（待诊断）

- 状态：自动化修复完成；场景 D/E 真实 Provider/TUI 已通过，其余场景待验证
- 影响范围：第十三章人工场景 A 的项目级 Agent 定义覆盖验证。

## 现象

用户提供的运行记录中，调用 `subagent_type="marker-role"` 后返回 `SUBAGENT-FIXTURE-ALPHA-7e31`，而 `docs/ch13-subagent/manual_scenarios.md` 要求项目级定义生效时最终报告 `PROJECT-AGENT-PROMPT-72bc`。

## 当前进展

已确认场景 A 的预期及相关源码位置；定义发现按内置/插件、用户、项目的顺序合并，项目来源允许覆盖，且定义式子 Agent 将命中的 `SystemPrompt` 设置为子 Runner 的系统提示。已有 `TestDiscoverPrecedence` 覆盖来源优先级；`internal/agent.TestDefinitionSubAgentRunsToCompletion` 还断言定义正文作为子 Runner 的稳定系统提示送入 Provider。但尚未覆盖“从目录发现项目定义后，正文实际送入子 Runner”的单个端到端链路。

已运行：`go test ./internal/subagent ./internal/agent -run 'TestDiscoverPrecedence|TestDefinitionSubAgentRunsToCompletion' -count=1`，两个包均通过。它们分别证明项目级覆盖和命中定义的正文注入；但它们是两个独立测试，尚未构成该人工场景的完整确定性复现。

2026-08-22 现场检查确认：fixture 根目录存在项目级 `.mewcode/agents/marker-role.md`，隔离 `HOME` 下也存在用户级同名定义；启动路径和用户提供的 `test_home` 相符。两个定义的正文也与场景文档一致。会话记录显示主 Agent 实际把子任务改写为“读取文件中包含的 coverage test marker，并且只返回该 marker”，子任务最终确实返回了文件标记。该任务措辞与角色正文的“无论任务如何表述，必须包含项目级标记”发生了直接语义竞争。

配置结构包含预期的 `agent.max_iterations`、`enable_verification_agent` 和权限字段；未输出配置值以避免泄露密钥。工作树存在未提交修改，且当前二进制建于源码树外；仅凭 build ID 不能证明该二进制与当前工作树一致。

2026-08-22 后续现场检查确认：项目级 `marker-role.md` 已删除，fixture 目录当前仅保留 `slow-probe.md`；隔离用户目录中仍存在用户级 `marker-role.md`。最新会话中，`agent` 调用以 `subagent_type="marker-role"` 成功完成，而内置定义中不存在该名称、项目定义也已不存在，所以用户级角色已被发现并选中；若未发现应返回 unknown subagent type，而不会完成。

会话仍显示主 Agent 将子任务明确改写为“返回文件中的唯一标记且不含其他文本”，子 Agent 返回文件标记。故此现象不是用户级定义没有被读取，而是模型在互相冲突的软提示中遵循了具体子任务，忽略了用户角色正文的“无论任务如何表述”要求。此前项目级定义存在时的会话也曾返回项目标记，进一步表明结果由模型指令服从决定，不能作为确定性的定义发现/覆盖验收。

## 修复进展

已更新 `docs/ch13-subagent/manual_scenarios.md`：场景 A 仍要求子 Agent 读取 fixture 以验证只读工具可用，但明确要求最终只报告角色定义指定的覆盖标记，不再要求报告文件标记。通过条件同时排除 fixture 标记。这消除了角色正文与委派任务之间的直接冲突。

2026-08-22 真实 Provider 复测修订后的场景仍返回 `MARKER_READONLY_OK`，而不是用户级定义中的 `USER-AGENT-PROMPT-19e4`。现场确认项目级定义仍不存在、用户级定义仍存在，且新会话文件已创建。

最新会话表明根因更具体：主 Agent 在传给已选中 `marker-role` 的 `prompt` 中自行编造了“覆盖测试标记必须是：`MARKER_READONLY_OK`”。子 Agent 返回该值。主 Agent 无法从 `agent` 工具 schema 得知角色正文中的实际标记，却擅自把“角色定义指定的标记”具体化，造成与子 Agent 系统提示冲突。

已再次修订人工场景：仅让子 Agent 读取 fixture，且明确禁止主 Agent 在委派 `prompt` 中猜测、指定、转述或改写标记；新增预检，要求 TUI 中可见的 `agent.prompt` 不含任何具体标记，否则该轮无效并以新会话重试。此前文档修订已被该版本替代。该措施只能提高人工场景的有效性，不能把模型指令服从变为程序级保证。

2026-08-22 再次复测满足预检（主 Agent 的可见委派文本没有具体覆盖标记）后，子 Agent 仍返回 fixture 标记；主 Agent 也明确表示看不到角色定义正文，这一事实本身符合定义式隔离。

现场配置使用 `protocol=openai`、`model=gpt-5.4-mini`。源码确认定义式子 Runner 将 `definition.SystemPrompt` 赋给 `StableSystem`；OpenAI 请求构造将非空 `StableSystem` 作为首条 `input` 的 `role: system` 发送。因此角色提示在本地构造层没有丢失。剩余待验证风险是上游 OpenAI-compatible 服务对 `system` role 的优先级/兼容性，以及模型本身未遵循系统提示。尝试用 Python 读取配置的安全字段失败（环境未安装 PyYAML），未读取或输出任何密钥。

用户报告连续两次新启动中第一次返回用户级标记、第二次失败。尝试用 GNU `find -printf` 排序 macOS 会话文件未产生结果（macOS `find` 不支持该选项）；未修改 fixture。改用 macOS `stat` 后已定位最新两个会话：`20260822-155418-16f8.jsonl`（23:54:29）与 `20260822-155446-eb93.jsonl`（23:54:55）。

对比结论：两次均使用 `subagent_type="marker-role"`，且委派文本都未自行指定覆盖标记。第一次子 Agent 仅返回 `USER-AGENT-PROMPT-19e4`，符合预期。第二次子 Agent 实际返回了 `SUBAGENT-FIXTURE-ALPHA-7e31` 和 `USER-AGENT-PROMPT-19e4`；这证明用户级角色提示已生效。随后主 Agent 违反“只转述唯一标记”的要求，从混合结果中挑选了 fixture 标记，导致表面失败。当前角色定义只要求最终报告“包含”角色标记，未要求排他输出，是混合结果得以通过的文档缺口。

## 修复进展

已更新 `docs/ch13-subagent/manual_scenarios.md`：用户级和项目级 `marker-role` 模板均要求完成只读检查后最终回复必须且只能为各自覆盖标记，明确禁止 fixture 内容、摘要和其他文本；场景 A 的两次验收均改为检查可见 `agent` 工具结果的 `data.result` 是否恰好等于预期值。当前代码已正确发现、覆盖和注入定义，无需为该文档验收缺口修改运行时代码。修改后的 fixture 定义尚未以真实 Provider 复测，状态为待验证。

尚未取得实际 fixture、启动参数、Agent 调用日志或可运行的确定性复现，因此不能将该记录判定为实现 bug。后续将先构建不访问真实 Provider 的自动化复现，区分定义未发现、覆盖顺序错误、角色提示未注入和用户提供记录并非实际程序输出等情况。

## 本次排查（待处理）

已重新核对第 13 章 Spec、人工场景 B、实现计划和验收清单。场景 B 明确要求：`enable_verification_agent: true` 后**退出并重新启动**，随后调用必须成功启动 `Verification`。用户给出的“不可用”文本是开关关闭时的预期，不是开启并重启后的正确输出。

源码静态检查已确认开关链路完整：YAML `agent.enable_verification_agent` 被 `internal/config/load.go` 读取到 `cfg.Agent.EnableVerificationAgent`，`cmd/mewcode/main.go` 将其传给 `subagent.Discover`，而 `Discover` 调用 `Builtins(true)` 时会加入嵌入的 `verification.md`。因此优先怀疑实际启动进程仍使用旧配置、编辑的不是 `--config` 指向的文件、未在修改后完整重启，或启动的是旧二进制。

已运行确定性验证：`go test ./internal/config ./internal/subagent -run 'Test(EnableVerificationAgentConfig|BuiltinsVerificationSwitch)$' -count=1 -v`，通过。它验证了 YAML 的显式 `true` 被正确加载，以及 `Builtins(true)` 确实包含 `Verification`。

2026-08-23 现场复核 `/private/tmp/mewcode-ch13-manual.q2LEr1/config.yaml`：字段位于正确的 `agent` 层级（第 20 行），值为 `true`；运行进程 PID 18562 于 00:01:07 启动，配置修改时间为 00:01:03，故该进程是在修改后启动，且命令确实为 `mewcode --config config.yaml`。配置本身没有问题。

关键证据在最新会话 `20260822-160107-a2f2.jsonl`：`agent` 工具返回 `success: true`、`status: "completed"` 和 `task_id: "subagent-1"`，这只能说明 `Verification` 已被解析、创建并运行至完成；若未加载，工具层会像紧邻的旧会话 `20260822-160016-d636.jsonl` 一样返回 `success: false` 及 `unknown subagent type "Verification"`。最新会话中“Unavailable — this environment does not provide …”是**已启动的 Verification 子 Agent 的模型文本**，不是 MewCode 的类型解析错误。用户要求该子 Agent“不调用工具、只报告是否可用”，导致模型自行判断并错误声称不可用；该文本不可信，但本场景的角色加载/创建条件已通过。

结论：配置和启动命令无误；当前应将人工记录标为“已加载，子 Agent 报告内容错误（模型行为）”，不能把该输出解释为开关未生效。

## 文档修复（2026-08-23）

已修改 `docs/ch13-subagent/manual_scenarios.md` 的场景 B：委派任务不再要求子 Agent 判断 `Verification` 是否可用，而是只要求其不调用其他工具并给出简短完成报告；开启后的通过条件改为检查 `agent` 工具结构化返回中的 `success: true`、完成状态和任务 ID。结果记录模板也改为记录“未知类型错误 / 结构化成功”，避免把模型生成的自我可用性陈述误当作运行时诊断。该修订消除了人工验收提示词的语义错误；无需修改运行时代码。

## 场景 D ESC 提示的显示顺序异常（待处理）

用户提供的场景 D 转录表明：按 `Esc` 后，`agent` 工具以 `async_launched` 和 `subagent-1` 返回，主会话随后成功处理“你好”和 `hello`；这符合 ESC 将前台任务接管到后台且不阻塞输入的条件。由于转录未显示 `manual-esc-background` 的 completed/failed/cancelled 终态通知，场景 D 尚未整体通过。

但即时提示“子 Agent 已转入后台。”在其后两轮正常对话之后仍显示在视图最底部，而非按发生时序位于触发 ESC 的原始回合与下一轮用户消息之间。这不符合第 13 章 AC9 的状态变化可观察性，也违反现有 TUI 对系统提示应位于会话轮次间的显示约定。

静态检查定位到 `internal/tui/model.go:AddSystemMessage`：只要 `m.hasTransientTaskView()` 为真（ESC 时主任务仍在收尾），就将提示锚定为 `after: -1`。`internal/tui/view.go:renderCurrentContent` 只在所有已持久化消息渲染完后才渲染 `after: -1`，且该锚点不会在主任务结束后重新定位；因此后续正常轮次会永远排在该提示之前。现有 `TestSystemMessageStaysBetweenConversationTurns` 仅覆盖没有 transient task 的普通系统消息，未覆盖 ESC 接管时的锚定与主任务完成后的重排。

**终态通知缺失已确认根因**：`NewModelWithPermissions` 虽调用 `runner.SubscribeSubAgentTasks()` 并保存频道，但 `Model.Init()` 只返回文本输入的 `Focus()` 命令，未同时启动 `waitForSubAgentNotification(m.subAgentNotifications)`。而 TUI 只有收到 `subAgentNotificationMsg` 后才会再次注册下一次等待；由于第一次等待从未启动，`TaskManager.finish()` 发布的 completed/failed/cancelled 通知一直停留在订阅频道中，永远不会进入 `Model.Update`，故不会渲染终态系统提示。`/status` 显示后台任务数为 0 只说明任务已不再处于 running 状态，不能说明 TUI 已消费并展示了它的终态。

用户已确认修复范围和规格：修复 ESC 提示时序、后台终态展示、首次及连续通知监听；不改变任务状态机、模型会话历史或安全摘要边界。已将 F12–F14、N9–N11、AC12–AC15 回写到第 13 章 `spec.md`。

技术设计亦已获批准并回写 `plan.md`：`Init()` 并行启动首次通知等待；每条通知后续订阅保持；ESC 提示使用仅在当前主回合成功提交后解析的临时锚点，失败/取消时清除该锚点并保持既有末尾临时显示。

任务拆解已获批准并回写 `task.md` 的 T20–T23：依序建立监听和临时锚点、在终态解析锚点、补充 TUI 回归测试、同步验收文档与问题记录。

验收清单已获批准并回写 `checklist.md`，新增 AC12–AC15 的时序、首次/连续终态通知、兼容安全和真实场景 D 条目。四份文档均已批准，可按 T20–T23 开始实现。

实现进展：已修改 `internal/tui/model.go`，`Init()` 现在以 `tea.Batch` 同时启动输入焦点、已有权限监听和首次子任务通知等待；系统消息新增仅用于当前主任务的延迟定位状态。测试与终态处理尚未完成，因此状态仍为待处理。

实现进展：已修改 `internal/tui/update.go`；主任务 completed/stopped 后将待定位提示锚定到已提交显示历史末尾，failed/cancelled 后清除待定位状态并保留末尾临时显示。尚未添加或运行回归测试，因此状态仍为待处理。

实现进展：已在 `internal/tui/tui_test.go` 添加 ESC 排序、首次终态通知和连续通知测试；已运行 `gofmt -w internal/tui/model.go internal/tui/update.go internal/tui/tui_test.go && go test ./internal/tui -run 'Test(EscapeSystemMessageStaysBeforeLaterConversationTurn|InitConsumesFirstSubAgentTerminalNotification|SubAgentTerminalNotificationsContinueInOrder)$' -count=1`，通过。该结果证明 ESC 提示在成功主回合后重定位、初始化消费首个终态、连续通知顺序消费和输入可用。

随后已运行 `go test ./internal/tui -count=1` 与 `go test ./internal/tui ./internal/agent -run 'Test.*(Escape|SubAgent|TaskNotification)' -count=1`，均通过；TUI 全量和 ESC/通知兼容路径无回归。已同步更新场景 D，明确 ESC 提示必须位于其原回合后、下一条普通请求前；checklist 已记录上述自动化证据。已执行 `go test ./... -count=1`：本次 TUI 改动相关的 `internal/tui`、`internal/agent` 与其他非 OpenAI 包均通过；但全量回归仍失败于 `internal/provider/openai.TestStreamCapturesFinalRequestPayload`。现场输出和源码均表明日志当前实际包含 `message: body.Input` 请求正文，属于既有 Provider 日志泄露问题的回归，已同步更新 `bugs/2026-08-14/003-provider-request-body-logged.md`，不归因于本次 TUI 改动。该命令还再次产生 `cmd/mewcode/.mewcode/` 测试产物，已同步既有记录。

已删除该测试产物并复核工作树不再包含它。已运行 `go build ./cmd/mewcode && git diff --check`，均通过。

结论：ESC 提示时序和 TUI 后台终态通知的自动化修复完成；全量回归的唯一失败是已单独记录的 Provider 请求正文日志回归。真实 Provider/TUI 场景 D 尚未复测，故本记录状态为“自动化修复完成；真实 Provider/TUI 待验证”。最后已运行 `git diff --check`，无输出；工作树还包含本章此前已存在的 Fork/文档改动，以及本次 TUI、问题记录和文档改动。

## 场景 D 复测：前台超时与 ESC 后取消（待诊断）

用户最新转录显示两类失败：（1）两次 `sleep 25` 的前台定义式调用在 30 秒后由 `agent` 工具返回 `type: timeout`、`timeout_ms: 30000`，随后任务 failed；（2）改为两次 `sleep 10` 并按 ESC 后，工具正确返回 `async_launched`，即时 ESC 提示也按时显示，但后台任务随后进入 cancelled。

初步现场源码证据：`internal/tools/executor.go` 对所有工具调用以默认 30 秒创建 deadline；`agent` 工具的前台等待包含整个子 Agent 生命周期，故 50 秒任务必然超过这一外层 deadline，违背 F9 的 120 秒前台自动后台设计。`SubAgentRuntime.dispatch` 在前台启动任务时把该可取消 context 直接传给 `TaskManager.Launch`；ESC 仅调用 `MarkBackground`，不会替换已创建任务的 context。因此即使 ESC 已异步返回，子 Agent 仍会在原外层 deadline或父调用取消时收到取消，解释了第二次的 cancelled 终态。

已检查最新会话文件的安全结构（仅记录条目序号、角色和字段形状，未输出 prompt 或工具正文）；其包含两次 `agent` 工具调用及对应结果，和用户转录一致。该检查未提供可安全引用的更细粒度取消时间线。

用户授权直接完成本次小范围的 mew-spec 文档更新。已在第 13 章既有文档加入 F15、N12、AC16，以及 T24–T25：仅 Agent 前台生命周期跳过通用 30 秒期限；前台以可接管 lifetime context 转发父取消，接管后停止转发；普通工具期限不变。

实现进展：`internal/tools/executor.go` 已仅对非 `agent` 工具施加 Executor 的通用 deadline；`internal/agent/subagent.go` 已以剥离父 deadline 的 lifetime context 启动前台子任务，并只在前台父 context 取消分支显式取消它。ESC/自动接管返回时不再取消该 lifetime context。

已运行 `gofmt -w internal/tools/executor.go internal/tools/tools_test.go internal/agent/subagent.go internal/agent/subagent_test.go && go test ./internal/tools ./internal/agent -run 'Test(ExecutorDoesNotApplyGeneralTimeoutToAgent|ExecutorUnknownAndTimeout|ForegroundSubAgentContextDetachesParentDeadline)$' -count=1`，通过：Agent 调用不继承通用 Executor deadline，普通工具期限仍有效，前台 lifetime context 不继承父 deadline 且可显式取消。随后 `go test ./internal/tools ./internal/agent ./internal/tui -count=1 && go build ./cmd/mewcode && git diff --check` 通过。已同步人工场景 D/E 与 checklist。

再次执行 `go test ./... -count=1`：本次 Agent/TUI/Tools 相关包均通过；全量仍仅失败于已单独记录的 OpenAI 请求正文日志回归 `TestStreamCapturesFinalRequestPayload`，未发现本次改动引入的额外失败。该测试产生的既有会话目录产物已删除并复核不再出现在工作树。

2026-08-23 用户以真实 Provider/TUI 复测场景 D：两次 `sleep 10` 的前台定义式任务在按 ESC 后返回 `async_launched` 和 `subagent-1`；即时提示“子 Agent 已转入后台。”位于原回合之后、下一轮“你好”之前；主会话正常回复；随后 TUI 显示 `manual-esc-background completed: 完成`。这同时确认通知监听、显示时序以及接管后任务不再受原前台 context 取消影响。场景 D 已通过。

2026-08-23 用户以真实 Provider/TUI 复测场景 E：全程未按 ESC 的五次 `sleep 25` 前台定义式任务返回 `async_launched` 和 `subagent-2`，随后显示 `manual-auto-background completed: 完成`。这符合约 120 秒自动接管、同一任务继续运行及终态通知的预期，且证明通用 30 秒工具期限未截断任务。转录未展示内部 `run_command` 次数或精确接管时刻，故最多五次且不重复启动仍以自动化测试为决定性证据。

## 场景 C 追加排查（进行中）

用户提供的 Fork 后台调用返回 `unknown subagent type "fork"`。根据场景 C 的定义，Fork 必须**省略** `subagent_type`；该错误字面上表示实际工具参数包含了 `subagent_type: "fork"`，而不是运行时将省略类型的调用错误地分流为 Fork。

用户已确认修复取舍：将 `subagent_type: "fork"` 作为 Fork 的兼容别名，使模型即使错误填入该值仍进入 Fork 强制后台路径；`fork` 成为保留值，不能再作为定义式角色名。已在既有第 13 章 `spec.md` 增加 F1a、N8 与 AC1a，且该规格已获批准。

已在既有 `plan.md` 设计兼容路径：运行时将省略类型和 `fork` 归一为 Fork，二者复用后台、历史与递归限制逻辑；定义解析拒绝忽略大小写和首尾空白后等于 `fork` 的名称；工具 schema 同步说明两种调用方式。该计划已获批准。

已核对现有实现与测试落点：兼容分流位于 `internal/agent/subagent.go`，保留名校验位于 `internal/subagent/definition.go`，schema 断言在 `internal/tools/agent_test.go`，Fork 端到端模拟在 `internal/agent/subagent_test.go`。

已在既有 `task.md` 增加 T17–T19：先拒绝含变体的保留角色名，再实现并测试 `fork` 兼容分流及 schema 说明，最后同步场景 C、验收清单和全量验证。该任务拆解已获批准。

已在既有 `checklist.md` 增加 AC1a/N8 的可观察验收：字段缺失、空白和 `fork` 均须进入强制后台 Fork，保留名及其变体必须被拒绝；端到端条目同时覆盖两种 Fork 写法。四份文档均已获批准，已开始实现。

T17 已完成并验证：`internal/subagent/definition.go` 拒绝去除首尾空白且大小写无关等于 `fork` 的定义名称；`internal/subagent/subagent_test.go` 覆盖 `fork`、` Fork `、`FORK` 被拒绝及 `forker` 可用。目标测试通过。

T18 已完成代码与目标验证：`internal/agent/subagent.go` 已将 Fork 判定提取为统一函数，空白或大小写无关等于 `fork` 的类型均走 Fork 路径；`internal/tools/agent.go` 的稳定 schema 已说明省略字段或传入 `fork` 均为 Fork。`internal/agent/subagent_test.go` 的 Fork 端到端模拟传入 `subagent_type:"fork"`，并覆盖省略、空白、大小写变体与普通定义式类型的判定；`internal/tools/agent_test.go` 断言 schema 同时说明省略和 `fork`。已运行 `go test ./internal/subagent ./internal/agent ./internal/tools -run 'Test.*(Fork|AgentTool|DefinitionSubAgent)' -count=1 -v`，全部通过。

T19 进行中：已更新 README 的 SubAgent 说明，公开记录 `fork` 兼容写法与保留名称；已更新场景 C，允许省略字段或传入 `fork`（含大小写和首尾空白变体）作为有效 Fork 调用。

已运行格式化、`go test ./... -count=1` 和构建链。除 `internal/provider/openai.TestStreamCapturesFinalRequestPayload` 外，所有包测试通过；该既有失败仍错误期待日志包含请求正文 `request-canary`，实际输出只有安全元数据，已同步既有 bug `2026-08-14/003-provider-request-body-logged.md`。随后已运行 `go test ./internal/subagent ./internal/agent ./internal/tools -run 'Test.*(Fork|Reserved|AgentTool|DefinitionSubAgent)' -count=1 && go build ./cmd/mewcode && git diff --check`，全部通过。已将这些结果和未执行的真实 Provider 场景 C 写入第 13 章 checklist；真实 Provider 人工复测尚未完成。

用户随后提供场景 C 复测结果：定义式后台任务返回 `subagent-1`，后续 Fork 请求返回不同的 `subagent-2`，两者均为 `success: true`、`status: async_launched`，主 Agent 均未等待子任务完成。这通过了 Fork 强制后台和不阻塞的核心人工条件。该转录未显示实际 `agent` 参数，故它不能单独证明本次走的是省略字段的旧路径还是 `subagent_type:"fork"` 兼容路径；若要验收兼容修复，仍需保留可见调用参数或会话记录。终态通知及主会话后续通知消费也尚未在该转录中验证。

2026-08-23 已从 fixture 会话 `20260822-160830-78d3.jsonl` 提取实际 `agent` 调用的安全结构字段：调用带有 `name: "manual-fork-background"`、`run_in_background: true`，但同时错误传入 `subagent_type: "fork"`。运行时 `dispatch` 只在该字段为空白时选择 Fork；非空值走定义式查找，因注册表不存在名为 `fork` 的角色，故经工具层包装为该错误。这与 `internal/agent/subagent.go` 和 `internal/tools/agent.go` 的实现、以及稳定 schema 中“省略该字段即 Fork”的约定一致。配置的相关字段也正确：`agent.enable_verification_agent: false`、`agent.max_iterations: 20`、`permissions.mode: relaxed`；fixture 项目定义中存在 `marker-role` 和 `slow-probe`。未发现配置或运行时分流缺陷。

结论：这是主模型未遵守“不要提供 `subagent_type`”指令导致的无效人工轮次，不是 Fork 强制后台功能失败。应以新会话重试，且在权限确认前检查可见调用参数中 `subagent_type` 缺失（而非值为 `fork`）；只有该前置条件满足后，才可依据 `async_launched` 和任务 ID 验收 Fork。

已同步更新 `docs/ch13-subagent/manual_scenarios.md` 场景 C，增加该调用参数预检和无效轮次的记录方式，避免将模型构造参数错误误诊为运行时失败。

已运行 `go test ./internal/agent ./internal/tools -run 'Test(ForkSubAgentPreservesHistoryAndRunsBackground|DefinitionSubAgentRunsToCompletion|AgentTool)' -count=1 -v`，全部通过；其中 Fork 测试实际省略 `subagent_type` 并验证其保留历史且后台运行，工具测试验证稳定 schema 与分发路径。`git diff --check` 无输出。

用户随后提供的完整 TUI 记录再次确认：虽然用户指令要求省略字段，实际 `agent` 工具结果明确为 `unknown subagent type "fork"`。该错误只能由实际请求携带 `subagent_type: "fork"` 触发；MewCode 在工具失败后的文字“该环境不支持 Fork”是对错误的错误归因，不是运行时事实。

2026-08-23 已直接读取该场景会话 `20260822-160830-78d3.jsonl` 的第 12 条记录（仅提取路由字段，未输出 prompt 或敏感信息）：`tool_name=agent`、`name=manual-fork-background`、`subagent_type_present=true`、`subagent_type="fork"`、`run_in_background=true`。源码分支是 `strings.TrimSpace(input.SubagentType) == ""` 才为 Fork；因此该次请求不可能进入 Fork 路径。另发现 fixture 中还有一份包含同一任务名的会话 `20260822-161416-733b.jsonl`；已核对其第 12 条记录，路由字段与前一份完全相同：`subagent_type_present=true`、`subagent_type="fork"`、`run_in_background=true`。两次均不是省略类型字段的 Fork 请求。未使用真实 Provider 重跑字段确实缺失的有效场景 C，因此该人工验收项仍待一次参数正确的复测。

## 场景 F Worktree 隔离边界复测（通过；人工脚本已修正）

2026-08-23 现场会话 `20260822-172432-4b95.jsonl` 显示主 Agent 实际调用了 `agent`，参数为 `subagent_type="marker-role"` 与 `isolation="worktree"`；工具立即返回 `success: false` 和明确诊断 `worktree isolation is not supported in this chapter`。这完全符合场景 F、Spec 的“不支持且不得静默降级”要求；未创建子任务，更没有自行创建或改用 Worktree。

原始前后目录清单的唯一差异是 fixture `.mewcode/sessions/` 下新增当前主会话 JSONL。它是 MewCode 正常会话持久化副作用，不是 Worktree 或隔离目录；因此原文要求对包含该目录的完整 `find` 输出零差异，会把正确行为误报为失败。已将 `manual_scenarios.md` 的场景 F 快照命令改为排除 `.mewcode/sessions/`，同时保留对其他 fixture 路径变化的检查。

结论：本轮场景 F 的 Worktree 部分通过；发现并修复的是人工验收脚本的假阳性，不是 SubAgent Worktree 隔离实现缺陷。

### 场景 F 递归能力边界（模型未尝试；自动化已覆盖）

同一会话随后显示主 Agent 成功创建了 `marker-role` 子 Agent（`subagent-1`）；主调用使用 `isolation="none"`，这不违反递归场景的输入要求。子 Agent 的自然语言回复明确称“不会尝试”内部 `agent` 调用，并报告未创建第二个子 Agent。因此该人工轮次只能按场景文档记录为“模型未尝试递归调用”，**不能**作为“子 Agent 不具备 `agent` 工具”的直接人工证据；模型拒绝调用与运行时过滤是不同的结论。

本地定义的 `marker-role` 白名单仅含 `read_file`，且 `internal/subagent.FilterTools` 无条件移除 `agent`。已运行 `go test ./internal/subagent ./internal/tools -run 'Test(FilterTools|AgentToolRejectsUnsupportedIsolation)$' -count=1`，两个包通过：前者断言即使定义白名单包含 `agent`，定义式后台子 Agent 最终也只保留 `read_file`；后者断言 `isolation: worktree` 返回 validation error。递归过滤由该自动化测试作为决定性证据。

该轮返回的 `USER-AGENT-PROMPT-1234` 与当前 fixture 项目级 `marker-role.md` 的正文一致，但不同于人工方案准备步骤中的 `PROJECT-AGENT-PROMPT-72bc`。用户已确认前者为其手动修改的预期标记；它不影响场景 F 的递归边界结论。

### 会话与日志全量复核（进行中）

已完成对 fixture 下全部运行数据的只读结构化扫描。21 个会话文件中 1 个是正常的 0 字节启动会话，其余均为有效 JSONL；22 个日志文件、共 148 行也全为有效 JSON。会话没有格式损坏或未知工具调用：共 26 次 `agent` 调用，结果为 13 次前台 completed、8 次 async_launched、3 次 execution_error、1 次 timeout、1 次 validation_error。

3 次 execution_error 均可由当时人工条件解释：关闭 `Verification` 的预期 unknown-type 轮次 1 次，以及实现 Fork 兼容前模型传入 `subagent_type="fork"` 的无效轮次 2 次；之后同一参数走 Fork 的兼容路径已获得 async_launched。唯一 timeout 是 F15 修复前的两次 `sleep 25` 前台任务；其后的 ESC 与 120 秒自动接管复测均为 async_launched 并有 completed 终态。唯一 validation_error 是场景 F 的 `isolation=worktree` 预期拒绝。因此会话记录没有发现新的第 13 章运行时残留问题。

日志复核另外发现两项：Provider 日志仍有完整消息/工具输出字段，已同步既有 `bugs/2026-08-14/003-provider-request-body-logged.md`；所有 22 条 `stage=subagent` 日志只记录 `status=running`，没有对应的 completed、failed 或 cancelled 终态日志，已创建独立问题记录 `bugs/2026-08-23/001-ch13-subagent-terminal-status-not-logged.md`。

随后也扫描了仓库根目录下全部历史测试日志：149 个 JSONL 文件、351 条记录全部可解析，且无会话文件；这些日志不含 `fields.message` 请求/响应正文，也没有 Ch13 `subagent` 阶段记录。它们没有提供新的残留问题证据，也不能推翻 fixture 中真实运行二进制的两项发现。
