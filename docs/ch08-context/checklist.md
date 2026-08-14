# MewCode 上下文管理 Checklist

> 每一项通过运行代码或观察脚本化 Agent 行为验证；所有摘要测试使用测试 Provider，不访问真实模型服务。

## 第一层：工具结果预算

- [x] **AC1 单项持久化与回读放行**：超阈值工具结果在下一轮历史中变为大小、预览和会话专属路径；通过该路径回读的完整内容不再被第二次替换。（验证：`go test ./internal/context ./internal/agent -run 'SingleResult|Persist|Readback'`，期望历史不含完整原结果、文件内容完整且回读保持原文。）
- [x] **AC2 聚合预算按大小收缩**：多个工具结果分别未超单项阈值、合计超消息预算时，最大结果先被替换，直到总量合规，调用顺序不变。（验证：`go test ./internal/context -run 'Aggregate|Largest|Order'`，期望替换集合与结果顺序断言通过。）
- [x] **AC1/AC2 写入即终态**：后续轮次不会重新替换或恢复已提交消息，持久化结果只经路径按需读取。（验证：`go test ./internal/context ./internal/conversation -run 'Final|Stable|Persist'`，期望历史快照逐轮保持不变。）

## 结果目录生命周期与恢复定位

- [x] **AC11 惰性创建**：创建新会话、创建 Runner 和创建结果存储对象均不产生 context 目录；首次超过阈值的工具结果才创建 `.mewcode/context/<会话 ID>/tool-results/`。（验证：`go test ./internal/context ./internal/agent -count=1`，通过。）
- [x] **AC11 目录迁移完成**：新结果引用均位于 `.mewcode/context`，不再创建 `.mew/context`。（验证：`TestRunnerPersistsToolResultsBeforeCommit` 检查新目录存在、旧目录不存在，已通过。）
- [x] **AC12 恢复会话续写**：以相同持久会话 ID 创建恢复后的 Runner 时，旧结果引用仍可读取；新大结果写入同一 `tool-results` 目录且不覆盖已有文件。（验证：`TestResultStoreReusesSessionDirectoryWithoutOverwriting` 已通过。）
- [x] **AC12 安全降级**：没有工作区或持久会话 ID 时，超阈值工具结果保留原文且任务继续完成。（验证：`TestPrepareResultsWithoutStoreKeepsOriginalContent` 已通过。）

## 阈值、自动与手动压缩

- [x] **AC3 默认自动线正确**：默认窗口 200K、摘要预留 20K、自动余量 13K 时，167K 以上自动压缩，未超过时不压缩。（验证：`go test ./internal/context ./internal/agent -run 'Default|167000|Automatic'`，期望摘要 Provider 调用次数分别为 1 和 0。）
- [x] **AC3 usage 锚点估算正确**：最近真实输入 usage 之前的历史不重复按字符累计，只有锚点后的新增消息影响估算。（验证：`go test ./internal/context -run 'Estimate|UsageAnchor|Increment'`，期望估算值只随新增消息变化。）
- [x] **AC4 `/compact` 无条件执行**：低于自动线时输入 `/compact` 仍启动无工具摘要任务，并展示 manual、压缩前后 Token。（验证：`go test ./internal/tui ./internal/agent -run 'Compact|Manual|LowUsage'`，期望请求 Tools 为空且有压缩事件。）
- [x] **AC9 强制线正确**：自动熔断后，达到 177K（200K - 20K - 3K）仍执行强制压缩。（验证：`go test ./internal/agent -run 'Forced|177000|Breaker'`，期望触发原因为 forced。）

## 摘要质量与历史重建

- [x] **AC5 摘要请求受限**：摘要请求明确禁止工具调用，Tools 为空，输出预算使用配置值；提示包含 analysis 草稿、summary 正稿和九段结构。（验证：`go test ./internal/context ./internal/agent -run 'Summary.*Prompt|NoTools|NineSections'`，期望请求和提示断言通过。）
- [x] **AC5 仅保留正式摘要**：摘要 Provider 返回 `<analysis>` 与 `<summary>` 时，重建历史只包含 summary；缺失/空 summary 失败且原历史未改。（验证：`go test ./internal/context -run 'ExtractSummary|AnalysisDiscarded|MalformedSummary'`，期望内容与原子性断言通过。）
- [x] **摘要重复标签容错**：真实 Provider 返回多个完整 summary 块时，仅最后一个完整块进入重建历史；标签外文本和先前块均被丢弃，嵌套、空白或未闭合标签仍失败。（验证：`go test ./internal/context ./internal/agent -run 'ExtractSummarySelectsLastCompleteNonEmptyBlock|AutomaticCompactBeforeNormalRequest|LogsContextCompactionLifecycle' -count=1`，期望通过且日志不含摘要正文。）
- [x] **AC6 近期消息成组保留**：重建后保留约 10K Token 或至少五条近期消息；assistant 工具调用不会脱离对应 user tool result。（验证：`go test ./internal/context -run 'Recent|Minimum|ToolPair'`，期望消息配对和最小数量断言通过。）
- [x] **AC5/AC6 边界消息防臆测**：重建历史包含要求重新读取缺失文件、代码和错误细节的边界消息，用户原话按优先级保留。（验证：`go test ./internal/context -run 'Boundary|UserVerbatim'`，期望提示文本和用户消息断言通过。）

