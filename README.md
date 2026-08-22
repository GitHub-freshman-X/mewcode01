# MewCode

MewCode 是一个用 Go 编写的全屏终端 AI Agent。它通过 Anthropic Messages API 或 OpenAI Responses API 提供流式多轮对话，并能在权限控制的项目工作区中读写文件、执行命令、调用 MCP 工具，以及使用会话、记忆、Skill 与 Hook 扩展工作流。

本项目按章节逐步实现 Agent 能力。本文是当前可用功能与运行方式的统一说明；每章的设计边界、实现计划和验收项见 [docs](docs/README.md)。

## 能力概览

| 阶段 | 当前能力 |
|---|---|
| 基础支撑 | 项目 `.env` 加载、结构化本地日志、日志按日期归档与调用位置、启动横幅、消息视觉层级与会话分隔。 |
| [第 2 章：对话](docs/ch02-chat/spec.md) | 全屏 TUI、Anthropic / OpenAI Provider、SSE 流式输出、多轮对话、取消恢复与 Claude Extended Thinking。 |
| [第 3 章：工具](docs/ch03-tools/spec.md) | `read_file`、`write_file`、`edit_file`、`run_command`、`find_files`、`search_code`，以及结构化校验、超时与结果回灌。 |
| [第 4 章：Agent Loop](docs/ch04-loop/spec.md) | 多步 ReAct 工具循环、只读工具并发/副作用工具串行调度、停止边界、`/plan` 与 `/do`。 |
| [第 5 章：System Prompt](docs/ch05-system_prompt/spec.md) | 模块化系统提示词、运行环境与模式注入、稳定/动态内容分离及 Provider 缓存用量观测。 |
| [第 6 章：权限](docs/ch06-permissions/spec.md) | 工作区真实路径沙箱、不可配置绕过的高危命令拦截、分层规则与人在回路确认。 |
| [第 7 章：MCP](docs/ch07-mcp/spec.md) | stdio 和 Streamable HTTP MCP 服务、变量展开、工具发现、远端调用及连接生命周期管理。 |
| [第 8 章：上下文](docs/ch08-context/spec.md) | 大工具结果落盘、上下文预算、自动/手动/紧急压缩及会话历史重建。 |
| [第 9 章：记忆与会话](docs/ch09-memory/spec.md) | JSONL 会话持久化与恢复、跨会话指令、用户/项目记忆、自动提取与惰性治理。 |
| [第 10 章：斜杠命令](docs/ch10-slash_command/spec.md) | 命令注册、解析、Tab 补全、模式状态和会话级 Token 统计。 |
| [第 11 章：Skill](docs/ch11-skills/spec.md) | 项目级/用户级 Skill 发现、按需加载、动态命令、工具白名单、inline 与 fork 执行。 |
| [第 12 章：Hook](docs/ch12-hook/spec.md) | 生命周期事件、条件匹配、命令/提示词/HTTP 动作，以及 `pre_tool_use` 工具拦截。 |

## 快速开始

### 前置条件

- Go 1.25 或更高版本
- 可访问模型服务的网络和 API Key
- 支持 ANSI 的现代终端

在项目根目录创建一份本地配置，再构建并启动：

```sh
cp .mewcode/config.example.yaml .mewcode/config.yaml
# 编辑 .mewcode/config.yaml，填写模型、地址与 API Key
go build -o mewcode ./cmd/mewcode
./mewcode --config ./.mewcode/config.yaml
```

Windows PowerShell：

```powershell
go build -o mewcode.exe ./cmd/mewcode
.\mewcode.exe --config .\.mewcode\config.yaml
```

`--config` 所指配置文件会作为完整主配置加载，不会与默认主配置合并。

## 主配置

未指定 `--config` 时，MewCode 使用 `os.UserConfigDir()/mewcode/config.yaml`：

| 系统 | 默认位置 |
|---|---|
| macOS | `~/Library/Application Support/mewcode/config.yaml` |
| Linux | `$XDG_CONFIG_HOME/mewcode/config.yaml`；通常为 `~/.config/mewcode/config.yaml` |
| Windows | `%AppData%\mewcode\config.yaml` |

