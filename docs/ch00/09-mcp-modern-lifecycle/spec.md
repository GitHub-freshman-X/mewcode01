# MCP Modern Lifecycle Compatibility Spec

## 背景

当前 MewCode 的 MCP Client 固定使用旧版握手生命周期：连接后执行 `initialize`、`notifications/initialized` 与 `tools/list`。对于 HTTP Server，服务端可返回 `Mcp-Session-Id`，客户端在后续请求中携带该标识，并在关闭时通过 `DELETE` 结束协议会话。

MCP `2026-07-28` 引入无状态生命周期：客户端通过 `server/discover` 探测 Server，每个请求独立携带必要元数据，不使用 `initialize`、`Mcp-Session-Id` 或协议级会话关闭。第三方 MCP Server 的升级节奏并不一致，MewCode 需要在不破坏既有配置与 Server 的前提下兼容两个生命周期。

## 目标

- 默认优先使用 MCP `2026-07-28` 无状态生命周期。
- 对只支持旧版握手的 MCP Server 自动降级，继续保持可用。
- 不新增用户配置字段；协议协商对用户透明。
- 保持远端工具注册、权限门禁与 Agent Loop 的既有使用体验。

## 功能需求

- **F1 自动生命周期协商**：对每个已配置 MCP Server，先尝试新版 `server/discover`；协商结果分为 `modern` 或 `legacy`，并在本次 MewCode 进程内复用。适用于 `stdio` 与 Streamable HTTP。

- **F2 新版成功路径**：当 `server/discover` 确认支持 `2026-07-28` 时，不发送 `initialize` 或 `notifications/initialized`；后续 `tools/list`、`tools/call` 等请求按新版格式携带每请求元数据。HTTP 请求同时携带新版要求的协议与路由 headers。

- **F3 新版工具发现与注册**：新版协商成功后仍执行 `tools/list`，获得工具名称、描述和输入 Schema；仅在发现完整成功后注册为现有 `<server>__<tool>` 工具。工具定义的缓存遵循 Server 返回的 `ttlMs` 与 `cacheScope`，且私有缓存不得跨授权上下文复用。

- **F4 旧版自动降级**：出现以下情形时，关闭新版探测路径并在同一 Server 上执行现有旧版流程：
  1. Server 明确声明不支持 `2026-07-28`；
  2. `server/discover` 不受支持；
  3. `server/discover` 在限定时间内无响应。

  降级后执行 `initialize`、`notifications/initialized`、`tools/list`，并保持现有 session 行为。

- **F5 不降级的失败分类**：新版探测或调用中的网络、TLS、认证/授权、限流、服务端 `5xx` 及取消等错误，不得触发旧版降级；应保留可诊断的失败分类，且不泄露 token、Authorization、请求正文或环境变量值。

- **F6 生命周期隔离**：新版路径不创建、保存或销毁 `Mcp-Session-Id`，也不发送协议级 session `DELETE`；旧版路径完全保留现有 session header 和关闭逻辑。新版已协商成功后，后续业务调用失败不重新回退旧版。

- **F7 现有工具体验兼容**：无论协商到哪个版本，已发现工具继续通过当前 Registry、权限门禁、Agent Loop 与 Provider 工具定义路径使用；未配置 MCP 或单个 Server 失败时，内置工具和其他 Server 不受影响。

- **F8 兼容边界**：本次只实现新版生命周期、发现、工具列表和工具调用所需的协议变化；不新增旧式 HTTP+SSE transport，也不在本次实现 MRTR、Tasks、订阅、Resources、Prompts、Sampling 或 Roots。

## 非功能需求

- **N1 兼容性**：现有只支持旧握手协议的 MCP Server 必须继续可用；不改动用户现有 `mcp_servers` 配置，也不改变 `stdio` 与 `http` 的配置格式。

- **N2 协商成本**：每个 Server 在一次 MewCode 进程生命周期内最多完成一次版本协商；新版探测成功或降级成功后，不在每次工具调用前重新探测。

- **N3 超时边界**：新版探测使用独立、有限的超时，避免老 Server 忽略 `server/discover` 时无限阻塞启动；超时后只允许一次旧版降级尝试。

