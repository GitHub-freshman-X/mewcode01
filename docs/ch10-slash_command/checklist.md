# MewCode Slash Command Checklist

> 每项均以自动化测试、可观察界面状态或日志内容验证；测试使用本地替身、临时目录和内存日志，不调用真实模型、网络或用户实际目录。

## 命令核心

- [ ] **AC1 注册与冲突失败**：九个内置命令可查询；名称重复、名称与别名冲突、别名冲突均在创建注册中心时失败并包含冲突标识。（验证：go test ./internal/command -run 'Test.*Registry|Test.*Conflict' -count=1，期望全部断言通过）
- [ ] **AC2 解析与查找**：/HELP、别名 /h 均命中 help；/ 显示可见命令；未知命令提示 /help；空输入不创建 Agent 请求。（验证：go test ./internal/command ./internal/tui -run 'Test.*(Parse|Alias|Unknown|Empty|CommandList)' -count=1，期望请求计数和反馈断言通过）
- [ ] **AC6 补全**：隐藏命令不出现在候选中；唯一候选直接填入输入；多个候选显示稳定菜单并可选择。（验证：go test ./internal/command ./internal/tui -run 'Test.*Completion' -count=1，期望候选、输入框和菜单断言通过）
- [ ] **AC7 帮助与参数提示**：帮助列表含名称、别名、说明和用法；help 查询单条命令正确；缺少必需参数显示对应参数提示。（验证：go test ./internal/command -run 'Test.*(Help|ArgPrompt)' -count=1，期望文本断言通过）

## 执行模式与界面集成

- [ ] **AC3 本地与提示词命令边界**：/help、/status、/clear、/session、/memory 的本地分支不启动 Agent 任务；/compact 走既有 ModeCompact；/review <关注点> 通过普通 Agent 请求发送固定审查要求与附加关注点。（验证：go test ./internal/command ./internal/tui -run 'Test.*(Local|Compact|Review|Prompt)' -count=1，期望请求模式和消息断言通过）
- [ ] **AC4 计划与执行模式**：默认状态执行 /plan 后状态栏显示 [PLAN]；在计划状态再次执行 /plan 后显示 [DEFAULT]；/plan <需求> 进入计划模式并提交 ModePlan；/do 先显示默认模式并触发 ModeDo 执行待执行计划。（验证：go test ./internal/command ./internal/tui -run 'Test.*(Plan|Do|Mode|Status)' -count=1，期望模式、状态栏和请求断言通过）
- [ ] **AC5 输入分流与既有交互**：非斜杠文本仍按当前模式交给 Agent；命令输入不落入普通对话；任务取消与权限确认状态下的按键处理保持既有行为。（验证：go test ./internal/tui -run 'Test.*(Dispatch|Plain|Cancel|Permission)' -count=1，期望请求、任务和确认断言通过）
- [ ] **系统消息隔离**：命令反馈显示在 TUI，但不会写入 Session history、display journal 或后续模型请求。（验证：go test ./internal/tui ./internal/conversation -run 'Test.*SystemMessage' -count=1，期望会话快照不含反馈文本）
- [ ] **候选菜单操作**：多个匹配后，上下键只改变选项；Tab 或 Enter 接受当前选项；无候选时不改变输入。（验证：go test ./internal/tui -run 'Test.*Completion.*(Select|Accept|Empty)' -count=1，期望输入框和选择索引断言通过）

## 会话与记忆命令

- [ ] **会话切换安全**：/clear、/session new 与 /session resume 创建或恢复后，Runner 和 TUI 均指向同一新 Session；活跃 Agent 任务期间切换被拒绝且不改原会话。（验证：go test ./internal/agent ./internal/tui -run 'Test.*(Session.*Switch|Clear|Resume)' -count=1，期望 Session ID 与拒绝分支断言通过）
- [ ] **会话管理**：/session 无参数显示当前 ID、消息数；list、new、resume <id>、delete <id> 分别调用第九章既有存储能力，并对缺失参数或无效 ID 给出可操作提示。（验证：go test ./internal/command ./internal/tui -run 'Test.*Session' -count=1，期望服务调用和系统消息断言通过）
- [ ] **记忆管理**：/memory 显示用户级和项目级概要；list、add <类别> <内容> 与 clear 只影响既有记忆目录及索引；未知类别、空内容和目录外文件被拒绝。（验证：go test ./internal/memory ./internal/command -run 'Test.*(Memory|Command.*Memory)' -count=1，期望文件范围、索引及错误提示断言通过）
- [ ] **清空确认**：/memory clear 先提供确认提示；未经明确确认不得删除任何记忆文件或索引。（验证：go test ./internal/command ./internal/tui -run 'Test.*Memory.*Clear.*Confirm' -count=1，期望确认前后目录快照断言通过）

## 安全、回归与配置

- [ ] **AC9 安全日志**：命令生命周期日志包含命令名、类型、状态、耗时或候选数；不含用户参数、review prompt、会话正文、Token 或密钥。（验证：go test ./internal/command ./internal/tui -run 'Test.*Command.*Log' -count=1，期望敏感测试字符串不出现在日志）
- [ ] **配置边界**：本章未添加配置项，.mewcode/config.example.yaml 保持不变；命令清单来自代码注册。（验证：git diff -- .mewcode/config.example.yaml；期望无输出，并运行 go test ./cmd/mewcode -run 'Test.*Run' -count=1）
- [ ] **构建、测试与静态检查**：命令核心、Agent、记忆、TUI、启动层和全项目测试通过，格式与静态检查无问题。（验证：依次运行 gofmt -d internal/command internal/agent internal/memory internal/tui cmd/mewcode、go test ./... -count=1、go vet ./...、git diff --check；期望所有命令退出码 0，gofmt 无输出）
- [ ] **离线性**：命令相关测试使用 Provider、UI、会话、记忆及日志替身，不访问真实模型或网络。（验证：审查命令测试替身，并运行 go test ./... -count=1；期望无外部凭据或网络依赖）

## 端到端场景

- [ ] **从计划到执行**：启动默认 TUI，执行 /plan，观察 [PLAN]；输入需求，观察 ModePlan 请求和待执行计划；执行 /do，观察 [DEFAULT]，并由 ModeDo 处理已保存计划。（验证：go test ./internal/tui ./internal/agent -run 'Test.*EndToEnd.*Plan.*Do' -count=1，期望模式、请求和计划消费断言通过）
- [ ] **本地快车道与提示词快捷方式**：依次提交 /status、/help、/review 特别注意并发安全。前两项仅产生系统消息且不启动任务，最后一项启动普通 Agent 请求，prompt 同时包含审查基线和额外关注点。（验证：go test ./internal/command ./internal/tui -run 'Test.*EndToEnd.*(Local|Review)' -count=1，期望任务计数与 prompt 断言通过）
- [ ] **发现与纠错流程**：输入 / 后用 Tab 查看并选择候选；输入未知命令并看到 /help 引导；输入缺参的会话或记忆子命令并看到用法提示。（验证：go test ./internal/tui ./internal/command -run 'Test.*EndToEnd.*(Completion|Unknown|ArgPrompt)' -count=1，期望菜单、提示和输入框断言通过)

