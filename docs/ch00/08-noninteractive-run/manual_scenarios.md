# MewCode 非交互式单任务执行人工测试方案

## 目标与范围

本方案在独立 fixture、隔离用户配置目录和真实 Provider 下，验证 `mewcode run` 的用户可见行为：任务输入、流式文本、JSON 结果、非交互权限拒绝、超时和临时状态。

本方案不替代自动化测试。事件竞态、退出码的全部分支、信号竞争、MCP 生命周期、后台子 Agent 的过滤和日志字段脱敏仍以自动化测试为决定性证据。真实模型没有按要求调用工具时，记录“模型未调用目标工具”，不要通过手工修改 fixture 代替 Agent 操作。

## 安全约束

- 只在本方案创建且以 `/private/tmp/mewcode-ch00-run.` 开头的 fixture 中执行；不得在本项目仓库、真实项目或真实用户配置目录中运行写入场景。
- 使用专用、限额的 API 凭据。不得记录 API Key、完整任务文本、模型回复、JSON 完整内容、日志正文或请求头。
- 除场景 C 的 fixture 内单文件写入外，拒绝 Agent 的任何写入、网络、包安装、越界路径或破坏性命令。
- 非交互入口不会出现确认界面；确认规则未命中时应由 Agent 收到拒绝结果。不要为了让场景通过而放宽权限。

## 准备 fixture 与测试配置

以下命令以 macOS 的 POSIX shell 为例。在项目根目录执行：

```sh
project_root=$(git rev-parse --show-toplevel)
fixture_root=$(mktemp -d /private/tmp/mewcode-ch00-run.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch00-run-home.XXXXXX)
binary=/private/tmp/mewcode-ch00-run

mkdir -p "$fixture_root/.mewcode" "$test_home"
printf 'READ-MARKER-4f21\n' > "$fixture_root/readonly.txt"
printf 'INITIAL-WRITE-MARKER-7c9a\n' > "$fixture_root/write-target.txt"
printf 'fixture_root=%s\ntest_home=%s\nbinary=%s\n' \
  "$fixture_root" "$test_home" "$binary"
```

复制一份已验证可用的真实 Provider 配置为 `<real-config.yaml>`。为验证权限拒绝，配置必须使用默认权限模式；不要使用 `relaxed`：

```yaml
max_tokens: 2048
thinking:
  enabled: false
agent:
  max_iterations: 10
permissions:
  mode: default
```

构建二进制。后续所有命令均应从 `$fixture_root` 执行，以确保它是 Agent 工作区：

```sh
go -C "$project_root" build -o "$binary" ./cmd/mewcode
cd "$fixture_root"
```

## 场景 A：输入校验

以下命令只测试参数，均不应发起模型请求。每条命令的退出码必须为 `1`，stderr 应包含可定位的参数错误：

```sh
HOME="$test_home" "$binary" run --config <real-config.yaml>
HOME="$test_home" "$binary" run --config <real-config.yaml> --prompt 'x' --prompt-file ./task.md
HOME="$test_home" "$binary" run --config <real-config.yaml> --prompt '   '
HOME="$test_home" "$binary" run --config <real-config.yaml> --prompt 'x' --timeout -1s
```

通过条件：没有创建 `$fixture_root/.mewcode/sessions/`，且每条命令返回 `1`。若最后一条在 shell 中被解析为其他参数错误，也记录实际 stderr，但仍不得启动模型。

## 场景 B：短文本、文件输入与 JSON 输出

先执行默认文本模式。任务明确限定为只读，正常情况下无需确认：

```sh
HOME="$test_home" "$binary" run --config <real-config.yaml> \
  --prompt '只读取 readonly.txt，并在最终答复中仅报告其中的唯一标记。不要调用其他工具。'
```

通过条件：stdout 可实时出现文本，进程退出码为 `0`，最终输出含 `READ-MARKER-4f21`；stderr 不出现正常 Agent 文本。

再准备任务文件并验证 JSON：

