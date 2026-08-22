# Provider 日志记录完整请求正文

- 状态：回归，待处理
- 发现日期：2026-08-14
- 影响范围：OpenAI provider 请求日志

## 现象

第九章手工测试日志的 `provider.request` 记录含有完整请求正文，包含 system prompt、用户输入、自定义指令和长期记忆内容。

## 风险

这违反项目对日志仅记录安全元数据的约束，可能使本地日志泄露敏感上下文。

## 建议修复

请求日志只保留 provider、模型、消息数、工具数、请求大小、耗时和状态等元数据；不得记录请求正文。

## 修复与验证

OpenAI 请求日志现仅记录 provider、请求大小、消息数和工具数，不再写入请求正文。

2026-08-14 10:18 的 fixture 日志仍写入 `request` 正文，表明该 fixture 尚未重新构建并使用本修复。

2026-08-14 09:29 第十章全量验收与单测 `go test ./internal/provider/openai -run '^TestStreamCapturesFinalRequestPayload$' -count=1` 均稳定失败：测试仍要求日志包含 `request-canary`，实际日志只有 `message_count`、`provider`、`request_bytes`、`stage` 和 `tool_count`。该现象符合当前安全日志实现，未发现第十章 Slash Command 改动影响 Provider。待处理项是更新该过时测试，使其断言请求正文和 API key 均不会进入日志，并确认 OpenAI 包全量测试通过。

2026-08-16：再次执行 `go test ./... -count=1` 时同一测试失败，输出仍只有安全元数据；其余第十章相关包均通过。本次未修改 Provider 实现或该测试，待后续独立修复。

2026-08-16：会话级 Token 用量改动后的全项目回归再次仅失败于同一断言；Provider 日志仍只包含安全元数据。会话、Agent、命令与 TUI 包均通过，本次未触及 OpenAI Provider。

2026-08-23：第十三章 Fork 参数兼容修复的全量回归再次仅失败于 `TestStreamCapturesFinalRequestPayload`。测试输出中的日志仍只含安全元数据，测试却期待 `request-canary` 出现；未触及 Provider 实现。该过时测试继续阻塞 `go test ./... -count=1` 的全绿验证，待独立修复。

2026-08-23 后续全量回归显示该问题已回归：`go test ./... -count=1` 在同一测试失败，日志 JSON 的 `fields.message` 直接包含 `request-canary`。源码 `internal/provider/openai/client.go` 也确认安全元数据日志调用被注释，而当前调用记录了 `message: body.Input`。这会泄露请求正文，违反 N5；与本次 TUI 改动无关，但阻塞全量测试且必须独立恢复仅记录 provider、请求大小、消息数和工具数的日志行为。全量测试同时再次生成 `cmd/mewcode/.mewcode/` 测试产物，已按关联问题处理。
