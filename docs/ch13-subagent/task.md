# MewCode SubAgent 子任务分发 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/subagent/definition.go`、`discover.go`、`filter.go` | 定义、四来源加载、工具过滤 |
| 新建 | `internal/subagent/task_manager.go` | 进程内任务状态、事件消费、通知与接管 |
| 新建 | `internal/subagent/builtins/*.md` | 四个内置角色定义 |
| 新建 | `internal/subagent/*_test.go` | 定义、加载、过滤和任务管理自动测试 |
| 新建 | `internal/tools/agent.go`、`agent_test.go` | 固定 Agent 工具和运行时桥接 |
| 新建 | `internal/agent/subagent.go`、`run_completion.go` | 创建子 Runner、Fork、执行到终态 |
| 修改 | `internal/agent/event.go`、`runner.go`、`scheduler.go` | 运行时注入、子任务事件、通知队列 |
| 修改 | `internal/agent/subagent.go`、`subagent_test.go` | Fork 兼容分流与后台/历史回归测试 |
| 修改 | `internal/subagent/definition.go`、`subagent_test.go` | 保留 `fork` 角色名校验与来源加载回归 |
| 修改 | `internal/tools/agent.go`、`agent_test.go` | Fork 兼容语义的 schema 说明与断言 |
| 修改 | `internal/hooks/executor.go`、`internal/hooks/*_test.go` | Hook agent 动作接入同一 Runtime |
| 修改 | `internal/tui/model.go`、`update.go`、`view.go`、`tui_test.go` | 子任务视图、ESC 接管与通知渲染 |
| 修改 | `internal/config/config.go`、`load.go`、`validate.go`、测试 | Verification 开关加载、默认值和校验 |
| 修改 | `cmd/mewcode/main.go`、`main_test.go` | 组合根、定义发现、TaskManager 与 Hook 注入 |
| 修改 | `.mewcode/config.example.yaml`、`README.md`、`docs/README.md` | 配置完整面、用户说明、章节索引 |
| 新建 | `docs/ch13-subagent/manual_scenarios.md` | TUI 前台、ESC、后台通知人工验证 |
| 修改 | `docs/ch13-subagent/理论学习：SubAgent 子任务分发.md` | 用户级定义路径改为跨平台配置目录 |

## T1：增加 Verification 配置开关

**文件：** `internal/config/config.go`、`internal/config/load.go`、`internal/config/validate.go`、相应测试

**依赖：** 无

**步骤：**
1. 在 `AgentConfig` 增加 `EnableVerificationAgent`，YAML 键为 `enable_verification_agent`，默认 false。
2. 在严格 YAML 加载结构与默认值合并中保留该字段。
3. 增加默认关闭、显式开启和未知/非法配置仍被拒绝的测试。

**验证：** `go test ./internal/config -count=1` 通过。

## T2：定义 Agent 定义模型与 frontmatter 解析

**文件：** `internal/subagent/definition.go`、`internal/subagent/definition_test.go`

**依赖：** 无

**步骤：**
1. 定义来源、创建模式、Definition 和有效模型/权限模式校验。
2. 解析 YAML frontmatter 和 Markdown body，要求名称、用途与正文非空，校验工具列表、最大轮次和权限模式。
3. 为缺失分隔符、空正文、无效模型、无效权限模式、负最大轮次及省略最大轮次时采用默认值编写测试。

**验证：** `go test ./internal/subagent -run 'Test(Parse|Validate)Definition' -count=1` 通过。

## T3：提供并加载内置角色

**文件：** `internal/subagent/builtins/{explore,plan,general-purpose,verification}.md`、`internal/subagent/definition.go`、测试

**依赖：** T1、T2

**步骤：**
1. 添加四个嵌入式 Markdown 定义，使用既有权限模式和明确职责。
2. 实现内置定义加载：始终提供 Explore、Plan、general-purpose；由配置开关决定 Verification。
3. 测试默认集合、启用 Verification 的集合以及每个定义的可解析性。

