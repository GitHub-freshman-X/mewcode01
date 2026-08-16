# MewCode System Prompt Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/prompt/module.go` | Module、ModuleKey、排序与稳定渲染 |
| 新建 | `internal/prompt/sections.go` | 七个固定模块与可选模块构造 |
| 新建 | `internal/prompt/environment.go` | Environment、环境收集与环境消息渲染 |
| 新建 | `internal/prompt/mode.go` | Mode、ModeInjection、InjectionPolicy |
| 新建 | `internal/prompt/builder.go` | BuildContext、BuildBundle、缓存策略生成 |
| 新建 | `internal/prompt/tools.go` | 工具描述增强 |
| 新建 | `internal/prompt/clock.go` | Clock 接口与系统 clock |
| 新建 | `internal/prompt/prompt_test.go` | 提示词模块、动态隔离、模式注入和工具增强测试 |
| 新建 | `internal/provider/prompt.go` | PromptBundle、SystemMessage、CachePolicy |
| 修改 | `internal/provider/provider.go` | ChatRequest、ToolDefinition 扩展 |
| 修改 | `internal/provider/event.go` | Usage 缓存字段与 Add 汇总 |
| 修改 | `internal/provider/openai/request.go` | OpenAI 系统提示、动态补充和工具缓存映射 |
| 修改 | `internal/provider/openai/stream.go` | OpenAI cached_tokens 解析 |
| 修改 | `internal/provider/openai/openai_test.go` | OpenAI 请求快照与缓存 usage 测试 |
| 修改 | `internal/provider/anthropic/request.go` | Anthropic system blocks 与 cache_control 映射 |
| 修改 | `internal/provider/anthropic/stream.go` | Anthropic cache usage 解析 |
| 修改 | `internal/provider/anthropic/anthropic_test.go` | Anthropic 请求快照与缓存 usage 测试 |
| 修改 | `internal/agent/mode.go` | `/plan` 用户提示迁移、`/do` 文本整理、mode 转换辅助 |
| 修改 | `internal/agent/runner.go` | 每轮构建 PromptBundle 与增强工具定义 |
| 修改 | `internal/agent/runner_test.go` | Plan/Do 注入隔离、环境注入和回归测试 |
| 修改 | `internal/tui/tui_test.go` | 展示历史不含系统补充消息 |
| 新建 | `docs/ch05-system_prompt/manual_scenarios.md` | 人工对比场景 |
| 修改 | `docs/README.md` | ch05 文档索引补齐 |
| 修改 | `internal/prompt/environment.go` | 跨平台 Shell 推导与安全回退 |
| 修改 | `internal/prompt/prompt_test.go` | 缺失 `SHELL` 的 Windows 兼容性回归测试 |
| 修改 | `docs/ch05-system_prompt/{spec,plan,task,checklist}.md` | 记录 Windows Shell 兼容性需求、设计、任务和验收 |
| 修改 | `bugs/2026-08-16/005-windows-shell-environment-required.md` | 记录修复方案与验证结果 |

## T1: 定义 Provider 中立提示词承载类型

**文件：** `internal/provider/prompt.go`、`internal/provider/provider.go`
**依赖：** 无
**步骤：**
1. 新建 `PromptBundle`，包含 `StableSystem`、`DynamicSystem`、`CachePolicy`。
2. 新建 `SystemMessage`，包含 `Tag`、`Content`、`Cacheable`。
3. 新建 `CachePolicy`，包含 `Enable`、`StableSystem`、`StableTools`、`DynamicMessages`。
4. 在 `ChatRequest` 增加 `Prompt PromptBundle`。
5. 在 `ToolDefinition` 增加 `Cacheable bool`。

**验证：** `go test ./internal/provider/...` 编译通过。

## T2: 扩展统一 Usage 缓存字段

**文件：** `internal/provider/event.go`
**依赖：** 无
**步骤：**
1. 在 `Usage` 增加 `CacheReadInputTokens`、`CacheCreationInputTokens`、`CacheUnavailable`。
2. 更新 `Usage.Add`，累加缓存读写 token，并在任一侧不可用时保留不可用状态。
3. 保持现有 `InputTokens`、`OutputTokens` 行为不变。

