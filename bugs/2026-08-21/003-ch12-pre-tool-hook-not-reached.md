# 第十二章工具前 Hook 未被模型工具调用流程触达

- 状态：待处理
- 发现日期：2026-08-21
- 影响范围：真实 Provider 下，模型应调用 `run_command` 且由 `pre_tool_use` Hook 拦截的流程。

## 现象

按第十二章场景 C 连续五次输入相同请求，均未产生预期的 Hook 拒绝工具结果。可见行为包括：

- 模型在工具调用前以“提示词/探测/任务无关/缺少代码上下文”等理由自行拒绝；
- 部分回复重复；
- 一次调用实际成功执行了 `printf HOOK-BLOCK-7c2a`；
- 一次在未运行 Slash Command 时显示 `HOOK-SLASH-COMMAND-42e8`。

预期行为是 `run_command` 被 `pre_tool_use` Hook 在权限确认前拒绝，工具结果包含 `HOOK-BLOCKED-REASON-7c2a`，随后模型仅解释该 Hook 拒绝原因。

## 当前进展

已确认现有 001 仅修复了主配置严格解析对 `hooks` 顶层字段的拒绝，尚不能证明真实工具执行路径会调用 Hook 引擎。本问题需要先建立不依赖真实模型选择工具的自动化反馈用例，再定位工具调度、Hook 拦截结果和 prompt 通知事件的接入点。

2026-08-21 文档与测试索引检查：第十二章 Spec 的 F5/AC3、Plan 的 Scheduler 接入点和 Checklist 都明确要求此行为；代码库已有 `TestSchedulerHookRejectDoesNotExecute`。下一步将先运行该测试确认它是否能覆盖并复现本问题，再检查其与真实 Agent Loop 的差异。

2026-08-21：已运行 `go test ./internal/agent -run '^TestSchedulerHookRejectDoesNotExecute$' -count=1`，结果通过（约 0.6 秒）。该测试只能证明 Scheduler 被直接调用时会拦截，不覆盖真实 Provider 的工具选择、工具定义提示或 Hook 通知的可见性，因此不是本问题的红色复现用例。

2026-08-21 代码检查：`Scheduler.executePermitted` 在真实工具执行前调用 `RunPreTool`，拒绝时可生成 Hook 失败工具结果，因此“模型未发起调用”发生在该路径之前、由真实模型的工具选择决定。另发现 `Scheduler.executeAndNotify` 将每次成功的 `run_command` 错误映射为 `command_execute`，会导致 `HOOK-SLASH-COMMAND-42e8` 在未执行 Slash Command 时出现；该事件应由 Slash Command 分流处触发。

2026-08-21 条件解析检查：场景 C 的原条件可被解析为两个有效的 AND 条件，但其字段语义错误：`args.command` 是可执行文件名 `printf`，标记实际位于参数数组 `args.args`。Scheduler 为该工具构造的上下文包含这两个 JSON 参数；人工配置应匹配 `args.args =~ /HOOK-BLOCK-7c2a/`。

2026-08-21 事件来源检查：`internal/command.Dispatch` 已在 Slash Command 分流处触发 `command_execute`，并有对应单元测试；Scheduler 额外在成功的 `run_command` 后触发同一事件，构成确认的重复且语义错误。此前已修复的活跃任务工具输出重复渲染问题不能解释本次重复模型文本，后者仍缺少可复现证据。

2026-08-21：已新增 `TestSchedulerRunCommandDoesNotEmitSlashCommandHook` 作为确定性回归用例。首次运行因受限环境无法写入 Go 构建缓存而未能执行；获准后重试，测试稳定失败：成功的 `run_command` 使仅监听 `command_execute` 的 prompt Hook 收到 `slash notification`。该结果直接复现了场景 C 中意外出现的 `HOOK-SLASH-COMMAND-42e8`。

## 修复方案

移除 Scheduler 对成功 `run_command` 的 `command_execute` 事件派发；该事件只由 `internal/command.Dispatch` 的 Slash Command 分流产生。新增回归测试，断言普通命令工具调用不会触发仅监听 Slash Command 事件的 Hook。

## 验证进展

