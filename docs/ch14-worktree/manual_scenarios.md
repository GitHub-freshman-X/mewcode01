# 第十四章 Git Worktree 文件系统隔离人工测试方案

## 目标与范围

本方案使用独立 Git fixture、隔离用户配置目录和真实 Provider，验证第十四章可见行为：`/worktree` 生命周期、创建后初始化、显式工作目录切换、会话恢复、删除保护、过期临时目录清理，以及 Fork 和定义式 SubAgent 的 Worktree 隔离。

本方案不替代自动化测试。名称校验组合、Git 子进程环境、快速恢复是否完全不启动 Git、并发锁、缓存键和模型工具调用参数，仍以单元/集成测试为决定性证据。模型未调用指定工具时，记录“模型未调用目标工具”，不要在真实项目中替代执行。

## 安全约束

- 只操作本方案创建且以 `/private/tmp/mewcode-ch14-manual.` 开头的 fixture；不得在本仓库、真实项目或真实用户配置目录中测试。
- 使用专用、限额的真实 API 凭据。记录中不得包含 API key、完整提示词、模型回复、会话 JSONL、日志正文或本地配置内容。
- 只允许 fixture 内文件和 `.mewcode/worktrees/` 被写入。拒绝模型发起的外部网络、包安装、越界路径或破坏性命令。
- `--discard` 会丢弃未提交内容，仅能用于本方案刚创建的 Worktree，且必须先观察一次拒绝删除。

## 准备 fixture、角色和启动配置

在项目根目录执行。以下命令创建独立 Git 仓库、被忽略的运行文件、依赖目录和 hooks：

```sh
project_root=$(git rev-parse --show-toplevel)
fixture_root=$(mktemp -d /private/tmp/mewcode-ch14-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch14-home.XXXXXX)
case "$(go env GOOS)" in
  darwin) user_mewcode="$test_home/Library/Application Support/mewcode" ;;
  *) user_mewcode="$test_home/.config/mewcode" ;;
esac
binary=/private/tmp/mewcode-ch14

mkdir -p "$fixture_root/.mewcode/agents" "$fixture_root/.githooks" "$fixture_root/node_modules" "$user_mewcode"
printf 'MAIN-SAME-FILE-41ac\n' > "$fixture_root/same.txt"
printf 'LOCAL-CONFIG-7e31\n' > "$fixture_root/.local.env"
printf 'IGNORED-RUNTIME-93ad\n' > "$fixture_root/runtime.json"
printf 'dependency fixture\n' > "$fixture_root/node_modules/fixture.txt"
printf '#!/bin/sh\nexit 0\n' > "$fixture_root/.githooks/pre-commit"
chmod +x "$fixture_root/.githooks/pre-commit"
printf '.local.env\nruntime.json\nnode_modules/\n' > "$fixture_root/.gitignore"
printf '# ignored runtime input\nruntime.json\n' > "$fixture_root/.worktreeinclude"

git -C "$fixture_root" init
git -C "$fixture_root" config user.email manual@example.invalid
git -C "$fixture_root" config user.name 'MewCode Manual Test'
git -C "$fixture_root" config core.hooksPath .githooks
git -C "$fixture_root" add . && git -C "$fixture_root" commit -m baseline
```

创建一个共享角色和一个要求 Worktree 隔离的角色：

```sh
cat > "$fixture_root/.mewcode/agents/shared-probe.md" <<'EOF'
---
name: shared-probe
description: 验证共享工作区角色。
tools: [read_file]
model: inherit
maxTurns: 3
permissionMode: relaxed
---
读取 same.txt 后只报告 SHARED-PROBE-2ad8。不要写文件或创建子 Agent。
EOF

cat > "$fixture_root/.mewcode/agents/worktree-probe.md" <<'EOF'
---
name: worktree-probe
description: 验证声明式 Worktree 隔离。
tools: [read_file, write_file]
model: inherit
maxTurns: 5
permissionMode: relaxed
isolation: worktree
---
你处于隔离 Worktree。重新读取 same.txt；若任务要求写入，只写入指定标记，然后简短报告。不要创建子 Agent。
EOF
```

复制已验证可用的真实 Provider 配置为 `<real-config.yaml>`，并包含：

```yaml
max_tokens: 2048
thinking:
  enabled: false
agent:
  max_iterations: 20
  enable_verification_agent: false
permissions:
  mode: default
worktree:
  local_files: [".local.env"]
  symlink_directories: ["node_modules"]
  retention_hours: 1
```

