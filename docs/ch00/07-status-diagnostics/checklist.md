# `/status` 诊断信息 Checklist

> 每项须通过自动化测试或可观察的本地 TUI 行为验证。`/status` 不得为收集信息而调用 Provider、Agent Loop 或读取敏感正文。

## 当前验收证据（2026-08-22）

- [x] `go test ./internal/status ./internal/subagent ./internal/memory ./internal/agent ./internal/command ./internal/tui ./cmd/mewcode -count=1` 通过。
- [x] `go test -race ./internal/status ./internal/subagent ./internal/agent ./internal/memory ./internal/command ./internal/tui -count=1` 通过。
- [x] `go test ./... -count=1` 通过。
- [x] `go build ./cmd/mewcode` 通过。
- [x] `git diff --check` 通过。
- [ ] `/private/tmp` fixture 的真实 Provider/TUI 后台任务场景尚未执行。

## 本地命令与基础状态

- [ ] **多行本地快照（AC1）**：空闲时执行 `/status` 显示多行系统反馈，且不产生 Agent 迭代、工具调用、Provider 请求、会话消息、Token 增量或文件写入。（验证：`go test ./internal/command ./internal/tui -run 'Test.*Status.*Local' -count=1`。）
- [ ] **运行环境（AC2）**：输出包含模式、工作目录、日志目录和权限模式；工作目录与启动工作区一致。（验证：构造固定工作区和日志目录的 Runner，执行 `/status`，断言字段和值。）
- [ ] **会话与用量（AC3）**：输出当前会话 ID、消息数、累计输入/输出 Token；新会话显示零，恢复会话恢复自身累计用量。（验证：`go test ./internal/conversation ./internal/command ./internal/tui -run 'Test.*(Status|Usage|Session.*Resume)' -count=1`。）

## 扩展与后台任务

- [ ] **扩展数量（AC4）**：输出实际 Runner registry 的工具数、Skill 数、用户/项目记忆数和 SubAgent 定义数；Skill Load 工具不会导致统计使用启动时旧 registry。（验证：`go test ./internal/agent ./internal/memory ./internal/command -run 'Test.*(Status|Snapshot|Memory|Skill)' -count=1`。）
- [ ] **无后台任务（AC5）**：没有任务运行时明确显示 `后台任务：0`，不显示历史终态任务。（验证：TaskManager 完成、失败、取消任务后执行快照断言。）
- [ ] **后台任务详情（AC5）**：显式后台、Fork、ESC 接管和 120 秒自动接管的运行中任务均显示 ID、名称、`running` 和时长；终态后立即从 `/status` 任务列表消失。（验证：`go test ./internal/subagent ./internal/agent ./internal/command -run 'Test.*(Background|Status|SubAgent)' -count=1`。）
- [ ] **稳定并发快照（AC5、N2、N3、N4）**：任务创建、接管、进度更新和终态变化并发发生时，`/status` 不 panic、不阻塞，任务按 StartedAt、ID 稳定排序。（验证：`go test -race ./internal/subagent ./internal/agent ./internal/command -run 'Test.*(Background|Status|TaskManager)' -count=1`。）

## 安全、降级与性能

- [ ] **敏感信息不泄露（AC6）**：配置、用户输入、任务提示、结果和失败中存在唯一敏感标记时，`/status` 输出不包含该标记或其他正文。（验证：`go test ./internal/status ./internal/agent ./internal/command -run 'Test.*(Status|Sensitive|Redact)' -count=1`。）
- [ ] **可选模块降级（AC7）**：Memory、Skill、SubAgent、权限或日志目录缺失时，仍显示基础模式、会话和 Token；缺失字段显示 `不可用` 或 `0`。（验证：`go test ./internal/agent ./internal/command ./internal/tui -run 'Test.*(Status|Optional|Nil)' -count=1`。）
- [ ] **零扫描与零网络（F1、N1）**：`/status` 仅读取内存快照；不扫描工作区或 memory 目录、不读取日志正文、不调用 Provider。（验证：使用计数/失败替身注入目录访问和 Provider，断言 `/status` 后计数为零。）

## 端到端人工场景

