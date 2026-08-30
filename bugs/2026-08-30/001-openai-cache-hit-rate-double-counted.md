# OpenAI 缓存命中率重复计数

## 状态

已修复。

## 影响范围

使用 OpenAI Responses Provider 的会话累计缓存命中率、TUI 状态栏和 `/status`。Claude 的计量口径不同，不受本问题直接影响。

## 用户可见现象

实际会话显示 `Token：in:6222`、`缓存读取：4118`、`缓存写入：0`，但缓存命中率显示 `40%`。OpenAI 的 `input_tokens` 已是输入 Token 总数，`cached_tokens` 是其中的细分；正确比例应为 `4118 / 6222 = 66.18%`，四舍五入为 `66%`。

## 根因

当前统一公式把缓存读取 Token 加到普通输入分母：

```text
cache_read / (input + cache_read + cache_creation)
```

该公式适用于 Claude（其 `input_tokens` 不包含缓存读/写），但不适用于 OpenAI Responses API。代码在 Provider 归一化时丢失了计量口径，导致展示层无法按 Provider 正确计算。

## 修复方案

在 Provider 归一化的 usage 中携带“输入 Token 是否包含缓存读取/写入”的计量口径，或在 OpenAI adapter 中把 `input_tokens` 归一化为不含缓存读取的普通输入 Token；然后分别覆盖 OpenAI 和 Claude 的累计比例、持久化恢复、状态栏与 `/status` 测试。

同时解析 OpenAI Responses 的 `input_tokens_details.cache_write_tokens`，避免较新模型的缓存写入统计丢失。

## 验证方式

- OpenAI fixture：`input_tokens=6222`、`cached_tokens=4118`、`cache_write_tokens=0` 时显示 `66%`。
- Claude fixture：继续使用 `cache_read / (input + cache_read + cache_creation)`。
- 恢复新旧会话后，缓存数与对应 Provider 的命中率保持一致。

## 当前进展

2026-08-30：根据真实 `/status` 输出定位。此前的 `go test ./... -count=1` 只验证统一公式自身，未覆盖两个 Provider 的不同分母语义。

2026-08-30：已实现 Provider 计量口径字段、OpenAI `cache_write_tokens` 解析和新 JSONL 记录字段；旧缓存记录因缺少该口径将显示未知。已补充 OpenAI 66%、Claude 57%、解析和持久化恢复测试；相关包测试与 `go test ./... -count=1`、`go build ./cmd/mewcode`、`git diff --check` 均通过。
