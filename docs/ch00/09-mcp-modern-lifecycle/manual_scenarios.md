# MCP 新版生命周期兼容人工测试方案

## 目标与范围

本方案在隔离 fixture、隔离用户配置目录、真实 Provider 与本地受控 MCP HTTP Server 下，验证新版 `server/discover` 优先、旧版自动降级、探测超时降级、认证失败不降级、工具调用和关闭语义。

本方案不替代自动化测试。并发、JSON-RPC 乱序响应、所有错误组合、stdio 子进程竞争、缓存 TTL 边界及敏感信息脱敏以 `internal/mcp` 自动化测试为决定性证据。模型未调用指定工具时，只记录“模型未调用目标 MCP 工具”，不得在真实项目补测。

## 安全约束

- 仅使用本方案创建且以 `/private/tmp/mewcode-ch00-mcp-manual.` 开头的目录；不得使用真实用户配置、项目或生产 MCP Server。
- 使用专用、限额的真实 Provider 凭据；不得记录 API key、Authorization 值、完整提示、回复、会话或日志正文。
- 受控 Server 只绑定 `127.0.0.1`。仅允许本次远端工具权限确认；拒绝网络访问、包安装、命令执行及 fixture 外写入。
- 不测试独立 HTTP+SSE transport、MRTR、Tasks、订阅、Resources、Prompts、Sampling 或 Roots。

## 准备 fixture 与受控 Server

在仓库根目录执行：

```sh
project_root=$(git rev-parse --show-toplevel)
fixture_root=$(mktemp -d /private/tmp/mewcode-ch00-mcp-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-ch00-mcp-home.XXXXXX)
binary=/private/tmp/mewcode-ch00-mcp
server_log="$fixture_root/mcp-server.log"
server_pid_file="$fixture_root/mcp-server.pid"
mkdir -p "$fixture_root/.mewcode"
printf 'MCP-MANUAL-FIXTURE-7d3a\n' > "$fixture_root/fixture.txt"
```

在 `$fixture_root/mcp_server.py` 写入以下最小受控 Server。日志只写入请求方法和 header 是否存在，绝不写 header 值或 body。

```python
import json, sys, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

log_path = sys.argv[1]
def log(path, method, headers, params):
    meta = isinstance(params, dict) and isinstance(params.get("_meta"), dict)
    with open(log_path, "a", encoding="utf-8") as f:
        f.write(f"path={path} method={method} session={'yes' if headers.get('MCP-Session-Id') else 'no'} protocol={'yes' if headers.get('MCP-Protocol-Version') else 'no'} mcp_method={headers.get('Mcp-Method','')} mcp_name={headers.get('Mcp-Name','')} meta={'yes' if meta else 'no'}\n")

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_): pass
    def reply(self, status, body=None, session=None):
        self.send_response(status)
        if session: self.send_header("MCP-Session-Id", session)
        if body is not None:
            raw = json.dumps(body).encode(); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        if body is not None: self.wfile.write(raw)
    def do_DELETE(self):
        log(self.path, "DELETE", self.headers, {}); self.reply(204)
    def do_POST(self):
        req = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))) or b"{}")
        method, params, req_id = req.get("method", ""), req.get("params", {}), req.get("id")
        log(self.path, method, self.headers, params)
        if self.path == "/unauthorized": self.reply(401); return
        if self.path == "/legacy" and method == "server/discover": self.reply(404); return
        if self.path == "/timeout-legacy" and method == "server/discover": time.sleep(3); return
        if method == "server/discover": result = {"supportedVersions":["2026-07-28"],"capabilities":{"tools":{}}}
        elif method == "initialize": result = {"protocolVersion":"2025-06-18"}
        elif method == "notifications/initialized": self.reply(202); return
        elif method == "tools/list": result = {"tools":[{"name":"manual_echo","description":"Return the manual marker.","inputSchema":{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}}],"ttlMs":60000,"cacheScope":"private"}
        elif method == "tools/call": result = {"content":[{"type":"text","text":"MCP-MANUAL-OK-5e62"}]}
        else: self.reply(200, {"jsonrpc":"2.0","id":req_id,"error":{"code":-32601,"message":"method not found"}}); return
        self.reply(200, {"jsonrpc":"2.0","id":req_id,"result":result}, "legacy-session" if method == "initialize" else None)

ThreadingHTTPServer(("127.0.0.1",18887), Handler).serve_forever()
```

启动后确认仅本地端口可访问：

```sh
python3 "$fixture_root/mcp_server.py" "$server_log" &
echo $! > "$server_pid_file"
sleep 1
nc -z 127.0.0.1 18887
```

创建 `$fixture_root/.mewcode/config.yaml`。将 `<real-provider-fields>` 替换为专用真实 Provider 配置，不能复制真实用户的 MCP 配置：

```yaml
<real-provider-fields>
agent:
  max_iterations: 12
  enable_verification_agent: false
permissions:
  mode: default
mcp_servers:
  modern: {type: http, url: http://127.0.0.1:18887/modern}
  legacy: {type: http, url: http://127.0.0.1:18887/legacy}
  timeout_legacy: {type: http, url: http://127.0.0.1:18887/timeout-legacy}
  unauthorized: {type: http, url: http://127.0.0.1:18887/unauthorized}
```

