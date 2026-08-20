# 第十二章 Hook 生命周期钩子人工测试方案

## 目标与范围

本方案在独立 fixture 工作区和真实 Provider 下，验证 Hook 的用户可见行为：三层配置追加、条件匹配、工具前拦截、工具后动作、提示词通知、`once`、异步 HTTP 故障隔离、Slash Command 事件、配置诊断和日志脱敏。

本方案不替代自动化测试。正则/Glob 边界、并发竞争、命令超时、HTTP 状态编码、结构化工具结果、上下文压缩和取消竞争继续由自动化测试负责。真实模型可能不按预期选择工具；遇到这种情况应记录“模型未调用目标工具”，不要为了通过而在真实项目中执行命令。

## 安全约束

- 只在本方案创建的 fixture 工作区运行；不要在项目仓库、真实工作区或真实用户目录启用测试 Hook。
- 使用专用、限额的真实 API 凭据。不得记录 API key、完整模型请求、完整模型回复、会话 JSONL、HTTP body、HTTP header 或日志正文。
- Agent 发起文件写入或命令执行请求时，只允许 fixture 根目录内的操作；除本方案明确的 `printf` 外，拒绝所有命令。
- HTTP 场景只访问本机 loopback 测试服务或预期拒绝连接的 `127.0.0.1:1`；不得配置真实 webhook。
- 每个场景记录输入、可见状态、fixture 文件状态和日志字段名；不要复制敏感正文。

## 准备 fixture、Hook 配置与真实启动配置

在项目根目录执行。以下命令创建隔离工作区、隔离的 Hook 用户目录和测试文件：

```sh
fixture_root=$(mktemp -d /private/tmp/mewcode-ch12-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch12-home.XXXXXX)
hook_user_dir="$test_home/.mewcode"
mkdir -p "$fixture_root/.mewcode" "$fixture_root/fixtures" "$hook_user_dir"
printf 'HOOK-FIXTURE-ALPHA-7e31\n' > "$fixture_root/fixtures/alpha.txt"
printf 'package fixture\n\nfunc  main(){println("HOOK-FORMAT-42ad")}\n' > "$fixture_root/fixtures/unformatted.go"
printf 'fixture_root=%s\ntest_home=%s\n' "$fixture_root" "$test_home"
```

写入三层 Hook 配置。用户级和项目级规则都应生效；本地级规则用于验证最终追加顺序。`127.0.0.1:1` 应立即拒绝连接，适合作为不依赖外网的异步失败场景。

```sh
cat > "$hook_user_dir/config.yaml" <<'EOF'
hooks:
  - id: user-session-context
    event: session_start
    action:
      type: prompt
      message: "HOOK-SESSION-CONTEXT-19e4：回答前先确认已收到该标记。"
    once: true
EOF

cat > "$fixture_root/.mewcode/config.yaml" <<'EOF'
hooks:
  - id: format-go-after-write
    event: post_tool_use
    if: 'tool == "write_file" && args.path ~= "*.go"'
    action:
      type: command
      command: 'gofmt -w "$FILE_PATH"'
      timeout: 10s
  - id: block-fixture-command
    event: pre_tool_use
    if: 'tool == "run_command" && args.command =~ /HOOK-BLOCK-7c2a/'
    action:
      type: command
      command: "echo 'HOOK-BLOCKED-REASON-7c2a'"
    reject: true
  - id: command-notification
    event: command_execute
    action:
      type: prompt
      message: "HOOK-SLASH-COMMAND-42e8"
EOF

cat > "$fixture_root/.mewcode/config.local.yaml" <<'EOF'
hooks:
  - id: local-write-notification
    event: post_tool_use
    if: 'tool == "write_file"'
    action:
      type: http
      url: "http://127.0.0.1:1/mewcode-hook"
      method: POST
      body: '{"path":"$FILE_PATH","event":"$EVENT"}'
    async: true
EOF
```

复制一份已经验证可用的真实 Provider 配置为 `<real-config.yaml>`。它只提供 Provider、模型和权限配置，不需要包含 `hooks`。建议关闭 thinking、降低输出上限：

```yaml
max_tokens: 2048
thinking:
  enabled: false
permissions:
  mode: default
```

构建临时二进制并启动。Hook 用户级配置由 `$HOME/.mewcode/config.yaml` 读取，因此必须通过 `HOME` 隔离。若运行环境不尊重 `HOME`，跳过用户级场景；不要指向真实 `~/.mewcode`。

```sh
go build -o /private/tmp/mewcode-ch12 /Users/xuchangan/Project/mine/mew-agent/mew01/cmd/mewcode
cd "$fixture_root"
HOME="$test_home" /private/tmp/mewcode-ch12 --config <real-config.yaml>
```

启动后执行 `/status`，记录当前工作目录、会话 ID、日志目录（`$fixture_root/logs/`）和初始 Token 用量。日志目录可能尚未存在；只有发生可记录的 Hook 后才会创建。

## 场景 A：配置加载、追加顺序与 once

### A1 三层规则均被加载

在 TUI 输入：

```text
不调用工具。只回答：HOOK-SESSION-CONTEXT-19e4。
```

通过条件：

- 最终回答能反映 `HOOK-SESSION-CONTEXT-19e4`，表明用户级 `session_start` prompt 通知进入了模型请求。
- 任务正常完成，未显示配置错误。
- 在本进程中再提交一次无工具问题时，不应再次出现首次上下文注入造成的重复效果，验证 `once: true`。
- 重启程序后，`once` 状态重置；首次任务可再次收到该通知。

