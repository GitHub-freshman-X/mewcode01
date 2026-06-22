# MewCode 纯对话基础 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `go.mod`、`go.sum` | Go 1.25 module 与锁定依赖 |
| 新建 | `.gitignore` | 忽略本地配置、二进制和测试产物 |
| 新建 | `config.example.yaml` | Anthropic/OpenAI 配置示例与假密钥 |
| 新建 | `README.md` | 构建、配置、运行、按键和安全说明 |
| 新建 | `.github/workflows/ci.yml` | Linux、macOS、Windows 构建测试与 Linux race |
| 新建 | `cmd/mewcode/main.go` | 参数解析、依赖组装、TUI 启动和退出码 |
| 新建 | `cmd/mewcode/main_test.go` | 配置失败、参数覆盖与安全错误测试 |
| 新建 | `internal/config/config.go` | 配置类型、协议常量和默认值 |
| 新建 | `internal/config/load.go` | 默认路径与严格 YAML 加载 |
| 新建 | `internal/config/validate.go` | 字段和跨字段校验 |
| 新建 | `internal/config/config_test.go` | 路径、解析、默认值、校验和密钥保护测试 |
| 新建 | `internal/provider/provider.go` | Provider 接口与 ChatRequest |
| 新建 | `internal/provider/message.go` | Role、Message、ContentBlock |
| 新建 | `internal/provider/event.go` | StreamEvent、AppError 和用户安全错误文本 |
| 新建 | `internal/provider/sse/decoder.go` | 通用 SSE 解码器 |
| 新建 | `internal/provider/sse/decoder_test.go` | SSE 分帧、Unicode、上限和异常测试 |
| 新建 | `internal/provider/anthropic/client.go` | Anthropic HTTP 和流生命周期 |
| 新建 | `internal/provider/anthropic/request.go` | Messages API 请求转换 |
| 新建 | `internal/provider/anthropic/stream.go` | Anthropic SSE 事件归一化 |
| 新建 | `internal/provider/anthropic/anthropic_test.go` | Anthropic 请求、thinking、流、错误和取消测试 |
| 新建 | `internal/provider/openai/client.go` | OpenAI HTTP 和流生命周期 |
| 新建 | `internal/provider/openai/request.go` | Responses API input 转换 |
| 新建 | `internal/provider/openai/stream.go` | OpenAI 类型化事件归一化 |
| 新建 | `internal/provider/openai/openai_test.go` | OpenAI 请求、流、错误和取消测试 |
| 新建 | `internal/provider/factory/factory.go` | 按协议创建 Provider |
| 新建 | `internal/provider/factory/factory_test.go` | 两种协议与错误分支测试 |
| 新建 | `internal/conversation/conversation.go` | 历史、活动 Turn 与提交边界 |
| 新建 | `internal/conversation/stream.go` | 增量聚合、完成、失败和取消 |
| 新建 | `internal/conversation/conversation_test.go` | 多轮、thinking、失败和取消测试 |
| 新建 | `internal/tui/model.go` | TUI Model 和组件状态 |
| 新建 | `internal/tui/update.go` | 输入、流事件、resize 和状态转移 |
| 新建 | `internal/tui/commands.go` | 异步等待 Provider 通道的 Bubble Tea Cmd |
| 新建 | `internal/tui/view.go` | 消息、thinking、错误和状态栏渲染 |
| 新建 | `internal/tui/keymap.go` | 提交、滚动、thinking、取消和退出按键 |
| 新建 | `internal/tui/styles.go` | 最小跨终端 Lip Gloss 样式 |
| 新建 | `internal/tui/run.go` | alternate screen 程序入口 |
| 新建 | `internal/tui/tui_test.go` | Update、渲染、流式、滚动、resize 和取消测试 |
| 新建 | `internal/testutil/streamserver.go` | 可记录请求并分段 flush SSE 的测试服务 |

## T1：初始化 Go module 与依赖

**文件：** `go.mod`、`go.sum`
**依赖：** 无

**步骤：**

1. 以 `github.com/GitHub-freshman-X/mewcode01` 初始化 module，并设置 Go 基线为 1.25。
2. 添加 `charm.land/bubbletea/v2`、`charm.land/bubbles/v2`、`charm.land/lipgloss/v2` 和 `go.yaml.in/yaml/v4` 的相互兼容稳定版本。
3. 运行 module tidy，锁定传递依赖。

