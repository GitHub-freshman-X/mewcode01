# MewCode 上下文管理 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/context/{config,estimate,results,compact}.go` | 阈值、估算、存盘、摘要和重建 |
| 新建 | `internal/context/context_test.go` | Context 纯逻辑与存储测试 |
| 修改 | `internal/config/{config,validate}.go` | YAML 配置、默认值与校验 |
| 修改 | `internal/config/config_test.go` | 配置测试 |
| 修改 | `internal/conversation/{session,session_test}.go` | 共享历史原子替换 |
| 修改 | `internal/agent/{event,history,runner}.go` | 模式、事件、持久会话 ID、Plan 替换和循环编排 |
| 修改 | `internal/agent/runner_test.go` | 自动/手动/Plan/重试集成测试 |
| 修改 | `cmd/mewcode/main.go` | ContextConfig 装配 |
| 修改 | `internal/tui/{update,tui_test}.go` | `/compact` 路由与展示验证 |
| 修改 | `docs/ch08-context/{spec,plan,task,checklist,manual_scenarios}.md` | 结果目录生命周期与恢复语义 |
| 修改 | `docs/README.md` | 第八章索引 |

## T1: 上下文配置与启动装配

**文件：** `internal/config/config.go`、`internal/config/validate.go`、`internal/config/config_test.go`、`cmd/mewcode/main.go`

**依赖：** 无

**步骤：**
1. 先写配置测试：缺省配置得到窗口 200000、摘要 20000、自动余量 13000、手动余量 3000；覆盖合法覆盖值、负数、窗口不足与零第一层预算。
2. 定义嵌套 `ContextConfig`，在 `applyDefaults` 应用默认值，在 `Validate` 拒绝非法组合。
3. 将配置映射为 `agent.Options.Context`，保持旧配置无新增字段仍能启动。
4. 运行配置与 main 注入测试。

**验证：** `go test ./internal/config ./cmd/mewcode` 通过。

## T2: 实现估算、阈值决策与结果持久化

**文件：** `internal/context/config.go`、`internal/context/estimate.go`、`internal/context/results.go`、`internal/context/context_test.go`

**依赖：** T1

**步骤：**
1. 写失败测试：真实 usage 锚点后仅新增消息参与字符估算；200K 默认值的自动线为 167K、强制线为 177K。
2. 实现 `Config`、`Manager.RecordUsage`、`Estimate`、`Decision` 与四种 Trigger。
3. 写失败测试：创建 `ResultStore` 但未持久化结果时不产生目录；单项超阈值首次写入才创建 `.mewcode/context/<会话 ID>/tool-results`，预览含大小和路径；聚合超限按长度降序替换且输出顺序不变；标记为持久化回读的结果不再次替换。
4. 写失败测试：以同一会话 ID 创建两个结果存储对象时，第二个对象写入不会覆盖第一个对象的结果，旧结果路径仍可读取；没有会话 ID 时不替换结果。
5. 实现 `ResultStore` 和 `PrepareResults`：延迟创建目录、使用 `0700`/`0600` 权限、以排他递增命名防覆盖，并限制预览长度、返回 `Persistence` 元数据。
5. 运行 Context 包测试。

**验证：** `go test ./internal/context` 通过。

