# MewCode

MewCode 是一个用 Go 构建的全屏终端 AI Agent。它通过 Anthropic Messages API 或 OpenAI Responses API 提供流式多轮对话，并在受权限保护的项目工作区中完成文件操作、命令执行、MCP 调用与 Agent 协作。

项目按十四个递进主题实现：从终端对话出发，逐步加入工具、执行循环、长期上下文、扩展机制、子任务和 Git Worktree 隔离。每章的设计边界、实施过程与验收记录见 [章节文档](docs/README.md)。

## 能力全景

| 章节 | 主题 | 当前能力 |
|---|---|---|
| 工程基础 | 配置、日志与终端体验 | 项目 .env 加载、严格 YAML 校验、结构化本地日志与诊断、日期归档和清晰的消息/会话视觉层级。 |
| [第 2 章](docs/ch02-chat/spec.md) | 流式对话 | 全屏 TUI、Anthropic / OpenAI Provider、SSE 流式多轮对话、取消恢复与 Claude Extended Thinking。 |
| [第 3 章](docs/ch03-tools/spec.md) | 工作区工具 | 读、写、编辑、查找、搜索文件和执行命令；参数校验、超时、结构化结果与历史回灌。 |
| [第 4 章](docs/ch04-loop/spec.md) | Agent Loop | 多步 ReAct 循环，只读工具并发、有副作用工具串行，以及 /plan、/do 工作流。 |
| [第 5 章](docs/ch05-system_prompt/spec.md) | System Prompt | 模块化系统指令、运行环境与模式注入、稳定/动态内容分离和缓存用量观测。 |
| [第 6 章](docs/ch06-permissions/spec.md) | 权限与安全 | 真实路径工作区沙箱、高危命令拦截、分层规则与交互确认。 |
| [第 7 章](docs/ch07-mcp/spec.md) | MCP Client | stdio 与 Streamable HTTP 服务、变量展开、工具发现、远端调用和连接生命周期。 |
| [第 8 章](docs/ch08-context/spec.md) | 上下文管理 | 大工具结果按需落盘、消息预算、自动/手动/强制/紧急压缩和历史重建。 |
| [第 9 章](docs/ch09-memory/spec.md) | 会话与记忆 | JSONL 会话存档与恢复、用户/项目指令、跨会话记忆、异步提取和惰性治理。 |
| [第 10 章](docs/ch10-slash_command/spec.md) | 斜杠命令 | 命令注册、解析、Tab 补全、模式切换、会话管理和 Token 统计。 |
| [第 11 章](docs/ch11-skills/spec.md) | Skill | 项目级/用户级发现、按需加载、动态命令、工具白名单和 inline / fork 执行。 |
| [第 12 章](docs/ch12-hook/spec.md) | Hook | 生命周期事件与条件匹配，命令、提示词、HTTP 动作，以及工具前拦截。 |
| [第 13 章](docs/ch13-subagent/spec.md) | SubAgent | 定义式与 Fork 子 Agent、前台进度、后台任务、完成通知、权限与会话隔离。 |
| [第 14 章](docs/ch14-worktree/spec.md) | Git Worktree | Worktree 创建、进入、恢复、受保护删除与过期清理；隔离子 Agent 使用显式工作目录。 |

用户输入经系统指令、记忆和 Skill 形成模型请求；Agent Loop 可调用内置或 MCP 工具，实际操作均通过 Hook 与权限门禁；会话、上下文和后台任务使长任务可持续，需要文件隔离的子任务则在独立 Worktree 中执行。

## 快速开始

### 前置条件

- Go 1.25 或更高版本
- 可访问模型服务的网络和 API Key
- 支持 ANSI 的现代终端

在项目根目录创建本地配置，填写模型服务信息后构建并启动：

~~~sh
cp .mewcode/config.example.yaml .mewcode/config.yaml
# 编辑 .mewcode/config.yaml，填写模型、地址与 API Key
go build -o mewcode ./cmd/mewcode
./mewcode --config ./.mewcode/config.yaml
~~~

Windows PowerShell：

