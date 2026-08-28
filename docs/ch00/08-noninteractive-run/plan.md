# MewCode 非交互式单任务执行 Plan

## 架构概览

在既有 `mewcode` 可执行文件中增加 `run` 子命令。顶层 CLI 先识别子命令：非 `run` 输入继续走现有 TUI 启动路径；`run` 将解析其专属参数、构建共享运行时，并直接消费一次 `agent.Runner` 事件流。

共享运行时初始化继续负责配置、工作区、Provider、指令、记忆索引、工具、MCP、Hook、权限、Skill、Worktree 与日志。通过启动选项区分交互式持久会话和非交互式临时会话，避免复制当前初始化逻辑或让两种入口的运行环境逐渐偏离。

## 核心数据结构

### `runOptions`

`cmd/mewcode` 内部的非交互命令参数：任务文本或任务文件、配置路径、总超时、JSON 输出开关。解析后仅保留已校验的非空任务文本和规范化超时。

### `runResult`

非交互任务的最终结果，JSON 字段固定为：

- `status`：`completed`、`failed`、`cancelled`、`timed_out` 或 `stopped`。
- `stop_reason`：Agent 的停止原因；启动阶段失败时为空。
- `error`：安全的错误摘要；无错误时为空。
- `final_text`：全部文本增量按到达顺序拼接得到的最终文本。
- `elapsed_ms`：从成功启动任务到终态的毫秒数。
- `iterations`：终态摘要的迭代数。
- `usage`：终态摘要的 Token 用量。

### `runtimeOptions`

共享启动过程的内部选项，至少表达是否持久化会话、是否启用长期记忆写入、是否配置 TUI 权限确认桥，以及是否允许子 Agent 转入后台。

## 模块设计

### `cmd/mewcode/main.go`

**职责：** 识别 `run` 子命令，同时保持旧参数语法和 TUI 路径不变；将当前大段启动流程提取为可按 `runtimeOptions` 配置的共享构建过程。

**对外接口：** 保持 `main()` 与现有 `run(args, stderr)` 测试入口；增加可注入标准输出的非交互执行函数，供离线测试捕获输出。

**非交互运行时选择：**

- 使用 `conversation.NewSession()`，不创建或持久化会话元数据，且不给 Runner 传入 session store 或 session ID。
- 不设置长期记忆服务，从而不触发提取和治理；仍加载记忆索引作为系统提示的只读输入。
- 不创建 `tui.PermissionBridge`，使 Scheduler 对 `ActionAsk` 直接返回既有拒绝工具结果。
- 共享已有安全日志、Hook、Skill、MCP、指令、权限与 Worktree 初始化。

### `cmd/mewcode/run.go`

**职责：** 提供非交互 CLI 的参数解析、任务事件消费、信号/超时控制、文本输出、JSON 序列化与退出码映射。

**对外接口：** 内部 `runNonInteractive` 接收已解析参数、共享运行时、标准输出与标准错误，返回进程退出码。

**执行流程：**

1. 用独立的 FlagSet 解析 `run --config`、`--prompt`、`--prompt-file`、`--timeout` 和 `--json`。
2. 校验互斥输入并读取任务文件；读取或校验失败返回退出码 1，且不启动模型。
3. 以可取消的根上下文启动 `ModeAct`；默认附加 30 分钟 deadline，`--timeout 0` 不附加 deadline；捕获中断信号并调用取消。
4. 消费所有 Agent 事件，按顺序累积文本增量与最后一个终态；默认模式立即把每段文本写入标准输出，JSON 模式不输出中间文本。
5. 将终态和上下文取消原因映射为 `runResult` 与退出码：完成为 0，启动/参数/运行失败为 1，取消为 2，超时为 3，安全停止为 4。
6. JSON 模式仅在结束后向标准输出编码一个 `runResult`；所有诊断始终写入标准错误。

### `internal/agent/subagent.go`

**职责：** 让运行时选择是否允许后台子 Agent，同时不改变交互式默认行为。

**设计：** 为 `SubAgentRuntime` 增加默认允许后台的开关。非交互入口关闭该开关后：显式后台请求和 Fork 类型请求返回既有结构化工具失败；隔离前台子 Agent 不会因自动后台计时器被转入后台，而是等待完成或受顶层总超时取消。交互式 TUI 保持当前显式/自动后台策略。

### `cmd/mewcode/main_test.go` 与子 Agent 测试

**职责：** 在无需真实 Provider 的情况下验证 CLI 分流、共享初始化选择和非交互端到端行为。

**覆盖：** 参数互斥、文件输入、文本/JSON 输出、退出码、权限请求自动拒绝、超时、取消、临时会话/记忆、后台子 Agent 拒绝，以及旧 TUI 启动回归。

## 模块交互

```text
mewcode run --config ... --prompt/--prompt-file ...
        │
        ▼
run 参数解析与任务文本校验 ──失败──> stderr + exit 1
        │
        ▼
共享运行时初始化（临时会话、无 Memory 写入、无 Confirmer、禁止后台子 Agent）
        │
        ▼
Runner.Start(ModeAct) + 信号/总超时 Context
        │
        ├─ text_delta ──> 默认 stdout 流式写出 / JSON 模式只累积
        ├─ ActionAsk ──> Scheduler 无 Confirmer，回传拒绝工具结果
        └─ terminal ──> runResult + stdout(JSON) 或 stderr(诊断) + 退出码
```

## 文件组织

```text
cmd/mewcode/
├── main.go          — CLI 分流与共享运行时构建
├── run.go           — 非交互参数、事件消费、输出和退出状态
└── main_test.go     — 入口与端到端离线测试
internal/agent/
├── subagent.go      — 后台子 Agent 运行时开关
└── subagent_test.go — 后台禁用与前台等待回归
docs/ch00/08-noninteractive-run/
├── spec.md
├── plan.md
├── task.md
├── checklist.md
└── manual_scenarios.md — 真实 Provider 下的人工验收方案
README.md            — 新命令、参数、输出与权限行为说明
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 命令结构 | `mewcode run` 子命令 | 与既有 TUI 启动兼容，且为后续独立命令保留清晰边界。 |
| 任务输入 | `--prompt` 与 `--prompt-file` 互斥 | 支持自动化长文本，同时消除输入优先级与 stdin 占用歧义。 |
| 共享启动 | 参数化现有初始化过程 | 保持配置、扩展和安全边界一致，避免两套启动逻辑分叉。 |
| 权限确认 | 不提供 Confirmer | Scheduler 已把缺少 confirmer 的 `ActionAsk` 转换为拒绝工具结果，复用既有安全语义。 |
| 临时状态 | 内存 Session、无 SessionStore/Memory 服务 | 保留一次任务的上下文能力，但不留下可恢复会话或新增长期记忆。 |
| 后台子 Agent | 非交互入口统一禁止 | 确保命令退出代表该次工作完整结束，不会丢失脱离的工作。 |
| 输出 | 默认流式文本；`--json` 单文档结果 | 同时服务终端用户与脚本，避免 JSON 被流式文本污染。 |
| 超时与中断 | 根 Context 统一取消 | Provider、工具和前台子 Agent 可沿已有 Context 链同步停止，并可准确映射终态。 |
