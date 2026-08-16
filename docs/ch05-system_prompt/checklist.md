# MewCode System Prompt Checklist

> 每一项通过运行代码或观察行为来验证，聚焦系统行为。

## 实现完整性

- [ ] **AC1 固定模块顺序稳定**：系统提示包含身份、系统约束、任务模式、动作执行、工具使用、语气风格、文本输出、环境信息，并按该顺序输出；相同输入连续构建结果一致。（验证：运行 `go test ./internal/prompt -run 'ModuleOrder|BuildBundle'`，期望顺序和快照测试通过）
- [ ] **AC2 可选模块无空占位**：未提供自定义指令、Skill、长期记忆时不输出空标题；提供后按自定义指令、已激活 Skill、长期记忆顺序追加。（验证：运行 `go test ./internal/prompt -run OptionalModules`，期望空输入无标题、非空输入顺序正确）
- [ ] **AC3 稳定与动态分离**：仅环境日期、工作区或模式变化时，稳定系统段和稳定工具描述不变，动态系统消息变化。（验证：运行 `go test ./internal/prompt -run StableDynamicSplit`，期望 stable snapshot 不变、dynamic 内容变化）
- [ ] **AC6 环境信息完整**：普通任务首轮请求包含工作区路径、OS、Shell、日期、任务模式和可用工具范围；缺失必填字段会返回诊断错误。（验证：运行 `go test ./internal/prompt -run 'CollectEnvironment|EnvironmentRequired'`，期望字段齐全且错误分支通过）
- [x] **AC16 Unix Shell 优先不变**：`SHELL` 有值时，环境信息使用该值，即使同时存在 `COMSPEC` 也不改变。（验证：`go test ./internal/prompt -run 'TestCollectEnvironmentShellPriority' -count=1` 已通过）
- [x] **AC16 Windows Shell 回退可用**：`SHELL` 缺失但 `COMSPEC` 有值时，环境信息构建成功并使用 `COMSPEC`，随后可生成 `mew.environment` 系统消息。（验证：`go test ./internal/prompt -run 'TestCollectEnvironmentCOMSPECFallback' -count=1` 已通过）
- [x] **AC16 无平台变量不阻断请求**：`SHELL` 与 `COMSPEC` 同时缺失时，环境信息仍具有非空 Shell 描述，不返回 `prompt environment requires shell`。（验证：`go test ./internal/prompt -run 'TestCollectEnvironmentShellFallback' -count=1` 已通过）
- [ ] **AC7 工具规则双重强化**：全局工具使用模块和各工具描述均包含关键工具规则，增强后工具定义保持输入顺序且 `Cacheable=true`。（验证：运行 `go test ./internal/prompt -run 'ToolRulesStable|EnhanceDefinitions'`，期望规则文本和顺序断言通过）
- [ ] **AC8 模式注入频率**：同一 Agent 任务第 1 轮完整注入，间隔轮次注入提醒，其余轮次精简或不注入。（验证：运行 `go test ./internal/prompt -run ModeInjection`，期望请求序列与策略一致）

## Provider 映射

- [ ] **AC4 Provider 中立请求**：Agent 只传递 `provider.PromptBundle`，Provider 专有字段只出现在 OpenAI/Anthropic 子包。（验证：运行 `rg -n 'cache_control|cached_tokens|instructions' internal/agent internal/prompt`，期望无 Provider 专有请求字段匹配）
- [ ] **AC4 OpenAI 请求映射**：OpenAI 请求体中稳定系统指令位于动态补充和用户任务之前，工具定义顺序稳定。（验证：运行 `go test ./internal/provider/openai -run 'Request|Prompt|System'`，期望请求快照通过）
- [ ] **AC4 Anthropic 请求映射**：Anthropic 请求体按 tools、system、messages 的缓存友好顺序组织，动态 system 不带缓存控制。（验证：运行 `go test ./internal/provider/anthropic -run 'Request|Prompt|Cache'`，期望请求快照通过）
- [ ] **AC12 缓存段位置可观察**：连续两次相同稳定提示的请求中，可缓存稳定段位置不变，动态环境变化不破坏稳定段快照。（验证：运行 `go test ./internal/provider/openai ./internal/provider/anthropic -run 'Cache|Prompt'`，期望缓存标记和快照断言通过）

## 模式与历史隔离

