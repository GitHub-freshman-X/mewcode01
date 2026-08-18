# MewCode Skill System Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/skills/skill.go` | Skill 元数据、执行模式、历史范围、Catalog、激活与运行时类型 |
| 新建 | `internal/skills/discover.go` | frontmatter 解析、三层来源扫描、优先级与校验 |
| 新建 | `internal/skills/manager.go` | 线程安全快照、激活状态与原子刷新 |
| 新建 | `internal/skills/runtime.go` | SOP 渲染、`{{args}}` 替换、工具交集与模型选择 |
| 新建 | `internal/skills/load_tool.go` | 系统级 `load_skill` 工具 |
| 新建 | `internal/skills/builtins.go`、样板资源 | `commit`、`review`、`test` 内置 Skill |
| 修改 | `internal/tools/registry.go` | 名称查询和按名称生成受限 Registry |
| 修改 | `internal/prompt/builder.go`、`internal/prompt/sections.go` | 轻量目录和已激活 SOP 模块 |
| 修改 | `internal/provider/provider.go` | 请求级模型覆盖字段 |
| 修改 | `internal/provider/anthropic/request.go`、`internal/provider/openai/request.go` | 请求级模型编码与默认回退 |
| 修改 | `internal/agent/event.go`、`internal/agent/runner.go` | Skill Invocation、运行时快照、inline/fork 分流和安全日志 |
| 修改 | `internal/command/command.go`、`internal/command/registry.go`、`internal/command/builtins.go` | Skill 服务、动态命令和 `/skills reload` |
| 修改 | `internal/tui/model.go`、`internal/tui/update.go` | 运行时命令表替换及会话切换清理 |
| 修改 | `cmd/mewcode/main.go` | 在工具/MCP 注册完成后初始化并注入 Skill Manager |
| 修改 | 现有 `internal/agent/runner_test.go`、`internal/command/command_test.go`、`internal/prompt/prompt_test.go`、`internal/tui/tui_test.go`、Provider 现有测试文件 | 在既有测试文件中覆盖本章行为，不新增 `_test.go` 文件 |
| 修改 | `README.md`、`.mewcode/config.example.yaml`、`docs/README.md` | 用户说明、无配置默认行为和章节索引 |
| 新建 | `docs/ch11-skills/manual_scenarios.md`、`docs/ch11-skills/checklist.md` | 人工验证场景与验收清单 |

## T1: 定义 Skill 领域模型与内置样板

**文件：** `internal/skills/skill.go`、`internal/skills/builtins.go` 及内置样板资源

**依赖：** 无

**步骤：**

1. 定义 Skill、Metadata、Catalog、Activation、Snapshot、Runtime、执行模式及历史范围类型与默认值。
2. 定义名称、模式和历史范围的合法取值校验。
3. 将 `commit`、`review`、`test` 样板以最低优先级内置来源提供，正文使用可读的 SOP，元信息包含名称和说明；三个内置样板均采用 inline，特别是 `review` 必须保留当前对话中的需求和偏好。
4. 确保内置样板不依赖工作区文件且不包含密钥、用户参数或运行时结果。

**验证：** `go test ./internal/skills` 编译通过；在既有 `internal/agent/runner_test.go` 中断言三个样板被发现。

## T2: 实现发现、frontmatter 解析与优先级校验

**文件：** `internal/skills/discover.go`、现有 `internal/agent/runner_test.go`

**依赖：** T1

**步骤：**

1. 解析首行 YAML frontmatter 和非空 Markdown 正文，读取 `name`、`description`、`tools`、`mode`、`context`、`model`。
2. 扫描用户级和项目级目录中的单文件入口及目录型 `SKILL.md` 入口，并合并内置来源。
3. 按内置、用户级、项目级的优先级合并同名 Skill。
4. 校验名称格式、必填说明、模式/历史范围、重复标识、内置命令冲突和白名单中不存在的工具；错误包含来源路径与字段或标识。
5. 在既有测试文件中覆盖单文件、目录型、项目覆盖用户、非法 frontmatter、冲突和未知工具。

**验证：** `go test ./internal/agent -run SkillDiscover` 通过；错误场景不发起 Provider 请求。

## T3: 实现 Manager、激活状态与原子刷新

**文件：** `internal/skills/manager.go`、现有 `internal/agent/runner_test.go`

**依赖：** T1、T2

**步骤：**

1. 以互斥锁保护当前 Catalog 和激活列表，所有对外快照做深拷贝。
2. 实现按名称激活、清除激活、轻量目录输出和元信息查询。
3. 实现候选 Catalog 先发现/校验、后单次替换的 Reload；失败时不改动 Catalog 或激活列表。
4. 为并发读取和刷新提供稳定行为，不暴露内部 map 或 slice。
5. 在既有测试中覆盖多项激活、未知 Skill、清除、刷新成功和刷新失败后旧状态仍可用。

