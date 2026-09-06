# 自动加载的 `fork` Skill 未进入独立会话

- 状态：已修复，自动化验证通过
- 发现日期：2026-09-06
- 影响范围：第十一章由模型通过 `load_skill` 自动加载的所有 `mode: fork` Skill。

## 现象

模型在普通自然语言任务中调用 `load_skill("<fork-skill>")` 后，下一轮主会话直接收到该 Skill 的完整 SOP。不会创建独立 Session，不应用 `context: full`、`recent` 或 `none`，也不会只将最终摘要回流主会话。

显式 `/skill-name` 调用同一个 `fork` Skill 时会走独立会话路径，因此两个入口的执行语义不一致。

## 根因

`internal/skills/load_tool.go` 直接调用 `Manager.Activate`，而 `RuntimeFor` 对全部 activation 一律生成 `ActivePrompts`。`ModeFork` 只在 `agent.Runner.Start` 收到非空 `Request.Skill` 时分流至 `startFork`，自动加载工具既没有创建 `SkillInvocation`，也没有后续调度机制。

## 修复方案

将自动 fork 拆为两个系统工具：`load_skill` 对 inline Skill 保持激活语义；对 fork Skill 仅返回名称、说明、模式和需执行标记，不改动主会话 activation。新增 `run_skill(name, prompt)`，通过 Runner 注入的执行桥复用显式 fork 的临时会话逻辑；子会话按 `context` 构造历史，完整 SOP 仅在子会话中出现，最终摘要作为主 Agent 的 ToolResult 回流。每轮同时暴露两个系统工具，且 Provider 工具定义改为使用受限 Registry，避免白名单视图不一致。

## 验证进展

2026-09-06：静态核对确认 `Runner.Start` 只对显式 `Request.Skill` 判断 `ModeFork`；`load_skill` 仅激活 Skill；`RuntimeFor` 无条件注入所有已激活 SOP。现有 `TestRunnerForkSkillReturnsOnlyFinalSummaryToMainSession` 仅覆盖显式 invocation，尚无自动加载 `fork` Skill 的回归测试。

2026-09-06：设计核对表明不应让 `load_skill` 在普通工具执行器内直接调用现有 `Runner.startFork`：父 Runner 此时已处于 active 状态，且工具接口只能返回 JSON，无法安全承接嵌套任务的生命周期、用量和事件。改为将 SOP 激活与 fork 执行分为明确动作，并抽取共享 fork 执行器。

2026-09-06：新增 `TestRunnerAutoForkSkillKeepsSOPOutOfMainSession`，覆盖 `load_skill` → `run_skill`：主会话不含 fork SOP，`context: none`、`full`、`recent` 三种子会话历史范围正确，摘要和子会话 Token 用量回流。`go test ./internal/skills ./internal/tools ./internal/prompt ./internal/agent ./internal/command ./internal/tui ./cmd/mewcode -count=1`、`go test ./... -count=1`、`go build ./cmd/mewcode` 与 `git diff --check` 均通过；未执行真实 Provider 手工场景。

2026-09-06：项目级 `.mewcode/skills/isolated_review.md` 已更新为场景 G 可用的 `isolated-review`：`mode: fork`、`context: none`，工具白名单仅允许 `run_command`、`read_file`、`search_code`，SOP 明确禁止修改工作区。`go test ./internal/agent -run 'TestRunnerAutoForkSkillKeepsSOPOutOfMainSession|TestRunnerForkSkillReturnsOnlyFinalSummaryToMainSession' -count=1` 通过；尚未用真实 Provider 完成手工场景 G。