**验证：** `go test ./internal/subagent -run TestBuiltin -count=1` 通过。

## T4：实现多来源发现与同名覆盖

**文件：** `internal/subagent/discover.go`、`internal/subagent/discover_test.go`

**依赖：** T2、T3

**步骤：**
1. 接收项目目录、用户配置目录和插件注入定义，扫描各目录下的 `*.md`。
2. 按 plugin → builtin → user → project 合并，并保留来源和路径用于诊断。
3. 测试项目/用户/内置/插件同名覆盖、缺失目录、目录内重复名称、无效定义和跨平台用户目录输入。

**验证：** `go test ./internal/subagent -run TestDiscover -count=1` 通过。

## T5：实现多层工具过滤

**文件：** `internal/subagent/filter.go`、`internal/subagent/filter_test.go`

**依赖：** T2

**步骤：**
1. 定义全局禁止列表和固定后台白名单，二者均包含/排除实际注册工具名。
2. 实现全局禁止 → 角色黑名单 → 可选白名单 → 后台白名单的确定性过滤。
3. 覆盖未知白名单工具、空白名单、黑白名单交集、定义式禁止 Agent、后台禁止 Agent 的测试。

**验证：** `go test ./internal/subagent -run TestFilter -count=1` 通过。

## T6：实现任务管理器与状态通知

**文件：** `internal/subagent/task_manager.go`、`internal/subagent/task_manager_test.go`

**依赖：** 无

**步骤：**
1. 实现安全摘要 TaskInfo、唯一 ID、状态迁移、快照查询和订阅通知。
2. 让 TaskManager 成为子 Runner 事件流唯一消费者，归集工具调用数、Token 用量、终态文本或失败摘要。
3. 实现前台等待与运行中任务接管，不重启事件流；测试完成、失败、取消、重复接管、订阅通知和并发查询。

**验证：** `go test -race ./internal/subagent -run TestTaskManager -count=1` 通过。

## T7：新增固定 Agent 工具与运行时桥接

**文件：** `internal/tools/agent.go`、`internal/tools/agent_test.go`

**依赖：** 无

**步骤：**
1. 定义固定 `agent` 工具 metadata 和完整参数 schema。
2. 解析并校验 AgentInput 的必填参数与可选字段，拒绝不支持的 Worktree isolation。
3. 从调用 Context 获取 SubAgentHost 并将调用/失败转换为既有 `tools.Result`；测试 schema 稳定、输入校验、缺 Host 和 Host 分发。

**验证：** `go test ./internal/tools -run TestAgentTool -count=1` 通过。