**验证：** 运行 `go mod edit -json`，期望 Module Path 正确且 GoVersion 为 `1.25`；运行 `go mod verify`，期望输出 `all modules verified`。

## T2：定义配置模型与默认值

**文件：** `internal/config/config.go`
**依赖：** T1

**步骤：**

1. 定义 `Protocol`、两个合法协议常量、`Config` 和 `ThinkingConfig`，YAML tag 与 Plan 保持一致。
2. 定义默认 `max_tokens = 4096`。
3. 提供只应用缺省值、不覆盖显式值的内部逻辑。

**验证：** 运行 `go test ./internal/config`，期望包编译通过。

## T3：实现默认配置路径与严格加载

**文件：** `internal/config/load.go`
**依赖：** T2

**步骤：**

1. 使用系统用户配置目录拼接 `mewcode/config.yaml`。
2. 实现 `Load(path)`，文件读取失败时附带路径和 config 阶段信息。
3. 使用 YAML v4 的 known-fields 与 single-document 选项解码。
4. 解码后应用默认值并调用 `Validate`。

**验证：** 运行 `go test ./internal/config`，期望包编译通过且无未定义接口。

## T4：实现配置校验

**文件：** `internal/config/validate.go`
**依赖：** T2

**步骤：**

1. 校验四个核心字段非空、协议取值合法、`base_url` 为带 scheme/host 的 HTTP(S) URL。
2. 校验 `max_tokens > 0`。
3. 禁止 OpenAI 使用 thinking。
4. Anthropic thinking 开启时校验预算至少 1024 且小于 `max_tokens`；关闭时拒绝非零预算以避免静默配置错误。
5. 确保所有错误只包含字段名和安全说明，不包含 API Key 值。

**验证：** 运行 `go test ./internal/config`，期望包编译通过。

## T5：覆盖配置行为测试

**文件：** `internal/config/config_test.go`
**依赖：** T3、T4

**步骤：**

1. 测试默认路径末尾为 `mewcode/config.yaml`。
2. 测试最小有效 Anthropic/OpenAI YAML 和默认 token 值。
3. 测试未知字段、多文档、非法 YAML、缺失核心字段、非法协议和 URL。
4. 测试 thinking 的协议限制、最小预算和总预算关系。
5. 使用独特假密钥断言所有错误文本均不包含该值。

**验证：** 运行 `go test ./internal/config -run Test -count=1`，期望全部通过。

## T6：定义统一消息模型

**文件：** `internal/provider/message.go`
**依赖：** T1

**步骤：**

1. 定义 `Role`、用户/助手角色常量。
2. 定义 text/thinking block 类型、`ContentBlock` 和 `Message`。
3. 提供深拷贝消息及消息切片的辅助函数，确保 history 快照不可变。

**验证：** 运行 `go test ./internal/provider`，期望包编译通过。

## T7：定义 Provider、流事件与安全错误

**文件：** `internal/provider/provider.go`、`internal/provider/event.go`
**依赖：** T6

**步骤：**

1. 定义 `ThinkingOptions`、`ChatRequest` 和 `Provider.Stream` 接口。
2. 定义五种流事件及 `StreamEvent`。
3. 定义四种错误阶段与 `AppError`，实现 `Error` 和 `Unwrap`。
4. 提供仅输出安全 `Message` 与状态码、不渲染底层 Cause 的用户文本函数。

**验证：** 运行 `go test ./internal/provider`，期望包编译通过。

## T8：实现 SSE 基础解码

**文件：** `internal/provider/sse/decoder.go`
**依赖：** T7

**步骤：**

1. 实现逐行读取和空行分帧，兼容 LF 与 CRLF。
2. 解析 `event`、`data`、`id`，按换行拼接多条 data，忽略注释及未知 SSE 字段。
3. 跳过没有业务字段的空帧；在正常 EOF 返回 `io.EOF`。
4. 统计单帧累计字节并在超过配置上限时返回明确错误。

**验证：** 运行 `go test ./internal/provider/sse`，期望包编译通过。

## T9：覆盖 SSE 边界测试

**文件：** `internal/provider/sse/decoder_test.go`
**依赖：** T8

**步骤：**

