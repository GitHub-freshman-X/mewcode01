# MewCode 工具系统 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/tools/tool.go` | Tool、Metadata、Result、ToolError 和错误类型 |
| 新建 | `internal/tools/registry.go` | 工具注册中心、重复名称校验、工具定义导出 |
| 新建 | `internal/tools/schema.go` | 本章所需的最小 JSON Schema 校验辅助 |
| 新建 | `internal/tools/executor.go` | 工具查找、超时、panic 捕获和 provider.ToolResult 转换 |
| 新建 | `internal/tools/workspace.go` | 工作区路径解析、文本文件读写、输出限制 |
| 新建 | `internal/tools/read_file.go` | `read_file` 工具 |
| 新建 | `internal/tools/write_file.go` | `write_file` 工具 |
| 新建 | `internal/tools/edit_file.go` | `edit_file` 原文唯一匹配替换工具 |
| 新建 | `internal/tools/run_command.go` | `run_command` 工具 |
| 新建 | `internal/tools/find_files.go` | `find_files` 工具 |
| 新建 | `internal/tools/search_code.go` | `search_code` 工具 |
| 新建 | `internal/tools/*_test.go` | 工具接口、注册、执行器、工作区和六个工具测试 |
| 修改 | `internal/provider/message.go` | 增加 ToolCall、ToolResult 与工具内容块 |
| 修改 | `internal/provider/provider.go` | ChatRequest 增加工具定义 |
| 修改 | `internal/provider/event.go` | 增加工具调用流事件 |
| 修改 | `internal/provider/anthropic/request.go` | Anthropic tools、tool_use、tool_result 请求转换 |
| 修改 | `internal/provider/anthropic/stream.go` | Anthropic tool_use 流事件解析 |
| 修改 | `internal/provider/anthropic/anthropic_test.go` | Anthropic 工具请求与流解析测试 |
| 修改 | `internal/provider/openai/request.go` | OpenAI Responses function tools、function_call_output 转换 |
| 修改 | `internal/provider/openai/stream.go` | OpenAI function call 参数分片解析 |
| 修改 | `internal/provider/openai/openai_test.go` | OpenAI 工具请求与流解析测试 |
| 修改 | `internal/conversation/conversation.go` | ChatOptions、Turn 状态、Complete 工具执行和历史回灌 |
| 修改 | `internal/conversation/stream.go` | tool call 分片累积、JSON 参数完成校验 |
| 修改 | `internal/conversation/conversation_test.go` | 工具调用、工具结果回灌、单轮边界测试 |
| 修改 | `internal/tui/model.go` | 工具状态所需字段与类型引用 |
| 修改 | `internal/tui/update.go` | EventCompleted 后执行工具并恢复输入 |
| 修改 | `internal/tui/view.go` | 工具调用和工具结果摘要渲染 |
| 修改 | `internal/tui/tui_test.go` | 工具状态展示和输入恢复测试 |
| 修改 | `cmd/mewcode/main.go` | 创建默认工具注册中心和执行器 |
| 修改 | `cmd/mewcode/main_test.go` | 启动组装包含工具定义与执行器的测试 |

## T1：扩展 Provider 工具数据模型

**文件：** `internal/provider/message.go`、`internal/provider/provider.go`、`internal/provider/event.go`
**依赖：** 无

**步骤：**

1. 在消息模型中增加 `BlockToolCall`、`BlockToolResult`、`ToolCall` 和 `ToolResult`。
2. 更新 `CloneMessage`，确保工具调用和工具结果指针被深拷贝。
3. 增加 `ToolDefinition`，并在 `ChatRequest` 中加入 `Tools []ToolDefinition`。
4. 增加 `EventToolCallStart`、`EventToolCallDelta`、`EventToolCallDone` 和 `ToolCallDelta`。
5. 确保已有 text、thinking、signature、completed 事件兼容不变。

**验证：** 运行 `go test ./internal/provider`，期望编译通过。

## T2：定义工具核心类型

**文件：** `internal/tools/tool.go`
**依赖：** T1

**步骤：**

1. 定义 `Tool` 接口，包含 `Metadata()` 与 `Execute(ctx, input)`。
2. 定义 `Metadata`、`Schema`、`Result`、`ToolError` 和错误类型常量。
3. 提供成功结果和失败结果构造辅助，保证 `ToolName` 与 `Success` 一致。
4. 提供将 `Result` 编码为 JSON 字符串的辅助，失败时返回内部错误结果。

