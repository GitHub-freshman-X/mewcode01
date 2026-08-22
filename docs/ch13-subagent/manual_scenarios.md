# 第十三章 SubAgent 子任务分发人工测试方案

## 目标与范围

本方案在独立 fixture 工作区、隔离用户配置目录和真实 Provider 下，验证第十三章的用户可见行为：定义发现与项目级覆盖、`Verification` 开关、定义式与 Fork 创建、显式/ESC/120 秒自动后台，以及后台终态通知和能力边界。

本方案不替代自动化测试。定义解析错误、工具过滤的全部组合、Fork 历史补全、Token 归集、并发状态迁移、取消竞争和日志字段脱敏仍由自动化测试负责。真实模型可能不按要求调用 `agent` 工具；遇到这种情况应记录“模型未调用目标 Agent 工具”，不要将其改为在真实工作区手工执行。

## 安全约束

- 只在本方案创建的 fixture 中运行；不得在项目仓库、真实项目或真实用户配置目录创建 Agent 定义。
- 使用专用、限额的真实 API 凭据。不得记录 API key、完整模型请求、完整子 Agent 回复、会话 JSONL 或日志正文。
- 遇到主 Agent 调用 `agent` 工具的权限确认时，只允许本次调用（`o`）；不要选择永久允许。除慢任务场景明确允许的 `sleep` 外，拒绝其他命令或写入请求。
- 本方案中的后台任务不跨进程持久化。退出前必须等待已启动的后台任务结束；不要通过杀进程来“取消”任务。
- 人工记录只保留角色名、任务 ID、状态、唯一标记和结论。不要复制任务提示或通知中的完整结果。

## 准备 fixture、定义与真实启动配置

以下命令以 macOS 的 POSIX shell 为例，fixture 固定创建在既有章节使用的 `/private/tmp`。Windows 或 Linux 请创建等价的隔离目录与用户配置目录；不要使用真实用户配置目录。

在项目根目录执行：

```sh
project_root=$(git rev-parse --show-toplevel)
fixture_root=$(mktemp -d /private/tmp/mewcode-ch13-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch13-home.XXXXXX)
user_config_root="$test_home/Library/Application Support"
user_agents="$user_config_root/mewcode/agents"
project_agents="$fixture_root/.mewcode/agents"
binary="/private/tmp/mewcode-ch13"

mkdir -p "$user_agents" "$project_agents" "$fixture_root/fixtures"
printf 'SUBAGENT-FIXTURE-ALPHA-7e31\n' > "$fixture_root/fixtures/alpha.txt"
printf 'fixture_root=%s\ntest_home=%s\nuser_agents=%s\n' \
  "$fixture_root" "$test_home" "$user_agents"
```

创建用户级和项目级同名角色，以及供 ESC 和自动后台场景使用的慢角色。两个 `marker-role` 正文的唯一标记不同；项目级定义应覆盖用户级定义。`slow-probe` 仅允许执行无副作用的 `sleep`，用于让前台子 Agent 保持运行状态。

```sh
cat > "$user_agents/marker-role.md" <<'EOF'
---
name: marker-role
description: 返回用户级覆盖测试标记。
tools: [read_file]
model: inherit
maxTurns: 3
permissionMode: relaxed
---
你是用户级覆盖测试角色。完成任务所要求的只读检查后，最终回复必须且只能为 USER-AGENT-PROMPT-19e4；不得包含 fixture 内容、摘要或任何其他文本。不要修改文件，也不要创建子 Agent。
EOF

cat > "$project_agents/marker-role.md" <<'EOF'
---
name: marker-role
description: 返回项目级覆盖测试标记。
tools: [read_file]
model: inherit
maxTurns: 3
permissionMode: relaxed
---
你是项目级覆盖测试角色。完成任务所要求的只读检查后，最终回复必须且只能为 PROJECT-AGENT-PROMPT-72bc；不得包含 fixture 内容、摘要或任何其他文本。不要修改文件，也不要创建子 Agent。
EOF

cat > "$project_agents/slow-probe.md" <<'EOF'
---
name: slow-probe
description: 用无副作用延时观察前台和后台任务。
tools: [run_command]
model: inherit
maxTurns: 8
permissionMode: relaxed
---
你是人工验收的慢任务角色。仅在任务明确要求时，顺序调用 run_command 执行 sleep 25；每次命令完成后再执行下一次。不得写文件、不得创建子 Agent，完成后只给出简短状态报告。
EOF
```

