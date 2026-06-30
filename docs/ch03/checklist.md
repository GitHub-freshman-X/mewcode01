# MewCode 工具系统 Checklist

> 每一项通过运行代码或观察行为来验证，聚焦系统行为。

## 实现完整性

- [ ] 统一工具能力边界已实现，任一测试工具可暴露名称、描述、参数 Schema 并返回结构化结果（验证：运行 `go test ./internal/tools -run 'TestTool|TestResult' -count=1`，期望通过）
- [ ] 默认工具注册中心包含且只包含六个核心工具：`read_file`、`write_file`、`edit_file`、`run_command`、`find_files`、`search_code`（验证：运行 `go test ./internal/tools -run TestDefaultRegistry -count=1`，期望通过）
- [ ] 工具注册中心拒绝重复名称，并能按名称查找工具、导出稳定排序的模型工具声明（验证：运行 `go test ./internal/tools -run TestRegistry -count=1`，期望通过）
- [ ] 工具执行器对未知工具、超时和 panic 都返回结构化失败结果，不向外抛出未处理错误（验证：运行 `go test ./internal/tools -run TestExecutor -count=1`，期望通过）
- [ ] 参数 Schema 校验覆盖必填字段、类型、枚举和数值范围，校验失败时工具动作不执行（验证：运行 `go test ./internal/tools -run TestSchema -count=1`，期望通过）

## 核心工具行为

- [ ] `read_file` 能读取工作区内文本文件，并返回路径、字节数、内容和截断标记（验证：运行 `go test ./internal/tools -run TestReadFile -count=1`，期望成功用例通过）
- [ ] `read_file` 拒绝不存在路径、目录、工作区外路径、二进制文件和过大文件，并返回清晰结构化错误（验证：运行 `go test ./internal/tools -run TestReadFileFailures -count=1`，期望通过）
- [ ] `write_file` 能写入工作区内已有父目录下的文本文件，写入后文件内容与请求内容一致（验证：运行 `go test ./internal/tools -run TestWriteFile -count=1`，期望成功用例通过）
- [ ] `write_file` 拒绝工作区外路径、缺失父目录、目录路径、超大内容和不可写路径，且不产生意外文件（验证：运行 `go test ./internal/tools -run TestWriteFileFailures -count=1`，期望通过）
- [ ] `edit_file` 在原文恰好出现一次时完成替换，并返回替换数量和写入信息（验证：运行 `go test ./internal/tools -run TestEditFile -count=1`，期望成功用例通过）
- [ ] `edit_file` 在原文出现零次或多次时保持文件不变，并返回匹配数量相关错误（验证：运行 `go test ./internal/tools -run TestEditFileNoUniqueMatch -count=1`，期望通过）
- [ ] `run_command` 返回退出码、stdout、stderr、是否超时和执行时长，成功与非零退出都能结构化返回（验证：运行 `go test ./internal/tools -run TestRunCommand -count=1`，期望通过）
- [ ] `run_command` 使用 `command` + `args` 执行，不依赖 shell 拼接，并在超时后结束命令（验证：运行 `go test ./internal/tools -run 'TestRunCommandNoShell|TestRunCommandTimeout' -count=1`，期望通过）
- [ ] `find_files` 按模式返回工作区相对路径，遵守 limit，跳过常见依赖目录并稳定排序（验证：运行 `go test ./internal/tools -run TestFindFiles -count=1`，期望通过）
- [ ] `search_code` 支持普通文本和正则搜索，返回文件、行号、列号和片段，并遵守 limit（验证：运行 `go test ./internal/tools -run TestSearchCode -count=1`，期望通过）
- [ ] 文件类工具无法通过相对路径或绝对路径逃逸工作区（验证：运行 `go test ./internal/tools -run 'TestWorkspace.*Escape|Test.*OutsideWorkspace' -count=1`，期望通过）
- [ ] 超大文件、海量搜索结果和长命令输出会被截断并标记，不撑爆返回结果（验证：运行 `go test ./internal/tools -run 'Test.*Truncated|TestTruncate' -count=1`，期望通过）

## Provider 集成

- [ ] Provider 中立模型支持工具声明、assistant tool call 和 user tool result 内容块，且消息拷贝不会共享可变指针（验证：运行 `go test ./internal/provider -run 'Test.*Tool|TestClone' -count=1`，期望通过）
- [ ] Anthropic 请求中包含 `tools` 声明，并能把 assistant tool call 和 user tool result 转成 Messages API 内容块（验证：运行 `go test ./internal/provider/anthropic -run 'Test.*Tool.*Request|Test.*Request' -count=1`，期望通过）
- [ ] Anthropic 流式 tool_use 参数分片能转换为中立工具调用事件，并在完成时生成完整参数（验证：运行 `go test ./internal/provider/anthropic -run 'Test.*Tool.*Stream|Test.*Stream' -count=1`，期望通过）
- [ ] OpenAI Responses 请求中包含 function tools，并能把工具结果历史转为 `function_call_output`（验证：运行 `go test ./internal/provider/openai -run 'Test.*Tool.*Request|Test.*Request' -count=1`，期望通过）
- [ ] OpenAI function call 参数 delta 和 done 事件能转换为中立工具调用事件，并保留 call id、名称和参数（验证：运行 `go test ./internal/provider/openai -run 'Test.*Tool.*Stream|Test.*Stream' -count=1`，期望通过）
- [ ] malformed 工具参数流、未知工具事件和供应商错误事件不会导致 Provider panic，并返回可诊断错误或安全忽略（验证：运行 `go test ./internal/provider/anthropic ./internal/provider/openai -run 'Test.*Malformed|Test.*Unknown|Test.*Error' -count=1`，期望通过）

