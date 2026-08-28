# MewCode 非交互式单任务执行 Checklist

> 本清单中的命令和观察点以实现完成后的实际行为为准；执行结果将在开发完成时回填。

## 命令与输入

- [ ] **AC1 子命令配置**：运行 `mewcode run --config <有效配置> --prompt '任务'` 时使用该配置启动；运行既有 `mewcode --config <有效配置>` 时仍进入 TUI。（验证：`go test ./cmd/mewcode -run 'TestRun.*(Config|TUI)' -count=1`）
- [ ] **AC2 短文本输入**：仅提供非空 `--prompt` 时，命令启动一条普通 Agent 任务。（验证：离线 Provider 测试断言收到一次 `ModeAct` 请求。）
- [ ] **AC2 文件输入**：仅提供非空 `--prompt-file` 时，文件文本作为任务启动。（验证：临时任务文件的离线测试断言 Provider 收到精确文本。）
- [ ] **AC2 输入失败**：缺少输入、同时提供两种输入、空文本或无法读取任务文件时，命令在模型请求前失败并返回退出码 1。（验证：`go test ./cmd/mewcode -run 'Test.*Run.*(Input|Prompt|File)' -count=1`）
- [ ] **AC3 工作目录与模式**：非交互调用以当前目录作为工具工作区，且只启动 `ModeAct`。（验证：捕获 Runner 请求和工具工作区的离线集成测试。）

## 共享运行环境与安全边界

- [ ] **AC3/AC4 共享加载**：非交互调用加载项目/用户指令、记忆索引、Skill、Hook、MCP、权限规则和安全日志，与交互入口使用相同来源。（验证：`go test ./cmd/mewcode -run 'Test.*NonInteractive.*(Instructions|Memory|Skill|Hook|MCP|Log)' -count=1`）
- [ ] **AC4 权限自动拒绝**：未匹配且需确认的副作用工具调用不会阻塞；Agent 收到拒绝工具结果并可继续其后续轮次。（验证：`go test ./cmd/mewcode -run 'Test.*NonInteractive.*Permission' -count=1`）
- [ ] **AC5 前台子 Agent**：非交互调用中的前台子 Agent 在顶层命令完成前结束，或被总超时/取消同步终止。（验证：`go test ./internal/agent -run 'Test.*Foreground.*SubAgent' -count=1`）
- [ ] **AC5 后台与 Fork 拒绝**：显式后台和 Fork 子 Agent 请求返回结构化工具失败；既有交互式运行时的后台行为不变。（验证：`go test ./internal/agent -run 'Test.*(Background|Fork)' -count=1`）
- [ ] **AC10 临时会话**：非交互调用后不生成可恢复会话，且不触发长期记忆写入；现有安全元数据日志仍存在。（验证：`go test ./cmd/mewcode -run 'Test.*NonInteractive.*(Temporary|Memory|Log)' -count=1`）
- [ ] **N3 不扩大权限与不泄露敏感内容**：非交互模式不自动批准需确认操作，且日志、stderr、JSON 不含密钥、原始任务文本、工具结果正文或请求头。（验证：敏感 canary 字符串测试与日志/输出断言。）

## 生命周期、输出与状态