## T8：扩展 Agent 事件与运行时注入

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`、`internal/agent/*_test.go`

**依赖：** T6、T7

**步骤：**
1. 增加子任务状态事件载荷和 Options 中的 SubAgentRuntime。
2. 在 Runner/Scheduler 的调用 Context 注入当前 SubAgentHost、创建来源标记和前台进度转发器。
3. 增加带锁的待发送任务通知队列，并在每轮模型请求前追加 `<task-notification>` 消息。
4. 测试通知不修改系统提示、主会话空闲时保留通知、通知内容不含敏感正文。

**验证：** `go test ./internal/agent -run 'Test.*(SubAgent|TaskNotification)' -count=1` 通过。

## T9：实现定义式子 Runner 工厂

**文件：** `internal/agent/subagent.go`、`internal/agent/subagent_test.go`

**依赖：** T4、T5、T7、T8

**步骤：**
1. 解析指定角色，按调用参数覆盖模型，并新建独立 Session、权限引擎和上下文管理器。
2. 应用 T5 的工具过滤，使用角色 System Prompt 和任务构造定义式首轮请求。
3. 测试空白历史、角色提示、模型/轮次/权限选择、消息/权限/用量隔离及共享 Provider、Hook、工作区。

**验证：** `go test ./internal/agent -run TestDefinitionSubAgent -count=1` 通过。

## T10：实现 Fork 子 Runner 与递归阻断

**文件：** `internal/agent/subagent.go`、`internal/agent/subagent_test.go`

**依赖：** T5、T8

**步骤：**
1. 克隆父历史并补齐末尾无结果的工具调用，追加 Fork Boilerplate 和任务消息。
2. 保持父已渲染系统提示、工具集及请求前缀；创建独立 Session、权限、上下文与用量归集。
3. 以 Context 来源标记拒绝 Fork 再 Fork；测试占位结果、前缀保持、缓存 Token 归集和递归拒绝。

**验证：** `go test ./internal/agent -run TestForkSubAgent -count=1` 通过。

## T11：实现 RunToCompletion 与后台分流

**文件：** `internal/agent/run_completion.go`、`internal/agent/subagent.go`、测试

**依赖：** T6、T8、T9、T10

**步骤：**
1. 消费子任务事件到纯文本完成、迭代上限、取消或失败，并返回最终文本或安全失败摘要。
2. 定义式前台同步等待；显式后台立即返回任务 ID；Fork 无条件走后台。
3. 为前台任务加入 120 秒定时接管；使用可注入时钟/时长测试自动后台，不等待真实时间。
4. 测试三种终态、显式后台、Fork 强制后台和 120 秒自动接管。

**验证：** `go test ./internal/agent -run 'Test.*(RunToCompletion|Background|AutoBackground)' -count=1` 通过。

## T12：接入 Hook 与启动组合根

**文件：** `internal/hooks/executor.go`、`internal/hooks/*_test.go`、`cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** T1、T3、T4、T6、T11

**步骤：**
1. 在启动入口由项目根、`os.UserConfigDir()`、配置开关和插件定义构造 Registry、TaskManager、Runtime 和 Agent 工具。
2. 将 Hook 的 AgentRunner 适配到同一 Runtime，替换“运行时未接入”占位行为。
3. 测试默认不加载 Verification、开启后可用、用户级目录来自 UserConfigDir，以及 Hook agent 动作走真实运行时桥接。

**验证：** `go test ./cmd/mewcode ./internal/hooks -run 'Test.*(SubAgent|Agent)' -count=1` 通过。

## T13：实现 TUI 进度、ESC 接管与终态提示

**文件：** `internal/tui/model.go`、`internal/tui/update.go`、`internal/tui/view.go`、`internal/tui/tui_test.go`

**依赖：** T6、T8、T11

**步骤：**
1. 在 Model 保存前台/后台子任务视图并处理子任务状态事件。
2. 当前台子 Agent 可接管时，ESC 调用 Runner 的后台接管接口；保留 Ctrl+C 取消主任务的语义。
3. 渲染任务 ID、名称、状态、耗时、进度和安全终态通知，接管后恢复文本输入。
4. 测试前台进度渲染、ESC 状态迁移、输入恢复、后台完成/失败通知和 Ctrl+C 回归。

**验证：** `go test ./internal/tui -run 'Test.*(SubAgent|Background|Escape)' -count=1` 通过。

## T14：补齐端到端回归与安全日志测试

**文件：** `internal/agent/runner_test.go`、`internal/subagent/*_test.go`、`internal/tui/tui_test.go`

**依赖：** T8-T13

**步骤：**
1. 用脚本 Provider 覆盖主 Agent 调用定义式和 Fork Agent 的完整工具结果回灌链。
2. 覆盖子任务失败、取消和后台完成不终止主 Agent，且下一次请求收到 task-notification。
3. 验证日志字段只有模式、状态、计数、耗时、用量、模型和类型等安全元数据，不含 prompt、工具正文、密钥或原始错误。
4. 运行既有 Agent、工具、权限、Hook、TUI 的全量回归。

**验证：** `go test ./internal/agent ./internal/subagent ./internal/tools ./internal/hooks ./internal/tui ./internal/permissions -count=1` 通过。

## T15：同步用户文档、配置与人工场景

**文件：** `.mewcode/config.example.yaml`、`README.md`、`docs/README.md`、`docs/ch13-subagent/manual_scenarios.md`、`docs/ch13-subagent/理论学习：SubAgent 子任务分发.md`

**依赖：** T1、T3、T12、T13

**步骤：**
1. 在配置示例列出 `agent.enable_verification_agent: false` 实际默认值。
2. 在 README 说明跨平台用户定义目录、项目目录、frontmatter、覆盖顺序、内置角色、后台行为、ESC、通知及本章限制。
3. 在文档索引增加第 13 章；在理论文档将 `~/.mewcode/agents/` 改为用户配置目录路径。
4. 编写人工场景：定义覆盖、Verification 开关、显式后台、超过 120 秒自动后台、ESC 接管、后台完成通知和不支持 Worktree。

**验证：** `rg -n 'enable_verification_agent|UserConfigDir|ch13-subagent|ESC|worktree' README.md .mewcode/config.example.yaml docs/README.md docs/ch13-subagent` 输出预期说明。

## T16：执行全量验证并记录证据

**文件：** `docs/ch13-subagent/checklist.md`（验收阶段填写证据）

**依赖：** T14、T15

**步骤：**
1. 执行格式化、全量单元测试和构建。
2. 执行 T15 的人工场景并记录实际结果、未验证项和环境前提。
3. 按 checklist 逐项填写命令、观察结果和通过/失败状态；失败时先修复并重跑对应验证。

**验证：** `gofmt -w` 后 `go test ./... -count=1` 与 `go build ./cmd/mewcode` 均通过。

## T17：拒绝保留的 Fork 角色名

**文件：** `internal/subagent/definition.go`、`internal/subagent/subagent_test.go`

**依赖：** T2、T4

**步骤：**
1. 在定义校验中将去除首尾空白且大小写无关等于 `fork` 的名称判为无效，并保留现有名称和错误诊断约定。
2. 增加直接解析与通过来源发现加载的测试，覆盖 `fork`、` Fork ` 和其他正常名称。

**验证：** `go test ./internal/subagent -run 'Test(ParseDefinition|Discover).*Fork|Test.*Reserved.*Fork' -count=1` 通过。

## T18：将 `fork` 类型归一为 Fork 创建

**文件：** `internal/agent/subagent.go`、`internal/agent/subagent_test.go`、`internal/tools/agent.go`、`internal/tools/agent_test.go`

**依赖：** T17

**步骤：**
1. 在运行时将省略类型、空白类型和 `fork`（忽略首尾空白及大小写）统一识别为 Fork，避免对该名称进行定义查找。
2. 保持两种 Fork 输入均强制后台，并复用父历史、占位工具结果和递归阻断逻辑。
3. 更新工具 schema 的 `subagent_type` 描述；不增加或删除参数。
4. 增加脚本 Provider 回归测试：显式 `fork` 返回异步任务、子请求带 Fork boilerplate 和父历史；定义式非 `fork` 行为保持不变；schema 含兼容说明。

**验证：** `go test ./internal/agent ./internal/tools -run 'Test.*(Fork|AgentTool|DefinitionSubAgent)' -count=1` 通过。

## T19：同步人工验收与全量回归

**文件：** `docs/ch13-subagent/manual_scenarios.md`、`docs/ch13-subagent/checklist.md`

**依赖：** T18

**步骤：**
1. 将场景 C 的预检调整为接受字段缺失或值为 `fork`，两者均按 Fork 验收；其他类型值仍记录为模型调用错误。
2. 在 checklist 中增加兼容 Fork 与保留角色名的可观察验收项和自动化验证命令。
3. 格式化变更的 Go 文件，执行目标测试、全量测试和构建，并将实际证据写入 checklist；真实 Provider 场景如未执行必须标明。

**验证：** `gofmt -w internal/agent/subagent.go internal/agent/subagent_test.go internal/subagent/definition.go internal/subagent/subagent_test.go internal/tools/agent.go internal/tools/agent_test.go && go test ./... -count=1 && go build ./cmd/mewcode` 通过。

## T20：建立通知监听与临时提示锚点

**文件：** `internal/tui/model.go`

**依赖：** T13

**步骤：**
1. 为系统消息增加仅表示“等待当前主任务提交后定位”的内部状态；保持普通系统消息和失败/取消临时消息的现有锚点语义。
2. 在 TUI 初始化命令中并行启动输入焦点、已有权限桥监听（如有）和首次子 Agent 通知等待。
3. 不修改 TaskManager、Runner 的模型通知注入或持久会话历史。

**验证：** `go test ./internal/tui -run 'Test.*(SystemMessage|SubAgent)' -count=1` 通过。

## T21：在主任务终态解析 ESC 提示位置

**文件：** `internal/tui/update.go`

**依赖：** T20

**步骤：**
1. ESC 接管产生的系统提示标记为等待当前主任务提交后定位。
2. 主任务以 completed 或 stopped 终态结束且显示历史已提交后，将该提示定位到当前回合末尾。
3. 主任务 failed 或 cancelled 时清除等待定位标记，保留其在临时回合之后的显示，避免未来成功回合重定位旧提示。

**验证：** ESC 排序回归测试在实现前失败、实现后通过；`go test ./internal/tui -run 'Test.*(Escape|SystemMessage)' -count=1` 通过。

## T22：补充 TUI 回归测试

**文件：** `internal/tui/tui_test.go`

**依赖：** T20、T21

**步骤：**
1. 构造 ESC 提示在主任务运行期间产生、主回合提交、再提交下一轮对话的场景，断言其相对顺序正确。
2. 构造初始化后的后台任务终态通知，断言首次通知被消费并渲染任务名称、终态和安全摘要。
3. 构造连续多个终态通知，断言均只显示一次且按接收顺序显示；同时断言输入仍可用。

**验证：** `go test ./internal/tui -count=1` 通过。

## T23：同步验收文档与问题记录

**文件：** `docs/ch13-subagent/checklist.md`、`docs/ch13-subagent/manual_scenarios.md`、`bugs/2026-08-22/002-ch13-marker-role-override-unverified.md`、`bugs/2026-08-22/README.md`

**依赖：** T22

**步骤：**
1. 将 AC12–AC15 的自动化验证命令、实际结果和未执行的真实 Provider/TUI 复测写入 checklist。
2. 在场景 D 中要求确认 ESC 提示位于下一轮输入前，并分别确认后台任务终态通知。
3. 在问题记录中记录修复方案、验证结果和最终状态；若真实 Provider/TUI 未复测，明确标为待验证。

**验证：** `git diff --check` 无输出，文档中的命令和测试名称与实现一致。

## T24：修复 Agent 前台期限与接管 context

**文件：** `internal/tools/executor.go`、`internal/tools/tools_test.go`、`internal/agent/subagent.go`、`internal/agent/subagent_test.go`

**依赖：** T11、T20–T22

**步骤：**
1. 仅让 `agent` 工具跳过 Executor 的通用 30 秒 deadline；保持其他工具的期限和超时错误不变。
2. 为前台子任务建立独立 lifetime context；仅在任务仍前台时转发父 context 的取消。
3. ESC 或自动接管后停止该取消转发，复用原 TaskManager 任务和 Worker；未接管时保持 `Ctrl+C` 取消。
4. 增加通用期限豁免、普通工具期限保持、接管后父取消不终止任务、未接管父取消终止任务的回归测试。

**验证：** `go test ./internal/tools ./internal/agent -run 'Test.*(Agent|Background|AutoBackground|Timeout|Cancellation)' -count=1` 通过。

## T25：同步期限修复验收

**文件：** `docs/ch13-subagent/checklist.md`、`docs/ch13-subagent/manual_scenarios.md`、`bugs/2026-08-22/002-ch13-marker-role-override-unverified.md`

**依赖：** T24

**步骤：**
1. 将 AC16 的自动化验证和真实场景 D/E 复测要求写入 checklist。
2. 在场景 D/E 记录通用 30 秒期限不应截断前台 120 秒自动接管，以及接管后必须完成或给出真实终态。
3. 更新问题记录的修复方案、验证证据与未验证项。

**验证：** `git diff --check` 无输出。

## T26：增加单次、非阻塞的终态回调

**文件：** `internal/subagent/task_manager.go`、`internal/subagent/subagent_test.go`

**依赖：** T6

**步骤：**
1. 在 `LaunchRequest` 增加可选终态回调；回调只接收冻结的 `TaskInfo` 快照。
2. 在 `finish` 完成状态更新、关闭等待通道并发布既有通知后异步调用回调；保持非法或重复终态不触发回调。
3. 增加 completed、failed、cancelled 回调测试，断言每个任务仅触发一次、回调获得最终状态、后台标记、用量和工具调用数；验证回调阻塞不延迟 `Wait` 或通知可见性。

**验证：** `go test -race ./internal/subagent -run 'TestTaskManager.*(Completion|Terminal|Callback)' -count=1` 通过。

## T27：写入安全的 SubAgent 终态日志

**文件：** `internal/agent/subagent.go`、`internal/agent/subagent_test.go`

**依赖：** T26

**步骤：**
1. 在 `SubAgentRuntime.dispatch` 创建任务时注册终态回调，捕获父 Logger、创建模式和子 Runner 的实际模型。
2. 将终态快照映射为一条 `stage=subagent` 日志：状态、模式、后台标记、模型、工具调用数、三类 Token 用量和毫秒耗时；仅为 failed/cancelled 加入安全失败摘要。
3. 增加定义式前台 completed、显式或接管后台 completed、Fork completed、failed 和 cancelled 的日志断言；每项确认仅一条终态日志，且不含 prompt、模型消息、工具正文、密钥、请求头或原始错误。

**验证：** `go test ./internal/agent ./internal/subagent -run 'Test.*(SubAgent|TaskManager).*(Terminal|Log|Completion|Cancellation)' -count=1` 通过。

## T28：同步验收证据与问题记录

**文件：** `docs/ch13-subagent/checklist.md`、`bugs/2026-08-23/001-ch13-subagent-terminal-status-not-logged.md`

**依赖：** T27

**步骤：**
1. 在 checklist 增加 completed、failed、cancelled 的终态日志、全路径覆盖和脱敏验证项，并填写实际命令和结果。
2. 更新问题记录的根因、修复方案、验证方式和最终状态；未执行的真实 Provider 复测须明确标注。
3. 运行格式化、定向测试、race、相关包回归、构建和 `git diff --check`。

**验证：** `gofmt -w internal/subagent/task_manager.go internal/subagent/subagent_test.go internal/agent/subagent.go internal/agent/subagent_test.go && go test -race ./internal/subagent ./internal/agent -count=1 && go build ./cmd/mewcode && git diff --check` 通过。

## 执行顺序

```text
T1 ─┬→ T3 → T4 ─┬→ T9 ─┐
T2 ─┘          │       ├→ T11 → T12 ─┬→ T14 → T16
               T5 → T6 → T8 ─┘        └→ T15 ─┘
                       └────────→ T10 ─┘
T11 ─────────────────────────────→ T13 → T15
T2 → T4 → T17 → T18 → T19
T13 → T20 → T21 → T22 → T23
T11 → T24 → T25
T6 → T26 → T27 → T28
```