**验证：** 运行 `go test ./internal/tools`，期望包编译通过。

## T3：实现最小 Schema 校验

**文件：** `internal/tools/schema.go`、`internal/tools/schema_test.go`
**依赖：** T2

**步骤：**

1. 支持 object schema 的 `required`、`properties`、`type`、`enum`、整数最小值和最大值校验。
2. 对缺少必填字段、类型不匹配和 enum 不合法返回 `validation_error`。
3. 对本章不使用的 schema 关键字保持忽略，不阻塞工具执行。
4. 添加测试覆盖 string、boolean、integer、array、required、enum 和范围错误。

**验证：** 运行 `go test ./internal/tools -run TestSchema -count=1`，期望全部通过。

## T4：实现工具注册中心

**文件：** `internal/tools/registry.go`、`internal/tools/registry_test.go`
**依赖：** T2、T3

**步骤：**

1. 实现 `NewRegistry`、`Register`、`Get`、`List` 和 `Definitions`。
2. `Register` 校验名称、描述、schema，并拒绝重复名称。
3. `List` 与 `Definitions` 使用稳定顺序，避免测试和 UI 输出抖动。
4. 测试注册成功、重复名称、未知工具、定义导出和稳定排序。

**验证：** 运行 `go test ./internal/tools -run TestRegistry -count=1`，期望全部通过。

## T5：实现工作区路径与文本辅助

**文件：** `internal/tools/workspace.go`、`internal/tools/workspace_test.go`
**依赖：** T2

**步骤：**

1. 实现 `Workspace.Resolve`，对输入路径做 clean、abs、rel 检查，拒绝逃逸工作区。
2. 实现文本文件读取，拒绝目录、二进制内容和超过读取上限的文件。
3. 实现文本写入，要求父目录存在，拒绝目录路径和超过写入上限的内容。
4. 实现输出截断辅助，返回截断标记。
5. 测试合法路径、`..` 逃逸、绝对路径逃逸、目录、二进制、大文件、父目录不存在和 UTF-8 内容。

**验证：** 运行 `go test ./internal/tools -run 'TestWorkspace|TestTruncate' -count=1`，期望全部通过。

## T6：实现 `read_file` 工具

**文件：** `internal/tools/read_file.go`、`internal/tools/read_file_test.go`
**依赖：** T3、T5

**步骤：**

1. 定义 `read_file` metadata、描述和参数 schema。
2. 解析 `path` 参数并执行 schema 校验。
3. 使用 `Workspace.ReadText` 读取内容，并返回 `path`、`bytes`、`content`、`truncated`。
4. 将路径不存在、目录、工作区外、二进制和过大文件映射为结构化错误。
5. 测试成功读取、参数错误和所有失败分支。

**验证：** 运行 `go test ./internal/tools -run TestReadFile -count=1`，期望全部通过。

## T7：实现 `write_file` 工具

**文件：** `internal/tools/write_file.go`、`internal/tools/write_file_test.go`
**依赖：** T3、T5

**步骤：**

1. 定义 `write_file` metadata、描述和参数 schema。
2. 解析 `path` 与 `content`，并执行 schema 校验。
3. 使用 `Workspace.WriteText` 写入内容，返回 `path` 和 `bytes_written`。
4. 将工作区外路径、父目录不存在、目录路径、内容过大和写入失败映射为结构化错误。
5. 测试成功写入后文件内容一致，并覆盖失败分支。

**验证：** 运行 `go test ./internal/tools -run TestWriteFile -count=1`，期望全部通过。

## T8：实现 `edit_file` 工具

**文件：** `internal/tools/edit_file.go`、`internal/tools/edit_file_test.go`
**依赖：** T3、T5

**步骤：**

1. 定义 `edit_file` metadata、描述和 `path`、`old_text`、`new_text` 参数 schema。
2. 读取目标文本文件，拒绝空 `old_text`。
3. 统计 `old_text` 出现次数，恰好一次时替换并写回。
4. 匹配零次或多次时返回 `conflict` 或 `not_found` 错误，并保持文件不变。
5. 测试成功替换、零匹配、多匹配、空原文、工作区外路径和读写错误。

