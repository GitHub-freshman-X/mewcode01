# 第七章 MCP 客户端手工测试场景

> 适用版本：第七章实现完成后。执行前请先在独立测试目录运行 MewCode，避免 filesystem Server 读到或写到真实项目内容。

## 通用准备

1. 建立临时工作区并进入：

   ```bash
   WORKDIR="$(mktemp -d)"
   mkdir -p "$WORKDIR/.mewcode" "$WORKDIR/fixtures"
   printf 'hello MCP\n' > "$WORKDIR/fixtures/hello.txt"
   cd "$WORKDIR"
   ```

2. 准备可用的主配置 `~/Library/Application Support/mewcode/config.yaml`，其中包含有效的 Provider 配置。也可以通过 `--config` 指定其他完整主配置文件。以下场景只展示需要新增或替换的 `mcp_servers` 段。
3. 每个场景完成后，退出 MewCode，并删除临时目录：

   ```bash
   rm -rf "$WORKDIR"
   ```

## 场景 1：stdio Server 发现与工具调用

**目的：** 验证 stdio 子进程、初始化、工具发现、工具命名空间和 `tools/call` 完整链路。

**前置条件：** 已安装 Node.js 与 `npx`。可先用官方 MCP Inspector 验证 Server 本身可运行：

```bash
npx -y @modelcontextprotocol/inspector npx @modelcontextprotocol/server-filesystem "$WORKDIR/fixtures"
```

**主配置：**

```yaml
mcp_servers:
  filesystem:
    type: stdio
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "${MCP_TEST_ROOT}"]
```

**操作：**

1. 在启动 MewCode 前设置 `export MCP_TEST_ROOT="$WORKDIR/fixtures"`。
2. 启动 MewCode。
3. 让 Agent 执行：`读取 MCP filesystem server 中 hello.txt 的内容。`
4. 若处于默认权限模式，在确认界面选择“本次允许”。

**预期：**

- 工具列表或 Agent 行为能表明存在 `filesystem__...` 命名空间工具，而不是未加前缀的远端名称。
- Agent 能返回 `hello MCP`，工具结果来自远端 Server。
- Server 的 stderr 日志（若有）不显示为 JSON-RPC 协议错误。
- 退出 MewCode 后不存在遗留的 filesystem Server 子进程。

## 场景 2：单一主配置中的 Server 声明

**目的：** 验证 MCP Server 只从已选定的主配置中读取，不使用项目目录中的 MCP 覆盖文件。

**主配置：**

```yaml
mcp_servers:
  filesystem:
    type: stdio
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "${MCP_TEST_ROOT}"]
```

**操作：**

1. 设置 `export MCP_TEST_ROOT="$WORKDIR/fixtures"` 并在 `$WORKDIR` 启动 MewCode。
2. 让 Agent 读取 `hello.txt`。

**预期：**

- 读取成功，证明已选定主配置中的 Server 声明生效。
- `$WORKDIR/.mewcode/` 中不存在或包含其他 YAML 文件都不改变 MCP Server 集合。
- 只有一个名为 `filesystem` 的连接和一组 `filesystem__...` 工具。

## 场景 3：缺失环境变量只隔离对应 Server

**目的：** 验证 `${VAR}` 缺失时的安全诊断与多 Server 故障隔离。

**配置：**

```yaml
mcp_servers:
  healthy:
    type: stdio
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "${MCP_TEST_ROOT}"]
  broken:
    type: stdio
    command: "${MISSING_MCP_COMMAND}"
```

**操作：**

1. 只设置 `export MCP_TEST_ROOT="$WORKDIR/fixtures"`；确保 `MISSING_MCP_COMMAND` 未设置。
2. 启动 MewCode，记录启动诊断。
3. 让 Agent 通过 `healthy__...` 工具读取 `hello.txt`。

**预期：**

- 启动继续，健康 Server 可用。
- 诊断包含 `broken`、配置/环境变量阶段与缺失变量名；不包含任何 Authorization 值、环境变量值或完整环境。
- `broken__...` 工具不会出现，且 Agent 不会把它误认为可调用。

## 场景 4：Streamable HTTP、session header 与 SSE 响应

**目的：** 验证 HTTP headers、`MCP-Session-Id`、协商协议版本，以及 JSON/SSE 两种 Streamable HTTP response。

**前置条件：** 准备一个仅用于测试的 Streamable HTTP MCP Server，并记录其 endpoint，例如 `http://127.0.0.1:PORT/mcp`。该 Server 必须：初始化响应返回 `MCP-Session-Id`；后续 `tools/list` 与 `tools/call` 记录请求 headers；至少一次以 JSON 响应、一次以 SSE 响应返回 JSON-RPC message。

**配置：**

```yaml
mcp_servers:
  http_test:
    type: http
    url: ${MCP_HTTP_URL}
    headers:
      Authorization: Bearer ${MCP_HTTP_TOKEN}
      X-Test-Run: ch07-manual
```

**操作：**

1. 设置 `MCP_HTTP_URL` 和非生产 token `MCP_HTTP_TOKEN`。
2. 启动 MewCode，调用 `http_test__...` 的一个工具两次；让测试 Server 对其中一次调用返回 SSE。
3. 查看测试 Server 的请求记录，然后退出 MewCode。

**预期：**

- 初始化、发现、两次调用均携带 `Authorization` 和 `X-Test-Run`。
- 初始化后的请求均携带同一个 `MCP-Session-Id` 与 `MCP-Protocol-Version`。
- JSON 与 SSE 调用都得到可读的工具结果。
- 退出时 Server 收到带 session header 的 DELETE。
- MewCode 的日志和 Agent 工具结果绝不回显 token 值。

## 场景 5：HTTP 会话失效不自动恢复

**目的：** 验证本章明确不做自动重连、重新握手或调用重试。

**前置条件：** 使用场景 4 的测试 Server；使它在成功发现后，对首次 `tools/call` 返回 HTTP 404。

**操作：**

1. 启动 MewCode，并让 Agent 调用 `http_test__...` 工具。
2. 观察 Agent 最终回复和测试 Server 请求记录。

**预期：**

- 该工具调用返回结构化失败，说明会话已失效；Agent 可继续给出替代答复。
- 404 后没有新的 `initialize`、`notifications/initialized`、`tools/list` 或第二次相同 `tools/call` 请求。
- 重启 MewCode 后，才会发起一套新的初始化和工具发现流程。

## 场景 6：远端工具默认权限门禁

**目的：** 验证远端 MCP 工具即使看似只读，也默认按有副作用工具处理。

**配置：** 保持 `permissions.mode: default`，并配置任一可发现 MCP Server。

**操作：**

1. 启动 MewCode，让 Agent 调用一个远端 MCP 工具。
2. 在首次确认界面选择“拒绝”；记录工具是否真实执行。
3. 重复调用并选择“本会话允许”；再执行相同调用一次。

**预期：**

- 首次调用出现权限确认；拒绝后 Server 不收到 `tools/call`，Agent 获得权限失败结果。
- 选择本会话允许后当前调用执行成功；第二次相同调用不再询问。
- 内置只读工具和 Plan/Do 的既有行为不因 MCP 配置改变。

## 结果记录模板

每个场景请记录：MewCode commit、操作系统、配置（删去 token）、实际工具名、是否出现确认、Server 请求顺序、退出后的资源清理结果，以及与预期不一致的原始安全日志。