将 [`.mewcode/config.example.yaml`](.mewcode/config.example.yaml) 复制到上述位置或任意 `--config` 路径。最小可运行配置如下：

```yaml
protocol: openai # 或 anthropic
model: your-model-name
base_url: https://api.example.com
api_key: replace-with-your-api-key
```

配置使用严格 YAML 校验：未知字段、缺少必填字段或无效取值会在发起模型请求前失败。常用字段如下。

| 字段 | 说明 |
|---|---|
| `max_tokens` | 单次模型响应上限，默认 `4096`。 |
| `thinking.enabled` / `thinking.budget_tokens` | 仅 Anthropic；预算至少 `1024` 且小于 `max_tokens`。OpenAI 配置中不要设置 `thinking`。 |
| `agent.max_iterations` | 单个 Agent 任务最多循环次数，默认 `20`。 |
| `agent.enable_verification_agent` | 是否加载内置 `Verification` 子 Agent，默认 `false`。 |
| `agent.context` | 上下文窗口、摘要预留、安全余量，以及单工具结果和单消息结果预算。完整字段与默认值见模板。 |
| `permissions.mode` | 无规则命中时的权限策略：`strict`、`default`（默认）或 `relaxed`。 |
| `mcp_servers` | MCP 服务映射；`stdio` 使用 `command`、`args`、`env`，`http` 使用 `url`、`headers`。值中的 `${VAR}` 会从环境变量展开。 |

主配置中的 `mcp_servers` 仅来自本次选定的主配置文件；项目级配置不会覆盖或追加它。MCP 工具名称固定为 `<server>__<tool>`，并默认按有副作用工具处理，仍须通过权限门禁。

## 日常使用

普通文本会启动 Agent Loop：模型可多轮调用允许的工具，直至产生最终答复、被取消或触及安全停止条件。`/plan` 仅开放读取、找文件、搜索代码；完成的计划会追加到当前会话，`/do` 才以完整工具集执行这些计划。计划/执行任务被取消或异常停止时，待执行计划不会被清空。

- `Enter`：提交输入。
- `Page Up` / `Page Down`：滚动历史。
- `Tab`：补全斜杠命令。
- `Ctrl+T`：展开或折叠 Claude thinking。
- `Ctrl+C`：生成期间取消当前任务；空闲时退出。
- `Esc`：当前台子 Agent 运行时，将其转入后台。

### 斜杠命令

| 命令 | 用途 |
|---|---|
| `/help [命令]` | 显示帮助（别名：`/h`）。 |
| `/compact` | 在无活动任务时手动压缩当前上下文。 |
| `/clear` | 新建并切换会话。 |
| `/plan [需求]` | 切换计划模式，或以只读工具规划需求。 |
| `/do` | 执行当前会话累积的待执行计划。 |
| `/session [list\|new\|resume <id>\|delete <id>]` | 查看、创建、恢复或删除会话（别名：`/s`）。 |
| `/memory [list\|add <类别> <内容>\|clear]` | 管理记忆（别名：`/m`）；清空时须再次输入确认。 |
| `/status` | 显示本地诊断快照：模式、工作目录、日志目录、权限、会话、累计 Token、工具/Skill/记忆/SubAgent 数量及运行中后台任务。 |
| `/skills reload` | 重新发现 Skill 并刷新对应命令。 |
| `/<skill-name> [参数]` | 执行已发现的 Skill；内置 `commit`、`review`、`test`。 |
| `/exit` | 空闲时退出 MewCode。 |

## 项目文件与扩展

| 用途 | 用户级位置 | 项目级位置 |
|---|---|---|
| 指令 | `<用户配置目录>/mewcode/MEWCODE.md` | `<项目根>/.mewcode/MEWCODE.md` |
| 记忆 | `<用户配置目录>/mewcode/memory/` | `<项目根>/.mewcode/memory/` |
| 会话与大工具结果 | — | `<项目根>/.mewcode/sessions/`、`<项目根>/.mewcode/context/` |
| Skill | `<用户配置目录>/mewcode/skills/` | `<项目根>/.mewcode/skills/` |
| SubAgent 定义 | `<用户配置目录>/mewcode/agents/` | `<项目根>/.mewcode/agents/` |
| 权限规则 | `<用户配置目录>/mewcode/permissions.yaml` | `<项目根>/.mewcode/permissions.yaml` |
| Hook | `<用户配置目录>/mewcode/config.yaml` | `<项目根>/.mewcode/config.yaml` |