**验证：** 运行 `go test ./internal/tools -run TestEditFile -count=1`，期望全部通过。

## T9：实现 `run_command` 工具

**文件：** `internal/tools/run_command.go`、`internal/tools/run_command_test.go`
**依赖：** T3、T5

**步骤：**

1. 定义 `run_command` metadata、描述和 `command`、`args`、`timeout_ms` 参数 schema。
2. 使用 `exec.CommandContext`，工作目录固定为 workspace root，不通过 shell 拼接。
3. 捕获 stdout、stderr、退出码、运行时长和超时状态。
4. 限制 `timeout_ms` 不超过执行器上限；未提供时使用执行器 context。
5. 测试成功命令、非零退出、启动失败、超时、输出截断和参数错误。

**验证：** 运行 `go test ./internal/tools -run TestRunCommand -count=1`，期望全部通过。

## T10：实现 `find_files` 工具

**文件：** `internal/tools/find_files.go`、`internal/tools/find_files_test.go`
**依赖：** T3、T5

**步骤：**

1. 定义 `find_files` metadata、描述和 `pattern`、`limit` 参数 schema。
2. 递归遍历工作区，跳过 `.git`、`node_modules`、`vendor` 等常见目录。
3. 支持 `*`、`?`、`**` 风格模式，并返回相对路径。
4. 按稳定顺序返回匹配，遵守 limit 并设置 `truncated`。
5. 测试匹配、无匹配、非法模式、limit、跳过目录和稳定排序。

**验证：** 运行 `go test ./internal/tools -run TestFindFiles -count=1`，期望全部通过。

## T11：实现 `search_code` 工具

**文件：** `internal/tools/search_code.go`、`internal/tools/search_code_test.go`
**依赖：** T3、T5、T10

**步骤：**

1. 定义 `search_code` metadata、描述和 `pattern`、`regex`、`limit` 参数 schema。
2. 递归遍历工作区文本文件，跳过二进制文件和常见忽略目录。
3. `regex=false` 时按普通子串搜索；`regex=true` 时使用 Go regexp。
4. 返回文件、行号、列号和片段，遵守 limit 并设置 `truncated`。
5. 测试普通搜索、正则搜索、无匹配、非法正则、limit、Unicode 和二进制跳过。

**验证：** 运行 `go test ./internal/tools -run TestSearchCode -count=1`，期望全部通过。

## T12：实现默认工具注册与执行器

**文件：** `internal/tools/registry.go`、`internal/tools/executor.go`、`internal/tools/executor_test.go`
**依赖：** T4、T6、T7、T8、T9、T10、T11

**步骤：**

1. 实现 `NewDefaultRegistry(root)`，注册六个核心工具。
2. 实现 `NewExecutor` 和 `Executor.Execute`。
3. `Execute` 处理未知工具、context 超时、panic 捕获和 Result JSON 编码。
4. 将 `tools.Result` 转成 `provider.ToolResult`，保留 call id、工具名、结果内容和错误标记。
5. 测试默认工具数量、未知工具、成功执行、超时工具、panic 工具和 JSON 结果形态。

**验证：** 运行 `go test ./internal/tools -count=1`，期望全部通过。

## T13：扩展 Anthropic 请求转换

**文件：** `internal/provider/anthropic/request.go`、`internal/provider/anthropic/anthropic_test.go`
**依赖：** T1、T4

**步骤：**

1. 在 Anthropic 请求体中增加 `tools` 字段，转换 `provider.ToolDefinition`。
2. 将 assistant `BlockToolCall` 转换为 `tool_use` content block。
3. 将 user `BlockToolResult` 转换为 `tool_result` content block，并设置错误标记。
4. 保持 text 与 thinking block 的现有转换行为不变。
5. 测试工具声明、assistant tool_use、user tool_result、混合文本消息和非法工具块。

**验证：** 运行 `go test ./internal/provider/anthropic -run 'Test.*Tool.*Request|Test.*Request' -count=1`，期望全部通过。

## T14：扩展 Anthropic 工具流解析

**文件：** `internal/provider/anthropic/stream.go`、`internal/provider/anthropic/anthropic_test.go`
**依赖：** T1、T13

**步骤：**

