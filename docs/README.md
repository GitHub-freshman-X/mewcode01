# MewCode 文档索引

项目文档按章节顺序和主题命名：

```text
chNN-topic/
├── spec.md
├── plan.md
├── task.md
└── checklist.md
```

## 章节

| 章节 | 主题 | 文档 |
|---|---|---|
| `ch02-chat` | 纯对话基础：Provider、流式多轮对话、TUI 与配置 | [Spec](ch02-chat/spec.md) · [Plan](ch02-chat/plan.md) · [Tasks](ch02-chat/task.md) · [Checklist](ch02-chat/checklist.md) |
| `ch03-tools` | 工具系统：工具注册、文件与命令工具、工具调用和结果回灌 | [Spec](ch03-tools/spec.md) · [Plan](ch03-tools/plan.md) · [Tasks](ch03-tools/task.md) · [Checklist](ch03-tools/checklist.md) |
| `ch04-loop` | Agent Loop：自主多轮工具循环、停止边界与 Plan/Do 工作流 | [Spec](ch04-loop/spec.md) · [Plan](ch04-loop/plan.md) · [Tasks](ch04-loop/task.md) · [Checklist](ch04-loop/checklist.md) |
| `ch05-system_prompt` | System Prompt：结构化系统指令、动态注入与缓存观测 | [Spec](ch05-system_prompt/spec.md) · [Plan](ch05-system_prompt/plan.md) · [Tasks](ch05-system_prompt/task.md) · [Checklist](ch05-system_prompt/checklist.md) |
| `ch06-permissions` | Permission System：危险命令黑名单、路径沙箱、规则与人在回路确认 | [Spec](ch06-permissions/spec.md) · [Plan](ch06-permissions/plan.md) · [Tasks](ch06-permissions/task.md) · [Checklist](ch06-permissions/checklist.md) |
| `ch07-mcp` | MCP Client：外部工具配置、发现、调用与生命周期管理 | [Spec](ch07-mcp/spec.md) · [Plan](ch07-mcp/plan.md) · [Tasks](ch07-mcp/task.md) · [Checklist](ch07-mcp/checklist.md) |
| `ch08` | 上下文管理：工具结果预算、摘要压缩、`/compact` 与紧急恢复 | [Spec](ch08/spec.md) · [Plan](ch08/plan.md) · [Tasks](ch08/task.md) · [Checklist](ch08/checklist.md) · [Manual scenarios](ch08/manual_scenarios.md) · [Summary tolerance design](ch08/summary-parser-tolerance-design.md) · [Summary tolerance plan](ch08/summary-parser-tolerance-plan.md) · [Reference](ch08/context-compression-and-token-management.md) |
| `ch10-slash_command` | Slash Command：注册、解析、补全与 TUI 命令分流 | [Spec](ch10-slash_command/spec.md) · [Plan](ch10-slash_command/plan.md) · [Tasks](ch10-slash_command/task.md) · [Checklist](ch10-slash_command/checklist.md) · [Reference](ch10-slash_command/理论学习：Slash%20Command%20命令框架.md) |

新增章节时继续采用 `chNN-topic` 格式，并在本页追加索引。

## 中途补充

| 目录 | 主题 | 文档 |
|---|---|---|
| `ch00/01-logging` | Application Logging：可复用本地结构化日志与 MCP 全生命周期观测 | [Spec](ch00/01-logging/spec.md) |
| `ch00/02-env-loading` | Environment Loading：项目 `.env` 自动加载与安全变量优先级 | [Spec](ch00/02-env-loading/spec.md) |
| `ch00/03-log-source-location` | Log Source Location：日志调用点文件与行号 | [Spec](ch00/03-log-source-location/spec.md) |
