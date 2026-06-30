# 001 普通文本完成事件被误判为工具调用完成

## 状态

已修复。

## 现象

模型已经正常输出回答，随后界面又显示请求失败。例如：

```text
MewCode
你好！有什么我可以帮您的吗？

错误: request failed; check your connection and configuration
```

## 影响

- 正常文本对话会在回答完成后被标记为失败。
- 用户容易误判为 API Key、网络或模型配置异常。
- 工具系统接入后，普通对话的稳定性受到影响。

## 根因

OpenAI Responses 流中的 `response.output_item.done` 不只用于 function call，也会用于普通 message。

原解析逻辑无条件把该事件转换为 `EventToolCallDone`。Conversation 随后尝试校验工具调用 ID 和名称，普通文本事件没有这些字段，因此产生内部错误。

## 修复

在 `internal/provider/openai/stream.go` 中检查 output item 类型：

- 只有 `function_call` 才转换为工具调用完成事件。
- 普通 `message` 的 `response.output_item.done` 被安全忽略。

## 验证

新增回归测试覆盖：

- 普通 message 的 output item done 不产生工具事件。
- function call 的 output item done 正常产生 `EventToolCallDone`。
- `go test ./...` 通过。
