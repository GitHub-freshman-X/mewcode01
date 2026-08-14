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

## T8：索引更新与章节级验证

**文件：** docs/README.md、docs/ch10-slash_command/{spec,plan,task,checklist}.md  
**依赖：** T1–T7

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

期望：全部命令退出码为 0，且九个命令的注册、分流、模式切换、补全和安全日志由自动化测试覆盖。

## 执行顺序

    T1 → T2 ────────────┐
    T3 ─────────────────┼→ T5 → T6 → T7 → T8
    T4 ─────────────────┘

- T1、T3、T4 可并行执行。
- T2 依赖 T1。
- T5 依赖命令、会话切换与记忆服务。
- T6、T7 按顺序接入 TUI；T8 更新索引并做全量验证。