2026-08-21：应用最小修复并运行 `gofmt` 后，`go test ./internal/agent -run '^TestSchedulerRunCommandDoesNotEmitSlashCommandHook$' -count=1` 通过（约 0.6 秒）。仍需运行 Hook、Command 与 Agent 相关测试，并补充真实 Provider 场景 C 的人工结论。

2026-08-21：新增完整 Agent 链路测试首次编译失败，原因是测试断言误把会话快照消息当作含 `Results` 字段的记录；产品代码未执行。测试将改为检查第二次 Provider 请求中写回的 `ToolResult` 内容后重试。

2026-08-21：修正测试断言后，完整链路稳定复现了命令被放行。根因是人工场景的条件错误地匹配 `args.command =~ /HOOK-BLOCK-7c2a/`；实际调用参数为 `command: "printf"` 与 `args: ["HOOK-BLOCK-7c2a"]`，标记位于 `args.args`。这是场景 C 一次成功执行命令的直接原因；引擎与 Scheduler 的工具前调用顺序未失效。

2026-08-21：将人工条件修正为 `args.args =~ /HOOK-BLOCK-7c2a/` 后，以下定向回归均通过：`TestRunnerRejectsRunCommandWithPreToolHook`、`TestSchedulerHookRejectDoesNotExecute`、`TestSchedulerRunCommandDoesNotEmitSlashCommandHook`，以及 `go test ./internal/hooks ./internal/command -count=1`。前两项覆盖完整 Agent 回灌链和 Scheduler 零执行，后一项覆盖 Slash Command 事件不再由普通命令工具触发。

2026-08-21：`go test ./... -count=1` 与 `go build ./cmd/mewcode` 均通过。构建/测试发现一个本轮生成的未跟踪 `cmd/mewcode/.mewcode/` 目录；检查后确认其中仅含本轮测试生成的会话 JSONL，将按项目规范清理。

2026-08-21：已清理该本轮生成的 `cmd/mewcode/.mewcode/` 会话目录，未触及其他文件。

## 当前状态

已修复并自动化验证通过的部分：场景条件字段与错误的 Slash Command 事件来源。2026-08-21 后续人工复测仍显示模型在工具调用前以“与当前任务无关的命令探测”为由拒绝，未触达 Hook。

代码检索定位到默认工具提示含“避免与当前任务无关”的命令规则，和场景 C 的显式安全测试命令直接冲突。该规则是当前最可能的模型拒绝诱因；需要通过调整提示词边界并用请求快照测试锁定，不应再把该表现仅记录为模型随机未调用。

2026-08-21 请求构造检查：每轮请求均包含增强后的全部工具定义，`run_command` 未被从普通执行模式的可见工具集中移除；当前证据不支持“工具未提供给 Provider”。`run_command` 专属说明确实要求“避免与当前任务无关”，与用户给出的模型拒绝理由逐字同源。

2026-08-21：新增 `TestEnhanceDefinitionsAndToolRulesStable` 断言，要求 `run_command` 的工具说明把用户显式、安全命令交由 Hook、权限和确认机制裁决。测试稳定失败，输出确认当前说明仍要求避免“与当前任务无关”的命令；该失败为本次模型调用前拒绝的可重复提示词证据。

2026-08-21：已将 `run_command` 提示词改为：用户明确请求、工作区内的安全命令应发起调用并由 Hook、权限和确认机制裁决，不得仅因任务关联性跳过；同时把场景 C 的会话通知改为纯上下文通知，移除“回答前确认”的指令。`go test ./internal/prompt ./internal/agent -run '^(TestEnhanceDefinitionsAndToolRulesStable|TestRunnerRejectsRunCommandWithPreToolHook)$' -count=1` 通过。

2026-08-21：`go test ./... -count=1` 与 `go build ./cmd/mewcode` 均通过。测试再次生成 `cmd/mewcode/.mewcode/` 会话目录，检查后确认仅含本轮测试会话 JSONL，将清理；同时发现未跟踪的 `bugs/2026-08-21/004-...`，该文件不属于本次场景 C 改动，将保留不动。

2026-08-21：已清理本轮生成的 `cmd/mewcode/.mewcode/`，未修改未跟踪的 004 记录。

## 建议下一步

构造覆盖 `run_command` 的最小执行链测试：断言拒绝规则阻止执行器、返回规则动作输出，并验证 `command_execute` 提示只在 Slash Command 事件后注入。
