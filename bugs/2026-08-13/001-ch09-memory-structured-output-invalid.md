# 自动记忆提取和治理缺少完整结构化输出协议，真实模型响应无法解析

## 状态

待真实 API 验证。

## 影响范围

第九章真实 Provider 场景中，自动记忆提取和惰性治理可能无法落盘；主任务仍显示成功，用户无法从 TUI 直接得知后台失败。

## 用户可见现象

用户请求记住 Go 偏好后，`memory/` 目录只保留既有 `MEMORY.md`，没有新增记忆文件。

## 复现证据

`/private/tmp/mewcode-ch09-manual.Gk0Y93/logs/2026/08/13/mewcode-20260813T082932.849595000-10188.jsonl` 记录两次 `memory_extract` 为 `response_invalid`，响应大小分别为 94 和 151 字节；对应 `memory_consolidation` 亦为 `response_invalid`。

## 根因

提取与治理请求只要求返回“受限 JSON 操作”，没有给出字段、类别、约束和完整 JSON 示例；本地解析器则只接受裸 JSON 数组。真实模型返回 fenced JSON 或说明文本时被严格解析拒绝。

## 修复方案

已为两个后台请求加入明确 schema 与示例；解析时只剥离单个 JSON Markdown 代码块，再保持既有操作与路径校验。TUI 仍不显示后台落盘状态，需通过文件系统和安全日志确认。

## 验证方式

- `TestMemoryServiceExtractAcceptsFencedJSONOperations` 已先失败后通过。
- `TestExtractionRequestDefinesOperationSchema` 已验证请求包含字段与四种类别。
- `go test ./internal/memory ./internal/agent ./cmd/mewcode -count=1` 于 2026-08-13 通过。
- `go vet ./...` 与 `git diff --check` 于 2026-08-13 通过。

## 后续工作

使用同一 fixture 重新执行真实 Provider 手工场景，确认用户级记忆文件和索引落盘；若仍失败，检查 `memory_extract` 的安全状态字段。
