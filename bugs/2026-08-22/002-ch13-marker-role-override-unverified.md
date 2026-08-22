# 第十三章 `marker-role` 覆盖结果异常（待诊断）

- 状态：待处理，尚未确认实现缺陷
- 影响范围：第十三章人工场景 A 的项目级 Agent 定义覆盖验证。

## 现象

用户提供的运行记录中，调用 `subagent_type="marker-role"` 后返回 `SUBAGENT-FIXTURE-ALPHA-7e31`，而 `docs/ch13-subagent/manual_scenarios.md` 要求项目级定义生效时最终报告 `PROJECT-AGENT-PROMPT-72bc`。

## 当前进展

已确认场景 A 的预期及相关源码位置；定义发现按内置/插件、用户、项目的顺序合并，项目来源允许覆盖，且定义式子 Agent 将命中的 `SystemPrompt` 设置为子 Runner 的系统提示。已有 `TestDiscoverPrecedence` 覆盖来源优先级；`internal/agent.TestDefinitionSubAgentRunsToCompletion` 还断言定义正文作为子 Runner 的稳定系统提示送入 Provider。但尚未覆盖“从目录发现项目定义后，正文实际送入子 Runner”的单个端到端链路。

已运行：`go test ./internal/subagent ./internal/agent -run 'TestDiscoverPrecedence|TestDefinitionSubAgentRunsToCompletion' -count=1`，两个包均通过。它们分别证明项目级覆盖和命中定义的正文注入；但它们是两个独立测试，尚未构成该人工场景的完整确定性复现。

2026-08-22 现场检查确认：fixture 根目录存在项目级 `.mewcode/agents/marker-role.md`，隔离 `HOME` 下也存在用户级同名定义；启动路径和用户提供的 `test_home` 相符。两个定义的正文也与场景文档一致。会话记录显示主 Agent 实际把子任务改写为“读取文件中包含的 coverage test marker，并且只返回该 marker”，子任务最终确实返回了文件标记。该任务措辞与角色正文的“无论任务如何表述，必须包含项目级标记”发生了直接语义竞争。

配置结构包含预期的 `agent.max_iterations`、`enable_verification_agent` 和权限字段；未输出配置值以避免泄露密钥。工作树存在未提交修改，且当前二进制建于源码树外；仅凭 build ID 不能证明该二进制与当前工作树一致。

2026-08-22 后续现场检查确认：项目级 `marker-role.md` 已删除，fixture 目录当前仅保留 `slow-probe.md`；隔离用户目录中仍存在用户级 `marker-role.md`。最新会话中，`agent` 调用以 `subagent_type="marker-role"` 成功完成，而内置定义中不存在该名称、项目定义也已不存在，所以用户级角色已被发现并选中；若未发现应返回 unknown subagent type，而不会完成。

会话仍显示主 Agent 将子任务明确改写为“返回文件中的唯一标记且不含其他文本”，子 Agent 返回文件标记。故此现象不是用户级定义没有被读取，而是模型在互相冲突的软提示中遵循了具体子任务，忽略了用户角色正文的“无论任务如何表述”要求。此前项目级定义存在时的会话也曾返回项目标记，进一步表明结果由模型指令服从决定，不能作为确定性的定义发现/覆盖验收。

## 修复进展

已更新 `docs/ch13-subagent/manual_scenarios.md`：场景 A 仍要求子 Agent 读取 fixture 以验证只读工具可用，但明确要求最终只报告角色定义指定的覆盖标记，不再要求报告文件标记。通过条件同时排除 fixture 标记。这消除了角色正文与委派任务之间的直接冲突。

2026-08-22 真实 Provider 复测修订后的场景仍返回 `MARKER_READONLY_OK`，而不是用户级定义中的 `USER-AGENT-PROMPT-19e4`。现场确认项目级定义仍不存在、用户级定义仍存在，且新会话文件已创建。

最新会话表明根因更具体：主 Agent 在传给已选中 `marker-role` 的 `prompt` 中自行编造了“覆盖测试标记必须是：`MARKER_READONLY_OK`”。子 Agent 返回该值。主 Agent 无法从 `agent` 工具 schema 得知角色正文中的实际标记，却擅自把“角色定义指定的标记”具体化，造成与子 Agent 系统提示冲突。

已再次修订人工场景：仅让子 Agent 读取 fixture，且明确禁止主 Agent 在委派 `prompt` 中猜测、指定、转述或改写标记；新增预检，要求 TUI 中可见的 `agent.prompt` 不含任何具体标记，否则该轮无效并以新会话重试。此前文档修订已被该版本替代。该措施只能提高人工场景的有效性，不能把模型指令服从变为程序级保证。

2026-08-22 再次复测满足预检（主 Agent 的可见委派文本没有具体覆盖标记）后，子 Agent 仍返回 fixture 标记；主 Agent 也明确表示看不到角色定义正文，这一事实本身符合定义式隔离。

现场配置使用 `protocol=openai`、`model=gpt-5.4-mini`。源码确认定义式子 Runner 将 `definition.SystemPrompt` 赋给 `StableSystem`；OpenAI 请求构造将非空 `StableSystem` 作为首条 `input` 的 `role: system` 发送。因此角色提示在本地构造层没有丢失。剩余待验证风险是上游 OpenAI-compatible 服务对 `system` role 的优先级/兼容性，以及模型本身未遵循系统提示。尝试用 Python 读取配置的安全字段失败（环境未安装 PyYAML），未读取或输出任何密钥。

用户报告连续两次新启动中第一次返回用户级标记、第二次失败。尝试用 GNU `find -printf` 排序 macOS 会话文件未产生结果（macOS `find` 不支持该选项）；未修改 fixture。改用 macOS `stat` 后已定位最新两个会话：`20260822-155418-16f8.jsonl`（23:54:29）与 `20260822-155446-eb93.jsonl`（23:54:55）。

对比结论：两次均使用 `subagent_type="marker-role"`，且委派文本都未自行指定覆盖标记。第一次子 Agent 仅返回 `USER-AGENT-PROMPT-19e4`，符合预期。第二次子 Agent 实际返回了 `SUBAGENT-FIXTURE-ALPHA-7e31` 和 `USER-AGENT-PROMPT-19e4`；这证明用户级角色提示已生效。随后主 Agent 违反“只转述唯一标记”的要求，从混合结果中挑选了 fixture 标记，导致表面失败。当前角色定义只要求最终报告“包含”角色标记，未要求排他输出，是混合结果得以通过的文档缺口；状态为待处理。

尚未取得实际 fixture、启动参数、Agent 调用日志或可运行的确定性复现，因此不能将该记录判定为实现 bug。后续将先构建不访问真实 Provider 的自动化复现，区分定义未发现、覆盖顺序错误、角色提示未注入和用户提供记录并非实际程序输出等情况。