1. 测试单行、多行 data、注释、id、未知字段、LF 和 CRLF。
2. 测试中文内容被分段读取后仍保持 UTF-8 正确。
3. 测试 EOF 前最后一帧、纯空流和不完整行。
4. 测试超过 1 MiB 限制时返回错误且不分配无限内存。

**验证：** 运行 `go test ./internal/provider/sse -count=1`，期望全部通过。

## T10：实现流式测试服务

**文件：** `internal/testutil/streamserver.go`
**依赖：** T1

**步骤：**

1. 封装 `httptest.Server`，记录请求路径、请求头和请求体。
2. 支持设置响应状态、响应头和按顺序写入的 SSE 帧。
3. 每帧调用 `http.Flusher.Flush`，并支持测试控制的帧间阻塞点。
4. 提供关闭方法，测试结束时不遗留连接。

**验证：** 运行 `go test ./internal/testutil`，期望包编译通过。

## T11：实现 Anthropic 请求转换

**文件：** `internal/provider/anthropic/request.go`
**依赖：** T7

**步骤：**

1. 定义仅在 Anthropic 包内可见的 Messages 请求、消息、text/thinking block 和 thinking 配置 JSON 类型。
2. 将统一用户/助手消息按原顺序转换，保留 thinking 文本和 signature。
3. 填充 model、max_tokens、stream，并仅在启用时加入 enabled thinking 与 budget。
4. 对非法角色、非法 block 或缺失 signature 的已提交 thinking 返回 request 阶段错误。

**验证：** 运行 `go test ./internal/provider/anthropic`，期望包编译通过。

## T12：实现 Anthropic 流事件归一化

**文件：** `internal/provider/anthropic/stream.go`
**依赖：** T8、T11

**步骤：**

1. 定义最小 Anthropic SSE JSON envelope 与 delta 类型。
2. 映射 message start、thinking/text/signature delta、message stop 和 error。
3. 保留 `index` 到统一 `BlockIndex`，未知事件返回“忽略”结果。
4. 对已知事件的非法 JSON 或缺失必要字段返回 stream 阶段错误。

**验证：** 运行 `go test ./internal/provider/anthropic`，期望包编译通过。

## T13：实现 Anthropic HTTP 流生命周期

**文件：** `internal/provider/anthropic/client.go`
**依赖：** T11、T12

**步骤：**

1. 实现 Options、构造函数及保留 base path 的 `/v1/messages` 端点拼接。
2. 创建带 context 的 POST 请求，设置 `x-api-key`、版本、Content-Type 和 Accept。
3. 对非 2xx 响应读取有限错误体并转换为安全 `AppError`，不包含请求头或密钥。
4. 在 goroutine 中驱动 SSE Decoder，按顺序发送统一事件和一次终结结果。
5. 处理取消、EOF 前缺少 message stop、响应体关闭及通道关闭。

**验证：** 运行 `go test ./internal/provider/anthropic`，期望包编译通过。

## T14：测试 Anthropic 正常流与 thinking

**文件：** `internal/provider/anthropic/anthropic_test.go`
**依赖：** T10、T13

**步骤：**

1. 使用测试服务断言路径、认证头、版本头和 `stream: true`。
2. 断言 model、max_tokens、用户历史和 assistant 文本转换。
3. 构造 thinking、signature、text 增量，断言事件顺序、index 和最终完成。
4. 发起第二轮请求，断言 thinking block 与 signature 原样回传。
5. 测试关闭 thinking 时请求体不包含 thinking 字段。

**验证：** 运行 `go test ./internal/provider/anthropic -run 'Test.*(Request|Stream|Thinking)' -count=1`，期望全部通过。

## T15：测试 Anthropic 错误与取消

**文件：** `internal/provider/anthropic/anthropic_test.go`
**依赖：** T13、T14

**步骤：**

1. 覆盖 401、429、500，并断言安全消息、状态码和密钥不泄露。
2. 覆盖 malformed JSON、未知事件继续、EOF 缺少完成事件。
3. 在服务端阻塞流时取消 context，断言终结结果为 `context.Canceled` 且连接关闭。
4. 断言成功路径仅产生一个完成事件和一个 nil 终结结果。

**验证：** 运行 `go test ./internal/provider/anthropic -count=1`，期望全部通过。

## T16：实现 OpenAI 请求转换