1. 解析 `content_block_start` 中的 `tool_use`，发出 `EventToolCallStart`。
2. 解析 `content_block_delta.delta.type = "input_json_delta"`，发出 `EventToolCallDelta`。
3. 在对应 content block 停止时发出 `EventToolCallDone`。
4. 保持 text、thinking、signature、message stop 和 error 解析兼容。
5. 测试 JSON 参数分片、多 block index、未知事件忽略和 malformed JSON。

**验证：** 运行 `go test ./internal/provider/anthropic -run 'Test.*Tool.*Stream|Test.*Stream' -count=1`，期望全部通过。

## T15：扩展 OpenAI 请求转换

**文件：** `internal/provider/openai/request.go`、`internal/provider/openai/openai_test.go`
**依赖：** T1、T4

**步骤：**

1. 在 Responses 请求体中增加 function tools 声明。
2. 支持把 assistant `BlockToolCall` 转换为 function call input item。
3. 支持把 user `BlockToolResult` 转换为 `function_call_output` input item。
4. 保持现有 user/assistant 文本 input 转换兼容。
5. 测试工具声明、function_call_output、含工具历史的请求和非法工具块。

**验证：** 运行 `go test ./internal/provider/openai -run 'Test.*Tool.*Request|Test.*Request' -count=1`，期望全部通过。

## T16：扩展 OpenAI 工具流解析

**文件：** `internal/provider/openai/stream.go`、`internal/provider/openai/openai_test.go`
**依赖：** T1、T15

**步骤：**

1. 解析 function call item 创建事件，发出 `EventToolCallStart`。
2. 解析 `response.function_call_arguments.delta`，发出 `EventToolCallDelta`。
3. 解析 `response.function_call_arguments.done`，发出 `EventToolCallDone`，包含 call id、名称和完整参数。
4. 保持现有 output text、completed、failed、incomplete 和 error 事件兼容。
5. 测试参数分片拼接所需字段、done 完整参数、未知事件忽略和错误事件。

**验证：** 运行 `go test ./internal/provider/openai -run 'Test.*Tool.*Stream|Test.*Stream' -count=1`，期望全部通过。

## T17：更新 Conversation 工具状态与请求

**文件：** `internal/conversation/conversation.go`
**依赖：** T1、T12

**步骤：**

1. 在 `ChatOptions` 中加入工具定义和执行器。
2. 在 `Start` 中把工具定义传入 `provider.ChatRequest`。
3. 扩展 `TurnState`，增加工具请求、执行中和执行完成状态。
4. 在 `Turn` 中记录本轮工具结果。
5. 调整 `IsBusy`，确保工具执行期间仍被视为 busy。

**验证：** 运行 `go test ./internal/conversation`，期望包编译通过。

## T18：实现 Conversation 工具事件累积

**文件：** `internal/conversation/stream.go`、`internal/conversation/conversation_test.go`
**依赖：** T17

**步骤：**

1. 在 `Apply` 中处理 `EventToolCallStart`，创建或定位 tool call block。
2. 在 `Apply` 中处理 `EventToolCallDelta`，累积 JSON 参数分片。
3. 在 `Apply` 中处理 `EventToolCallDone`，写入完整参数并校验 JSON 可解析。
4. 对缺失 call id、缺失工具名、参数 JSON malformed 和 block 类型冲突返回错误。
5. 测试分片参数拼接、done 完整参数、多 block index 和 malformed JSON。

**验证：** 运行 `go test ./internal/conversation -run 'Test.*ToolCall.*Apply|Test.*Malformed' -count=1`，期望全部通过。

## T19：实现 Conversation 工具执行与历史回灌

**文件：** `internal/conversation/conversation.go`、`internal/conversation/conversation_test.go`
**依赖：** T12、T18

**步骤：**

1. 将 `Complete` 改为接收 context，并提交 user + assistant 消息到 history。
2. 当 assistant 消息不含 tool call 时保持原完成行为。
3. 当含 tool call 时调用执行器，生成 user tool result 消息并追加到 history。
4. 工具失败也作为 tool result 追加，不把本轮标记为 `TurnFailed`。
5. 确保工具结果回灌后不自动再次调用 Provider。
6. 测试成功工具、未知工具、工具错误、超时、历史顺序和单轮边界。

