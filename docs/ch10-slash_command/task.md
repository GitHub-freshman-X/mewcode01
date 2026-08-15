# MewCode Slash Command Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | internal/command/{command,registry,parser,complete,builtins}.go | 命令模型、注册、解析、补全与内置 Handler。 |
| 新建 | internal/command/command_test.go | 命令核心和 Handler 边界测试。 |
| 修改 | internal/agent/{runner,runner_test}.go | 空闲会话替换及测试。 |
| 修改 | internal/memory/{service,memory_test}.go | 受限记忆命令服务及测试。 |
| 修改 | internal/tui/{model,update,view,keymap,run}.go | 命令运行时、分流、补全、视图与依赖注入。 |
| 修改 | internal/tui/tui_test.go | TUI 集成和回归测试。 |
| 修改 | internal/command/{command,builtins,command_test}.go | 会话标题透传、格式化与 `/session list` 回归测试。 |
| 修改 | internal/tui/{model,update,view,tui_test}.go | 本地命令文本与反馈的锚定临时展示及顺序回归测试。 |
| 修改 | internal/conversation/store_test.go | 首条用户消息标题派生的持久化回归测试（如现有覆盖不足）。 |
| 修改 | internal/conversation/{journal,session,store}.go 及测试 | 会话级 Token 增量记录、恢复聚合与旧 JSONL 兼容。 |
| 修改 | internal/agent/{runner,runner_test}.go | Provider 用量归属当前会话及恢复后的压缩时机验证。 |
| 修改 | internal/tui/{model,view,tui_test}.go | 状态栏与 `/status` 读取会话累计用量。 |
| 修改 | internal/command/{command,builtins,command_test}.go | `/exit` 注册、退出请求边界与命令核心测试。 |
| 修改 | internal/tui/{model,update,tui_test}.go | 空闲退出分流与任务/权限状态守卫测试。 |
| 修改 | cmd/mewcode/{main,main_test}.go | 命令依赖装配及启动回归。 |
| 修改 | docs/README.md | 第十章索引。 |

## T1：实现命令模型、注册中心与解析

**文件：** internal/command/command.go、registry.go、parser.go、command_test.go  
**依赖：** 无

**步骤：**

1. 定义 Kind、Command、Handler、Invocation、CommandContext，以及 UI、会话、记忆和日志服务接口；核心包不得依赖 TUI。
2. 实现按注册顺序保存的 Registry、名称/别名索引与 NewRegistry 冲突检测，统一小写规范化。
3. 实现只识别 / 前缀的解析器，明确处理空输入、单独 /、大小写命令名和首段空白后的参数。
4. 先为冲突、大小写、别名、非命令、空命令和参数修剪补测试，再完成最小实现。

**验证：** go test ./internal/command -run 'Test.*Registry|Test.*Parse' 通过。

## T2：实现补全、帮助、分发与九个内置命令

**文件：** internal/command/complete.go、builtins.go、command_test.go  
**依赖：** T1

**步骤：**

1. 实现名称及别名补全，排除隐藏命令、去重并稳定排序。
2. 注册 /help、/compact、/clear、/plan、/do、/session、/memory、/status、/review，以及参考文档的别名、用法、说明和参数提示。
3. 实现分发：/ 显示可见命令，未知命令给出 /help 引导，缺参显示 ArgPrompt；生命周期日志只记录安全元数据。
4. 实现 Handler：本地消息、/plan 模式与计划请求、/do 默认模式与 ModeDo、/compact 的 ModeCompact，以及 /review 的固定审查请求。
5. 以假 UI、会话、记忆、日志服务覆盖补全、帮助、未知命令、参数提示、模式、请求类型和日志脱敏。

**验证：** go test ./internal/command 通过。

## T3：支持 Runner 空闲会话替换

**文件：** internal/agent/runner.go、internal/agent/runner_test.go  
**依赖：** 无

**步骤：**

1. 增加仅在空闲状态可调用的会话替换入口，拒绝 nil Session、无效 ID 和活跃任务。
2. 在一个临界区更新 Session、SessionID 与上下文结果存储；失败不得改变原状态。
3. 测试活跃任务拒绝、成功切换后请求写入新会话和结果目录不串会话。

**验证：** go test ./internal/agent -run 'Test.*Replace.*Session|Test.*Session.*Switch' 通过。

## T4：暴露受限记忆命令服务

**文件：** internal/memory/service.go、internal/memory/memory_test.go  
**依赖：** 无

**步骤：**

1. 基于既有用户级、项目级目录与 MEMORY.md 提供概要、列表、按类别手动添加和清空入口。
2. 手动添加复用类别验证、路径 containment、frontmatter 和索引更新，拒绝未知类别、空内容和越界写入。
3. 清空仅影响受管理记忆文件与索引，不影响自动提取、治理锁或其他项目文件。
4. 覆盖两级列表、添加、索引更新、非法输入和清空范围。

