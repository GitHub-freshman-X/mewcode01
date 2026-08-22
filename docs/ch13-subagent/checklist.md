# MewCode SubAgent 子任务分发 Checklist

> 每项须通过运行代码、自动测试或可观察的 TUI 行为验证。验收时在条目下补充实际命令、结果和证据；未通过项必须说明实际行为与后续处理。

## 定义、配置与统一工具

- [ ] **固定 Agent 工具（AC1）**：注册表与首轮模型请求仅出现一个名称和 schema 稳定的 `agent` 工具；指定 `subagent_type` 走定义式，省略该字段走 Fork。（验证：`go test ./internal/tools ./internal/agent -run 'Test.*(AgentTool|DefinitionSubAgent|ForkSubAgent)' -count=1`，期望路径选择和 schema 断言通过。）
- [ ] **四来源加载与覆盖（AC2）**：项目、平台用户配置目录、内置、插件注入定义均可加载；同名最终取项目 > 用户 > 内置 > 插件；无效 frontmatter、模型、权限模式、轮次和来源内重复名称被拒绝。（验证：`go test ./internal/subagent -run 'Test.*(Discover|Definition)' -count=1`，期望覆盖和错误诊断断言通过。）
- [ ] **内置角色与 Verification 开关（AC3）**：默认只有 `Explore`、`Plan`、`general-purpose`；`agent.enable_verification_agent: true` 后才出现 `Verification`。（验证：`go test ./internal/config ./internal/subagent ./cmd/mewcode -run 'Test.*(Verification|Builtin)' -count=1`，期望默认关闭、显式开启均通过。）
- [ ] **配置与用户文档（AC3、N6）**：配置示例标明 `agent.enable_verification_agent: false`；README 说明 `<项目根>/.mewcode/agents/`、`os.UserConfigDir()/mewcode/agents/`、frontmatter、优先级、后台和限制；文档索引存在第 13 章入口。（验证：`rg -n 'enable_verification_agent|UserConfigDir|ch13-subagent|SubAgent' .mewcode/config.example.yaml README.md docs/README.md`，期望每项有对应说明。）

## 创建模式、隔离与能力边界

- [ ] **定义式隔离（AC4）**：定义式首轮请求仅包含角色系统提示和任务，不含父历史；调用参数可覆盖定义模型；会话、权限、读缓存和 Token 用量不影响父 Agent。（验证：`go test ./internal/agent -run TestDefinitionSubAgent -count=1`，期望请求、隔离和共享基础设施断言通过。）
- [ ] **Fork 前缀与合法历史（AC4、AC11）**：Fork 保留父系统提示和历史前缀、补齐末尾悬空工具调用结果，再追加 Fork 约束与任务；Provider 返回的缓存 Token 正确归集但没有伪造命中。（验证：`go test ./internal/agent -run TestForkSubAgent -count=1`，期望历史合法、前缀保持和用量断言通过。）
- [ ] **Fork 参数兼容与保留名（AC1a、N8）**：省略 `subagent_type`、空白类型和 `subagent_type: "fork"` 均启动强制后台 Fork，而不是查找定义式角色；定义来源中的 `fork`、大小写或首尾空白变体均被拒绝，其他角色仍可加载。（验证：`go test ./internal/agent ./internal/subagent ./internal/tools -run 'Test.*(Fork|Reserved)' -count=1`，期望兼容调用返回异步任务、历史继承和递归限制不变、保留名有诊断错误。）
- [ ] **多层工具过滤（AC5）**：定义式按全局禁止、角色黑名单、角色白名单过滤；后台再受固定白名单限制；未知白名单工具报错。（验证：`go test ./internal/subagent -run TestFilter -count=1`，期望过滤后的工具集与错误断言通过。）
- [ ] **递归与隔离参数防护（AC5、N1）**：Fork 再 Fork 被运行时来源标记拒绝；后台 Agent 无法创建子 Agent；`isolation: worktree` 返回明确的未支持错误且不创建任务。（验证：`go test ./internal/agent ./internal/tools -run 'Test.*(Fork|Background|Isolation)' -count=1`，期望拒绝断言通过。）

## 执行、后台与通知