```sh
printf '%s\n' '只读取 readonly.txt，并在最终答复中仅报告其中的唯一标记。不要调用其他工具。' > task.md
HOME="$test_home" "$binary" run --config <real-config.yaml> --prompt-file ./task.md --json > result.json
status=$?
python3 - <<'PY'
import json
result = json.load(open("result.json"))
required = {"status", "stop_reason", "error", "final_text", "elapsed_ms", "iterations", "usage"}
assert required <= result.keys(), result
assert result["status"] == "completed", result
assert "READ-MARKER-4f21" in result["final_text"], result
print({key: result[key] for key in ("status", "stop_reason", "elapsed_ms", "iterations")})
PY
test "$status" -eq 0
```

通过条件：`result.json` 是单个可解析 JSON 文档，状态为 `completed`，且 shell 退出码为 `0`。记录状态和计数，不要保存完整 `final_text`。

## 场景 C：需确认的写入自动拒绝

运行以下任务，不要修改权限配置：

```sh
HOME="$test_home" "$binary" run --config <real-config.yaml> --json \
  --prompt '必须尝试只使用 write_file 将 write-target.txt 的内容改为 SHOULD-NOT-BE-WRITTEN-19d2。若工具返回失败，直接简短说明失败；不要改用命令或其他写入方式。' \
  > permission-result.json
```

通过条件：命令不会等待输入或出现 TUI；`write-target.txt` 仍为 `INITIAL-WRITE-MARKER-7c9a`。`permission-result.json` 的最终状态可以是 `completed` 或 `failed`，但若模型调用了工具，其结果必须体现拒绝而非成功写入。若模型未调用 `write_file`，记录“模型未调用目标工具”，本轮不作为权限结论。

```sh
cat write-target.txt
```

## 场景 D：总超时

设置极短总超时，使任务在模型请求阶段被取消：

```sh
HOME="$test_home" "$binary" run --config <real-config.yaml> --json --timeout 1ms \
  --prompt '只回答 OK。' > timeout-result.json
status=$?
python3 - <<'PY'
import json
result = json.load(open("timeout-result.json"))
assert result["status"] == "timed_out", result
print(result["status"], result["elapsed_ms"])
PY
test "$status" -eq 3
```

通过条件：JSON 状态为 `timed_out`，退出码为 `3`，且 stdout 中没有 JSON 之外的文本。网络或 Provider 在请求前失败时，记录实际错误；该情况不能证明超时行为。

## 场景 E：临时会话与日志

完成场景 B 至 D 后，检查运行产物：

```sh
find "$fixture_root/.mewcode/sessions" -maxdepth 1 -type f -print 2>/dev/null
find "$fixture_root/logs" -type f -name '*.jsonl' -print
rg -n 'READ-MARKER-4f21|INITIAL-WRITE-MARKER-7c9a|SHOULD-NOT-BE-WRITTEN-19d2' \
  "$fixture_root/logs" || true
```

通过条件：sessions 查询没有输出，日志文件存在，最后一条搜索没有匹配。日志可以包含阶段、状态、计数、耗时和 Token 等安全元数据，但不得包含任务正文、文件内容或密钥。

## 结果记录模板

| 场景 | 命令/配置 | stdout、stderr 或文件证据 | 结果 |
|---|---|---|---|
| A 输入校验 | 四种非法输入 | 退出码 1、未创建会话 | 通过/失败 |
| B 文本与 JSON | `--prompt`、`--prompt-file --json` | 标记、JSON 字段、退出码 0 | 通过/失败/模型未完成 |
| C 权限拒绝 | 默认权限写入请求 | 文件内容不变、无阻塞 | 通过/失败/模型未调用 |
| D 超时 | `--timeout 1ms --json` | `timed_out`、退出码 3 | 通过/失败/Provider 失败 |
| E 临时状态 | sessions、logs 搜索 | 无会话文件、日志脱敏 | 通过/失败 |

## 清理

先确认目标，避免删除真实目录：

```sh
printf 'fixture_root=%s\ntest_home=%s\nbinary=%s\n' "$fixture_root" "$test_home" "$binary"
```

仅当 `fixture_root`、`test_home` 分别以 `/private/tmp/mewcode-ch00-run.`、`/private/tmp/mewcode-ch00-run-home.` 开头，且 `binary` 恰为 `/private/tmp/mewcode-ch00-run` 时，才执行：

```sh
rm -rf "$fixture_root" "$test_home"
rm -f "$binary"
```

变量为空或路径不符合临时模式时停止清理并手工确认；不得删除真实用户目录、项目目录或其他 fixture。
