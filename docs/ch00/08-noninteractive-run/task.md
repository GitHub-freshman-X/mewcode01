# MewCode 非交互式单任务执行 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `cmd/mewcode/main.go` | CLI 分流、共享运行时初始化与临时运行时选项 |
| 新建 | `cmd/mewcode/run.go` | `run` 参数、任务输入、事件消费、输出、超时与退出码 |
| 修改 | `cmd/mewcode/main_test.go` | 非交互入口离线测试与既有 TUI 回归 |
| 修改 | `internal/agent/subagent.go` | 后台子 Agent 禁用开关与前台等待语义 |
| 修改 | `internal/agent/subagent_test.go` | 后台禁用、Fork 拒绝与前台子 Agent 回归 |
| 修改 | `README.md` | 用户可见命令、参数、输出、权限与超时说明 |
| 新建 | `docs/ch00/08-noninteractive-run/{spec,plan,task,checklist,manual_scenarios}.md` | 本章需求、设计、任务与自动化/人工验收记录 |

## T1: 定义非交互命令契约与离线测试支点

**文件：** `cmd/mewcode/run.go`、`cmd/mewcode/main_test.go`

**依赖：** 无

**步骤：**

1. 在新文件中定义命令参数、最终结果和退出码的内部表示。
2. 提供可注入标准输出、标准错误、时钟/信号上下文或等价依赖的执行边界，避免测试依赖真实终端和真实模型。
3. 扩展已有测试替身，使其可发出文本、终态、工具调用和取消相关事件。
4. 为参数缺失、双输入、空输入与任务文件读取失败编写失败测试。

**验证：** `go test ./cmd/mewcode -run 'Test.*Run' -count=1` 通过，且失败用例不调用 Provider。

## T2: 实现 CLI 分流与共享运行时构建

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`

**依赖：** T1

**步骤：**

1. 在顶层参数解析前识别精确的 `run` 子命令；其余输入继续走现有 TUI 参数解析。
2. 将现有启动流程抽取为共享构建过程，保持配置、Provider、工作区工具、MCP、Hook、Skill、指令、记忆索引、权限和日志的加载顺序。
3. 为 TUI 构建持久会话、SessionStore、Memory 服务和 `tui.PermissionBridge`；为 `run` 构建内存 Session，不传入 SessionStore、Session ID、Memory 服务或确认桥，并关闭后台子 Agent。
4. 增加回归测试，断言 `mewcode --config <path>` 仍调用 TUI，而 `mewcode run --config <path> ...` 不调用 TUI。

**验证：** `go test ./cmd/mewcode -run 'TestRun(ConfigOverride|DoesNotPrintStartupCatBannerBeforeTUI|.*NonInteractive.*)' -count=1` 通过。

## T3: 实现单任务执行、文本与 JSON 输出

**文件：** `cmd/mewcode/run.go`、`cmd/mewcode/main_test.go`

**依赖：** T1、T2

**步骤：**

1. 启动一条 `ModeAct` 任务并消费直到终态；按事件顺序累积文本和终态摘要。
2. 默认模式实时向标准输出写入文本增量，且不将诊断写入标准输出。
3. `--json` 模式抑制中间文本，只在结束时输出单个包含约定字段的 JSON 文档。
4. 将完成、失败和安全停止映射为约定的退出码与结果状态。
5. 添加文本流、JSON 可解析性、JSON 字段完整性、标准错误分流和退出码测试。

**验证：** `go test ./cmd/mewcode -run 'Test.*(Text|JSON|Exit|Stopped)' -count=1` 通过。

## T4: 接入总超时与中断取消

**文件：** `cmd/mewcode/run.go`、`cmd/mewcode/main_test.go`

**依赖：** T3

**步骤：**

1. 解析时长参数，默认 30 分钟，零值表示不设置总 deadline。
2. 以统一根 Context 把总 deadline 与进程中断传递给 Agent。
3. 在终态汇总时区分 deadline 超时和用户取消，并返回对应退出码与 JSON 状态。
4. 使用可控 Context 或时钟替身编写超时、关闭超时和取消的离线测试。

**验证：** `go test ./cmd/mewcode -run 'Test.*(Timeout|Cancel)' -count=1` 通过。

## T5: 禁止非交互后台子 Agent

**文件：** `internal/agent/subagent.go`、`internal/agent/subagent_test.go`

**依赖：** T2

**步骤：**

1. 为 `SubAgentRuntime` 加入默认保持开启的后台许可设置。
2. 后台许可关闭时，拒绝显式后台和 Fork 子 Agent 请求，并返回结构化工具失败。
3. 禁止隔离前台子 Agent 的自动后台切换，改为等待其终态或接受父任务 Context 取消。
4. 覆盖交互式默认仍可后台、非交互显式后台拒绝、Fork 拒绝以及前台完成的测试。

**验证：** `go test ./internal/agent -run 'Test.*(SubAgent|Background|Fork)' -count=1` 通过。

## T6: 验证临时状态与权限自动拒绝

**文件：** `cmd/mewcode/main_test.go`

**依赖：** T2、T3、T5

**步骤：**

1. 用需要确认的副作用工具调用验证：非交互执行不会等待确认，且后续模型轮可收到拒绝工具结果。
2. 验证本次运行没有在会话目录创建可恢复会话，也没有触发长期记忆写入；记忆索引仍能作为只读系统模块加载。
3. 验证安全日志初始化仍被执行，且测试输出不含任务正文或工具结果正文。

**验证：** `go test ./cmd/mewcode -run 'Test.*(Permission|Temporary|Memory|Log)' -count=1` 通过。

## T7: 更新用户文档与章节记录

**文件：** `README.md`、`docs/ch00/08-noninteractive-run/{spec,plan,task,checklist}.md`

**依赖：** T3、T4、T5、T6

**步骤：**

1. 在 README 中增加 `mewcode run` 的示例、参数说明、默认超时、输出模式、退出码和“需确认即拒绝”的权限说明。
2. 对照实际实现更新本章 Spec、Plan、Task、Checklist 和人工场景中的可观察命令和完成状态，确保不描述未实现功能。
3. 不修改 `.mewcode/config.example.yaml`，因为本章不新增配置文件字段。

**验证：** `rg -n 'mewcode run|--prompt-file|--json|30' README.md docs/ch00/08-noninteractive-run` 能命中准确说明；`git diff --check` 通过。

## T8: 格式化、构建与全量回归

**文件：** 本章涉及的全部 Go 文件与文档

**依赖：** T4、T5、T6、T7

**步骤：**

1. 格式化修改的 Go 文件。
2. 运行命令包、子 Agent 包和全量测试。
3. 构建 CLI，并用本地测试替身完成至少一条非交互端到端调用，验证退出码与输出协议。
4. 将实际通过的命令、未验证事项和验收结果记录到 `checklist.md`；本任务不是 bug 修复，不新增 `bugs/` 记录。

**验证：** `gofmt -w cmd/mewcode/main.go cmd/mewcode/run.go internal/agent/subagent.go`、`go test ./cmd/mewcode ./internal/agent`、`go test ./...`、`go build ./cmd/mewcode` 和 `git diff --check` 全部通过。

## 执行顺序

```text
T1 → T2 → T3 → T4 ─┐
          └→ T5 ──┼→ T6 → T7 → T8
```