复制一份已验证可用的真实 Provider 配置为 `<real-config.yaml>`。保留连接所需字段，并确保它包含以下配置；可关闭 thinking、降低 `max_tokens` 以控制成本：

```yaml
agent:
  max_iterations: 20
  enable_verification_agent: false
permissions:
  mode: default
```

构建并从 fixture 启动。必须通过 `HOME` 隔离用户级目录；`--config` 可以指向 fixture 外的真实测试配置，但不能是包含真实用户数据的默认配置。

```sh
go -C "$project_root" build -o "$binary" ./cmd/mewcode
cd "$fixture_root"
HOME="$test_home" "$binary" --config <real-config.yaml>
```

启动后执行 `/status`，记录当前工作目录、会话 ID 和日志目录。若显示的工作目录不是 `$fixture_root`，立即退出并修正启动位置。

## 场景 A：定义发现与覆盖顺序

在 TUI 输入以下请求；`marker-role` 的大小写和拼写必须保持一致：

```text
必须调用一次 agent 工具，使用 subagent_type="marker-role" 创建定义式子 Agent。委派它只执行只读检查：读取 fixtures/alpha.txt。子 Agent 的最终报告格式和覆盖测试标记完全由 `marker-role` 的角色定义决定；不要在委派任务中猜测、指定、转述或改写任何标记。等待子 Agent 完成后，只报告它返回的唯一标记；不要自行读取文件。
```

若有权限确认，只允许本次 `agent` 调用。先检查可见的 `agent` 调用参数：`prompt` 不得包含任何具体覆盖标记（包括 `MARKER_READONLY_OK`）或要求返回 fixture 内容；若出现，记录为“主 Agent 改写委派任务”，本轮结果无效并以新会话重试。通过条件：

- 主 Agent 调用了 `agent` 工具，且前台调用结束后可见子 Agent 的结果。
- `agent` 工具结果中的 `data.result` 必须恰好等于 `PROJECT-AGENT-PROMPT-72bc`；不得包含 `USER-AGENT-PROMPT-19e4`、`SUBAGENT-FIXTURE-ALPHA-7e31` 或其他文本。这同时证明项目级定义覆盖了用户级定义，且子 Agent 遵守角色的排他输出契约。
- 子 Agent 不应请求写入或创建更多子 Agent；若出现此类请求，拒绝并记录实际工具名。

退出 MewCode，删除项目级同名定义后重新启动：

```sh
rm -f "$project_agents/marker-role.md"
cd "$fixture_root"
HOME="$test_home" "$binary" --config <real-config.yaml>
```

重复相同请求。通过条件：`agent` 工具结果中的 `data.result` 恰好等于 `USER-AGENT-PROMPT-19e4`。定义只在启动时发现，因此删除文件后必须重启；本章没有 Agent 定义热刷新命令。

为后续场景恢复项目级定义：

```sh
cp "$user_agents/marker-role.md" "$project_agents/marker-role.md"
python3 - <<'PY'
from pathlib import Path
path = Path(".mewcode/agents/marker-role.md")
path.write_text(path.read_text().replace("USER-AGENT-PROMPT-19e4", "PROJECT-AGENT-PROMPT-72bc").replace("用户级覆盖测试", "项目级覆盖测试"), encoding="utf-8")
PY
```

重启后再继续；不要假定运行中的进程会读取恢复后的文件。

## 场景 B：内置角色与 `Verification` 开关

当前配置保持 `enable_verification_agent: false`。在 TUI 输入：

```text
必须调用一次 agent 工具，使用 subagent_type="Verification" 创建定义式子 Agent。委派任务：不调用其他工具，完成后只给出一句简短的任务完成报告。等待工具返回后，仅根据 `agent` 工具的结构化返回判断并报告调用是否成功；若工具返回失败，直接说明失败原因；不要改用其他角色。
```

通过条件：`agent` 工具返回未知 `Verification` 类型或等价的不可用诊断；主会话仍可继续输入。此处不要求模型逐字复述错误，但不得静默改用其他角色。

退出程序，在 `<real-config.yaml>` 中将开关改为 `true` 后重新启动：

```yaml
agent:
  max_iterations: 20
  enable_verification_agent: true
```

重复上述请求。通过条件：`agent` 工具的结构化返回为成功（`success: true`），且包含完成状态和任务 ID，证明名为 `Verification` 的定义式子 Agent 已成功启动并得到完成结果。不要将子 Agent 最终文本中对自身“是否可用”的陈述作为类型可用性的证据；角色是否加载只能由 `agent` 工具是否成功解析并启动该类型判断。该角色的系统职责是只读验证；本场景不授权写入或破坏性命令。

