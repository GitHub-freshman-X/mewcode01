# 本地 ToolSearch 将多关键词整句作为子串匹配，真实 MCP 无法发现

**状态：** 已修复

## 现象

真实 Context7 MCP 已连接并注册 `context7__resolve-library-id`、`context7__query-docs`，但模型以“Context7 React documentation useEffect cleanup”等多关键词查询调用本地 `tool_search` 时连续获得空结果。

## 根因

本地搜索将整个查询字符串作为一个字面子串与单个工具名或 description 比较。Context7 单个工具的名称和说明不会包含这整句，因此即使各关键词分别具有意义，也不会命中。

## 真实验证

2026-09-07 最新 OpenAI 会话三次 `tool_search` 均返回空数组，随后模型结束。最新 Anthropic 会话也先搜索失败；模型依靠启动时可见的工具名绕过搜索，并在修正参数后成功调用 Context7，但会话未留下最终总结，故不满足场景 E 的完整验收。

## 建议修复

按已确认的产品契约移除关键词查询：模型从稳定名称目录选择完整唯一名，`tool_search` 仅以 `tool_name` 精确加载 schema。这样避免自然语言与 description 不一致造成的伪发现失败。
