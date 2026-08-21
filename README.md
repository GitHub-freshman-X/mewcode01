# MewCode

MewCode 是一个使用 Go 构建的全屏终端 AI Agent。它可通过 Anthropic Messages API 或 OpenAI Responses API 进行流式多轮对话，并让模型在受权限控制的工作区内读取、搜索和修改文件、执行命令、调用 MCP 工具。

当前项目以章节方式从零构建 Agent。README 说明当前已实现的用户可见能力；每章的详细边界、设计与验收标准请见 [`docs/`](docs/README.md)。

## 已实现能力

| 章节 | 能力 |
|---|---|
| [基础支撑（ch00）](docs/ch00/01-logging/spec.md) | 结构化日志、项目 `.env` 加载、日志文件按日期归档与调用位置记录。 |
| [第 2 章：对话](docs/ch02-chat/spec.md) | 全屏 TUI、Anthropic/OpenAI Provider、SSE 流式输出、多轮对话与 Claude Extended Thinking。 |
| [第 3 章：工具](docs/ch03-tools/spec.md) | 读、写、编辑文件，执行命令，查找文件和搜索代码；工具结果回灌模型上下文。 |
| [第 4 章：Agent Loop](docs/ch04-loop/spec.md) | 多步工具循环、停止边界，以及 `/plan` 和 `/do` 工作流。 |
| [第 5 章：系统提示词](docs/ch05-system_prompt/spec.md) | 分层结构化系统指令、运行时上下文注入和缓存相关观测。 |
| [第 6 章：权限](docs/ch06-permissions/spec.md) | 工作区路径沙箱、危险操作拦截、规则优先级和人在回路确认。 |
| [第 7 章：MCP](docs/ch07-mcp/spec.md) | stdio 与 HTTP MCP 服务的配置、工具发现、调用及生命周期管理。 |
| [第 8 章：上下文](docs/ch08-context/spec.md) | 工具结果预算、会话摘要压缩、`/compact` 与上下文恢复。 |
| [第 9 章：记忆与会话](docs/ch09-memory/spec.md) | JSONL 会话持久化、跨会话指令和记忆加载、自动记忆提取与治理。 |
| [第 10 章：斜杠命令](docs/ch10-slash_command/spec.md) | 命令注册、解析、补全与十个内置命令。 |
| [第 11 章：Skill](docs/ch11-skills/spec.md) | 项目级和用户级 Skill 发现、按需加载、动态命令、工具白名单及 inline/fork 执行模式。 |
| [第 12 章：Hook](docs/ch12-hook/spec.md) | 生命周期事件自动化、条件匹配、动作执行与工具调用前拦截。 |

## 快速开始

### 前置条件

- Go 1.25 或更高版本
- 可访问所选模型服务的网络和 API Key
- 支持 ANSI 的现代终端

获取源码后，首先配置一份配置文件，模版为.mewcode/config.example.yaml，至少配置protocol、model、base_url、api_key这几个字段，然后在项目根目录执行：

```sh
go build -o mewcode ./cmd/mewcode
./mewcode --config ./mewcode/config.yaml
```

Windows PowerShell：

```powershell
go build -o mewcode.exe ./cmd/mewcode
.\mewcode.exe --config ./mewcode/config.yaml
```

`--config` 指定的文件会完整替代默认主配置，不会与默认文件合并。

## 配置

默认主配置由 Go 的 `os.UserConfigDir()` 决定：

| 系统 | 默认位置 |
|---|---|
| macOS | `~/Library/Application Support/mewcode/config.yaml` |
| Linux | `$XDG_CONFIG_HOME/mewcode/config.yaml`；未设置时通常为 `~/.config/mewcode/config.yaml` |
| Windows | `%AppData%\mewcode\config.yaml` |

可复制项目中的 [`.mewcode/config.example.yaml`](.mewcode/config.example.yaml) 到上述位置，再替换模型和密钥。最小配置如下：

```yaml
protocol: openai # 或 anthropic
model: your-model-name
base_url: https://api.example.com
api_key: replace-with-your-api-key
```

常用可选项：

- `max_tokens`：单次模型响应上限，默认 `4096`。
- `thinking.enabled` 与 `thinking.budget_tokens`：仅 Anthropic 支持；预算至少为 `1024`，且必须小于 `max_tokens`。
- `agent.max_iterations`：Agent Loop 上限，默认 `20`。
- `agent.context`：上下文窗口、摘要和工具结果预算。
- `permissions.mode`：没有规则命中时的默认权限模式。
- `mcp_servers`：配置 stdio 或 HTTP MCP 服务。stdio 服务可填写 `command`、`args`、`env`；HTTP 服务可填写 `url`、`headers`。
- `hooks`：在 `os.UserConfigDir()/mewcode/config.yaml` 或 `<项目>/.mewcode/config.yaml` 中声明生命周期自动化规则；规则按用户、项目顺序追加。每条规则包含 `event`、可选 `if` 和 `action`。条件可用 `==`、`!=`、`=~`、`~=` 与单一的 `&&` 或 `||` 组合；动作中可使用 `$EVENT`、`$TOOL_NAME`、`$FILE_PATH`、`$MESSAGE`、`$ERROR`、`$TOOL_ARGS.<名称>`。`pre_tool_use` 可通过 `reject: true` 拦截工具调用，且不能设置 `async: true`。可用动作是 `command`、`prompt`、`http`、`agent`；`command.timeout` 控制最长运行时间，`agent` 当前只保留后续运行时对接点。

OpenAI Responses API 不支持 `thinking` 配置。配置字段会被严格校验，未知字段或无效值会使程序在启动前失败。

## 使用

### 基本操作