完成本场景后将测试配置恢复为 `false`，退出并重启后再继续，以免后续测试或日常使用意外增加可用角色。

## 场景 C：显式后台与 Fork 强制后台

先确保没有仍在运行的慢任务。启动后输入以下定义式后台请求：

```text
必须调用一次 agent 工具，使用 subagent_type="marker-role"、run_in_background=true、name="manual-defined-background"。任务是只报告覆盖测试标记。调用后不要等待结果，先告诉我工具返回的 task_id。
```

通过条件：工具结果包含 `status: async_launched`、非空 `task_id` 和指定名称；主 Agent 不会等待子 Agent 的最终文本才继续。记下任务 ID，但不要记录完整结果。

再发送一条普通文本，建立可供 Fork 继承的父会话历史：

```text
不调用工具，只回答：FORK-PARENT-CONTEXT-42e8。
```

随后输入：

```text
必须调用一次 agent 工具，但不要提供 subagent_type，以 Fork 方式委派任务“只报告你继承到的唯一父会话标记”。设置 name="manual-fork-background"。调用后立即报告 task_id；不要等待子 Agent 完成。
```

先检查可见的 `agent` 调用参数：`subagent_type` 可以完全缺失，也可以为 `"fork"`（大小写和首尾空白均兼容）；两种写法都必须走 Fork。其他值按定义式角色查找；若模型传入其他值并导致未知类型，应记录为“模型未按要求创建 Fork”，本轮无效并以新会话重试。

通过条件：

- Fork 调用也立即返回不同的 `task_id` 和 `async_launched`，即使请求中没有设置 `run_in_background: true`。
- 两个后台任务完成后，TUI 显示含任务名称和终态的系统通知；通知可以在主 Agent 原请求结束后到达。
- 在通知到达后再发送普通请求，要求它概括两个后台任务的状态。主 Agent 能继续处理输入，并能根据下一轮收到的任务通知说明完成、失败或取消状态。

真实模型可能在 Fork 中不复述 `FORK-PARENT-CONTEXT-42e8`；这只记录为“模型未复述继承上下文”，不否定 Fork 强制后台。历史前缀保持和未完成工具调用补齐由自动化测试验收。

## 场景 D：ESC 将前台子 Agent 转入后台

以配置中 `enable_verification_agent: false`（或 true，均不影响本场景）重启一个干净进程。在 TUI 输入：

```text
必须调用一次 agent 工具，使用 subagent_type="slow-probe"、run_in_background=false、name="manual-esc-background"。委派任务：顺序执行两次 sleep 25，每次都必须通过 run_command；完成后只报告完成。不要等待我确认。
```

允许主 Agent 的一次 `agent` 调用。通用工具默认 30 秒期限不得在按 ESC 前截断该前台子 Agent；看到状态栏出现“子 Agent 前台运行 · ESC 转后台”且慢角色正在执行第一条 `sleep` 时，按 `Esc` 一次。接管后即使原前台工具调用已返回或其 context 被取消，子 Agent 仍必须继续同一任务。

通过条件：

- TUI 显示“子 Agent 已转入后台。”；主 Agent 随后完成当前回合，输入框恢复可用。继续发送一条普通请求并获得回复后，该提示仍必须位于触发 ESC 的原回合之后、该普通请求之前，不能永久停留在聊天底部。
- 输入一条不调用工具的短问题，主 Agent 能正常接收并回答，证明后台任务未阻塞主会话。
- 等待慢角色结束后，TUI 显示 `manual-esc-background` 的 completed、failed 或 cancelled 终态通知；正常未拒绝命令时预期为 completed。
- `Ctrl+C` 仍用于取消当前主任务，不应被 `Esc` 的后台化语义替代。

如果模型未发起 `agent` 调用、未使用 `run_command`，或任务结束得快而来不及按 `Esc`，记录实际情况后重启本场景；不要用真实项目中的长命令制造延迟。

## 场景 E：120 秒自动转后台

本场景需要约两分钟，并依赖 POSIX `sleep`。确认场景 D 的后台任务已结束后，输入：

```text
必须调用一次 agent 工具，使用 subagent_type="slow-probe"、run_in_background=false、name="manual-auto-background"。委派任务：顺序执行五次 sleep 25，每次都必须通过 run_command；完成后只报告完成。不要等待我确认。整个过程中我不会按 Esc。
```

允许一次主 `agent` 调用后，不要按 `Esc` 或 `Ctrl+C`。通用 30 秒工具期限不得提前截断该前台等待；约 120 秒后通过条件：

