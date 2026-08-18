# 第十一章 Skill System 手工测试方案

## 目标与当前范围

本方案在独立 fixture 工作区和真实 Provider 下验证 Skill 的发现、覆盖、轻量目录、按需加载、显式命令、`{{args}}`、工具白名单、手动刷新及会话清理。

> 当前实现状态：inline、fork、动态命令和刷新均已接入。所有场景仍须按本方案完成真实 Provider 验收。

## 安全约束

- 只在临时工作区执行，任何写入或 Git 提交请求均拒绝。
- 使用限额测试凭据；不记录 API key、完整 SOP、模型请求、工具结果正文或日志正文。
- 用户级目录会由 `os.UserConfigDir()` 决定。macOS 测试必须使用隔离 `HOME`；若系统未隔离该目录，不运行用户级覆盖场景。

## 准备

```sh
fixture_root=$(mktemp -d /private/tmp/mewcode-ch11-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch11-home.XXXXXX)
project_skills="$fixture_root/.mewcode/skills"
user_skills="$test_home/Library/Application Support/mewcode/skills"
mkdir -p "$project_skills" "$user_skills" "$fixture_root/fixtures"
printf 'SKILL-FIXTURE-ALPHA-81ad\n' > "$fixture_root/fixtures/alpha.txt"

cat > "$user_skills/commit.md" <<'EOF'
---
name: commit
description: 用户级提交流程。
mode: inline
---
USER-COMMIT-SOP-19e4 {{args}}
EOF

cat > "$project_skills/commit.md" <<'EOF'
---
name: commit
description: 项目级提交流程。
mode: inline
---
PROJECT-COMMIT-SOP-72bc {{args}}
EOF

mkdir -p "$project_skills/read-alpha/examples"
cat > "$project_skills/read-alpha/SKILL.md" <<'EOF'
---
name: read-alpha
description: 读取 fixture 中的 alpha 标记。
mode: inline
tools: [read_file]
---
READ-ALPHA-SOP-34fa {{args}}
EOF
printf 'RESOURCE-MUST-NOT-BE-IN-FIRST-PROMPT-6e2d\n' > "$project_skills/read-alpha/examples/example.md"
```

在项目根目录构建并从 fixture 启动。`<real-config.yaml>` 应是可用的 Provider 配置，建议关闭 thinking：

```sh
go build -o /private/tmp/mewcode-ch11 /Users/xuchangan/Project/mine/mew-agent/mew01/cmd/mewcode
cd "$fixture_root"
HOME="$test_home" /private/tmp/mewcode-ch11 --config <real-config.yaml>
```

记录启动后的 `/help`、`/skills reload` 反馈和测试所用 commit。日志检查只记录字段名及是否泄露标记，不复制日志全文。

## 场景 A：发现、优先级与命令目录

输入 `/help`，并在输入框分别键入 `/co`、`/re` 后按 Tab。

通过条件：

- `/commit`、`/review`、`/test` 三个内置样板，以及 `/read-alpha` 均在帮助和补全中。
- `/commit` 的说明为“项目级提交流程”，证明项目级定义覆盖用户级与内置定义。
- `examples/example.md` 不会成为命令，也不应因发现阶段被加载。

## 场景 B：显式 inline 调用与参数替换

输入：

```text
/commit 只检查状态，不要创建提交。
```

通过条件：任务启动为 Agent 请求；模型行为遵循项目级定义而非用户级或内置定义，且附加要求被纳入任务。允许模型读取 Git 状态；任何提交或写入请求均拒绝。

再输入：

```text
/read-alpha 请只报告文件中的唯一标记。
```

若出现权限确认，只允许一次 `read_file`。通过条件：结果包含 `SKILL-FIXTURE-ALPHA-81ad`；未列入白名单的写入工具不应被调用。

## 场景 C：自然语言按需加载

新启动一个进程以获得空的激活状态，输入：

```text
读取 fixtures/alpha.txt 并报告唯一标记。
```

通过条件：模型可从轻量目录发现 `read-alpha` 并调用 `load_skill`；加载工具的可见结果只有名称、说明和模式，不能显示 `READ-ALPHA-SOP-34fa`、参数或 examples 内容。下一轮才依照 SOP 读取文件并返回标记。

若真实模型未自主选择该 Skill，记录为“模型未匹配”，不据此否定目录注入；两阶段请求内容与加载工具脱敏由自动化测试作确定性验收。

## 场景 D：刷新成功与原子失败

在另一个终端创建新 Skill：

```sh
cat > "$project_skills/echo-skill.md" <<'EOF'
---
name: echo-skill
description: 回显额外要求。
mode: inline
---
ECHO-SOP {{args}}
EOF
```

在 TUI 输入 `/skills reload`，随后输入 `/echo-skill HELLO-SKILL-4a7c`。通过条件：刷新成功提示出现，`/echo-skill` 出现在帮助中，任务能接收附加参数。

将该文件改为非法内容（例如删除 `description`），再次输入 `/skills reload`。通过条件：出现含路径或字段的诊断；此前的 `/echo-skill` 命令和已激活 Skill 仍保持可用。恢复合法文件并再次刷新。

## 场景 E：会话切换清除激活状态

先执行 `/read-alpha 保持激活`，完成后输入 `/clear`；在新会话输入无关的普通问题，例如“只回答 SKILL-CLEAR-OK，不调用工具”。

通过条件：新会话仍可从 `/help` 看到 `/read-alpha`，但不会继承上一会话的已激活 SOP 或参数。重复 `/session new`、`/session resume <id>` 时也应遵循同一规则。

## 场景 F：fork 独立审查

在 fixture 中初始化一个 Git 变更后执行 `/review`。完成实现后通过条件：fork 请求按 `context: none` 不携带主会话消息；主会话只追加最终审查摘要，Token 用量仍增加；取消或失败不得生成伪摘要。

通过条件：不会出现“fork skills are not implemented”；中间工具调用和回复不会写入主会话；完成后主会话只显示 `/review` 与最终审查摘要。

## 结果记录模板

| 场景 | 输入/命令 | 可见证据 | 安全/文件证据 | 结果 |
|---|---|---|---|---|
| A 发现与覆盖 |  | help、Tab、说明 | 目录入口与资源未注册 | 通过/失败 |
| B inline 与白名单 |  | Agent 与最终答复 | 只读工具请求 | 通过/失败 |
| C 按需加载 |  | load_skill 后继续 | 不回显 SOP 或资源 | 通过/失败/模型未匹配 |
| D 刷新 |  | 成功和失败诊断 | 旧命令保留 | 通过/失败 |
| E 会话清理 |  | 新会话 help 与答复 | 不继承激活 SOP | 通过/失败 |
| F fork |  | 摘要与 Token | 主历史隔离 | 通过/失败 |

## 清理

退出后删除本方案创建的临时文件：

```sh
rm -rf "$fixture_root" "$test_home"
rm -f /private/tmp/mewcode-ch11
```