- `Enter`：提交输入。
- `Page Up` / `Page Down`：滚动消息历史。
- `Tab`：补全斜杠命令。
- `Ctrl+T`：展开或折叠 Claude thinking 内容。
- `Ctrl+C`：生成中取消当前任务；空闲时退出。
- 消息区以背景色区分用户消息、最终答复及思考和工具过程；执行 `/clear`、`/session new` 或 `/session resume` 后会显示带会话 ID 与标题的分隔条，方便在同一终端的滚动历史中定位会话起点。

### 斜杠命令

| 命令 | 用途 |
|---|---|
| `/help [命令]` | 显示命令帮助（别名：`/h`）。 |
| `/compact` | 请求压缩当前上下文。 |
| `/clear` | 新建并切换会话。 |
| `/plan [需求]` | 切换计划模式，或在计划模式下提交需求。 |
| `/do` | 执行当前待执行计划。 |
| `/session [list\|new\|resume <id>\|delete <id>]` | 管理会话（别名：`/s`）。 |
| `/memory [list\|add <类别> <内容>\|clear]` | 管理记忆（别名：`/m`）。 |
| `/status` | 查看当前状态和会话 Token 用量。 |
| `/review [关注点]` | 让 Agent 结合当前对话中的需求与偏好审查当前 Git diff（别名：`/r`）。 |
| `/skills reload` | 重新扫描 Skill，并更新可用的 Skill 命令。 |
| `/commit [要求]` | 使用内置 Git 提交流程分析当前变更并创建提交。 |
| `/test [要求]` | 使用内置测试流程识别并运行与当前变更相关的验证。 |
| `/exit` | 空闲时退出 MewCode。 |

`/plan` 模式仅提供只读探索能力；`/do` 恢复完整工具权限并执行当前会话中累积的计划。普通文本输入会启动 Agent Loop，模型可以在权限规则和交互确认的约束内持续调用工具，直到产出最终回答或到达停止边界。

### Skill

Skill 将可复用的 AI 操作流程保存为 Markdown 文件。启动时 MewCode 会从以下目录发现 Skill；同名时项目级定义优先于用户级定义，也可覆盖内置 Skill：

- 项目级：`<项目根>/.mewcode/skills/`
- 用户级：`<用户配置目录>/mewcode/skills/`

Skill 可以是上述目录中的单个 Markdown 文件，或一个包含 `SKILL.md` 入口文件的子目录。目录中的模板、示例、脚本和参考资料可由 Skill 流程按需使用，但不会在发现阶段加载。入口文件使用 YAML frontmatter，正文是提供给模型的操作流程：

```markdown
---
name: review-api
description: 审查 API 变更并报告可操作问题。
mode: inline
tools: [read_file, search_code]
---

检查本次 API 变更，重点关注兼容性和错误处理。
```

`name` 只能包含小写字母、数字和连字符；`description` 必填。`mode` 默认为 `inline`，共享当前对话；设为 `fork` 时在独立对话执行，完成后只向主对话回流摘要。fork Skill 可用 `context: full`、`recent` 或 `none` 控制带入的主对话历史。`tools` 是可选白名单，只会缩小模型可见的工具范围，不会提升权限。正文中的 `{{args}}` 会在通过 `/skill-name 参数` 显式调用时替换为参数。

MewCode 内置 `commit`、`review` 和 `test` 三个 inline Skill；已发现的任意 Skill 也会自动成为同名斜杠命令，并支持 `/help` 与 Tab 补全。普通对话中，模型会先看到 Skill 名称与说明，必要时通过 `load_skill` 按需加载完整流程。新增、修改或删除本地 Skill 后执行 `/skills reload`；若刷新失败，先前可用的 Skill、命令和已激活状态会保持不变。执行 `/clear` 或切换会话会清除已激活的 Skill。

## 本地文件、权限与安全

- 项目根目录的 `.env` 会在启动时加载；已存在于系统环境中的同名变量优先，不会被 `.env` 覆盖。
- 权限规则每次启动合并三层：`~/.mewcode/permissions.yaml`、`<项目根>/.mewcode/permissions.yaml` 和 `<项目根>/.mewcode/permissions.local.yaml`。优先级从高到低为本地级、项目级、用户级；缺失文件视为空规则。
- 项目级会话保存在 `<项目根>/.mewcode/sessions/`；项目级记忆保存在 `<项目根>/.mewcode/memory/`。
- 用户级指令文件为 `<用户配置目录>/mewcode/MEWCODE.md`，项目级指令文件为 `<项目根>/.mewcode/MEWCODE.md`。用户级记忆保存在 `<用户配置目录>/mewcode/memory/`。
- API Key 以明文保存在 YAML 中。请限制配置文件权限（Unix 可使用 `chmod 600`），并且不要将配置、`.env` 或密钥提交到版本控制。

## 跨平台构建

项目使用 Go 原生交叉编译，无需特定 Shell。以下命令从项目根目录执行：

```sh
# macOS 或 Linux 当前平台构建
go build -o mewcode ./cmd/mewcode

# 为 Windows 构建（在 macOS/Linux shell 中执行）
GOOS=windows GOARCH=amd64 go build -o mewcode.exe ./cmd/mewcode

# 为 macOS 或 Linux 构建（在 PowerShell 中执行）
$env:GOOS = "darwin" # Linux 使用 "linux"
$env:GOARCH = "amd64"
go build -o mewcode ./cmd/mewcode
```

运行时仍需在目标系统准备该系统的主配置文件，并保证其中配置的 MCP 子进程或 HTTP 服务在目标环境可用。

## 文档与问题记录

- [章节文档索引](docs/README.md)
- [Bug 记录索引](bugs/README.md)
- [贡献与开发约束](AGENTS.md)