## 历史隔离、熔断与恢复

- [x] **AC7 Plan 历史隔离**：多轮 `/plan` 触发压缩时仅替换 `taskHistory`；结束后共享 Session 与规划前相同。（验证：`go test ./internal/agent ./internal/conversation -run 'Plan.*Compact|Plan.*Isolation'`，期望共享历史快照相等。）
- [x] **AC7 普通执行与 Do 共享更新**：普通执行和 `/do` 压缩后，下一次正常请求读取更新后的共享历史。（验证：`go test ./internal/agent -run 'Act.*Compact|Do.*Compact'`，期望后续请求出现边界消息和摘要。）
- [x] **AC8 三次失败熔断**：三次自动摘要失败后不再进行第四次自动摘要，历史不被失败摘要部分改写。（验证：`go test ./internal/agent -run 'Breaker|ThreeFailures|NoFourthAttempt'`，期望调用计数为 3。）
- [x] **AC8 手动恢复资格**：熔断后 `/compact` 成功会清零失败计数，后续到达自动线可再次自动压缩。（验证：`go test ./internal/agent -run 'Manual.*Reset|Breaker.*Recovery'`，期望后续 automatic 调用存在。）

## 紧急恢复、配置与界面

- [x] **AC9 紧急重试有上限**：正常请求被识别为上下文超限时，系统执行 emergency 压缩并重试原请求一次；重试仍失败则终态失败。（验证：`go test ./internal/agent -run 'Emergency|ContextTooLong|SingleRetry'`，期望正常请求总次数为 2。）
- [x] **AC10 配置兼容与校验**：无 context 配置时应用四个默认值；负预算、零第一层预算、窗口不足及非法覆盖均被拒绝。（验证：`go test ./internal/config -run 'Context|Defaults|Invalid'`，期望兼容配置通过、非法配置报字段错误。）
- [x] **AC10 Provider 中立与离线可测**：Anthropic 与 OpenAI request adapter 均能接收摘要形式的空工具请求；全部测试替身不访问网络。（验证：`go test ./internal/provider/... ./internal/agent -run 'Summary|Context|Request'`，期望两个 adapter 的请求结构和离线断言通过。）
- [x] **AC4/AC10 TUI 只消费事件**：TUI 显示压缩原因、前后估算与失败摘要，不能修改阈值或历史；活动任务期间 `/compact` 被现有单任务保护拒绝。（验证：`go test ./internal/tui -run 'Compact|CompactionEvent|Busy'`，期望展示状态与拒绝错误断言通过。）

## 编译与回归

- [x] **关联包竞态检查通过**。（验证：`go test -race ./internal/context/... ./internal/conversation ./internal/agent ./internal/config ./internal/tui`，期望通过且无 race。）
- [x] **全项目测试通过**。（验证：`go test ./...`，期望通过。）
- [x] **第八章文档可发现**。（验证：`rg -n 'ch08|上下文管理' docs/README.md`，期望索引包含参考、Spec、Plan、Tasks 与 Checklist 链接。）

## 端到端场景

- [x] **E2E 1：长工具循环自动压缩**：脚本化 Agent 连续读取大文件，第一层保存超大结果，累计历史超过 167K 后产生摘要；随后模型继续并给出最终回复。（验证：运行 `go test ./internal/agent -run 'AutomaticCompactBeforeNormalRequest|PersistsToolResultsBeforeCommit'`，期望持久化事件、automatic 事件和 completed 终态依次出现。）
- [x] **E2E 2：用户主动话题切换**：已有低用量历史时输入 `/compact`，摘要成功后再提交新任务；新请求包含边界消息与摘要且能正常完成。（验证：运行 `go test ./internal/tui ./internal/agent -run 'ManualCompact|CompactionEvent'`，期望 manual 后下一任务 completed。）
- [x] **E2E 3：Plan 不污染主会话**：`/plan` 读取足量内容并压缩后返回计划，随后普通任务或 `/do` 的请求不包含规划的内部工具结果。（验证：运行 `go test ./internal/agent -run 'PlanCompactReplacesOnlyTaskHistory|Plan.*Isolation'`，期望 Plan 内有压缩、共享历史没有规划中间消息。）
- [ ] **E2E 4：恢复后继续存放大结果**：创建会话并产生一个大工具结果，记录会话 ID 和结果路径；恢复同一会话后再产生一个大结果。（验证：两个结果均位于 `.mewcode/context/<相同会话 ID>/tool-results/`，均可读取，文件数为二。）