构建并启动（`HOME` 必须隔离）：

```sh
go -C "$project_root" build -o "$binary" ./cmd/mewcode
cd "$fixture_root"
HOME="$test_home" "$binary" --config <real-config.yaml>
```

执行 `/status`，记录初始工作目录、会话 ID 和日志目录。初始工作目录必须为 `$fixture_root`。

## 场景 A：创建、列出与快速恢复

在 TUI 依次输入：

```text
/worktree create review/a
/worktree list
/worktree create review/a
```

通过条件：首次返回 `.mewcode/worktrees/review+a`、`worktree-review+a` 和非空 HEAD；列表显示相同名称、路径和分支；第二次创建返回同一目录与 HEAD，未创建第二份目录。

在另一终端检查：

```sh
git -C "$fixture_root" worktree list
git -C "$fixture_root" branch --list 'worktree-review+a'
test -f "$fixture_root/.mewcode/worktrees/review+a/.git"
```

通过条件：主目录与 Worktree 都在 `git worktree list` 中，分支存在，`.git` 是指针文件。不要编辑该指针文件。

## 场景 B：安全名称拒绝

在 TUI 分别输入：

```text
/worktree create .
/worktree create ..
/worktree create team//bad
/worktree create ../escape
/worktree create bad name
/worktree create aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
```

每次都应有校验错误或用法提示，不能卡住。随后运行：

```sh
git -C "$fixture_root" worktree list
git -C "$fixture_root" branch --list 'worktree-*'
find "$fixture_root/.mewcode/worktrees" -mindepth 1 -maxdepth 1 -print 2>/dev/null
```

通过条件：只存在场景 A 的 `review+a`。若任何非法输入创建目录、分支或 fixture 外路径，立即停止并记录为安全失败。

## 场景 C：创建后初始化

输入 `/worktree create init-check`，然后在终端执行：

```sh
wt="$fixture_root/.mewcode/worktrees/init-check"
cmp "$fixture_root/.local.env" "$wt/.local.env"
cmp "$fixture_root/runtime.json" "$wt/runtime.json"
test -L "$wt/node_modules"
test "$(readlink "$wt/node_modules")" = "$fixture_root/node_modules"
test "$(git -C "$wt" config core.hooksPath)" = .githooks
```

通过条件：本地配置和 `.worktreeinclude` 文件已复制，依赖目录是指向主工作区的软链接，hooks 路径继承。删除 `$fixture_root/.local.env` 后再创建 `init-missing` 也必须成功，证明可选初始化失败不破坏 Worktree；随后立刻用 `printf 'LOCAL-CONFIG-7e31\n' > "$fixture_root/.local.env"` 恢复它。

## 场景 D：进入、显式 cwd 和退出

在 TUI 输入：

```text
/worktree enter review/a
/status
只使用 write_file 将 same.txt 写为 WORKTREE-A-6e52；完成后停止，不调用命令。
```

允许一次 fixture 内写入。通过条件：`/status` 显示 `$fixture_root/.mewcode/worktrees/review+a`；启动终端的 `pwd` 不变；随后检查主目录仍为 `MAIN-SAME-FILE-41ac`，而 Worktree 文件为 `WORKTREE-A-6e52`：

```sh
cat "$fixture_root/same.txt"
cat "$fixture_root/.mewcode/worktrees/review+a/same.txt"
```

输入 `/worktree exit` 和 `/status`。通过条件：状态回到 `$fixture_root`，`worktree_session.json` 被清除，Worktree 目录保留。

## 场景 E：会话恢复与删除保护

再次输入 `/worktree enter review/a`，确认 session 文件存在：

```sh
test -f "$fixture_root/.mewcode/worktree_session.json"
```

退出并用同一命令重启。通过条件：第一次 `/status` 已显示 Worktree 路径。执行 `/worktree exit` 后重启一次，状态应回到主目录，且 `/worktree list` 仍可见 `review/a`。

先删除干净目录：`/worktree remove init-check`；目录和 `worktree-init-check` 分支都应消失。然后执行 `/worktree enter review/a`、`/worktree remove review/a`，应因当前会话拒绝；退出后再次删除，必须因未提交修改拒绝并提示 `--discard`。检查 `same.txt` 仍在且 `git status --porcelain` 非空。最后只对这个 fixture 输入：

