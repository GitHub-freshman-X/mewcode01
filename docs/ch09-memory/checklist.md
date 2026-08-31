# MewCode 跨会话记忆与会话持久化 Checklist

> 每一项通过运行代码或观察行为验证；验收全程使用临时目录、固定时钟和 Provider 测试替身，不访问真实模型、网络或用户实际目录。

## 指令与记忆注入

- [ ] **AC1 分层首轮注入**：用户级和项目级 `MEWCODE.md` 同时存在时，首轮 Prompt 同时包含两者，用户级文本出现在项目级文本之前；两个 `MEMORY.md` 索引亦被注入。任一文件缺失时，其他可用文件仍注入。（验证：运行 `go test ./internal/instructions ./internal/agent -run 'Instruction|OptionalModules|MemoryInjection' -count=1`，期望顺序和缺失文件分支断言通过）
- [ ] **AC2 项目内引用安全**：有效嵌套引用被展开；循环只展开一次；第 5 层后的引用停止展开；项目根外和不存在路径以安全标记保留，且后续内容仍存在。（验证：运行 `go test ./internal/instructions -run 'Include|Instruction' -count=1`，期望每个引用边界断言通过）
- [ ] **AC7 索引双上限**：超过 200 行或 25KB 的 `MEMORY.md` 在首轮请求中只保留上限内内容，并包含截断提示。（验证：运行 `go test ./internal/memory ./internal/agent -run 'Index.*Limit|MemoryInjection' -count=1`，期望行数、字节数和提示断言通过）

## 会话持久化与恢复

- [ ] **AC3 JSONL 追加与 Provider 中立**：含工具调用的一轮任务结束后，会话 JSONL 可重建用户、助手、工具调用和工具结果；记录不含厂商协议特有的原始请求格式。模拟末行中断时，完整前缀仍可恢复。（验证：运行 `go test ./internal/conversation -run 'Journal|RoundTrip|Partial|Provider' -count=1`，期望记录字段和恢复断言通过）
- [ ] **AC4 容错恢复与上下文衔接**：恢复会跳过坏行、从首个未配对工具调用处截断，超过 24 小时加入时间跨度提醒；恢复后的超长历史在下一次模型调用前仍触发第八章压缩。（验证：运行 `go test ./internal/conversation ./internal/agent -run 'Restore|Malformed|Unpaired|TimeGap|Recovered.*Compact' -count=1`，期望历史、提醒和压缩请求断言通过）
- [x] **AC5 生命周期与联动清理**：创建的会话 ID 满足 `YYYYMMDD-HHMMSS-xxxx` 且同秒不冲突；扫描仅凭 JSONL 得到列表元信息；启动清理删除最后活跃严格超过 30 天的会话 JSONL 及同 ID context 目录，保留未过期和恰好 30 天的会话及其 context。（证据：`go test ./internal/conversation ./cmd/mewcode -count=1` 于 2026-08-31 通过；会话存储与启动测试覆盖文件和元信息。）
- [x] **联动清理失败可重试**：context 目录不存在时仍删除 JSONL；context 删除失败时 JSONL 保留；JSONL 删除失败时 JSONL 保留且下次清理在 context 缺失状态下继续完成。无效 JSONL、相邻会话、其他会话 context 和没有对应 JSONL 的孤儿 context 均不被触碰。（证据：`TestSessionStoreCleanupExpiredRemovesMatchingContextOnly` 与 `TestSessionStoreCleanupExpiredRetriesAfterDeleteFailures` 使用临时目录和删除函数替身覆盖，2026-08-31 通过。）
- [ ] **崩溃写入顺序**：Journal 追加失败时，history、display 和 pending plans 均不变；第八章的 `ReplaceHistory` 不重写原始 JSONL。（验证：运行 `go test ./internal/conversation -run 'Journal.*Failure|ReplaceHistory' -count=1`，期望内存快照和文件内容不变）

## 自动记忆与治理

