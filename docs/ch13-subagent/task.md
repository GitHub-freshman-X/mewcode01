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
| 修改 | `internal/agent/*_test.go` | 定义式/Fork、隔离、通知与后台路径测试 |
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

## 执行顺序

```text
T1 ─┬→ T3 → T4 ─┬→ T9 ─┐
T2 ─┘          │       ├→ T11 → T12 ─┬→ T14 → T16
               T5 → T6 → T8 ─┘        └→ T15 ─┘
                       └────────→ T10 ─┘
T11 ─────────────────────────────→ T13 → T15
```
