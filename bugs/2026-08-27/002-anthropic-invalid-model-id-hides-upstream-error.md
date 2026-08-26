# Anthropic 请求超过 prompt cache 断点上限，返回 HTTP 400

## 状态

已修复。

## 现象

使用已修正模型 ID `claude-haiku-4-5-20251001` 后，简单问候请求仍返回 `Anthropic returned HTTP 400`，界面未显示上游的具体错误体。

## 根因

初始配置的 `claude-haiku-4.5` 确实不是 Anthropic API 模型 ID，已更正为 `claude-haiku-4-5-20251001`。但根本问题在于 Anthropic Provider 为稳定系统提示和每个可缓存工具定义都写入 `cache_control: {type: ephemeral}`。默认工具集使一次请求包含 7 个缓存断点，而 Anthropic Messages API 最多接受 4 个。

当前 Anthropic Provider 对非 2xx 响应只保留状态码并主动丢弃响应体，导致用户无法从界面辨别缓存断点超限和其他 400 原因。

## 处理建议

保留正确模型 ID。Anthropic Provider 现在仅对最后一个可缓存工具添加 `cache_control`，并保留稳定系统提示的缓存标记；一次请求最多产生两个缓存断点。后续可在不泄露密钥或敏感载荷的前提下，提取并展示上游响应中安全的错误摘要。

## 验证

- 同一模型、密钥、路径与请求头的无工具最小请求返回 HTTP 200。
- 加入 1 个系统缓存断点和 6 个工具缓存断点后稳定返回 HTTP 400，响应为：`A maximum of 4 blocks with cache_control may be provided. Found 7.`
- 修复后，1 个系统缓存断点和 6 个工具定义（仅最后一个工具带缓存断点）的真实代理请求返回 HTTP 200。
- `go test ./internal/provider/anthropic -run 'TestBuildRequest|TestRequestStreamThinking|TestParseAnthropicCacheUsage' -count=1` 通过。
