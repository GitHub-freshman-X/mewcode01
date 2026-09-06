# 第零章原生 Tool Search 与 MCP 工具延迟加载人工测试方案

## 目标与范围

本方案使用隔离 fixture、本地 stdio MCP Server 和真实 Anthropic / OpenAI Provider，验证四类用户可见行为：

| 场景 | Provider | 模型能力 | 预期 |
|---|---|---|---|
| A | Anthropic | 支持 Tool Search | MCP 工具延迟发现后仍能由本地 Client 调用。 |
| B | Anthropic | 不支持 Tool Search | 自动回退平铺完整工具，仍能调用 MCP 工具。 |
| C | OpenAI Responses | 支持 Tool Search | MCP namespace 经服务端搜索后，仍能由本地 Client 调用。 |
| D | OpenAI Responses | 不支持 Tool Search | 自动回退平铺完整工具，仍能调用 MCP 工具。 |

本方案不替代自动化测试。请求 JSON 是否精确包含 Anthropic `defer_loading`、OpenAI namespace、分块上限和无未知字段的回退，由本章 Provider 单元测试作为决定性证据；真实 Provider 测试重点是“模型能否发现能力、最终调用是否回到本地 MCP Client、回退是否不报错”。

## 安全约束

- 仅在本方案创建的 `/private/tmp/mewcode-ch00-tool-search-manual.*` fixture 中运行；不得使用项目仓库或真实项目目录。
- 使用专用、限额的真实 API key。不得将 key、完整请求、完整模型输出、会话 JSONL 或日志正文复制到测试记录。
- 测试 MCP Server 只返回固定标记，不读写文件、不联网、不执行 shell 命令；拒绝模型提出的其他写入、命令或网络工具调用。
- 每轮仅允许 `demo__lookup_fixture` 这个 MCP 工具调用。若模型请求其他工具，拒绝并记录实际工具名。
- 真实模型未按提示调用 MCP 工具时，记录“模型未调用目标工具”，不要通过手工伪造工具结果替代。

## 准备 fixture 和本地 MCP Server

在项目根目录执行。以下 fixture 通过故意不支持 `server/discover` 走项目既有 MCP 兼容路径；这不影响本章的 Provider Tool Search 验收。

```sh
project_root=$(git rev-parse --show-toplevel)
fixture_root=$(mktemp -d /private/tmp/mewcode-ch00-tool-search-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch00-tool-search-home.XXXXXX)
binary=/private/tmp/mewcode-ch00-tool-search

mkdir -p "$fixture_root"
printf 'fixture_root=%s\ntest_home=%s\nbinary=%s\n' "$fixture_root" "$test_home" "$binary"
```

在 `$fixture_root/mcp_demo.py` 写入以下最小 stdio MCP Server。它的唯一工具名为 `lookup_fixture`，MewCode 注册后将其公开为 `demo__lookup_fixture`；成功调用只返回 `MCP-TOOL-SEARCH-FIXTURE-6f42`。

```python
import json
import sys

def send(identifier, result=None, error=None):
    payload = {"jsonrpc": "2.0", "id": identifier}
    if error is not None:
        payload["error"] = error
    else:
        payload["result"] = result
    print(json.dumps(payload), flush=True)

for line in sys.stdin:
    request = json.loads(line)
    method = request.get("method")
    identifier = request.get("id")
    if identifier is None:
        continue
    if method == "server/discover":
        send(identifier, error={"code": -32601, "message": "method not found"})
    elif method == "initialize":
        send(identifier, {"protocolVersion": "2025-11-25", "capabilities": {}, "serverInfo": {"name": "demo", "version": "1"}})
    elif method == "tools/list":
        send(identifier, {"tools": [{"name": "lookup_fixture", "description": "Return the fixed manual Tool Search verification token.", "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False}}]})
    elif method == "tools/call":
        send(identifier, {"content": [{"type": "text", "text": "MCP-TOOL-SEARCH-FIXTURE-6f42"}], "isError": False})
    else:
        send(identifier, error={"code": -32601, "message": "method not found"})
```

构建二进制：

```sh
go -C "$project_root" build -o "$binary" ./cmd/mewcode
```

为四个场景分别准备配置。把下列 `<API_KEY>` 替换为各自的专用测试 key，`args` 中的路径必须替换为当前 `$fixture_root/mcp_demo.py` 的绝对路径。所有配置必须保留 `tool_search: auto`，才能验证自动启用和自动回退。

