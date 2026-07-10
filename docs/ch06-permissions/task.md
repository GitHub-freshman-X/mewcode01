# MewCode Permission System Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/permissions/parser.go` | 规则键解析、effect 校验、精确/glob 匹配 |
| 新建 | `internal/permissions/parser_test.go` | 规则解析与匹配测试 |
| 新建 | `internal/permissions/path.go` | 项目真实路径沙箱 |
| 新建 | `internal/permissions/path_test.go` | 相对路径、绝对路径、符号链接逃逸测试 |
| 新建 | `internal/permissions/blacklist.go` | 危险命令正则黑名单 |
| 新建 | `internal/permissions/blacklist_test.go` | 黑名单命中与误伤测试 |
| 新建 | `internal/permissions/config.go` | 三层规则 YAML 加载与本地规则写入 |
| 新建 | `internal/permissions/config_test.go` | 规则文件加载、优先级、非法配置测试 |
| 新建 | `internal/permissions/request.go` | 工具调用参数转权限请求 |
| 新建 | `internal/permissions/request_test.go` | 命令、文件、搜索匹配对象构造测试 |
| 新建 | `internal/permissions/engine.go` | 权限决策主流程、模式默认行为、确认应用 |
| 新建 | `internal/permissions/engine_test.go` | 黑名单、沙箱、规则层级、模式和确认测试 |
| 新建 | `internal/permissions/result.go` | 权限拒绝转工具结果 |
| 新建 | `internal/permissions/result_test.go` | permission_error 工具结果测试 |
| 修改 | `internal/tools/tool.go` | Metadata 增加权限元数据类型 |
| 修改 | `internal/tools/read_file.go` | 声明路径匹配对象和路径参数 |
| 修改 | `internal/tools/write_file.go` | 声明路径匹配对象和路径参数 |
| 修改 | `internal/tools/edit_file.go` | 声明路径匹配对象和路径参数 |
| 修改 | `internal/tools/run_command.go` | 声明命令匹配对象 |
| 修改 | `internal/tools/find_files.go` | 声明模式匹配对象 |
| 修改 | `internal/tools/search_code.go` | 声明模式匹配对象 |
| 修改 | `internal/tools/tools_test.go` | 工具权限元数据与旧行为回归 |
| 修改 | `internal/agent/event.go` | 新增权限事件和事件字段 |
| 修改 | `internal/agent/mode.go` | Options 增加权限引擎与确认桥 |
| 修改 | `internal/agent/scheduler.go` | 工具执行前权限判定、确认、拒绝结果 |
| 修改 | `internal/agent/scheduler_test.go` | 调度顺序、权限拒绝、确认取消测试 |
| 修改 | `internal/agent/runner.go` | 创建带权限门禁的 Scheduler |
| 修改 | `internal/agent/runner_test.go` | Agent Loop 权限拒绝继续与 Plan/Do 兼容 |
| 修改 | `internal/config/config.go` | PermissionConfig |
| 修改 | `internal/config/load.go` | 权限模式默认值加载 |
| 修改 | `internal/config/validate.go` | 权限模式校验 |
| 修改 | `internal/config/config_test.go` | 旧配置兼容和权限配置测试 |
| 修改 | `cmd/mewcode/main.go` | 组装权限引擎、规则路径、TUI 确认桥 |
| 修改 | `cmd/mewcode/main_test.go` | 启动组装与配置错误测试 |
| 新建 | `internal/tui/permissions.go` | TUI PermissionBridge |
| 修改 | `internal/tui/model.go` | pending permission 状态 |
| 修改 | `internal/tui/update.go` | 确认按键处理和任务取消 |
| 修改 | `internal/tui/view.go` | 权限确认提示与状态展示 |
| 修改 | `internal/tui/tui_test.go` | 权限确认交互测试 |
| 修改 | `config.example.yaml` | 示例 `permissions.mode` |
| 修改 | `docs/README.md` | ch06 文档索引 |
| 修改 | `.gitignore` | 忽略 `.mewcode/permissions.local.yaml` |

## T1: 建立权限规则解析与匹配