如果模型没有逐字复述标记，记录模型实际答复；不要仅凭文字风格判断失败。自动化测试负责确定性验证请求是否包含通知。

### A2 配置位置与错误定位

退出 TUI，在本地配置末尾临时加入：

```yaml
  - id: invalid-async-pre-tool
    event: pre_tool_use
    async: true
    action:
      type: command
      command: "true"
```

重新启动。通过条件：启动失败，错误同时包含 `config.local.yaml`、`invalid-async-pre-tool` 和 async/pre-tool 约束信息。随后删除该测试规则再重启；有效规则应恢复工作。

## 场景 B：工具后格式化和异步 HTTP 故障隔离

### B1 `post_tool_use` 格式化

在 TUI 输入：

```text
只使用 write_file 重写 fixtures/unformatted.go，保留其中的字符串 HOOK-FORMAT-42ad；完成后停止，不运行其他命令。
```

如果出现权限确认，仅允许这一次 fixture 内 `write_file`。任务完成后在另一个终端检查：

```sh
gofmt -d "$fixture_root/fixtures/unformatted.go"
grep -n 'HOOK-FORMAT-42ad' "$fixture_root/fixtures/unformatted.go"
```

通过条件：

- `gofmt -d` 无输出，且唯一标记仍存在。
- TUI 仍显示原始工具调用和结果；格式化 Hook 失败不得替换或遮蔽主工具结果。
- 本地 HTTP Hook 连接失败不应使任务失败、取消或停在权限确认状态。

### B2 异步失败日志

等待数秒让异步动作退出，再只检查日志字段和值是否出现，不复制日志正文：

```sh
find "$fixture_root/logs" -type f -name '*.jsonl' -print
rg -n '"stage":"hook"|"rule_id":"local-write-notification"|"status":"failed"' "$fixture_root/logs"
```

通过条件：至少一条记录含 Hook 阶段、规则标识、`http` 动作、失败状态和安全元数据；日志不应包含 HTTP body 中的路径全文、请求头或 API key。

## 场景 C：工具前拦截与模型反馈

在 TUI 输入：

```text
先用 run_command 执行 printf HOOK-BLOCK-7c2a。若该操作被拒绝，请解释拒绝原因，然后不要尝试其他命令。
```

通过条件：

- 不出现权限确认，也不真正执行该命令；fixture 中没有新文件或命令副作用。
- TUI 的工具结果为失败，并包含 Hook 拒绝语义及 `HOOK-BLOCKED-REASON-7c2a`。
- Agent 可读取拒绝原因并给出说明，而不是因一次工具失败直接崩溃。

为避免把工具前 Hook 与权限系统混淆，规则刻意拦截无害的 `printf`；不要在人工场景中测试 `rm -rf` 或其他破坏性命令。

## 场景 D：Slash Command 与生命周期通知

在 TUI 依次输入：

```text
/status
/help
不调用工具，只回答：SLASH-HOOK-FOLLOW-UP-55df。
```

通过条件：

- `/status` 和 `/help` 仍是本地即时反馈，不启动 Agent Loop。
- 两个 Slash Command 都不会报错；后续普通模型请求可接收 `command_execute` Hook 通知。
- 日志中可观察到 `command_execute` 和规则标识 `command-notification`，但不记录完整用户输入或模型正文。

## 场景 E：条件、变量和日志脱敏

临时把项目级格式化规则的 `command` 改为：

```yaml
command: 'printf "event=$EVENT tool=$TOOL_NAME path=$FILE_PATH arg=$TOOL_ARGS.path"'
```

再次请求写入任意 `.go` 文件。通过条件：命令能运行且变量被替换；未提供的 `$TOOL_ARGS.missing` 应替换为空字符串。恢复 `gofmt` 命令后继续其他场景。

再在 prompt、HTTP body 和 HTTP header 中分别加入唯一敏感标记，例如 `HOOK-SECRET-8ad1`，触发一次 Hook 后执行：

```sh
rg -n 'HOOK-SECRET-8ad1' "$fixture_root/logs" || true
```

通过条件：命令没有匹配输出。日志允许包含事件、规则标识、动作类型、状态、耗时、退出码或 HTTP 状态码，但不得包含上述敏感标记。

## 结果记录模板

| 场景 | 输入或配置 | 可见证据 | 文件/日志证据 | 结果 |
|---|---|---|---|---|
| A 配置与 once |  | 首次通知、重启后重置、非法配置诊断 | 配置路径与规则标识 | 通过/失败 |
| B 格式化与异步失败 |  | 工具调用、任务正常完成 | `gofmt -d` 为空、脱敏失败日志 | 通过/失败 |
| C 工具前拦截 |  | 结构化失败结果、模型说明 | 无命令副作用 | 通过/失败 |
| D Slash Command |  | 本地即时反馈、后续模型请求 | command 事件安全日志 | 通过/失败 |
| E 变量与脱敏 |  | 变量替换后动作完成 | 敏感标记未出现在日志 | 通过/失败 |

## 清理

退出 MewCode 后，只删除本方案确认创建的临时目录和二进制：

```sh
rm -rf "$fixture_root" "$test_home"
rm -f /private/tmp/mewcode-ch12
```

执行删除前先检查变量：

```sh
printf 'fixture_root=%s\ntest_home=%s\n' "$fixture_root" "$test_home"
```

若变量为空、不是 `/private/tmp/mewcode-ch12-*` 或 `/private/tmp/mewcode-ch12-home-*`，停止清理并手动确认路径。