- 前台等待自动结束，主 Agent 工具结果以异步任务 ID 返回；该子 Agent 没有从头重新开始。
- 主回合结束后输入框可用；在第五次 `sleep` 完成后，TUI 显示 `manual-auto-background` 的终态通知。
- 若记录了每次调用，`sleep` 总次数最多为五；自动后台化不能重复执行已经开始的子任务。

若 Provider、权限或模型行为使五次顺序调用无法稳定完成，记录为“真实 Provider 场景未完成”；120 秒阈值、同一任务接管和不重启由可注入时钟的自动化测试作为决定性证据。

## 场景 F：隔离与递归能力边界

在 fixture 中记录当前目录清单（忽略运行时正常持久化的会话 JSONL），随后在 TUI 输入：

```sh
find "$fixture_root" -maxdepth 3 \
  -path "$fixture_root/.mewcode/sessions" -prune -o -print | sort \
  > /private/tmp/mewcode-ch13-before-worktree.txt
```

```text
必须调用一次 agent 工具，使用 subagent_type="marker-role"，并设置 isolation="worktree"。任务可以是任意只读检查。若失败，直接报告工具错误；不得自行创建 Git worktree 或改用 isolation="none"。
```

通过条件：`agent` 工具返回“worktree isolation is not supported in this chapter”或等价的明确未支持错误。比较目录清单：

```sh
find "$fixture_root" -maxdepth 3 \
  -path "$fixture_root/.mewcode/sessions" -prune -o -print | sort \
  > /private/tmp/mewcode-ch13-after-worktree.txt
diff -u /private/tmp/mewcode-ch13-before-worktree.txt \
  /private/tmp/mewcode-ch13-after-worktree.txt
```

MewCode 会在 fixture 的 `.mewcode/sessions/` 持久化当前主会话；该目录中新出现的 JSONL 是正常运行产物，不能作为隔离目录证据。通过：`diff` 无输出，且没有创建 Worktree 或其他隔离工作目录。

最后输入：

```text
必须调用一次 agent 工具，使用 subagent_type="marker-role"。委派任务：在子 Agent 内再次调用 agent 工具，创建一个 general-purpose 子 Agent，并报告调用结果。不要由主 Agent 自己创建第二个子 Agent。
```

通过条件：定义式子 Agent 不具备 `agent` 工具，因而无法递归派生；主会话仍保持可用。可再以 `run_in_background=true` 重复一次，验证后台子 Agent 同样不能派生。若模型没有尝试内部调用，记录“模型未尝试递归调用”；工具集强制过滤由自动化测试验收。

## 结果记录模板

| 场景 | 输入/配置 | TUI 证据 | 文件或状态证据 | 结果 |
|---|---|---|---|---|
| A 定义覆盖 |  | 子 Agent 结果中的唯一标记 | 重启后项目级→用户级切换 | 通过/失败/模型未调用 |
| B Verification | 开关 false / true | 未知类型错误 / `agent` 结构化成功、完成状态和任务 ID | 配置字段已切换并恢复 | 通过/失败/模型未调用 |
| C 显式与 Fork 后台 |  | 两个 task ID、终态通知、可继续输入 | ID 不同，Fork 强制异步 | 通过/失败/模型未调用 |
| D ESC 后台化 |  | 前台状态、ESC 提示、输入恢复 | `manual-esc-background` 终态 | 通过/失败/模型未调用 |
| E 自动后台化 |  | 约 120 秒后异步返回、终态通知 | 不重复启动，最多五次 sleep | 通过/失败/未完成 |
| F 边界 | worktree / 递归请求 | 明确未支持、递归不可用 | Worktree 前后目录无变化 | 通过/失败/模型未尝试 |

## 清理

退出 MewCode，并确认所有慢任务都已有终态通知后，只删除本方案创建的路径：

```sh
printf 'fixture_root=%s\ntest_home=%s\nbinary=%s\n' \
  "$fixture_root" "$test_home" "$binary"
```

只有当 `fixture_root` 和 `test_home` 分别以 `/private/tmp/mewcode-ch13-manual.`、`/private/tmp/mewcode-ch13-home.` 开头，且 `binary` 等于 `/private/tmp/mewcode-ch13` 时，才执行：

```sh
rm -rf "$fixture_root" "$test_home"
rm -f "$binary" \
  /private/tmp/mewcode-ch13-before-worktree.txt \
  /private/tmp/mewcode-ch13-after-worktree.txt
```

若变量为空、路径不符合上述临时目录模式，停止清理并手工确认；不得删除真实用户配置或项目目录。