**文件：** `internal/permissions/parser.go`、`internal/permissions/parser_test.go`
**依赖：** 无
**步骤：**
1. 定义 `Effect`、`Scope`、`Rule`、`Action`、`Stage` 的基础类型和值。
2. 实现 `ParseRule(key string, effect Effect, scope Scope, index int)`，解析 `工具名(模式)`。
3. 校验工具名、括号结构、空模式、effect 只能为 `allow` 或 `deny`。
4. 实现 `IsGlob`，识别 `*`、`?`、`[` 等 glob 元字符。
5. 实现 `MatchRule`，无 glob 元字符时精确匹配，有 glob 元字符时按 glob 匹配。
6. 添加测试覆盖合法规则、非法规则、精确匹配、glob 匹配和不匹配场景。

**验证：** `go test ./internal/permissions -run 'ParseRule|MatchRule|Glob'` 通过。

## T2: 实现真实路径沙箱

**文件：** `internal/permissions/path.go`、`internal/permissions/path_test.go`
**依赖：** T1
**步骤：**
1. 定义 `Sandbox` 和 `PathCheck`。
2. 实现 `NewSandbox(root string)`，保存清理后的项目根和项目根真实路径。
3. 实现 `Resolve(raw, parameter string)`，把相对路径按项目根展开，绝对路径直接处理。
4. 对已存在路径使用真实路径；对待创建路径解析已存在父目录的真实路径，再拼接不存在尾部。
5. 用真实项目根做路径边界判断，返回项目内相对路径。
6. 添加测试覆盖项目内文件、`..` 逃逸、绝对路径逃逸、指向项目外的符号链接、符号链接父目录写入逃逸。

**验证：** `go test ./internal/permissions -run 'Sandbox|Resolve|Symlink'` 通过。

## T3: 实现危险命令黑名单

**文件：** `internal/permissions/blacklist.go`、`internal/permissions/blacklist_test.go`
**依赖：** T1
**步骤：**
1. 定义 `BlacklistMatch`。
2. 建立危险命令正则表，覆盖破坏性删除、磁盘格式化、权限破坏、递归系统路径修改、大规模进程终止和 fork bomb。
3. 实现 `CheckCommandBlacklist(commandText string)`，返回匹配原因或 nil。
4. 确保黑名单不读取用户配置，也不暴露关闭开关。
5. 添加测试覆盖应拦截命令和常见安全命令，如 `git status`、`go test ./...`。

**验证：** `go test ./internal/permissions -run 'Blacklist|DangerousCommand'` 通过。

## T4: 实现三层规则文件加载与本地写入

**文件：** `internal/permissions/config.go`、`internal/permissions/config_test.go`
**依赖：** T1
**步骤：**
1. 定义 `FilePaths`、`RuleSet`、`RuleStore` 和规则文件 YAML 结构。
2. 实现 `DefaultFilePaths(workspace string)`，返回用户级、项目级、本地级规则路径。
3. 实现 `LoadRuleSet(paths FilePaths)`，缺失文件视为空，存在文件必须严格解析。
4. 校验未知字段、非法 YAML、非法 effect、非法规则键和非法 glob。
5. 实现并发安全的 `RuleStore`，按 Session、Local、Project、User 顺序查找。
6. 实现 `AppendLocalAllow`，创建父目录并把 allow 规则追加到本地级 YAML。
7. 添加测试覆盖缺失文件、三层加载、非法配置报错、永久 allow 写入。

**验证：** `go test ./internal/permissions -run 'RuleSet|RuleStore|LocalAllow|Config'` 通过。

## T5: 扩展工具权限元数据

**文件：** `internal/tools/tool.go`、`internal/tools/read_file.go`、`internal/tools/write_file.go`、`internal/tools/edit_file.go`、`internal/tools/run_command.go`、`internal/tools/find_files.go`、`internal/tools/search_code.go`、`internal/tools/tools_test.go`
**依赖：** T1
**步骤：**
1. 在 `tools.Metadata` 增加 `Permission PermissionMetadata`。
2. 定义 `PermissionTarget`，支持 path、command、pattern、none。
3. 定义 `PermissionMetadata`，包含 `Target` 和 `PathParams`。
4. 为 `read_file`、`write_file`、`edit_file` 声明 path target 与 `path` 参数。
5. 为 `run_command` 声明 command target。
6. 为 `find_files`、`search_code` 声明 pattern target。
7. 保持现有工具 Schema、名称、描述、Safety 和执行行为不变。
8. 添加测试确认六个核心工具权限元数据齐全，并跑旧工具行为测试。

**验证：** `go test ./internal/tools -run 'PermissionMetadata|ReadFile|WriteFile|EditFile|RunCommand|FindFiles|SearchCode'` 通过。