- [ ] **跑到底终态（AC6）**：子 Agent 在纯文本、迭代上限、取消和失败时分别形成正确终态，返回最终文本或安全失败摘要，归集 Token 和工具调用计数。（验证：`go test ./internal/agent ./internal/subagent -run 'Test.*(RunToCompletion|TaskManager)' -count=1`，期望四类终态和统计断言通过。）
- [ ] **显式、超时和 Fork 后台（AC7）**：显式后台立即返回任务 ID；前台在 120 秒阈值触发后接管同一运行任务而不重启；Fork 始终后台；任务查询可观察 ID、名称、状态、耗时、用量与结果。（验证：`go test ./internal/agent ./internal/subagent -run 'Test.*(Background|AutoBackground|Fork)' -count=1`，期望三条入口和状态快照断言通过。）
- [ ] **主对话任务通知（AC8）**：后台任务完成、失败和取消均形成 `<task-notification>`；主 Agent 空闲时通知保留到下一请求；通知不改系统提示也不包含 prompt、工具正文或原始错误。（验证：`go test ./internal/agent -run TestTaskNotification -count=1`，期望模型请求与安全内容断言通过。）
- [ ] **失败隔离与安全日志（AC10）**：子任务失败、超时或取消不终止主 Agent；日志仅含模式、状态、计数、耗时、用量、模型和类型等安全元数据。（验证：`go test ./internal/agent ./internal/subagent -run 'Test.*(Failure|Cancellation|SafeLog)' -count=1`，期望主流程继续和日志脱敏断言通过。）

## 子 Agent 终态日志

- [x] **终态一次性记录（AC17）**：completed、failed、cancelled 任务各产生一条且仅一条 `stage=subagent` 终态日志；重复或非法终态不得额外记录。（验证：`go test -race ./internal/subagent ./internal/agent -count=1` 通过；`TestTaskManagerTerminalCallbackReceivesEachTerminalSnapshot` 覆盖三类终态的单次回调。）
- [x] **字段与安全性（AC17、N13）**：终态日志包含状态、模式、后台标记、模型、工具调用数、三类 Token 用量和耗时；不含 prompt、模型消息、工具结果正文、密钥、请求头或原始错误。（验证：`go test ./internal/agent -run 'Test(LogSubAgentTerminalWritesSafeMetadata|DefinitionSubAgentRegistersTerminalLogCallback)$' -count=1` 通过；覆盖字段、实际模型/用量与 result/failure canary 脱敏。）
- [ ] **创建与接管路径覆盖（AC18）**：定义式前台、显式后台、Fork、ESC 接管和自动后台化均记录同一终态语义。（验证：`go test ./internal/agent ./internal/tui -run 'Test.*(SubAgent|Background|Fork|Escape|AutoBackground).*Terminal.*Log' -count=1`，期望每条路径均有一条终态记录。）
- [ ] **非阻塞与兼容（AC19、N14）**：阻塞终态日志回调不阻塞 `Wait`、任务通知、前台返回或后台执行；既有 ESC、自动后台、取消和通知注入测试继续通过。（验证：`go test -race ./internal/subagent ./internal/agent ./internal/tui -run 'Test.*(TaskManager|SubAgent|Escape|Background|TaskNotification)' -count=1`，期望退出码为 0。）
- [ ] **真实 Provider 日志复测**：在隔离 fixture 运行完成、失败或取消的子 Agent 后，日志中同时存在 `running` 与相同任务的安全终态记录；未执行时明确标注。（验证：按 `manual_scenarios.md` 的隔离启动方式运行并仅检查安全元数据。2026-08-23 已复测定义式前台 completed：最新日志含一条 running 与一条 completed，终态含工具调用数、Token 用量和耗时；failed/cancelled 与后台路径仍待复测。）

## TUI 与 Hook 集成