```yaml
# anthropic-supported.yaml
protocol: anthropic
model: claude-sonnet-4-6
base_url: https://api.anthropic.com
api_key: <ANTHROPIC_API_KEY>
max_tokens: 1024
tool_search: auto
thinking: { enabled: false }
permissions: { mode: default }
mcp_servers:
  demo:
    type: stdio
    command: python3
    args: ["/absolute/path/to/mcp_demo.py"]
```

```yaml
# anthropic-unsupported.yaml
# 若该旧模型已不可用，换成账户可用、支持 function tool、但不在 Tool Search 支持表中的 Claude 模型。
protocol: anthropic
model: claude-3-5-haiku-latest
base_url: https://api.anthropic.com
api_key: <ANTHROPIC_API_KEY>
max_tokens: 1024
tool_search: auto
thinking: { enabled: false }
permissions: { mode: default }
mcp_servers:
  demo:
    type: stdio
    command: python3
    args: ["/absolute/path/to/mcp_demo.py"]
```

```yaml
# openai-supported.yaml
protocol: openai
model: gpt-5.4-mini
base_url: https://api.openai.com
api_key: <OPENAI_API_KEY>
max_tokens: 1024
tool_search: auto
permissions: { mode: default }
mcp_servers:
  demo:
    type: stdio
    command: python3
    args: ["/absolute/path/to/mcp_demo.py"]
```

```yaml
# openai-unsupported.yaml
protocol: openai
model: gpt-4.1
base_url: https://api.openai.com
api_key: <OPENAI_API_KEY>
max_tokens: 1024
tool_search: auto
permissions: { mode: default }
mcp_servers:
  demo:
    type: stdio
    command: python3
    args: ["/absolute/path/to/mcp_demo.py"]
```

启动每一个场景时均从 fixture 目录运行并隔离 `HOME`：

```sh
cd "$fixture_root"
HOME="$test_home" "$binary" --config /absolute/path/to/<scenario>.yaml
```

启动后先执行 `/status`。通过前提：工作目录为 `$fixture_root`，并且 MCP `demo` 已成功连接；如果启动诊断显示 `demo` 未注册，停止该场景并修复 fixture，而不是继续测试 Provider 行为。

## 统一模型输入与证据

每个场景在 TUI 输入相同内容：

```text
必须完成这个 MCP 工具调用：调用 demo__lookup_fixture，参数是空 JSON 对象。不要调用任何其他工具，也不要猜测返回值。收到工具结果后，只输出工具结果中的固定标记，不要添加任何其他文本。
```

权限确认出现时，只允许本次 `demo__lookup_fixture` 调用。通过的共同条件：

- TUI 显示或工具事件记录表明调用的是 `demo__lookup_fixture`；
- 最终回答恰为 `MCP-TOOL-SEARCH-FIXTURE-6f42`；
- 没有文件改动、命令执行或其他 MCP 调用；
- 退出后重启下一个场景，不能在同一进程中切换配置或模型。

模型可能不遵循“必须调用”的指令。这类情况只记录为“模型未调用目标工具”，可在同一场景新开会话重试一次；两次均未调用则标记为真实模型行为失败，不能用手工输入工具结果掩盖。

## 场景 A：Anthropic 支持 deferred loading

使用 `anthropic-supported.yaml` 启动并执行统一模型输入。预期：`claude-sonnet-4-6` 在 `auto` 模式下启用 Anthropic Tool Search；`demo__lookup_fixture` 不是初始可见工具，模型需要搜索并发现它，然后由本地 Client 调用。

额外记录：是否出现与 Tool Search 对应的 Provider 状态提示（若 UI/Provider 返回），但不能将其缺失单独判失败；当前 UI 不以搜索事件作为用户可见功能。最终调用和固定标记是手工验收依据。

## 场景 B：Anthropic 不支持时回退

完全退出场景 A，使用 `anthropic-unsupported.yaml` 重新启动并执行统一模型输入。预期：启动和请求均不因 `defer_loading` 或 Tool Search 类型报错，MCP 工具可直接调用并得到同一固定标记。

若模型 ID 已被平台下线，记录“模型不可用”而非 Tool Search 失败，并改用账户可用的非支持模型重测。不得将 `tool_search` 改为 `disabled`，否则无法验证 `auto` 的能力回退。

## 场景 C：OpenAI 支持 deferred namespace