## T6: 构建权限请求

**文件：** `internal/permissions/request.go`、`internal/permissions/request_test.go`
**依赖：** T2、T5
**步骤：**
1. 定义 `Request`，包含 call id、工具名、原始参数、安全分类、匹配对象和路径检查结果。
2. 实现 `BuildRequest(call provider.ToolCall, tool tools.Tool, sandbox Sandbox)`。
3. 解析工具参数 JSON，只提取权限判断需要的字段。
4. 对 path target 生成项目相对路径作为 `MatchTarget`，并填充 `Paths`。
5. 对 command target 将 `command` 和 `args` 规范化为一行命令文本。
6. 对 pattern target 使用 `pattern` 参数作为 `MatchTarget`。
7. 实现 `SuggestedRuleKey`，用于确认后生成规则。
8. 添加测试覆盖命令 quoting、路径相对化、查找/搜索 pattern、非法参数 JSON。

**验证：** `go test ./internal/permissions -run 'BuildRequest|CommandText|SuggestedRuleKey'` 通过。

## T7: 实现权限决策引擎

**文件：** `internal/permissions/engine.go`、`internal/permissions/engine_test.go`
**依赖：** T3、T4、T6
**步骤：**
1. 定义 `Mode`、`Decision`、`Confirmation`、`Choice`。
2. 实现 `Engine.Decide`，按黑名单、沙箱、会话、本地、项目、用户、模式默认的顺序决策。
3. 实现严格、默认、放行三种模式的默认行为。
4. 确保显式 deny 不被放行模式覆盖，显式 allow 可在严格模式下放行。
5. 实现 `ApplyConfirmation`，处理本会话 allow 和永久 allow；本次 allow 不写规则。
6. 添加测试覆盖黑名单不可绕过、符号链接沙箱、四层规则优先级、三种权限模式、确认应用。

**验证：** `go test ./internal/permissions -run 'Engine|Decide|Mode|Confirmation'` 通过。

## T8: 实现权限拒绝工具结果

**文件：** `internal/permissions/result.go`、`internal/permissions/result_test.go`
**依赖：** T7
**步骤：**
1. 实现 `DeniedToolResult(call provider.ToolCall, decision Decision)`。
2. 使用现有工具结果 JSON 结构，错误类型为 `permission_error`。
3. 在 details 中写入决策阶段、工具名、规则来源或安全摘要，避免写入敏感绝对路径。
4. 确保 `provider.ToolResult.IsError=true`，call id 和工具名保持原始调用值。
5. 添加测试解析 JSON 并验证字段。

**验证：** `go test ./internal/permissions -run 'DeniedToolResult|PermissionError'` 通过。

## T9: 接入 Agent 权限事件与选项

**文件：** `internal/agent/event.go`、`internal/agent/mode.go`
**依赖：** T7、T8
**步骤：**
1. 定义 `PermissionBridge` 接口。
2. 在 `Options` 增加 `Permissions *permissions.Engine` 和 `Confirmer PermissionBridge`。
3. 在 `EventType` 增加 `permission_decision`、`permission_request`、`permission_response`。
4. 在 `Event` 增加权限决策和确认字段。
5. 更新默认 options，允许权限系统为 nil；nil 时使用兼容模式直接执行。
6. 添加编译测试或现有 agent 测试确认不配置权限时行为不变。

**验证：** `go test ./internal/agent -run 'Options|Event|Scheduler'` 编译通过。

## T10: 在 Scheduler 执行前接入权限门禁

**文件：** `internal/agent/scheduler.go`、`internal/agent/scheduler_test.go`
**依赖：** T8、T9
**步骤：**
1. 修改 `NewScheduler`，接收权限引擎和确认桥，同时保留 nil 权限兼容路径。
2. 在每个工具调用执行前调用 `Engine.Decide`。
3. allow 时执行原工具；deny 时生成权限失败工具结果；ask 时发出确认请求并调用确认桥。
4. 根据确认结果执行、写入会话规则、写入本地规则、拒绝或取消。
5. 保持结果按模型原始工具调用顺序写回。
6. 保持只读并发和副作用串行批次语义；需要确认的调用不并发弹多个确认。
7. 添加测试覆盖允许执行、拒绝不执行、ask 本次允许、ask 本会话允许、ask 拒绝、ask 取消、多工具顺序。