**验证：** `go test ./internal/provider/... ./internal/agent -run 'Usage|Token'` 通过。

## T3: 建立 prompt 模块基础类型

**文件：** `internal/prompt/module.go`、`internal/prompt/clock.go`
**依赖：** T1
**步骤：**
1. 定义 `Module`、`ModuleKey` 和固定模块 key。
2. 定义 `Mode` 独立枚举：`act`、`plan`、`do`。
3. 实现模块排序与渲染辅助，模块之间使用单个空行分隔。
4. 定义 `Clock` 接口和系统 clock。

**验证：** `go test ./internal/prompt -run 'Module|Clock'` 编译通过并通过模块排序测试。

## T4: 实现固定模块与可选模块构造

**文件：** `internal/prompt/sections.go`、`internal/prompt/prompt_test.go`
**依赖：** T3
**步骤：**
1. 实现身份、系统约束、任务模式、动作执行、工具使用、语气风格、文本输出七个固定模块。
2. 实现自定义指令、已激活 Skill、长期记忆三个可选模块构造。
3. 可选模块内容为空时不输出标题或空段落。
4. 添加测试验证固定模块顺序和可选模块顺序。

**验证：** `go test ./internal/prompt -run 'ModuleOrder|OptionalModules'` 通过。

## T5: 实现环境信息收集与渲染

**文件：** `internal/prompt/environment.go`、`internal/prompt/prompt_test.go`
**依赖：** T3
**步骤：**
1. 定义 `Environment`。
2. 实现 `CollectEnvironment(mode Mode, registry *tools.Registry, workspace string, clock Clock)`。
3. 收集工作区路径、OS、Shell、日期、模式和当前可用工具名。
4. 缺少 workspace、registry、Shell 或工具范围时返回可诊断错误。
5. 将环境渲染为 `provider.SystemMessage{Tag:"mew.environment"}`。

**验证：** `go test ./internal/prompt -run 'Environment|CollectEnvironment'` 通过。

## T6: 实现模式补充注入策略

**文件：** `internal/prompt/mode.go`、`internal/prompt/prompt_test.go`
**依赖：** T3
**步骤：**
1. 定义 `InjectionKind`、`ModeInjection`、`InjectionPolicy`。
2. 实现 `ModeInjectionFor`。
3. 首轮输出完整模式说明。
4. 达到 `ReminderEvery` 时输出关键规则提醒。
5. 其他轮次按 `BriefEvery` 输出精简提醒或不输出。
6. 为 act、plan、do 三种模式分别提供文本。

**验证：** `go test ./internal/prompt -run 'ModeInjection'` 通过。

## T7: 实现 Prompt Bundle 构建

**文件：** `internal/prompt/builder.go`、`internal/prompt/prompt_test.go`
**依赖：** T4、T5、T6
**步骤：**
1. 定义 `BuildContext` 和 `OptionalModules`。
2. 实现 `BuildBundle(ctx BuildContext) (provider.PromptBundle, []Module, error)`。
3. 将稳定固定模块和稳定可选模块拼入 `StableSystem`。
4. 将环境信息和模式注入放入 `DynamicSystem`。
5. 默认生成启用稳定系统与稳定工具缓存、不缓存动态消息的 `CachePolicy`。
6. 添加动态字段变化不改变稳定段的测试。

**验证：** `go test ./internal/prompt -run 'BuildBundle|StableDynamicSplit'` 通过。

## T8: 实现工具描述增强

**文件：** `internal/prompt/tools.go`、`internal/prompt/prompt_test.go`
**依赖：** T1、T3
**步骤：**
1. 实现 `EnhanceDefinitions(defs []provider.ToolDefinition, mode Mode)`。
2. 为所有工具追加稳定的通用工具规则。
3. 为读、写、改、执行命令、找文件、搜索代码追加对应关键规则。
4. Plan Mode 下追加不得请求副作用工具的稳定提醒。
5. 设置增强后工具的 `Cacheable=true`。
6. 保持输入顺序不变。

