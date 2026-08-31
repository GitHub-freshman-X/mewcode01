# MewCode 跨会话记忆与会话持久化 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/instructions/paths.go` | 用户级与项目级路径解析。 |
| 新建 | `internal/instructions/loader.go` | 两层指令加载与项目内 `@` 引用展开。 |
| 新建 | `internal/instructions/loader_test.go` | 指令优先级与引用安全测试。 |
| 修改 | `internal/conversation/session.go` | 增加 Journal 依赖并保证先持久化后更新内存。 |
| 新建 | `internal/conversation/journal.go` | Provider 无关 JSONL 编解码。 |
| 新建 | `internal/conversation/store.go` | 会话创建、列表、恢复、删除与清理。 |
| 修改 | `internal/conversation/session_test.go` | Session 持久化失败与计划记录测试。 |
| 新建 | `internal/conversation/store_test.go` | JSONL 容错、扫描、恢复、清理测试。 |
| 新建 | `internal/memory/paths.go` | 记忆类别至用户级/项目级目录映射。 |
| 新建 | `internal/memory/index.go` | MEMORY.md 读取限制与原子更新。 |
| 新建 | `internal/memory/operation.go` | 受限记忆操作与本地验证。 |
| 新建 | `internal/memory/service.go` | 提取、治理门槛与并发锁。 |
| 新建 | `internal/memory/memory_test.go` | 索引、操作、锁与治理测试。 |
| 修改 | `internal/agent/event.go` | 记忆后台状态事件（仅安全元数据）。 |
| 修改 | `internal/agent/runner.go` | Prompt OptionalModules 注入和终态后台记忆编排。 |
| 修改 | `internal/agent/runner_test.go` | 注入、异步提取与失败隔离集成测试。 |
| 修改 | `cmd/mewcode/main.go` | 初始化路径、会话存储、记忆服务和新会话。 |
| 修改 | `cmd/mewcode/main_test.go` | 启动创建会话与清理集成测试。 |
| 修改（按需） | `.mewcode/config.example.yaml` | 仅当实现中引入配置项时，列出默认值。 |

## T1：实现路径与分层指令加载

**文件：** `internal/instructions/paths.go`、`internal/instructions/loader.go`、`internal/instructions/loader_test.go`
**依赖：** 无

**步骤：**

1. 定义 `Paths`，由显式用户配置根目录和工作区根目录构造用户级 `mewcode`、项目 `.mewcode`、sessions 与两类 memory 目录。
2. 实现指令加载：先读取 `<用户根>/MEWCODE.md`，再读取 `<项目根>/.mewcode/MEWCODE.md`，以 `---` 分隔非空内容。
3. 实现只针对项目级文件的 `@相对路径` 展开，使用绝对路径 visited 集合、深度上限 5 与项目根 containment 检查；失败位置写入 HTML 注释后继续。
4. 先编写用户级在前、项目级在后、循环、五层限制、根外路径、缺失引用与根文件读取失败的测试，再完成最小实现。

**验证：** `go test ./internal/instructions -run 'Test.*Instruction|Test.*Include|Test.*Paths'` 通过。

## T2：实现 JSONL Journal 与 Session 原子提交

**文件：** `internal/conversation/journal.go`、`internal/conversation/session.go`、`internal/conversation/session_test.go`
**依赖：** 无

**步骤：**

1. 定义 Provider 无关的 JSONL 记录及工具调用、工具结果记录，编码 `provider.Message` 的文本、工具调用和工具结果；为普通历史和规划展示记录定义用途字段。
2. 为 `conversation.Session` 增加可选 `Journal`，使 `CommitRound` 和 `CommitPlan` 在修改 history、display 或 pending plans 前调用 Journal 追加记录。
3. 保持 `ReplaceHistory` 不写入 Journal，保证第八章压缩不重写原始会话记录。
4. 编写包含工具调用的一轮消息、`/plan` 展示记录和 Journal 追加失败的测试；失败时断言内存 history、display 与 pending plans 均未变化。

**验证：** `go test ./internal/conversation -run 'Test.*Journal|TestSession'` 通过。

## T3：实现会话创建、扫描、恢复与过期清理

**文件：** `internal/conversation/store.go`、`internal/conversation/store_test.go`
**依赖：** T2

**步骤：**

1. 实现 `SessionStore.Create`，创建 `<项目根>/.mewcode/sessions/<YYYYMMDD-HHMMSS-xxxx>.jsonl`，为会话附加 Journal，并确保同秒创建的 ID 不重复。
2. 实现 `List`：仅扫描经验证的 `.jsonl` 文件，从文件名取得创建时间，从记录得到标题、最后活跃时间和消息数，不创建元信息文件。
3. 实现 `Restore`：跳过坏行；验证工具调用和工具结果配对；从第一处不配对调用开始截断；恢复普通 history、计划 display 与 pending plans；超过 24 小时时加入时间跨度提醒。
4. 实现 `Delete` 与 `CleanupExpired`，只操作 sessions 目录内合法命名的 JSONL 文件，清理最后活跃超过 30 天的会话。
5. 用临时目录和固定时钟编写创建、同秒唯一、坏行、半行、配对截断、计划恢复、扫描元信息、时间跨度提醒及 30 天清理测试。