- [ ] **AC6 自动提取、分类与去重**：正常完成的普通聊天、`/do`、`/plan` 均可异步产生或更新正确目录、类别、frontmatter 和索引；无价值结果不写入；重复语义由模型返回的更新操作合并而非创建第二个文件。（验证：运行 `go test ./internal/memory ./internal/agent -run 'Extract|Memory.*Mode|Noop|Update|Plan|Do' -count=1`，期望文件、索引和调用次数断言通过）
- [ ] **AC8 惰性治理与锁**：目录、24 小时时间门、10 分钟节流、至少 5 个会话和锁均满足时才触发治理；治理只能改记忆目录；有效锁阻止第二次执行；主任务无需等待治理完成。（验证：运行 `go test ./internal/memory ./internal/agent -run 'Consolidate|Lock|Throttle|SessionGate|NonBlocking' -count=1`，期望门槛、锁和非阻塞断言通过）
- [ ] **后台失败隔离**：`/compact`、取消、模型失败、迭代限制和未知工具停止不触发提取；提取或治理失败不会将已完成主任务改为失败。（验证：运行 `go test ./internal/agent -run 'Memory.*(Compact|Cancelled|Failure|Iteration|Unknown)|Background.*Failure' -count=1`，期望终态和调用次数断言通过）

## 安全、配置与回归

- [ ] **AC9 安全日志与路径隔离**：日志只含阶段、状态、数量、类型、时长或大小等元数据，不含指令、会话、记忆、工具结果或密钥正文；所有写入均在用户级或项目级记忆目录、会话目录内。（验证：运行 `go test ./internal/instructions ./internal/conversation ./internal/memory ./internal/agent -run 'Log|Path|Containment|Safe' -count=1`，期望敏感文本缺失且越界写入被拒绝）
- [x] **过期清理安全日志**：启动期 context 联动清理记录阶段、状态、数量和错误类别，但不写入会话 ID、context 路径或工具结果 canary。（证据：`TestSessionStoreCleanupExpiredLogsOnlySafeMetadata` 读取真实临时 JSONL 日志并断言安全字段与 canary 缺失；`go test ./internal/conversation ./cmd/mewcode -count=1` 于 2026-08-31 通过。）
- [ ] **固定配置约定**：用户级根目录由 `os.UserConfigDir()/mewcode` 派生，而非 `--config` 路径；本章未新增 YAML 配置项。若实现时新增配置项，则配置加载、校验和 `.mewcode/config.example.yaml` 必须同步通过。（验证：运行 `go test ./cmd/mewcode ./internal/config -run 'UserConfig|DefaultPath|Config' -count=1`，期望路径和配置断言通过）
- [x] **离线全量回归**：所有新增自动化测试不访问真实模型或用户目录，既有 Provider、Prompt、Agent、TUI 与上下文管理测试保持通过。（证据：`go test ./... -count=1` 于 2026-08-31 通过。）
- [x] **静态与格式检查**：Go 文件经过格式化，静态检查与补丁空白检查通过。（证据：`gofmt -d`、`go vet ./...` 与 `git diff --check` 于 2026-08-31 通过。）

## 本次修复的离线验收

- [x] **无需人工 Provider 测试**：所有联动清理场景只依赖临时项目目录、固定时钟、删除函数测试替身与本地 Logger；不创建网络连接、不调用模型、不读取真实用户配置目录。（证据：新增测试均使用 `t.TempDir`、固定时间、stub Provider 和本地 Logger；`go test ./internal/conversation ./cmd/mewcode -count=1` 于 2026-08-31 通过。）

## 端到端场景

- [ ] **新会话获得长期上下文**：在临时用户目录写入个人 `MEWCODE.md` 与 `MEMORY.md`，在临时项目写入项目 `MEWCODE.md`、项目 `MEMORY.md` 和项目内引用文件；启动新会话并发送一条普通请求，观察 Provider 测试替身收到的首轮 Prompt 依次包含用户规则、项目规则、两个受限索引和展开的项目引用内容。（验证：运行 `go test ./cmd/mewcode ./internal/agent -run 'EndToEnd.*NewSession|Startup.*Injection' -count=1`，期望首轮请求快照断言通过）
- [ ] **存档后安全恢复**：创建包含工具调用、工具结果与计划的会话，向 JSONL 追加坏行和未配对工具调用，模拟超过 24 小时后恢复；观察完整前缀与计划可用、坏尾不进入模型历史、时间跨度提醒出现，并确认恢复不自动成为下一次启动的默认会话。（验证：运行 `go test ./internal/conversation ./cmd/mewcode -run 'EndToEnd.*Restore|Restore.*Corrupt|Startup.*NewSession' -count=1`，期望恢复历史、提醒和新会话断言通过）