- **N4 并发正确性**：同一 Server 的并发请求仍须按 JSON-RPC ID 正确关联；新版与旧版的状态不得相互污染；关闭过程中不得遗留等待请求、HTTP 流或 stdio 子进程。

- **N5 安全性**：协议探测、降级、请求与响应的日志只能记录 Server 名、阶段、协议代际、状态、耗时和安全计数；不得记录 URL 查询敏感值、Authorization、环境变量值、请求/响应正文或工具参数。

- **N6 可观测性**：每个 Server 都能在诊断或日志中明确看到协商结果：`modern`、`legacy_fallback` 或失败阶段；降级原因至少区分“不支持”和“探测超时”。

- **N7 可测试性**：不依赖真实外网服务；受控 stdio/HTTP Server 覆盖新版成功、显式不支持、超时降级、不应降级的认证/网络类失败、旧版 session 保留、以及新版不携带 session header 等场景。

- **N8 确定性**：给定相同配置和受控 Server 行为，协商代际、工具注册集合、请求序列和错误分类一致。

## 不做的事

- 不新增 `protocol_version`、`force_modern`、`force_legacy` 等用户配置字段。
- 不移除或改变现有旧版 MCP 生命周期；它作为自动降级路径继续保留。
- 不支持已弃用的独立 HTTP+SSE 旧传输（SSE endpoint 发现与旧 POST endpoint 模式）。
- 不实现新版 MRTR 的用户输入往返、Tasks、`subscriptions/listen`、Resources、Prompts、Sampling、Roots 或授权协议改造。
- 不实现跨进程、跨启动的协议协商或工具目录缓存。
- 不在新版协商成功后，因普通工具调用失败而切换回旧版。
- 不自动重试可能有副作用的 `tools/call`。
- 不改变远端工具默认经过权限确认的安全策略。
- 不增加 MCP Server 配置界面、手动重连按钮、后台健康检查或自动重连机制。
- 不要求或修改用户已有 MCP Server；兼容逻辑完全由 Client 侧承担。

## 验收标准

- **AC1 新版协商成功**：受控 `stdio` 与 HTTP Server 对 `server/discover` 返回 `2026-07-28` 支持信息后，Client 不发送 `initialize` 或 `notifications/initialized`，仍成功执行 `tools/list` 并注册工具。

- **AC2 新版每请求元数据**：新版 HTTP 的 `tools/list` 与 `tools/call` 请求包含正确的协议版本、方法和工具路由 headers，以及每请求 `_meta` 协议信息；请求不含 `Mcp-Session-Id`，关闭时不发 session `DELETE`。

- **AC3 新版工具调用**：新版发现的远端工具经 Agent 调用后，受控 Server 收到正确的 `tools/call` 参数，结果按现有结构回灌 Agent Loop。

- **AC4 显式不支持时降级**：Server 对新版发现明确表示不支持后，Client 恰好执行一次旧版初始化、初始化完成通知与工具发现；旧版工具能被注册和调用。

- **AC5 探测超时时降级**：Server 忽略 `server/discover` 直到探测超时后，Client 发起一次旧版握手；不会再次进行新版探测，也不会无限等待。

- **AC6 不降级错误**：新版探测遇到认证失败、网络/TLS 失败、限流或 `5xx` 时，不发送 `initialize`；该 Server 不注册工具，其他 Server 与内置工具仍继续可用。

- **AC7 旧版 HTTP session 保留**：自动降级的旧版 HTTP Server 返回 `Mcp-Session-Id` 后，后续 `tools/list` 与 `tools/call` 携带相同 ID；关闭时保留现有 `DELETE` 行为。

- **AC8 协商结果复用**：同一进程内同一 Server 的多次工具调用不重复执行 `server/discover` 或旧版握手；请求使用首次确定的生命周期。

- **AC9 安全诊断**：协商、降级与失败日志可识别 Server、阶段、协商结果/原因，但不包含 Authorization、环境变量展开值、URL 敏感查询、MCP 请求或响应正文。

- **AC10 回归**：现有 `stdio`、旧版 Streamable HTTP、工具注册、权限门禁、Agent Loop 与全量测试继续通过；未配置 MCP 时启动和内置工具行为不变。

- **AC11 文档**：本独立章节包含 Spec、Plan、Tasks、Checklist 和用户可读的使用/兼容性说明；根目录 README 的 MCP 支持说明与实际行为一致。
