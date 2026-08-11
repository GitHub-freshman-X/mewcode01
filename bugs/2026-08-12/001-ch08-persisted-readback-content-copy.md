# 已持久化工具结果回读按内容识别导致内存复制和截断回读失效

## 状态

已修复。

## 影响范围

第八章第一层工具结果持久化。大工具结果的完整内容会被 `ResultStore` 再保留一份；当后续 `read_file` 因输出上限返回截断内容时，系统会把该回读结果再次持久化。

## 根因

`ResultStore.contents` 以完整工具结果文本为集合键，`Manager.isReadback` 也以 `data.content` 的完全相等判断是否放行。该判断既复制大内容，也不能识别被截断的回读内容。

## 当前进展

已将 `ResultStore.contents` 替换为 `paths`。持久化成功后登记规范绝对路径；`isReadback` 只解析 `read_file` 的成功结构化返回，并以 `data.path` 的规范绝对路径精确匹配当前会话登记集合。完整工具结果不再被 `ResultStore` 复制保存。

## 验证

- `go test ./internal/context -run 'TestPrepareResultsDoesNotRepersistWrappedReadFileContent|TestPrepareResultsRepersistsReadFileFromUnregisteredPath' -count=1`：修改前按预期失败；修复后通过。
- `go test ./internal/context -count=1`：通过。
- `go test ./internal/agent -run 'TestRunnerPersistsToolResultsBeforeCommit|TestRunnerLogsToolResultPersistenceAndEmergencyRetry' -count=1`：通过。

完整 `Context|Persist|Readback` Agent 筛选会被独立待处理问题 [002](002-ch08-context-log-summary-test-mismatch.md) 阻断；该失败与本修复无共同修改文件。