**文件：** `internal/provider/openai/request.go`
**依赖：** T7

**步骤：**

1. 定义 Responses 请求及 input message 的包内 JSON 类型。
2. 按顺序转换本地完整历史，用户和 assistant 均只发送 text block。
3. 跳过 Claude thinking block，不发送 signature。
4. 填充 model、input、max_output_tokens 和 `stream: true`；非法角色或没有可见文本的消息返回 request 阶段错误。

**验证：** 运行 `go test ./internal/provider/openai`，期望包编译通过。

## T17：实现 OpenAI 流事件归一化

**文件：** `internal/provider/openai/stream.go`
**依赖：** T8、T16

**步骤：**

1. 定义带 `type`、`delta` 和错误摘要的最小 Responses 事件 envelope。
2. 映射 response created、output text delta 和 response completed。
3. 将 response failed、response incomplete 与 error 转换为 stream 阶段错误。
4. 未知类型返回“忽略”，已知事件 malformed JSON 返回错误。

**验证：** 运行 `go test ./internal/provider/openai`，期望包编译通过。

## T18：实现 OpenAI HTTP 流生命周期

**文件：** `internal/provider/openai/client.go`
**依赖：** T16、T17

**步骤：**

1. 实现 Options、构造函数及保留 base path 的 `/responses` 端点拼接。
2. 创建带 context 的 POST 请求，设置 Bearer、Content-Type 和 Accept。
3. 对非 2xx 响应读取有限错误体并转换为安全 `AppError`。
4. 驱动 SSE Decoder，按统一通道契约发送事件和终结结果。
5. 处理取消、EOF 前缺少 response completed、响应体及通道关闭。

**验证：** 运行 `go test ./internal/provider/openai`，期望包编译通过。

## T19：测试 OpenAI 正常流与多轮请求

**文件：** `internal/provider/openai/openai_test.go`
**依赖：** T10、T18

**步骤：**

1. 断言 `/responses` 路径、Bearer 头、model、max_output_tokens 和 stream 字段。
2. 断言多轮用户/助手文本按顺序进入 input。
3. 加入 Claude thinking block，断言 OpenAI 请求只包含可见文本。
4. 逐帧返回 created、多个 UTF-8 delta 和 completed，断言统一事件顺序与完整文本。

**验证：** 运行 `go test ./internal/provider/openai -run 'Test.*(Request|Stream)' -count=1`，期望全部通过。

## T20：测试 OpenAI 错误与取消

**文件：** `internal/provider/openai/openai_test.go`
**依赖：** T18、T19

**步骤：**

1. 覆盖 401、429、500，并断言密钥不出现在错误中。
2. 覆盖 failed、incomplete、error、malformed JSON、未知事件和截断流。
3. 阻塞流后取消 context，断言收到 `context.Canceled` 且连接释放。
4. 断言成功路径只产生一个 completed 和一个 nil 终结结果。

**验证：** 运行 `go test ./internal/provider/openai -count=1`，期望全部通过。

## T21：实现并测试 Provider 工厂

**文件：** `internal/provider/factory/factory.go`、`internal/provider/factory/factory_test.go`
**依赖：** T5、T15、T20

**步骤：**

1. 将已校验 Config 的 base URL 解析为 URL 并构造对应 Options。
2. 为 anthropic/openai 返回对应 Provider，注入调用方 HTTP Client。
3. 对未知协议返回安全错误。
4. 测试两种选择、HTTP Client 注入和错误分支。

**验证：** 运行 `go test ./internal/provider/factory -count=1`，期望全部通过。

## T22：实现 Conversation 生命周期

**文件：** `internal/conversation/conversation.go`
**依赖：** T7

**步骤：**

1. 定义 ChatOptions、TurnState、Turn 和 Conversation。
2. 实现构造、History 与 ActiveTurn 深拷贝查询。
3. 实现 Start：拒绝空输入和并发活动请求，创建子 context 与活动 Turn，并向 Provider 发送历史快照加当前用户消息。
4. 实现 Complete、Fail、Cancel 的状态保护和 cancel function 生命周期。

**验证：** 运行 `go test ./internal/conversation`，期望包编译通过。

## T23：实现流增量聚合

**文件：** `internal/conversation/stream.go`
**依赖：** T22

**步骤：**

