# 第十章 Slash Command 真实 API 人工测试方案

## 目标与范围

本方案在真实启动 `mewcode` 并连接真实 Provider 后，验证第十章的 Slash Command 交互效果：命令发现、别名、帮助、Tab 补全、本地反馈、计划/执行模式、会话切换、记忆管理与 `/review` 提示词快捷方式。

本方案只验证用户可见的真实使用效果，不创建或运行新的自动化测试文件。注册冲突、日志字段脱敏、活跃任务切换会话的竞争条件、畸形会话文件与网络失败等确定性边界，仍由自动化测试负责。

## 安全约束

- 只在独立 fixture 工作区运行；不得在项目仓库或真实工作目录内执行 `/clear`、`/session delete`、`/memory clear`。
- 使用专门的、限额的真实 API 凭据。配置文件不得提交、截图或粘贴到测试记录中。
- 真实模型可能调用工具；出现权限确认时，只允许 fixture 工作区内的只读操作。任何写入请求均拒绝。
- 记录命令、可见状态、会话 ID、文件计数和结果；不要记录完整提示词、模型回复、会话 JSONL、记忆正文、API key 或日志正文。
- 一个 TUI 实例仅创建一个初始会话。退出并重新启动程序才是新的初始会话；`/clear` 与 `/session new` 则在同一实例内切换会话。

## 准备 fixture 与真实配置

在项目根目录执行以下命令。它会创建隔离工作区、隔离用户配置根和测试文件：

```sh
fixture_root=$(mktemp -d /private/tmp/mewcode-ch10-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch10-home.XXXXXX)
user_mewcode="$test_home/Library/Application Support/mewcode"
mkdir -p "$fixture_root/.mewcode/memory" "$fixture_root/fixtures" "$user_mewcode/memory"
printf 'SLASH-FIXTURE-ALPHA-7e31\n' > "$fixture_root/fixtures/alpha.txt"
printf 'SLASH-FIXTURE-BETA-93ac\n' > "$fixture_root/fixtures/beta.txt"
printf '%s\n' '- [baseline-user](baseline-user.md) — user baseline' > "$user_mewcode/memory/MEMORY.md"
printf '%s\n' '- [baseline-project](baseline-project.md) — project baseline' > "$fixture_root/.mewcode/memory/MEMORY.md"
printf 'fixture_root=%s\ntest_home=%s\n' "$fixture_root" "$test_home"
```

复制一份已验证可用的真实 API 配置为 `<real-config.yaml>`。保留 `protocol`、`model`、`base_url` 和 `api_key`，建议关闭 thinking 以控制成本：

```yaml
max_tokens: 2048
thinking:
  enabled: false
permissions:
  mode: default
```

构建临时二进制并启动。当前 macOS 实现通过 `os.UserConfigDir()` 读取 `$HOME/Library/Application Support`；以下命令使用隔离的 `HOME`，因此用户级记忆只会写入 `$test_home`。若运行环境忽略 `HOME`，请停止并跳过涉及用户级记忆的场景，绝不要替换真实用户目录。

```sh
go build -o /private/tmp/mewcode-ch10 /Users/xuchangan/Project/mine/mew-agent/mew01/cmd/mewcode
cd "$fixture_root"
HOME="$test_home" /private/tmp/mewcode-ch10 --config <real-config.yaml>
```

启动后先确认状态栏显示 `[DEFAULT]`，并保存当前工作目录与初始会话 ID（可通过 `/session` 查看）。

## 场景 A：发现、帮助与补全

### A1 命令目录、别名与未知命令

依次输入：

```text
/
/HELP
/h
/not-a-command
```

通过条件：

- `/` 显示可用的九个内置命令：`help`、`compact`、`clear`、`plan`、`do`、`session`、`memory`、`status`、`review`。
- `/HELP` 与 `/h` 都显示帮助，且帮助包含名称、别名（如有）、说明和用法。
- 未知命令提供 `/help` 引导，不启动模型请求、不改变会话消息数。

### A2 Tab 补全和候选选择

分别在输入框中键入以下内容，不要按 Enter：

```text
/he
/
/zz
```

操作与通过条件：

1. `/he` 后按 Tab：输入框直接补成 `/help `。
2. `/` 后按 Tab：出现稳定的命令候选菜单；使用上下键改变高亮，再按 Tab 或 Enter 接受当前候选。
3. `/zz` 后按 Tab：输入内容保持不变，不出现候选。

## 场景 B：本地快车道与普通对话

### B1 本地命令不访问真实 API

依次输入：

```text
/status
/help plan
/session
/memory
```

通过条件：每条均立即显示系统反馈，不显示 Agent 迭代、工具调用或流式模型回复；会话消息数不增加。`/status` 包含当前模式与 Token 用量；`/memory` 显示用户级和项目级概要。

### B2 普通文本仍走真实模型

输入：

```text
不调用工具，只回答：SLASH-CHAT-OK。
```

通过条件：TUI 显示 Agent 执行与模型回复 `SLASH-CHAT-OK`；`/session` 的消息数增加。随后再输入 `/status`，确认其仍是即时本地反馈，不会再次请求模型。

