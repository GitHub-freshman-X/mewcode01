# 003 工具结果在 TUI 中可能重复展示

## 状态

待修复。

## 现象

一次工具调用成功后，相同结果可能在界面中出现两次：

```text
工具结果: write_file 成功
{"tool_name":"write_file","success":true,...}

工具结果: write_file 成功
{"tool_name":"write_file","success":true,...}
```

工具结果目前还使用“你”作为消息标签，容易让用户误认为 JSON 是自己输入的内容。

## 影响

- 用户无法判断工具是否实际执行了两次。
- 写文件、运行命令等有副作用的工具会引发额外担忧。
- 对话历史可读性较差。

## 已确认的代码问题

### Conversation 缺少幂等提交保护

`Conversation.Complete()` 每次调用都会把 active turn 和工具结果追加到 history。Turn 没有 committed 标记；如果同一响应收到重复完成事件，存在重复提交和重复执行工具的风险。

### TUI 同时渲染 history 和已提交的 active turn

工具完成后状态为 `TurnToolCompleted`。TUI 目前只在状态等于 `TurnCompleted` 时停止渲染 active turn，因此已经写入 history 的内容仍可能从 active turn 路径再次展示。

### 工具结果沿用 user 标签

Provider 协议要求 tool result 以 user 侧消息回灌，但这不代表 TUI 应把它显示成用户输入。协议角色和界面角色没有分离。

## 建议修复

1. 为 Turn 增加 committed 状态，保证 `Complete()` 幂等。
2. 工具结果写入 history 后，将 active turn 标记为不可再次渲染，或清空 active turn。
3. TUI 为 `BlockToolResult` 使用独立的“工具”标签。
4. 默认显示工具名称、成功状态和摘要，原始 JSON 可折叠或按需展示。
5. 增加测试，断言一次完成事件和重复完成事件都只产生一份历史与一次工具执行。

## 验证计划

- 单次工具调用的结果在 View 中只出现一次。
- 重复 `EventCompleted` 不会重复执行工具。
- 下一轮请求仍能看到唯一的一份 tool result 历史。
- 工具结果不再显示为“你”。