~~~powershell
go build -o mewcode.exe ./cmd/mewcode
.\mewcode.exe --config .\.mewcode\config.yaml
~~~

--config 指定的文件会作为完整主配置加载，不会与默认主配置合并。

## 配置

未指定 --config 时，MewCode 使用 os.UserConfigDir()/mewcode/config.yaml：

| 系统 | 默认位置 |
|---|---|
| macOS | ~/Library/Application Support/mewcode/config.yaml |
| Linux | $XDG_CONFIG_HOME/mewcode/config.yaml；通常为 ~/.config/mewcode/config.yaml |
| Windows | %AppData%\mewcode\config.yaml |

将 [配置模板](.mewcode/config.example.yaml) 复制到该位置或任意 --config 路径。最小配置如下：

~~~yaml
protocol: openai # 或 anthropic
model: your-model-name
base_url: https://api.example.com
api_key: replace-with-your-api-key
~~~

配置采用严格校验：未知字段、缺失必填字段或无效取值会在模型请求前失败。完整字段和默认值以配置模板为准，常用配置包括：

| 字段 | 说明 |
|---|---|
| max_tokens | 单次模型回复上限，默认 4096。 |
| thinking.enabled / thinking.budget_tokens | 仅 Anthropic 可用；预算至少 1024 且小于 max_tokens。 |
| agent.max_iterations | 单个 Agent 任务的最大循环次数，默认 20。 |
| agent.enable_verification_agent | 是否启用内置 Verification 子 Agent，默认 false。 |
| agent.context | 上下文窗口、摘要预留、安全余量及工具结果预算。 |
| permissions.mode | 规则未命中时的策略：strict、default（默认）或 relaxed。 |
| mcp_servers | MCP 服务定义；支持 stdio 与 http，配置值可引用环境变量。 |
| worktree | 本地文件复制、依赖目录链接及临时 Worktree 保留时间。 |

主配置的 mcp_servers 只取自本次选定的主配置文件；项目级配置不会追加它。发现后的 MCP 工具名固定为 <server>__<tool>，默认按有副作用工具处理，仍须通过权限检查。

## 日常使用

普通文本会启动 Agent Loop：模型可在权限允许的范围内多轮调用工具，直到给出最终答复、被取消或达到安全停止条件。/plan 只开放读取、找文件和搜索代码；生成的计划会留在当前会话，使用 /do 以完整工具集执行。

- Enter：提交输入。
- Page Up / Page Down：滚动历史。
- Tab：补全斜杠命令。
- Ctrl+T：展开或折叠 Claude thinking。
- Ctrl+C：生成期间取消当前任务；空闲时退出。
- Esc：前台子 Agent 运行时将其转为后台。

### 斜杠命令

| 命令 | 用途 |
|---|---|
| /help [命令] | 显示帮助（别名：/h）。 |
| /compact | 无活动任务时手动压缩当前上下文。 |
| /clear | 新建并切换会话。 |
| /plan [需求] | 切换计划模式，或以只读工具规划需求。 |
| /do | 执行当前会话累积的待执行计划。 |
| /session | 列出、创建、恢复或删除会话（别名：/s）。 |
| /memory | 管理记忆（别名：/m）；清空需再次确认。 |
| /status | 查看模式、工作目录、日志、权限、会话、Token、累计缓存读取/写入与命中率、扩展和后台任务；旧会话缓存历史未知时显示 `—`。 |
| /worktree | 创建、列出、进入、退出或删除 Git Worktree。 |
| /skills reload | 重新发现 Skill 并刷新动态命令。 |
| /<skill-name> [参数] | 执行已发现的 Skill；内置 commit、review、test。 |
| /exit | 空闲时退出 MewCode。 |

### 非交互式单任务

`mewcode run` 在当前工作目录执行一条普通 Agent 任务，适合脚本和自动化调用。它不创建可恢复会话，也不会写入长期记忆：

~~~sh
mewcode run --config ./.mewcode/config.yaml --prompt "修复当前项目的测试失败"
mewcode run --config ./.mewcode/config.yaml --prompt-file ./task.md --json
~~~