项目级定义在同名 Skill 时覆盖用户级定义；Skill 可为单个 Markdown 文件，或包含 `SKILL.md` 的目录。入口使用 YAML frontmatter，正文是模型按需加载的操作流程：

```markdown
---
name: review-api
description: 审查 API 变更并报告可操作问题。
mode: inline
tools: [read_file, search_code]
---

检查本次 API 变更，重点关注兼容性和错误处理。
```

`name` 只能使用小写字母、数字与连字符；`mode` 默认为 `inline`，也可为隔离会话执行的 `fork`。`tools` 只能缩小模型可见工具范围，不能提升权限。修改本地 Skill 后执行 `/skills reload`。

Hook 配置可在用户主配置或项目 `.mewcode/config.yaml` 的 `hooks` 顶层字段中声明；规则按“用户、项目”顺序追加。支持 `command`、`prompt`、`http`、`agent` 动作；`agent` 动作会以后台 `general-purpose` 子 Agent 执行。`pre_tool_use` 可使用 `reject: true` 拦截真实工具调用，且不能设置 `async: true`。完整示例见 [配置模板](.mewcode/config.example.yaml) 与 [Hook Spec](docs/ch12-hook/spec.md)。

### SubAgent

主 Agent 可通过固定的 `agent` 工具委派子任务。定义式子 Agent 从上表的用户级、项目级目录发现；同名定义按项目级、用户级、内置级、插件级的顺序覆盖。定义文件使用 YAML frontmatter（`name`、`description`、`tools`、`disallowedTools`、`model`、`maxTurns`、`permissionMode`）和 Markdown 操作说明。

内置 `Explore`、`Plan`、`general-purpose` 始终可用；`Verification` 需设置 `agent.enable_verification_agent: true`。定义式子 Agent 使用独立对话和权限记录；省略 `subagent_type` 或设为 `fork` 的 Fork 子 Agent 继承父对话并始终后台运行，`fork` 是不可用于角色定义的保留名称。显式后台、运行 120 秒自动后台及按 `Esc` 都会创建或接管进程内任务，完成结果以通知回传主对话。后台任务不会跨会话持久化，且暂不支持 `isolation: worktree`、Fork 再 Fork 或后台任务再派生子 Agent。

## 权限与安全

- 启动时加载项目根目录 `.env`；已有系统环境变量优先，`.env` 不会覆盖它们。
- 文件工具只允许访问项目工作区内的真实路径；符号链接逃逸也会被拦截。
- 高危命令黑名单不可由配置放行。其他工具请求依次受会话、项目、用户规则及当前权限模式约束，并可能要求交互确认。
- 权限规则的优先级为会话级、项目级、用户级；“始终允许”写入 `<项目根>/.mewcode/permissions.yaml`。
- API Key、`.env`、用户级权限规则、会话与记忆通常包含私有信息，不应提交到版本控制；Unix 系统建议对配置文件使用 `chmod 600`。

## 构建与验证

```sh
# 当前平台
go build -o mewcode ./cmd/mewcode
go test ./...

# 在 macOS/Linux shell 为 Windows 构建
GOOS=windows GOARCH=amd64 go build -o mewcode.exe ./cmd/mewcode
```

在 PowerShell 中为 macOS 或 Linux 构建：

```powershell
$env:GOOS = "darwin" # Linux 使用 "linux"
$env:GOARCH = "amd64"
go build -o mewcode ./cmd/mewcode
```

交叉编译只生成二进制；目标系统仍需准备自己的主配置，并保证其中引用的 MCP 子进程或 HTTP 服务在目标环境可用。

## 文档与问题记录

- [章节文档索引](docs/README.md)
- [Bug 记录索引](bugs/README.md)
- [开发约束](AGENTS.md)