## T2a: 将持久会话 ID 接入结果目录定位

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`、`internal/agent/runner_test.go`、`cmd/mewcode/main.go`

**依赖：** T2

**步骤：**
1. 写失败测试：新建会话的 Runner 使用创建返回的会话 ID；恢复会话的 Runner 使用恢复返回的同一 ID。
2. 在 Agent 选项中传递持久会话 ID；有工作区且 ID 有效时创建以 `.mewcode/context` 为根的 ResultStore，否则不创建存储对象。
3. 在启动装配层把 `SessionStore.Create` / `SessionStore.Restore` 的 `SessionMeta.ID` 传入 Runner；不新增恢复命令或 TUI。
4. 验证新建会话、模拟恢复会话和无 ID 降级行为。

**验证：** `go test ./internal/agent ./cmd/mewcode -run 'ResultStore|Session.*ID|Restore'` 通过。

## T3: 实现摘要提示、近期边界和历史重建

**文件：** `internal/context/compact.go`、`internal/context/context_test.go`

**依赖：** T2

**步骤：**
1. 写失败测试：摘要请求 Tools 为空、输出预算为配置值，提示包含禁止工具、analysis 草稿及九段 summary 要求。
2. 实现摘要请求构造和 `<summary>` 提取；空标签、缺标签和取消结果返回错误且不修改调用方历史。
3. 写失败测试：重建历史包含边界消息、summary 和约 10K/至少五条近期消息；工具调用与结果不可被切开；用户原话位于保留区或 summary 要求中。
4. 实现消息组边界、重建和压缩后 Token 估算。
5. 运行 Context 包全量测试。

**验证：** `go test ./internal/context -run 'Summary|Rebuild|Recent|Estimate|Result'` 通过。

## T4: 提供共享与 Plan 临时历史替换

**文件：** `internal/conversation/session.go`、`internal/conversation/session_test.go`、`internal/agent/history.go`

**依赖：** T3

**步骤：**
1. 写测试验证 `Session.ReplaceHistory` 深拷贝且并发快照不会看到半更新。
2. 实现锁内整体替换，保持 display、pendingPlans 与原有 CommitRound 行为不变。
3. 写并实现 `taskHistory.Replace`；验证它只改临时历史，原 Session 快照保持不变。
4. 运行 conversation 和 agent history 相关测试。

**验证：** `go test ./internal/conversation ./internal/agent -run 'Replace|Plan.*Isolation'` 通过。

## T5: 扩展 Agent 事件、模式和摘要执行路径

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T2、T2a、T3、T4

**步骤：**
1. 写失败测试：压缩事件含触发原因及前后 Token；`ModeCompact` 任务无工具调用且活跃任务时被拒绝。
2. 增加 Context Manager 到 Options、`ModeCompact`、压缩事件类型与 event payload。
3. 在 Runner 每轮正常 Provider 请求前执行自动/强制压缩；摘要使用当前 Provider、空工具定义、完整流收集与目标历史替换。
4. 在工具结果 `CommitRound` 前调用 `PrepareResults` 并发出持久化事件；成功 Provider usage 后更新 Manager 锚点。
5. 运行事件和工具结果集成测试。

**验证：** `go test ./internal/agent -run 'Compact|Context|ToolResult|Usage'` 通过。

## T6: 实现熔断、Plan 隔离和紧急重试

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`

**依赖：** T5

**步骤：**
1. 写失败测试：三次自动摘要失败后不再自动调用摘要；手动摘要成功后清零并恢复自动资格。
2. 写失败测试：Plan 多轮压缩只替换 taskHistory；普通和 `/do` 替换共享 Session 历史。
3. 实现摘要失败状态机与历史目标选择。
4. 写失败测试：模拟统一 Provider 上下文超限错误时，Runner 执行一次紧急摘要并仅重试同一正常请求一次。
5. 实现错误分类、单次紧急重试和终态映射，确保取消和摘要失败不提交半轮历史。
6. 运行完整 Agent 测试。

**验证：** `go test ./internal/agent` 通过。

## T7: 接入 `/compact`、TUI 与文档

**文件：** `internal/tui/update.go`、`internal/tui/tui_test.go`、`docs/README.md`

**依赖：** T5、T6

**步骤：**
1. 写失败测试：`/compact` 解析为 `ModeCompact`；空闲时开始任务、活动任务时显示现有忙碌错误。
2. 更新命令解析及 Agent 事件消费，展示触发类型、压缩前后估算值与失败错误，且不暴露敏感持久化内容。
3. 更新第八章 spec、plan、task、checklist 和人工测试方案，明确 `.mewcode/context`、惰性创建和恢复会话续写；不保留旧 `.mew/context` 指引。
4. 在文档索引加入 ch08 的四份开发文档及参考资料链接。
4. 运行 TUI 和文档索引检查。

**验证：** `go test ./internal/tui && rg -n 'ch08|上下文管理' docs/README.md` 输出预期条目。

## T8: 回归与端到端验证

**文件：** 全项目

**依赖：** T1–T7

**步骤：**
1. 运行 context、conversation、agent、config、tui 目标包的 race 检查。
2. 运行全项目测试；失败时回到归属任务修复并重跑对应验证。
3. 执行受控端到端场景：多轮大工具结果触发存盘，历史越过 167K 后摘要，`/compact` 在低用量时仍成功，Plan 压缩后共享历史未变。
4. 记录实际命令结果，更新第八章 checklist。

**验证：** `go test -race ./internal/context/... ./internal/conversation ./internal/agent ./internal/config ./internal/tui && go test ./...` 通过。

## 执行顺序

```text
T1 → T2 → T2a → T3 → T4 → T5 → T6 → T7 → T8
```
