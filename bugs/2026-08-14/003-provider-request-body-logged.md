# Provider 日志记录完整请求正文

- 状态：已修复，待完整验证
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