**验证：** `go test ./internal/prompt -run 'EnhanceDefinitions|ToolRulesStable'` 通过。

## T9: 接入 Agent 请求准备语义

**文件：** `internal/agent/mode.go`、`internal/agent/runner_test.go`
**依赖：** T3
**步骤：**
1. 增加 `agent.Mode` 到 `prompt.Mode` 的转换辅助。
2. 修改 Plan Mode 的 `prepareRequest`，用户 prompt 只保留原始任务。
3. 保留 `displayPrompt="/plan <任务>"` 和只读 registry 过滤。
4. 整理 Do Mode prompt，仅保留全部计划及执行任务语义，不重复长期工具规则。
5. 添加测试验证 `/plan` 请求用户文本不含只读系统约束，`/do` 仍包含所有计划。

**验证：** `go test ./internal/agent -run 'PrepareRequest|Plan|Do'` 通过。

## T10: 在 Runner 每轮构建 PromptBundle

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`
**依赖：** T5、T7、T8、T9
**步骤：**
1. 为 Runner options 增加 workspace、clock、InjectionPolicy 的默认配置。
2. 在每轮 Provider 调用前收集环境。
3. 调用 `BuildBundle` 生成 prompt bundle。
4. 调用 `EnhanceDefinitions` 生成增强工具定义。
5. 将 `Prompt` 和增强工具传入 `provider.ChatRequest`。
6. 确保 Prompt 不进入 Session 历史或 Display 历史。

**验证：** `go test ./internal/agent -run 'PromptBundle|PlanTaskHistory|DoExcludesPlanInternalHistory'` 通过。

## T11: 映射 OpenAI 请求体

**文件：** `internal/provider/openai/request.go`、`internal/provider/openai/openai_test.go`
**依赖：** T1、T10
**步骤：**
1. 扩展 OpenAI request body，支持稳定系统指令位置。
2. 将 `Prompt.StableSystem` 放在请求最前面。
3. 将 `Prompt.DynamicSystem` 放在用户任务前，优先使用 system/developer 语义；必要时渲染为带标签的系统补充文本。
4. 保持工具定义稳定顺序。
5. 添加请求快照测试，验证稳定段、动态段和用户任务顺序。

**验证：** `go test ./internal/provider/openai -run 'Request|Prompt|System'` 通过。

## T12: 映射 Anthropic 请求体

**文件：** `internal/provider/anthropic/request.go`、`internal/provider/anthropic/anthropic_test.go`
**依赖：** T1、T10
**步骤：**
1. 扩展 Anthropic request body 的 system block 表达。
2. 将 `Prompt.StableSystem` 放入 top-level system blocks。
3. 将 `Prompt.DynamicSystem` 作为后续 system blocks 或带标签系统文本。
4. 按 `CachePolicy` 给稳定 system 或稳定工具添加 `cache_control`。
5. 确保动态 system 不带缓存控制。
6. 添加请求快照测试，验证 tools、system、messages 顺序。

**验证：** `go test ./internal/provider/anthropic -run 'Request|Prompt|Cache'` 通过。

## T13: 解析 OpenAI 缓存用量

**文件：** `internal/provider/openai/stream.go`、`internal/provider/openai/openai_test.go`
**依赖：** T2
**步骤：**
1. 扩展 OpenAI usage 结构以读取 `input_tokens_details.cached_tokens` 或当前 API 返回的等效字段。
2. 将 cached tokens 映射到 `Usage.CacheReadInputTokens`。
3. 无缓存字段时设置不可用或保持零值，并保证任务完成事件仍正常。
4. 添加 stream 解析测试。

**验证：** `go test ./internal/provider/openai -run 'Usage|CachedTokens'` 通过。

## T14: 解析 Anthropic 缓存用量

**文件：** `internal/provider/anthropic/stream.go`、`internal/provider/anthropic/anthropic_test.go`
**依赖：** T2
**步骤：**
1. 扩展 Anthropic usage 结构以读取 `cache_creation_input_tokens` 和 `cache_read_input_tokens`。
2. 兼容可能出现的 5m/1h 写入细分，并汇总到创建字段。
3. 将缓存字段映射到统一 `Usage`。
4. 添加 message_start/message_delta usage 测试。

**验证：** `go test ./internal/provider/anthropic -run 'Usage|Cache'` 通过。

## T15: 保持 TUI 展示隔离

**文件：** `internal/tui/tui_test.go`、`internal/agent/runner_test.go`
**依赖：** T10
**步骤：**
1. 添加测试确认系统级动态补充不出现在 display history。
2. 确认 `/plan <任务>` 仍按 displayPrompt 展示。
3. 确认共享模型历史不包含 `mew.environment`、`mew.mode.plan` 等系统标签。

**验证：** `go test ./internal/tui ./internal/agent -run 'Display|SystemPrompt|Plan'` 通过。

## T16: 编写人工对比场景文档

**文件：** `docs/ch05-system_prompt/manual_scenarios.md`
**依赖：** 无
**步骤：**
1. 新建人工场景文档。
2. 写入编辑前读取场景。
3. 写入搜索优先使用搜索工具场景。
4. 写入规划模式只读场景。
5. 写入 `/do` 不受只读规划污染场景。
6. 写入环境信息正确使用场景。
7. 每个场景包含输入、观察点、期望行为、失败信号。

**验证：** `rg -n '编辑前读取|搜索优先|规划模式|/do|环境信息' docs/ch05-system_prompt/manual_scenarios.md` 均有匹配。

## T17: 更新文档索引

**文件：** `docs/README.md`
**依赖：** T16
**步骤：**
1. 将 ch05 链接扩展为 Spec、Plan、Tasks、Checklist。
2. 先保留 Checklist 链接，待阶段四生成文件后可用。

**验证：** `rg -n 'ch05-system_prompt.*Spec.*Plan.*Tasks.*Checklist' docs/README.md` 有匹配。

## T18: 回归与集成验证

**文件：** 全项目
**依赖：** T1-T17
**步骤：**
1. 运行 prompt 包测试。
2. 运行 provider 包测试。
3. 运行 agent 包测试。
4. 运行 tui 包测试。
5. 运行全项目测试。
6. 修复因提示词请求模型扩展导致的编译或回归问题。

**验证：** `go test ./...` 通过。

## T19: 修复跨平台 Shell 环境采集

**文件：** `internal/prompt/environment.go`、`internal/prompt/prompt_test.go`
**依赖：** 无
**步骤：**
1. 保持 `SHELL` 为首选值，确保 Unix 现有输出不变。
2. `SHELL` 缺失时读取 `COMSPEC`，兼容 Windows PowerShell 与 CMD 的默认环境。
3. 两者均缺失时使用稳定、非空的环境描述回退值，不再中断 Agent 请求。
4. 添加单元测试，分别锁定 `SHELL` 优先、`COMSPEC` 回退与双变量缺失回退。
5. 构建环境系统消息，断言缺失 `SHELL` 时不再产生 `prompt environment requires shell`。

**验证：** `go test ./internal/prompt -run 'CollectEnvironment|EnvironmentMessage' -count=1` 通过；`env -u SHELL go test ./internal/prompt -run '^TestCollectEnvironment$' -count=1` 通过。

## T20: 同步缺陷记录并执行完整验证

**文件：** `bugs/2026-08-16/005-windows-shell-environment-required.md`、`bugs/2026-08-16/README.md`
**依赖：** T19
**步骤：**
1. 将缺陷状态更新为已修复，保留真实 Windows 模型请求验证为待办项。
2. 写入最小复现、修复方案与已执行的自动化验证命令。
3. 运行完整测试与 Windows 交叉编译，确认发布产物可生成。

**验证：** `go test ./...` 和 `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/mewcode` 通过。

## 执行顺序

```text
T1 -> T2
T1 -> T3 -> T4 -> T5 -> T6 -> T7 -> T8
T3 -> T9 -> T10
T10 -> T11 -> T13
T10 -> T12 -> T14
T10 -> T15
T16 -> T17
T1..T17 -> T18
T19 -> T20
```