**验证：** go test ./internal/memory -run 'Test.*Command|Test.*Memory.*(Add|List|Clear|Summary)' 通过。

## T5：装配命令运行时到 TUI

**文件：** internal/tui/model.go、run.go、cmd/mewcode/main.go、cmd/mewcode/main_test.go  
**依赖：** T1、T2、T3、T4

**步骤：**

1. 向 Model 注入命令注册中心、会话存储、记忆服务和日志，并初始化默认模式、系统消息和补全状态。
2. 由 Model 实现命令的 UI/会话/记忆服务边界：启动请求、模式、Token、系统消息、会话创建恢复删除与空闲 Runner 切换。
3. 扩展 Run、RunWithPermissions 和 main 装配，复用既有 SessionStore、Memory Service、Logger 和初始 Session，不新增配置。
4. 更新启动替身与测试，验证依赖注入且 config.example.yaml 不变。

**验证：** go test ./cmd/mewcode ./internal/tui -run 'Test.*(Run|NewModel|Command)' 通过。

## T6：接入回车分流与模式命令

**文件：** internal/tui/update.go、internal/tui/tui_test.go  
**依赖：** T2、T5

**步骤：**

1. 用命令解析和分发替换硬编码 parseRequest 斜杠分支，保留空输入、活跃任务、权限确认、取消和普通文本路径。
2. 非命令按当前模式启动 ModeAct 或 ModePlan；命令后清空输入并刷新；本地命令不得创建 Agent Task。
3. 实现 /plan 进入、退出与携带需求提交；/do 先退出计划模式，再执行已有待执行计划。
4. 覆盖普通消息、/HELP、未知命令、/、/plan toggle、/plan <需求>、/do、/review、/compact 及权限等待。

**验证：** go test ./internal/tui -run 'Test.*(Command|Plan|Do|Dispatch|Permission)' 通过。

## T7：实现 Tab 补全、系统反馈与状态栏

**文件：** internal/tui/update.go、view.go、keymap.go、tui_test.go  
**依赖：** T2、T5、T6

**步骤：**

1. 增加 Tab、方向键和候选接受键；单项直接写入，多项菜单支持方向键、Tab 或 Enter 接受。
2. 本地命令反馈以系统消息渲染，不写入会话历史或发送给 Agent。
3. 修改状态栏：始终显示 [DEFAULT] 或 [PLAN]、Token 用量及 /help 提示；保留任务与权限状态。
4. 覆盖零/单/多候选、隐藏命令、候选接受、系统消息隔离和模式可见性。

**验证：** go test ./internal/tui -run 'Test.*(Completion|SystemMessage|Status|Mode)' 通过。

## T8：按交互顺序显示本地命令和反馈

**文件：** internal/tui/model.go、internal/tui/update.go、internal/tui/view.go、internal/tui/tui_test.go
**依赖：** T2、T5、T6、T7

**步骤：**

1. 将临时展示状态由仅系统反馈扩展为带角色、内容和会话位置锚点的条目；用户命令条目使用与普通用户消息一致的视觉角色，系统反馈保持系统角色。
2. 在命令分发前记录本次临时反馈的起始位置与会话锚点；仅当分发结束后未启动 Agent 任务时，把原始命令文本插入该批反馈之前。启动 Agent 的命令继续走既有任务转录，不重复显示命令。
3. 修改视图渲染，在对应会话消息之后插入同一锚点的临时条目；当前失败任务的反馈仍位于其任务转录之后。
4. 覆盖“任务 1 → 本地命令 → 系统反馈 → 任务 2”、连续本地命令、未知命令和缺参提示；断言临时内容不进入 Session snapshot、Journal 或后续 Provider 请求。

**验证：** `go test ./internal/tui -run 'Test.*(Command.*Display|SystemMessage|Chronological)' -count=1` 通过。

## T9：在会话列表中显示首条用户消息标题

**文件：** internal/command/command.go、internal/command/builtins.go、internal/command/command_test.go、internal/conversation/store_test.go
**依赖：** T2

**步骤：**

1. 将既有 `conversation.SessionMeta.Title` 透传到命令层 `SessionMeta`，不修改 JSONL 格式、会话扫描规则或创建流程。
2. 在命令层实现纯本地标题格式化：去除首尾空白、按 Unicode 字符边界截断并添加省略号；空标题使用明确占位文本。
3. 调整 `/session list` 输出为“会话 ID + 标题”，使标题成为主要识别信息；当前会话摘要同步使用同一格式。
4. 覆盖含中文/emoji 的长标题、空会话、首条记录非用户消息与既有消息数兼容性；断言命令执行不启动 Agent 或 Provider 请求。

