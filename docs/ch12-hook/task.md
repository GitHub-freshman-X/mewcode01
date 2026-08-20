# MewCode Hook 生命周期钩子 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/hooks/types.go`、`condition.go` | Hook 领域模型、上下文变量、条件解析与匹配 |
| 新建 | `internal/hooks/config.go` | 三层 YAML 读取、追加合并与加载期校验 |
| 新建 | `internal/hooks/executor.go`、`engine.go` | 四类动作、日志、一次执行、异步与工具前拒绝 |
| 新建 | `internal/hooks/*_test.go` | Hook 包行为与配置的自动化测试 |
| 修改 | `internal/agent/event.go`、`runner.go` | Hook 注入、通知保存和非工具生命周期事件 |
| 修改 | `internal/agent/scheduler.go` | 工具前拦截、拒绝结果回灌和工具后事件 |
| 修改 | `internal/agent/runner_test.go`、`scheduler_test.go` | 生命周期、拒绝、提示词和故障隔离集成测试 |
| 修改 | `cmd/mewcode/main.go`、`main_test.go` | Hook 路径、加载器与 Engine 组合根 |
| 修改 | `README.md`、`.mewcode/config.example.yaml`、`docs/README.md` | 用户配置说明、完整示例与章节索引 |
| 新建 | `docs/ch12-hook/manual_scenarios.md`、`checklist.md` | 人工场景与验收清单 |

## T1: 定义 Hook 领域模型和上下文变量

**文件：** `internal/hooks/types.go`、`internal/hooks/types_test.go`

**依赖：** 无

**步骤：**

1. 定义完整生命周期事件集合、动作类型、规则、动作、运行结果和可注入动作依赖。
2. 定义事件合法性与工具前事件的专属约束。
3. 实现事件上下文字段访问，以及 `$EVENT`、`$TOOL_NAME`、`$FILE_PATH`、`$MESSAGE`、`$ERROR`、`$TOOL_ARGS.<name>` 模板替换。
4. 确保未知字段和不存在变量稳定地返回空字符串，且不修改原始工具参数。
5. 为所有对模型或日志可见的结果文本保留安全摘要边界，不保存密钥、请求头或完整外部载荷。

**验证：** `go test ./internal/hooks -run 'Test(Context|Event)'` 通过。

## T2: 实现条件解析、校验与匹配

**文件：** `internal/hooks/condition.go`、`internal/hooks/condition_test.go`

**依赖：** T1

**步骤：**

1. 解析无条件、单条件、全满足和任一满足表达式，生成预校验的条件组。
2. 实现 `==`、`!=`、`=~`、`~=` 对事件上下文的精确、反向、正则和 glob 匹配。
3. 在加载期校验正则、glob、字段/操作符格式和空表达式。
4. 明确拒绝同一表达式混用 `&&` 和 `||`。
5. 测试每类操作符、参数字段、未知字段、组合短路和所有非法输入。

**验证：** `go test ./internal/hooks -run TestCondition` 通过。

## T3: 实现三层配置加载与集中校验

**文件：** `internal/hooks/config.go`、`internal/hooks/config_test.go`

**依赖：** T1、T2

**步骤：**

1. 解析用户级 `~/.mewcode/config.yaml`、项目级 `.mewcode/config.yaml`、本地级 `.mewcode/config.local.yaml` 中的 `hooks` 序列；缺失文件或字段视为空。
2. 按用户、项目、本地顺序追加规则并分配稳定的默认标识和声明顺序。
3. 校验事件、动作类型、`reject`/`async` 约束，以及 command、prompt、http、agent 的必填字段和 command 超时格式。
4. 将条件解析错误包装为包含配置文件、规则标识或序号和字段的可诊断错误。
5. 测试三来源合并、缺失文件、无条件规则、每类合法动作和全部非法配置场景。

**验证：** `go test ./internal/hooks -run 'Test(LoadRuleSet|Validate)'` 通过。

## T4: 实现四种动作执行器与安全日志

**文件：** `internal/hooks/executor.go`、`internal/hooks/executor_test.go`

**依赖：** T1

**步骤：**

1. 实现 command 动作的变量展开、退出状态/输出摘要采集和带上下文取消的超时终止。
2. 通过注入的通知接收器实现 prompt 动作，不直接改写 System Prompt。
3. 通过可替换 HTTP Client 实现 HTTP 方法、头和请求体发送；只把状态码、耗时和大小等安全元数据交给日志。
4. 定义 `AgentRunner` 接口和默认占位执行器，明确返回“运行时未接入”而不是启动真实子 Agent。
5. 为成功、退出失败、超时、HTTP 成功/失败、提示词通知、Agent 占位与日志脱敏建立测试；所有外部交互使用测试进程或本地测试服务。

**验证：** `go test ./internal/hooks -run TestExecutor` 通过。

## T5: 实现 Hook 分发、一次执行、异步隔离与拒绝短路

**文件：** `internal/hooks/engine.go`、`internal/hooks/engine_test.go`

**依赖：** T2、T3、T4

**步骤：**

1. 构造保持合并后声明顺序的 Engine，并只向匹配规则传递对应事件上下文。
2. 实现非工具前事件的同步分发；动作失败只写安全日志并继续后续规则。
3. 实现本进程内一次执行标记；确保并发触发不会重复执行同一 once 规则，重建 Engine 后状态重置。
4. 实现后台异步动作，确保调度不等待其完成且失败不会逃逸到 Agent 主流程。
5. 实现 `RunPreTool`：同步顺序执行、首个拒绝短路、返回简短拒绝原因；非拒绝失败不取消工具。
6. 测试顺序、无条件匹配、once、异步非阻塞、失败隔离、拒绝短路和并发安全。