- [ ] **前台进度与 ESC 接管（AC9）**：TUI 显示子 Agent 进度；有可接管任务时按 ESC 转后台并恢复输入；Ctrl+C 仍取消主任务。（验证：`go test ./internal/tui -run 'Test.*(SubAgent|Background|Escape)' -count=1`，期望状态迁移、输入恢复和取消回归通过。）
- [ ] **后台终态展示（AC8、AC9）**：后台完成、失败或取消后，TUI 显示含任务名称、状态和安全摘要的系统通知，主会话仍可提交后续输入。（验证：`go test ./internal/tui -run Test.*SubAgent -count=1`，期望渲染和可输入断言通过。）
- [ ] **ESC 提示时序（AC12）**：前台子 Agent 运行时按 ESC，完成该主回合后再完成一轮对话；“子 Agent 已转入后台。”位于原回合之后、后续用户消息之前。（验证：`go test ./internal/tui -run 'Test.*Escape.*' -count=1`，期望排序断言通过。）
- [ ] **首次终态通知（AC13）**：启动 TUI 后发布一个后台任务 `completed`、`failed` 或 `cancelled` 通知；视图显示任务名、终态和安全摘要。（验证：`go test ./internal/tui -run 'Test.*SubAgent.*Notification.*' -count=1`，期望首次通知被消费。）
- [ ] **连续通知（AC14）**：连续发布多个后台任务终态；每条只显示一次、按接收顺序显示，输入仍可用。（验证：`go test ./internal/tui -run 'Test.*SubAgent.*Notification.*' -count=1`，期望全部顺序断言通过。）
- [ ] **通知兼容性与安全（AC15）**：ESC 接管、`Ctrl+C`、主 Agent 的下一请求任务通知注入保持通过；终态展示不含 prompt、工具结果正文或原始错误。（验证：`go test ./internal/tui ./internal/agent -run 'Test.*(Escape|SubAgent|TaskNotification)' -count=1`，期望退出码为 0。）
- [ ] **前台期限与接管 context（AC16）**：超过通用 30 秒期限但未到 120 秒阈值的前台 Agent 调用不被截断；ESC/自动接管后父调用取消不终止同一子任务；未接管前台任务仍随父取消结束；非 Agent 工具期限不变。（验证：`go test ./internal/tools ./internal/agent -run 'Test.*(Agent|Background|AutoBackground|Timeout|Cancellation)' -count=1`。）
- [ ] **Hook agent 动作（AC10）**：Hook 的 `agent` 动作调用同一 SubAgentRuntime，不再返回“未接入”；其失败不阻断主 Agent。（验证：`go test ./internal/hooks ./cmd/mewcode -run 'Test.*(Agent|SubAgent)' -count=1`，期望运行时桥接和故障隔离断言通过。）

## 兼容性、构建与全量回归

- [ ] **未配置兼容性（AC11）**：没有自定义定义且 Verification 未启用时，既有主 Agent、Skill、权限、Hook、工具和会话流程保持通过。（验证：`go test ./internal/agent ./internal/tools ./internal/hooks ./internal/skills ./internal/permissions -count=1`，期望全部通过。）
- [ ] **并发安全（N2、N4）**：任务查询、订阅、通知和前后台接管不出现数据竞争。（验证：`go test -race ./internal/subagent ./internal/agent ./internal/tui -count=1`，期望退出码为 0 且无 race 报告。）
- [ ] **格式化、全量测试与构建（N7）**：变更文件已格式化，所有单元测试及可执行程序构建通过。（验证：`gofmt -w $(git diff --name-only -- '*.go') && go test ./... -count=1 && go build ./cmd/mewcode`，期望命令全部退出 0。）

## 端到端人工场景

- [ ] **定义覆盖场景**：在临时项目 `.mewcode/agents/` 和隔离用户配置目录各放置同名角色；启动应用并要求主 Agent 委派该角色。（验证：项目级角色提示/行为生效，用户级和内置版本未生效；删除项目定义后用户级版本生效。）
- [ ] **前台转后台场景**：请求一个会持续调用工具的定义式子 Agent，在 TUI 看到前台进度后按 ESC。（验证：任务显示为后台运行，输入框立即可用；输入新请求能被主 Agent 接收；原任务最终显示完成、失败或取消通知。）
- [ ] **显式与 Fork 后台场景**：分别请求显式后台定义式任务和未指定类型或 `subagent_type: "fork"` 的 Fork 任务。（验证：两种 Fork 写法均立即返回不同任务 ID，Fork 不阻塞主会话；完成后下一轮主 Agent 请求可见 task-notification。）
- [ ] **自动后台与边界场景**：使用测试时钟或测试配置触发等价的 120 秒前台阈值，并请求 `isolation: worktree`。（验证：超时任务被接管且未重新开始；Worktree 请求明确报告本章未支持，未创建隔离工作区。）

## 当前验收证据（2026-08-20）