1. 根据 block index 查找或创建 assistant content block。
2. 将 thinking、signature 和 text delta 追加到正确字段。
3. 让 started/thinking/generating/completed 事件驱动合法状态转移。
4. 对无活动 Turn、非法事件顺序和 block 类型冲突返回错误，不产生 panic。

**验证：** 运行 `go test ./internal/conversation`，期望包编译通过。

## T24：测试完成、多轮与 thinking 聚合

**文件：** `internal/conversation/conversation_test.go`
**依赖：** T23

**步骤：**

1. 实现记录 ChatRequest 的 fake Provider。
2. 测试首轮请求、增量聚合和完成后提交两条历史。
3. 测试第二轮请求包含第一轮完整历史。
4. 测试 thinking 文本、signature 和普通文本按 block index 正确保存。
5. 修改查询返回值后断言内部历史未被篡改。

**验证：** 运行 `go test ./internal/conversation -run 'Test.*(Complete|MultiTurn|Thinking|Copy)' -count=1`，期望全部通过。

## T25：测试失败、取消和状态边界

**文件：** `internal/conversation/conversation_test.go`
**依赖：** T22、T24

**步骤：**

1. 测试失败和取消不提交用户或部分 assistant 消息。
2. 测试取消调用 context cancel，并允许随后开始新请求。
3. 测试活动请求期间拒绝再次 Start。
4. 测试无活动 Turn 时 Complete/Apply 的安全错误。
5. 测试取消、失败、完成的状态均可供 TUI 观察。

**验证：** 运行 `go test ./internal/conversation -count=1`，期望全部通过。

## T26：建立 TUI Model、按键和样式

**文件：** `internal/tui/model.go`、`internal/tui/keymap.go`、`internal/tui/styles.go`
**依赖：** T25

**步骤：**

1. 定义 Model，持有 Conversation、textarea、viewport、尺寸、自动跟随和 thinking 展开状态。
2. 初始化 textarea 的高度、提示文字、Unicode 输入和焦点。
3. 定义 Enter、滚动、`Ctrl+T`、`Ctrl+C` 的 key binding。
4. 定义可在无色终端退化的用户、助手、thinking、错误和状态样式。

**验证：** 运行 `go test ./internal/tui`，期望包编译通过。

## T27：实现消息与状态渲染

**文件：** `internal/tui/view.go`
**依赖：** T26

**步骤：**

1. 将已提交 history 和活动 Turn 转为 viewport 内容。
2. 分别渲染用户文本、assistant 文本、thinking 面板和错误摘要。
3. 生成期间显示 thinking 内容；完成后默认显示折叠摘要，展开时显示全文。
4. 渲染 idle、connecting、thinking、generating、completed、cancelled、failed 状态栏。
5. View 组合 viewport、textarea 和状态栏，尺寸不足时仍返回可显示内容。

**验证：** 运行 `go test ./internal/tui`，期望包编译通过。

## T28：实现异步流等待命令

**文件：** `internal/tui/commands.go`
**依赖：** T26

**步骤：**

1. 定义 stream event、terminal error 与 closed Bubble Tea 消息。
2. 实现等待一个 Provider 事件或终结结果的 Cmd，不在 Update 中阻塞。
3. 依据通道契约处理正常 nil、context cancellation、业务错误和意外关闭。
4. 每次返回事件后保留通道引用，以便 Update 安排下一次等待。

**验证：** 运行 `go test ./internal/tui`，期望包编译通过。

## T29：实现提交与流状态更新

**文件：** `internal/tui/update.go`
**依赖：** T27、T28

**步骤：**

1. 空闲时将 textarea 更新委托给组件；Enter 对非空输入调用 Conversation.Start。
2. 提交后清空并失焦输入框，保存流通道并安排等待命令。
3. 收到 stream event 时调用 Apply、刷新 viewport，并继续等待。
4. 完成后调用 Complete；收到错误后区分取消与失败，恢复输入焦点。
5. 阻止生成期间再次提交。

**验证：** 运行 `go test ./internal/tui`，期望包编译通过。

## T30：实现 resize、滚动与自动跟随

**文件：** `internal/tui/update.go`、`internal/tui/view.go`
**依赖：** T29

**步骤：**