构建并从 fixture 启动。`HOME` 必须隔离：

```sh
go -C "$project_root" build -o "$binary" ./cmd/mewcode
cd "$fixture_root"
HOME="$test_home" "$binary" --config "$fixture_root/.mewcode/config.yaml"
```

执行 `/status`，确认工作目录为 `$fixture_root` 并记录日志目录。`unauthorized` 的失败不应阻止 TUI、内置工具或其他 MCP Server 启动。

## 场景 A：新版发现、注册与调用

在 TUI 输入：

```text
必须调用一次 MCP 工具 modern__manual_echo，参数 message 为 "manual modern"。不要调用其他工具；拿到结果后只报告固定标记。
```

允许本次远端工具调用。通过条件：工具结果包含 `MCP-MANUAL-OK-5e62`。另一终端执行：

```sh
rg 'path=/modern' "$server_log"
```

期望依次有 `server/discover`、`tools/list`、`tools/call`；三项都为 `session=no protocol=yes meta=yes`。`tools/call` 还应有 `mcp_method=tools/call mcp_name=manual_echo`。不得出现 `initialize` 或 `DELETE`。

## 场景 B：旧版自动降级与 session

在 TUI 输入：

```text
必须调用一次 MCP 工具 legacy__manual_echo，参数 message 为 "manual legacy"。不要调用其他工具；拿到结果后只报告固定标记。
```

通过条件：结果包含固定标记。检查：

```sh
rg 'path=/legacy' "$server_log"
```

期望顺序为 `server/discover`、`initialize`、`notifications/initialized`、`tools/list`、`tools/call`；后两个请求必须有 `session=yes`。退出 MewCode 后，日志必须包含恰好一次 `path=/legacy method=DELETE session=yes`。重启后重新探测是预期行为，因为协商结果只缓存于单个进程。

## 场景 C：探测超时降级

启动后三秒内不要请求工具。等待约三秒后输入：

```text
必须调用一次 MCP 工具 timeout_legacy__manual_echo，参数 message 为 "manual timeout fallback"。不要调用其他工具；拿到结果后只报告固定标记。
```

通过条件：工具调用成功，且：

```sh
rg 'path=/timeout-legacy' "$server_log"
```

只出现一次 `server/discover`，随后出现一次旧版握手、`tools/list` 与 `tools/call`；不得无限阻塞或再次探测。若本机时序不稳定，记录实际时间和顺序；一次降级的决定性证据仍是自动化测试。

## 场景 D：认证失败不降级与隔离

只检查安全的日志字段和值：

```sh
rg 'path=/unauthorized' "$server_log"
rg -n '"server":"unauthorized"|"stage":"negotiate"' <status 显示的日志目录>
```

通过条件：`/unauthorized` 只有一次 `server/discover`，绝无 `initialize`、`tools/list`、`tools/call` 或 `DELETE`；场景 A、B、C 中任一已注册工具或内置只读工具仍可用。诊断可识别 Server 和阶段，但不得含 header 值、凭据或 MCP 正文。

## 场景 E：关闭语义隔离

完成 A、C 后退出 MewCode，执行：

```sh
rg 'path=/modern method=DELETE|path=/timeout-legacy method=DELETE' "$server_log" || true
```

通过条件：`/modern` 无 DELETE；`/timeout-legacy` 已降级，应有一次 `session=yes` 的 DELETE。这证明关闭策略取决于协商后的生命周期，而不是单纯取决于 `type: http`。

## 结果记录模板

| 场景 | TUI 证据 | Server/日志证据 | 结果 |
|---|---|---|---|
| A 新版 | `modern__manual_echo` 与固定标记 | discover/list/call；无 session | 通过/失败/模型未调用 |
| B 旧版 | `legacy__manual_echo` 与固定标记 | 404 后握手、session、DELETE | 通过/失败/模型未调用 |
| C 超时 | `timeout_legacy__manual_echo` 与固定标记 | 一次探测、一次回退 | 通过/失败/未完成 |
| D 隔离 | 其余工具仍可用 | 401 后无握手 | 通过/失败 |
| E 关闭 | 正常退出 | modern 无 DELETE，legacy 有 DELETE | 通过/失败 |

## 清理

先退出 MewCode，再核对目标：

```sh
printf 'fixture_root=%s\ntest_home=%s\nbinary=%s\nserver_pid=%s\n' "$fixture_root" "$test_home" "$binary" "$(cat "$server_pid_file")"
```

仅当 `fixture_root`、`test_home` 分别以 `/private/tmp/mewcode-ch00-mcp-manual.`、`/private/tmp/mewcode-ch00-mcp-home.` 开头，`binary` 恰为 `/private/tmp/mewcode-ch00-mcp` 且 PID 文件位于 fixture 内时，才执行：

```sh
kill "$(cat "$server_pid_file")"
rm -rf "$fixture_root" "$test_home"
rm -f "$binary"
```

变量为空、路径不匹配或 PID 无法确认时停止清理并手工处理；不得删除真实用户目录、项目目录、真实 MCP Server 或未确认的数据。