- [x] `go test ./... -count=1` 通过。
- [x] `go build ./cmd/mewcode` 通过。
- [x] `go test -race ./internal/subagent ./internal/agent ./internal/tui -count=1` 通过。
- [x] `rg -n 'enable_verification_agent|UserConfigDir|ch13-subagent|ESC|worktree' README.md .mewcode/config.example.yaml docs/README.md docs/ch13-subagent` 已验证配置、文档索引和跨平台路径说明。
- [ ] 真实 Provider/TUI 的定义覆盖、ESC 转后台、120 秒自动后台和后台完成通知尚未人工执行；前提是准备可用 Provider 配置与隔离项目目录。

## ESC 提示与后台终态通知修复证据（2026-08-23）

- [x] `gofmt -w internal/tui/model.go internal/tui/update.go internal/tui/tui_test.go && go test ./internal/tui -run 'Test(EscapeSystemMessageStaysBeforeLaterConversationTurn|InitConsumesFirstSubAgentTerminalNotification|SubAgentTerminalNotificationsContinueInOrder)$' -count=1` 通过：ESC 提示在成功主回合后位于下一轮前；TUI 初始化消费首个终态；连续终态按顺序显示且输入可用。
- [x] `go test ./internal/tui -count=1` 通过。
- [x] `go test ./internal/tui ./internal/agent -run 'Test.*(Escape|SubAgent|TaskNotification)' -count=1` 通过：ESC、TUI 通知与主 Agent 通知注入兼容路径无回归。
- [x] `go build ./cmd/mewcode && git diff --check` 通过。
- [ ] `go test ./... -count=1` 未全绿：本次变更相关的 TUI、Agent 与其余非 OpenAI 包通过；仅 `internal/provider/openai.TestStreamCapturesFinalRequestPayload` 失败，当前日志实际含请求正文，详见 `bugs/2026-08-14/003-provider-request-body-logged.md`。
- [x] `gofmt -w internal/tools/executor.go internal/tools/tools_test.go internal/agent/subagent.go internal/agent/subagent_test.go && go test ./internal/tools ./internal/agent -run 'Test(ExecutorDoesNotApplyGeneralTimeoutToAgent|ExecutorUnknownAndTimeout|ForegroundSubAgentContextDetachesParentDeadline)$' -count=1` 通过：Agent 不继承通用期限，普通工具期限保持，前台 lifetime context 不继承父 deadline 且可显式取消。
- [x] `go test ./internal/tools ./internal/agent ./internal/tui -count=1 && go build ./cmd/mewcode && git diff --check` 通过。
- [x] 真实 Provider/TUI 场景 D 已复测：两次 `sleep 10` 的定义式前台任务在 ESC 接管后返回 `async_launched`/`subagent-1`；ESC 提示位于原回合后、下一轮“你好”前；主会话可继续回复；随后显示 `manual-esc-background completed: 完成`。该结果确认接管后任务未被原前台调用取消。
- [x] 真实 Provider/TUI 场景 E 已复测：未按 ESC 的五次 `sleep 25` 定义式前台任务返回 `async_launched`/`subagent-2`，随后显示 `manual-auto-background completed: 完成`。这确认任务未受通用 30 秒期限截断并在既有自动后台路径后完成；该转录未显示每次内部 `run_command`，故“五次且不重复启动”仍以自动化测试为决定性证据。

## Fork 参数兼容验收证据（2026-08-23）

- [x] `go test ./internal/subagent ./internal/agent ./internal/tools -run 'Test.*(Fork|Reserved|AgentTool|DefinitionSubAgent)' -count=1` 通过：`fork` 保留名及变体被拒绝；省略、空白和 `fork` 变体均被识别为 Fork；`subagent_type: "fork"` 的模拟调用保留父历史并后台运行；schema 说明兼容写法。
- [x] `go build ./cmd/mewcode` 通过。
- [x] `git diff --check` 通过。
- [ ] `go test ./... -count=1` 未全绿：仅 `internal/provider/openai.TestStreamCapturesFinalRequestPayload` 仍期待日志包含请求正文，与当前脱敏实现冲突；详情见 `bugs/2026-08-14/003-provider-request-body-logged.md`。
- [ ] 尚未以真实 Provider/TUI 重跑场景 C；下次复测应验证 `subagent_type: "fork"` 立即返回 `async_launched` 和任务 ID。