1. 处理 WindowSizeMsg，扣除输入框和状态栏高度后设置 viewport 尺寸。
2. 将滚动按键和鼠标/组件消息委托给 viewport。
3. 用户离开底部后关闭自动跟随；回到底部时恢复。
4. 新增流内容时仅在自动跟随开启时跳到底部。
5. resize 后保持合法 offset，不产生负尺寸或越界。

**验证：** 运行 `go test ./internal/tui`，期望包编译通过。

## T31：实现 thinking 切换、取消与退出

**文件：** `internal/tui/update.go`
**依赖：** T30

**步骤：**

1. `Ctrl+T` 切换最近一条含 thinking 消息的展开状态，无 thinking 时保持不变。
2. 活动生成期间 `Ctrl+C` 调用 Conversation.Cancel，并等待流终结后恢复输入。
3. 空闲时 `Ctrl+C` 返回退出命令。
4. 取消状态保留部分回复用于显示，但不更改 committed history。

**验证：** 运行 `go test ./internal/tui`，期望包编译通过。

## T32：测试 TUI 基础布局与交互

**文件：** `internal/tui/tui_test.go`
**依赖：** T31

**步骤：**

1. 测试初始 View 包含输入区、消息区和 idle 状态。
2. 测试中英文输入、Enter 提交、输入清空及生成中禁用再次提交。
3. 测试 WindowSizeMsg 后组件尺寸合法，小终端不 panic。
4. 测试长内容滚动、离底不自动跳转、回底恢复跟随。
5. 测试无 thinking 与有 thinking 时 `Ctrl+T` 行为。

**验证：** 运行 `go test ./internal/tui -run 'Test.*(View|Input|Resize|Scroll|Thinking)' -count=1`，期望全部通过。

## T33：测试 TUI 流式、错误与取消

**文件：** `internal/tui/tui_test.go`
**依赖：** T28、T32

**步骤：**

1. 向测试通道逐个发送 started、thinking、text、completed，逐次执行 Cmd/Update 并断言中间 View 已更新。
2. 断言 thinking 完成后自动折叠，signature 永不出现在 View。
3. 注入认证、限流、服务端和 stream 错误，断言安全摘要与输入恢复。
4. 测试生成期间 `Ctrl+C` 取消且下一轮仍可提交。
5. 测试空闲 `Ctrl+C` 产生退出命令。

**验证：** 运行 `go test ./internal/tui -count=1`，期望全部通过。

## T34：实现 TUI 运行入口

**文件：** `internal/tui/run.go`
**依赖：** T33

**步骤：**

1. 提供 `Run(conversation)` 创建 Model 和 Bubble Tea Program。
2. 启用 alternate screen，不启用本章范围外的鼠标功能。
3. 将 Program 运行错误返回调用方；退出时取消尚存活动请求。

**验证：** 运行 `go test ./internal/tui`，期望全部通过；运行 `go test ./internal/tui -run TestNonExistent`，期望包可独立编译。

## T35：实现 CLI 组装与 HTTP 超时

**文件：** `cmd/mewcode/main.go`
**依赖：** T21、T25、T34

**步骤：**

1. 将可测试的 `run(args, stderr)` 与 `main()` 分离。
2. 使用独立 FlagSet 解析 `--config`；未提供时调用 DefaultPath。
3. 加载配置并创建带 Dial、TLS handshake、response header、idle connection 超时且无 Client 总超时的 HTTP Client。
4. 依次创建 Provider、Conversation 并调用 TUI Run。
5. 初始化失败输出安全错误并返回非零退出码；正常退出返回零。

**验证：** 运行 `go test ./cmd/mewcode`，期望包编译通过；运行 `go build ./cmd/mewcode`，期望生成成功。

## T36：测试 CLI 参数、失败与脱敏

**文件：** `cmd/mewcode/main_test.go`
**依赖：** T35

**步骤：**

1. 注入加载和 TUI 运行依赖，避免测试进入真实终端。
2. 测试 `--config` 覆盖默认路径。
3. 测试未知参数、缺失文件、非法配置和 Provider 创建失败返回非零。
4. 使用独特假密钥断言 stderr 不含密钥。
5. 测试成功组装时调用 TUI 且返回零。

**验证：** 运行 `go test ./cmd/mewcode -count=1`，期望全部通过。

## T37：添加配置示例与忽略规则

**文件：** `config.example.yaml`、`.gitignore`
**依赖：** T5