**验证：** `go test ./internal/agent -run 'Scheduler|Permission|Confirm'` 通过。

## T11: Runner 集成权限 Scheduler

**文件：** `internal/agent/runner.go`、`internal/agent/runner_test.go`
**依赖：** T10
**步骤：**
1. 在 Runner 创建 Scheduler 时传入 options 中的权限引擎和确认桥。
2. 添加脚本化 Provider 场景：模型先请求被拒绝工具，再根据权限失败返回最终文本。
3. 验证权限拒绝不会让 Agent 失败或停止在错误终态。
4. 添加 Plan Mode 测试，确认仍只暴露只读工具，且只读工具仍经过权限判断。
5. 添加 Do Mode 测试，确认全部工具恢复后仍受权限约束。

**验证：** `go test ./internal/agent -run 'Runner|Permission|Plan|Do'` 通过。

## T12: 配置层接入权限模式

**文件：** `internal/config/config.go`、`internal/config/load.go`、`internal/config/validate.go`、`internal/config/config_test.go`
**依赖：** T7
**步骤：**
1. 增加 `PermissionConfig` 和 `Config.Permissions`。
2. 在加载时为缺失模式设置 `default`。
3. 校验模式只能为 `strict`、`default`、`relaxed` 或空值。
4. 保持旧配置文件不包含 `permissions` 时可正常加载。
5. 添加测试覆盖默认值、三个合法模式、非法模式、OpenAI/Anthropic 既有配置兼容。

**验证：** `go test ./internal/config` 通过。

## T13: 主程序组装权限系统

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`
**依赖：** T4、T7、T10、T12
**步骤：**
1. 在启动时根据工作区调用 `permissions.DefaultFilePaths`。
2. 调用 `permissions.LoadRuleSet` 加载规则；配置错误时向 stderr 输出诊断并退出。
3. 创建 `permissions.Sandbox`、`RuleStore` 和 `Engine`。
4. 创建 TUI 权限确认桥并传给 Runner options。
5. 保持没有权限规则文件时正常启动。
6. 添加 main 测试覆盖缺失规则文件、非法规则文件、权限模式传入 Runner。

**验证：** `go test ./cmd/mewcode -run 'Permission|Config|Run'` 通过。

## T14: 实现 TUI 权限确认桥

**文件：** `internal/tui/permissions.go`、`internal/tui/model.go`、`internal/tui/update.go`、`internal/tui/view.go`、`internal/tui/tui_test.go`
**依赖：** T9、T10
**步骤：**
1. 新建 TUI `PermissionBridge`，用 channel 把权限请求送入 Model，并等待用户选择。
2. 在 Model 增加 pending permission 状态和当前确认请求。
3. 在 Update 中，当存在确认请求时，按键处理本次允许、本会话允许、永久允许、拒绝、取消。
4. 确认状态下阻止普通输入提交，但保持 Ctrl+C 可取消任务。
5. 在 View 中展示工具名、匹配对象、原因和可选按键。
6. 添加测试覆盖确认提示展示、选择本次允许、选择拒绝、取消任务。

**验证：** `go test ./internal/tui -run 'Permission|Confirm|Cancel'` 通过。

## T15: 更新工具、配置和章节文档辅助文件

**文件：** `config.example.yaml`、`docs/README.md`、`.gitignore`
**依赖：** T12、T13
**步骤：**
1. 在 `config.example.yaml` 增加 `permissions.mode: default` 示例。
2. 在 `docs/README.md` 增加 `ch06` 索引。
3. 在 `.gitignore` 增加 `.mewcode/permissions.local.yaml`。
4. 确认不覆盖用户已有 `.gitignore` 其他修改。

**验证：** `git diff -- config.example.yaml docs/README.md .gitignore` 只包含第六章相关改动。

## T16: 权限系统全量回归

**文件：** 全项目
**依赖：** T1-T15
**步骤：**
1. 运行权限包全部测试。
2. 运行工具、Agent、配置、TUI、主程序相关测试。
3. 运行全项目测试。
4. 如有失败，按失败归属回到对应任务修复并重跑。

**验证：** `go test ./...` 通过。

## 执行顺序

```text
T1
 ├─ T2
 ├─ T3
 └─ T4
      ↓
T5 → T6 → T7 → T8
              ↓
          T9 → T10 → T11
          ↓       ↘
        T12 → T13 → T14
              ↓
             T15
              ↓
             T16
```

