# 原生与本地 Tool Search 的 MCP 工具加载 Checklist

## 策略与请求布局

- [ ] `disabled` 发出与改动前等价的完整 MCP 工具定义（验证：Provider 请求体测试）。
- [ ] 官方且受支持的 Anthropic/OpenAI 组合发出完整 deferred MCP schema 和各自官方 Tool Search（验证：Provider 请求体测试）。
- [ ] 非官方、未知或不支持模型的 `auto`/`enabled` 请求不含任何 MCP schema，仅含内置工具、`tool_search` 与 `mcp_call`（验证：Provider 请求体测试）。
- [ ] Local 与 Full 请求均不包含任何原生 Provider 延迟加载字段（验证：Provider 请求体测试）。

## 本地搜索与分发

- [ ] `tool_search.tool_name` 为目录中的完整唯一名时返回该 MCP 工具完整 schema（验证：LocalCatalog 单元测试）。
- [ ] 关键词、自然语言、带 `select:` 前缀与不存在名称产生可理解错误且不执行 MCP 调用（验证：LocalCatalog 与 Agent/Scheduler 测试）。
- [ ] 搜索结果作为普通工具结果进入下一次模型请求，且该请求的工具列表不因结果改变（验证：两轮 Agent Loop 测试）。
- [ ] `mcp_call` 只接受已注册 MCP 工具及其所属 server，拒绝内置工具、伪造名称和 server 不匹配（验证：LocalCatalog 与 Scheduler 测试）。
- [ ] 有效 `mcp_call` 经现有权限门禁、输入校验与本地 MCP Client 成功调用（验证：受控 MCP Agent Loop 测试）。
- [ ] 权限拒绝与输入校验失败均不触发 MCP RPC（验证：Scheduler 测试）。

## 原生回归与安全

- [ ] Anthropic 服务端搜索历史仍可原样回传，且不触发本地执行（验证：现有 Anthropic 流与 Agent 测试）。
- [ ] OpenAI namespace 的稳定分组、最大十个成员和最终本地调用映射保持通过（验证：现有 OpenAI 测试）。
- [ ] 日志不包含 schema 正文、搜索 query、工具参数、工具结果正文、密钥或请求头（验证：受控日志测试或人工审阅）。

## 文档、构建与端到端

- [ ] `.mewcode/config.example.yaml`、README、Spec、Plan、Tasks 和人工方案的模式语义一致（验证：文档审阅）。
- [ ] 本地路径端到端：模型从稳定名称目录选择名称 → 调用 `tool_search` 精确加载 schema → 下一轮调用 `mcp_call` → 本地 fixture 收到目标 MCP 调用并返回固定标记（验证：受控 Agent Loop 测试；可选真实兼容端点人工测试）。
- [ ] 原生路径端到端仍可经 Provider Tool Search 调用本地 MCP fixture（验证：现有受控测试和可用的真实 Provider 测试）。
- [ ] `go test ./...`、`go build ./cmd/mewcode`、`git diff --check` 通过（验证：命令输出）。

## 实际执行记录

> 实现完成后回填每项的执行日期、命令和结果；失败项须说明原因和下一步。

## 实际执行记录（2026-09-07）

- [x] Provider 策略、Anthropic/OpenAI 请求布局与 Agent 回归：`go test ./internal/provider ./internal/provider/anthropic ./internal/provider/openai ./internal/toolsearch ./internal/agent -count=1` 通过。
- [x] 全量回归、构建与格式检查：`go test ./... && go build ./cmd/mewcode && git diff --check` 通过。
- [x] `mcp_call` 兼容唯一名与 MCP 原始名：`go test ./internal/toolsearch ./internal/agent -count=1` 通过；`TestCatalogResolveAcceptsLocalAndRemoteNames` 覆盖两种名称和错误 server 拒绝。
- [x] 名称目录精确加载：`go test ./internal/toolsearch ./internal/agent -count=1` 通过；`TestCatalogLoadRequiresExactDirectoryName` 覆盖完整名称成功及远端名、关键词、`select:` 前缀拒绝。
- [ ] 真实兼容网关人工场景尚未执行；自动化验证覆盖本地目录、请求布局与 Agent 分发链路。