任务必须且只能通过 `--prompt` 或 `--prompt-file` 之一提供。`--timeout` 默认是 `30m`，使用 `--timeout 0` 可关闭总超时；`--json` 会在任务结束时向标准输出写入一个 JSON 结果，默认模式则实时输出 Agent 文本。所有诊断写入标准错误。

非交互模式沿用项目的指令、Hook、Skill、MCP 和权限规则，但不会等待人工确认：需要确认的工具调用会自动作为拒绝结果返回给 Agent。前台子 Agent 可以完成，后台和 Fork 子 Agent 请求会被拒绝。退出码为：`0` 完成、`1` 启动/参数/运行失败、`2` 取消、`3` 总超时、`4` 迭代或未知工具安全停止。

## 项目约定与扩展

| 用途 | 用户级位置 | 项目级位置 |
|---|---|---|
| 指令 | <用户配置目录>/mewcode/MEWCODE.md | <项目根>/.mewcode/MEWCODE.md |
| 记忆 | <用户配置目录>/mewcode/memory/ | <项目根>/.mewcode/memory/ |
| Skill | <用户配置目录>/mewcode/skills/ | <项目根>/.mewcode/skills/ |
| SubAgent 定义 | <用户配置目录>/mewcode/agents/ | <项目根>/.mewcode/agents/ |
| 权限规则 | <用户配置目录>/mewcode/permissions.yaml | <项目根>/.mewcode/permissions.yaml |
| 会话与大工具结果 | — | <项目根>/.mewcode/sessions/、<项目根>/.mewcode/context/ |
| Worktree | — | <项目根>/.mewcode/worktrees/ |
| Hook | 用户主配置 | <项目根>/.mewcode/config.yaml |

项目级同名 Skill、子 Agent 定义和权限规则优先于用户级定义。Skill 可以是单个 Markdown 文件，也可以是含 SKILL.md 的目录；正文只会在激活后作为 SOP 加载。Skill 的工具白名单只能缩小模型可见工具范围，不能提高权限；编辑后运行 /skills reload。

内置 Explore、Plan、general-purpose 子 Agent 始终可用，Verification 需在配置中启用。定义式子 Agent 使用独立对话和权限记录；Fork 子 Agent 继承父对话、始终后台运行，并在唯一临时 Worktree 中执行。定义式 Agent 可声明 isolation: worktree；干净目录会自动清理，有未提交修改或新提交的目录会被保留以保护成果。

Hook 可在用户主配置或项目 .mewcode/config.yaml 的 hooks 顶层声明；两处规则按“用户、项目”顺序追加。支持 command、prompt、http 与 agent 动作，其中 agent 是可校验的预留动作。pre_tool_use 可用 reject: true 拦截真实工具调用，且不能设为 async: true。详细示例见[配置模板](.mewcode/config.example.yaml)和 [Hook Spec](docs/ch12-hook/spec.md)。

## 权限与数据安全

- 启动时加载项目根目录 .env；已有系统环境变量优先，.env 不会覆盖它们。
- 文件工具只可访问项目工作区内的真实路径，符号链接逃逸同样会被阻止。
- 高危命令黑名单不能通过配置放行。其他请求依次受会话、项目、用户规则及当前权限模式约束，并可能要求交互确认。
- API Key、.env、用户级权限规则、会话、记忆和工具结果通常含私有信息，不应提交到版本控制；Unix 系统建议将配置文件设为 chmod 600。

## 构建与验证

~~~sh
go build -o mewcode ./cmd/mewcode
go test ./...

# 在 macOS/Linux shell 为 Windows 构建
GOOS=windows GOARCH=amd64 go build -o mewcode.exe ./cmd/mewcode
~~~

交叉编译只生成二进制。目标系统仍需准备自己的主配置，并保证配置引用的 MCP 子进程或 HTTP 服务可用。

## 文档与问题记录

- [章节文档索引](docs/README.md)
- [Bug 记录索引](bugs/README.md)
- [开发约束](AGENTS.md)