**验证：** `go test ./internal/conversation` 通过。

## T4：实现记忆路径、索引和受限本地操作

**文件：** `internal/memory/paths.go`、`internal/memory/index.go`、`internal/memory/operation.go`、`internal/memory/memory_test.go`
**依赖：** 无

**步骤：**

1. 定义四类 `MemoryKind` 与目标目录映射：`user`、`feedback` 到用户级，`project`、`reference` 到项目级。
2. 实现 `MEMORY.md` 加载，分别限制 200 行和 25KB，在截断时附加说明；缺失索引视为空内容。
3. 定义 `MemoryOperation`，解析模型 JSON 数组并校验 action、类别、slug 文件名、frontmatter、目标路径与目录 containment。
4. 实现单记忆文件与索引的原子更新；非法操作、越界路径、未知类别、无效 JSON 或 `noop` 都不得写入文件。
5. 编写目录映射、frontmatter、索引更新、双上限、非法操作拒绝和无操作不写入测试。

**验证：** `go test ./internal/memory -run 'Test.*Index|Test.*Operation|Test.*MemoryPath'` 通过。

## T5：实现记忆提取、治理门槛和锁

**文件：** `internal/memory/service.go`、`internal/memory/memory_test.go`
**依赖：** T3、T4

**步骤：**

1. 为记忆服务定义可替换的 Provider 调用、时钟、会话列表与日志依赖，构造不含工具定义的提取和治理请求。
2. 实现 `Extract`：输入模式和转录，接收最终文本，解析并执行受限操作；模型无操作时不产生文件修改。
3. 实现 `MaybeConsolidate`：检查目录存在、距锁文件上次成功整理至少 24 小时、10 分钟扫描节流、至少 5 个会话和锁可获得性；锁内容保存 PID，1 小时后视为过期。
4. 使提取与治理日志只包含阶段、状态、类别、数量、时长和大小等安全元数据，避免写入转录、指令、模型输出和错误载荷正文。
5. 编写 Provider 测试替身，覆盖提取 create/update/delete/noop、无工具请求、无效模型响应、时间/会话/节流门、锁竞争、过期锁和治理写入范围。

**验证：** `go test ./internal/memory` 通过。

## T6：在 Runner 注入上下文并异步编排记忆

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`、`internal/agent/runner_test.go`
**依赖：** T2、T5

**步骤：**

1. 扩展 `agent.Options`，接收预加载的 `prompt.OptionalModules` 和 `memory.MemoryService`；所有 `prompt.BuildBundle` 调用传入 OptionalModules。
2. 在普通聊天、`/do` 和 `/plan` 的最终文本回复已成功提交之后，复制对应可见转录，在 goroutine 内调用提取与治理。
3. 新增可选记忆事件或日志状态，仅传递类别、数量、阶段、状态和错误类型；TUI 不依赖该事件才能完成主任务。
4. 明确排除 `/compact`、取消、流失败、迭代限制和未知工具停止路径；记忆后台错误不得替换已完成的终态事件。
5. 扩展 scripted Provider 与可控 MemoryService 测试替身，验证首轮 Prompt 注入、三种模式触发、排除路径、异步非阻塞与错误隔离。

**验证：** `go test ./internal/agent` 通过。

## T7：接入启动入口并完成章节级验证

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`、`.mewcode/config.example.yaml`（按需）
**依赖：** T1、T3、T4、T5、T6

**步骤：**

1. 在启动入口通过 `os.UserConfigDir()` 派生用户级 `mewcode` 路径，不从 `--config` 文件位置推导。
2. 初始化 SessionStore，先执行过期清理、再创建新会话；不增加自动恢复、Slash Command 或 TUI 选择。
3. 加载两层指令和两个 memory 索引，构造 `prompt.OptionalModules` 与 `MemoryService` 后传给 Runner。
4. 核对实现没有新增可配置项；若发现新增项，补充配置加载、校验、`.mewcode/config.example.yaml` 默认值及测试，否则保持示例配置不变。
5. 编写启动路径与依赖构造测试，验证每次启动新建会话、清理已发生、用户级路径独立于 `--config`。
6. 运行格式化、完整测试与静态检查，更新本章文档中的验证记录；检查本轮未产生无关 `.mew/` 或 `logs/` 目录，若确认由本轮生成则清理。

**验证：**

