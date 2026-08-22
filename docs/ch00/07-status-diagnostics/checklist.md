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
