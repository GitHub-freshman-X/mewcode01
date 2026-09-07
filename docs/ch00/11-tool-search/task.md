# 原生与本地 Tool Search 的 MCP 工具加载 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `internal/provider/{provider,tool_search}.go` | 用策略取代二元 ToolSearch 状态。 |
| 新建 | `internal/toolsearch/catalog.go` | 本地目录、虚拟定义、检索和目标解析。 |
| 修改 | `internal/agent/{runner,scheduler}.go` | 本地目录提示、虚拟调用处理和安全分发。 |
| 修改 | `internal/provider/{anthropic,openai}/request.go` | 仅 Native 策略发送官方字段。 |
| 修改 | `internal/prompt/tools.go` | 避免对隐藏 MCP schema 施加无关工具规则。 |
| 修改 | 对应 `*_test.go` | 单元、请求体与 Agent Loop 覆盖。 |
| 修改 | `.mewcode/config.example.yaml`、`README.md` 与章节文档 | 用户说明、人工方案、验收记录。 |

## T1：策略模型与 Provider 编码

**文件：** `internal/provider/{provider,tool_search}.go`、`internal/provider/tool_search_test.go`、`internal/provider/{anthropic,openai}/{request,openai}_test.go`

**依赖：** 无

**步骤：**

1. 用四值 ToolSearch 策略替换 `Enabled`，并保持 `disabled` 选择 Full。
2. 令官方支持组合选择现有 Native 策略，其余有效组合选择 Local。
3. 限制 Anthropic/OpenAI 原生编码仅在相应 Native 策略出现。
4. 为 Full、Native 与 Local 的请求布局和模型/端点判定编写精确测试。

**验证：** `go test ./internal/provider ./internal/provider/anthropic ./internal/provider/openai -count=1`。

## T2：本地目录与虚拟工具定义

**文件：** `internal/toolsearch/catalog.go`、`internal/toolsearch/catalog_test.go`

**依赖：** T1

**步骤：**

1. 从已排序的 Registry definitions 提取 MCP 工具，并定义 `tool_search` 与 `mcp_call` schema。
2. 构建稳定名称目录和 Local 路径的可见 definitions。
3. 实现完整唯一名的精确 schema 加载，拒绝关键词、自然语言与带前缀的名称。
4. 校验 `mcp_call` 的 server、工具名与 arguments，并拒绝内置工具和不一致的 server/tool 组合。

**验证：** `go test ./internal/toolsearch -count=1`。

## T3：Agent Loop 与安全分发

**文件：** `internal/agent/{runner,scheduler}.go`、`internal/agent/{runner,scheduler}_test.go`

**依赖：** T1、T2

**步骤：**

1. 在会话初始化时创建稳定 LocalCatalog，并只在 Local 策略生成名称目录提示和虚拟 tools。
2. 让 `tool_search` 的结果由现有工具结果历史写回下一轮请求。
3. 在 Scheduler 中解析 `mcp_call`，再针对真实 MCP 工具执行已有 Hook、权限、schema 校验和 Executor。
4. 保持原调用 ID 的结果关联，覆盖无效调用、权限拒绝和成功 RPC。

**验证：** `go test ./internal/agent ./internal/mcp -count=1`。

## T4：提示词、用户文档与人工方案

**文件：** `internal/prompt/tools.go`、`.mewcode/config.example.yaml`、`README.md`、`docs/ch00/11-tool-search/{manual_scenarios,checklist}.md`

**依赖：** T3

**步骤：**

1. 将 Local 路径提示限定为“从名称目录选择完整名称，再以该名称调用 tool_search，最后用 mcp_call”。
2. 更新配置说明：`auto` 和 `enabled` 为原生优先、本地兜底；`disabled` 全量加载。
3. 将人工回退场景改为验证本地搜索、schema 工具结果和 mcp_call，而非平铺 MCP 工具。
4. 记录不会在日志中暴露 schema、查询、参数或结果正文的验证方式。

**验证：** 文档与测试命令一致；`git diff --check` 通过。

## T5：全量回归与验收记录

**文件：** 本章 `checklist.md`、必要的 bug 记录

**依赖：** T1–T4

**步骤：**

1. 执行格式化、目标测试、全量测试、构建和 diff 检查。
2. 根据实际输出回填 checklist；若发现缺陷，按 `bugs/README.md` 记录状态、证据和修复。
3. 清理本轮验证生成且与交付无关的临时产物。

**验证：** `go test ./...`、`go build ./cmd/mewcode`、`git diff --check`。

## 执行顺序

```text
T1 → T2 → T3 → T4 → T5
```
