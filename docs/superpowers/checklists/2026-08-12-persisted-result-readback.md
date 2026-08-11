# 已持久化工具结果回读识别 Checklist

## 回读识别

- [x] 已持久化工具结果在当前会话的 `tool-results` 目录生成文件，`ResultStore` 仅登记其规范绝对路径，不保存完整结果内容副本。
- [x] `read_file` 成功返回、且 `data.path` 精确等于当前 `ResultStore` 已登记路径时，`PrepareResults` 原样保留该工具结果，并且不产生新的 `Persistence`。
- [x] 即使回读 JSON 的 `data.content` 是已持久化文件的截断前缀、与完整原文不相等，只要 `data.path` 命中，仍不再次持久化。
- [x] 路径未登记时，即使 `data.content` 恰好等于某个已持久化原文，也不获得豁免；超限结果会按既有规则重新持久化。
- [ ] `read_file` 返回失败、JSON 非法、`success:false`、缺失 `data.path` 或路径不能规范化时，均不获得豁免。

## 范围与回归

- [ ] `read_file` 文件上限仍为 512 KiB、输出上限仍为 64 KiB；本次不新增 offset/limit 参数。
- [x] `PrepareResults` 的单项阈值、消息聚合阈值、按长度降序替换和原始结果顺序保持不变。
- [x] `go test ./internal/context -count=1` 通过。
- [ ] `go test ./internal/agent -run 'Context|Persist|Readback' -count=1` 仍由独立问题 002 失败；相关持久化 Agent 测试已通过。
- [x] `gofmt -w internal/context/results.go internal/context/context_test.go` 后，`git diff --check` 无输出。

## 记录与提交

- [x] `bugs/2026-08-12/001-ch08-persisted-readback-content-copy.md` 记录根因、路径集合修复方案、实际验证命令与最终状态。
- [x] `bugs/README.md` 与当天 README 索引已同步。
- [ ] 修复提交不包含用户原有的 `internal/provider/openai/client.go` 修改和 `docs/superpowers/plans/2026-08-11-summary-diagnostics.md` 删除。