- [ ] **AC6 默认超时**：未指定 `--timeout` 时创建 30 分钟总 deadline。（验证：参数解析单元测试。）
- [ ] **AC6 关闭超时**：`--timeout 0` 不创建总 deadline。（验证：参数解析单元测试。）
- [ ] **AC6 超时与取消**：总 deadline 到达时任务被取消、结果状态为 `timed_out`、退出码为 3；中断取消时状态为 `cancelled`、退出码为 2。（验证：`go test ./cmd/mewcode -run 'Test.*(Timeout|Cancel)' -count=1`）
- [ ] **AC7 默认文本流**：默认模式按到达顺序实时写出模型文本增量，成功完成后 stdout 等于完整最终文本，stderr 不含正常文本。（验证：捕获缓冲区的离线流式 Provider 测试。）
- [ ] **AC8 JSON 单文档**：`--json` 模式不输出中间文本或诊断到 stdout，结束后 stdout 可解析为一个 JSON 文档。（验证：`go test ./cmd/mewcode -run 'Test.*JSON' -count=1`）
- [ ] **AC8 JSON 字段完整**：JSON 结果含 `status`、`stop_reason`、`error`、`final_text`、`elapsed_ms`、`iterations` 与 `usage`；无错误字段保持稳定空值语义。（验证：JSON 解码和字段断言测试。）
- [ ] **AC9 退出码**：完成为 0，启动/参数/运行失败为 1，取消为 2，超时为 3，迭代或未知工具安全停止为 4。（验证：`go test ./cmd/mewcode -run 'Test.*Exit' -count=1`）
- [ ] **N1/N2 非阻塞与可脚本处理**：无 TUI 时不会等待权限输入或后台任务；同一终态的 JSON schema 与退出码稳定。（验证：权限、子 Agent、JSON 和退出码的离线集成测试。）

## 兼容性与回归

- [ ] **AC11 TUI 回归**：现有 TUI 启动、配置覆盖、会话创建和权限桥测试全部通过。（验证：`go test ./cmd/mewcode -run 'TestRun' -count=1`）
- [ ] **AC11 离线覆盖**：非交互入口的参数、输出、权限、临时状态和生命周期测试不访问真实模型服务。（验证：`go test ./cmd/mewcode ./internal/agent -count=1`）
- [ ] **构建与全量回归**：格式化、全量 Go 测试和 CLI 构建通过。（验证：`gofmt -w cmd/mewcode/main.go cmd/mewcode/run.go internal/agent/subagent.go`、`go test ./...`、`go build ./cmd/mewcode`、`git diff --check`）
- [ ] **文档一致性**：README 写明 `mewcode run`、两种输入、30 分钟默认超时、JSON 输出、退出码和“需确认即拒绝”的权限行为；本章四份文档与实际实现一致。（验证：`rg -n 'mewcode run|--prompt-file|--json|30|确认' README.md docs/ch00/08-noninteractive-run`）

## 端到端场景

- [ ] **场景 1：成功的自动化调用**：在临时工作区和本地离线 Provider 下运行 `mewcode run --config <测试配置> --prompt '创建所需改动' --json`，观察 stdout 是单个 JSON、状态为 `completed`、退出码为 0，且工作区改动由既有权限规则决定。（验证：命令包端到端测试。）
- [ ] **场景 2：需确认的写操作**：在默认权限且没有匹配允许规则时运行会请求写文件的任务，观察进程不等待输入，Agent 收到拒绝结果，输出与退出码遵循其最终 Agent 终态。（验证：脚本化 Provider 的两轮端到端测试。）
- [ ] **场景 3：超时的自动化调用**：使用阻塞的本地测试 Provider 并设置很短 `--timeout`，观察任务被取消、JSON 状态为 `timed_out`、退出码为 3，且 stdout 没有非 JSON 内容。（验证：命令包端到端测试。）

## 本次执行记录（2026-08-28）

- [x] 参数互斥、空输入、负超时、默认文本输出、JSON 单文档和超时终态已由 `go test ./cmd/mewcode ./internal/agent` 验证。
- [x] 禁用后台时的显式后台与 Fork 子 Agent 拒绝已由 `TestSubAgentRuntimeDisableBackgroundRejectsBackgroundAndFork` 验证；交互式既有子 Agent 测试同步通过。
- [x] 命令包、Agent 包与全仓库回归已通过：`go test ./...`。
- [x] CLI 构建已通过：`go build ./cmd/mewcode`；构建产生的本地二进制已清理。
- [x] 文档与代码格式检查已通过：`git diff --check`。
- [ ] 未执行真实模型服务、真实进程中断信号和实际 MCP 服务的人工端到端验证；这些不影响离线自动化验证结论。