**验证：** `go test -race ./internal/agent -run 'Skill(Activation|Reload)'` 通过。

## T4: 实现 SOP 运行时渲染与 `load_skill` 工具

**文件：** `internal/skills/runtime.go`、`internal/skills/load_tool.go`、现有 `internal/agent/runner_test.go`

**依赖：** T3

**步骤：**

1. 根据 Snapshot 渲染按激活顺序排列的 SOP 段落，并只替换正文中的 `{{args}}`。
2. 对多个 Skill 工具白名单取交集；处理空白名单和互斥白名单；检测冲突的指定模型。
3. 实现只接受 Skill 名称的 `load_skill` 工具，调用 Manager 激活并仅返回名称、说明和模式等安全元数据。
4. 确保工具不回显 SOP 正文、参数或附属资源内容。
5. 在既有测试中覆盖参数替换、自动激活空参数、多 Skill、白名单交集、模型冲突和 LoadSkill 输出脱敏。

**验证：** `go test ./internal/agent -run 'Skill(Runtime|Load)'` 通过。

## T5: 增加工具 Registry 的受限视图

**文件：** `internal/tools/registry.go`、现有 `internal/tools/tools_test.go`

**依赖：** T4

**步骤：**

1. 提供稳定排序的工具名称查询接口。
2. 实现按允许名称生成的受限 Registry，复用原始 Tool 实例，不复制或改变工具元数据。
3. 对空白名单保留所有基础工具；对显式白名单仅保留其工具与系统级 `load_skill`。
4. 将未知要求视为错误，避免绕过启动或刷新校验。
5. 扩展既有测试验证 Definitions、Get 和执行路径均只看到受限工具。

**验证：** `go test ./internal/tools` 通过。

## T6: 注入轻量目录与已激活 SOP

**文件：** `internal/prompt/builder.go`、`internal/prompt/sections.go`、现有 `internal/prompt/prompt_test.go`

**依赖：** T3、T4

**步骤：**

1. 为 `OptionalModules` 增加可用 Skill 轻量目录字段。
2. 添加“可用 Skill”模块，仅渲染名称、说明与 LoadSkill 调用指引。
3. 调整“已激活 Skill”模块顺序，使完整 SOP 位于所有可选环境补充中最显眼的位置。
4. 保持无 Skill 时的既有 prompt 输出不变。
5. 扩展既有测试，断言初始 prompt 不含正文；激活后每轮都含完整 SOP；多 Skill 顺序稳定。

**验证：** `go test ./internal/prompt` 通过。

## T7: 支持 Provider 请求级模型覆盖

**文件：** `internal/provider/provider.go`、`internal/provider/anthropic/request.go`、`internal/provider/openai/request.go` 及既有 Provider 测试文件

**依赖：** 无

**步骤：**

1. 在 ChatRequest 增加可选模型字段，零值保持现有行为。
2. 令 Anthropic 和 OpenAI 请求编码优先使用请求模型，空值回退到客户端默认模型。
3. 扩展既有 Provider 测试验证两种协议的覆盖和回退。

**验证：** `go test ./internal/provider/...` 通过。