```text
/worktree remove review/a --discard
```

通过条件：目录与分支都被删除。

## 场景 F：定义式、Fork 与共享 SubAgent

先创建 `/worktree create observer`。在 TUI 输入：

```text
必须调用一次 agent 工具，使用 subagent_type="worktree-probe"、run_in_background=false。委派任务：重新读取 same.txt，然后仅使用 write_file 将它改为 DEFINITION-WORKTREE-8c14；完成后简短报告。不要由主 Agent 自己写文件。
```

允许该 Agent 调用和 fixture 内写入。通过条件：子任务结果或通知包含保留 Worktree 的路径和分支；主目录的 `same.txt` 仍为 `MAIN-SAME-FILE-41ac`；`find "$fixture_root/.mewcode/worktrees" -maxdepth 1 -type d -name 'tmp-*' -print` 至少显示一个有修改而被保留的临时目录。

然后请求 Fork：

```text
必须调用一次 agent 工具，不提供 subagent_type（Fork）。委派任务：只读取 same.txt，报告是否能看到文件；不要写入、不要创建更多子 Agent。调用后立即只报告 task_id，不要等待。
```

通过条件：即使未传 isolation，也异步启动；终态通知到达后，新建的干净 `tmp-*` 目录自动消失。最后运行：

```text
必须调用一次 agent 工具，使用 subagent_type="shared-probe"、run_in_background=false。只读取 same.txt 后返回角色规定的唯一标记。
```

通过条件：前台完成、没有新增 `tmp-*` Worktree，并返回 `SHARED-PROBE-2ad8` 或等价的角色报告。模型不调用 `agent` 时，记录实际行为。

## 场景 G：过期临时目录、日志与脱敏

创建 `/worktree create tmp-expired` 并退出。仅对这一干净临时目录回拨两小时（macOS）：

```sh
touch -t "$(date -v-2H +%Y%m%d%H%M.%S)" "$fixture_root/.mewcode/worktrees/tmp-expired"
```

重新启动，因 `retention_hours: 1`，`tmp-expired` 应被删除。将场景 F 中有修改的 `tmp-*` 目录同样回拨后重启，它必须保留，证明清理 fail-closed。

只检查日志字段和值，不复制正文：

```sh
find "$fixture_root/logs" -type f -name '*.jsonl' -print
rg -n '"stage":"worktree_(create|recovery|enter|exit|remove|cleanup)"' "$fixture_root/logs"
rg -n 'DEFINITION-WORKTREE-8c14|LOCAL-CONFIG-7e31|IGNORED-RUNTIME-93ad' "$fixture_root/logs" || true
```

通过条件：存在安全的 `stage`、`status`、`kind` 等生命周期字段；最后一条命令无匹配。日志不得含任务正文、文件内容、密钥、请求头或原始错误载荷。

## 结果记录模板

| 场景 | TUI 证据 | 文件、Git 或日志证据 | 结果 |
|---|---|---|---|
| A 创建与恢复 | 路径、分支、HEAD、列表 | `git worktree list` | 通过/失败 |
| B 名称安全 | 校验错误或用法 | 无额外目录、分支或越界路径 | 通过/失败 |
| C 初始化 | 创建成功 | `cmp`、软链接、hooks config | 通过/失败 |
| D 显式 cwd | `/status` 路径切换 | 同名文件内容隔离 | 通过/失败 |
| E 恢复与删除 | 恢复、拒绝、最终删除 | session、目录和分支 | 通过/失败 |
| F SubAgent | task ID、通知或路径 | `tmp-*` 保留或删除 | 通过/失败/模型未调用 |
| G 清理与日志 | 重启后可继续使用 | 清理规则和日志脱敏 | 通过/失败 |

## 清理

退出 MewCode、确认后台任务已有终态后，先检查目标：

```sh
printf 'fixture_root=%s\ntest_home=%s\nbinary=%s\n' "$fixture_root" "$test_home" "$binary"
```

仅当 `fixture_root`、`test_home` 分别以 `/private/tmp/mewcode-ch14-manual.`、`/private/tmp/mewcode-ch14-home.` 开头，且 `binary` 恰为 `/private/tmp/mewcode-ch14` 时，才执行：

```sh
rm -rf "$fixture_root" "$test_home"
rm -f "$binary"
```

变量为空或路径不符合临时模式时停止清理并手工确认；不得删除真实用户目录、项目目录或未确认 Worktree。
