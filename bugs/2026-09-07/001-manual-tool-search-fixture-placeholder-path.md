# 本地 ToolSearch 对中英文不匹配查询返回空目录

**状态：** 已修复

## 现象

本地 `tool_search` 成功执行但返回空 `tools` 数组，尽管 MCP `demo__lookup_fixture` 已成功注册。

## 根因

本地 ToolSearch 当前仅以查询字符串在工具唯一名与 description 中做小写字面子串匹配。人工提示使用中文“能返回固定验证标记的 MCP 工具”，但 fixture 工具名为 `demo__lookup_fixture`、description 为英文 `Return the fixed manual Tool Search verification token.`，没有共同子串，因此返回空数组。

## 最小修复

人工 fixture 已将 description 补充为与测试提示一致的中文能力描述，测试输入改为搜索“固定验证标记”。该场景现在验证正常关键词搜索，不再依赖 `select:` 精确选择。跨语言语义搜索不属于当前本地字符串匹配的范围，后续如需支持应独立设计。

## 验证

修复方式：更新 fixture 的 MCP description 与人工场景输入，使关键词“固定验证标记”成为 description 的字面子串；重启 fixture 后可由现有 `strings.Contains` 搜索逻辑命中。

2026-09-07 真实验证：最新 OpenAI 与 Anthropic 会话均以该关键词命中 `demo__lookup_fixture`。
