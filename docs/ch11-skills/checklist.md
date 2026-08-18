# MewCode Skill System Checklist

> 每项以可观察行为或实际命令验证。实施完成后在本文件记录命令、结果与必要的失败说明。

## Skill 发现与定义

- [ ] 项目级与用户级目录均被扫描，且项目级同名 Skill 覆盖用户级版本。（验证：在临时两级目录写入同名 Skill，启动或发现后检查轻量目录与正文来源。）
- [ ] 单文件 `.md` 和目录中的 `SKILL.md` 都可作为入口，目录附属资源不会自动进入初始 prompt。（验证：创建两种样例并断言目录资源文本未出现在首次请求。）
- [ ] 内置 `commit`、`review`、`test` 三个样板可被发现，且项目或用户级同名 Skill 可覆盖样板。（验证：查询 Catalog 与命令帮助。）
- [ ] 缺失 `name` 或 `description`、非法名称、重复标识、非法 `mode`/`context`、空正文和不存在的白名单工具均导致可定位的启动或刷新错误。（验证：逐个提供无效样例，断言错误含路径和字段或标识。）

## 两阶段加载与持续注入

- [ ] 首次请求仅包含每项 Skill 的名称和说明，既不包含 SOP 正文，也不包含样板附属资源。（验证：捕获首次 Provider 请求并比较系统 prompt。）
- [ ] `load_skill` 始终出现在模型可见工具中；调用后只返回安全元数据，不回显 SOP、调用参数或目录资源。（验证：检查工具定义和 ToolResult。）
- [ ] `load_skill` 成功后，下一轮请求包含被激活 Skill 的完整 SOP；同一任务后续每轮仍包含该 SOP。（验证：脚本 Provider 先调用工具再继续，比较连续请求。）
- [ ] 多项 Skill 可同时激活，SOP 按激活顺序稳定注入。（验证：激活两项并比较连续请求中的模块顺序。）
- [ ] `/skill-name 参数` 仅替换该 Skill SOP 中的 `{{args}}`；自然语言加载的 `{{args}}` 为空。（验证：比较显式调用与 `load_skill` 调用后的请求内容。）

## 工具白名单与模型选择

- [ ] 非空白名单只向模型和调度器提供列出的工具及 `load_skill`；未列工具既不出现在环境/定义中，也不能被执行。（验证：捕获工具定义并尝试未列工具调用。）
- [ ] 空白名单不额外收窄基础工具；多个已激活 Skill 的非空白名单取交集。（验证：分别激活空白名单、单项白名单和两项相交白名单。）
- [ ] 启动与刷新都拒绝引用不存在工具的白名单，且不替换此前有效 Catalog。（验证：先加载有效 Skill，再改为未知工具并执行 `/skills reload`。）
- [ ] Skill 指定模型时，Anthropic 和 OpenAI 请求均使用该模型；未指定时继续使用全局默认模型；多个冲突的指定模型给出诊断错误。（验证：检查两种请求编码及冲突激活结果。）

## 命令与刷新

- [ ] `/commit`、`/review`、`/test` 出现在 `/help` 和 Tab 补全中，并创建对应的 Skill Agent 请求。（验证：TUI 或 Command 测试检查帮助、补全和请求 Invocation。）
- [ ] `/skills reload` 成功后新增、修改、删除的 Skill 及命令一次性生效。（验证：变更项目级目录后刷新，检查命令表和轻量目录。）
- [ ] `/skills reload` 失败后旧 Catalog、旧命令表和已激活 Skill 全部仍可用。（验证：将有效文件改为无效文件后刷新，再执行旧命令并检查注入。）

## 执行模式与会话语义

- [ ] inline Skill 使用主会话历史，执行过程和结果按既有 Agent Loop 写入主历史。（验证：显式 inline 调用后检查主 Session history 和 display。）
- [ ] fork Skill 的临时执行不会把中间用户消息、工具调用、工具结果或回复写入主历史；完成后主历史只收到最终摘要。（验证：执行 fork Skill 后比较临时调用序列与主 Session history。）
- [ ] fork 的 `full`、`recent`、`none` 分别传入完整历史、最近五条消息和空历史；其 Provider Token 用量仍计入主会话。（验证：预置超过五条的会话并检查 fork 首次请求和会话 usage。）
- [ ] fork 取消或失败时不向主历史写入伪摘要，并保持现有取消/错误显示语义。（验证：脚本 Provider 返回错误或取消任务，检查主 Session。）
- [ ] 新建、恢复或 `/clear` 之后已激活 Skill 不再注入；发现的 Skill 命令仍可再次激活。（验证：激活 Skill 后切换会话，检查下一请求与 `/help`。）

## 安全、兼容性与回归

- [ ] Skill 发现、刷新、激活、模式分流、白名单过滤和清除均记录结构化生命周期日志，且日志中不含 SOP 正文、参数、工具结果、模型输出或密钥。（验证：写入可识别敏感占位文本后检查日志文件。）
- [ ] 无 Skill 时，普通对话、`/plan`、`/do`、`/compact`、权限确认、取消、会话持久化和 Token 统计保持既有行为。（验证：运行相关既有自动测试。）
- [ ] 所有本章自动化覆盖均落在既有 `_test.go` 文件；本轮未保留新建 `_test.go` 文件。（验证：检查 `git diff --name-status` 与 `git status --short`。）
- [ ] 格式化、目标包测试、完整测试和 CLI 构建均通过。（验证：运行 `gofmt -w`、`go test ./internal/skills ./internal/tools ./internal/prompt ./internal/provider/... ./internal/agent ./internal/command ./internal/tui ./cmd/mewcode`、`go test ./...`、`go build ./cmd/mewcode`。）

## 端到端场景

- [ ] 自动加载：启动后输入“帮我提交当前变更”，模型先看到轻量目录、调用 `load_skill("commit")`，下一轮按 commit SOP 继续，未激活 Skill 正文不出现。（验证：脚本 Provider 记录完整请求序列。）
- [ ] 显式调用与刷新：在项目级目录新增带 `{{args}}` 的 inline Skill，执行 `/skills reload` 后输入 `/skill-name 额外要求`；下一请求出现替换后的 SOP。将文件改为非法后再次刷新，旧命令和已激活 SOP 仍可用。（验证：TUI 集成测试或手工场景记录。）
- [ ] 独立审查：执行 `mode: fork, context: none` 的 review Skill；独立请求不包含主历史，主会话最终仅新增审查摘要，Token 用量增加。（验证：预置主会话并比较请求及 Session history。）