```bash
gofmt -w internal/instructions internal/conversation internal/memory internal/agent cmd/mewcode
go test ./...
go vet ./...
git diff --check
```

期望：命令均以退出码 0 完成；第九章不新增 `/session` 命令。

## 执行顺序

```text
T1 ───────────────────────────────────┐
T2 → T3 ─┬─→ T5 ─┐                     ├─→ T7
T4 ──────┘         ├─→ T6 ─────────────┘
```

- T1 后可并行执行 T2 与 T4。
- T3 和 T4 完成后执行 T5。
- T2 和 T5 完成后执行 T6。
- T7 负责真实启动入口集成及全量验证。

---

## 过期会话联动清理补充任务

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `internal/conversation/store.go` | 将超期会话的 context 目录纳入启动期清理，并保留失败重试语义。 |
| 修改 | `internal/conversation/store_test.go` | 使用临时目录、固定时钟与可注入删除失败覆盖联动清理。 |
| 修改 | `cmd/mewcode/main.go` | 将既有 Logger 注入 SessionStore。 |
| 修改 | `cmd/mewcode/main_test.go` | 验证启动路径传入 Logger 且不改变新会话创建顺序。 |
| 修改 | `docs/ch09-memory/checklist.md` | 记录自动化验收项与证据。 |
| 修改 | `bugs/2026-08-31/001-expired-session-context-orphaned.md` | 记录修复方案、验证和最终状态。 |

## T8：实现可重试的会话与 context 联动清理

**文件：** `internal/conversation/store.go`、`internal/conversation/store_test.go`

**依赖：** 无

**步骤：**

1. 从现有 `sessionsDir` 推导同级 `context` 根目录；为有效会话 ID 构造并校验该根目录的直接子路径。
2. 令 `CleanupExpired` 对每个超期会话先清理 context 目录（缺失视为成功）、再调用既有 JSONL 删除；仅两步均成功时增加清理计数。
3. 保持 `Delete` 的手动删除语义不变；context 删除失败立即返回并保留 JSONL，JSONL 删除失败则保留 JSONL 供下一次清理重试。
4. 为目录与文件删除增加仅包内测试可替换的函数边界，避免依赖权限位或操作系统差异来模拟失败。
5. 使用临时目录和固定时钟先编写测试，再完成最小实现：成功联动删除、context 缺失、context 失败保留 JSONL、JSONL 失败可重试、刚好 30 天、未过期、无效 JSONL、其他会话和孤儿 context 均不受影响。

**验证：** `go test ./internal/conversation -run 'TestSessionStore.*Cleanup|TestSessionStoreDelete' -count=1` 通过。

## T9：接入安全日志并验证启动顺序

**文件：** `internal/conversation/store.go`、`internal/conversation/store_test.go`、`cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** T8

**步骤：**

1. 为 SessionStore 注入既有 Logger，默认使用 Nop；成功、context 缺失和失败仅记录阶段、状态、清理数量与错误类别，不记录会话 ID、目录路径或内容。
2. 在交互式启动路径创建 SessionStore 后、`CleanupExpired` 前注入已创建的 Logger；保持非交互入口不创建持久会话的现有行为。
3. 以真实 Logger 指向临时项目目录，断言日志包含安全状态字段，且刻意植入的会话 ID、路径标记和工具结果标记均不出现。
4. 扩展启动测试，断言过期清理仍发生在创建新 JSONL 前；不连接真实 Provider、不读取用户目录。

**验证：** `go test ./internal/conversation ./cmd/mewcode -run 'Test.*(Cleanup|Expired|SessionStore|Run)' -count=1` 通过。

## T10：章节、Bug 记录与回归验证

**文件：** `docs/ch09-memory/checklist.md`、`bugs/2026-08-31/001-expired-session-context-orphaned.md`

**依赖：** T8、T9

**步骤：**

1. 以实际命令和结果更新 checklist 的 AC5 与安全日志验收证据，明确本项全部由自动化测试覆盖、无需真实 Provider 手工测试。
2. 将 Bug 记录更新为已修复，写入实际根因、删除顺序、失败重试语义、已通过命令和未验证事项；若任一验证失败，改为待处理并说明原因。
3. 运行格式化、关联包回归、完整回归、静态检查和补丁检查；仅清理由本轮验证产生且确认无关的运行目录或日志目录。

**验证：**

```bash
gofmt -w internal/conversation/store.go internal/conversation/store_test.go cmd/mewcode/main.go cmd/mewcode/main_test.go
go test ./internal/conversation ./cmd/mewcode -count=1
go test ./... -count=1
go vet ./...
git diff --check
```

期望：所有命令退出码为 0；若既有无关失败阻塞全量回归，应在 Bug 记录中明确区分。

## 补充执行顺序

```text
T8 → T9 → T10
```
