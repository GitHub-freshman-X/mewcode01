# 第八章真实 API 人工测试方案

## 目标与范围

本方案在真正启动 `mewcode`、连接真实 Provider 后，验证第八章的主流程：第一层工具结果持久化、`/compact` 手动摘要、自动摘要、`/plan` 临时历史隔离和 `/do` 的共享历史更新。

为控制成本和保证可重复性，测试使用独立 fixture 工作区与低阈值配置。三次摘要失败熔断、强制压缩、紧急重试、畸形摘要和网络错误属于错误注入场景，继续由自动化测试验证，不以真实 API 的偶发行为作为人工验收依据。

## 安全约束

- 从独立 fixture 工作区启动程序；Agent 只读测试文件，不修改项目工作区。
- 从已有可用配置复制测试配置，只改 `agent.context`；不得提交或记录 `api_key`。
- 仅记录压缩事件、文件计数、唯一标记和最终结论，不保存完整工具结果或摘要正文。
- 出现权限确认时选择单次允许；测试结束后可删除整个 fixture 目录。

## 准备 fixture

```sh
fixture_root=$(mktemp -d /private/tmp/mewcode-ch08-manual.XXXXXX)
mkdir -p "$fixture_root/fixtures"

FIXTURE_ROOT="$fixture_root" python3 - <<'PY'
from pathlib import Path
import os

root = Path(os.environ["FIXTURE_ROOT"]) / "fixtures"

def write(name, size, begin, end):
    body = (f"{name} test payload. " * ((size // (len(name) + 15)) + 20))[:size]
    (root / name).write_text(f"{begin}\n{body}\n{end}\n", encoding="utf-8")

write("large.txt", 3200, "BEGIN-LARGE", "END-LARGE")
write("medium-a.txt", 1800, "BEGIN-MEDIUM-A", "END-MEDIUM-A")
write("medium-b.txt", 1400, "BEGIN-MEDIUM-B", "END-MEDIUM-B")
write("medium-c.txt", 1200, "BEGIN-MEDIUM-C", "END-MEDIUM-C")
for index in range(1, 5):
    write(f"context-{index:02d}.txt", 3600, f"BEGIN-CONTEXT-{index:02d}", f"END-CONTEXT-{index:02d}")
print(root.parent)
PY
```

记下脚本输出的 `<fixture_root>`。运行前基线检查：

```sh
find <fixture_root> -maxdepth 4 -type f | sort
```

此时应只有 `fixtures/` 的文件，尚不存在 `.mew/context/.../tool-results`。

如未构建二进制，在项目根目录执行：

```sh
go build -o /Users/xuchangan/Project/mine/mew-agent/mew01/mewcode /Users/xuchangan/Project/mine/mew-agent/mew01/cmd/mewcode
```

每次测试从 fixture 工作区启动：

```sh
cd <fixture_root>
/Users/xuchangan/Project/mine/mew-agent/mew01/mewcode --config <测试配置绝对路径>
```

## 测试配置

从已验证可用的真实 API 配置复制两份。保留 `protocol`、`model`、`base_url`、`api_key`，并关闭 thinking。以下是应替换的部分。

### `ch08-layer1.yaml`

```yaml
max_tokens: 2048
thinking:
  enabled: false
agent:
  max_iterations: 12
  context:
    window_tokens: 20000
    summary_output_tokens: 2000
    auto_safety_tokens: 3000
    manual_safety_tokens: 1000
    single_result_chars: 2400
    message_result_chars: 3500
    preview_chars: 300
    recent_tokens: 2000
    recent_message_minimum: 5
permissions:
  mode: default
```

### `ch08-auto.yaml`

```yaml
max_tokens: 2048
thinking:
  enabled: false
agent:
  max_iterations: 12
  context:
    window_tokens: 14000
    summary_output_tokens: 2000
    auto_safety_tokens: 1500
    manual_safety_tokens: 500
    single_result_chars: 10000
    message_result_chars: 12000
    preview_chars: 300
    recent_tokens: 2000
    recent_message_minimum: 5
permissions:
  mode: default
```

第二份配置的自动线为 10,500 Token，强制线为 11,500 Token。实际触发轮次会随 Provider 返回的真实 `input_tokens` 而变化。

## 会话 A：第一层和摘要

使用 `ch08-layer1.yaml` 启动。

### A1 单项持久化

输入：

```text
只调用一次 read_file，读取 fixtures/large.txt。读取后用一句话说明首尾标记；不要读取其他文件，不要修改文件。
```

通过：TUI 显示“工具结果已持久化 1 项”；执行下列命令后恰有一个文件，且其最后一行为 `END-LARGE`。

```sh
find <fixture_root>/.mew/context -path '*/tool-results/*' -type f -print
tail -n 1 <持久化结果文件>
```