**步骤：**

1. 添加 Anthropic 主示例，包含四个核心字段、max_tokens 和可选 thinking。
2. 用注释给出 OpenAI Responses 配置切换示例，不引入多 profile 结构。
3. 所有密钥值使用明显假值。
4. 忽略根目录常见本地配置名、`mewcode` 二进制、覆盖率和测试产物，不忽略示例文件或 docs。

**验证：** 运行 `git check-ignore config.example.yaml`，期望该文件不被忽略；运行 `git check-ignore mewcode.yaml`，期望该本地配置名被忽略。

## T38：编写用户文档

**文件：** `README.md`
**依赖：** T35、T37

**步骤：**

1. 说明 Go 1.25 要求、源码构建和运行方式。
2. 说明各平台默认配置目录及 `--config` 覆盖。
3. 解释四个核心字段、max_tokens、Anthropic thinking 约束和两种 base URL 示例。
4. 列出 Enter、滚动、`Ctrl+T`、`Ctrl+C` 的行为。
5. 明确 API Key 为明文，要求限制文件权限并禁止提交版本库。
6. 明确本章不包含 tool use、文件操作和会话持久化。

**验证：** 运行 `rg -n 'protocol|model|base_url|api_key|max_tokens|Ctrl\+T|Ctrl\+C|权限|版本控制' README.md`，期望每个主题都有匹配。

## T39：添加三平台 CI

**文件：** `.github/workflows/ci.yml`
**依赖：** T36

**步骤：**

1. 在 push 和 pull_request 上运行 Linux、macOS、Windows matrix。
2. 使用 Go 1.25，执行 `go test ./...`、`go vet ./...` 和 `go build ./cmd/mewcode`。
3. 添加 Linux 专属 `go test -race ./...`。
4. 不配置任何供应商密钥，确保测试完全离线于真实 API。

**验证：** 运行 YAML 解析检查，期望 workflow 语法有效；运行 `rg -n 'ubuntu|macos|windows|go test|go vet|go build|-race' .github/workflows/ci.yml`，期望所有平台和检查均存在。

## T40：执行完整本地验证

**文件：** 全部新增 Go 文件、`go.mod`、`go.sum`
**依赖：** T1–T39

**步骤：**

1. 对所有 Go 文件运行 `gofmt` 并确认无差异。
2. 运行 module tidy 与 verify，确认依赖锁定一致。
3. 运行全部单元和集成测试。
4. 运行 vet 与 race 测试。
5. 交叉构建 Linux、Darwin、Windows 的 `cmd/mewcode`，输出到临时目录而非仓库。
6. 检查仓库中不存在形似真实密钥的测试数据或本地配置。

**验证：** `gofmt -l .` 无输出；`go mod tidy` 后 Git 无额外依赖差异；`go mod verify`、`go test ./...`、`go vet ./...`、`go test -race ./...` 全部成功；三个目标平台均构建成功。

## 执行顺序

```text
T1
├─→ T2 → T3 ─┐
│       T4 ───┴→ T5 ───────────────────────────────┐
├─→ T6 → T7 → T8 → T9                              │
│             └→ T11 → T12 → T13 → T14 → T15 ─┐   │
│             └→ T16 → T17 → T18 → T19 → T20 ─┴→ T21
└─→ T10 ────────────────────────┘                   │
                                                    ├→ T35 → T36 → T39 ─┐
T7 → T22 → T23 → T24 → T25 → T26 → T27 → T28      │                    │
                                  └→ T29 → T30 → T31 → T32 → T33 → T34 ┘

T5 → T37 → T38 ─────────────────────────────────────────────────────────┐
T1–T39 ───────────────────────────────────────────────────────────────→ T40
```

可并行部分：T11–T15 与 T16–T20；配置链 T2–T5 与 Provider 基础链 T6–T10；文档 T37–T38 与后期 TUI 测试。

## 建议提交点

1. T1–T10：`chore: initialize module and core config/stream abstractions`
2. T11–T21：`feat: add anthropic and openai streaming providers`
3. T22–T25：`feat: add in-memory conversation lifecycle`
4. T26–T34：`feat: add streaming terminal chat interface`
5. T35–T39：`feat: wire mewcode cli and project documentation`
6. T40 修复（如有）：`test: complete chapter 2 acceptance hardening`