## 场景 C：计划模式与执行

### C1 无参数切换

依次输入：

```text
/plan
/plan
```

通过条件：第一次后状态栏变为 `[PLAN]`，第二次恢复 `[DEFAULT]`；两次均不访问真实 API。

### C2 带需求的计划与 `/do`

输入：

```text
/plan 仅制定一个只读计划：读取 fixtures/alpha.txt，核对其中唯一标记；不要修改文件。
```

等待计划完成。通过条件：状态栏为 `[PLAN]`，模型只给出计划而不写入 fixture。

再输入：

```text
/do
```

若权限确认出现，选择单次允许只读 `read_file`。通过条件：状态栏在请求启动前回到 `[DEFAULT]`；执行读取并最终回复包含 `SLASH-FIXTURE-ALPHA-7e31`；`fixtures/` 下文件没有被修改。

## 场景 D：会话命令

先执行 `/session`，记下 `session_before`。

### D1 列表、新建与清空

依次输入：

```text
/session list
/session new
/session
/clear
/session
```

通过条件：

- `list` 至少包含 `session_before`。
- `new` 后当前会话 ID 改变，消息数从空会话开始；`/clear` 后再次变为另一个新 ID。
- 切换后 Runner 不再向旧会话追加内容：在新会话发送 `不调用工具，只回答 SESSION-NEW-OK。`，然后 `/session list`，新会话消息数增加，旧会话不变。

### D2 恢复和删除

从 `/session list` 选择一个非当前 ID，依次输入：

```text
/session resume <non-current-id>
/session
/session delete <another-non-current-id>
```

通过条件：恢复后当前 ID 等于指定 ID，且能看到其原有对话；删除后 `list` 不再显示被删除的 ID。删除当前会话应得到可操作的拒绝提示，不得丢失当前状态。

## 场景 E：记忆命令

仅在用户级配置目录已隔离时执行本节。

### E1 查看、添加与清空确认

依次输入：

```text
/memory list
/memory add user 我偏好测试标记 MEMORY-USER-6d4e。
/memory add project 本项目测试标记 MEMORY-PROJECT-2a91。
/memory list
```

通过条件：列表显示现有索引和新增的受管记忆；仅以下目录发生变化：

```sh
find "$user_mewcode/memory" "$fixture_root/.mewcode/memory" -maxdepth 1 -type f | sort
```

输入一次 `/memory clear`。通过条件：只出现二次确认提示，文件和索引不变化。再次输入相同命令后，受管记忆 Markdown 与 `MEMORY.md` 被清空；`fixtures/`、`.mewcode/sessions/` 与其他项目文件保持不变。

### E2 非法输入

输入：

```text
/memory add unknown 不能写入
/memory add user
/memory nonsense
```

通过条件：均显示用法或可操作错误，不创建目录外文件，不改变现有记忆索引。

## 场景 F：`/compact` 与 `/review`

### F1 手动压缩

完成至少一轮普通对话后输入：

```text
/compact
```

通过条件：它启动真实模型任务并显示既有上下文压缩状态；完成后仍可继续输入。若会话上下文不足以产生可观察的 Token 缩减，记录实际反馈，不将其判为 Slash Command 分流失败。

### F2 代码审查快捷方式

在 fixture 根目录初始化一个小型 Git 变更：

```sh
printf 'review candidate\n' > "$fixture_root/review.txt"
git -C "$fixture_root" init
git -C "$fixture_root" add review.txt
git -C "$fixture_root" commit -m baseline
printf 'review candidate changed\n' > "$fixture_root/review.txt"
```

回到 TUI 输入：

```text
/review 特别关注并发安全与错误处理。
```

通过条件：该命令启动普通 Agent 请求；模型审查当前 diff，并将附加关注点反映在结果中。它不是本地反馈，可能读取文件或运行只读 Git 命令；只允许 fixture 内的请求。

## 结果记录模板

| 场景 | 实际命令/输入 | TUI 证据 | 文件或会话证据 | 结果 |
|---|---|---|---|---|
| A 命令发现与补全 |  | 帮助、候选、未知命令反馈 | 消息数不变 | 通过/失败 |
| B 本地与普通分流 |  | 本地即时反馈、一次真实 Agent 回复 | 会话消息数变化 | 通过/失败 |
| C Plan/Do |  | `[PLAN]`、`[DEFAULT]`、任务状态 | fixture 未写入 | 通过/失败 |
| D 会话管理 |  | ID 与错误提示 | sessions 列表变化 | 通过/失败 |
| E 记忆管理 |  | 摘要、确认与用法提示 | 仅 memory 目录变化 | 通过/失败 |
| F compact/review |  | 任务启动与完成状态 | Git diff 保持可见 | 通过/失败 |

## 清理

退出 MewCode 后执行：

```sh
rm -rf "$fixture_root" "$test_home"
rm -f /private/tmp/mewcode-ch10
```

若运行环境未使用隔离 `HOME`，不能执行用户目录删除命令；只移除本方案确认创建的测试记忆文件，并先核对它们不属于既有数据。