## T8: 将 Skill Runtime 接入主 Agent Loop

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`、现有 `internal/agent/runner_test.go`

**依赖：** T4、T5、T6、T7

**步骤：**

1. 在 Agent Request 定义显式 Skill Invocation，在 Options 注入 Skill Manager。
2. 每次模型调用前获取 Manager 快照，构造轻量目录、已激活 SOP、受限工具 Registry 和可选模型。
3. 让环境、工具定义、调度器和执行器使用同一受限 Registry；LoadSkill 工具激活后从下一轮重新取快照。
4. 保持 compact、plan、do、权限确认、取消、会话 journal 和 Token 统计既有语义；压缩及后台记忆继续使用默认模型。
5. 在关键生命周期添加仅含名称、状态、模式、数量和耗时的结构化日志。
6. 扩展既有 Runner 测试覆盖两阶段加载、白名单收窄、系统加载工具始终可见、持续注入、日志脱敏及无 Skill 回归。

**验证：** `go test ./internal/agent -run 'Runner.*Skill|TestRunnerFinalAnswer|TestRunnerToolLoop'` 通过。

## T9: 实现 fork 执行与摘要回流

**文件：** `internal/agent/runner.go`、现有 `internal/agent/runner_test.go`

**依赖：** T8

**步骤：**

1. 根据显式调用 Skill 的元信息在 inline 和 fork 之间分流。
2. 为 fork 创建无 journal 的临时会话，并按 `full`、`recent`、`none` 构造初始历史。
3. 在临时会话完成工具循环，不将中间 prompt、工具调用或回复写入主历史。
4. 将最终文本转为摘要，以合成请求和助手摘要提交主会话，并把实际用量累计至主会话。
5. 扩展既有测试验证三种历史范围、主历史隔离、摘要回流、取消/错误不产生伪摘要及 Token 累计。

**验证：** `go test ./internal/agent -run 'ForkSkill|SkillFork'` 通过。

## T10: 集成动态 Slash Command 与手动刷新

**文件：** `internal/command/command.go`、`internal/command/registry.go`、`internal/command/builtins.go`、现有 `internal/command/command_test.go`

**依赖：** T3、T8、T9

**步骤：**

1. 为 CommandContext 定义最小 Skill 服务接口，避免 command 依赖具体 Manager。
2. 组合内置命令、`/skills reload` 命令和当前 Catalog 的动态 Skill 命令，并复用重复命令校验。
3. 实现 `/skills reload`：刷新成功后返回新命令表所需信号；失败时保留既有命令表并显示诊断。
4. 将动态 Skill 命令映射为带名称和参数的 Agent Request，支持帮助和 Tab 补全。
5. 扩展既有测试覆盖三个样板的帮助/补全、显式参数、刷新新增/删除、刷新失败保持旧命令。

**验证：** `go test ./internal/command` 通过。

## T11: 在 TUI 与启动链路装配 Skill 服务

**文件：** `internal/tui/model.go`、`internal/tui/update.go`、`internal/tui/tui_test.go`、`cmd/mewcode/main.go`、现有 `cmd/mewcode/main_test.go`

**依赖：** T10

**步骤：**

1. 在基础工具与 MCP 工具全部注册后创建 Manager，并将项目根目录、用户配置目录、可用工具名和 Logger 传入。
2. 将 Manager 注入 Runner 与 TUI，启动时以当前 Catalog 构建命令 Registry。
3. 在 `/skills reload` 成功后原子替换 TUI 命令表，在失败时不改变表。
4. 在新建、恢复和清空会话的路径清除已激活 Skill，保留发现的命令。
5. 扩展既有 TUI 与 main 测试验证命令分流、刷新和会话切换清理。

**验证：** `go test ./cmd/mewcode ./internal/tui` 通过。

## T12: 更新用户文档、人工场景和索引

**文件：** `README.md`、`.mewcode/config.example.yaml`、`docs/README.md`、`docs/ch11-skills/manual_scenarios.md`

**依赖：** T11

**步骤：**

1. 说明项目级和用户级目录、单文件/目录型入口、frontmatter 字段、`{{args}}`、inline/fork 及工具白名单语义。
2. 说明自动发现、`load_skill`、显式 Slash Command 和 `/skills reload` 的用户可见行为。
3. 记录三个内置样板，并明确本章不提供市场、版本管理和自动文件监听。
4. 在配置示例说明 Skill 无需新增配置项及默认发现路径。
5. 新增人工场景覆盖首次发现、自动加载、参数替换、白名单、fork、刷新成功/失败、会话清理和日志脱敏。
6. 在 docs 索引加入第十一章 Spec、Plan、Tasks、Checklist、人工场景和理论学习稿链接。

**验证：** `rg -n 'skills reload|load_skill|ch11-skills' README.md .mewcode/config.example.yaml docs/README.md docs/ch11-skills/manual_scenarios.md` 输出预期说明。

## T13: 执行验收清单与全量回归

**文件：** `docs/ch11-skills/checklist.md`、所有本章修改文件

**依赖：** T1–T12

**步骤：**

1. 根据已批准 Spec 将所有 AC 转为可执行的 checklist 条目。
2. 运行格式化、目标包测试、全量测试和构建，记录实际证据。
3. 按人工场景完成至少一次端到端验证，记录通过或未通过及原因。
4. 检查 `bugs/`：若验证发现缺陷，按约定创建或更新记录；无缺陷则不创建无关记录。
5. 删除本轮为验证临时新增的任何 `_test.go` 文件，并确认保留的自动化覆盖仅修改既有测试文件。
6. 清理本轮生成且非任务直接相关的 `.mew/`、`logs/` 等运行产物，并确认未触及用户已有内容。

**验证：** `gofmt -w` 后 `go test ./...` 与 `go build ./cmd/mewcode` 均通过；`git status --short` 仅包含本章预期文件。

## 执行顺序

```text
T1 → T2 → T3 → T4 → T5 ─┐
                 T6 ───┼→ T8 → T9 → T10 → T11 → T12 → T13
T7 ─────────────────────┘
```