### A2 持久化结果回读放行

输入：

```text
使用 read_file 读取上一步工具结果中显示的持久化文件路径。必须通过工具读取，不要凭记忆回答；然后只回答该文件最后一个非空行。
```

通过：回答为 `END-LARGE`；持久化文件总数仍为一，且本轮无新的“工具结果已持久化”事件。

### A3 聚合预算

输入：

```text
在同一个模型回复中、在写任何解释之前，恰好调用三次 read_file，分别读取 fixtures/medium-a.txt、fixtures/medium-b.txt、fixtures/medium-c.txt。三次调用完成后，只报告每个文件的首尾标记；不要修改文件。
```

通过：三个工具调用在同一轮；TUI 显示持久化一项；`tool-results` 总数由一变为二；新增文件含 `BEGIN-MEDIUM-A` 和 `END-MEDIUM-A`。若模型拆为多个 Agent 轮次，本场景重启会话重试，不能判通过。

### A4 手动摘要

空闲时输入：

```text
/compact
```

通过：TUI 显示 `上下文压缩: manual <before> -> <after> tokens`。随后输入：

```text
不调用工具：概括本会话已经验证的两种工具结果压缩行为，以及 large.txt 的末尾标记。
```

回答应包括单项持久化、聚合预算和 `END-LARGE`。

### A5 自动摘要

退出程序，使用 `ch08-auto.yaml` 在同一 fixture 工作区重新启动。依次发送以下四条消息，每条任务结束后再发送下一条：

```text
只调用 read_file 读取 fixtures/context-01.txt。回答必须包含 BEGIN-CONTEXT-01 和 END-CONTEXT-01，不要读其他文件。
```

将文件名和编号依次替换为 `02`、`03`、`04`。通过：第 2–4 个任务之间至少出现一次 `automatic` 事件，且 after 小于 before。若第四轮仍未触发，继续读取一次 `context-01.txt`，直到出现 automatic。

之后输入：

```text
不调用工具：列出本会话先后读取过的 context 文件编号，并说明最后一个文件的首尾标记。
```

回答应包括 `01` 至 `04` 与 `BEGIN-CONTEXT-04`、`END-CONTEXT-04`。

## 会话 B：Plan 隔离和 Do 共享历史

使用 `ch08-auto.yaml` 开启全新会话。先创建带随机值的 plan 专用文件：

```sh
printf 'PLAN-ISOLATION-NONCE-84f3c2a9\n' > <fixture_root>/fixtures/plan-only.txt
```

### B1 Plan 临时历史隔离

输入：

```text
/plan 请先读取 fixtures/plan-only.txt 和 fixtures/context-01.txt 到 fixtures/context-04.txt，理解这些文件后，给出一个只读核对计划。计划中不要泄露 plan-only.txt 的完整内容。
```

若 Plan 内达到阈值，应出现 automatic 事件。计划结束后不执行 `/do`，直接输入：

```text
不调用工具：上一条 /plan 内部读取的 plan-only.txt 中，那个不可猜的完整 nonce 是什么？
```

通过：模型不能给出 `PLAN-ISOLATION-NONCE-84f3c2a9`，而是说明它不掌握该值或要求重新读取文件。TUI 显示规划文本是预期行为，不代表该内容进入共享模型历史。

### B2 Do 共享历史

启动另一个新会话，输入：

```text
/plan 制定一个计划：依次读取 fixtures/context-01.txt 到 fixtures/context-04.txt，核对每个文件的首尾标记，并在执行结束时输出编号与末尾标记对应表。
```

计划完成后输入：

```text
/do
```

通过：`/do` 执行工具并正常结束；期间或紧接其后的普通请求前出现 automatic 事件。随后输入：

```text
不调用工具：输出刚才执行任务读取的四个 context 文件编号，以及 context-04 的末尾标记。
```

回答应包括四个编号和 `END-CONTEXT-04`。

## 记录模板

| 场景 | 配置 | TUI 证据 | 文件证据 | 结果 |
|---|---|---|---|---|
| A1 单项持久化 | layer1 | `persisted 1` | 一个结果文件，末尾标记正确 | 通过/失败 |
| A2 回读放行 | layer1 | 无新增 persisted | 文件数不变 | 通过/失败 |
| A3 聚合预算 | layer1 | `persisted 1` | 新增文件为 medium-a | 通过/失败 |
| A4 手动摘要 | layer1 | `manual before -> after` | 不适用 | 通过/失败 |
| A5 自动摘要 | auto | `automatic before -> after` | 不适用 | 通过/失败 |
| B1 Plan 隔离 | auto | Plan 内事件（如触发） | 不适用 | 通过/失败 |
| B2 Do 共享历史 | auto | Do 内或后续 automatic | 不适用 | 通过/失败 |