**验证：** 运行 `go test ./internal/conversation -count=1`，期望全部通过。

## T20：更新 TUI 工具状态处理

**文件：** `internal/tui/update.go`、`internal/tui/model.go`、`internal/tui/tui_test.go`
**依赖：** T19

**步骤：**

1. 更新 `EventCompleted` 分支，调用新的 `Conversation.Complete(ctx)`。
2. 工具执行期间保持输入失焦，并在完成或失败后恢复输入。
3. 取消逻辑覆盖工具执行中状态，确保不会留下 busy 状态。
4. 调整测试用 fake conversation 或 fake provider，覆盖工具完成后可继续输入。

**验证：** 运行 `go test ./internal/tui -run 'Test.*Tool|Test.*Update' -count=1`，期望全部通过。

## T21：更新 TUI 工具渲染

**文件：** `internal/tui/view.go`、`internal/tui/tui_test.go`
**依赖：** T19

**步骤：**

1. 在消息渲染中显示 assistant tool call 摘要。
2. 在消息渲染中显示 user tool result 的成功或失败摘要。
3. 更新状态栏文案，覆盖 `tool_requested`、`tool_running`、`tool_completed`。
4. 对工具参数和结果摘要做截断，不在界面完整展开大 JSON。
5. 测试工具名称、成功摘要、失败摘要、状态栏和长结果截断。

**验证：** 运行 `go test ./internal/tui -run 'Test.*Tool.*View|Test.*Status' -count=1`，期望全部通过。

## T22：接入 CLI 默认工具系统

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`
**依赖：** T12、T19

**步骤：**

1. 在启动时获取当前工作目录作为工具工作区 root。
2. 创建默认工具注册中心和 30 秒执行器。
3. 将工具 definitions 和 executor 放入 `conversation.ChatOptions`。
4. 保持配置加载、Provider 创建和 TUI 启动错误处理不变。
5. 测试启动组装会传入六个工具定义和非空执行器。

**验证：** 运行 `go test ./cmd/mewcode -count=1`，期望全部通过。

## T23：补齐 Provider 端到端工具流测试

**文件：** `internal/provider/anthropic/anthropic_test.go`、`internal/provider/openai/openai_test.go`
**依赖：** T14、T16

**步骤：**

1. 使用测试 SSE 服务构造 Anthropic tool_use 参数分片流，断言输出中立工具事件顺序。
2. 使用测试 SSE 服务构造 OpenAI function call 参数分片流，断言输出中立工具事件顺序。
3. 对两个 Provider 都断言工具声明随请求体发送。
4. 对两个 Provider 都断言工具结果历史能转换回对应 API 格式。

**验证：** 运行 `go test ./internal/provider/anthropic ./internal/provider/openai -count=1`，期望全部通过。

## T24：补齐对话层端到端工具场景

**文件：** `internal/conversation/conversation_test.go`
**依赖：** T19、T23

**步骤：**

1. 构造 fake provider，流式返回一次 `read_file` tool call。
2. 使用真实 executor 和临时工作区执行工具。
3. 断言 history 顺序为 user、assistant tool_call、user tool_result。
4. 断言下一次 `Start` 发送给 provider 的 messages 包含 tool result。
5. 断言工具结果回灌后 provider 没有被自动调用第二次。

**验证：** 运行 `go test ./internal/conversation -run TestToolRoundTrip -count=1`，期望通过。

## T25：运行全量测试与格式化

**文件：** 全部 Go 文件
**依赖：** T1-T24

**步骤：**

1. 运行 `gofmt` 格式化所有新增和修改的 Go 文件。
2. 运行 `go test ./...`。
3. 若失败，按失败包修复并重新运行对应包测试。
4. 再次运行 `go test ./...`，确认全量通过。

**验证：** 运行 `go test ./...`，期望全部通过。

## 执行顺序

```text
T1
 ├─→ T2 → T3 → T4
 │              └─→ T12
 ├─→ T5 → T6 → T7 → T8 → T9 → T10 → T11 ─┘
 ├─→ T13 → T14 ─┐
 └─→ T15 → T16 ─┴─→ T23

T12 + T18 → T19 → T20 → T21 → T22
T17 ─────→ T18
T19 + T23 → T24
T1-T24 → T25
```

