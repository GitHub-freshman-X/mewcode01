# 002 非 AppError 被统一提示隐藏真实原因

## 状态

已修复。

## 现象

Conversation、TUI 或工具事件处理产生普通 Go error 时，界面只显示：

```text
request failed; check your connection and configuration
```

该提示与真实原因可能完全无关。

## 影响

- 无法从界面判断失败发生在网络、Provider 流解析、Conversation 状态还是工具执行阶段。
- 容易把程序内部缺陷误诊为用户配置问题。
- 增加复现和调试成本。

## 根因

`provider.UserError` 只展示 `*provider.AppError` 的具体内容。其他 error 全部返回固定兜底文案，丢失了原始错误信息。

## 修复

调整 `provider.UserError`：

- `AppError` 继续显示结构化阶段和 HTTP 状态。
- 非空普通 error 显示 `request failed: <真实错误>`。
- nil error 使用简短兜底提示。

Provider 返回的错误仍通过已有的密钥清理逻辑处理，避免直接泄露凭据。

## 验证

- 普通内部错误现在能够显示具体原因。
- Provider 密钥脱敏测试继续通过。
- `go test ./...` 通过。