- [ ] **AC5 系统补充不污染展示**：动态系统补充进入模型请求，但不出现在 TUI 用户消息展示和共享会话历史中。（验证：运行 `go test ./internal/tui ./internal/agent -run 'Display|SystemPrompt'`，期望 display/history 中无 `mew.environment` 或 `mew.mode` 标签）
- [ ] **AC9 Plan Mode 用户任务纯净**：`/plan <任务>` 的模型用户消息只包含任务本身，只读探索和最终计划要求位于系统级动态补充消息。（验证：运行 `go test ./internal/agent -run 'PlanPrompt|PlanTaskHistory'`，期望请求消息和历史隔离断言通过）
- [ ] **AC10 Do Mode 不含规划只读约束**：多个待执行计划执行 `/do` 时，请求按顺序携带全部计划，且不包含 Plan Mode 只读补充。（验证：运行 `go test ./internal/agent -run 'DoExcludesPlanInternalHistory|DoUsesFullRegistry'`，期望计划完整且无只读污染）
- [ ] **AC14 工具安全边界不变**：Plan Mode 仍只开放只读工具，普通执行和 `/do` 仍按第 4 章工具调度执行。（验证：运行 `go test ./internal/agent -run 'PlanToolDefinitions|Scheduler|DoUsesFullRegistry'`，期望全部通过）

## 缓存用量

- [ ] **AC11 OpenAI 缓存用量解析**：OpenAI 响应包含 cached tokens 时，统一 Usage 暴露缓存读取 token；无字段时任务仍正常完成。（验证：运行 `go test ./internal/provider/openai -run 'Usage|CachedTokens'`，期望缓存字段和无字段分支通过）
- [ ] **AC11 Anthropic 缓存用量解析**：Anthropic 响应包含 cache creation/read 字段时，统一 Usage 暴露缓存创建和读取 token；5m/1h 写入细分被汇总。（验证：运行 `go test ./internal/provider/anthropic -run 'Usage|Cache'`，期望缓存字段映射通过）
- [ ] **AC15 Usage 汇总正确**：Agent 多轮任务的 Summary 用量累加普通 token 与缓存 token，不因 Provider 未报告缓存字段失败。（验证：运行 `go test ./internal/agent -run 'Usage|Token'`，期望累计值断言通过）

## 人工对比场景

- [ ] **AC13 人工场景文档完整**：人工场景文档包含编辑前读取、搜索优先、规划模式只读、`/do` 不受只读污染、环境信息正确使用五类场景。（验证：运行 `rg -n '编辑前读取|搜索优先|规划模式|/do|环境信息' docs/ch05-system_prompt/manual_scenarios.md`，期望五类场景均匹配）
- [ ] **AC13 场景可执行观察**：每个人工场景都写明输入、观察点、期望行为和失败信号。（验证：人工 review `docs/ch05-system_prompt/manual_scenarios.md`，期望每个场景四项齐全）

## 编译与测试

- [ ] **AC15 Prompt 包离线测试**：提示词构建、动态注入、工具增强和环境收集测试无需真实模型服务即可运行。（验证：运行 `go test ./internal/prompt`，期望通过）
- [ ] **AC15 Provider 包离线测试**：OpenAI 与 Anthropic 请求映射和 usage 解析测试无需访问真实 API 即可运行。（验证：运行 `go test ./internal/provider/...`，期望通过）
- [ ] **AC14 Agent/TUI 回归测试**：Agent Loop、Plan Mode、Do Mode、历史隔离和 TUI 展示回归测试通过。（验证：运行 `go test ./internal/agent ./internal/tui`，期望通过）
- [ ] **AC14 全项目测试**：第 2 章到第 5 章相关改动未破坏现有功能。（验证：运行 `go test ./...`，期望通过）
- [x] **Release Windows 编译**：无 CGO 依赖时可从当前开发机生成 Windows 64 位发布产物。（验证：`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /private/tmp/mewcode-windows-amd64.exe ./cmd/mewcode` 已通过；验证产物已清理）

## 端到端场景

- [ ] **E2E 1：普通任务提示注入**：提交普通任务后，脚本化 Provider 观察到稳定系统提示、动态环境信息、增强工具定义和用户任务均位于预期位置，任务最终正常完成。（验证：运行 Agent 端到端测试，期望 Provider 捕获请求符合快照且终态为 completed）
- [ ] **E2E 2：规划后执行不污染**：先执行 `/plan 创建文件`，再执行 `/do`；规划请求只开放只读工具且用户任务纯净，执行请求恢复完整工具且不包含规划只读补充。（验证：运行脚本化 Provider 端到端测试，期望请求历史、工具集合和 pending plan 消费符合预期）
- [ ] **E2E 3：缓存用量可观测**：脚本化 Provider 返回两轮带缓存字段的 usage，Agent 终态 Summary 显示累计缓存读取/创建 token。（验证：运行 Agent usage 端到端测试，期望 Summary.Usage 与两轮字段之和一致）
