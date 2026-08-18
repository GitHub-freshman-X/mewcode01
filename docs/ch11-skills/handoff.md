# 第十一章 Skill System 交接

## 当前状态

- 当前分支：`codex/ch11-skills`，从 `main` 创建。
- 第十一章尚未开始实现；仅新增了本章文档。
- `spec.md`、`plan.md`、`task.md` 已获用户明确确认。
- `checklist.md` 已生成，但尚未获用户审批。根据 `mew-spec` 的硬性门禁，在用户明确确认 Checklist 前不得编写实现代码。
- 工作区目前的未跟踪变更仅为 `docs/ch11-skills/`。

## 下一步

1. 请求用户审批 [checklist.md](checklist.md)。
2. 用户明确批准后，宣告四份文档已审批，并严格按 [task.md](task.md) 的 T1 至 T13 实施。
3. 每个任务完成后运行其中列明的验证；若发现或修复 bug，同步按项目规则检查并更新 `bugs/`。
4. 全部实现后按 [checklist.md](checklist.md) 逐项记录实际证据，并完成 [manual_scenarios.md](manual_scenarios.md)（该文件尚未创建）的人工场景。

## 已确认的产品决策

- 热更新采用手动方式：`/skills reload`；不实现文件监听或自动刷新。
- Skill 目录为项目级 `.mewcode/skills/` 与用户级 `os.UserConfigDir()/mewcode/skills/`；项目级覆盖用户级。
- 启动仅注入名称和说明；完整 SOP 仅通过系统级 `load_skill` 按需激活。
- 激活 SOP 每轮持续注入，可同时激活多个 Skill。
- 前端参数占位符固定为 `{{args}}`；自然语言自动加载时替换为空。
- 支持 inline 与 fork；fork 仅将最终摘要回流到主会话，历史范围是 `full`、`recent`、`none`。
- 多个非空工具白名单取交集，`load_skill` 不受白名单限制；刷新失败必须保留旧 Catalog、命令与激活状态。
- 内置样板为 `commit`、`review`、`test`。
- 本章不实现市场、分发、版本管理、远程安装或后台文件监听。

## 正式文档与参考

- [Spec](spec.md)：已批准的范围、功能需求和验收标准。
- [Plan](plan.md)：已批准的模块、接口、数据流、文件组织和技术决策。
- [Tasks](task.md)：已批准的 T1–T13 实施顺序和验证命令。
- [Checklist](checklist.md)：待用户审批的可观察验收清单。
- [理论学习：Skill 可复用技能包](理论学习：Skill%20可复用技能包.md)：从飞书完整复制的章节学习材料。
- [第十章 Spec](../ch10-slash_command/spec.md)：现有 Slash Command 行为与兼容边界。

## 实现注意事项

- 当前已有 `prompt.OptionalModules.ActiveSkills`，但未实际接入 Skill；设计要求另增“可用 Skill”轻量目录模块，并让完整 SOP 保持在最显眼的可选模块位置。
- 现有 `tools.Registry` 没有白名单视图；实现必须令环境、Provider 工具定义、调度器及执行器使用同一受限 Registry。
- 现有 Provider 模型由客户端固定；本章计划在 `provider.ChatRequest` 增加请求级模型覆盖，并令 Anthropic/OpenAI 只在请求模型非空时覆盖默认模型。
- 现有 Agent Loop 没有通用 fork；按 Plan 以无 journal 的临时 `conversation.Session` 执行，并将最终摘要写入主会话。中间过程不得污染主历史。
- 已生效项目规则：本轮新增的任何 `_test.go` 文件在完成前必须删除。因此实施测试应优先扩展既有测试文件；最终检查不能留下本轮新建的 `_test.go` 文件。
- 本章改变用户可见命令和运行方式时，需更新根 `README.md`、`.mewcode/config.example.yaml` 及 `docs/README.md`。
- 对本章关键生命周期接入 `internal/logging`，只记录安全元数据，不记录 SOP、参数、prompt、工具结果正文、密钥或未经脱敏错误载荷。

## Suggested skills

- `mew-spec`：继续执行已批准的规格驱动流程，尤其是 Checklist 审批后按任务实施与验收。
- `diagnosing-bugs`：仅当实施或验证出现难以定位的失败、异常或性能回归时使用。
- `lark-doc`：仅当需要再次核对或更新飞书原始学习材料时使用。
