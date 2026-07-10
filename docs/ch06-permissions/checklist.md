# MewCode Permission System Checklist

> 每一项通过运行代码或观察行为来验证，聚焦系统行为。

## 实现完整性

- [ ] **AC1 统一权限门禁生效**：工具调用在真实执行前经过权限判断；被拒绝的调用不执行真实工具动作，并以结构化权限失败结果写回模型。（验证：运行 `go test ./internal/agent ./internal/permissions -run 'Permission|DeniedToolResult|Runner'`，期望工具执行次数为 0、工具结果 `IsError=true` 且 Agent 继续下一轮）
- [ ] **AC2 危险黑名单不可绕过**：危险命令命中黑名单时，即使存在 allow 规则、放行模式或确认允许，也不会启动真实命令。（验证：运行 `go test ./internal/permissions ./internal/agent -run 'Blacklist|DangerousCommand'`，期望决策阶段为 blacklist 且命令执行次数为 0）
- [ ] **AC3 真实路径沙箱防逃逸**：相对路径、绝对路径和指向项目外的符号链接访问项目外文件时均被拒绝，项目内真实路径继续进入规则判断。（验证：运行 `go test ./internal/permissions -run 'Sandbox|Resolve|Symlink'`，期望逃逸路径返回权限错误、项目内路径返回相对路径）
- [ ] **AC4 规则格式和匹配正确**：`run_command(git *)`、`read_file(docs/**)` 和精确文件规则能命中对应调用，不会误命中相似字符串。（验证：运行 `go test ./internal/permissions -run 'ParseRule|MatchRule|BuildRequest'`，期望精确和 glob 断言通过）
- [ ] **AC5 规则优先级正确**：同一调用被多层规则覆盖时，会话级高于本地级，本地级高于项目级，项目级高于用户级，显式规则高于权限模式。（验证：运行 `go test ./internal/permissions -run 'RuleStore|Priority|ExplicitRule'`，期望每层移除后决策按优先级回落）
- [ ] **AC6 权限模式默认行为正确**：无规则时，严格模式所有工具询问，默认模式只读允许且副作用询问，放行模式普通调用允许但仍不能绕过黑名单和沙箱。（验证：运行 `go test ./internal/permissions -run 'Mode|DefaultDecision|Relaxed'`，期望三种模式断言通过）
- [ ] **AC8 永久允许写入本地规则**：用户选择永久允许后，本地级规则文件新增 allow 规则；重新加载后相同调用命中本地级规则，用户级和项目级文件不变。（验证：运行 `go test ./internal/permissions -run 'AppendLocalAllow|LoadRuleSet'`，期望本地文件内容和重载决策正确）
- [ ] **AC10 配置错误不宽松运行**：非法 YAML、未知结果值、非法规则键或非法 glob 会产生可诊断错误，系统不以更宽松权限继续运行。（验证：运行 `go test ./internal/permissions ./cmd/mewcode -run 'Invalid|Config|Rule'`，期望错误包含配置路径或规则键）
- [ ] **AC13 新工具可接入权限判断**：新增测试工具通过权限元数据声明匹配对象和路径参数后，无需改动 Agent Loop 即可参与权限判断。（验证：运行 `go test ./internal/permissions ./internal/agent -run 'CustomTool|PermissionMetadata'`，期望测试工具按声明生成权限请求）

## Agent 集成

- [ ] **AC1 权限拒绝不终止 Agent Loop**：脚本化 Provider 先请求被拒绝工具，再根据权限失败结果返回最终文本，任务以 completed 结束。（验证：运行 `go test ./internal/agent -run 'PermissionDeniedContinues|Runner'`，期望终态为 completed 且历史包含权限失败工具结果）
- [ ] **AC7 本次允许只影响当前调用**：需要确认的调用选择本次允许后当前工具执行，下一次相同调用仍会再次请求确认。（验证：运行 `go test ./internal/agent -run 'AllowOnce|Permission'`，期望两次确认请求均出现）
- [ ] **AC7 本会话允许生成临时规则**：需要确认的调用选择本会话允许后当前工具执行，会话内相同调用不再询问。（验证：运行 `go test ./internal/agent ./internal/permissions -run 'AllowSession|SessionRule'`，期望第二次调用直接 allow）
- [ ] **AC7 拒绝确认返回权限失败**：用户选择拒绝时，真实工具不执行，模型收到权限失败工具结果，Agent 可继续下一轮。（验证：运行 `go test ./internal/agent -run 'ConfirmDeny|Permission'`，期望工具执行次数为 0 且写回结果顺序正确）
- [ ] **AC7/AC15 取消确认取消任务**：Agent 等待确认时取消任务，不启动当前工具或剩余工具，并发出取消终态。（验证：运行 `go test ./internal/agent -run 'ConfirmCancel|Cancelled'`，期望终态为 cancelled 且无后续工具执行）
- [ ] **AC9 多工具结果顺序稳定**：一次响应包含允许读、拒绝写、允许读时，允许工具按原调度执行，拒绝工具生成失败结果，三个结果按模型原始调用顺序写回。（验证：运行 `go test ./internal/agent -run 'Scheduler|PermissionOrder|MultiTool'`，期望结果顺序与调用顺序一致）
- [ ] **AC11 权限事件可观察**：allow、deny、ask、确认允许和确认拒绝都会产生可观察的权限事件，事件包含工具名、决策阶段和安全摘要。（验证：运行 `go test ./internal/agent -run 'PermissionEvent|Decision|Response'`，期望事件序列和字段断言通过）
- [ ] **AC12 Plan/Do 兼容**：Plan Mode 仍只包含只读工具且只读调用仍经过权限系统；Do Mode 恢复全部工具后仍受权限约束。（验证：运行 `go test ./internal/agent -run 'Plan|Do|Permission'`，期望工具集合、权限决策和计划消费边界符合预期）