- [ ] **fixture 启动诊断**：在 `/private/tmp/mewcode-ch13-manual.*` fixture 启动 MewCode，执行 `/status`。（验证：工作目录等于 fixture，日志目录位于该工作目录下，权限模式、会话 ID、消息数、工具/Skill/记忆/SubAgent 数量均可见；TUI 不显示 Agent 迭代。）
- [ ] **后台任务诊断**：在同一 fixture 让主 Agent 显式后台启动一个定义式 SubAgent，任务运行期间执行 `/status`，完成后再次执行。（验证：运行期间显示该任务的安全 ID、名称、`running` 和时长；完成后显示 `后台任务：0`，不显示任务结果正文。）

## 构建与回归

- [ ] **格式化、竞态、全量测试与构建**：变更 Go 文件已格式化，相关竞态测试、全量测试和可执行程序构建均通过。（验证：`gofmt -w $(git diff --name-only -- '*.go') && go test -race ./internal/status ./internal/subagent ./internal/agent ./internal/memory ./internal/command ./internal/tui && go test ./... -count=1 && go build ./cmd/mewcode`。）

## 缓存命中率扩展验收

- [x] **Provider 用量归一化（AC8）**：OpenAI 返回的 `cached_tokens` 累计为缓存读取；Claude 返回的 `cache_read_input_tokens` 和 `cache_creation_input_tokens` 分别累计为缓存读取和缓存写入。（证据：既有 provider adapter 测试与 `go test ./... -count=1` 于 2026-08-30 通过。）
- [x] **累计公式（AC8、AC9）**：会话命中率等于缓存读取占去重后的实际总输入；OpenAI 的缓存读取/写入已包含在 `input_tokens` 中，Claude 的缓存读取/写入独立计量。多轮累加后按总量计算，而非平均每轮百分比；百分比按整数四舍五入。（证据：`TestUsageCacheHitRate` 与 `go test ./internal/conversation ./internal/provider -count=1` 于 2026-08-30 通过。）
- [x] **无数据与未知历史（AC9、AC11）**：新会话尚无用量、分母为零以及由旧格式日志恢复的会话，状态栏和 `/status` 均显示 `缓存：—` 或等价缓存命中率 `—`，不显示虚假的 `0%`，且不 panic。（证据：`TestUsageCacheHitRate`、旧记录恢复测试和相关包测试于 2026-08-30 通过。）
- [x] **新会话持久化（AC11）**：新增 usage JSONL 记录包含缓存读取、缓存写入及可用性所需元数据；重新恢复后 in/out、缓存读/写和命中率与写入前一致。（证据：`TestSessionRecordUsagePersistsCacheTokenCounts`、`TestSessionStoreRestoreAggregatesKnownCacheUsage` 通过。）
- [x] **旧日志兼容（AC11）**：不含缓存字段的既有 usage JSONL 仍能恢复原有 in/out Token；缓存统计标记为未知，不修改原文件。（证据：`TestSessionStoreRestoreAggregatesUsageAndIgnoresItForMetadata` 通过。）
- [ ] **状态栏展示（AC3、AC9）**：idle、流式、终态和前台子 Agent 四种状态均显示相同的会话累计 `缓存：NN%`；窄窗口仍可渲染、滚动和继续输入。（自动化证据：`TestStatusShowsCacheHitRate` 与 `go test ./internal/tui -count=1` 于 2026-08-30 通过；真实 Provider 场景见 `manual_scenarios.md`，窄窗口检查未执行。）
- [x] **`/status` 展示（AC4、AC10）**：保留 `Token：in/out`，并固定顺序显示“缓存读取”“缓存写入”“缓存命中率”；命令不访问 Provider 或增加 Agent 迭代。（证据：`TestStatusCommandRendersSafeRuntimeSnapshot` 与 `go test ./internal/command -count=1` 于 2026-08-30 通过。）
- [x] **聚合数据安全（AC11、AC12）**：缓存日志记录、状态栏和 `/status` 只含 Token 数与百分比；植入 Prompt、工具结果、API Key、原始响应标记后均不泄露。（证据：持久化 schema 仅含聚合 Token，沿用 `/status` 安全输出测试；`go test ./... -count=1` 于 2026-08-30 通过。）
- [x] **回归与构建（AC12）**：格式化后，conversation、provider、command、TUI 相关测试、全量测试和 `mewcode` 构建均通过。（证据：`go test ./... -count=1 && go build ./cmd/mewcode && git diff --check` 于 2026-08-30 通过。）
