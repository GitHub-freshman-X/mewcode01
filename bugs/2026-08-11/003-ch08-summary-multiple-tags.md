# 真实 API 摘要返回多个 summary 标签导致自动压缩失败

## 状态

待处理：已兼容多个彼此完整的 summary 块；真实 API 又返回了嵌套/连续重复开始标签的变体。

## 用户可见现象

首次真实 API 自动压缩已触发，但 TUI 显示：

```text
上下文压缩: automatic 失败: multiple summary tags
错误: request failed: multiple summary tags
```

兼容多个完整块后，真实 API 再次自动压缩时显示：

```text
上下文压缩: automatic 失败: summary tags are nested
错误: request failed: summary tags are nested
```

当前 Agent task 随即失败，原历史不被摘要替换。

## 根因

摘要提示要求只返回一个 `<summary>...</summary>`，但真实模型输出不稳定：先返回多个完整摘要块，随后又返回在第一个闭合标签前出现第二个开始标签的变体。当前扫描器将后一种形态判为嵌套并拒绝，避免将不明确的内容写入 history。摘要正文不写日志，因此尚不能判断它是连续重复开始标签、真正嵌套块，还是正文中包含了标签字面量。

## 影响范围

真实 Provider 未稳定遵守 XML 输出契约时，自动、手动、强制和紧急摘要可能输出多个完整块。

## 修复方案

- 加强摘要提示，禁止标签外文本、第二个 summary 标签及在正文中出现字面量 `<summary>`。
- 解析器扫描所有完整、非空、非嵌套 summary 块，只采用最后一个块；其余块和标签外文本不会写入 history。
- 缺失、空白、嵌套或未闭合标签仍失败，原历史保持不变。
- 完成日志仅记录候选数量、是否采用最后块和摘要长度，不记录摘要正文。

## 验证方式

- `go test ./internal/context ./internal/agent -count=1`
- `go test ./...`
- `go test -race ./internal/context ./internal/agent`

## 后续工作

1. 决定是否兼容嵌套/连续重复开始标签，并定义可安全选择的唯一摘要规则。
2. 在不记录摘要正文的前提下，增加标签序列形态的安全分类日志。
3. 根据决策补充失败测试、实现和真实 API 复测。