## TUI 集成

- [ ] **AC7/AC11 确认请求展示完整**：TUI 收到权限确认请求后展示工具名、匹配对象、原因和可选操作。（验证：运行 `go test ./internal/tui -run 'PermissionView|ConfirmPrompt'`，期望视图文本包含工具名、match target 和操作提示）
- [ ] **AC7 本次/本会话/永久允许按键有效**：确认状态下选择本次、本会话、永久允许会向 Agent 返回对应 choice。（验证：运行 `go test ./internal/tui -run 'AllowOnce|AllowSession|AllowPermanent'`，期望 bridge 收到正确 choice）
- [ ] **AC7 拒绝和取消按键有效**：确认状态下拒绝返回 deny，Ctrl+C 取消当前 Agent 任务且不提交普通输入。（验证：运行 `go test ./internal/tui -run 'PermissionDeny|PermissionCancel'`，期望任务取消或 deny choice 被发送）
- [ ] **确认状态不干扰普通输入**：不存在权限确认时，普通输入、`/plan`、`/do` 的交互行为保持原样。（验证：运行 `go test ./internal/tui -run 'Submit|Plan|Do'`，期望既有 TUI 行为测试通过）

## 配置与文档

- [ ] **AC6/AC14 旧配置兼容**：不包含 `permissions` 的旧配置文件可以加载，并默认使用 `default` 权限模式。（验证：运行 `go test ./internal/config -run 'Default|Permission|Load'`，期望默认模式为 default）
- [ ] **AC10 主程序暴露配置错误**：存在非法权限规则文件时，主程序启动失败并向 stderr 输出可诊断错误；规则文件缺失时正常启动。（验证：运行 `go test ./cmd/mewcode -run 'PermissionConfig|InvalidRule|MissingRule'`，期望错误和成功分支通过）
- [ ] **配置示例包含权限模式**：示例配置包含 `permissions.mode: default`，且不改变现有 Provider 配置含义。（验证：运行 `rg -n 'permissions:|mode: default' config.example.yaml`，期望两项均匹配）
- [ ] **文档索引包含 ch06**：`docs/README.md` 登记第六章权限系统文档链接。（验证：运行 `rg -n 'ch06|权限|Permission' docs/README.md`，期望匹配到第六章索引）
- [ ] **本地规则不进入版本库**：`.gitignore` 忽略 `.mewcode/permissions.local.yaml`，且没有覆盖用户原有忽略规则。（验证：运行 `git diff -- .gitignore`，期望仅新增本地权限规则忽略项和用户已有改动共存）

## 编译与测试

- [ ] **权限包离线测试通过**：规则解析、路径沙箱、黑名单、规则加载、请求构建、决策引擎和拒绝结果均可离线测试。（验证：运行 `go test ./internal/permissions`，期望通过）
- [ ] **工具包回归通过**：六个核心工具权限元数据齐全，原有读写改查搜和命令行为不变。（验证：运行 `go test ./internal/tools`，期望通过）
- [ ] **Agent/TUI 回归通过**：权限接入未破坏 Agent Loop、调度、Plan/Do、取消和 TUI 展示。（验证：运行 `go test ./internal/agent ./internal/tui`，期望通过）
- [ ] **配置和主程序测试通过**：配置加载、权限模式校验、规则文件组装和启动错误处理通过。（验证：运行 `go test ./internal/config ./cmd/mewcode`，期望通过）
- [ ] **全项目测试通过**：第 2 章到第 6 章所有自动化测试通过。（验证：运行 `go test ./...`，期望通过）

## 端到端场景

- [ ] **E2E 1：危险命令硬拦截**：在放行模式且存在 `run_command(*)=allow` 的规则下，模型请求危险命令，系统仍拒绝并回灌权限失败，模型随后改用安全方案完成任务。（验证：运行脚本化 Agent 端到端测试，期望命令未启动、终态 completed、最终文本说明已调整策略）
- [ ] **E2E 2：路径沙箱阻止符号链接逃逸**：工作区内存在指向项目外的符号链接，模型请求读取该链接路径，系统拒绝读取并让模型改读项目内合法文件。（验证：运行脚本化 Agent 端到端测试，期望项目外文件未读取、权限失败结果写回、后续合法读取成功）
- [ ] **E2E 3：人在回路本会话允许**：默认模式下模型请求写文件，TUI/脚本确认器选择本会话允许；第一次写入触发确认，第二次相同写入不再询问。（验证：运行 Agent/TUI 集成测试，期望确认请求计数为 1、两次写入均成功）
- [ ] **E2E 4：永久允许跨重载生效**：默认模式下模型请求一个未命中的副作用调用，用户选择永久允许；重建权限引擎后相同调用由本地级规则直接允许。（验证：运行权限配置集成测试，期望本地规则文件存在且重载后决策来源为 local）
- [ ] **E2E 5：Plan/Do 权限边界**：先执行 `/plan` 读取项目生成计划，再执行 `/do` 写文件；规划阶段只读工具仍受权限规则影响，执行阶段写工具按默认模式请求确认或按规则执行。（验证：运行脚本化 Provider 端到端测试，期望 Plan 历史隔离、Do 工具集合完整、权限事件和计划消费正确）