## 对话与界面集成

- [ ] Conversation 发起请求时会把当前工具声明传给 Provider（验证：运行 `go test ./internal/conversation -run TestConversationSendsToolDefinitions -count=1`，期望通过）
- [ ] Conversation 能拼接流式工具调用 JSON 参数，并在 malformed JSON 时返回可诊断错误（验证：运行 `go test ./internal/conversation -run 'Test.*ToolCall.*Apply|Test.*Malformed' -count=1`，期望通过）
- [ ] 模型请求工具后，Conversation 会提交 user、assistant tool call、user tool result 到历史，且下一次请求能看到工具结果（验证：运行 `go test ./internal/conversation -run TestToolRoundTrip -count=1`，期望通过）
- [ ] 工具执行失败、未知工具和超时会作为 tool result 进入历史，不把对话流程变成崩溃或不可继续状态（验证：运行 `go test ./internal/conversation -run 'Test.*Tool.*Failure|Test.*Tool.*Timeout|Test.*UnknownTool' -count=1`，期望通过）
- [ ] 工具结果回灌后不会自动再次请求模型；只有用户再次提交输入才发起下一次 Provider 调用（验证：运行 `go test ./internal/conversation -run TestToolResultStopsBeforeAgentLoop -count=1`，期望通过）
- [ ] TUI 能显示工具调用名称、执行中状态、成功摘要和失败摘要（验证：运行 `go test ./internal/tui -run 'Test.*Tool.*View|Test.*Status' -count=1`，期望通过）
- [ ] 工具执行完成或失败后输入区恢复可用，用户可以继续输入下一条消息（验证：运行 `go test ./internal/tui -run 'Test.*Tool.*Update|Test.*InputRestored' -count=1`，期望通过）
- [ ] TUI 和 Conversation 测试只依赖中立 Provider 事件，不解析 Anthropic 或 OpenAI 原始 SSE 字段（验证：运行 `go test ./internal/conversation ./internal/tui -count=1`，期望通过）

## 编译与测试

- [ ] 所有新增和修改的 Go 文件都已格式化（验证：运行 `gofmt -w` 处理任务清单中的 Go 文件后，再运行 `git diff --check`，期望无 whitespace error）
- [ ] 工具层单元测试全部通过（验证：运行 `go test ./internal/tools -count=1`，期望通过）
- [ ] Provider 工具调用相关测试全部通过（验证：运行 `go test ./internal/provider ./internal/provider/anthropic ./internal/provider/openai -count=1`，期望通过）
- [ ] 对话层与 TUI 集成测试全部通过（验证：运行 `go test ./internal/conversation ./internal/tui -count=1`，期望通过）
- [ ] CLI 组装测试确认默认工具注册中心和执行器已接入启动流程（验证：运行 `go test ./cmd/mewcode -count=1`，期望通过）
- [ ] 项目全量测试通过，不访问真实模型 API（验证：运行 `go test ./...`，期望全部通过，测试服务均使用本地 fake server）

## 端到端场景

- [ ] 场景 1：模型流式请求 `read_file`，系统执行工具并把文件内容作为 tool result 写入历史；下一次用户输入时 Provider 请求包含该 tool result（验证：运行 `go test ./internal/conversation -run TestToolRoundTrip -count=1`，期望 history 顺序和下一次请求内容断言通过）
- [ ] 场景 2：模型请求 `edit_file`，原文在目标文件中恰好出现一次时文件被修改；出现零次或多次时文件保持不变并返回错误结果（验证：运行 `go test ./internal/tools -run 'TestEditFile|TestEditFileNoUniqueMatch' -count=1`，期望通过）
- [ ] 场景 3：模型请求超时命令，系统在超时后返回带 `timed_out` 标记的 tool result，TUI 显示失败摘要并允许继续输入（验证：运行 `go test ./internal/tools ./internal/conversation ./internal/tui -run 'TestRunCommandTimeout|Test.*Tool.*Timeout|Test.*InputRestored' -count=1`，期望通过）
- [ ] 场景 4：Anthropic 和 OpenAI 都能通过同一套 Conversation 工具事件完成工具调用解析，上层不依赖供应商原始事件（验证：运行 `go test ./internal/provider/anthropic ./internal/provider/openai ./internal/conversation -run 'Test.*Tool.*Stream|TestToolRoundTrip' -count=1`，期望通过）