**验证：** `go test -race ./internal/hooks -run TestEngine` 通过。

## T6: 将提示词通知与非工具生命周期接入 Runner

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T5

**步骤：**

1. 在 Agent Options 中注入 Hook Engine 与线程安全的 prompt 通知收集器，零值保持现有行为。
2. 在会话、轮次、模型发送前、模型接收后、上下文压缩、错误及正常/取消/失败结束节点映射并运行对应 Hook。
3. 将 prompt 动作输出包成普通 Hook 通知，追加到下一次模型请求可见的消息历史，不改变 `prompt.BuildBundle` 的 System Prompt 内容。
4. 让通知遵循现有历史提交和压缩路径，避免在 Plan 临时历史、取消或失败时产生半轮提交。
5. 扩展 Runner 测试覆盖事件时机、通知可见性、System Prompt 不变、压缩兼容、无 Hook 回归和失败隔离。

**验证：** `go test ./internal/agent -run 'TestRunner.*Hook|TestRunnerFinalAnswer|TestRunnerToolLoop'` 通过。

## T7: 在 Scheduler 接入工具前拦截与工具后事件

**文件：** `internal/agent/scheduler.go`、`internal/agent/scheduler_test.go`

**依赖：** T5、T6

**步骤：**

1. 为 Scheduler 注入 Hook Engine 和构造工具 Hook 上下文所需的调用参数与安全文件路径信息。
2. 在权限决策和真实执行器之前同步调用工具前 Hook；拒绝时创建与现有失败语义一致的标准工具结果，且不调用权限或真实工具。
3. 对未拒绝调用保持既有权限判断、确认、并发批次和结果原始顺序。
4. 在真实工具或拒绝结果生成后触发工具后 Hook，并保证其失败不影响结果回灌。
5. 映射权限请求、文件变更及命令执行等可由现有执行路径可靠观察到的事件。
6. 测试真实工具零执行、拒绝原因回灌、模型下一轮调整、多个规则短路、权限不可被放行、只读并发调度与工具后失败隔离。

**验证：** `go test ./internal/agent -run 'TestScheduler.*Hook|TestRunner.*HookReject'` 通过。

## T8: 在启动链路加载并装配 Hook

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** T3、T5、T6、T7

**步骤：**

1. 在解析用户配置目录和工作区后构造 Hook 三层路径，加载并校验规则。
2. 使用同一 Logger、HTTP Client 与 prompt 通知接收器构造 Engine，并注入 Runner/Scheduler 所需 Options。
3. 在启动和退出生命周期运行对应 Hook，并让加载错误阻止启动且显示规则来源与字段。
4. 对现有 Slash Command 分流加入可观测的 command 执行 Hook，保持原命令处理结果不变。
5. 扩展 main 测试覆盖无规则启动、三层加载、非法规则启动失败和不泄露敏感字段的日志。

**验证：** `go test ./cmd/mewcode` 通过。

## T9: 更新配置示例、README、索引与人工场景

**文件：** `.mewcode/config.example.yaml`、`README.md`、`docs/README.md`、`docs/ch12-hook/manual_scenarios.md`

**依赖：** T8

**步骤：**

1. 在配置示例中记录全部 `hooks` 配置项及实际默认值，展示格式化、工具前拒绝、会话提示和异步 HTTP 通知示例。
2. 在 README 说明三份配置位置、追加顺序、三要素、四种动作、变量、一次/异步/超时限制与既有权限边界。
3. 在文档索引加入第 12 章 Spec、Plan、Tasks、Checklist、人工场景与理论学习稿链接。
4. 新增人工场景，覆盖格式化、危险调用拦截、首次提示词、HTTP 失败不阻断、配置错误和日志脱敏。
5. 不在示例中写入真实 webhook、密钥、请求头或会泄露用户信息的命令。

**验证：** `rg -n 'hooks:|pre_tool_use|post_tool_use|ch12-hook' README.md .mewcode/config.example.yaml docs/README.md docs/ch12-hook/manual_scenarios.md` 输出预期说明。

## T10: 执行验收清单与全量回归

**文件：** `docs/ch12-hook/checklist.md`、所有本章修改文件

**依赖：** T1–T9

**步骤：**

1. 根据已批准 Spec 将 AC1–AC10 转为可观测的 checklist 条目。
2. 运行格式化、Hook 包测试、Agent/启动链路测试、全量测试和构建，记录实际证据。
3. 按人工场景至少完成一条“工具前拒绝后模型调整”和一条“异步动作失败不阻断”的端到端验证。
4. 每次修改或执行诊断、测试命令后检查 `bugs/`；若发现实际缺陷，按目录约定创建或更新记录；无缺陷不创建无关记录。
5. 清理本轮运行产生且不属于任务产物的 `.mew/`、`logs/` 等目录，并确认不触及用户已有内容。

**验证：** `gofmt -w` 后，`go test ./...` 与 `go build ./cmd/mewcode` 均通过；`git status --short` 仅包含本章预期改动。

## 执行顺序

```text
T1 → T2 → T3 ─┐
      T4 ────┼→ T5 → T6 → T7 → T8 → T9 → T10
              │
              └────────────────────────────────────
```