**验证：** `go test ./internal/command ./internal/conversation -run 'Test.*(Session.*Title|Session.*List)' -count=1` 通过。

## T10：持久化会话级 Token 用量

**文件：** internal/conversation/journal.go、internal/conversation/session.go、internal/conversation/store.go、internal/conversation/{session,store}_test.go、internal/agent/runner.go、internal/agent/runner_test.go、internal/tui/model.go、internal/tui/view.go、internal/tui/tui_test.go、internal/command/command_test.go
**依赖：** T2、T3、T5

**步骤：**

1. 为既有 JSONL 增加兼容的 `usage` 增量记录；校验输入、输出均为非负值，并使旧 history/plan 记录和旧 JSONL 文件继续可读。
2. 向 `Session` 增加获取与记录累计用量的入口，保证先成功追加 Journal、后更新内存；恢复时只聚合用量行，不把它们当作对话、标题、计划或消息数。
3. 在 Runner 成功完成普通、计划、执行或压缩的 Provider 调用后，把该调用的实际报告用量记入当前 Session；记账失败按现有任务错误路径处理，避免显示未持久化的数值。
4. 将 TUI `TokenUsage`、`/status` 和状态栏改为读取 Session 累计用量；会话新建显示零，恢复会话显示聚合结果。
5. 覆盖多轮及压缩用量累加、切换会话互不串值、重启恢复、旧 JSONL 默认零、无 Provider 的本地命令不变更用量；覆盖恢复本身不启动压缩或 Provider、下一条 Agent 任务仍在首次 Provider 请求前按历史内容触发既有压缩判断。

**验证：** `go test ./internal/conversation ./internal/agent ./internal/command ./internal/tui -run 'Test.*(Usage|Token|Session.*Resume|Restore.*Compact)' -count=1` 通过。

## T11：实现 `/exit` 本地退出命令

**文件：** internal/command/command.go、internal/command/builtins.go、internal/command/command_test.go、internal/tui/model.go、internal/tui/update.go、internal/tui/tui_test.go
**依赖：** T2、T5、T6

**步骤：**

1. 为命令 UI 边界增加退出请求能力；默认命令注册 `/exit`，Handler 只发出退出请求，不创建 Agent 请求或系统消息。
2. TUI 保存本次命令的退出请求，并在 `/exit` 成功分发后返回 Bubble Tea 的退出命令；退出前不提交命令到 Session、Display Journal 或 JSONL。
3. 覆盖 `/help` 与补全发现 `/exit`、空闲 `/exit` 返回退出命令、零 Agent/Provider 请求和零会话写入。
4. 覆盖活跃任务、权限确认与 `Ctrl+C` 的既有优先级，确保这些状态下不会把 `/exit` 当作可立即执行的命令。

**验证：** `go test ./internal/command ./internal/tui -run 'Test.*(Exit|CommandList|Completion|Permission)' -count=1` 通过。

## T12：索引更新与章节级验证

**文件：** docs/README.md、docs/ch10-slash_command/{spec,plan,task,checklist}.md  
**依赖：** T1–T11

**步骤：**

1. 在文档索引加入第十章的 Spec、Plan、Tasks、Checklist 与参考文档链接。
2. 确认不新增配置项，保持 .mewcode/config.example.yaml 不变。
3. 运行格式化、目标包测试、全量测试、静态检查和 diff 检查；失败时回到归属任务修复并重跑。
4. 检查本轮新增 .mew/ 或 logs/ 运行产物，仅清理可确认由本轮验证产生的目录。

**验证：**

    gofmt -w internal/command internal/agent/runner.go internal/agent/runner_test.go internal/memory/service.go internal/memory/memory_test.go internal/tui cmd/mewcode
    go test ./internal/command ./internal/agent ./internal/memory ./internal/tui ./cmd/mewcode
    go test ./...
    go vet ./...
    git diff --check

期望：全部命令退出码为 0，且十个命令的注册、分流、模式切换、补全、安全日志、命令显示顺序、会话标题、会话级 Token 用量和退出行为由自动化测试覆盖。

## 执行顺序

    T1 → T2 ────────────┐
    T3 ─────────────────┼→ T5 → T6 → T7 → T8 ─┐
    T4 ─────────────────┘                        ├→ T12
           └────────────→ T9 ────────────────────┤
                         T10 ─────────────────────┤
                               T11 ───────────────┘

- T1、T3、T4 可并行执行。
- T2 依赖 T1。
- T5 依赖命令、会话切换与记忆服务。
- T6、T7、T8 按顺序接入 TUI；T9 可在 T2 后独立完成；T10 依赖会话切换、命令装配和 TUI 状态；T11 依赖命令与 TUI 分流；T12 更新索引并做全量验证。