完全退出场景 B，使用 `openai-supported.yaml` 重新启动并执行统一模型输入。预期：`gpt-5.4-mini` 在 `auto` 模式下启用 OpenAI Responses Tool Search。模型初始只看到 `demo` 对应 namespace 的概述；发现成员后发出名为 `demo__lookup_fixture` 的 function call，仍由本地 MCP Client 返回固定标记。

记录最终工具名。不要因为模型在内部进行 Tool Search 而自行向本地 MCP fixture 增加一个 `tool_search` 工具：OpenAI 的搜索由服务端执行，fixture 只应接收最终 `tools/call`。

## 场景 D：OpenAI 不支持时回退

完全退出场景 C，使用 `openai-unsupported.yaml` 重新启动并执行统一模型输入。预期：`gpt-4.1` 不会收到 OpenAI Tool Search 或 deferred namespace 字段，调用不会因不识别 beta 字段而失败，并能使用平铺的 `demo__lookup_fixture` 工具得到固定标记。

若这里出现 OpenAI 的 schema/未知字段 `4xx`，记录完整错误类别和模型 ID（不要记录请求正文或 key）；这表示回退失败。若最终调用成功但模型未调用目标工具，按统一模型输入的重试规则记录。

## 配置模式补充检查

在任一支持模型配置副本中，把 `tool_search: auto` 改为 `disabled`，重新启动并执行统一输入。预期：工具仍可调用，行为与回退路径一致。恢复 `auto` 后继续。

在 `openai-unsupported.yaml` 副本中改为 `tool_search: enabled` 并启动。预期：程序在发起模型请求前给出本地配置错误，说明该模型不支持 Tool Search；不得向 Provider 发送请求。完成后将配置恢复为 `auto`。

## 结果记录模板

| 场景 | 模型和配置 | `demo` 注册 | 最终工具调用 | 固定标记 | 结果 |
|---|---|---|---|---|---|
| A Anthropic 支持 | `claude-sonnet-4-6` / `auto` | 是/否 | 名称或未调用 | 是/否 | 通过/失败/模型未调用 |
| B Anthropic 回退 | 实际模型 / `auto` | 是/否 | 名称或未调用 | 是/否 | 通过/失败/模型不可用 |
| C OpenAI 支持 | `gpt-5.4-mini` / `auto` | 是/否 | 名称或未调用 | 是/否 | 通过/失败/模型未调用 |
| D OpenAI 回退 | `gpt-4.1` / `auto` | 是/否 | 名称或未调用 | 是/否 | 通过/失败/模型未调用 |
| 模式检查 | `disabled` / `enabled` | 是/否 | 成功/本地拒绝 | 是/否 | 通过/失败 |

## 实际执行记录

| 场景 | 模型和配置 | `demo` 注册 | 最终工具调用 | 固定标记 | 结果 |
|---|---|---|---|---|---|
| C OpenAI 支持 | `gpt-5.4-mini` / 官方端点 / `auto` | 是 | `demo__lookup_fixture` | 是 | 通过。请求工具数为 11：9 个内置工具、1 个 MCP namespace、1 个服务端 Tool Search。 |
| D OpenAI 回退 | `gpt-4.1` / 官方端点 / `auto` | 是 | `demo__lookup_fixture` | 是 | 通过。请求工具数为 10：9 个内置工具和 1 个平铺 MCP 工具；未发送 Tool Search 或 namespace。 |

两次调用均由本地 MCP Client 完成，日志没有记录密钥、请求正文或工具结果正文。Anthropic 场景和 `disabled` / `enabled` 模式检查尚未执行。

## 清理

退出 MewCode 并确认没有正在运行的 MCP 子进程后，先打印目标：

```sh
printf 'fixture_root=%s\ntest_home=%s\nbinary=%s\n' "$fixture_root" "$test_home" "$binary"
```

仅当 `fixture_root` 与 `test_home` 分别以 `/private/tmp/mewcode-ch00-tool-search-manual.`、`/private/tmp/mewcode-ch00-tool-search-home.` 开头，且 `binary` 恰为 `/private/tmp/mewcode-ch00-tool-search` 时，才执行：

```sh
rm -rf "$fixture_root" "$test_home"
rm -f "$binary"
```

变量为空、路径不符合临时模式或仍有进程运行时，停止清理并手工确认；不得删除项目目录、真实用户目录或其他临时目录。
